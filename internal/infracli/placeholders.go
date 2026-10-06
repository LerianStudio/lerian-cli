package infracli

import (
	"context"
	"fmt"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// resolvePlaceholders asks for the tokens a template still has in it, so the
// file that gets written can be applied rather than edited first.
//
// Until now init wrote <ROUTE53-ZONE-ARN> into the file, mentioned it in a
// column of the summary, and the run that followed refused at the preflight —
// after every other question had been answered. The value was always knowable at
// the point the file was written; nobody asked for it.
//
// Only what is still unresolved: the egress address has its own question, --set
// answers any token from the command line, and asking again for something
// already answered is how a wizard becomes something to click through.
func resolvePlaceholders(
	ctx context.Context,
	ask *prompter,
	zones infra.ZoneLister,
	units []infra.Unit,
	environment, profile string,
	replacements map[string]string,
) error {
	pending, err := pendingTokens(units, environment, replacements)
	if err != nil || len(pending) == 0 {
		return err
	}
	if !ask.interactive {
		// Nobody to ask. The file is written with the token in it and the summary
		// says so, which is what happened before this existed — a scripted run has
		// --set for exactly this.
		return nil
	}

	theme := newStyle(ask.out)
	fmt.Fprintf(ask.out, "\n%s\n", theme.bold("==> Values the templates still need"))
	fmt.Fprintf(ask.out, "  %s\n", theme.dim(
		"left unanswered these are written as-is, and the run refuses at the preflight"))

	for _, token := range pending {
		value, askErr := askForPlaceholder(ctx, ask, zones, token, profile)
		if askErr != nil {
			return askErr
		}
		replacements[token] = value
	}
	return nil
}

// pendingTokens is every placeholder across these roots that nothing has
// answered yet, in a stable order.
func pendingTokens(units []infra.Unit, environment string, answered map[string]string) ([]string, error) {
	seen := map[string]bool{}
	var pending []string

	for _, unit := range units {
		tokens, err := infra.PlaceholdersIn(unit, environment)
		if err != nil {
			return nil, err
		}
		for _, token := range tokens {
			// The egress address is asked for by name, before this runs.
			if infra.IsEgressPlaceholder(token) || answered[token] != "" || seen[token] {
				continue
			}
			seen[token] = true
			pending = append(pending, token)
		}
	}
	return pending, nil
}

// askForPlaceholder asks for one token, from the account where the account can
// answer it.
func askForPlaceholder(
	ctx context.Context,
	ask *prompter,
	zones infra.ZoneLister,
	token, profile string,
) (string, error) {
	if isHostedZoneToken(token) {
		if value, ok, err := askForHostedZones(ctx, ask, zones, token, profile); err != nil || ok {
			return value, err
		}
		// The account could not be asked. Fall through to typing it, rather than
		// failing a setup over a lookup that is a convenience.
	}

	return ask.ask("What is "+token+"?",
		"The templates leave this to you; it is written into the variables file as given.",
		"", "--set "+token+"=<value>")
}

// isHostedZoneToken reports whether a token wants a Route53 zone.
//
// Matched on the token's own words rather than on a list of known tokens: the
// templates are a separate repository on their own release cycle, and a build of
// this CLI that only recognized today's spelling would stop helping the moment
// they added <ROUTE53-PUBLIC-ZONE-ARN>.
func isHostedZoneToken(token string) bool {
	upper := strings.ToUpper(token)
	return strings.Contains(upper, "ZONE") && strings.Contains(upper, "ROUTE53")
}

// askForHostedZones offers the account's public zones. The second return is
// false when the account could not be asked, which is a reason to type the value
// rather than to fail.
func askForHostedZones(
	ctx context.Context,
	ask *prompter,
	zones infra.ZoneLister,
	token, profile string,
) (string, bool, error) {
	if zones == nil {
		return "", false, nil
	}

	found, err := zones.ListHostedZones(ctx, profile)
	if err != nil {
		fmt.Fprintf(ask.out, "  %s\n", newStyle(ask.out).dim(
			"could not list the hosted zones in this account: "+err.Error()))
		return "", false, nil
	}

	options := hostedZoneOptions(found)
	if len(options) == 0 {
		fmt.Fprintf(ask.out, "  %s\n", newStyle(ask.out).dim(
			"no public hosted zone in this account to offer"))
		return "", false, nil
	}

	picked, err := ask.pickMany("Which hosted zone(s) may the cluster write to?",
		"For "+token+". ExternalDNS and cert-manager publish records the internet must see.",
		"--set", options, nil)
	if err != nil {
		return "", false, err
	}

	// Joined with the quotes the template already has around the token:
	// external_dns_hosted_zone_arns = ["<ROUTE53-ZONE-ARN>"] becomes a list of
	// however many were picked.
	return strings.Join(picked, `", "`), true, nil
}

// hostedZoneOptions is the public zones, since these roles publish records the
// internet has to resolve. A private zone would be accepted by IAM and then
// never serve the record anybody was waiting for.
func hostedZoneOptions(zones []infra.HostedZone) []option {
	options := make([]option, 0, len(zones))
	for _, zone := range zones {
		if zone.Private {
			continue
		}
		options = append(options, option{
			value: zone.ARN(),
			label: zone.Name,
			note:  zone.ID,
		})
	}
	return options
}

// fillPendingVarFiles offers to fill the tokens a written variables file still
// has, and returns the readiness as it stands afterwards.
//
// This is the same question resolvePlaceholders asks at init time, asked again
// where the refusal actually lands. The file may predate the question — written
// by an older build, or by hand — and a run that stops with "replace every <...>
// token" has just proved it knows exactly which ones, in which file, while
// somebody is sitting in front of it.
//
// Anything that is not a placeholder problem is left alone: a missing file is a
// different failure with a different answer.
func fillPendingVarFiles(
	ctx context.Context,
	ask *prompter,
	zones infra.ZoneLister,
	readiness []infra.Readiness,
	environment, profile string,
) []infra.Readiness {
	if !ask.interactive {
		return readiness
	}

	var pending []infra.Unit
	for _, entry := range readiness {
		if !entry.Ready() && strings.Contains(entry.Problem, "unresolved placeholder") {
			pending = append(pending, entry.Unit)
		}
	}
	if len(pending) == 0 {
		return readiness
	}

	values := map[string]string{}
	err := resolveWrittenPlaceholders(ctx, ask, zones, pending, environment, profile, values)
	//nolint:nilerr // Declining is leaving the files as they were. q, r and ctrl-c
	// all arrive as an error here, and so does a failure to read one of the files;
	// either way nothing was filled in, and the run goes on to refuse with the
	// same message it would have refused with. Returning the error instead would
	// replace a precise "these lines still have tokens" with whatever went wrong
	// in the offer to fix them.
	if err != nil {
		return readiness
	}
	if len(values) == 0 {
		return readiness
	}

	for _, unit := range pending {
		changed, err := infra.FillPlaceholders(unit, environment, values)
		if err != nil {
			fmt.Fprintf(ask.out, "  could not write %s: %v\n", infra.VarFile(unit, environment), err)
			continue
		}
		if changed > 0 {
			fmt.Fprintf(ask.out, "  filled %d line(s) in %s\n", changed, infra.VarFile(unit, environment))
		}
	}
	fmt.Fprintln(ask.out)

	// Read back rather than assumed: what the file holds now is what the run is
	// about to use, and a value that did not land has to stop the run.
	return infra.CheckReadiness(unitsOf(readiness), environment)
}

// resolveWrittenPlaceholders asks for the tokens these written files still hold.
func resolveWrittenPlaceholders(
	ctx context.Context,
	ask *prompter,
	zones infra.ZoneLister,
	units []infra.Unit,
	environment, profile string,
	values map[string]string,
) error {
	seen := map[string]bool{}
	var tokens []string
	for _, unit := range units {
		found, err := infra.PendingPlaceholders(unit, environment)
		if err != nil {
			return err
		}
		for _, token := range found {
			if !seen[token] {
				seen[token] = true
				tokens = append(tokens, token)
			}
		}
	}
	if len(tokens) == 0 {
		return nil
	}

	theme := newStyle(ask.out)
	fmt.Fprintf(ask.out, "\n%s\n", theme.bold("==> Values these templates still need"))
	fmt.Fprintf(ask.out, "  %s\n", theme.dim("answer them here and the run carries on; leave and nothing is written"))

	for _, token := range tokens {
		value, err := askForPlaceholder(ctx, ask, zones, token, profile)
		if err != nil {
			return err
		}
		values[token] = value
	}
	return nil
}

// unitsOf is the roots a readiness report covers.
func unitsOf(readiness []infra.Readiness) []infra.Unit {
	units := make([]infra.Unit, 0, len(readiness))
	for _, entry := range readiness {
		units = append(units, entry.Unit)
	}
	return units
}

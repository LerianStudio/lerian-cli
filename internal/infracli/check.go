package infracli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// checkUsage is printed by `lerian infra check --help`.
const checkUsage = `lerian infra check — is this machine able to run the CLI

Usage:
  lerian infra check [flags]

Reports every dependency this tool shells out to, and the templates checkout it
would use, in one pass. It makes no AWS call and reads no environment
configuration, so it answers one question only: can this machine run
lerian infra at all.

The same checks run during a normal invocation, each one immediately before the
first call that needs it. That ordering is deliberate — what is verified is
exactly what is about to be used — but it surfaces problems one at a time, so a
bare machine is fixed over as many rounds as it has gaps. This command collects
them instead.

Exits non-zero when anything is missing, which makes it usable as a CI gate.

Flags:
  --repo <path>           the checkout to report on. Skips discovery.
  --templates-dir <path>  where the managed checkout lives
                          (default ~/lerian/lerian-terraform-foundation)
  -h, --help              this message
`

// checkResult is one row of the report. Detail carries the full remediation of a
// failure, which the underlying errors already write well, so this command adds
// nothing to it beyond a place to print it.
type checkResult struct {
	name    string
	summary string
	detail  string
	ok      bool
}

func runCheck(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var opts struct {
		repo         string
		templatesDir string
	}

	flags := flag.NewFlagSet("lerian infra check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, checkUsage) }
	flags.StringVar(&opts.repo, "repo", "", "path to the checkout")
	flags.StringVar(&opts.templatesDir, "templates-dir", "",
		"where the managed checkout lives (default ~/lerian/lerian-terraform-foundation)")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if rest := flags.Args(); len(rest) > 0 {
		return fmt.Errorf("unexpected argument %q\nRun lerian infra check --help", rest[0])
	}

	results := []checkResult{
		checkAWSCLI(ctx),
		checkTerraform(ctx),
		checkGit(),
		checkTemplates(ctx, opts.repo, os.Getenv("LERIAN_TF_REPO"), opts.templatesDir),
	}

	return reportChecks(stdout, results)
}

// checkAWSCLI defers to the same verification a real run makes, which covers the
// major version too: v1 cannot export a profile's credentials.
func checkAWSCLI(ctx context.Context) checkResult {
	result := checkResult{name: "aws", summary: binaryPath("aws"), ok: true}
	if err := infra.RequireAWSCLI(ctx); err != nil {
		result.ok = false
		result.summary = "not usable"
		result.detail = err.Error()
	}
	return result
}

// checkTerraform builds the same CLI a run builds. Constructing it is the check:
// it resolves the binary and rejects anything below MinTerraformVersion.
func checkTerraform(ctx context.Context) checkResult {
	_, err := infra.NewCLI(ctx)
	return terraformResult(err)
}

// terraformResult turns the outcome of building the CLI into a row. It is split
// from checkTerraform so a run can gate on the same verification without paying
// for a second version probe: NewCLI shells out to the binary.
func terraformResult(err error) checkResult {
	result := checkResult{name: "terraform", summary: binaryPath("terraform"), ok: true}
	if err != nil {
		result.ok = false
		result.summary = "not usable"
		result.detail = err.Error()
	}
	return result
}

func checkGit() checkResult {
	result := checkResult{name: "git", summary: binaryPath("git"), ok: true}
	if _, err := infra.NewGitCLI(); err != nil {
		result.ok = false
		result.summary = "not usable"
		result.detail = err.Error()
	}
	return result
}

// checkTemplates reports which checkout a run would resolve to, and at which
// version. Discovery has more than one source — a flag, $LERIAN_TF_REPO, the
// working directory and its parents, the managed path — so naming the winner is
// worth a row of its own. It has to resolve from the same inputs a run does, or
// the row reports a checkout that is not the one about to be used.
func checkTemplates(ctx context.Context, repo, envRepo, templatesDir string) checkResult {
	result := checkResult{name: "templates", ok: true}

	layout, source, err := resolveLayout(repo, envRepo, templatesDir)
	if err != nil {
		result.ok = false
		result.summary = "not found"
		result.detail = err.Error()
		return result
	}
	result.summary = templatesLine(ctx, layout, source)
	return result
}

// checkAWSSession asks whether the operator is logged in at all: it resolves
// every profile in ~/.aws and passes when at least one of them answers.
//
// Which profile is the right one is a later question — the environment decides
// that, and the account guard checks it against environments.conf. This one only
// separates "logged in somewhere" from "logged in nowhere", because the second is
// the state in which no stage can run and every question asked first is wasted.
//
// The only check here that makes an AWS call, which is why it is not part of
// `lerian infra check` unless asked for: that command is a CI gate, and a gate
// that needs credentials cannot run before they exist.
func checkAWSSession(ctx context.Context, identity infra.Identity) checkResult {
	result := checkResult{name: "aws session", ok: true}

	profiles, err := infra.ListAWSProfiles()
	if err != nil {
		result.ok = false
		result.summary = "cannot read ~/.aws"
		result.detail = err.Error()
		return result
	}
	if len(profiles) == 0 {
		result.ok = false
		result.summary = "no profiles"
		result.detail = "There is no profile in ~/.aws to log in with. Create one:\n\n" +
			"  aws configure sso\n\n" +
			"Then run this command again."
		return result
	}

	resolved := infra.ResolveProfiles(ctx, identity, profiles, "")

	var usable []string
	for _, entry := range resolved {
		if entry.Usable() {
			usable = append(usable, entry.Profile.Name)
		}
	}
	if len(usable) == 0 {
		hint := infra.LoginHint(resolved)
		if hint == "" {
			hint = "Check the credentials for these profiles."
		}
		result.ok = false
		result.summary = "not logged in"
		result.detail = fmt.Sprintf("None of the %d profile(s) in ~/.aws resolve right now.\n"+
			"An expired SSO session is the usual cause. This revives them:\n\n  %s\n\n"+
			"Then run this command again.", len(resolved), hint)
		return result
	}

	result.summary = fmt.Sprintf("%d of %d profiles resolve: %s",
		len(usable), len(resolved), strings.Join(usable, ", "))
	return result
}

// binaryPath is for the report only. The verification is the infra function next
// to it; this just says where the binary it accepted lives, because "ok" without
// a path hides a second copy earlier in PATH.
func binaryPath(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return "not found in PATH"
	}
	return path
}

func reportChecks(out io.Writer, results []checkResult) error {
	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> Environment check"))

	width := 0
	for _, r := range results {
		if len(r.name) > width {
			width = len(r.name)
		}
	}

	failed := 0
	for _, r := range results {
		mark := "ok"
		if !r.ok {
			mark = theme.alert("missing")
			failed++
		}
		fmt.Fprintf(out, "  %-*s  %-52s %s\n", width, r.name, r.summary, mark)
	}

	// The remediations come after the table rather than inline, so the table stays
	// scannable when several things are wrong — which is the case this command
	// exists for.
	for _, r := range results {
		if r.ok || r.detail == "" {
			continue
		}
		// indent() leaves the first line alone, because its other callers place it
		// themselves. Here the whole block is being pushed under a heading.
		fmt.Fprintf(out, "\n%s\n%s\n", theme.bold("  "+r.name), indent("    "+r.detail, "    "))
	}

	if failed == 0 {
		fmt.Fprintf(out, "\n  %d checks, all ok. No AWS call was made.\n", len(results))
		return nil
	}
	fmt.Fprintf(out, "\n  %d of %d checks failed.\n", failed, len(results))
	return fmt.Errorf("check: %s", strings.Join(failedNames(results), ", "))
}

func failedNames(results []checkResult) []string {
	var names []string
	for _, r := range results {
		if !r.ok {
			names = append(names, r.name)
		}
	}
	return names
}

// requireEnvironment verifies what a run is about to use and reports every gap
// at once, rather than returning at the first.
//
// It gates on exactly what the run needs and nothing more. git stays out: a run
// never clones, only init does, and demanding a tool that will not be called is
// the failure this package's ordering exists to avoid. The AWS CLI is exempt on
// a dry run for the same reason — a dry run makes no AWS call. The templates
// checkout is not here either, because a run cannot reach this point without
// having resolved it.
//
// On success it prints nothing: the run has its own preflight block, and a
// preflight is everything that has to be true before the first question, run in
// one pass and reported together.
//
// One pass rather than one at a time. A run verifies each dependency immediately
// before the call that needs it, which is right for a scripted invocation and
// wrong for a person sitting at a menu: they fix terraform, run again, and only
// then hear that the AWS CLI is v1. Collapsing those rounds is the whole point of
// this block, and of `lerian infra check`.
//
// Before the questions rather than after them, for the same reason. The three
// questions take real thought — which environment, which stacks, plan or apply —
// and a machine that cannot run anything makes all three answers worthless.
//
// identity is a variable so the session check can be exercised without an AWS
// account behind it.
var checkIdentity infra.Identity = infra.CLIIdentity{}

func preflight(
	ctx context.Context,
	out io.Writer,
	layout infra.Layout,
	source checkoutSource,
	dryRun bool,
) (*infra.CLI, error) {
	terraform, tfErr := infra.NewCLI(ctx)

	results := []checkResult{terraformResult(tfErr)}

	awsCLI := checkResult{name: "aws", ok: true}
	if !dryRun {
		awsCLI = checkAWSCLI(ctx)
		results = append(results, awsCLI)
	}

	// No git. It is only used by init, to clone, and a run that already has its
	// checkout never calls it — gating on it would demand a tool this command does
	// not need. `lerian infra check` reports it, because that command answers the
	// wider question of whether the machine can do everything.
	results = append(results, templatesResult(ctx, layout, source))

	// Only worth asking when there is an AWS CLI to ask with: without one the
	// session check fails for the reason already on the line above it, and two
	// rows for one problem read as two problems.
	if !dryRun && awsCLI.ok {
		results = append(results, checkAWSSession(ctx, checkIdentity))
	}

	for _, r := range results {
		if !r.ok {
			return nil, reportChecks(out, results)
		}
	}
	return terraform, nil
}

// templatesResult reports the checkout a run has already resolved — which repo,
// from which source, at which version. It takes the resolved layout rather than
// resolving its own, so the row cannot name a checkout other than the one about
// to be used.
func templatesResult(ctx context.Context, layout infra.Layout, source checkoutSource) checkResult {
	return checkResult{name: "templates", summary: templatesLine(ctx, layout, source), ok: true}
}

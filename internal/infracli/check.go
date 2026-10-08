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
                          (default ~/.lerian/lerian-terraform-foundation)
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
	// optional marks a row that is reported but never gates. Nothing in a run
	// needs it; it is here so somebody reading the table knows what this machine
	// can and cannot be asked to do, which is a different question from whether
	// the run will work.
	optional bool
}

// blocking is a row that stops a run. An optional row that failed is a fact
// about the machine, not a failure of the check.
func (r checkResult) blocking() bool { return !r.ok && !r.optional }

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
		"where the managed checkout lives (default ~/.lerian/lerian-terraform-foundation)")

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
		checkGH(ctx),
		checkTemplates(ctx, opts.repo, os.Getenv("LERIAN_TF_REPO"), opts.templatesDir),
	}

	// This command reaches no AWS API at all, which is what makes it usable as a
	// CI gate — before any credential exists.
	return reportChecks(stdout, results, " No AWS call was made.")
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
// checkGH reports the GitHub CLI, and whether it is logged in.
//
// Optional, and it has to stay that way. Nothing in the deploy itself touches
// GitHub: the one thing that does is the export, which is offered after a run
// has finished and asks for gh at the moment it needs it. A row that failed a
// run over a tool the run never calls is the same mistake as gating a run on
// git.
//
// Logged in or not is part of the row rather than a second one. "installed" is
// not the useful fact; "can create a repository right now" is, and those differ
// by a login.
func checkGH(ctx context.Context) checkResult {
	result := checkResult{name: "gh", summary: binaryPath("gh"), ok: true, optional: true}

	gh, err := infra.NewGHCLI()
	if err != nil {
		result.ok = false
		result.summary = "not installed — only for creating the exported repository on GitHub"
		return result
	}

	account, loggedIn := gh.GHStatus(ctx)
	if !loggedIn {
		// Not ok: the row exists to say whether a repository can be created from
		// here, and a logged-out gh cannot. optional keeps it out of the verdict,
		// so this reads "absent" in grey rather than failing anything.
		result.ok = false
		result.summary = binaryPath("gh") + "  (not logged in — gh auth login, or the CLI offers it)"
		return result
	}
	if name := account.String(); name != "not logged in" {
		result.summary += "  (" + name + ")"
	}
	return result
}

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
func checkAWSSession(ctx context.Context, identity infra.Identity) (checkResult, []infra.ResolvedProfile) {
	result := checkResult{name: "aws session", ok: true}

	profiles, err := infra.ListAWSProfiles()
	if err != nil {
		result.ok = false
		result.summary = "cannot read ~/.aws"
		result.detail = err.Error()
		return result, nil
	}

	// No profiles is not the same as no credentials. CI exports them, and so does
	// anyone who has a key rather than a portal — neither has a ~/.aws to read, and
	// both can deploy.
	if len(profiles) == 0 {
		if caller, err := identity.CallerIdentity(ctx, "", ""); err == nil && caller.Account != "" {
			result.summary = "credentials in the environment reach account " + caller.Account
			return result, []infra.ResolvedProfile{{Caller: caller}}
		}

		result.ok = false
		result.summary = "no credentials"
		result.detail = "There is nothing in ~/.aws and no credentials in the environment.\n" +
			"Either sign in to an IAM Identity Center portal, or configure an access key:\n\n" +
			"  aws configure sso\n" +
			"  aws configure\n\n" +
			"Then run this command again."
		return result, nil
	}

	resolved := infra.ResolveProfiles(ctx, identity, profiles, "")

	var usable []string
	for _, entry := range resolved {
		if entry.Usable() {
			usable = append(usable, entry.Profile.Name)
		}
	}
	if len(usable) == 0 {
		result.ok = false
		result.summary = "not logged in"
		result.detail = sessionAdvice(resolved)
		return result, resolved
	}

	result.summary = fmt.Sprintf("%d of %d profiles resolve: %s",
		len(usable), len(resolved), nameAFew(usable))
	return result, resolved
}

// sessionAdvice is what to do about no profile resolving.
//
// Two different failures used to share one message. With nothing to log into —
// profiles holding static keys, or a [default] carrying only a region — "an
// expired SSO session is the usual cause, this revives them" names a cause that
// does not exist and then offers a sentence where a command should be. A client
// who has never used SSO was told to renew a session they never had.
func sessionAdvice(resolved []infra.ResolvedProfile) string {
	if hint := infra.LoginHint(resolved); hint != "" {
		return fmt.Sprintf("None of the %d profile(s) in ~/.aws resolve right now.\n"+
			"An expired SSO session is the usual cause. This revives them:\n\n  %s\n\n"+
			"Then run this command again.", len(resolved), hint)
	}
	return fmt.Sprintf("None of the %d profile(s) in ~/.aws resolve right now, and none\n"+
		"of them can be signed in to: they carry no SSO configuration, so there is\n"+
		"no session to renew. Give one of them working credentials:\n\n"+
		"  aws configure sso --profile <name>      # IAM Identity Center\n"+
		"  aws configure --profile <name>          # access key and secret\n\n"+
		"Then run this command again.", len(resolved))
}

// nameAFew lists the first few names and counts the rest.
//
// The row answers one question — is there a session at all — and the count is
// the answer. The names are there so a machine with the wrong profiles configured
// is recognizable at a glance; all nine of them turn a one-line verdict into a
// wrapped paragraph, on exactly the machine this tool is built for, where there
// is a profile per account.
func nameAFew(names []string) string {
	const most = 3
	if len(names) <= most {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:most], ", "), len(names)-most)
}

// ssoLogin is a variable so the offer can be exercised without an AWS account
// and without opening a browser.
var ssoLogin = infra.SSOLogin

// awsConfigure is a variable for the same reason: so the setup can be exercised
// without writing into anybody's ~/.aws.
var awsConfigure = infra.ConfigureAWS

// offerSetup asks how this machine should get credentials, and runs it.
//
// A machine with nothing in ~/.aws is the normal state of a machine somebody has
// just been handed, and this CLI is not only run by people who already have our
// profiles configured. Reporting "create a profile, then run this again" makes
// the first experience of the tool a round trip through a second program.
func offerSetup(ctx context.Context, ask *prompter, out io.Writer) bool {
	if ask == nil || !ask.interactive {
		return false
	}

	answer, err := ask.pick("This machine has no AWS credentials. Set them up now?",
		"Either one writes to ~/.aws, which is where every AWS tool reads them from.", "",
		[]option{
			{value: "sso", label: "sign in to an SSO portal",
				note: "aws configure sso — what an organization hands out; opens a browser"},
			{value: "keys", label: "use an access key",
				note: "aws configure — an access key id and secret"},
			{value: "no", label: "not now", note: "leaves the instructions below"},
		}, "")
	// Declining is not a failure of its own, and neither is a selector that could
	// not draw: the check has already written what to run by hand, and replacing
	// that with "the prompt failed" helps nobody. Nothing here produces an error
	// the caller could act on, so none is returned.
	if err != nil || answer == "no" {
		return false
	}

	fmt.Fprintln(out)
	if err := awsConfigure(ctx, answer, os.Stdin, out, out); err != nil {
		fmt.Fprintf(out, "\n  %v\n", err)
		return false
	}
	return true
}

// loginTargets is what can be logged into, one per SSO session, in the order the
// sessions were first seen.
//
// One per session rather than one per profile: profiles behind the same
// [sso-session] are revived together, and offering the same login a dozen times
// is the noise LoginHint was written to avoid. A profile with no session — a
// static access key in ~/.aws/credentials — has nothing to log into and is left
// out rather than offered a command that cannot work.
func loginTargets(resolved []infra.ResolvedProfile) []infra.SSOTarget {
	seen := map[string]bool{}
	targets := make([]infra.SSOTarget, 0, len(resolved))

	for _, entry := range resolved {
		if entry.Usable() || entry.Profile.SSOSession == "" {
			continue
		}
		if seen[entry.Profile.SSOSession] {
			continue
		}
		seen[entry.Profile.SSOSession] = true
		targets = append(targets, infra.SSOTarget{Session: entry.Profile.SSOSession})
	}
	return targets
}

// offerLogin asks whether to log in now, and does it.
//
// The alternative is telling the operator to leave, run a command in another
// program and start over — for a command this one can run, with a session name it
// has already read. Reported rather than silent: logging somebody into an AWS
// account without asking is not a default worth having, however convenient.
func offerLogin(
	ctx context.Context,
	ask *prompter,
	out io.Writer,
	targets []infra.SSOTarget,
) (bool, error) {
	if ask == nil || !ask.interactive || len(targets) == 0 {
		return false, nil
	}

	target := targets[0]
	if len(targets) > 1 {
		options := make([]option, 0, len(targets))
		for _, candidate := range targets {
			options = append(options, option{
				value: candidate.Name(),
				label: candidate.Name(),
				note:  "aws " + strings.Join(candidate.Args(), " "),
			})
		}
		chosen, err := ask.pick("Which SSO session should be revived?",
			"The profiles behind it are logged in together.", "", options, "")
		if err != nil {
			return false, err
		}
		for _, candidate := range targets {
			if candidate.Name() == chosen {
				target = candidate
			}
		}
	}

	answer, err := ask.pick("Log in to AWS now?",
		"Runs aws "+strings.Join(target.Args(), " ")+", which opens a browser.", "",
		[]option{
			{value: "yes", label: "log in now", note: "opens the browser and waits"},
			{value: "no", label: "cancel", note: "leaves the instructions below"},
		}, "")
	//nolint:nilerr // Declining is not a failure of its own, and neither is a
	// selector that could not run: the session check has already written the reason
	// and the command to fix it by hand. Returning the error here would replace a
	// remediation the operator can act on with "the prompt failed".
	if err != nil || answer != "yes" {
		return false, nil
	}

	fmt.Fprintf(out, "\n  %s\n\n", newStyle(out).dim("aws "+strings.Join(target.Args(), " ")))
	if err := ssoLogin(ctx, target, os.Stdin, out, out); err != nil {
		fmt.Fprintf(out, "\n  %v\n", err)
		return false, nil
	}
	return true, nil
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

// note is the line printed after a clean report. It belongs to the caller
// because only the caller knows what it just did: `lerian infra check` can say it
// made no AWS call, and the preflight — which asks AWS who every profile is —
// cannot.
func reportChecks(out io.Writer, results []checkResult, note string) error {
	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> Environment check"))

	width := 0
	for _, r := range results {
		if len(r.name) > width {
			width = len(r.name)
		}
	}

	failed := writeRows(out, theme, results, width)

	// The remediations come after the table rather than inline, so the table stays
	// scannable when several things are wrong — which is the case this command
	// exists for.
	for _, r := range results {
		if !r.blocking() || r.detail == "" {
			continue
		}
		// indent() leaves the first line alone, because its other callers place it
		// themselves. Here the whole block is being pushed under a heading.
		fmt.Fprintf(out, "\n%s\n%s\n", theme.bold("  "+r.name), indent("    "+r.detail, "    "))
	}

	if failed == 0 {
		fmt.Fprintf(out, "\n  %d checks, all ok.%s\n", len(results), note)
		return nil
	}
	fmt.Fprintf(out, "\n  %d of %d checks failed.\n", failed, len(results))
	return fmt.Errorf("check: %s", strings.Join(failedNames(results), ", "))
}

// writeRows paints the table and returns how many rows failed.
//
// The verdict leads the row. Scanning for what failed is then running an eye down
// the left edge rather than down a ragged right one: the summaries are paths and
// versions of wildly different lengths, and a verdict at the end of them is the
// first thing to wrap off a narrow terminal.
func writeRows(out io.Writer, theme style, results []checkResult, width int) int {
	const verdictWidth = 7 // "missing"

	failed := 0
	for _, r := range results {
		// Padded before it is colored. Padding a string that already carries escape
		// sequences counts the escapes as characters, and the colored column lands
		// one word to the right of the plain ones.
		mark := theme.pass(fmt.Sprintf("%-*s", verdictWidth, "ok"))
		switch {
		// Dim rather than red, and "absent" rather than "missing": the row is
		// reporting what this machine cannot be asked to do, and a red line for a
		// tool no run calls teaches people to read past red lines.
		case !r.ok && r.optional:
			mark = theme.dim(fmt.Sprintf("%-*s", verdictWidth, "absent"))
		case !r.ok:
			mark = theme.alert(fmt.Sprintf("%-*s", verdictWidth, "missing"))
			failed++
		}
		fmt.Fprintf(out, "  %s  %-*s  %s\n", mark, width, r.name, r.summary)
	}
	return failed
}

func failedNames(results []checkResult) []string {
	var names []string
	for _, r := range results {
		if r.blocking() {
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
	ask *prompter,
	out io.Writer,
	layout infra.Layout,
	source checkoutSource,
	dryRun bool,
) (*infra.CLI, []infra.ResolvedProfile, error) {
	terraform, tfErr := infra.NewCLI(ctx)

	results := []checkResult{terraformResult(tfErr)}

	var profiles []infra.ResolvedProfile

	awsCLI := checkResult{name: "aws", ok: true}
	if !dryRun {
		awsCLI = checkAWSCLI(ctx)
		results = append(results, awsCLI)
	}

	// No git. It is only used by init, to clone, and a run that already has its
	// checkout never calls it — gating on it would demand a tool this command does
	// not need. `lerian infra check` reports it, because that command answers the
	// wider question of whether the machine can do everything.
	// No gh either, for the same reason git is left out: deploying touches
	// neither. The export offered after a run can use gh — that is the one path
	// that reaches GitHub — and it asks for it there, where the answer is about
	// something the operator just chose to do. A preflight is the list of what
	// this run needs, and padding it with tools the run will not call is how the
	// list stops being read.
	results = append(results, templatesResult(ctx, layout, source))

	// Only worth asking when there is an AWS CLI to ask with: without one the
	// session check fails for the reason already on the line above it, and two
	// rows for one problem read as two problems.
	if !dryRun && awsCLI.ok {
		session, resolved := checkAWSSession(ctx, checkIdentity)
		profiles = resolved

		// Offered before the report rather than after it: the report ends the
		// command, and the whole point is not to end it.
		if !session.ok {
			// Two different failures with two different answers: nothing configured
			// at all is a setup, an expired session is a login.
			if session.summary == "no credentials" {
				if offerSetup(ctx, ask, out) {
					session, profiles = checkAWSSession(ctx, checkIdentity)
				}
			} else if loggedIn, err := offerLogin(ctx, ask, out, loginTargets(resolved)); err != nil {
				return nil, nil, err
			} else if loggedIn {
				session, profiles = checkAWSSession(ctx, checkIdentity)
			}
		}
		results = append(results, session)
	}

	// Printed whether or not anything is wrong, when there is somebody who asked
	// for it by picking infra from the menu. Which checkout and which terraform a
	// run is about to use is worth a line each, and showing them only on failure
	// means never seeing them on the run that matters.
	//
	// A scripted invocation — one that named its --env and asked nothing — keeps
	// the output it always had: a pipeline does not want a table on every call.
	watching := watchingPreflight(ask)
	for _, r := range results {
		if r.blocking() {
			return nil, nil, reportChecks(out, results, "")
		}
	}
	if watching {
		if err := reportChecks(out, results, ""); err != nil {
			return nil, nil, err
		}
	}
	return terraform, profiles, nil
}

// templatesResult verifies the lerian-terraform-foundation checkout a run has
// already resolved: which repo, from which source, at which version — and
// whether that version is one this binary can read.
//
// It takes the resolved layout rather than resolving its own, so the row cannot
// name a checkout other than the one about to be used.
func templatesResult(ctx context.Context, layout infra.Layout, source checkoutSource) checkResult {
	summary, ref := inspectTemplates(ctx, layout, source)
	return templatesVerdict(layout.Root, ref, summary)
}

// templatesVerdict decides whether that checkout can be used.
//
// A label was not enough. A directory that is not a checkout, or one older than
// the templates this binary reads, fails later with a missing file or an unknown
// variable — several questions and one terraform init after the row that was
// supposed to warn about it printed "ok".
func templatesVerdict(root, ref, summary string) checkResult {
	result := checkResult{name: "templates", summary: summary, ok: true}

	if !infra.IsCheckout(root) {
		result.ok = false
		result.summary = "not a checkout"
		result.detail = fmt.Sprintf("%s does not hold lerian-terraform-foundation.\n"+
			"A checkout is recognized by the directories examples/aws/_modules and\n"+
			"examples/aws/backend; at least one of them is missing there.\n\n"+
			"%s", root, pointingOptions())
		return result
	}

	if infra.RefBelowMin(ref) {
		result.ok = false
		result.summary = "older than " + infra.TemplatesMinRef
		result.detail = fmt.Sprintf("The checkout at %s is %s, and this binary reads %s and later.\n"+
			"An older layout is missing roots and variables this expects, so the failure\n"+
			"would come later, as a missing file.\n\n"+
			"  git -C %s fetch --tags && git -C %s checkout %s\n\n"+
			"Or let the CLI move it:\n\n"+
			"  lerian infra init --sync --templates-ref %s",
			root, ref, infra.TemplatesMinRef, root, root, infra.TemplatesMinRef, infra.TemplatesMinRef)
		return result
	}

	return result
}

// watchingPreflight is whether there is somebody reading the preflight table. A
// scripted invocation gets the output it always had: a pipeline does not want a
// table on every call, and does not want rows about tools it will not use.
func watchingPreflight(ask *prompter) bool { return ask != nil && ask.interactive }

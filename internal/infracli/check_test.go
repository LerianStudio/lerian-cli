package infracli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// The reason this command exists: a run verifies each dependency immediately
// before the first call that needs it, so a bare machine surfaces one gap per
// round. The report has to list every failure at once, or it buys nothing over
// the ordinary flow.
func TestReportListsEveryFailureAtOnce(t *testing.T) {
	results := []checkResult{
		{name: "aws", summary: "not usable", detail: "install the AWS CLI v2", ok: false},
		{name: "terraform", summary: "not usable", detail: "terraform not found in PATH", ok: false},
		{name: "git", summary: "/usr/bin/git", ok: true},
		{name: "templates", summary: "not found", detail: "no checkout found", ok: false},
	}

	var out bytes.Buffer
	err := reportChecks(&out, results, "")

	if err == nil {
		t.Fatal("reportChecks returned nil with three failures; the exit code is what makes this usable as a CI gate")
	}
	for _, name := range []string{"aws", "terraform", "templates"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name the failing check %q: %v", name, err)
		}
	}
	for _, remediation := range []string{
		"install the AWS CLI v2",
		"terraform not found in PATH",
		"no checkout found",
	} {
		if !strings.Contains(out.String(), remediation) {
			t.Errorf("report dropped a remediation: %q\n%s", remediation, out.String())
		}
	}
}

func TestReportSucceedsWhenEverythingIsPresent(t *testing.T) {
	results := []checkResult{
		{name: "aws", summary: "/usr/local/bin/aws", ok: true},
		{name: "git", summary: "/usr/bin/git", ok: true},
	}

	var out bytes.Buffer
	if err := reportChecks(&out, results, ""); err != nil {
		t.Fatalf("reportChecks on an all-ok report = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "all ok") {
		t.Errorf("report does not say it passed:\n%s", out.String())
	}
}

// The command promises to touch no AWS API, which is what lets it answer "can
// this machine run the tool" without a configured profile. Saying so in the
// output is part of the contract.
// The footer is the caller's claim, not the report's, because only the caller
// knows what it just did: this command reaches no AWS API, and the preflight —
// which asks AWS who every profile is — must not say the same thing.
func TestReportPrintsTheNoteItIsGiven(t *testing.T) {
	var out bytes.Buffer
	results := []checkResult{{name: "git", summary: "/usr/bin/git", ok: true}}

	if err := reportChecks(&out, results, " No AWS call was made."); err != nil {
		t.Fatalf("reportChecks = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "No AWS call was made") {
		t.Errorf("report does not carry the note it was given:\n%s", out.String())
	}

	var bare bytes.Buffer
	_ = reportChecks(&bare, results, "")
	if strings.Contains(bare.String(), "AWS call") {
		t.Errorf("a report given no note made a claim anyway:\n%s", bare.String())
	}
}

// And `lerian infra check` is the caller that makes it.
func TestTheCheckCommandClaimsItMadeNoAWSCall(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	t.Setenv("LERIAN_TF_REPO", checkout)

	var out, errOut bytes.Buffer
	if err := runCheck(context.Background(), nil, &out, &errOut); err != nil {
		t.Skipf("this machine is missing a dependency, so there is no clean report to read: %v", err)
	}
	if !strings.Contains(out.String(), "No AWS call was made") {
		t.Errorf("the check no longer states that it reached no AWS API:\n%s", out.String())
	}
}

// A passing row still names the path, because "ok" alone hides the case where
// PATH resolves to a different binary than the operator expects.
func TestReportKeepsThePathOfAPassingCheck(t *testing.T) {
	var out bytes.Buffer
	_ = reportChecks(&out, []checkResult{{name: "aws", summary: "/opt/homebrew/bin/aws", ok: true}}, "")

	if !strings.Contains(out.String(), "/opt/homebrew/bin/aws") {
		t.Errorf("report dropped the resolved path:\n%s", out.String())
	}
}

// git is the one dependency every machine is likely to have, so it is the only
// check that can run here without assuming an installed toolchain.
func TestCheckGitReportsTheResolvedBinary(t *testing.T) {
	result := checkGit()

	if result.name != "git" {
		t.Errorf("name = %q, want %q", result.name, "git")
	}
	if result.ok && !strings.Contains(result.summary, "git") {
		t.Errorf("a passing git check does not name the binary: %q", result.summary)
	}
	if !result.ok && result.detail == "" {
		t.Error("a failing check carries no remediation, which is the whole value of the row")
	}
}

func TestBinaryPathSaysSoWhenTheBinaryIsAbsent(t *testing.T) {
	got := binaryPath("lerian-infra-a-binary-that-does-not-exist")

	if !strings.Contains(got, "not found") {
		t.Errorf("binaryPath on a missing binary = %q, want it to say not found", got)
	}
}

// The row exists to name the checkout a run would use, so it has to resolve
// from the same inputs a run does. $LERIAN_TF_REPO is one of them, and reading
// it is the wiring in runCheck — not in checkTemplates, which takes the value
// as a parameter. Exercising runCheck is therefore the only level at which
// forgetting to read the variable shows up.
func TestCheckReadsTheRepoEnvironmentVariable(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	t.Setenv("LERIAN_TF_REPO", checkout)
	// Nowhere else points at a checkout, so the variable is the only thing that
	// can produce one.
	t.Chdir(t.TempDir())

	var out, errOut bytes.Buffer
	// The error depends on whether this machine has the toolchain, which is not
	// what this test is about; the templates row is.
	_ = runCheck(context.Background(), []string{"--templates-dir", filepath.Join(t.TempDir(), "absent")},
		&out, &errOut)

	// Asserting on the row, not on the whole output: when the variable is set and
	// resolution still fails, the "no checkout found" remediation lists it among
	// the places it looked, so the path appears either way.
	row := templatesRow(t, out.String())
	if !strings.Contains(row, checkout) {
		t.Errorf("the templates row does not name the checkout $LERIAN_TF_REPO selects, "+
			"so check reports a different one than the run that follows:\n  %s", row)
	}
}

// templatesRow returns the report line for the templates check.
// templatesRow finds the row by its name field rather than by the start of the
// line: the verdict leads every row, so "templates" is the second field.
func templatesRow(t *testing.T, report string) string {
	t.Helper()
	for _, line := range strings.Split(report, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "templates" {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("the report has no templates row:\n%s", report)
	return ""
}

func TestCheckRejectsAPositionalArgument(t *testing.T) {
	var out, errOut bytes.Buffer

	err := runCheck(context.Background(), []string{"typo"}, &out, &errOut)

	if err == nil {
		t.Fatal("runCheck accepted a positional argument; a typo would run the checks and look like success")
	}
	if !strings.Contains(err.Error(), "typo") {
		t.Errorf("error does not name the offending argument: %v", err)
	}
}

// The gate exists to collapse a round trip: returning at the first gap sends the
// operator to install terraform, run again, and only then learn the AWS CLI is
// missing too.
func TestPreflightReportsBothToolsAtOnce(t *testing.T) {
	layout := layoutFor(t)
	t.Setenv("PATH", t.TempDir()) // neither binary resolvable

	var out bytes.Buffer
	_, err := preflight(context.Background(), nil, &out, layout, sourceFlag, false)

	if err == nil {
		t.Fatal("preflight passed with no terraform and no aws in PATH")
	}
	for _, tool := range []string{"terraform", "aws"} {
		if !strings.Contains(out.String(), tool) {
			t.Errorf("the report does not mention %q:\n%s", tool, out.String())
		}
	}
}

// A dry run makes no AWS call, so demanding the AWS CLI for one would block a
// command that never uses it.
func TestPreflightExemptsTheAWSCLIOnADryRun(t *testing.T) {
	layout := layoutFor(t)

	var out bytes.Buffer
	_, err := preflight(context.Background(), nil, &out, layout, sourceFlag, true)

	if err != nil && strings.Contains(out.String(), "aws") {
		t.Errorf("a dry run was gated on the AWS CLI:\n%s", out.String())
	}
}

// git is only used by init, to clone. Gating a run on it would demand a tool the
// run never calls.
func TestPreflightDoesNotGateOnGit(t *testing.T) {
	layout := layoutFor(t)

	var out bytes.Buffer
	_, _ = preflight(context.Background(), nil, &out, layout, sourceFlag, true)

	if strings.Contains(out.String(), "git") {
		t.Errorf("a run was gated on git, which only init uses:\n%s", out.String())
	}
}

// stubIdentity answers for profiles without an AWS account behind them.
type stubIdentity struct {
	usable map[string]bool
}

func (s stubIdentity) CallerIdentity(_ context.Context, profile, _ string) (infra.Caller, error) {
	if s.usable[profile] {
		return infra.Caller{Account: "111122223333", ARN: "arn:aws:iam::111122223333:user/" + profile}, nil
	}
	return infra.Caller{}, errors.New("the SSO session has expired")
}

// awsConfig writes a ~/.aws/config holding the named profiles, and points HOME at
// it so the profile listing reads this one rather than the machine's.
func awsConfig(t *testing.T, profiles ...string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	var body strings.Builder
	for _, name := range profiles {
		fmt.Fprintf(&body, "[profile %s]\nregion = us-east-1\nsso_session = lerian\n\n", name)
	}
	dir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A machine where nothing in ~/.aws resolves cannot run a single stage, and the
// operator finds out before answering anything rather than after — the questions
// are three, and the answer to all of them is wasted on a run that cannot start.
func TestAnExpiredSessionIsReportedBeforeTheQuestions(t *testing.T) {
	awsConfig(t, "sandbox", "production")

	result, _ := checkAWSSession(context.Background(), stubIdentity{})

	if result.ok {
		t.Fatal("a machine with no usable profile passed the session check")
	}
	if !strings.Contains(result.detail, "aws sso login") {
		t.Errorf("the failure does not say how to fix it:\n%s", result.detail)
	}
}

// One profile that resolves is enough to start. Which one is the right one is a
// later question — this one only asks whether the operator is logged in at all.
func TestOneUsableProfileIsEnough(t *testing.T) {
	awsConfig(t, "sandbox", "production")

	result, _ := checkAWSSession(context.Background(), stubIdentity{usable: map[string]bool{"production": true}})

	if !result.ok {
		t.Errorf("a machine with a usable profile failed the session check: %s", result.detail)
	}
	if !strings.Contains(result.summary, "production") {
		t.Errorf("the summary does not name the profile that resolves: %q", result.summary)
	}
}

// An empty ~/.aws is a different failure from an expired session, and it needs a
// different fix: there is nothing to log in to yet.
func TestNoProfilesAtAllSaysSo(t *testing.T) {
	awsConfig(t)

	result, _ := checkAWSSession(context.Background(), stubIdentity{})

	if result.ok {
		t.Fatal("a machine with no profiles at all passed the session check")
	}
	if !strings.Contains(result.detail, "aws configure sso") {
		t.Errorf("the failure does not say how to create a profile:\n%s", result.detail)
	}
}

// layoutFor is a checkout the preflight can report on without resolving one.
func layoutFor(t *testing.T) infra.Layout {
	t.Helper()
	layout, err := infra.NewLayout(fakeCheckout(t, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

// The session is part of the preflight, not something a later stage discovers.
func TestThePreflightAsksWhetherTheOperatorIsLoggedIn(t *testing.T) {
	layout := layoutFor(t)
	awsConfig(t, "sandbox")
	previous := checkIdentity
	checkIdentity = stubIdentity{}
	t.Cleanup(func() { checkIdentity = previous })

	var out bytes.Buffer
	_, _ = preflight(context.Background(), nil, &out, layout, sourceFlag, false)

	if !strings.Contains(out.String(), "aws session") {
		t.Errorf("the preflight never asked whether there is a session:\n%s", out.String())
	}
}

// The verdict comes first. Scanning for what failed should be running an eye
// down the left edge, not down a ragged right one — the summaries are paths and
// versions of wildly different lengths, and on a narrow terminal the column that
// holds the verdict is the first thing to wrap off the screen.
func TestTheVerdictIsTheFirstThingOnTheLine(t *testing.T) {
	results := []checkResult{
		{name: "terraform", summary: "/opt/homebrew/bin/terraform", ok: true},
		{name: "aws session", summary: "not logged in", detail: "log in", ok: false},
	}

	var out bytes.Buffer
	_ = reportChecks(&out, results, "")

	for _, line := range strings.Split(out.String(), "\n") {
		trimmed := strings.TrimSpace(stripANSI(line))
		switch {
		case strings.HasPrefix(trimmed, "terraform"), strings.HasPrefix(trimmed, "aws session /"):
			t.Errorf("the line starts with the name rather than the verdict: %q", trimmed)
		}
	}
	if !strings.Contains(stripANSI(out.String()), "ok       terraform") {
		t.Errorf("the verdict does not lead the row:\n%s", stripANSI(out.String()))
	}
	if !strings.Contains(stripANSI(out.String()), "missing  aws session") {
		t.Errorf("the failing verdict does not lead its row:\n%s", stripANSI(out.String()))
	}
}

// The names line up under each other whether or not the verdict beside them is
// colored. Padding a string that carries escape sequences counts the escapes,
// which is how a colored column ends up one word to the right of the plain ones.
func TestTheNamesLineUpWhateverTheVerdictIs(t *testing.T) {
	results := []checkResult{
		{name: "terraform", summary: "ok", ok: true},
		{name: "aws session", summary: "no", detail: "x", ok: false},
	}

	var plain, colored bytes.Buffer
	_ = reportChecks(&plain, results, "")
	_ = reportChecks(&coloredWriter{&colored}, results, "")

	if columnOf(t, stripANSI(plain.String()), "terraform") != columnOf(t, stripANSI(colored.String()), "terraform") {
		t.Errorf("color moved the name column:\nplain:\n%s\ncolored:\n%s", plain.String(), stripANSI(colored.String()))
	}
}

func columnOf(t *testing.T, text, name string) int {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if at := strings.Index(line, name); at != -1 {
			return at
		}
	}
	t.Fatalf("%q is not in:\n%s", name, text)
	return -1
}

// coloredWriter is a writer the style believes is a terminal.
type coloredWriter struct{ inner *bytes.Buffer }

func (c *coloredWriter) Write(p []byte) (int, error) { return c.inner.Write(p) }

func stripANSI(text string) string {
	var out strings.Builder
	for len(text) > 0 {
		if strings.HasPrefix(text, "\x1b[") {
			if end := strings.IndexByte(text, 'm'); end != -1 {
				text = text[end+1:]
				continue
			}
		}
		out.WriteByte(text[0])
		text = text[1:]
	}
	return out.String()
}

// Being told to run `aws sso login` and then run this again is a round trip
// through another program for something this one has everything it needs to do:
// the AWS CLI is already a verified dependency, and the session name is already
// in the profile it just read.
func TestAnExpiredSessionCanBeRevivedWithoutLeaving(t *testing.T) {
	layout := layoutFor(t)
	awsConfig(t, "sandbox", "production")

	// Nothing resolves, then everything does — which is what logging in changes.
	loggedIn := false
	previous := checkIdentity
	checkIdentity = conditionalIdentity{usable: &loggedIn}
	t.Cleanup(func() { checkIdentity = previous })

	var attempted []infra.SSOTarget
	previousLogin := ssoLogin
	ssoLogin = func(_ context.Context, target infra.SSOTarget, _ io.Reader, _, _ io.Writer) error {
		attempted = append(attempted, target)
		loggedIn = true
		return nil
	}
	t.Cleanup(func() { ssoLogin = previousLogin })

	ask, _ := selectorFor(t, keyEnterSeq)

	var out bytes.Buffer
	_, err := preflight(context.Background(), ask, &out, layout, sourceFlag, false)

	if len(attempted) != 1 {
		t.Fatalf("logged in %d times, want once: %v", len(attempted), attempted)
	}
	if attempted[0].Session != "lerian" {
		t.Errorf("logged into %q, want the session the profiles share", attempted[0].Session)
	}
	// And the run carries on: the reason it stopped is gone.
	if err != nil && strings.Contains(err.Error(), "aws session") {
		t.Errorf("the session was still reported as failed after logging in: %v\n%s", err, out.String())
	}
}

// Declining leaves the command exactly where it was: the report, the login
// instruction, and a non-zero exit.
func TestDecliningTheLoginLeavesTheInstruction(t *testing.T) {
	layout := layoutFor(t)
	awsConfig(t, "sandbox")

	previous := checkIdentity
	checkIdentity = stubIdentity{}
	t.Cleanup(func() { checkIdentity = previous })

	called := false
	previousLogin := ssoLogin
	ssoLogin = func(context.Context, infra.SSOTarget, io.Reader, io.Writer, io.Writer) error {
		called = true
		return nil
	}
	t.Cleanup(func() { ssoLogin = previousLogin })

	// Down once, onto "cancel", then Enter.
	ask, _ := selectorFor(t, keyDownSeq+keyEnterSeq)

	var out bytes.Buffer
	_, err := preflight(context.Background(), ask, &out, layout, sourceFlag, false)

	if called {
		t.Error("the login ran without being accepted")
	}
	if err == nil {
		t.Fatal("declining the login let the run continue with no session")
	}
	if !strings.Contains(out.String(), "aws sso login") {
		t.Errorf("the instruction for doing it by hand is gone:\n%s", out.String())
	}
}

// A profile backed by a static access key has no session to revive, so there is
// nothing to offer and offering anyway would run a command that cannot work.
func TestNoLoginIsOfferedForProfilesWithoutASession(t *testing.T) {
	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "static", Source: "credentials"}, Err: errors.New("expired")},
	}

	if targets := loginTargets(resolved); len(targets) != 0 {
		t.Errorf("offered %v for a profile with no SSO session", targets)
	}
}

// Profiles behind one session share one login, which is the whole reason the
// session name is worth reading: a dozen account profiles are revived by a single
// command, not by a dozen identical ones.
func TestProfilesSharingASessionShareOneLogin(t *testing.T) {
	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "dev", SSOSession: "acme"}, Err: errors.New("expired")},
		{Profile: infra.AWSProfile{Name: "stg", SSOSession: "acme"}, Err: errors.New("expired")},
		{Profile: infra.AWSProfile{Name: "other", SSOSession: "second"}, Err: errors.New("expired")},
	}

	targets := loginTargets(resolved)

	if len(targets) != 2 {
		t.Fatalf("got %d targets, want one per session: %v", len(targets), targets)
	}
	if targets[0].Session != "acme" || targets[1].Session != "second" {
		t.Errorf("targets = %v, want the two sessions in order", targets)
	}
}

// conditionalIdentity resolves once the flag it watches is set.
type conditionalIdentity struct{ usable *bool }

func (c conditionalIdentity) CallerIdentity(_ context.Context, profile, _ string) (infra.Caller, error) {
	if *c.usable {
		return infra.Caller{Account: "111122223333", ARN: "arn:aws:iam::111122223333:user/" + profile}, nil
	}
	return infra.Caller{}, errors.New("the SSO session has expired")
}

// The report is what the operator asked for by picking infra, not an error
// message that happens to be formatted as a table. It appears whether or not
// anything is wrong: knowing which checkout and which terraform a run is about to
// use is worth a line each, and seeing them only when something breaks means
// never seeing them on the run that matters.
func TestTheReportAppearsEvenWhenEverythingPasses(t *testing.T) {
	layout := layoutFor(t)
	awsConfig(t, "sandbox")

	previous := checkIdentity
	checkIdentity = stubIdentity{usable: map[string]bool{"sandbox": true}}
	t.Cleanup(func() { checkIdentity = previous })

	ask, _ := selectorFor(t, "")

	var out bytes.Buffer
	if _, err := preflight(context.Background(), ask, &out, layout, sourceFlag, false); err != nil {
		t.Fatalf("preflight on a working machine = %v\n%s", err, out.String())
	}

	report := stripANSI(out.String())
	if !strings.Contains(report, "Environment check") {
		t.Errorf("nothing was reported for a machine where everything passed:\n%s", report)
	}
	for _, row := range []string{"terraform", "templates", "aws session"} {
		if !strings.Contains(report, row) {
			t.Errorf("the %q row is missing from a passing report:\n%s", row, report)
		}
	}
}

// A scripted run — one that named its --env and asked nothing — keeps the output
// it always had. The block is for the person who chose from a menu; a pipeline
// gets a table it did not ask for on every single invocation.
func TestAScriptedRunIsNotGivenTheReport(t *testing.T) {
	layout := layoutFor(t)
	awsConfig(t, "sandbox")

	previous := checkIdentity
	checkIdentity = stubIdentity{usable: map[string]bool{"sandbox": true}}
	t.Cleanup(func() { checkIdentity = previous })

	var out bytes.Buffer
	if _, err := preflight(context.Background(), nil, &out, layout, sourceFlag, false); err != nil {
		t.Fatalf("preflight = %v", err)
	}

	if out.Len() != 0 {
		t.Errorf("a scripted run was given a report it did not ask for:\n%s", out.String())
	}
}

// "No AWS call was made" is true of `lerian infra check` and false of the
// preflight, which asks AWS who the profiles are. A footer that says otherwise is
// a claim about what just happened, and it would be wrong.
func TestTheNoAWSCallClaimIsOnlyMadeWhereItHolds(t *testing.T) {
	layout := layoutFor(t)
	awsConfig(t, "sandbox")

	previous := checkIdentity
	checkIdentity = stubIdentity{usable: map[string]bool{"sandbox": true}}
	t.Cleanup(func() { checkIdentity = previous })

	ask, _ := selectorFor(t, "")

	var out bytes.Buffer
	_, _ = preflight(context.Background(), ask, &out, layout, sourceFlag, false)

	if strings.Contains(out.String(), "No AWS call") {
		t.Errorf("the preflight claimed it made no AWS call, having just made several:\n%s", out.String())
	}
}

// The row answers one question — is there a session — so it says how many
// resolve and names a few. Naming all of them turns a one-line verdict into a
// wrapped paragraph on a machine with a profile per account, which is the machine
// this tool is built for.
func TestTheSessionRowStaysOneLine(t *testing.T) {
	names := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india"}
	awsConfig(t, names...)

	usable := map[string]bool{}
	for _, name := range names {
		usable[name] = true
	}

	result, _ := checkAWSSession(context.Background(), stubIdentity{usable: usable})

	if !result.ok {
		t.Fatalf("every profile resolves and the check failed: %s", result.detail)
	}
	if displayWidth(result.summary) > 60 {
		t.Errorf("the summary is %d columns and will wrap:\n%s", displayWidth(result.summary), result.summary)
	}
	if !strings.Contains(result.summary, "9") {
		t.Errorf("the summary does not say how many resolve: %q", result.summary)
	}
}

// The templates row is a verification, not a label. A directory that is not a
// checkout, or one older than this binary can read, is a run that fails later
// with a missing file or an unknown variable — and the row that was supposed to
// warn about it said "ok".
func TestTheTemplatesRowFailsWhenTheCheckoutCannotBeUsed(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		ref     string
		wantOK  bool
		wantSay string
	}{
		{
			name:    "a directory that is not a checkout",
			root:    t.TempDir(),
			ref:     "v9.9.9",
			wantOK:  false,
			wantSay: "examples/aws/_modules",
		},
		{
			name:    "a checkout older than this binary reads",
			root:    fakeCheckout(t, "", ""),
			ref:     "v0.0.1",
			wantOK:  false,
			wantSay: infra.TemplatesMinRef,
		},
		{
			name:   "a checkout this binary can read",
			root:   fakeCheckout(t, "", ""),
			ref:    infra.TemplatesMinRef,
			wantOK: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := templatesVerdict(test.root, test.ref, test.root+" @ "+test.ref)

			if result.ok != test.wantOK {
				t.Fatalf("ok = %v, want %v (%s)", result.ok, test.wantOK, result.detail)
			}
			if test.wantSay != "" && !strings.Contains(result.detail, test.wantSay) {
				t.Errorf("the failure does not mention %q:\n%s", test.wantSay, result.detail)
			}
		})
	}
}

// Color on both verdicts, and the word under it either way. The color is
// redundant by design — a reader without it, or reading a saved log, loses
// nothing — so it can afford to be there.
func TestBothVerdictsAreColoredAndStillReadable(t *testing.T) {
	results := []checkResult{
		{name: "terraform", summary: "/usr/bin/terraform", ok: true},
		{name: "templates", summary: "not found", detail: "clone it", ok: false},
	}

	var painted bytes.Buffer
	theme := style{enabled: true}
	writeRows(&painted, theme, results, len("terraform"))

	if !strings.Contains(painted.String(), theme.pass("ok     ")) {
		t.Errorf("the passing verdict is not colored:\n%q", painted.String())
	}
	if !strings.Contains(painted.String(), theme.alert("missing")) {
		t.Errorf("the failing verdict is not colored:\n%q", painted.String())
	}

	plain := stripANSI(painted.String())
	for _, word := range []string{"ok", "missing"} {
		if !strings.Contains(plain, word) {
			t.Errorf("%q survives only as color:\n%s", word, plain)
		}
	}
}

// And with styling off there is no color at all, because the same report is
// routinely redirected into a file or a CI log.
func TestNoColorWhereThereIsNoTerminal(t *testing.T) {
	var out bytes.Buffer
	_ = reportChecks(&out, []checkResult{{name: "terraform", summary: "/usr/bin/terraform", ok: true}}, "")

	if strings.Contains(out.String(), "\x1b") {
		t.Errorf("an escape sequence reached a report nobody is watching:\n%q", out.String())
	}
}

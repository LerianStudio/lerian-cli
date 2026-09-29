package infracli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	err := reportChecks(&out, results)

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
	if err := reportChecks(&out, results); err != nil {
		t.Fatalf("reportChecks on an all-ok report = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "all ok") {
		t.Errorf("report does not say it passed:\n%s", out.String())
	}
}

// The command promises to touch no AWS API, which is what lets it answer "can
// this machine run the tool" without a configured profile. Saying so in the
// output is part of the contract.
func TestReportSaysNoAWSCallWasMade(t *testing.T) {
	var out bytes.Buffer
	if err := reportChecks(&out, []checkResult{{name: "git", summary: "/usr/bin/git", ok: true}}); err != nil {
		t.Fatalf("reportChecks = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "No AWS call was made") {
		t.Errorf("report does not state that no AWS call was made:\n%s", out.String())
	}
}

// A passing row still names the path, because "ok" alone hides the case where
// PATH resolves to a different binary than the operator expects.
func TestReportKeepsThePathOfAPassingCheck(t *testing.T) {
	var out bytes.Buffer
	_ = reportChecks(&out, []checkResult{{name: "aws", summary: "/opt/homebrew/bin/aws", ok: true}})

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
func templatesRow(t *testing.T, report string) string {
	t.Helper()
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "templates") {
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
	_, err := preflight(context.Background(), &out, layout, sourceFlag, false)

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
	_, err := preflight(context.Background(), &out, layout, sourceFlag, true)

	if err != nil && strings.Contains(out.String(), "aws") {
		t.Errorf("a dry run was gated on the AWS CLI:\n%s", out.String())
	}
}

// git is only used by init, to clone. Gating a run on it would demand a tool the
// run never calls.
func TestPreflightDoesNotGateOnGit(t *testing.T) {
	layout := layoutFor(t)

	var out bytes.Buffer
	_, _ = preflight(context.Background(), &out, layout, sourceFlag, true)

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

	result := checkAWSSession(context.Background(), stubIdentity{})

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

	result := checkAWSSession(context.Background(), stubIdentity{usable: map[string]bool{"production": true}})

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

	result := checkAWSSession(context.Background(), stubIdentity{})

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
	_, _ = preflight(context.Background(), &out, layout, sourceFlag, false)

	if !strings.Contains(out.String(), "aws session") {
		t.Errorf("the preflight never asked whether there is a session:\n%s", out.String())
	}
}

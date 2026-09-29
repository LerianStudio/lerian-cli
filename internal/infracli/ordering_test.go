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

// Everything the guided questions lead to ends in terraform and the AWS CLI, so
// a machine missing one cannot finish whatever is chosen. The tools are verified
// before the first question rather than after the last: asking first spends
// three answers and then reports the failure where it reads as a problem with
// the choices rather than with the machine.
//
// Driven through prepareChoices rather than run, because the ordering only
// exists when there is somebody to ask, and run builds its prompter from the
// real stdin — which is not a terminal under go test.
func TestTheToolsAreVerifiedBeforeTheQuestionsAreAsked(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no terraform, no aws

	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	ask, painted := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	var report bytes.Buffer
	_, err = prepareChoices(context.Background(), ask, catalog, &opts, layout, sourceFlag, &report)

	if err == nil {
		t.Fatal("preparing a guided run with no tools in PATH returned nil")
	}
	if !strings.Contains(report.String(), "terraform") {
		t.Errorf("the failure does not name the missing tool:\n%s", report.String())
	}
	// guidedRun paints this first. Seeing it means the questions ran ahead of the
	// verification, which is the ordering this test exists to pin.
	if strings.Contains(painted.String(), "Which environment?") {
		t.Errorf("the run asked before checking the tools:\n%s", painted.String())
	}
	if opts.environment != "" {
		t.Errorf("an answer was collected before the tools were verified: %q", opts.environment)
	}
}

// Without a terminal nothing is asked, so there are no answers to protect — and
// the argument and configuration errors are both cheaper to produce and more
// specific than "terraform is missing". A script with a bad --target should hear
// about the target.
func TestWithoutATerminalTheConfigurationErrorComesFirst(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	t.Setenv("LERIAN_TF_REPO", checkout)
	t.Setenv("PATH", t.TempDir())

	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"--target", "infra-base"}, &stdout, &stderr)

	if err == nil {
		t.Fatal("a run with no --env returned nil")
	}
	if !strings.Contains(err.Error(), "--env is required") {
		t.Errorf("error = %q, want the missing environment rather than the missing tool", err)
	}
}

// --list resolves the catalog from the checkout and prints it. It reaches no
// tool, so gating it would demand terraform to answer a question about what is
// on disk.
func TestListingTargetsNeedsNoTools(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	if err := os.MkdirAll(filepath.Join(checkout, "examples", "aws", "products", "midaz", "postgres"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LERIAN_TF_REPO", checkout)
	t.Setenv("PATH", t.TempDir())

	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--list"}, &stdout, &stderr); err != nil {
		t.Fatalf("--list on a machine with no tools = %v\n%s", err, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "not usable") {
		t.Errorf("--list was gated on a tool it never calls")
	}
}

// The credential is checked as soon as the environment names the profile, not
// after every question has been answered.
//
// This is the failure an operator actually hits: the environment picks the AWS
// account, the account picks the profile, and an expired SSO session for that
// profile is only discovered at the point the first stage tries to run. By then
// the target and the action have been answered too, and all three answers are
// lost to a run that could never have started.
func TestTheCredentialIsCheckedAsSoonAsTheEnvironmentIsKnown(t *testing.T) {
	ask, painted := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	expired := errors.New("the SSO session for profile \"lerian-sandbox\" has expired")
	err := guidedRun(catalog, &opts, ask, infra.Layout{}, func(string) error { return expired })

	if !errors.Is(err, expired) {
		t.Fatalf("guidedRun = %v, want the credential failure", err)
	}
	// The environment was answered; nothing after it was asked.
	if opts.environment == "" {
		t.Error("the run failed before the environment was even chosen")
	}
	if strings.Contains(painted.String(), "operate on") {
		t.Errorf("the target was asked after the credential had already failed:\n%s", painted.String())
	}
	if opts.action != "" {
		t.Errorf("an action was collected after the credential failed: %q", opts.action)
	}
}

// With a working credential the questions carry on as before.
func TestAWorkingCredentialAsksTheRestOfTheQuestions(t *testing.T) {
	ask, painted := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	var checked string
	if err := guidedRun(catalog, &opts, ask, infra.Layout{}, func(env string) error { checked = env; return nil }); err != nil {
		t.Fatalf("guidedRun = %v", err)
	}

	if checked != opts.environment {
		t.Errorf("the credential was checked for %q, the environment chosen was %q", checked, opts.environment)
	}
	if !strings.Contains(painted.String(), "operate on") {
		t.Errorf("the questions stopped at the environment:\n%s", painted.String())
	}
}

// The environment picks the AWS account, and until now the account only appeared
// on the confirmation before an apply — so a plan never named it at all, and even
// an apply named it after every question had been answered. It is the one fact
// worth knowing before choosing, not after.
func TestTheEnvironmentOptionsNameTheAccount(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
		"prd": "account_id = 999988887777\nregion = us-east-1\nprofile = production",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	options := environmentOptions(layout)

	byValue := map[string]option{}
	for _, opt := range options {
		byValue[opt.value] = opt
	}
	if !strings.Contains(byValue["dev"].note, "111122223333") {
		t.Errorf("dev does not name its account: %q", byValue["dev"].note)
	}
	if !strings.Contains(byValue["dev"].note, "sandbox") {
		t.Errorf("dev does not name the profile it uses: %q", byValue["dev"].note)
	}
	if !strings.Contains(byValue["prd"].note, "999988887777") {
		t.Errorf("prd does not name its account: %q", byValue["prd"].note)
	}
	// Two environments must never look alike here: picking the wrong one is
	// picking the wrong account.
	if byValue["dev"].note == byValue["prd"].note {
		t.Errorf("dev and prd read identically: %q", byValue["dev"].note)
	}
}

// An environment the configuration says nothing about keeps its plain
// description rather than claiming an account it does not have.
func TestAnUnconfiguredEnvironmentClaimsNoAccount(t *testing.T) {
	layout, err := infra.NewLayout(fakeCheckout(t, "", ""))
	if err != nil {
		t.Fatal(err)
	}

	for _, opt := range environmentOptions(layout) {
		if strings.Contains(opt.note, "account") {
			t.Errorf("%s claims an account with no configuration to read: %q", opt.value, opt.note)
		}
		if opt.note == "" {
			t.Errorf("%s lost its description", opt.value)
		}
	}
}

// writeEnvConfig writes an environments.conf holding the given sections.
func writeEnvConfig(t *testing.T, checkout string, sections map[string]string) {
	t.Helper()

	var body strings.Builder
	for name, values := range sections {
		fmt.Fprintf(&body, "[%s]\n%s\n\n", name, values)
	}
	path := filepath.Join(checkout, "examples", "aws", "environments.conf")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

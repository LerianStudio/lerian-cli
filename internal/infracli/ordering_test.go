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
	err := guidedRun(context.Background(), catalog, &opts, ask, configuredLayout(t), configuredProfiles(), func(string) error { return expired })

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
	if err := guidedRun(context.Background(), catalog, &opts, ask, configuredLayout(t), configuredProfiles(), func(env string) error { checked = env; return nil }); err != nil {
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

// configuredLayout is a checkout whose environments.conf declares every
// environment, for tests about the questions rather than about the configuration.
func configuredLayout(t *testing.T) infra.Layout {
	t.Helper()

	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = dev-profile",
		"stg": "account_id = 444455556666\nregion = us-east-1\nprofile = stg-profile",
		"prd": "account_id = 999988887777\nregion = us-east-1\nprofile = prd-profile",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

// The question an operator can answer is "which account", not "which of our
// three environment names".
//
// dev, stg and prd are the names of files in the templates repo —
// backend/<env>.hcl holds the state, envs/<env>.tfvars holds the sizing — so they
// cannot be removed from the model. They can stop being the question. The account
// is what the operator is deciding, it is what the guard checks, and it is the
// thing that is theirs rather than ours.
func TestTheQuestionIsWhichAccount(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "sandbox"}, Caller: infra.Caller{Account: "111122223333"}},
		{Profile: infra.AWSProfile{Name: "elsewhere"}, Caller: infra.Caller{Account: "999988887777"}},
		{Profile: infra.AWSProfile{Name: "stale", SSOSession: "acme"}, Err: errors.New("expired")},
	}

	options := accountOptions(layout, resolved)

	byValue := map[string]option{}
	for _, opt := range options {
		byValue[opt.value] = opt
	}

	// The configured one is choosable and says what it is, in the operator's terms
	// first: the account, then the profile that reaches it.
	if byValue["sandbox"].disabled {
		t.Error("the configured account cannot be chosen")
	}
	if !strings.Contains(byValue["sandbox"].label+byValue["sandbox"].note, "111122223333") {
		t.Errorf("the row does not name the account: %+v", byValue["sandbox"])
	}

	// An account with no section cannot be deployed into — there is no state
	// backend and no variables file for it — and the row says so instead of
	// offering a choice that fails later.
	if !byValue["elsewhere"].disabled {
		t.Error("an account with no configuration was offered")
	}
	if !strings.Contains(byValue["elsewhere"].note, "init") {
		t.Errorf("the row does not say how to configure it: %q", byValue["elsewhere"].note)
	}

	// A profile whose session died is offered, because choosing it is how you say
	// "log me into that one".
	if byValue["stale"].disabled {
		t.Error("a profile with an expired session was not offered, so there is no way to log into it")
	}
	if !strings.Contains(byValue["stale"].note, "log in") {
		t.Errorf("the row does not say it needs a login: %q", byValue["stale"].note)
	}
}

// The environment is derived from the account rather than asked for.
func TestTheEnvironmentComesFromTheAccount(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
		"prd": "account_id = 999988887777\nregion = us-east-1\nprofile = production",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := environmentForProfile(layout, "sandbox", "111122223333"); !ok || got != "dev" {
		t.Errorf("sandbox/111122223333 = %q,%v — want dev", got, ok)
	}
	if got, ok := environmentForProfile(layout, "production", "999988887777"); !ok || got != "prd" {
		t.Errorf("production/999988887777 = %q,%v — want prd", got, ok)
	}
	if _, ok := environmentForProfile(layout, "nobody", "000000000000"); ok {
		t.Error("an unconfigured account resolved to an environment")
	}
}

// The guided run asks for the account and never mentions an environment name.
func TestTheGuidedRunAsksForTheAccount(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "sandbox"}, Caller: infra.Caller{Account: "111122223333"}},
	}

	ask, painted := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	if err := guidedRun(context.Background(), catalog, &opts, ask, layout, resolved, nil); err != nil {
		t.Fatalf("guidedRun = %v\n%s", err, painted.String())
	}

	if !strings.Contains(painted.String(), "account") {
		t.Errorf("the account was never mentioned:\n%s", painted.String())
	}
	if strings.Contains(painted.String(), "Which environment") {
		t.Errorf("the operator was asked for one of our environment names:\n%s", painted.String())
	}
	// And the environment still reaches the rest of the run, derived.
	if opts.environment != "dev" {
		t.Errorf("environment = %q, want dev derived from the account", opts.environment)
	}
}

// With no profile reaching a configured account there is nothing to ask, and the
// answer is init rather than a menu of dead rows.
func TestNoConfiguredAccountSendsTheOperatorToInit(t *testing.T) {
	layout, err := infra.NewLayout(fakeCheckout(t, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "sandbox"}, Caller: infra.Caller{Account: "111122223333"}},
	}

	ask, painted := selectorFor(t, keyEnterSeq)
	opts := options{}

	err = guidedRun(context.Background(), infra.Catalog{}, &opts, ask, layout, resolved, nil)

	if err == nil {
		t.Fatal("a run was offered an account it cannot deploy into")
	}
	if !strings.Contains(err.Error(), "lerian infra init") {
		t.Errorf("the error does not say how to configure one: %v", err)
	}
	if strings.Contains(painted.String(), "Which account") {
		t.Errorf("a question was asked that had no answer:\n%s", painted.String())
	}
}

// configuredProfiles are the profiles that reach the accounts configuredLayout
// declares, in the order the environments are declared, so the first row of the
// menu is dev.
func configuredProfiles() []infra.ResolvedProfile {
	return []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "dev-profile"}, Caller: infra.Caller{Account: "111122223333"}},
		{Profile: infra.AWSProfile{Name: "stg-profile"}, Caller: infra.Caller{Account: "444455556666"}},
		{Profile: infra.AWSProfile{Name: "prd-profile"}, Caller: infra.Caller{Account: "999988887777"}},
	}
}

// Choosing a profile whose session has died is how an operator says "log me into
// that one". It logs into that profile's session — not into whichever one the
// preflight happened to offer first — and then carries on with the account they
// picked.
func TestChoosingAnExpiredProfileLogsIntoThatOne(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "sandbox", SSOSession: "acme"}, Err: errors.New("expired")},
	}

	var attempted []infra.SSOTarget
	previousLogin := ssoLogin
	ssoLogin = func(_ context.Context, target infra.SSOTarget, _ io.Reader, _, _ io.Writer) error {
		attempted = append(attempted, target)
		return nil
	}
	t.Cleanup(func() { ssoLogin = previousLogin })

	previousIdentity := checkIdentity
	checkIdentity = stubIdentity{usable: map[string]bool{"sandbox": true}}
	t.Cleanup(func() { checkIdentity = previousIdentity })

	// Enter picks sandbox, Enter accepts the login, then the remaining questions.
	ask, _ := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	if err := guidedRun(context.Background(), catalog, &opts, ask, layout, resolved, nil); err != nil {
		t.Fatalf("guidedRun = %v", err)
	}

	if len(attempted) != 1 || attempted[0].Session != "acme" {
		t.Fatalf("logged into %v, want the session behind the chosen profile", attempted)
	}
	if opts.environment != "dev" {
		t.Errorf("environment = %q, want the one the chosen account maps to", opts.environment)
	}
}

// The accounts that can be deployed into come first, because the cursor starts on
// the first row: a list that opens on something unusable makes the most likely
// keypress a mistake. Then the ones a login would revive, then the rest.
func TestTheUsableAccountsComeFirst(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "aaa-unconfigured"}, Caller: infra.Caller{Account: "999988887777"}},
		{Profile: infra.AWSProfile{Name: "bbb-expired", SSOSession: "acme"}, Err: errors.New("expired")},
		{Profile: infra.AWSProfile{Name: "sandbox"}, Caller: infra.Caller{Account: "111122223333"}},
	}

	options := accountOptions(layout, resolved)

	if options[0].value != "sandbox" {
		t.Errorf("the list opens on %q, which is not the one that can be deployed into", options[0].value)
	}
	if options[0].disabled {
		t.Error("the first row cannot be chosen")
	}
	if options[1].value != "bbb-expired" {
		t.Errorf("the second row is %q, want the one a login would revive", options[1].value)
	}
	if options[2].value != "aaa-unconfigured" {
		t.Errorf("the last row is %q, want the one nothing can be done about here", options[2].value)
	}
}

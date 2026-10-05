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

	expired := errors.New("the SSO session for profile \"acme-sandbox\" has expired")
	err := guidedRun(context.Background(), catalog, &opts, ask, configuredLayout(t), configuredProfiles(), func(string, string, bool) error { return expired })

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
	if err := guidedRun(context.Background(), catalog, &opts, ask, configuredLayout(t), configuredProfiles(), func(env string, _ string, _ bool) error { checked = env; return nil }); err != nil {
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
	// Bootstrapped, which is the state these tests are about: with no state
	// backend the target list offers bootstrap and nothing else, by design.
	for _, environment := range infra.Environments {
		writeBackendFile(t, checkout, environment)
	}
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
		{Profile: infra.AWSProfile{Name: "stale", SSOSession: "acme", CanSignIn: true}, Err: errors.New("expired")},
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

	// An account with no section is offered too: choosing it is how it gets set
	// up. It says that, rather than naming a second command to go and run.
	if byValue["elsewhere"].disabled {
		t.Error("an account that could be set up was offered as unusable")
	}
	if !strings.Contains(byValue["elsewhere"].note, "sets it up") {
		t.Errorf("the row does not say what choosing it does: %q", byValue["elsewhere"].note)
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
	ask, painted := selectorFor(t, keyEnterSeq)
	opts := options{}

	// No profiles and no sections: nothing to offer from either side. A usable
	// profile here would have been offered as an account to set up, and the run
	// would have called the real init — which is what this test used to do while
	// claiming to cover the empty case.
	err = guidedRun(context.Background(), infra.Catalog{}, &opts, ask, layout, nil, nil)

	if err == nil {
		t.Fatal("a run was offered an account it cannot deploy into")
	}
	if !strings.Contains(err.Error(), "lerian infra init") {
		t.Errorf("the error does not say how to configure one: %v", err)
	}
	if strings.Contains(painted.String(), "Which AWS account") {
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
		{Profile: infra.AWSProfile{Name: "sandbox", SSOSession: "acme", CanSignIn: true}, Err: errors.New("expired")},
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
		{Profile: infra.AWSProfile{Name: "bbb-expired", SSOSession: "acme", CanSignIn: true}, Err: errors.New("expired")},
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

// With nothing ready, the question comes back — it is the only way to say which
// session to revive.
func TestNothingReadyStillAsks(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "sandbox", SSOSession: "acme", CanSignIn: true}, Err: errors.New("expired")},
	}

	previousLogin := ssoLogin
	ssoLogin = func(context.Context, infra.SSOTarget, io.Reader, io.Writer, io.Writer) error { return nil }
	t.Cleanup(func() { ssoLogin = previousLogin })
	previousIdentity := checkIdentity
	checkIdentity = stubIdentity{usable: map[string]bool{"sandbox": true}}
	t.Cleanup(func() { checkIdentity = previousIdentity })

	ask, painted := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	if err := guidedRun(context.Background(), catalog, &opts, ask, layout, resolved, nil); err != nil {
		t.Fatalf("guidedRun = %v\n%s", err, painted.String())
	}
	if !strings.Contains(painted.String(), "Which AWS account") {
		t.Errorf("with nothing ready there was no way to choose what to log into:\n%s", painted.String())
	}
}

// The account is always asked for, even when only one is ready.
//
// Deploying into an AWS account is not a step to be inferred on somebody's
// behalf: the operator says which one, every run, and the answer is visible in
// the scrollback afterwards.
func TestTheAccountIsAlwaysAsked(t *testing.T) {
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

	if !strings.Contains(painted.String(), "Which AWS account") {
		t.Errorf("the account was decided without asking:\n%s", painted.String())
	}
	if opts.environment != "dev" {
		t.Errorf("environment = %q", opts.environment)
	}
}

// And there is a way to arrive as somebody else: signing out and back in, which
// is the only way to change which identity the profiles resolve as.
func TestSigningOutIsOfferedAsAChoice(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "sandbox", SSOSession: "acme", CanSignIn: true}, Caller: infra.Caller{Account: "111122223333"}},
	}

	for _, opt := range accountOptions(layout, resolved) {
		if opt.value == signOutChoice {
			if opt.disabled {
				t.Error("the sign-out row cannot be chosen")
			}
			if !strings.Contains(opt.note, "ends the session") {
				t.Errorf("the row does not say what it costs: %q", opt.note)
			}
			return
		}
	}
	t.Error("there is no way to arrive as a different identity")
}

// Choosing it signs out, logs back in, and re-reads who the profiles are.
func TestSigningOutLogsBackIn(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	var order []string
	previousLogout := ssoLogout
	ssoLogout = func(context.Context, io.Reader, io.Writer, io.Writer) error {
		order = append(order, "logout")
		return nil
	}
	t.Cleanup(func() { ssoLogout = previousLogout })

	previousLogin := ssoLogin
	ssoLogin = func(_ context.Context, target infra.SSOTarget, _ io.Reader, _, _ io.Writer) error {
		order = append(order, "login:"+target.Name())
		return nil
	}
	t.Cleanup(func() { ssoLogin = previousLogin })

	previousIdentity := checkIdentity
	checkIdentity = stubIdentity{usable: map[string]bool{"sandbox": true}}
	t.Cleanup(func() { checkIdentity = previousIdentity })

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "sandbox", SSOSession: "acme", CanSignIn: true}, Caller: infra.Caller{Account: "111122223333"}},
	}

	var out bytes.Buffer
	// Down onto the sign-out row and Enter; Enter to accept the login; Enter again
	// for the account, because after signing in as somebody else the question is
	// asked afresh — which accounts are reachable has just changed.
	ask, _ := selectorFor(t, keyDownSeq+keyEnterSeq+keyEnterSeq+keyEnterSeq)

	choice, err := askForAccount(context.Background(), ask, &out, infra.Catalog{}, layout, resolved)
	if err != nil {
		t.Fatalf("askForAccount = %v\n%s", err, out.String())
	}

	if len(order) != 2 || order[0] != "logout" || order[1] != "login:acme" {
		t.Errorf("did %v, want a logout followed by a login", order)
	}
	if choice.environment != "dev" {
		t.Errorf("environment = %q after signing back in", choice.environment)
	}
}

// An account with no section is chosen, not refused.
//
// Telling somebody "run lerian infra init" is telling them to leave, learn a
// second command, and know which of three environment names to give it. They
// picked the account; setting it up is this command's job.
func TestAnUnsetAccountIsSetUpRatherThanRefused(t *testing.T) {
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
		{Profile: infra.AWSProfile{Name: "other", Region: "sa-east-1"}, Caller: infra.Caller{Account: "999988887777"}},
	}

	for _, opt := range accountOptions(layout, resolved) {
		if opt.value == "other" {
			if opt.disabled {
				t.Error("an account that could be set up was offered as unusable")
			}
			if strings.Contains(opt.note, "lerian infra init") {
				t.Errorf("the row still sends the operator to another command: %q", opt.note)
			}
			return
		}
	}
	t.Error("the unconfigured account is not in the list at all")
}

// Choosing it runs init for that account, with everything already known filled
// in — and the environment slot picked from whichever is free, because that is
// bookkeeping rather than a decision.
func TestChoosingAnUnsetAccountConfiguresIt(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	previous := runInitCommand
	runInitCommand = func(_ context.Context, args []string, _, _ io.Writer) error {
		got = args
		// What init would have written, so the run can carry on.
		writeEnvConfig(t, checkout, map[string]string{
			"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
			"stg": "account_id = 999988887777\nregion = sa-east-1\nprofile = other",
		})
		return nil
	}
	t.Cleanup(func() { runInitCommand = previous })

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "other", Region: "sa-east-1"}, Caller: infra.Caller{Account: "999988887777"}},
	}

	var out bytes.Buffer
	ask, _ := selectorFor(t, keyEnterSeq)

	choice, err := askForAccount(context.Background(), ask, &out, infra.Catalog{}, layout, resolved)
	if err != nil {
		t.Fatalf("askForAccount = %v\n%s", err, out.String())
	}

	// Everything already known is filled in — but not the region: init asks for
	// that one, because where every resource is created is a decision rather than
	// something to inherit from a profile configured for something else.
	joined := strings.Join(got, " ")
	for _, want := range []string{"--profile other", "--account 999988887777", "--env stg"} {
		if !strings.Contains(joined, want) {
			t.Errorf("init was run as %q, missing %q", joined, want)
		}
	}
	if strings.Contains(joined, "--region") {
		t.Errorf("init was given a region instead of asking for one: %q", joined)
	}
	if choice.environment != "stg" {
		t.Errorf("environment = %q, want the slot it was configured into", choice.environment)
	}
}

// Three is all there is. backend/<env>.hcl and envs/<env>.tfvars are files in the
// templates repo, one set per environment name, and there are three names — so a
// checkout already holding three accounts cannot take a fourth, and saying so is
// better than failing later with a missing file.
func TestAFourthAccountSaysWhatIsInTheWay(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111111111111\nregion = us-east-1\nprofile = one",
		"stg": "account_id = 222222222222\nregion = us-east-1\nprofile = two",
		"prd": "account_id = 333333333333\nregion = us-east-1\nprofile = three",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	_, err = configureAccount(context.Background(), &bytes.Buffer{}, layout,
		infra.ResolvedProfile{Profile: infra.AWSProfile{Name: "four"}, Caller: infra.Caller{Account: "444444444444"}})

	if err == nil {
		t.Fatal("a fourth account was configured into a checkout that holds three")
	}
	if !strings.Contains(err.Error(), "three") {
		t.Errorf("the error does not explain the limit: %v", err)
	}
}

// Credentials in the environment have no profile name, and a row labeled with an
// empty string is a row nobody can read. They are named for what they are.
func TestAmbientCredentialsAreNamedInTheList(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = -",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{{Caller: infra.Caller{Account: "111122223333"}}}

	options := accountOptions(layout, resolved)
	if len(options) == 0 {
		t.Fatal("ambient credentials produced no row")
	}
	if strings.TrimSpace(options[0].label) == "" {
		t.Errorf("the row has no label: %+v", options[0])
	}
	if !strings.Contains(options[0].label, "environment") {
		t.Errorf("label = %q, want it to say where the credentials come from", options[0].label)
	}
	if options[0].disabled {
		t.Error("ambient credentials that reach a configured account cannot be chosen")
	}
	// And they map to the environment whose section says "no profile".
	if got, ok := environmentForProfile(layout, "", "111122223333"); !ok || got != "dev" {
		t.Errorf("environmentForProfile = %q,%v — want dev", got, ok)
	}
}

// A dry run makes no AWS call, so it has no resolved profiles — and the account
// list was built entirely from those. The question became an error saying no
// section names a reachable account, which is false: the sections are right
// there, and nothing went looking.
//
// What a dry run can know is what the file says, so that is what it offers.
func TestADryRunOffersTheConfiguredAccounts(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
		"prd": "account_id = 999988887777\nregion = us-east-1\nprofile = production",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	ask, painted := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{dryRun: true}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	// No resolved profiles, which is what a dry run has.
	if err := guidedRun(context.Background(), catalog, &opts, ask, layout, nil, nil); err != nil {
		t.Fatalf("a dry run could not choose anything: %v\n%s", err, painted.String())
	}

	if !strings.Contains(painted.String(), "111122223333") {
		t.Errorf("the configured accounts were not offered:\n%s", painted.String())
	}
	if opts.environment == "" {
		t.Errorf("nothing was chosen:\n%s", painted.String())
	}
}

// With no profiles and no sections either, there is genuinely nothing — and the
// error says which of the two it is.
func TestNothingConfiguredAndNothingResolvedSaysSo(t *testing.T) {
	layout, err := infra.NewLayout(fakeCheckout(t, "", ""))
	if err != nil {
		t.Fatal(err)
	}

	ask, _ := selectorFor(t, keyEnterSeq)
	opts := options{}

	err = guidedRun(context.Background(), infra.Catalog{}, &opts, ask, layout, nil, nil)

	if err == nil {
		t.Fatal("an empty checkout with no credentials offered something")
	}
	if !strings.Contains(err.Error(), "lerian infra init") {
		t.Errorf("the error does not say how to start: %v", err)
	}
}

// A profile backed by an access key has no SSO session to revive, so offering
// `aws sso login --profile x` sends somebody to run a command that cannot work
// for them. Its own failure is the useful thing to show.
func TestAnAccessKeyProfileIsNotOfferedAnSSOLogin(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = keys",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	called := false
	previous := ssoLogin
	ssoLogin = func(context.Context, infra.SSOTarget, io.Reader, io.Writer, io.Writer) error {
		called = true
		return nil
	}
	t.Cleanup(func() { ssoLogin = previous })

	resolved := []infra.ResolvedProfile{{
		// In ~/.aws/credentials and nowhere else: an access key, no SSO.
		Profile: infra.AWSProfile{Name: "keys", Source: "credentials"},
		Err:     errors.New("the security token included in the request is expired"),
	}}

	ask, _ := selectorFor(t, keyEnterSeq)
	var out bytes.Buffer

	_, err = askForAccount(context.Background(), ask, &out, infra.Catalog{}, layout, resolved)

	if called {
		t.Error("an SSO login was offered for a profile that has no SSO session")
	}
	if err == nil {
		t.Fatal("a profile that cannot be used was accepted")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("the error does not carry what AWS said: %v", err)
	}
	if !strings.Contains(err.Error(), "aws configure") {
		t.Errorf("the error does not say how to fix an access key: %v", err)
	}
}

// The profile that was chosen is the profile the run uses.
//
// The environment is found by account, and a section may name a different
// profile that reaches the same one. Returning only the environment meant the
// run then read that section's profile — so picking the working profile A could
// still run as the expired profile B, with nothing on screen to say so.
func TestTheChosenProfileIsTheOneTheRunUses(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = written-in-the-file",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	// A different profile, reaching the same account.
	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "the-one-chosen"}, Caller: infra.Caller{Account: "111122223333"}},
	}

	ask, _ := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	if err := guidedRun(context.Background(), catalog, &opts, ask, layout, resolved, nil); err != nil {
		t.Fatalf("guidedRun = %v", err)
	}

	if opts.environment != "dev" {
		t.Errorf("environment = %q", opts.environment)
	}
	if opts.profile != "the-one-chosen" {
		t.Errorf("the run would use %q, but %q was chosen", opts.profile, "the-one-chosen")
	}
}

// Without the interactive flow there is nothing chosen, and the file decides —
// which is what a scripted run has always done.
func TestAScriptedRunKeepsTheProfileFromTheFile(t *testing.T) {
	opts := options{environment: "dev"}

	if opts.profile != "" {
		t.Errorf("a scripted run carries a chosen profile: %q", opts.profile)
	}
}

// And the chosen profile reaches the configuration the run uses.
//
// Storing it on the options is only half the job: the account guard, the
// credential resolution and every terraform process read EnvConfig.Profile, so
// the choice has to land there or it changes nothing.
func TestTheChosenProfileOverridesTheFile(t *testing.T) {
	config := infra.EnvConfig{Environment: "dev", AccountID: "111122223333", Profile: "written-in-the-file"}

	withChoice := applyChosenProfile(config, "the-one-chosen", true)
	if withChoice.Profile != "the-one-chosen" {
		t.Errorf("profile = %q, want the chosen one", withChoice.Profile)
	}
	// Everything else is the file's.
	if withChoice.AccountID != "111122223333" {
		t.Errorf("the account came from somewhere else: %q", withChoice.AccountID)
	}

	// A scripted run chooses nothing and the file stands.
	untouched := applyChosenProfile(config, "", false)
	if untouched.Profile != "written-in-the-file" {
		t.Errorf("a scripted run had its profile replaced with %q", untouched.Profile)
	}
}

// The early credential check has to use the profile that was chosen.
//
// It runs the moment the account is picked, which is the whole point of it — but
// it loaded the section and resolved that section's profile. Choosing the working
// profile A where the section names the expired profile B failed on B, before the
// run ever reached the place the choice is applied.
//
// Asserted on which profile would be resolved rather than on the callback being
// handed one: the first version of this test passed its own callback, so it only
// proved that guidedRun passes the value along — the half that was already
// working — and stayed green with the fix removed.
func TestTheEarlyCheckUsesTheChosenProfile(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = the-expired-one",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	chosen := credentialProfile(layout, "dev", "the-working-one", true)
	if chosen != "the-working-one" {
		t.Errorf("the early check would resolve %q, want the profile that was chosen", chosen)
	}

	// And a scripted run, which chose nothing, still checks the section's own.
	scripted := credentialProfile(layout, "dev", "", false)
	if scripted != "the-expired-one" {
		t.Errorf("a scripted run would resolve %q, want the file's", scripted)
	}

	// Ambient credentials have nothing to resolve; the account guard answers for
	// them a moment later.
	ambient := credentialProfile(layout, "dev", "", true)
	if ambient != "" {
		t.Errorf("ambient credentials would resolve %q", ambient)
	}
}

// And the choice reaches that check at all: guidedRun hands it over.
func TestTheChoiceReachesTheEarlyCheck(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = the-expired-one",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "the-working-one"}, Caller: infra.Caller{Account: "111122223333"}},
	}

	var handed []string
	ask, _ := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	err = guidedRun(context.Background(), catalog, &opts, ask, layout, resolved,
		func(_ string, profile string, _ bool) error {
			handed = append(handed, profile)
			return nil
		})
	if err != nil {
		t.Fatalf("guidedRun = %v", err)
	}

	if len(handed) != 1 || handed[0] != "the-working-one" {
		t.Errorf("the check was handed %v, want the profile that was chosen", handed)
	}
}

// Ambient credentials are chosen by name — and their name is the empty string,
// which is also what "nothing was chosen" looks like. The two mean opposite
// things: one says use the credentials in this environment, the other says leave
// the file alone.
//
// The same distinction initOptions already draws for --profile ” versus an
// absent --profile.
func TestChoosingAmbientCredentialsIsNotTheSameAsChoosingNothing(t *testing.T) {
	config := infra.EnvConfig{Environment: "dev", AccountID: "111122223333", Profile: "from-the-file"}

	chosenAmbient := applyChosenProfile(config, "", true)
	if chosenAmbient.Profile != "" {
		t.Errorf("profile = %q, want the ambient credentials that were chosen", chosenAmbient.Profile)
	}

	nothingChosen := applyChosenProfile(config, "", false)
	if nothingChosen.Profile != "from-the-file" {
		t.Errorf("profile = %q, want the file's own", nothingChosen.Profile)
	}

	named := applyChosenProfile(config, "a-named-one", true)
	if named.Profile != "a-named-one" {
		t.Errorf("profile = %q", named.Profile)
	}
}

// A dry run chooses no profile, and that is not the same as choosing the ambient
// credentials.
//
// The dry-run path asks from environments.conf and returns an empty profile
// because none was picked — but guidedRun was marking every interactive answer as
// a choice, so applyChosenProfile then cleared the profile the section declares.
// The run would print <ambient credentials> and hand terraform no profile at all.
//
// The third turn of one mistake: an empty profile means two different things, and
// only the code that asked knows which.
func TestADryRunChoosesNoProfile(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = from-the-file",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	var handed []bool
	ask, _ := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{dryRun: true}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	err = guidedRun(context.Background(), catalog, &opts, ask, layout, nil,
		func(_, _ string, chosen bool) error {
			handed = append(handed, chosen)
			return nil
		})
	if err != nil {
		t.Fatalf("guidedRun = %v", err)
	}

	if opts.profileChosen {
		t.Error("a dry run was recorded as having chosen a profile, which clears the section's")
	}
	if len(handed) != 1 || handed[0] {
		t.Errorf("the early check was told a profile was chosen: %v", handed)
	}
	// And the configuration keeps what the file says.
	config := infra.EnvConfig{Profile: "from-the-file"}
	if applyChosenProfile(config, opts.profile, opts.profileChosen).Profile != "from-the-file" {
		t.Error("the profile the section declares was replaced")
	}
}

// Where the infrastructure lands is part of choosing where to deploy, so the row
// says it. An account is a place, and so is a region; naming one without the
// other describes half the destination.
func TestTheAccountRowNamesTheRegion(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = sa-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "sandbox"}, Caller: infra.Caller{Account: "111122223333"}},
	}

	for _, opt := range accountOptions(layout, resolved) {
		if opt.value != "sandbox" {
			continue
		}
		if !strings.Contains(opt.note, "sa-east-1") {
			t.Errorf("the row does not say where the resources land: %q", opt.note)
		}
		return
	}
	t.Error("the configured account is not in the list")
}

// And the dry-run list, built from the file rather than from identities, says it
// too.
func TestTheDryRunListNamesTheRegion(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = sa-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	ask, painted := selectorFor(t, keyEnterSeq)
	if _, err := askFromConfig(ask, layout); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(painted.String(), "sa-east-1") {
		t.Errorf("the dry-run list does not name the region:\n%s", painted.String())
	}
}

// Setting up a new account does not inherit the region from the profile in
// silence. Where everything is created is a decision, and the profile's region is
// a suggestion about what to type — not an answer given on somebody's behalf.
func TestSettingUpAnAccountAsksForTheRegion(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = taken",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	var args []string
	previous := runInitCommand
	runInitCommand = func(_ context.Context, given []string, _, _ io.Writer) error {
		args = given
		writeEnvConfig(t, checkout, map[string]string{
			"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = taken",
			"stg": "account_id = 999988887777\nregion = eu-west-1\nprofile = fresh",
		})
		return nil
	}
	t.Cleanup(func() { runInitCommand = previous })

	// The profile declares a region, which used to be passed straight through.
	chosen := infra.ResolvedProfile{
		Profile: infra.AWSProfile{Name: "fresh", Region: "ap-northeast-1"},
		Caller:  infra.Caller{Account: "999988887777"},
	}

	if _, err := configureAccount(context.Background(), &bytes.Buffer{}, layout, chosen); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(strings.Join(args, " "), "--region") {
		t.Errorf("the region was decided from the profile instead of asked: %v", args)
	}
}

// The confirmation before a write names the region too. It is the last line read
// before typing yes, and "which account" without "where in it" is half the
// destination — an apply into the right account and the wrong region creates a
// second copy of everything, in a place nobody is looking at.
func TestTheConfirmationNamesWhereItLands(t *testing.T) {
	config := infra.EnvConfig{Environment: "dev", AccountID: "111122223333", Region: "sa-east-1"}

	line := destinationLine(config)

	for _, want := range []string{"dev", "111122223333", "sa-east-1"} {
		if !strings.Contains(line, want) {
			t.Errorf("the confirmation does not name %q: %q", want, line)
		}
	}
}

// "deploys as dev" is our bookkeeping showing through. The name picks
// backend/<env>.hcl and envs/<env>.tfvars, which matters to this tool and to
// nobody choosing where to deploy — they picked an account, in a region, and that
// is the whole of what they decided.
func TestTheRowDoesNotNameOurEnvironment(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = sa-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "sandbox"}, Caller: infra.Caller{Account: "111122223333"}},
	}

	for _, opt := range accountOptions(layout, resolved) {
		if opt.value != "sandbox" {
			continue
		}
		if strings.Contains(opt.note, "dev") {
			t.Errorf("the row shows our environment name: %q", opt.note)
		}
		// What it does say is where the resources land.
		for _, want := range []string{"111122223333", "sa-east-1"} {
			if !strings.Contains(opt.note, want) {
				t.Errorf("the row does not name %q: %q", want, opt.note)
			}
		}
		return
	}
	t.Error("the configured account is not in the list")
}

// Unless it is the only thing telling two rows apart. Two environments in one
// account is a real configuration — a dev and a staging sharing a sandbox — and
// then "account X in region Y" describes both, so the name earns its place.
func TestTheEnvironmentAppearsOnlyWhenItDisambiguates(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = sa-east-1\nprofile = one",
		"stg": "account_id = 111122223333\nregion = sa-east-1\nprofile = two",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "one"}, Caller: infra.Caller{Account: "111122223333"}},
		{Profile: infra.AWSProfile{Name: "two"}, Caller: infra.Caller{Account: "111122223333"}},
	}

	options := accountOptions(layout, resolved)

	var named int
	for _, opt := range options {
		if strings.Contains(opt.note, "dev") || strings.Contains(opt.note, "stg") {
			named++
		}
	}
	if named != 2 {
		t.Errorf("two rows reach the same account and %d name which is which:\n%+v", named, options)
	}
}

// The dry-run list is built from the file, where the sections are named dev, stg
// and prd — but that is the key of the section, not the name of the destination.
// The row reads as the account it is.
func TestTheDryRunRowsAreNotLabeledWithOurNames(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = sa-east-1\nprofile = sandbox",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	ask, painted := selectorFor(t, keyEnterSeq)
	chosen, err := askFromConfig(ask, layout)
	if err != nil {
		t.Fatal(err)
	}

	// The answer is still the environment, because that is what the rest of the run
	// needs — it is the label that changed.
	if chosen != "dev" {
		t.Errorf("askFromConfig returned %q, want the environment the run needs", chosen)
	}
	for _, line := range strings.Split(painted.String(), "\n") {
		if strings.Contains(line, "sandbox") && strings.Contains(line, "dev") {
			t.Errorf("the row is labeled with our environment name: %q", strings.TrimSpace(line))
		}
	}
	if !strings.Contains(painted.String(), "sandbox") {
		t.Errorf("the row does not name the profile it would use:\n%s", painted.String())
	}
}

// Until the state backend exists, bootstrap is the only thing that can run:
// everything else needs a bucket to keep its state in, and bootstrap is what
// creates it. The list says so instead of letting somebody pick a stack that
// fails at terraform init.
func TestBeforeBootstrapOnlyBootstrapIsOffered(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	// No backend/dev.hcl in this checkout.
	options := runTargetOptions(catalog, layout, "dev")

	byValue := map[string]option{}
	for _, opt := range options {
		byValue[opt.value] = opt
	}

	if byValue["bootstrap"].disabled {
		t.Error("bootstrap cannot be chosen, and it is the only thing that can run")
	}
	for _, name := range []string{"infra-base", "midaz", "all"} {
		if !byValue[name].disabled {
			t.Errorf("%s was offered with no state backend to write to", name)
		}
		if !strings.Contains(byValue[name].note, "bootstrap") {
			t.Errorf("%s does not say what is missing: %q", name, byValue[name].note)
		}
	}
}

// Once it exists, everything is on the table.
func TestAfterBootstrapEverythingIsOffered(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeBackendFile(t, checkout, "dev")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	for _, opt := range runTargetOptions(catalog, layout, "dev") {
		if opt.disabled {
			t.Errorf("%s is not offered although the backend exists: %q", opt.value, opt.note)
		}
	}
}

func writeBackendFile(t *testing.T, checkout, environment string) {
	t.Helper()

	path := filepath.Join(checkout, "examples", "aws", "backend", environment+".hcl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "bucket         = \"tfstate-" + environment + "-111122223333\"\n" +
		"region         = \"us-east-1\"\ndynamodb_table = \"tfstate-lock-" + environment + "\"\nencrypt = true\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A target whose tfvars were never written cannot run, and the list said nothing.
//
// init writes tfvars for the targets it was given — infra-base, usually — and the
// run menu offers the whole catalog. Choosing a product nobody configured
// spends two more answers and then fails with "4 of 4 stacks are NOT READY", which
// is a true message arriving three steps too late.
func TestATargetWithNoVariablesSaysSo(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeBackendFile(t, checkout, "dev")
	// infra-base is configured; midaz is in the catalog and was never set up.
	writeVarFile(t, checkout, "infra-base/vpc", "dev")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	byValue := map[string]option{}
	for _, opt := range runTargetOptions(catalog, layout, "dev") {
		byValue[opt.value] = opt
	}

	if !strings.Contains(byValue["midaz"].note, "not configured") {
		t.Errorf("midaz has no variables and the row does not say so: %q", byValue["midaz"].note)
	}
	if byValue["infra-base"].disabled {
		t.Errorf("infra-base is configured and was not offered: %q", byValue["infra-base"].note)
	}
	// Not disabled: choosing it is how it gets configured, the same as an account
	// that is not set up yet.
	if byValue["midaz"].disabled {
		t.Error("a target that only needs configuring was offered as unusable")
	}
}

func writeVarFile(t *testing.T, checkout, root, environment string) {
	t.Helper()

	path := filepath.Join(checkout, "examples", "aws", filepath.FromSlash(root), "envs", environment+".tfvars")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("environment = \""+environment+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Two sections reaching the same account, region and profile read identically in
// the dry-run list too, and there the environment is the only thing that tells
// them apart — the same rule the account rows follow.
func TestTheDryRunListDisambiguatesWhenItMust(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = sa-east-1\nprofile = same",
		"stg": "account_id = 111122223333\nregion = sa-east-1\nprofile = same",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	ask, painted := selectorFor(t, keyEnterSeq)
	if _, err := askFromConfig(ask, layout); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"dev", "stg"} {
		if !strings.Contains(painted.String(), name) {
			t.Errorf("two identical rows and %q is not shown to tell them apart:\n%s", name, painted.String())
		}
	}
}

// Going back re-asks the previous question rather than ending the run.
//
// The questions are a sequence, and the only way to correct the first one used to
// be ctrl-c — which throws away the ones answered correctly along with the
// mistake.
func TestGoingBackReAsksThePreviousQuestion(t *testing.T) {
	layout := configuredLayout(t)
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	// account, then back from the targets, then account again, then on through.
	ask, painted := selectorFor(t, keyEnterSeq+"r"+keyEnterSeq+keyEnterSeq+keyEnterSeq)
	opts := options{}

	if err := guidedRun(context.Background(), catalog, &opts, ask, layout, configuredProfiles(), nil); err != nil {
		t.Fatalf("guidedRun = %v\n%s", err, painted.String())
	}

	// The account question was painted twice: once, and once more on the way back.
	if asked := strings.Count(painted.String(), "Which AWS account?"); asked != 2 {
		t.Errorf("the account question was asked %d times, want 2 — once, then again on the way back", asked)
	}
	if opts.action == "" {
		t.Errorf("the run did not reach the end:\n%s", painted.String())
	}
}

// Back from the first question leaves the guided run, the way it would if there
// were nothing before it — because there is nothing before it.
func TestBackFromTheFirstQuestionLeaves(t *testing.T) {
	layout := configuredLayout(t)

	ask, _ := selectorFor(t, "r")
	opts := options{}

	err := guidedRun(context.Background(), infra.Catalog{}, &opts, ask, layout, configuredProfiles(), nil)

	if !errors.Is(err, infra.ErrAborted) {
		t.Errorf("guidedRun = %v, want the run to end", err)
	}
}

// Three reasons a profile does not resolve, and they need three different
// sentences: a session to revive, a key to replace, or nothing configured at all.
// Calling the third one "session expired" sends somebody to a login that fails
// with "Unable to locate credentials".
func TestAProfileWithNothingConfiguredSaysSo(t *testing.T) {
	layout, err := infra.NewLayout(fakeCheckout(t, "", ""))
	if err != nil {
		t.Fatal(err)
	}

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "empty", Source: "config"}, Err: errors.New("no credentials")},
		{Profile: infra.AWSProfile{Name: "stale", Source: "config", SSOSession: "acme", CanSignIn: true},
			Err: errors.New("expired")},
	}

	byValue := map[string]option{}
	for _, opt := range accountOptions(layout, resolved) {
		byValue[opt.value] = opt
	}

	if strings.Contains(byValue["empty"].note, "log in") {
		t.Errorf("a profile with nothing configured is offered a login: %q", byValue["empty"].note)
	}
	if !strings.Contains(byValue["empty"].note, "aws configure") {
		t.Errorf("the row does not say how to give it credentials: %q", byValue["empty"].note)
	}
	if byValue["empty"].disabled == false {
		t.Errorf("a profile nothing can be done with here is choosable: %+v", byValue["empty"])
	}

	// The one that can be revived still offers it.
	if !strings.Contains(byValue["stale"].note, "log in") {
		t.Errorf("an expired session is not offered a login: %q", byValue["stale"].note)
	}
}

// The targets this environment has variables for come first and arrive ticked.
// This question follows `init` writing those very files, and finding them on row
// 14 of 30 reads as the same list being asked again rather than as the short
// answer it is.
func TestConfiguredTargetsLeadTheList(t *testing.T) {
	layout := configuredLayout(t)
	catalog := infra.Catalog{
		Names:    []string{"zzz-last", "midaz"},
		Products: map[string][]string{"zzz-last": {"postgres"}, "midaz": {"postgres"}},
	}
	// midaz has variables; zzz-last sorts before it alphabetically and has none,
	// so only the ranking can put midaz above it.
	configureTarget(t, layout, "midaz", "postgres", "dev")

	options := runTargetOptions(catalog, layout, "dev")

	if options[0].value != "bootstrap" || options[1].value != "infra-base" {
		t.Fatalf("the fixed rows moved: %q, %q", options[0].value, options[1].value)
	}
	if options[2].value != "midaz" {
		t.Errorf("the configured target is not first below the foundation: %v", values(options))
	}
	if options[len(options)-1].value != "all" {
		t.Errorf("all is not last: %v", values(options))
	}
}

// configureTarget writes the variables file that makes a target count as
// configured, which is what targetIsConfigured reads.
func configureTarget(t *testing.T, layout infra.Layout, product, component, env string) {
	t.Helper()
	dir := filepath.Join(layout.ProductsDir(), product, component, "envs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, env+".tfvars"), []byte("region = \"us-east-1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Preselected from the disk, so the common run is Enter: whatever has tfvars is
// what somebody configured in order to deploy it.
func TestWhatIsConfiguredIsWhatIsPreselected(t *testing.T) {
	layout := configuredLayout(t)
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	ready := configuredTargets(catalog, layout, "dev")

	for _, name := range ready {
		if !targetIsConfigured(layout, catalog, name, "dev") {
			t.Errorf("%q is preselected and has no variables", name)
		}
	}
}

func values(options []option) []string {
	out := make([]string, 0, len(options))
	for _, opt := range options {
		out = append(out, opt.value)
	}
	return out
}

// Setting an account up means answering "what do you want to configure?" seconds
// earlier. Asking "what do you want to operate on?" straight after reads as the
// same list twice, and the only sensible second answer is the first one.
func TestTheTargetsAreNotAskedTwiceAfterSetup(t *testing.T) {
	layout := configuredLayout(t)
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	ask, painted := selectorFor(t, keyEnterSeq)
	opts := options{environment: "dev", target: "infra-base", targetsFromSetup: true}

	asked, err := askTargetStep(context.Background(), ask, catalog, layout, &opts)
	if err != nil {
		t.Fatalf("askTargetStep = %v", err)
	}

	if asked {
		t.Error("the target question was asked again after the setup chose it")
	}
	if strings.Contains(painted.String(), "What do you want to operate on") {
		t.Errorf("the list was painted anyway:\n%s", painted.String())
	}
	// Said, not silent: what is about to run has to be on screen.
	if !strings.Contains(painted.String(), "infra-base") {
		t.Errorf("what will run is not named:\n%s", painted.String())
	}
	if opts.target != "infra-base" {
		t.Errorf("target = %q", opts.target)
	}
}

// And with nothing decided for it, it still asks.
func TestTheTargetsAreAskedWhenNothingDecidedThem(t *testing.T) {
	layout := configuredLayout(t)
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	ask, painted := selectorFor(t, keyEnterSeq)
	opts := options{environment: "dev"}

	asked, err := askTargetStep(context.Background(), ask, catalog, layout, &opts)
	if err != nil {
		t.Fatalf("askTargetStep = %v", err)
	}
	if !asked {
		t.Errorf("the question was skipped with nothing to skip it for:\n%s", painted.String())
	}
}

// r has to reach the account question even when the step between them asked
// nothing. Landing on a screen that is not there returns immediately and moves
// forward again, so the key appears to do nothing at all.
func TestGoingBackSkipsAStepThatAskedNothing(t *testing.T) {
	asked := []bool{true, false, true}

	if back := previousQuestion(asked, 2); back != 0 {
		t.Errorf("r from the action lands on step %d, want the account at 0", back)
	}
	// And with nothing before it at all, there is nowhere to go.
	if back := previousQuestion([]bool{false, true}, 1); back != -1 {
		t.Errorf("previousQuestion = %d, want -1", back)
	}
}

// The skip has to be decided where the setup happens, not taken on trust. This
// drives the account step with an init that writes the variables, and checks the
// step carried that decision forward.
func TestSettingUpAnAccountDecidesTheTargets(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	previous := runInitCommand
	runInitCommand = func(_ context.Context, _ []string, _, _ io.Writer) error {
		// What init writes: the account map, and variables for the foundation.
		writeEnvConfig(t, checkout, map[string]string{
			"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = sandbox",
		})
		for _, unit := range []string{"vpc", "eks"} {
			dir := filepath.Join(checkout, "examples", "aws", "infra-base", unit, "envs")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "dev.tfvars"), []byte("region = \"us-east-1\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	}
	t.Cleanup(func() { runInitCommand = previous })

	resolved := []infra.ResolvedProfile{
		{Profile: infra.AWSProfile{Name: "sandbox", Region: "us-east-1"}, Caller: infra.Caller{Account: "111122223333"}},
	}
	ask, painted := selectorFor(t, keyEnterSeq)
	opts := options{}

	asked, err := askAccountStep(context.Background(), ask, catalog, layout, resolved, &opts, nil)
	if err != nil {
		t.Fatalf("askAccountStep = %v\n%s", err, painted.String())
	}
	if !asked {
		t.Error("the account question reported that it asked nothing")
	}

	if !opts.targetsFromSetup {
		t.Error("the setup chose the targets and the run will ask for them again")
	}
	if opts.target != "infra-base" {
		t.Errorf("target = %q, want what the setup wrote variables for", opts.target)
	}
}

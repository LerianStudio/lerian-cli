package infracli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// fakeCheckout builds a repository shaped like lerian-terraform-foundation, with
// whatever environments.conf and backend file the test needs. Every assertion
// below stops before Terraform or AWS is reached, so none of these tests needs
// either installed.
func fakeCheckout(t *testing.T, config, backend string) string {
	t.Helper()
	root := t.TempDir()
	aws := filepath.Join(root, "examples", "aws")

	for _, dir := range []string{
		filepath.Join(aws, "bootstrap"),
		filepath.Join(aws, "infra-base", "vpc"),
		filepath.Join(aws, "infra-base", "eks"),
		filepath.Join(aws, "products", "midaz", "postgres", "envs"),
		filepath.Join(aws, "products", "midaz", "valkey", "envs"),
		filepath.Join(aws, "backend"),
		// _modules and backend are the pair resolveLayout recognizes a checkout by,
		// so a fake that omits either one is not a checkout as far as the CLI is
		// concerned — which is exactly what TestRepoMustPointAtACheckout relies on.
		filepath.Join(aws, "_modules", "postgres-rds"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	for _, dir := range []string{
		filepath.Join(aws, "bootstrap"),
		filepath.Join(aws, "infra-base", "vpc"),
		filepath.Join(aws, "infra-base", "eks"),
		filepath.Join(aws, "products", "midaz", "postgres"),
		filepath.Join(aws, "products", "midaz", "valkey"),
	} {
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), nil, 0o600); err != nil {
			t.Fatalf("write main.tf: %v", err)
		}
	}
	if config != "" {
		if err := os.WriteFile(filepath.Join(aws, "environments.conf"), []byte(config), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	if backend != "" {
		if err := os.WriteFile(filepath.Join(aws, "backend", "dev.hcl"), []byte(backend), 0o600); err != nil {
			t.Fatalf("write backend: %v", err)
		}
	}
	return root
}

const goodConfig = "[dev]\naccount_id = 123456789012\nprofile = acme-dev\nregion = us-east-2\n"

func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := run(context.Background(), args, stdout, stderr)
	return stdout.String(), stderr.String(), err
}

func TestListNeedsNoEnvironmentAndNoConfiguration(t *testing.T) {
	// A checkout with no environments.conf at all: --list must still work, because
	// it is what an operator runs to find out what the flags accept.
	root := fakeCheckout(t, "", "")

	stdout, _, err := runCLI(t, "--repo", root, "--list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "midaz") || !strings.Contains(stdout, "postgres valkey") {
		t.Errorf("--list output does not show the discovered product:\n%s", stdout)
	}
}

// The offline account guard, end to end: a backend file left over from another
// account is caught before Terraform is looked up and before any AWS call.
func TestWrongAccountInTheBackendFileStopsTheRun(t *testing.T) {
	root := fakeCheckout(t, goodConfig, `
bucket         = "lerian-tfstate-dev-999999999999"
region         = "us-east-2"
dynamodb_table = "lerian-tfstate-lock-dev"
`)

	_, _, err := runCLI(t, "--repo", root, "--env", "dev", "--target", "midaz", "--dry-run")
	if err == nil {
		t.Fatal("the run continued with a backend belonging to another account")
	}
	if !strings.Contains(err.Error(), "account mismatch") {
		t.Errorf("error = %q, want the account mismatch", err)
	}
}

func TestMissingEnvironmentIsExplained(t *testing.T) {
	root := fakeCheckout(t, goodConfig, "")

	_, _, err := runCLI(t, "--repo", root, "--target", "midaz")
	if err == nil {
		t.Fatal("run succeeded without --env")
	}
	for _, want := range []string{"--env is required", "dev, stg, prd"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

// The most common typo is a product name, and it is pure filesystem work, so it
// is reported before the configuration is even opened.
func TestUnknownTargetIsReportedBeforeTheConfiguration(t *testing.T) {
	root := fakeCheckout(t, "", "")

	_, _, err := runCLI(t, "--repo", root, "--env", "dev", "--target", "ledger")
	if err == nil {
		t.Fatal("run succeeded with an unknown target")
	}
	if !strings.Contains(err.Error(), `unknown target "ledger"`) {
		t.Errorf("error = %q, want the unknown target rather than the missing config", err)
	}
}

func TestDestroyRefusesBootstrap(t *testing.T) {
	root := fakeCheckout(t, goodConfig, "")

	_, _, err := runCLI(t, "--repo", root, "--env", "dev",
		"--target", "bootstrap", "--action", "destroy")
	if !errors.Is(err, infra.ErrBootstrapDestroy) {
		t.Fatalf("error = %v, want ErrBootstrapDestroy", err)
	}
}

func TestRepoMustPointAtACheckout(t *testing.T) {
	t.Setenv("LERIAN_TF_REPO", "")

	_, _, err := runCLI(t, "--repo", t.TempDir(), "--list")
	if err == nil {
		t.Fatal("run succeeded outside a checkout")
	}
	if !strings.Contains(err.Error(), "LERIAN_TF_REPO") {
		t.Errorf("error = %q, want it to name every way of pointing at the repository", err)
	}
}

// The binary lives in the repository it drives, so the ordinary invocation has no
// --repo at all: the root is found by walking up from wherever the operator
// stands, which is usually several directories deep inside a stack.
func TestRepoIsDiscoveredByWalkingUpFromTheWorkingDirectory(t *testing.T) {
	t.Setenv("LERIAN_TF_REPO", "")
	root := fakeCheckout(t, "", "")
	t.Chdir(filepath.Join(root, "examples", "aws", "products", "midaz", "postgres"))

	stdout, _, err := runCLI(t, "--list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "midaz") {
		t.Errorf("--list did not resolve the checkout by walking up:\n%s", stdout)
	}
}

// A directory that holds only one of the two markers is not a checkout: the pair
// is what proves the AWS v2 layout, and half of it is what a partially-copied
// tree looks like.
func TestHalfTheMarkerIsNotACheckout(t *testing.T) {
	t.Setenv("LERIAN_TF_REPO", "")

	for _, marker := range []string{"_modules", "backend"} {
		t.Run(marker, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "examples", "aws", marker), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if _, _, err := runCLI(t, "--repo", root, "--list"); err == nil {
				t.Fatalf("a tree holding only examples/aws/%s was accepted", marker)
			}
		})
	}
}

func TestEnvironmentVariableLocatesTheRepo(t *testing.T) {
	root := fakeCheckout(t, "", "")
	t.Setenv("LERIAN_TF_REPO", root)
	t.Chdir(t.TempDir())

	stdout, _, err := runCLI(t, "--list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "midaz") {
		t.Errorf("$LERIAN_TF_REPO was not honored:\n%s", stdout)
	}
}

// Precedence, stated once: --repo wins over $LERIAN_TF_REPO, which wins over the
// walk. The variable here points at a directory that is not a checkout, so if it
// were consulted at all the run would fail rather than quietly use the wrong one.
func TestRepoFlagBeatsTheEnvironmentVariable(t *testing.T) {
	root := fakeCheckout(t, "", "")
	t.Setenv("LERIAN_TF_REPO", t.TempDir())
	t.Chdir(t.TempDir())

	stdout, _, err := runCLI(t, "--repo", root, "--list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "midaz") {
		t.Errorf("--repo did not take precedence over $LERIAN_TF_REPO:\n%s", stdout)
	}
}

func TestOutsideACheckoutTheErrorNamesEveryWayOfPointingAtOne(t *testing.T) {
	t.Setenv("LERIAN_TF_REPO", "")
	// And no remembered one either. A config is a source like the other four, so
	// without this the test asserts "nothing points anywhere" on a machine where
	// something does — and passes or fails according to whose machine runs it.
	isolatedHome(t)
	t.Chdir(t.TempDir())

	_, _, err := runCLI(t, "--list")
	if err == nil {
		t.Fatal("run succeeded outside any checkout")
	}
	for _, want := range []string{"--repo", "LERIAN_TF_REPO", "from inside the checkout"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestInvalidFlagValuesAreRejected(t *testing.T) {
	root := fakeCheckout(t, goodConfig, "")

	tests := []struct {
		name   string
		args   []string
		wantIn string
	}{
		{"action", []string{"--action", "aplly"}, "invalid action"},
		{"jobs", []string{"--jobs", "0"}, "invalid --jobs"},
		{"environment", []string{"--env", "prod"}, `invalid --env "prod"`},
		// Not a provider name: aws/azure/gcp carry their own redirects, asserted
		// by TestProviderPositionalRedirect.
		{"positional argument", []string{"midaz"}, "unexpected argument"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--repo", root, "--env", "dev"}, test.args...)
			_, _, err := runCLI(t, args...)
			if err == nil {
				t.Fatal("run succeeded, want an error")
			}
			if !strings.Contains(err.Error(), test.wantIn) {
				t.Errorf("error = %q, want it to contain %q", err, test.wantIn)
			}
		})
	}
}

// deploy.sh took the provider positionally and pointed azure/gcp at the legacy
// script. Removing it did not remove that signpost, and must not: examples/gcp and
// examples/azure are still on the pre-v2 layout, and only deploy-legacy.sh reaches
// them.
func TestProviderPositionalRedirect(t *testing.T) {
	for _, tc := range []struct{ arg, want string }{
		{"gcp", "deploy-legacy.sh"},
		{"azure", "deploy-legacy.sh"},
		{"aws", "--env dev"},
	} {
		var out, errOut bytes.Buffer
		err := run(context.Background(), []string{tc.arg}, &out, &errOut)
		if err == nil {
			t.Fatalf("%s: expected an error", tc.arg)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error should mention %q, got:\n%s", tc.arg, tc.want, err)
		}
	}
}

// checkoutTree builds a minimal directory that IsCheckout recognizes.
func checkoutTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range [][]string{
		{"examples", "aws", "_modules"},
		{"examples", "aws", "backend"},
	} {
		if err := os.MkdirAll(filepath.Join(append([]string{root}, dir...)...), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The managed checkout is LAST on purpose. An operator inside a development
// checkout must keep driving that one: resolving to the managed tree because it
// also happens to exist would run their command against different templates than
// the ones they are editing, and nothing in the output would say so.
func TestResolveLayoutPrefersTheWorkingDirectoryOverTheManagedCheckout(t *testing.T) {
	managed := checkoutTree(t)
	working := checkoutTree(t)

	t.Chdir(working)

	layout, source, err := resolveLayout("", "", managed)
	if err != nil {
		t.Fatal(err)
	}
	if source != sourceWorkingIn {
		t.Errorf("source = %q, want %q", source, sourceWorkingIn)
	}
	// macOS resolves TempDir through /var -> /private/var, so compare by suffix.
	if !strings.HasSuffix(layout.Root, filepath.Base(working)) {
		t.Errorf("layout.Root = %q, want the working checkout %q", layout.Root, working)
	}
}

// With nothing named and nothing nearby, the managed checkout is the fallback —
// which is what makes a binary downloaded from the releases page usable from any
// directory.
func TestResolveLayoutFallsBackToTheManagedCheckout(t *testing.T) {
	managed := checkoutTree(t)

	// A directory that is NOT inside any checkout.
	t.Chdir(t.TempDir())

	layout, source, err := resolveLayout("", "", managed)
	if err != nil {
		t.Fatalf("expected the managed checkout to resolve: %v", err)
	}
	if source != sourceManaged {
		t.Errorf("source = %q, want %q", source, sourceManaged)
	}
	if !strings.HasSuffix(layout.Root, filepath.Base(managed)) {
		t.Errorf("layout.Root = %q, want %q", layout.Root, managed)
	}
}

// "Not found" without "here is where I looked" leaves the operator guessing which
// of four mechanisms they got wrong. The failure must name all four AND show the
// value each one had.
func TestResolveLayoutFailureNamesEveryPlaceItLooked(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())

	_, _, err := resolveLayout("", "", filepath.Join(t.TempDir(), "absent"))
	if err == nil {
		t.Fatal("expected a failure with no checkout anywhere")
	}
	for _, want := range []string{
		"--repo", "$LERIAN_TF_REPO", "the working directory and parents",
		"the managed checkout", "init --clone",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure must mention %q:\n%v", want, err)
		}
	}
}

// A path the operator named that is not a checkout is a different failure from
// having named nothing — and it must still list every way of pointing at one.
func TestNotACheckoutStillListsAllFourOptions(t *testing.T) {
	empty := t.TempDir()
	_, _, err := resolveLayout(empty, "", "")
	if err == nil {
		t.Fatal("an empty directory is not a checkout")
	}
	if !strings.Contains(err.Error(), "init --clone") {
		t.Errorf("the fourth option must be offered here too:\n%v", err)
	}
	if !strings.Contains(err.Error(), "--repo") {
		t.Errorf("the failure must name the source it resolved from:\n%v", err)
	}
}

// The spinner repaints, so it must run only where a repaint means something. This
// was a documented invariant every caller had to remember, and the second caller
// forgot it: piping the output produced every frame on its own line, which in a CI
// log is hundreds of lines of carriage-return debris. The constructor enforces it
// now, so no future caller can reintroduce it.
func TestSpinnerIsInertWhenTheDestinationCannotRepaint(t *testing.T) {
	var mu sync.Mutex
	out := &bytes.Buffer{}

	spin := newSpinner(&mu, out, "cloning v1.6.0")
	// Long enough that a live spinner would have painted several frames.
	time.Sleep(150 * time.Millisecond)
	spin.Stop()

	if out.Len() != 0 {
		t.Errorf("a buffer is not a terminal, so nothing should have been painted:\n%q", out.String())
	}
	// Stop must be safe, and safe twice: the inert path closes no channel.
	spin.Stop()
}

// The incident behind this guard: credentials are resolved once for the whole run,
// the session had ~71 min left, the apply took 96. AWS finished the change, the
// state write lost its credential, the lock stayed held by a dead process and an
// errored.tfstate was left behind.
func TestCredentialLifetimeGuard(t *testing.T) {
	expiring := func(left time.Duration) infra.Credentials {
		return infra.Credentials{AccessKeyID: "AKIA", Expiration: time.Now().Add(left)}
	}

	tests := []struct {
		name        string
		credentials infra.Credentials
		action      infra.Action
		floor       time.Duration
		wantErr     bool
		wantOut     string
	}{
		{
			name:        "apply below the floor is refused",
			credentials: expiring(71 * time.Minute),
			action:      infra.ActionApply,
			floor:       2 * time.Hour,
			wantErr:     true,
		},
		{
			name:        "destroy below the floor is refused too",
			credentials: expiring(10 * time.Minute),
			action:      infra.ActionDestroy,
			floor:       time.Hour,
			wantErr:     true,
		},
		{
			name:        "apply above the floor runs and says how long it has",
			credentials: expiring(4 * time.Hour),
			action:      infra.ActionApply,
			floor:       time.Hour,
			wantOut:     "credential  4h0m0s left",
		},
		{
			name:        "a plan is never blocked: it writes no state",
			credentials: expiring(time.Minute),
			action:      infra.ActionPlan,
			floor:       8 * time.Hour,
		},
		{
			name:        "with no floor a short session is reported, not refused",
			credentials: expiring(20 * time.Minute),
			action:      infra.ActionApply,
			wantOut:     "short — a long apply may outlive it",
		},
		{
			name:        "a session with no expiry says nothing at all",
			credentials: infra.Credentials{AccessKeyID: "AKIA"},
			action:      infra.ActionApply,
			floor:       8 * time.Hour,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			err := guardCredentialLifetime(&out, test.credentials, test.action, test.floor, "acme-dev")

			if test.wantErr && err == nil {
				t.Fatalf("the run was allowed to start; output was %q", out.String())
			}
			if !test.wantErr && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if test.wantOut != "" && !strings.Contains(out.String(), test.wantOut) {
				t.Errorf("output = %q, want it to contain %q", out.String(), test.wantOut)
			}
			if test.wantOut == "" && !test.wantErr && out.Len() != 0 {
				t.Errorf("nothing should have been said, got %q", out.String())
			}
		})
	}
}

// The refusal has to tell the operator what to do about it, not just that it
// happened: an error that names no command leaves them guessing.
func TestCredentialLifetimeRefusalNamesTheRemedy(t *testing.T) {
	var out bytes.Buffer
	err := guardCredentialLifetime(&out,
		infra.Credentials{AccessKeyID: "AKIA", Expiration: time.Now().Add(30 * time.Minute)},
		infra.ActionApply, 2*time.Hour, "acme-dev")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	// The profile has to be named: this refusal happens before the preflight prints
	// it, and "aws sso login --profile <profile>" is not a command anyone can run.
	// And SSO is only one way a profile gets a session, so it is offered as a case
	// rather than as the answer.
	for _, want := range []string{"30m0s", "2h0m0s", "acme-dev", "IAM Identity Center", "anything else"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q:\n%v", want, err)
		}
	}
}

// The preflight check is not enough on its own: planning a large target and then
// waiting for a human to answer can burn most of a session, and later stages start
// later still. credentialLifetimeError is what the per-stage hook calls.
func TestCredentialLifetimeIsCheckedAgainAtWriteTime(t *testing.T) {
	// A session that was fine when the run started and is not fine now.
	expired := infra.Credentials{AccessKeyID: "AKIA", Expiration: time.Now().Add(5 * time.Minute)}

	if err := credentialLifetimeError(expired, infra.ActionApply, time.Hour, "acme-dev"); err == nil {
		t.Error("a stage was allowed to write with less than the floor left")
	}
	if err := credentialLifetimeError(expired, infra.ActionDestroy, time.Hour, "acme-dev"); err == nil {
		t.Error("destroy was allowed to write with less than the floor left")
	}
	// Without a floor there is nothing to enforce, only something to report.
	if err := credentialLifetimeError(expired, infra.ActionApply, 0, "acme-dev"); err != nil {
		t.Errorf("no floor was demanded, yet the run was refused: %v", err)
	}
	// A plan writes no state.
	if err := credentialLifetimeError(expired, infra.ActionPlan, time.Hour, "acme-dev"); err != nil {
		t.Errorf("a plan was refused: %v", err)
	}
}

// Without a profile in hand the hint must still be usable rather than printing a
// literal placeholder as if it were a command.
func TestRefreshHintWithoutAProfileStillReadsAsInstructions(t *testing.T) {
	hint := refreshHint("")
	for _, want := range []string{"IAM Identity Center", "anything else"} {
		if !strings.Contains(hint, want) {
			t.Errorf("the hint does not mention %q:\n%s", want, hint)
		}
	}
}

// A recorded checkout beats the managed path.
//
// The managed path is a convention — "there is a directory at ~/lerian" — while a
// recorded one was somebody saying which checkout to use. With the convention
// winning, a client who had cloned the templates somewhere of their own had no way
// to make the CLI use it: the answer was never asked for, because the managed
// path answered first.
func TestARecordedCheckoutBeatsTheManagedPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())

	// Both exist: one by convention, one because somebody chose it.
	managed, err := infra.ManagedCheckoutPath("")
	if err != nil {
		t.Fatal(err)
	}
	makeCheckout(t, managed)
	chosen := fakeCheckout(t, "", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = chosen
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	layout, source, err := resolveLayout("", "", "")
	if err != nil {
		t.Fatal(err)
	}

	if layout.Root != chosen {
		t.Errorf("resolved to %q, want the one that was chosen: %q", layout.Root, chosen)
	}
	if source != sourceRemembered {
		t.Errorf("source = %q, want remembered", source)
	}
}

// Standing inside a checkout still wins over both: whatever was recorded, the one
// somebody is looking at is the one they mean.
func TestTheWorkingDirectoryStillWinsOverARecordedOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	recorded := fakeCheckout(t, "", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = recorded
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	standing := fakeCheckout(t, "", "")
	t.Chdir(standing)

	layout, source, err := resolveLayout("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if layout.Root != standing {
		t.Errorf("resolved to %q, want the working directory %q", layout.Root, standing)
	}
	if source != sourceWorkingIn {
		t.Errorf("source = %q", source)
	}
}

func makeCheckout(t *testing.T, dir string) {
	t.Helper()
	for _, marker := range []string{"examples/aws/_modules", "examples/aws/backend"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(marker)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// The alternative to this offer is what the advice above it prints: a command to
// copy into another terminal, in a directory nobody memorizes, with a uuid in
// it. But it is never taken on one keypress — the cost of being wrong lands on
// somebody else's apply, not on this run.
func TestReleasingALockTakesTwoAnswers(t *testing.T) {
	locked := []infra.StageResult{{
		Plans: []infra.UnitResult{{
			Unit: infra.Unit{Name: "infra-base/eks", Dir: t.TempDir()},
			Err:  errors.New(lockedFailure),
		}},
	}}

	t.Run("declining the menu runs nothing", func(t *testing.T) {
		noDrain(t)
		var out bytes.Buffer
		spy := &spyUnlocker{}
		ask, painted := selectorFor(t, keyEnterSeq) // "leave it", the first row

		offerUnlock(context.Background(), ask, spy, &out, locked)

		if spy.ran {
			t.Error("it released the lock after being told to leave it")
		}

		if !strings.Contains(painted.String(), "Release the lock on infra-base/eks?") {
			t.Errorf("it did not offer:\n%s", painted.String())
		}
		// Who and how long ago, on the line the decision turns on.
		if !strings.Contains(painted.String(), "48 minute") &&
			!strings.Contains(painted.String(), "hour") {
			t.Errorf("the offer does not say how old the lock is:\n%s", painted.String())
		}
	})

	t.Run("declining the typed confirmation runs nothing", func(t *testing.T) {
		noDrain(t)
		var out bytes.Buffer
		spy := &spyUnlocker{}
		ask, _ := selectorFor(t, keyDownSeq+keyEnterSeq+"no\n")

		offerUnlock(context.Background(), ask, spy, &out, locked)

		if spy.ran {
			t.Error("it released after the confirmation was declined")
		}
	})

	t.Run("both answers release the one that is held", func(t *testing.T) {
		noDrain(t)
		var out bytes.Buffer
		spy := &spyUnlocker{}
		ask, _ := selectorFor(t, keyDownSeq+keyEnterSeq+"yes\n")

		offerUnlock(context.Background(), ask, spy, &out, locked)

		if !spy.ran {
			t.Fatalf("it did not release:\n%s", out.String())
		}
		if spy.id != "6b012100-a4eb-91d3-d0f2-e0a4ff5085d1" {
			t.Errorf("it released %q", spy.id)
		}
		if spy.unit.Name != "infra-base/eks" {
			t.Errorf("it released the lock of %q", spy.unit.Name)
		}
		if !strings.Contains(out.String(), "Run the same command again") {
			t.Errorf("it did not say what to do next:\n%s", out.String())
		}
	})

	// Guarded twice: offerUnlock returns early, and the selector refuses to run
	// without a terminal anyway. Removing the early return does not change what
	// this asserts — the behaviour is what matters, and it holds either way.
	t.Run("nothing is offered without a terminal", func(t *testing.T) {
		var out bytes.Buffer
		spy := &spyUnlocker{}

		offerUnlock(context.Background(), &prompter{out: &out}, spy, &out, locked)

		if spy.ran {
			t.Error("it released a lock with nobody to ask")
		}
		if out.String() != "" {
			t.Errorf("it asked with nobody there:\n%s", out.String())
		}
	})

	t.Run("a failure that is not a lock is left alone", func(t *testing.T) {
		var out bytes.Buffer
		ask, painted := selectorFor(t, keyEnterSeq)

		spy := &spyUnlocker{}
		offerUnlock(context.Background(), ask, spy, &out, []infra.StageResult{{
			Plans: []infra.UnitResult{{
				Unit: infra.Unit{Name: "infra-base/eks"},
				Err:  errors.New("Error: no matching EC2 VPC found"),
			}},
		}})

		if painted.String() != "" {
			t.Errorf("it offered to unlock something that is not locked:\n%s", painted.String())
		}
		if spy.ran {
			t.Error("it released a lock that was not reported")
		}
	})
}

// lockedFailure is the message terraform produces, trimmed to what matters.
const lockedFailure = `infra: terraform plan failed for infra-base/eks: exit status 1

Error: Error acquiring the state lock

Lock Info:
  ID:        6b012100-a4eb-91d3-d0f2-e0a4ff5085d1
  Path:      bucket/aws/infra-base/eks/terraform.tfstate
  Operation: OperationTypeApply
  Who:       someone@their-laptop.local
  Version:   1.16.3
  Created:   2020-01-01 00:00:00.000000 +0000 UTC
  Info:
`

// spyUnlocker records whether the command ran, and on what.
type spyUnlocker struct {
	ran  bool
	unit infra.Unit
	id   string
}

func (s *spyUnlocker) ForceUnlock(_ context.Context, unit infra.Unit, id string) error {
	s.ran, s.unit, s.id = true, unit, id
	return nil
}

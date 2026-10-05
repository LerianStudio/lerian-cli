package infracli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// fakeBackends answers the account question without credentials.
type fakeBackends struct {
	found []infra.StateBackend
	err   error
	calls int
}

func (f *fakeBackends) ListStateBackends(_ context.Context, _, _, _ string) ([]infra.StateBackend, error) {
	f.calls++
	return f.found, f.err
}

// The missing file means "nobody bootstrapped THIS CHECKOUT", which is not the
// same as "nobody bootstrapped this account": a fresh clone, or a colleague who
// ran bootstrap from their own machine, produces the first without the second.
// Treating them as one sent somebody to create a second state bucket beside the
// one already holding their infrastructure's state.
func TestAnExistingBackendIsFoundAndAdopted(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	lister := &fakeBackends{found: []infra.StateBackend{{
		Bucket:    "lerian-tfstate-dev-123456789012",
		Region:    "us-east-2",
		Env:       "dev",
		LockTable: "lerian-tfstate-lock-dev",
	}}}

	// Adopting is a confirmation now, not a menu: the bucket either is this
	// environment's own or is not a candidate at all.
	noDrain(t)
	var painted bytes.Buffer
	ask := &prompter{interactive: true, in: bufio.NewReader(strings.NewReader("yes\n")), out: &painted}

	if err := resolveBackend(context.Background(), ask, &painted, lister, layout,
		"dev", "someprofile", "us-east-2", "123456789012"); err != nil {
		t.Fatalf("resolveBackend = %v\n%s", err, painted.String())
	}

	written, err := os.ReadFile(layout.BackendFile("dev"))
	if err != nil {
		t.Fatalf("the backend file was not written: %v\n%s", err, painted.String())
	}
	for _, want := range []string{
		`bucket         = "lerian-tfstate-dev-123456789012"`,
		`region         = "us-east-2"`,
		`dynamodb_table = "lerian-tfstate-lock-dev"`,
		"encrypt        = true",
	} {
		if !strings.Contains(string(written), want) {
			t.Errorf("the adopted backend file is missing %q:\n%s", want, written)
		}
	}
}

// With a file already there, the account is not asked at all: the question is
// answered, and the answer is the bucket the state is under.
func TestAnAccountIsNotAskedWhenTheFileIsThere(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.BackendDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.BackendFile("dev"),
		[]byte("bucket = \"already-here\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	lister := &fakeBackends{}
	var out bytes.Buffer
	if err := resolveBackend(context.Background(), newPrompter(&out), &out, lister, layout,
		"dev", "p", "us-east-1", "123456789012"); err != nil {
		t.Fatal(err)
	}

	if lister.calls != 0 {
		t.Errorf("the account was called %d times for a question already answered", lister.calls)
	}
}

// An empty account is fact too, and it is the fact that makes bootstrap the
// right next step rather than a guess.
func TestNoBackendInTheAccountSaysSo(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := resolveBackend(context.Background(), newPrompter(&out), &out, &fakeBackends{}, layout,
		"dev", "p", "us-east-1", "123456789012"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "none in account 123456789012") {
		t.Errorf("the empty account is not reported:\n%s", out.String())
	}
	if _, err := os.Stat(layout.BackendFile("dev")); !os.IsNotExist(err) {
		t.Error("a backend file was written for a backend that does not exist")
	}
}

// A failed lookup is unknown, not no. Blocking the run over a question that
// only exists to prevent a duplicate bucket would trade a small risk for a
// certain stoppage.
func TestAFailedLookupDoesNotBlockTheRun(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	lister := &fakeBackends{err: os.ErrPermission}

	if err := resolveBackend(context.Background(), newPrompter(&out), &out, lister, layout,
		"dev", "p", "us-east-1", "123456789012"); err != nil {
		t.Fatalf("resolveBackend = %v, want the run to continue", err)
	}
	if !strings.Contains(out.String(), "could not list") {
		t.Errorf("the failure is not reported:\n%s", out.String())
	}
}

// Only this environment's own bucket is a candidate. lerian-tfstate-stg-<account>
// is stg's state, and adopting it as prd's would point two environments at one
// state file — the one mistake this screen must not make possible.
func TestOnlyThisEnvironmentsOwnBackendIsAdopted(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	// The account holds dev and stg, and prd is what was chosen.
	lister := &fakeBackends{found: []infra.StateBackend{
		{Bucket: "lerian-tfstate-dev-111122223333", Env: "dev", Region: "us-east-2"},
		{Bucket: "lerian-tfstate-stg-111122223333", Env: "stg", Region: "us-east-2"},
	}}

	var out bytes.Buffer
	if err := resolveBackend(context.Background(), newPrompter(&out), &out, lister, layout,
		"prd", "p", "us-east-2", "111122223333"); err != nil {
		t.Fatal(err)
	}

	if _, statErr := os.Stat(layout.BackendFile("prd")); !os.IsNotExist(statErr) {
		t.Error("another environment's bucket was adopted as prd's")
	}
	if !strings.Contains(out.String(), "none of them prd's") {
		t.Errorf("it does not say why nothing was adopted:\n%s", out.String())
	}
	// And it does not offer a menu of other environments' buckets.
	for _, other := range []string{"lerian-tfstate-dev-111122223333", "lerian-tfstate-stg-111122223333"} {
		if strings.Contains(out.String(), other) {
			t.Errorf("%s was offered for prd:\n%s", other, out.String())
		}
	}
}

// A backend with no lock table is usable and unsafe for concurrent runs. That is
// the one thing worth knowing before adopting it.
func TestAMissingLockTableIsSaidOutLoud(t *testing.T) {
	described := describeBackend(infra.StateBackend{
		Bucket: "lerian-tfstate-dev-123456789012", Env: "dev", Region: "us-east-1",
	})

	if !strings.Contains(described, "no lock table") {
		t.Errorf("describeBackend = %q", described)
	}
	if !strings.Contains(described, "us-east-1") {
		t.Errorf("the region is not shown, and adopting one in the wrong region fails at init: %q", described)
	}
}

// Overwriting points a stack at a different bucket, which is how it ends up
// applying against state it has never seen.
func TestAdoptingRefusesToOverwrite(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.BackendDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := []byte("bucket = \"the-one-the-state-is-in\"\n")
	if err := os.WriteFile(layout.BackendFile("dev"), existing, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = infra.AdoptBackend(layout, "dev", infra.StateBackend{
		Bucket: "a-different-bucket", Region: "us-east-1",
	})

	if err == nil {
		t.Fatal("the existing backend file was overwritten")
	}
	// The message, not merely an error: the branch below this guard reports
	// "cannot read" for the same input, so an assertion that something failed
	// passes with the guard deleted. Mine did.
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
	after, readErr := os.ReadFile(layout.BackendFile("dev"))
	if readErr != nil || string(after) != string(existing) {
		t.Errorf("the file changed: %q", after)
	}
}

// Without a region terraform init cannot reach the bucket, and a file written
// without one fails later with a redirect nobody reads as a region problem.
func TestAdoptingNeedsTheRegion(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	_, err = infra.AdoptBackend(layout, "dev", infra.StateBackend{Bucket: "somewhere"})

	if err == nil {
		t.Fatal("a backend file was written with no region")
	}
	if _, statErr := os.Stat(filepath.Join(layout.BackendDir(), "dev.hcl")); !os.IsNotExist(statErr) {
		t.Error("the file was created anyway")
	}
}

// The whole point of asking the account: with a backend adopted, the target list
// stops being one row. Driven through askTargetStep rather than through
// resolveBackend, because a test of the helper proves the helper works and says
// nothing about whether the step calls it.
func TestAdoptingABackendUnlocksTheTargetList(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = dev-profile",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	catalog := infra.Catalog{Names: []string{"midaz"}, Products: map[string][]string{"midaz": {"postgres"}}}

	// Before: no file, so bootstrap is the only thing that can run.
	locked := runTargetOptions(catalog, layout, "dev")
	for _, opt := range locked {
		if opt.value != "bootstrap" && !opt.disabled {
			t.Fatalf("%q is offered with no state backend", opt.value)
		}
	}

	// "yes" adopts the backend; the Enter after it takes the target the cursor is
	// on.
	noDrain(t)
	ask, painted := selectorFor(t, "yes\n"+keyEnterSeq)
	opts := options{
		environment: "dev",
		profile:     "dev-profile",
		backends: &fakeBackends{found: []infra.StateBackend{{
			Bucket:    "lerian-tfstate-dev-111122223333",
			Region:    "us-east-1",
			Env:       "dev",
			LockTable: "lerian-tfstate-lock-dev",
		}}},
	}

	if _, err := askTargetStep(context.Background(), ask, catalog, layout, &opts); err != nil {
		t.Fatalf("askTargetStep = %v\n%s", err, painted.String())
	}

	// After: the file is there, and every target is runnable.
	if !backendExists(layout, "dev") {
		t.Fatalf("the backend was not adopted:\n%s", painted.String())
	}
	for _, opt := range runTargetOptions(catalog, layout, "dev") {
		// Except bootstrap, which now has nothing to create.
		if opt.value == "bootstrap" {
			continue
		}
		if opt.disabled {
			t.Errorf("%q is still disabled after adopting a backend", opt.value)
		}
	}
}

// Nobody to answer means nothing to do with the answer, and the scan would
// spend an AWS call on every CI run to print something no one asked for.
func TestNoTerminalMeansNoLookup(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	writeEnvConfig(t, checkout, map[string]string{
		"dev": "account_id = 111122223333\nregion = us-east-1\nprofile = dev-profile",
	})
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	lister := &fakeBackends{}
	opts := options{environment: "dev", backends: lister}
	ask := &prompter{interactive: false, out: &bytes.Buffer{}}

	if err := askAboutBackend(context.Background(), ask, layout, &opts); err != nil {
		t.Fatal(err)
	}
	if lister.calls != 0 {
		t.Errorf("the account was called %d times with nobody to answer", lister.calls)
	}
}

// Adopting writes a file that decides where state lives, so it is confirmed.
// Removing the confirmation must break a test rather than pass quietly.
func TestAdoptingIsConfirmedBeforeItWrites(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	lister := &fakeBackends{found: []infra.StateBackend{{
		Bucket: "lerian-tfstate-dev-111122223333", Env: "dev", Region: "us-east-2",
		LockTable: "lerian-tfstate-lock-dev",
	}}}

	noDrain(t)
	var out bytes.Buffer
	// "no" is not "yes", which is the bar this prompt has always had.
	ask := &prompter{interactive: true, in: bufio.NewReader(strings.NewReader("no\n")), out: &out}

	err = resolveBackend(context.Background(), ask, &out, lister, layout,
		"dev", "p", "us-east-2", "111122223333")

	if !errors.Is(err, infra.ErrAborted) {
		t.Fatalf("resolveBackend = %v, want the declined confirmation", err)
	}
	if _, statErr := os.Stat(layout.BackendFile("dev")); !os.IsNotExist(statErr) {
		t.Error("the backend file was written without a confirmation")
	}
	if !strings.Contains(out.String(), "Use it as dev's state backend?") {
		t.Errorf("no confirmation was asked:\n%s", out.String())
	}
}

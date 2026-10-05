package infracli

import (
	"bytes"
	"context"
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

	ask, painted := selectorFor(t, keyEnterSeq)
	if err := resolveBackend(context.Background(), ask, ask.out, lister, layout,
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

// Every bucket is offered, not only the one whose name matches. A backend named
// by hand is still a backend somebody made deliberately, and hiding it leaves
// them where the old advice did: writing the file themselves.
func TestEveryBackendInTheAccountIsOffered(t *testing.T) {
	found := []infra.StateBackend{
		{Bucket: "lerian-tfstate-stg-123456789012", Env: "stg", Region: "us-east-1"},
		{Bucket: "lerian-tfstate-dev-123456789012", Env: "dev", Region: "us-east-2",
			LockTable: "lerian-tfstate-lock-dev"},
	}

	choices := backendOptions(found, "dev", "123456789012")

	if choices[0].value != "lerian-tfstate-dev-123456789012" {
		t.Errorf("this environment's own backend is not first: %+v", choices)
	}
	if !strings.Contains(choices[0].note, "us-east-2") {
		t.Errorf("the region is not shown, and adopting one in the wrong region fails at init: %q", choices[0].note)
	}
	values := make([]string, 0, len(choices))
	for _, choice := range choices {
		values = append(values, choice.value)
	}
	if !strings.Contains(strings.Join(values, " "), "lerian-tfstate-stg-123456789012") {
		t.Errorf("a backend made for another environment is hidden: %v", values)
	}
	if values[len(values)-1] != backendCreate {
		t.Errorf("creating a new one is not offered: %v", values)
	}
}

// A backend with no lock table is usable and unsafe for concurrent runs. That is
// the one thing worth knowing before adopting it.
func TestAMissingLockTableIsSaidOutLoud(t *testing.T) {
	choices := backendOptions([]infra.StateBackend{
		{Bucket: "lerian-tfstate-dev-123456789012", Env: "dev", Region: "us-east-1"},
	}, "dev", "123456789012")

	if !strings.Contains(choices[0].note, "no lock table") {
		t.Errorf("note = %q", choices[0].note)
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

	// Enter twice: adopt the backend, then take the target the cursor is on.
	ask, painted := selectorFor(t, keyEnterSeq+keyEnterSeq)
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

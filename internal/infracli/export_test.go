package infracli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// An export writes a whole tree and a git history. Landing it on top of
// something else mixes the two — so with nobody to ask, it still refuses.
func TestExportRefusesADirectoryWithAnythingInItWhenItCannotAsk(t *testing.T) {
	occupied := t.TempDir()
	if err := os.WriteFile(filepath.Join(occupied, "something.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := clearTheWay(context.Background(), &prompter{out: &out}, &out, occupied, nil)

	if err == nil {
		t.Fatal("an export was allowed on top of existing files")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("error = %v", err)
	}
	// And it says what is there, so the reason is not a mystery to somebody
	// reading a pipeline's log.
	if !strings.Contains(err.Error(), "1 file(s)") {
		t.Errorf("the refusal does not say what is in the way: %v", err)
	}
	// Nothing was removed on the way to refusing.
	if _, err := os.Stat(filepath.Join(occupied, "something.txt")); err != nil {
		t.Errorf("it deleted something with nobody to ask: %v", err)
	}
}

// A path that does not exist yet is the normal case.
func TestExportAcceptsAPathThatIsNotThere(t *testing.T) {
	var out bytes.Buffer
	path := filepath.Join(t.TempDir(), "new")
	if err := clearTheWay(context.Background(), &prompter{out: &out}, &out, path, nil); err != nil {
		t.Errorf("clearTheWay = %v, want it to accept a fresh path", err)
	}
}

// The repository holds what somebody set up. The twenty-seven products they
// never touched would be twenty-seven directories of someone else's decisions.
func TestOnlyRootsWithVariablesAreExported(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := infra.Discover(layout)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing configured yet.
	if units := configuredUnits(layout, catalog); len(units) != 0 {
		t.Fatalf("an unconfigured checkout exported %d roots", len(units))
	}

	// One root gets variables; only it travels.
	vpc := filepath.Join(layout.AWSDir(), "infra-base", "vpc", "envs")
	if err := os.MkdirAll(vpc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vpc, "dev.tfvars"), []byte("region = \"us-east-1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	units := configuredUnits(layout, catalog)
	if len(units) != 1 || !strings.HasSuffix(units[0].Name, "infra-base/vpc") {
		names := make([]string, 0, len(units))
		for _, unit := range units {
			names = append(names, unit.Name)
		}
		t.Errorf("exported %v, want the one root with variables", names)
	}
}

// With nothing configured there is nothing to take, and the message says what
// would make it work rather than reporting an empty copy as success.
func TestExportingAnUnconfiguredCheckoutSaysWhatIsMissing(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	t.Setenv("LERIAN_TF_REPO", checkout)
	t.Setenv("HOME", t.TempDir())

	var out bytes.Buffer
	err := ExportRepository(context.Background(), &out, filepath.Join(t.TempDir(), "repo"))

	if err == nil {
		t.Fatal("an empty export reported success")
	}
	if !strings.Contains(err.Error(), "tfvars") {
		t.Errorf("the error does not say what makes a root exportable: %v", err)
	}
}

// "Give a path that does not exist yet" is the right answer when the occupant is
// somebody's work and a pointless obstacle when it is last week's export of the
// same estate — which is the common case. So it asks.
func TestReplacingWhatIsThereIsOffered(t *testing.T) {
	t.Run("declining removes nothing and goes back", func(t *testing.T) {
		occupied := t.TempDir()
		if err := os.WriteFile(filepath.Join(occupied, "mine.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer
		ask, painted := selectorFor(t, keyEnterSeq) // "no", the first row

		err := clearTheWay(context.Background(), ask, &out, occupied, nil)

		if !errors.Is(err, errBack) {
			t.Errorf("declining returned %v, want a step back", err)
		}
		if _, err := os.Stat(filepath.Join(occupied, "mine.txt")); err != nil {
			t.Errorf("it deleted something after being told not to: %v", err)
		}
		// What is there is described before the question: nobody can decide from
		// the word "not empty".
		if !strings.Contains(out.String(), "1 file(s)") {
			t.Errorf("it did not say what is in the way:\n%s", out.String())
		}
		if !strings.Contains(painted.String(), "Replace it?") {
			t.Errorf("it did not ask:\n%s", painted.String())
		}
	})

	t.Run("work that exists nowhere else is asked about twice", func(t *testing.T) {
		noDrain(t)
		occupied := t.TempDir()
		if err := os.WriteFile(filepath.Join(occupied, "mine.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer
		// The menu, then the typed confirmation. One keypress is not the right
		// price for work no clone anywhere holds.
		ask, _ := selectorFor(t, keyDownSeq+keyEnterSeq+"yes\n")

		if err := clearTheWay(context.Background(), ask, &out, occupied, nil); err != nil {
			t.Fatal(err)
		}

		entries, err := os.ReadDir(occupied)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("%d entries left after replacing", len(entries))
		}
	})

	t.Run("the typed confirmation can be declined", func(t *testing.T) {
		noDrain(t)
		occupied := t.TempDir()
		if err := os.WriteFile(filepath.Join(occupied, "mine.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer
		ask, _ := selectorFor(t, keyDownSeq+keyEnterSeq+"no\n")

		if err := clearTheWay(context.Background(), ask, &out, occupied, nil); err == nil {
			t.Error("declining the confirmation went ahead anyway")
		}
		if _, err := os.Stat(filepath.Join(occupied, "mine.txt")); err != nil {
			t.Errorf("it deleted something after a declined confirmation: %v", err)
		}
	})
}

// Some paths are refused before anything is offered: a confirmation is consent
// to lose what was described, and in these the two are not the same thing.
//
// A templates checkout rather than the working directory, though both are
// protected by the same rule. This test answers yes to everything, and a test
// that answers yes while pointed at the source tree deletes the source tree the
// moment the safeguard regresses — which is not a hypothetical. What this one
// risks is a directory it made itself.
func TestAProtectedPathIsNotEvenOffered(t *testing.T) {
	noDrain(t)
	checkout := fakeCheckout(t, "", "")
	if err := os.WriteFile(filepath.Join(checkout, "mine.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	ask, painted := selectorFor(t, keyDownSeq+keyEnterSeq+"yes\n")

	// Named as in use, which is what makes it untouchable. A directory merely
	// shaped like a checkout is not: that was the bug — an export taken before
	// the layout changed has the same shape, and was protected as though it were
	// the templates.
	err := clearTheWay(context.Background(), ask, &out, checkout, []string{checkout})

	if !errors.Is(err, infra.ErrProtectedPath) {
		t.Fatalf("it offered to empty the checkout this run is reading: %v", err)
	}
	if strings.Contains(painted.String(), "Replace it?") {
		t.Errorf("it asked a question whose yes it would not honor:\n%s", painted.String())
	}
	if _, err := os.Stat(filepath.Join(checkout, "mine.txt")); err != nil {
		t.Errorf("it removed something anyway: %v", err)
	}
}

// The list of untouchable checkouts is the ones something depends on, assembled
// from what the CLI knows — not from what a directory looks like.
func TestWhatCountsAsACheckoutInUse(t *testing.T) {
	checkout := fakeCheckout(t, "", "")
	layout, err := infra.NewLayout(checkout)
	if err != nil {
		t.Fatal(err)
	}

	inUse := checkoutsInUse(layout)

	// The one this run resolved, above all: it is the directory being copied
	// from, and emptying it mid-export would take the source with it.
	if indexOf(inUse, layout.Root) < 0 {
		t.Errorf("the checkout this run is reading is not protected: %v", inUse)
	}
	// And the managed paths, which a later run discovers by convention.
	for _, managed := range infra.ManagedCheckoutPaths("") {
		if indexOf(inUse, managed) < 0 {
			t.Errorf("%s is not protected: %v", managed, inUse)
		}
	}
}

// The destination is emptied only after the plan succeeds. It used to be
// cleared first, so a checkout with nothing configured — or a module resolving
// outside it — left the directory the operator agreed to replace already gone,
// with nothing written in its place.
func TestNothingIsDeletedWhenThereIsNothingToExport(t *testing.T) {
	noDrain(t)
	// A checkout with no envs/<env>.tfvars anywhere: the export has nothing to do.
	t.Setenv("LERIAN_TF_REPO", fakeCheckout(t, "", ""))

	occupied := t.TempDir()
	if err := os.WriteFile(filepath.Join(occupied, "mine.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	// Answers yes to everything, so only the ordering can save the file — and
	// through the internal entry point, because the public one builds a
	// non-interactive prompter under go test and would refuse before it could
	// ever delete anything, proving nothing.
	ask, _ := selectorFor(t, keyDownSeq+keyEnterSeq+"yes\n")

	err := exportRepository(context.Background(), ask, &out, occupied)

	if err == nil {
		t.Fatal("it reported success with nothing configured")
	}
	if _, statErr := os.Stat(filepath.Join(occupied, "mine.txt")); statErr != nil {
		t.Errorf("it emptied the directory and then failed: %v", statErr)
	}
}

// And the question is not even asked, because asking about a destination for an
// export that cannot happen is a prompt with no decision behind it.
func TestTheDestinationIsNotAskedAboutWhenThereIsNothingToExport(t *testing.T) {
	noDrain(t)
	t.Setenv("LERIAN_TF_REPO", fakeCheckout(t, "", ""))

	occupied := t.TempDir()
	if err := os.WriteFile(filepath.Join(occupied, "mine.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	ask, painted := selectorFor(t, keyDownSeq+keyEnterSeq+"yes\n")

	_ = exportRepository(context.Background(), ask, &out, occupied)

	if strings.Contains(painted.String()+out.String(), "Replace it?") {
		t.Errorf("it asked about replacing a directory it was never going to write to:\n%s",
			painted.String()+out.String())
	}
}

package infracli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// The path being removed came from a config file written months ago. A mistyped
// or since-reused one would otherwise make "reset" a recursive delete of
// whatever lives there now — a home directory, a projects folder, anything.
func TestNothingButACheckoutIsRemoved(t *testing.T) {
	ordinary := t.TempDir()
	keep := filepath.Join(ordinary, "work.txt")
	if err := os.WriteFile(keep, []byte("not a checkout"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := RemoveTemplates(ordinary)

	if err == nil {
		t.Fatal("an ordinary directory was removed")
	}
	if _, statErr := os.Stat(keep); statErr != nil {
		t.Errorf("the contents went anyway: %v", statErr)
	}
}

func TestACheckoutIsRemoved(t *testing.T) {
	checkout := fakeCheckout(t, "", "")

	if err := RemoveTemplates(checkout); err != nil {
		t.Fatalf("RemoveTemplates = %v", err)
	}
	if _, err := os.Stat(checkout); !os.IsNotExist(err) {
		t.Errorf("%s is still there: %v", checkout, err)
	}
}

// The directory you are standing in is not in scope. `config reset` is run from
// wherever somebody happens to be, and a reset that deletes the repository you
// are sitting in because you were sitting in it is not a reset.
func TestTheWorkingDirectoryIsNotOfferedForDeletion(t *testing.T) {
	isolatedHome(t)
	working := fakeCheckout(t, "", "")
	t.Chdir(working)

	for _, found := range TemplatesFound(context.Background()) {
		if found.Path == working {
			t.Error("the directory the command was run from is up for deletion")
		}
	}
}

// A recorded path is in scope — it is the one the tool uses — and it is read
// before the config file goes, since afterwards nothing says where it was.
func TestTheRecordedCheckoutIsOfferedForDeletion(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())
	checkout := fakeCheckout(t, "", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = checkout
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	found := TemplatesFound(context.Background())
	if len(found) != 1 || found[0].Path != checkout {
		t.Fatalf("TemplatesFound = %+v, want the recorded %s", found, checkout)
	}
	if found[0].Managed {
		t.Error("a path somebody recorded is reported as one this tool cloned")
	}
}

// The clone can be made again from the remote; work that was never committed
// cannot. That is the part worth saying out loud before a deletion.
func TestUncommittedWorkIsCountedBeforeDeleting(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())
	checkout := fakeCheckout(t, "", "")

	tracked := filepath.Join(checkout, "main.tf")
	if err := os.WriteFile(tracked, []byte("# committed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@example.invalid", "-c", "user.name=t", "add", "main.tf"},
		{"-c", "user.email=t@example.invalid", "-c", "user.name=t", "commit", "-q", "-m", "first"},
	} {
		command := exec.Command("git", args...)
		command.Dir = checkout
		if out, err := command.CombinedOutput(); err != nil {
			t.Skipf("git is not usable here: %v\n%s", err, out)
		}
	}
	if err := os.WriteFile(tracked, []byte("# changed and not committed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = checkout
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	found := TemplatesFound(context.Background())
	if len(found) != 1 {
		t.Fatalf("TemplatesFound returned %d checkouts", len(found))
	}
	if len(found[0].Dirty) != 1 {
		t.Errorf("Dirty = %v, want the one uncommitted file", found[0].Dirty)
	}
	if got := found[0].Describe(); !strings.Contains(got, "1 file changed and not committed") {
		t.Errorf("the description does not mention the work that would be lost: %q", got)
	}
}

// A checkout with nothing pending says so, rather than leaving somebody to infer
// it from the absence of a warning.
func TestACleanCheckoutSaysNothingIsUncommitted(t *testing.T) {
	clean := TemplatesOnDisk{Path: "/somewhere", Managed: true}

	if got := clean.Describe(); !strings.Contains(got, "nothing uncommitted") {
		t.Errorf("Describe = %q", got)
	}
	if got := clean.Describe(); !strings.Contains(got, "cloned by this tool") {
		t.Errorf("Describe does not say whose directory it is: %q", got)
	}
}

// A recorded path is very often the managed one. Saying "you cloned this one"
// about a directory this tool made tells somebody deciding whether to delete it
// the opposite of the truth.
func TestTheManagedCheckoutIsNamedAsOursEvenWhenRecorded(t *testing.T) {
	isolatedHome(t)
	t.Chdir(t.TempDir())

	managed, err := infra.ManagedCheckoutPath("")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"examples/aws/_modules", "examples/aws/backend"} {
		if err := os.MkdirAll(filepath.Join(managed, filepath.FromSlash(marker)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TemplatesCheckout = managed
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	found := TemplatesFound(context.Background())
	if len(found) != 1 {
		t.Fatalf("the same directory is listed %d times", len(found))
	}
	if !found[0].Managed {
		t.Error("the managed path is reported as somebody else's clone")
	}
	if got := found[0].Describe(); !strings.Contains(got, "cloned by this tool") {
		t.Errorf("Describe = %q", got)
	}
}

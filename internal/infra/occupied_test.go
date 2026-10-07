package infra

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// "Not empty" is not something anybody can decide from. Whether that directory
// is last week's export of the same estate or six months of somebody's work is
// the entire question, and git knows the answer.
func TestWhatIsThereIsDescribedWellEnoughToDecideFrom(t *testing.T) {
	t.Run("loose files, no repository", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, "a.tf", "nested/b.tf", "nested/deep/c.tf")

		occupant, err := Inspect(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if occupant.Files != 3 {
			t.Errorf("counted %d files, want 3", occupant.Files)
		}
		if occupant.Repository {
			t.Error("it found a repository that is not there")
		}
		// Nothing anywhere else holds these — the plainest case of all.
		if !occupant.AtRisk() {
			t.Error("loose files were called recoverable")
		}
		if !strings.Contains(occupant.Describe(), "not a git repository") {
			t.Errorf("describe = %q", occupant.Describe())
		}
	})

	t.Run("a repository with work nothing else has", func(t *testing.T) {
		dir := gitRepo(t, "a.tf")

		occupant, err := Inspect(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if !occupant.Repository || occupant.Commits != 1 {
			t.Fatalf("read %+v", occupant)
		}
		// One commit, no remote: nothing else in the world holds it.
		if !occupant.Unpushed || !occupant.AtRisk() {
			t.Errorf("a repository with no remote was called safe: %+v", occupant)
		}
		if occupant.Files != 1 {
			t.Errorf("counted %d files — .git should not be in the count", occupant.Files)
		}
	})

	t.Run("uncommitted changes are noticed", func(t *testing.T) {
		dir := gitRepo(t, "a.tf")
		writeTree(t, dir, "scratch.txt")

		occupant, err := Inspect(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if !occupant.Dirty {
			t.Error("an uncommitted file went unnoticed")
		}
		if !strings.Contains(occupant.Describe(), "never committed") {
			t.Errorf("describe = %q", occupant.Describe())
		}
	})

	t.Run("an empty directory is empty", func(t *testing.T) {
		occupant, err := Inspect(context.Background(), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if !occupant.Empty() {
			t.Errorf("read %+v", occupant)
		}
	})

	t.Run("a path that is not there is empty too", func(t *testing.T) {
		occupant, err := Inspect(context.Background(), filepath.Join(t.TempDir(), "nope"))
		if err != nil {
			t.Fatalf("a missing path was an error: %v", err)
		}
		if !occupant.Empty() {
			t.Errorf("read %+v", occupant)
		}
	})
}

// A confirmation is consent to lose what was described. In these the two are not
// the same thing, so no answer authorizes it.
//
// RefuseToEmpty only — never EmptyDirectory — because these paths are real. A
// test that called the deleting function on the working directory is exactly how
// this package got deleted once: the safeguard was removed to check the test
// catches it, and the test caught it by watching its own source tree go.
func TestSomeDirectoriesAreRefusedWhateverTheAnswer(t *testing.T) {
	if home, err := os.UserHomeDir(); err == nil {
		if err := RefuseToEmpty(home); err == nil {
			t.Error("it would empty the home directory")
		}
	}
	if err := RefuseToEmpty(string(filepath.Separator)); err == nil {
		t.Error("it would empty the filesystem root")
	}
	if err := RefuseToEmpty(templatesCheckoutDir(t)); err == nil {
		t.Error("it would empty a templates checkout, which other runs read")
	}

	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := RefuseToEmpty(working); err == nil {
		t.Error("it would empty its own working directory")
	}
	if err := RefuseToEmpty(filepath.Dir(working)); err == nil {
		t.Error("it would empty a directory holding its own working directory")
	}

	// And an ordinary one is allowed.
	if err := RefuseToEmpty(t.TempDir()); err != nil {
		t.Errorf("an ordinary directory was refused: %v", err)
	}
}

// The contents, not the directory: the path may be a mount point or carry
// permissions somebody set, and recreating it is not the same as leaving it
// alone.
func TestEmptyingLeavesTheDirectoryItself(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, "a.tf", "nested/b.tf")
	if err := os.Chmod(dir, 0o751); err != nil {
		t.Fatal(err)
	}

	before, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := EmptyDirectory(dir); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("the directory itself went: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("%d entries left", len(entries))
	}
	after, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode() != after.Mode() {
		t.Errorf("the mode changed from %v to %v", before.Mode(), after.Mode())
	}
}

// The refusal holds in the function that actually deletes, not only in the one
// the caller is trusted to call first.
//
// Checked against a templates checkout rather than a real protected path: it is
// protected by the same rule, and it is a directory this test made. If the
// safeguard is ever removed, what gets deleted is a temporary directory and not
// somebody's home.
func TestEmptyingItselfRefusesAProtectedPath(t *testing.T) {
	checkout := templatesCheckoutDir(t)
	writeTree(t, checkout, "examples/aws/bootstrap/main.tf")

	if err := EmptyDirectory(checkout); err == nil {
		t.Fatal("it emptied a templates checkout")
	}
	if _, err := os.Stat(filepath.Join(checkout, "examples/aws/bootstrap/main.tf")); err != nil {
		t.Errorf("it removed something on the way to refusing: %v", err)
	}
}

// templatesCheckoutDir is a directory IsCheckout recognizes.
func templatesCheckoutDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"examples/aws/_modules", "examples/aws/backend"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeTree(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func gitRepo(t *testing.T, paths ...string) string {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, paths...)

	git, err := NewGitCLI()
	if err != nil {
		t.Skipf("no git here: %v", err)
	}
	if err := InitRepository(context.Background(), git, dir, "v1.11.0"); err != nil {
		t.Fatal(err)
	}
	return dir
}

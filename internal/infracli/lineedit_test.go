package infracli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Tab completes a directory, which is the key an operator already presses while
// typing a path.
func TestTabCompletesADirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "lerian-terraform-foundation"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lerian-notes.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	typed := filepath.Join(root, "lerian-t")
	line, pos, ok := completePath(typed, len(typed), '\t')

	if !ok {
		t.Fatal("Tab completed nothing")
	}
	want := filepath.Join(root, "lerian-terraform-foundation") + string(filepath.Separator)
	if line != want {
		t.Errorf("completed to %q, want %q", line, want)
	}
	if pos != len(line) {
		t.Errorf("cursor at %d, want the end at %d", pos, len(line))
	}
}

// A file is not an answer this question accepts, so offering one would complete
// to something that is then rejected.
func TestOnlyDirectoriesAreCompleted(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "foundation.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	typed := filepath.Join(root, "found")
	if _, _, ok := completePath(typed, len(typed), '\t'); ok {
		t.Error("a file was offered as a completion")
	}
}

// With several matches it completes as far as they agree, which is what lets
// repeated Tab walk down a path instead of stalling at the first fork.
func TestSeveralMatchesCompleteAsFarAsTheyAgree(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"lerian-cli", "lerian-charts"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	typed := filepath.Join(root, "ler")
	line, _, ok := completePath(typed, len(typed), '\t')

	if !ok {
		t.Fatal("Tab completed nothing with two matches")
	}
	if want := filepath.Join(root, "lerian-c"); line != want {
		t.Errorf("completed to %q, want the shared prefix %q", line, want)
	}
}

// Any other key is left to the terminal, and Tab in the middle of a line is not
// a completion request.
func TestCompletionKeepsOutOfTheWay(t *testing.T) {
	if _, _, ok := completePath("/tmp/x", 6, 'a'); ok {
		t.Error("an ordinary keystroke was treated as completion")
	}
	if _, _, ok := completePath("/tmp/x", 2, '\t'); ok {
		t.Error("Tab in the middle of the line was treated as completion")
	}
}

// Without a terminal there is nothing to edit on, and the caller has to fall
// back to the plain read rather than be told the answer could not be taken.
func TestEditingIsSkippedWithoutATerminal(t *testing.T) {
	_, handled, err := editableLine(&bytes.Buffer{}, "> ")

	if err != nil {
		t.Errorf("editableLine on a buffer = %v, want it to decline quietly", err)
	}
	if handled {
		t.Error("editableLine claimed a buffer as a terminal")
	}
}

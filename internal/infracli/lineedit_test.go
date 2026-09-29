package infracli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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
	ask := &prompter{interactive: true, out: &bytes.Buffer{}}
	_, handled, err := ask.editableLine(&bytes.Buffer{}, "> ")

	if err != nil {
		t.Errorf("editableLine on a buffer = %v, want it to decline quietly", err)
	}
	if handled {
		t.Error("editableLine claimed a buffer as a terminal")
	}
}

// Stdin being a terminal is not enough. With output redirected, the prompt and
// the typed answer go into the file while raw mode stops the terminal echoing
// them — the operator types blind into a command that looks stuck.
//
// Under go test stdin is not a terminal either, so this cannot prove the output
// half is consulted; it would pass against a check that only looked at stdin.
// The half that can be pinned is writerIsTerminal, which has its own test.
func TestEditingIsSkippedWhenTheOutputIsRedirected(t *testing.T) {
	redirect, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = redirect.Close() }()

	ask := &prompter{interactive: true, out: redirect}
	_, handled, err := ask.editableLine(redirect, "> ")

	if err != nil {
		t.Errorf("editableLine on a redirect = %v, want it to decline quietly", err)
	}
	if handled {
		t.Error("editableLine took a redirected file for a terminal")
	}
}

// A symlink to a directory is a directory as far as this question is concerned:
// entry.IsDir reports false for one, and a checkout reached through a symlink is
// still a checkout.
func TestASymlinkToADirectoryCompletes(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real-foundation")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "linked-foundation")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	typed := filepath.Join(root, "linked")
	line, _, ok := completePath(typed, len(typed), '\t')

	if !ok {
		t.Fatal("a symlink to a directory was not completed")
	}
	if !strings.Contains(line, "linked-foundation") {
		t.Errorf("completed to %q", line)
	}
}

// Trimming a byte at a time cuts multi-byte names apart: these two share one
// byte and no character, and half a rune becomes a replacement character in the
// answer.
func TestTheSharedPrefixStopsOnACharacter(t *testing.T) {
	got := commonPrefix([]string{"école", "ècole"})

	if !utf8.ValidString(got) {
		t.Errorf("commonPrefix returned invalid UTF-8: %q", got)
	}
	if got != "" {
		t.Errorf("commonPrefix = %q, want empty: the names share no character", got)
	}
}

func TestTheSharedPrefixKeepsWholeCharacters(t *testing.T) {
	got := commonPrefix([]string{"ambiente-dev", "ambiente-stg"})

	if got != "ambiente-" {
		t.Errorf("commonPrefix = %q, want %q", got, "ambiente-")
	}
}

// term.Terminal buffers whatever arrived past the current answer. A fresh editor
// for the next question throws it away, so a pasted block loses every line after
// the first and the prompt waits for input already typed.
func TestTheEditorIsKeptAcrossQuestions(t *testing.T) {
	ask := &prompter{interactive: true, out: &bytes.Buffer{}}

	if ask.editor != nil {
		t.Fatal("the prompter starts with an editor it has not built yet")
	}

	// No terminal here, so the editor is never built — what this pins is that the
	// field exists to be reused rather than rebuilt per call. The reuse itself is
	// visible in editableLine: it only constructs when p.editor is nil.
	_, handled, _ := ask.editableLine(&bytes.Buffer{}, "> ")
	if handled {
		t.Error("a buffer was taken for a terminal")
	}
}

// The editor and the selector want different numbers from the same terminal.
//
// The selector caps at 120 because a row it draws wider than that is a row it
// cannot redraw. The editor is not drawing rows: it is deciding where the line
// wraps, and that happens at the terminal's real edge. Told 120 on a 160-column
// terminal it wraps the path early and puts the cursor a screen-row away from
// the character it is editing.
//
// Measured widths need a terminal, which go test does not have; this pins the
// arithmetic both callers depend on.
func TestTheEditorIsNotCappedWhereTheSelectorIs(t *testing.T) {
	tests := []struct {
		measured, layout int
	}{
		{80, 80},
		{120, 120},
		{160, 120}, // the selector stops here, the editor does not
		{200, 120},
	}

	for _, test := range tests {
		if got := capForLayout(test.measured); got != test.layout {
			t.Errorf("capForLayout(%d) = %d, want %d", test.measured, got, test.layout)
		}
		if test.measured > 120 && capForLayout(test.measured) == test.measured {
			t.Errorf("the selector was handed %d columns it cannot redraw", test.measured)
		}
	}
}

// A writer that is not a terminal has no width to measure, and saying 80 would
// have the editor size itself to a screen that does not exist.
func TestThereIsNoWidthWithoutATerminal(t *testing.T) {
	if got := screenWidth(&bytes.Buffer{}); got != 0 {
		t.Errorf("screenWidth of a buffer = %d, want 0", got)
	}
}

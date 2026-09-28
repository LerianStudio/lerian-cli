package infracli

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/term"
)

// editableLine reads one line with the editing a shell gives: left and right to
// move, home and end, backspace anywhere in the line, and Tab to complete a path.
//
// The plain prompt reads through bufio in cooked mode, where the terminal itself
// handles backspace and nothing else — an arrow key arrives as an escape
// sequence and lands in the answer as "^[[D". That is fine for a short word and
// wrong for a filesystem path, which is long, easy to mistype near the middle,
// and the one answer an operator wants to correct rather than retype.
//
// It returns handled=false when there is no terminal to edit on, so the caller
// falls back to the plain read rather than this reporting a failure the operator
// cannot act on.
func (p *prompter) editableLine(out io.Writer, prompt string) (answer string, handled bool, err error) {
	// Both ends, not just stdin. With output redirected the prompt and the typed
	// answer go into the file while raw mode stops the terminal echoing them, so
	// the operator types blind into a command that looks stuck. writerIsTerminal
	// is the same check CanAsk makes, for the same reason.
	if !isTerminal(os.Stdin) || !writerIsTerminal(out) {
		return "", false, nil
	}

	restore, err := rawMode()
	if err != nil {
		// The terminal refused raw mode. Answering is still possible the plain way,
		// so this is a reason to fall back rather than to stop.
		return "", false, nil
	}
	defer restore()

	// The editor is kept on the prompter rather than built per question. When two
	// answers arrive in one read — a pasted block — term.Terminal holds the bytes
	// after the first newline in its own buffer, and a fresh editor for the next
	// question throws them away: the answer is gone and the prompt waits for
	// input that was already typed.
	terminal := p.editor
	if terminal == nil {
		terminal = term.NewTerminal(readWriter{in: os.Stdin, out: out}, prompt)
		terminal.AutoCompleteCallback = completePath
		p.editor = terminal
	}
	terminal.SetPrompt(prompt)
	// term.NewTerminal assumes 80 columns and does its cursor arithmetic with
	// that number, so on any other width a long path wraps where the editor does
	// not expect it and the cursor lands in the wrong place.
	if width := terminalWidth(out); width > 0 {
		_ = terminal.SetSize(width, 24)
	}

	line, err := terminal.ReadLine()
	if err != nil {
		return "", true, err
	}
	return strings.TrimSpace(line), true, nil
}

// readWriter joins the terminal's two halves. term.Terminal wants one value it
// can both read from and write to, and stdin and the prompt's destination are
// separate here — the prompt goes wherever the caller is writing, which is not
// always stdout.
type readWriter struct {
	in  io.Reader
	out io.Writer
}

func (rw readWriter) Read(p []byte) (int, error)  { return rw.in.Read(p) }
func (rw readWriter) Write(p []byte) (int, error) { return rw.out.Write(p) }

// completePath completes a directory on Tab, which is the key an operator
// already presses when typing a path.
//
// Only directories are offered: every answer this asks for is one, and a listing
// full of files is noise in front of the one entry that can be chosen.
func completePath(line string, pos int, key rune) (string, int, bool) {
	if key != '\t' || pos != len(line) {
		return "", 0, false
	}

	dir, prefix := filepath.Split(line)
	search := dir
	if search == "" {
		search = "."
	}

	entries, err := os.ReadDir(expandHome(search))
	if err != nil {
		return "", 0, false
	}

	var matches []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		// Not entry.IsDir(): that is false for a symlink pointing at a directory,
		// and a checkout reached through one is still a checkout.
		if !isDirectory(filepath.Join(expandHome(search), entry.Name())) {
			continue
		}
		matches = append(matches, entry.Name())
	}
	if len(matches) == 0 {
		return "", 0, false
	}
	sort.Strings(matches)

	// One match completes; several complete as far as they agree, which is what
	// lets repeated Tab walk down a path rather than stall on the first fork.
	completion := matches[0]
	if len(matches) > 1 {
		completion = commonPrefix(matches)
		if completion == prefix {
			return "", 0, false
		}
		newLine := dir + completion
		return newLine, len(newLine), true
	}

	newLine := dir + completion + string(filepath.Separator)
	return newLine, len(newLine), true
}

// expandHome resolves a leading ~ so completion works on the path shape people
// actually type.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// commonPrefix compares runes rather than bytes. Trimming a byte at a time cuts
// multi-byte names apart — "école" and "ècole" share one byte and no character,
// and the half rune that survives becomes a replacement character in the answer.
func commonPrefix(values []string) string {
	prefix := []rune(values[0])
	for _, value := range values[1:] {
		other := []rune(value)
		if len(other) < len(prefix) {
			prefix = prefix[:len(other)]
		}
		for i := range prefix {
			if prefix[i] != other[i] {
				prefix = prefix[:i]
				break
			}
		}
		if len(prefix) == 0 {
			return ""
		}
	}
	return string(prefix)
}

// isDirectory follows symlinks, which entry.IsDir does not.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

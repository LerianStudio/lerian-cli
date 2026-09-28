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
func editableLine(out io.Writer, prompt string) (answer string, handled bool, err error) {
	if _, ok := out.(*os.File); !ok {
		return "", false, nil
	}
	if !isTerminal(os.Stdin) {
		return "", false, nil
	}

	restore, err := rawMode()
	if err != nil {
		// The terminal refused raw mode. Answering is still possible the plain way,
		// so this is a reason to fall back rather than to stop.
		return "", false, nil
	}
	defer restore()

	terminal := term.NewTerminal(readWriter{in: os.Stdin, out: out}, prompt)
	terminal.AutoCompleteCallback = completePath

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
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
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

func commonPrefix(values []string) string {
	prefix := values[0]
	for _, value := range values[1:] {
		for !strings.HasPrefix(value, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}

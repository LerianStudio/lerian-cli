package infracli

// The selector renders a question whose answers are already known, instead of
// asking the operator to type one of them back.
//
// It adds no decision: every list here corresponds to a flag, and the rule from
// prompt.go still holds — without a terminal the question is refused by naming
// that flag. What changes is only how a known set is presented to someone who is
// standing in front of it.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// option is one row. note carries what makes the row decidable — for an AWS
// profile that is the account it reaches, which is the actual question.
type option struct {
	value    string
	label    string
	note     string
	disabled bool
}

// rawMode switches the terminal to raw and returns the restore func. It is a
// field rather than a direct call so a test can replace it: the key sequences are
// the thing under test, not the ioctl.
var rawMode = func() (func(), error) {
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() { _ = term.Restore(fd, state) }, nil
}

// plainSelection reports whether the operator asked for the typed prompt instead.
//
// It exists for the same reason LERIAN_SPINNER=ascii does: a terminal or a font
// that does not render what the UI assumes should not need a rebuild to be usable.
func plainSelection() bool { return os.Getenv("LERIAN_SELECT") == "plain" }

// pick asks for one value from a known set.
func (p *prompter) pick(question, purpose, flagName string, options []option, preset string) (string, error) {
	values, err := p.selectFrom(question, purpose, flagName, options, []string{preset}, false)
	if err != nil {
		return "", err
	}
	if len(values) == 0 {
		return "", fmt.Errorf("%s is required", question)
	}
	return values[0], nil
}

// pickMany asks for any number of values from a known set.
func (p *prompter) pickMany(question, purpose, flagName string, options []option, preset []string) ([]string, error) {
	return p.selectFrom(question, purpose, flagName, options, preset, true)
}

// selectFrom is the whole mechanism. The three fallbacks are deliberate and
// ordered: no terminal is a refusal, not a guess; LERIAN_SELECT=plain is the
// operator's choice; a terminal that cannot go raw is not the operator's fault.
func (p *prompter) selectFrom(
	question, purpose, flagName string,
	options []option,
	preset []string,
	multiple bool,
) ([]string, error) {
	if len(options) == 0 {
		return nil, fmt.Errorf("%s has no options to choose from", question)
	}

	fallback := strings.Join(preset, ",")
	if !p.interactive {
		if fallback != "" {
			return preset, nil
		}
		return nil, fmt.Errorf("%s is required and there is no terminal to ask\n"+
			"Pass %s explicitly.", question, flagName)
	}
	if plainSelection() {
		return p.askForValues(question, purpose, flagName, fallback, multiple)
	}

	restore, err := rawMode()
	if err != nil {
		fmt.Fprintf(p.out, "  %s\n",
			newStyle(p.out).dim("this terminal cannot be switched to raw mode; type the answer instead"))
		return p.askForValues(question, purpose, flagName, fallback, multiple)
	}
	defer restore()

	return p.runSelector(question, purpose, options, preset, multiple)
}

// askForValues is the typed prompt, reused verbatim so the two paths cannot drift.
func (p *prompter) askForValues(question, purpose, flagName, fallback string, multiple bool) ([]string, error) {
	answer, err := p.ask(question, purpose, fallback, flagName)
	if err != nil {
		return nil, err
	}
	if !multiple {
		return []string{answer}, nil
	}
	return splitList(answer), nil
}

// splitList reads the comma-joined form the flags have always taken.
func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (p *prompter) runSelector(
	question, purpose string,
	options []option,
	preset []string,
	multiple bool,
) ([]string, error) {
	chosen := map[string]bool{}
	for _, value := range preset {
		chosen[value] = true
	}

	cursor := 0
	for i, opt := range options {
		if !opt.disabled && chosen[opt.value] {
			cursor = i
			break
		}
	}
	if options[cursor].disabled {
		cursor = firstEnabled(options)
	}

	painted := 0
	for {
		painted = p.paintOptions(question, purpose, options, chosen, cursor, multiple, painted)

		key, err := readKey(p.in)
		if err != nil {
			return nil, fmt.Errorf("cannot read the answer to %q: %w", question, err)
		}

		switch key.kind {
		case keyUp:
			cursor = step(options, cursor, -1)
		case keyDown:
			cursor = step(options, cursor, +1)
		case keyAbort:
			fmt.Fprint(p.out, "\r\n")
			return nil, infra.ErrAborted
		case keySpace:
			if multiple && !options[cursor].disabled {
				value := options[cursor].value
				chosen[value] = !chosen[value]
			}
		case keyEnter:
			if !multiple {
				if options[cursor].disabled {
					continue
				}
				fmt.Fprint(p.out, "\r\n")
				return []string{options[cursor].value}, nil
			}
			selected := selectedValues(options, chosen)
			if len(selected) == 0 {
				// Enter on an empty multi-selection takes the row under the cursor,
				// so the obvious gesture works without having to press space first.
				if options[cursor].disabled {
					continue
				}
				selected = []string{options[cursor].value}
			}
			fmt.Fprint(p.out, "\r\n")
			return selected, nil
		case keyRune:
			// Letters and digits jump to the first row that starts with them. This is
			// the one place typing still helps: a list of twenty discovered targets is
			// faster to reach by name than by arrow.
			if next := jumpTo(options, key.rune, cursor); next >= 0 {
				cursor = next
			}
		}
	}
}

// paintOptions redraws the list in place and returns how many lines it wrote, so
// the next frame knows how far to go back up.
//
// It moves the cursor up and overwrites with spaces rather than using \x1b[K, for
// the same reason spinner.erase does: not every terminal honors the erase
// sequence, and \r plus blanks works everywhere.
func (p *prompter) paintOptions(
	question, purpose string,
	options []option,
	chosen map[string]bool,
	cursor int,
	multiple bool,
	previous int,
) int {
	theme := newStyle(p.out)
	if previous > 0 {
		fmt.Fprintf(p.out, "\x1b[%dA", previous)
	}

	lines := 0
	write := func(format string, args ...any) {
		fmt.Fprintf(p.out, "\r%-100s\r", "")
		fmt.Fprintf(p.out, format+"\r\n", args...)
		lines++
	}

	if previous == 0 {
		fmt.Fprint(p.out, "\r\n")
	}
	write("  %s", theme.bold(question))
	if purpose != "" {
		write("  %s", theme.dim(purpose))
	}
	hint := "↑↓ move · enter choose · q cancel"
	if multiple {
		hint = "↑↓ move · space toggle · enter confirm · q cancel"
	}
	write("  %s", theme.dim(hint))

	for i, opt := range options {
		pointer := "  "
		if i == cursor {
			pointer = theme.bold("❯ ")
		}
		mark := ""
		if multiple {
			box := " "
			if chosen[opt.value] {
				box = "x"
			}
			mark = "[" + box + "] "
		}

		label := opt.label
		if opt.disabled {
			label = theme.dim(label + "  (unavailable)")
		} else if i == cursor {
			label = theme.bold(label)
		}
		if opt.note != "" {
			label += "  " + theme.dim(opt.note)
		}
		write("  %s%s%s", pointer, mark, label)
	}
	return lines
}

func selectedValues(options []option, chosen map[string]bool) []string {
	var out []string
	for _, opt := range options {
		if !opt.disabled && chosen[opt.value] {
			out = append(out, opt.value)
		}
	}
	return out
}

// step moves the cursor, skipping rows that cannot be chosen and wrapping around.
func step(options []option, from, delta int) int {
	next := from
	for i := 0; i < len(options); i++ {
		next = (next + delta + len(options)) % len(options)
		if !options[next].disabled {
			return next
		}
	}
	return from
}

func firstEnabled(options []option) int {
	for i, opt := range options {
		if !opt.disabled {
			return i
		}
	}
	return 0
}

// jumpTo finds the next row whose label starts with r, searching after the cursor
// first so repeated presses cycle through the matches.
func jumpTo(options []option, r rune, cursor int) int {
	want := strings.ToLower(string(r))
	for i := 1; i <= len(options); i++ {
		at := (cursor + i) % len(options)
		if options[at].disabled {
			continue
		}
		if strings.HasPrefix(strings.ToLower(options[at].label), want) {
			return at
		}
	}
	return -1
}

type keyKind int

const (
	keyUp keyKind = iota
	keyDown
	keyEnter
	keySpace
	keyAbort
	keyRune
	keyOther
)

type keyPress struct {
	kind keyKind
	rune rune
}

// readKey decodes one keypress. In raw mode an arrow arrives as three bytes,
// ESC '[' and a letter, so the escape has to be read ahead before it can be
// called an abort.
//
// That read-ahead is why Escape alone is a poor abort key in a real terminal: a
// lone ESC is indistinguishable from the start of an arrow until the next byte
// arrives, so it only registers once another key is pressed. Telling the two
// apart needs a timeout, which is not worth the machinery here. Ctrl-C and q
// abort immediately and are what the hint line offers; ESC still works from a
// pipe, where the end of input resolves the ambiguity.
func readKey(in io.RuneScanner) (keyPress, error) {
	r, _, err := in.ReadRune()
	if err != nil {
		return keyPress{}, err
	}

	switch r {
	case '\r', '\n':
		return keyPress{kind: keyEnter}, nil
	case ' ':
		return keyPress{kind: keySpace}, nil
	case 3, 4: // Ctrl-C, Ctrl-D
		return keyPress{kind: keyAbort}, nil
	case 'q':
		return keyPress{kind: keyAbort}, nil
	case 'k':
		return keyPress{kind: keyUp}, nil
	case 'j':
		return keyPress{kind: keyDown}, nil
	case 27: // ESC, possibly the start of an arrow
		next, _, err := in.ReadRune()
		switch {
		case errors.Is(err, io.EOF):
			// ESC with nothing after it: Escape was pressed.
			return keyPress{kind: keyAbort}, nil
		case err != nil:
			return keyPress{}, err
		case next != '[':
			_ = in.UnreadRune()
			return keyPress{kind: keyAbort}, nil
		}
		final, _, err := in.ReadRune()
		if errors.Is(err, io.EOF) {
			return keyPress{kind: keyAbort}, nil
		}
		if err != nil {
			return keyPress{}, err
		}
		switch final {
		case 'A':
			return keyPress{kind: keyUp}, nil
		case 'B':
			return keyPress{kind: keyDown}, nil
		}
		return keyPress{kind: keyOther}, nil
	}

	if r > 32 && r < 127 {
		return keyPress{kind: keyRune, rune: r}, nil
	}
	return keyPress{kind: keyOther}, nil
}

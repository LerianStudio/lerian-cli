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
	"golang.org/x/text/width"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// option is one row. note carries what makes the row decidable — for an AWS
// profile that is the account it reaches, which is the actual question.
type option struct {
	value    string
	label    string
	note     string
	disabled bool
	// fixed marks a row that is part of the answer whatever else is chosen. It
	// paints as ticked and cannot be focused or toggled, which is the difference
	// from disabled: disabled means "not available to you", fixed means "already
	// decided, and here is what was decided".
	fixed bool
	// environment is the name this row deploys as, when it has one. It is not
	// shown unless two rows would otherwise read identically — see
	// nameTheAmbiguous.
	environment string
}

// selectable reports whether the cursor may land on this row. Both states are
// unreachable, for opposite reasons, and every piece of navigation has to skip
// both or the cursor lands on a row that cannot answer anything.
func (o option) selectable() bool { return !o.disabled && !o.fixed }

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
// errBack is a question answered with "take me back", which is a different
// answer from "stop". A caller that treats them alike turns r into q, which is
// the one thing somebody pressing r is trying to avoid.
var errBack = errors.New("infracli: back to the previous question")

// ErrBack is errBack for callers outside this package — the root menu, which has
// to know the difference between "back" and "leave" even though it has nowhere to
// go back to.
var ErrBack = errBack

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
		p.printOptions(options)
		return p.askForValues(question, purpose, flagName, fallback, multiple)
	}

	restore, err := rawMode()
	if err != nil {
		fmt.Fprintf(p.out, "\n  %s\n",
			newStyle(p.out).dim("this terminal cannot be switched to raw mode; type the answer instead"))
		p.printOptions(options)
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

// printOptions lists what can be answered, for the paths where the selector does
// not run.
//
// Without it the typed fallback asked the question with the options invisible.
// That is merely unhelpful for a closed set like dev/stg/prd, which the prompt
// names anyway — but for the AWS profiles it was a hazard: the question says to
// pick by account, the note carrying each account is what the selector would have
// shown, and pressing Enter on an unseen default can reach the wrong account.
func (p *prompter) printOptions(options []option) {
	theme := newStyle(p.out)
	for _, opt := range options {
		label := opt.label
		if opt.disabled {
			label = theme.dim(label + "  (unavailable)")
		}
		if opt.note != "" {
			label += "  " + theme.dim(opt.note)
		}
		fmt.Fprintf(p.out, "    %s\n", label)
	}
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
		if opt.selectable() && chosen[opt.value] {
			cursor = i
			break
		}
	}
	if !options[cursor].selectable() {
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
		case keyBack:
			fmt.Fprint(p.out, "\r\n")
			return nil, errBack
		case keySpace:
			if multiple && options[cursor].selectable() {
				value := options[cursor].value
				chosen[value] = !chosen[value]
			}
		case keyEnter:
			if !multiple {
				if !options[cursor].selectable() {
					continue
				}
				fmt.Fprint(p.out, "\r\n")
				return []string{options[cursor].value}, nil
			}
			selected := selectedValues(options, chosen)
			if len(selected) == 0 {
				// Enter on an empty multi-selection takes the row under the cursor,
				// so the obvious gesture works without having to press space first.
				if !options[cursor].selectable() {
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

	// Every line is blanked and written within the terminal's width, and nothing
	// is allowed to be wider. A line that wraps costs two display lines while this
	// counts one, and the next frame then moves the cursor up by too little and
	// paints over the wrong rows. The redraw arithmetic is only valid while no row
	// wraps, so the width is a correctness constraint, not cosmetics.
	width := terminalWidth(p.out)
	lines := 0
	write := func(text string) {
		fmt.Fprintf(p.out, "\r%s\r", strings.Repeat(" ", width))
		fmt.Fprintf(p.out, "%s\r\n", text)
		lines++
	}

	if previous == 0 {
		fmt.Fprint(p.out, "\r\n")
	}
	write("  " + theme.bold(fit(question, width-2)))
	if purpose != "" {
		write("  " + theme.dim(fit(purpose, width-2)))
	}
	hint := "↑↓ move · enter choose · r back · q cancel"
	if multiple {
		hint = "↑↓ move · space toggle · enter confirm · r back · q cancel"
	}
	write("  " + theme.dim(fit(hint, width-2)))

	for i, opt := range options {
		pointer, mark := "  ", ""
		if multiple {
			box := " "
			if chosen[opt.value] || opt.fixed {
				box = "x"
			}
			mark = "[" + box + "] "
		}

		// The plain text is assembled and measured BEFORE any styling: an escape
		// sequence has no width on screen but plenty of bytes, so truncating a
		// styled string would both measure wrong and risk cutting an escape in half.
		label := opt.label
		if opt.disabled {
			// Only disabled. A fixed row is the opposite of unavailable — it is in
			// the answer already — and its note is what says so.
			label += "  (unavailable)"
		}

		// The label is fitted like every other row: the note was already clipped
		// to what was left, but a label longer than the row wrapped it, and the
		// comment above is explicit that a wrapped row breaks the redraw
		// arithmetic. A long profile name is enough to do it.
		prefix := 2 + displayWidth(pointer) + displayWidth(mark)
		label = fit(label, width-prefix)
		room := width - prefix - displayWidth(label)
		note := ""
		if opt.note != "" && room > 4 {
			note = "  " + fit(opt.note, room-2)
		}

		if i == cursor {
			pointer = theme.bold("❯ ")
		}
		switch {
		case opt.disabled, opt.fixed:
			label = theme.dim(label)
		case i == cursor:
			label = theme.bold(label)
		}
		if note != "" {
			note = theme.dim(note)
		}
		write("  " + pointer + mark + label + note)
	}
	return lines
}

// fit truncates to at most width runes, marking the cut so a clipped value is not
// mistaken for a short one.
func fit(text string, width int) string {
	if width < 1 {
		return ""
	}
	if displayWidth(text) <= width {
		return text
	}

	// Budget for the ellipsis, which is itself one column, then take runes while
	// they fit. A wide rune that would straddle the limit is left out rather than
	// half-drawn.
	budget := width - 1
	var kept []rune
	used := 0
	for _, r := range text {
		w := runeWidth(r)
		if used+w > budget {
			break
		}
		kept = append(kept, r)
		used += w
	}
	return string(kept) + "…"
}

// displayWidth is how many terminal columns text occupies, which is not its rune
// count: East Asian wide and fullwidth characters take two. Measuring by runes
// let a label of wide characters pass the fit check and still wrap its row,
// which breaks the redraw arithmetic the same way an unfitted label did.
//
// Scoped to East Asian width on purpose. Grapheme clusters — a base rune plus
// combining marks, or an emoji joined with ZWJ — need a segmentation library to
// measure, and nothing this selector lists is built from them: they are AWS
// profile names, environment names and Terraform stack names.
func displayWidth(text string) int {
	total := 0
	for _, r := range text {
		total += runeWidth(r)
	}
	return total
}

func runeWidth(r rune) int {
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	default:
		return 1
	}
}

// screenWidth is what the terminal says it is, uncapped, and 0 when there is no
// terminal to ask. Sizing anything to a screen that does not exist is worse than
// not sizing it.
func screenWidth(out io.Writer) int {
	file, ok := out.(*os.File)
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || width < 1 {
		return 0
	}
	return width
}

// terminalWidth is the width the selector draws in: measured, or the 80 columns
// every terminal has had since the punched card, capped for layout.
//
// A measured width is reported as measured, however small. Substituting 80 for a
// narrow terminal tells the caller it has columns it does not have, and then
// every row it draws wraps — which is the one thing the redraw cannot survive.
// Too narrow to be usable is a different problem from too narrow to be correct,
// and narrowTerminal decides that one.
func terminalWidth(out io.Writer) int {
	width := screenWidth(out)
	if width == 0 {
		return 80
	}
	return capForLayout(width)
}

// capForLayout is the selector's ceiling, and only the selector's. A row wider
// than this is one it cannot redraw, so it stops drawing wider. The line editor
// has no rows to redraw — it needs the edge where the terminal actually wraps,
// which is screenWidth.
func capForLayout(width int) int {
	if width > 120 {
		return 120
	}
	return width
}

func selectedValues(options []option, chosen map[string]bool) []string {
	var out []string
	for _, opt := range options {
		// A fixed row is excluded on purpose. It is not something the operator
		// picked, and the caller that drew it is the one that already handles it —
		// returning it would have that caller add it twice.
		if opt.selectable() && chosen[opt.value] {
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
		if options[next].selectable() {
			return next
		}
	}
	return from
}

func firstEnabled(options []option) int {
	for i, opt := range options {
		if opt.selectable() {
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
		if !options[at].selectable() {
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
	// keyBack is one question back, not out: r, next to q on the keys line.
	keyBack
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
	case 'r', 'R':
		return keyPress{kind: keyBack}, nil
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
		// '[' is CSI and 'O' is SS3. A terminal in application cursor-key mode
		// (DECCKM) sends arrows as ESC O A, and term.MakeRaw does not reset DECCKM
		// — tmux or a full-screen program that exited badly can leave it set.
		// Without 'O' here the operator's FIRST arrow press would cancel the
		// command, which is the worst possible way to meet a new prompt.
		case next != '[' && next != 'O':
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

// minSelectorWidth is the narrowest terminal the list is still worth drawing in.
// Below it every label is clipped to an ellipsis and the hint says nothing, so
// the typed prompt — which wraps harmlessly — is the better answer.
const minSelectorWidth = 40

// narrowTerminal reports whether out is too narrow for the selector to be worth
// drawing.
func narrowTerminal(out io.Writer) bool { return terminalWidth(out) < minSelectorWidth }

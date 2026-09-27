package infracli

import (
	"bufio"
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// selectorFor builds a prompter fed by a canned key sequence, with the raw-mode
// switch replaced. The key sequences are what is under test, not the ioctl.
func selectorFor(t *testing.T, keys string) (*prompter, *bytes.Buffer) {
	t.Helper()

	previous := rawMode
	rawMode = func() (func(), error) { return func() {}, nil }
	t.Cleanup(func() { rawMode = previous })

	out := &bytes.Buffer{}
	return &prompter{interactive: true, in: bufio.NewReader(strings.NewReader(keys)), out: out}, out
}

var envOptions = []option{
	{value: "dev", label: "dev", note: "day to day"},
	{value: "stg", label: "stg"},
	{value: "prd", label: "prd", note: "production"},
}

const (
	keyDownSeq  = "\x1b[B"
	keyUpSeq    = "\x1b[A"
	keyEnterSeq = "\r"
)

func TestPickMovesWithTheArrowKeys(t *testing.T) {
	tests := []struct {
		name string
		keys string
		want string
	}{
		{"enter takes the preset", keyEnterSeq, "dev"},
		{"one step down", keyDownSeq + keyEnterSeq, "stg"},
		{"two steps down", keyDownSeq + keyDownSeq + keyEnterSeq, "prd"},
		{"down then back up", keyDownSeq + keyUpSeq + keyEnterSeq, "dev"},
		{"up from the first wraps to the last", keyUpSeq + keyEnterSeq, "prd"},
		{"j and k move too", "jj" + keyEnterSeq, "prd"},
		{"a letter jumps to the row", "p" + keyEnterSeq, "prd"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ask, _ := selectorFor(t, test.keys)
			got, err := ask.pick("Which environment?", "", "--env", envOptions, "dev")
			if err != nil {
				t.Fatalf("pick: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

// Ctrl-C and Esc have to abort rather than choose whatever the cursor happens to
// be on. In raw mode the terminal does not turn Ctrl-C into a signal, so the
// selector is the only thing that can honor it.
func TestPickAborts(t *testing.T) {
	for _, keys := range []string{"\x03", "\x1b", "\x04"} {
		ask, _ := selectorFor(t, keys)
		_, err := ask.pick("Which environment?", "", "--env", envOptions, "dev")
		if !errors.Is(err, infra.ErrAborted) {
			t.Errorf("keys %q: got %v, want ErrAborted", keys, err)
		}
	}
}

// A row that cannot be chosen — an AWS profile whose session expired — is shown
// so the operator understands why the account is missing, but the cursor must
// never land on it.
func TestPickSkipsDisabledRows(t *testing.T) {
	options := []option{
		{value: "expired", label: "expired", disabled: true},
		{value: "good", label: "good"},
		{value: "also-expired", label: "also-expired", disabled: true},
		{value: "other", label: "other"},
	}

	ask, _ := selectorFor(t, keyEnterSeq)
	got, err := ask.pick("Which profile?", "", "--profile", options, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "good" {
		t.Errorf("the cursor started on a disabled row: got %q", got)
	}

	ask, _ = selectorFor(t, keyDownSeq+keyEnterSeq)
	got, err = ask.pick("Which profile?", "", "--profile", options, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "other" {
		t.Errorf("moving down landed on a disabled row: got %q", got)
	}
}

func TestPickManyTogglesWithSpace(t *testing.T) {
	options := []option{
		{value: "infra-base", label: "infra-base"},
		{value: "midaz", label: "midaz"},
		{value: "reporter", label: "reporter"},
	}

	tests := []struct {
		name string
		keys string
		want []string
	}{
		{"space on the first, then confirm", " " + keyEnterSeq, []string{"infra-base"}},
		{"two rows", " " + keyDownSeq + " " + keyEnterSeq, []string{"infra-base", "midaz"}},
		{"space twice deselects", " " + " " + keyDownSeq + " " + keyEnterSeq, []string{"midaz"}},
		// Enter with nothing selected takes the row under the cursor, so the obvious
		// gesture works without having to press space first.
		{"enter with no selection takes the cursor row", keyDownSeq + keyEnterSeq, []string{"midaz"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ask, _ := selectorFor(t, test.keys)
			got, err := ask.pickMany("What do you want?", "", "--targets", options, nil)
			if err != nil {
				t.Fatalf("pickMany: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Errorf("got %v, want %v", got, test.want)
			}
		})
	}
}

// The rule from prompt.go has to survive: without a terminal the question is
// refused by naming its flag, never guessed. This is what keeps CI honest.
func TestSelectorWithoutATerminalRefusesAndNamesTheFlag(t *testing.T) {
	ask := &prompter{interactive: false, out: &bytes.Buffer{}}

	_, err := ask.pick("Which environment?", "", "--env", envOptions, "")
	if err == nil {
		t.Fatal("a question was answered with no terminal to ask")
	}
	if !strings.Contains(err.Error(), "--env") {
		t.Errorf("the refusal does not name the flag: %v", err)
	}

	// With a preset there is nothing to ask: the flag default stands.
	got, err := ask.pick("Which environment?", "", "--env", envOptions, "dev")
	if err != nil || got != "dev" {
		t.Errorf("a preset should be taken without asking: got %q, %v", got, err)
	}
}

// LERIAN_SELECT=plain falls back to the typed prompt, for the same reason
// LERIAN_SPINNER=ascii exists: a terminal that does not render what the UI
// assumes should not need a rebuild to be usable.
func TestPlainSelectionUsesTheTypedPrompt(t *testing.T) {
	t.Setenv("LERIAN_SELECT", "plain")

	// No raw mode is entered, so the input is read as a typed line.
	out := &bytes.Buffer{}
	ask := &prompter{interactive: true, in: bufio.NewReader(strings.NewReader("prd\n")), out: out}

	got, err := ask.pick("Which environment?", "", "--env", envOptions, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if got != "prd" {
		t.Errorf("got %q, want the typed answer", got)
	}
	if strings.Contains(out.String(), "↑↓") {
		t.Error("the selector was painted even though plain selection was asked for")
	}
}

// A terminal that cannot go raw is not the operator's fault: fall back rather
// than fail.
func TestSelectorFallsBackWhenRawModeFails(t *testing.T) {
	previous := rawMode
	rawMode = func() (func(), error) { return nil, errors.New("inappropriate ioctl") }
	t.Cleanup(func() { rawMode = previous })

	out := &bytes.Buffer{}
	ask := &prompter{interactive: true, in: bufio.NewReader(strings.NewReader("stg\n")), out: out}

	got, err := ask.pick("Which environment?", "", "--env", envOptions, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if got != "stg" {
		t.Errorf("got %q, want the typed answer", got)
	}
}

// NO_COLOR is honored by the shared style, and the selector must not paint any
// escape of its own around it.
func TestSelectorEmitsNoColorWhenNoColorIsSet(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	ask, out := selectorFor(t, keyEnterSeq)
	if _, err := ask.pick("Which environment?", "", "--env", envOptions, "dev"); err != nil {
		t.Fatal(err)
	}
	// \x1b[NA is the cursor move the redraw needs; color sequences are \x1b[NNm.
	if strings.Contains(out.String(), "m") && strings.Contains(out.String(), "\x1b[1m") {
		t.Errorf("a color escape survived NO_COLOR:\n%q", out.String())
	}
}

// The guided run is reached only where the command would otherwise fail, so it
// must reproduce the old defaults when the operator just presses Enter.
func TestGuidedRunWithThreeEntersReproducesTheDefaultRun(t *testing.T) {
	catalog := infra.Catalog{
		Names:    []string{"midaz"},
		Products: map[string][]string{"midaz": {"postgres", "valkey"}},
	}
	// env: Enter on the first row (dev). target: Enter takes the preselected
	// infra-base. action: Enter takes the preselected plan.
	ask, _ := selectorFor(t, keyEnterSeq+keyEnterSeq+keyEnterSeq)

	opts := options{target: "infra-base", action: "plan"}
	if err := guidedRun(catalog, &opts, ask); err != nil {
		t.Fatalf("guidedRun: %v", err)
	}
	if opts.environment != "dev" || opts.target != "infra-base" || opts.action != "plan" {
		t.Errorf("got --env %q --target %q --action %q, want dev / infra-base / plan",
			opts.environment, opts.target, opts.action)
	}
}

// And it has to be able to reach something other than the defaults, including a
// multi-target run.
func TestGuidedRunReachesACompositeTargetAndAnotherAction(t *testing.T) {
	catalog := infra.Catalog{
		Names:    []string{"midaz"},
		Products: map[string][]string{"midaz": {"postgres"}},
	}
	// env: down to stg, Enter.
	// target rows are bootstrap, infra-base, midaz, all. The preset already has
	// infra-base selected and the cursor on it, so pressing space here would
	// REMOVE it; the way to add a second target is to move and toggle that one.
	// down to midaz, space adds it, Enter.
	// action: down once to apply, Enter.
	keys := keyDownSeq + keyEnterSeq +
		keyDownSeq + " " + keyEnterSeq +
		keyDownSeq + keyEnterSeq
	ask, _ := selectorFor(t, keys)

	opts := options{target: "infra-base", action: "plan"}
	if err := guidedRun(catalog, &opts, ask); err != nil {
		t.Fatalf("guidedRun: %v", err)
	}
	if opts.environment != "stg" {
		t.Errorf("--env = %q, want stg", opts.environment)
	}
	if opts.target != "infra-base,midaz" {
		t.Errorf("--target = %q, want infra-base,midaz", opts.target)
	}
	if opts.action != "apply" {
		t.Errorf("--action = %q, want apply", opts.action)
	}
}

// Without a terminal the guided run does nothing at all, leaving the caller to
// fail with the error that names the flag. This is the property that keeps the
// command usable from CI, and it is the one worth guarding hardest.
func TestGuidedRunDoesNothingWithoutATerminal(t *testing.T) {
	ask := &prompter{interactive: false, out: &bytes.Buffer{}}
	opts := options{target: "infra-base", action: "plan"}

	if err := guidedRun(infra.Catalog{}, &opts, ask); err != nil {
		t.Fatalf("guidedRun: %v", err)
	}
	if opts.environment != "" {
		t.Errorf("an environment was chosen with nobody there to choose it: %q", opts.environment)
	}
}

// Aborting the guided run must abort the command, not fall through to a run with
// half the answers filled in.
func TestGuidedRunPropagatesAnAbort(t *testing.T) {
	ask, _ := selectorFor(t, "\x03")
	opts := options{target: "infra-base", action: "plan"}

	err := guidedRun(infra.Catalog{}, &opts, ask)
	if !errors.Is(err, infra.ErrAborted) {
		t.Errorf("got %v, want ErrAborted", err)
	}
}

// q aborts too, and unlike Escape it is unambiguous: a lone ESC cannot be told
// apart from the start of an arrow sequence until the next byte arrives, so in a
// real terminal it only registers once another key is pressed.
func TestPickAbortsOnQ(t *testing.T) {
	ask, out := selectorFor(t, "q")
	_, err := ask.pick("Which environment?", "", "--env", envOptions, "dev")
	if !errors.Is(err, infra.ErrAborted) {
		t.Errorf("got %v, want ErrAborted", err)
	}
	if !strings.Contains(out.String(), "q cancel") {
		t.Error("the hint line does not offer the abort key it accepts")
	}
}

// A read error that is not the end of input must surface, not be silently turned
// into an abort: a broken terminal and a canceled prompt are different events.
func TestReadKeyPropagatesARealError(t *testing.T) {
	_, err := readKey(bufio.NewReader(errorReader{}))
	if err == nil {
		t.Fatal("a read failure was swallowed")
	}
	if errors.Is(err, infra.ErrAborted) {
		t.Error("a read failure was reported as an abort")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("terminal went away") }

// A terminal in application cursor-key mode sends arrows as ESC O A / ESC O B
// instead of ESC [ A / ESC [ B, and term.MakeRaw does not reset that mode: tmux
// or a full-screen program that exited badly can leave it set. Treating the
// sequence as an abort meant the operator's first arrow press canceled the
// command.
func TestPickUnderstandsApplicationCursorKeys(t *testing.T) {
	const ss3Down, ss3Up = "\x1bOB", "\x1bOA"

	ask, _ := selectorFor(t, ss3Down+keyEnterSeq)
	got, err := ask.pick("Which environment?", "", "--env", envOptions, "dev")
	if err != nil {
		t.Fatalf("an SS3 arrow aborted the prompt: %v", err)
	}
	if got != "stg" {
		t.Errorf("got %q, want stg", got)
	}

	ask, _ = selectorFor(t, ss3Down+ss3Up+keyEnterSeq)
	got, err = ask.pick("Which environment?", "", "--env", envOptions, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if got != "dev" {
		t.Errorf("got %q, want dev", got)
	}
}

// The typed fallbacks have to show what can be answered. For the AWS profiles
// this is not cosmetic: the question says to pick by account, and the account
// lives in the note the selector would have drawn.
func TestTypedFallbacksShowTheOptions(t *testing.T) {
	options := []option{
		{value: "acme-dev", label: "acme-dev", note: "account 123456789012 · us-east-2"},
		{value: "acme-prd", label: "acme-prd", note: "account 999999999999 · us-east-1"},
		{value: "stale", label: "stale", note: "session expired", disabled: true},
	}

	t.Run("plain selection", func(t *testing.T) {
		t.Setenv("LERIAN_SELECT", "plain")
		out := &bytes.Buffer{}
		ask := &prompter{interactive: true, in: bufio.NewReader(strings.NewReader("acme-prd\n")), out: out}

		if _, err := ask.pick("Which AWS profile?", "", "--profile", options, "acme-dev"); err != nil {
			t.Fatal(err)
		}
		assertShowsAccounts(t, out.String())
	})

	t.Run("raw mode unavailable", func(t *testing.T) {
		previous := rawMode
		rawMode = func() (func(), error) { return nil, errors.New("inappropriate ioctl") }
		t.Cleanup(func() { rawMode = previous })

		out := &bytes.Buffer{}
		ask := &prompter{interactive: true, in: bufio.NewReader(strings.NewReader("acme-prd\n")), out: out}

		if _, err := ask.pick("Which AWS profile?", "", "--profile", options, "acme-dev"); err != nil {
			t.Fatal(err)
		}
		assertShowsAccounts(t, out.String())
	})
}

func assertShowsAccounts(t *testing.T, printed string) {
	t.Helper()
	for _, want := range []string{"acme-dev", "123456789012", "acme-prd", "999999999999", "session expired"} {
		if !strings.Contains(printed, want) {
			t.Errorf("the typed prompt hid %q, so the operator cannot pick by account:\n%s", want, printed)
		}
	}
}

// The redraw moves the cursor up by the number of lines it believes it wrote.
// That arithmetic is only valid while no row wraps, so nothing may be wider than
// the terminal — a wrapped row costs two display lines, the next frame moves up
// too little, and the list smears over itself.
func TestPaintedRowsNeverExceedTheWidth(t *testing.T) {
	long := []option{
		{
			value: "shared",
			label: "shared",
			note:  "all products resolve one tier you provision separately — cheaper from the third product on",
		},
		{value: "dedicated", label: "dedicated", note: "every product gets its own datastores"},
	}

	ask, out := selectorFor(t, keyEnterSeq)
	t.Setenv("NO_COLOR", "1") // measure the text, not the escapes
	if _, err := ask.pick(
		"Should each product own its datastores, or share one set?",
		"Applies to every datastore of the target.",
		"--mode", long, "dedicated"); err != nil {
		t.Fatal(err)
	}

	// A bytes.Buffer is not a terminal, so the width falls back to 80.
	for _, line := range strings.Split(out.String(), "\r\n") {
		line = strings.TrimLeft(line, " \r")
		if runes := len([]rune(line)); runes > 80 {
			t.Errorf("a painted row is %d runes wide and would wrap at 80:\n%q", runes, line)
		}
	}
}

func TestFitMarksWhatItCuts(t *testing.T) {
	tests := []struct {
		text  string
		width int
		want  string
	}{
		{"short", 20, "short"},
		{"exactly-ten", 11, "exactly-ten"},
		{"truncate-me-here", 10, "truncate-…"},
		{"anything", 0, ""},
		// Multi-byte runes must be counted as one column each, not as their bytes.
		{"São Paulo é aqui", 6, "São P…"},
	}
	for _, test := range tests {
		if got := fit(test.text, test.width); got != test.want {
			t.Errorf("fit(%q, %d) = %q, want %q", test.text, test.width, got, test.want)
		}
	}
}

// longestPaintedRow returns the widest display row the selector wrote, with the
// escape sequences removed: they cost bytes and no columns.
func longestPaintedRow(painted string) int {
	plain := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`).ReplaceAllString(painted, "")
	widest := 0
	for _, line := range strings.Split(plain, "\n") {
		// Each row is blanked with a run of spaces and rewritten after a carriage
		// return, so what the terminal shows is whatever follows the last one.
		parts := strings.Split(line, "\r")
		shown := strings.TrimRight(parts[len(parts)-1], " ")
		if n := len([]rune(shown)); n > widest {
			widest = n
		}
	}
	return widest
}

// The redraw moves the cursor up by the number of rows it believes it wrote, so
// a row that wraps costs two display lines while being counted as one and the
// next frame paints over the wrong ones. Every row has to fit — the label
// included, which is the one that was written unclipped.
func TestNoPaintedRowExceedsTheTerminalWidth(t *testing.T) {
	long := strings.Repeat("a-very-long-profile-name", 8)
	prompter, out := selectorFor(t, keyEnterSeq)

	_, err := prompter.pick("Which profile?", "", "--profile", []option{
		{value: "long", label: long, note: strings.Repeat("note ", 20)},
		{value: "short", label: "short"},
	}, "")
	if err != nil {
		t.Fatalf("pick = %v", err)
	}

	// terminalWidth falls back to 80 for a buffer, which is not a terminal.
	if widest := longestPaintedRow(out.String()); widest > 80 {
		t.Errorf("a painted row is %d columns wide, past the 80 the redraw assumes", widest)
	}
	if !strings.Contains(out.String(), "…") {
		t.Error("the long label was not marked as clipped, so a truncated name reads as a short one")
	}
}

// Reporting 80 for a 30-column terminal tells the caller it has columns it does
// not have, and then every row wraps. Too narrow to be usable is a separate
// question, answered by narrowTerminal.
func TestTerminalWidthDoesNotInventColumns(t *testing.T) {
	if got := terminalWidth(&bytes.Buffer{}); got != 80 {
		t.Errorf("terminalWidth on a non-terminal = %d, want the 80 fallback", got)
	}
	if minSelectorWidth < 20 {
		t.Errorf("minSelectorWidth = %d, too low to hold a pointer, a label and a hint", minSelectorWidth)
	}
}

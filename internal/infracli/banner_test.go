package infracli

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// The banner is decoration, and decoration in a pipe is damage: `lerian ... |
// grep` and a CI log both read every one of those lines as output. A writer that
// is not a terminal gets nothing at all.
func TestNoBannerWhereNobodyIsWatching(t *testing.T) {
	var out bytes.Buffer
	Banner(&out, "v1.7.0")

	if out.Len() != 0 {
		t.Errorf("a non-terminal was painted on:\n%s", out.String())
	}
}

// The block wordmark is 46 columns wide. Painted into anything narrower it wraps,
// and a wrapped wordmark is not a wordmark — it is six broken lines. The single
// line says the same thing and fits.
func TestTheWordmarkGivesWayOnANarrowTerminal(t *testing.T) {
	wide := renderBanner("v1.7.0", 100, false)
	narrow := renderBanner("v1.7.0", 40, false)

	if !strings.Contains(wide, "██") {
		t.Errorf("the block wordmark is missing at 100 columns:\n%s", wide)
	}
	for _, line := range strings.Split(narrow, "\n") {
		if width := displayWidth(line); width > 40 {
			t.Errorf("a %d-column line was drawn on a 40-column terminal: %q", width, line)
		}
	}
	if !strings.Contains(narrow, "lerian-cli") {
		t.Errorf("the narrow banner does not name the tool:\n%s", narrow)
	}
}

// Both forms carry the version. It is the one piece of information in there, and
// the reason a banner is worth any lines at all.
//
// Both name the tool too, in the form each has available: the wide one draws it,
// the narrow one — which has no wordmark — writes it.
func TestBothFormsCarryTheVersion(t *testing.T) {
	for _, width := range []int{100, 40} {
		painted := renderBanner("v1.7.0", width, false)
		if !strings.Contains(painted, "v1.7.0") {
			t.Errorf("no version at %d columns:\n%s", width, painted)
		}
	}
	if !strings.Contains(renderBanner("v1.7.0", 40, false), "lerian-cli") {
		t.Error("the narrow banner, which has no wordmark, does not name the tool either")
	}
	if !strings.Contains(renderBanner("v1.7.0", 100, false), "██") {
		t.Error("the wide banner does not draw the name")
	}
}

// A build with no version stamped says so rather than printing a bare separator
// with nothing after it.
func TestAnUnstampedBuildStillReads(t *testing.T) {
	painted := renderBanner("dev", 100, false)

	if strings.Contains(painted, "· \n") || strings.HasSuffix(strings.TrimRight(painted, "\n"), "·") {
		t.Errorf("a separator with nothing after it:\n%q", painted)
	}
	if !strings.Contains(painted, "dev") {
		t.Errorf("the development build is not identified:\n%s", painted)
	}
}

// The width the narrow form is chosen by is the width the wide form actually
// occupies. Counted by hand they drift, and the drift shows up as a wordmark cut
// off on exactly the terminals the check was written to protect.
func TestTheMeasuredWidthIsTheDrawnWidth(t *testing.T) {
	measured := wordmarkWidth()

	painted := renderBanner("v1.7.0", measured, false)
	if !strings.Contains(painted, "██") {
		t.Fatalf("the wordmark was dropped at the width it is said to need (%d)", measured)
	}
	for _, line := range strings.Split(painted, "\n") {
		if width := displayWidth(line); width > measured {
			t.Errorf("a %d-column line at a declared width of %d: %q", width, measured, line)
		}
	}
}

// screen is the smallest terminal that can judge an animation: it applies the
// writes, the carriage returns and the cursor moves, and reports what is left on
// it. Asserting on the raw bytes instead would pass for an animation that ends
// with the wordmark half overwritten — the bytes are all there, in the wrong
// places.
type screen struct {
	lines []string
	row   int
	col   int
}

// Write makes it a destination the banner can be painted on directly.
func (s *screen) Write(p []byte) (int, error) {
	s.put(string(p))
	return len(p), nil
}

func (s *screen) put(text string) {
	for len(text) > 0 {
		switch {
		case strings.HasPrefix(text, "\n"):
			s.row++
			s.col = 0
			text = text[1:]
		case strings.HasPrefix(text, "\r"):
			s.col = 0
			text = text[1:]
		case strings.HasPrefix(text, "\x1b["):
			end := strings.IndexAny(text, "ABCDmKJ")
			if end == -1 {
				return
			}
			if text[end] == 'A' {
				up := 1
				if n, err := strconv.Atoi(text[2:end]); err == nil {
					up = n
				}
				s.row -= up
				if s.row < 0 {
					s.row = 0
				}
			}
			text = text[end+1:]
		default:
			r, size := utf8.DecodeRuneInString(text)
			s.writeRune(r)
			text = text[size:]
		}
	}
}

func (s *screen) writeRune(r rune) {
	for len(s.lines) <= s.row {
		s.lines = append(s.lines, "")
	}
	line := []rune(s.lines[s.row])
	for len(line) <= s.col {
		line = append(line, ' ')
	}
	line[s.col] = r
	s.lines[s.row] = string(line)
	s.col++
}

func (s *screen) String() string {
	var out strings.Builder
	for _, line := range s.lines {
		out.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return out.String()
}

// The animation is a way of arriving at the banner, not a different banner. What
// is on the screen when it finishes is what a terminal that skipped it would
// have printed — otherwise the two paths drift and only one of them is ever
// looked at.
func TestTheAnimationLandsOnExactlyTheBanner(t *testing.T) {
	banner := renderBanner("v1.7.0", 100, false)

	var painted screen
	revealBanner(&painted, banner, 0, bannerShape(100, false))

	want := trimTrailing(banner)
	got := trimTrailing(painted.String())
	if got != want {
		t.Errorf("the animation ended somewhere else.\n--- on screen ---\n%s\n--- static ---\n%s", got, want)
	}
}

// And the narrow banner, which has no wordmark to light up, arrives whole just
// the same.
func TestTheNarrowAnimationLandsOnTheNarrowBanner(t *testing.T) {
	banner := renderBanner("v1.7.0", 40, false)

	var painted screen
	revealBanner(&painted, banner, 0, bannerShape(40, false))

	if trimTrailing(painted.String()) != trimTrailing(banner) {
		t.Errorf("narrow animation ended at:\n%q\nwant:\n%q", painted.String(), banner)
	}
}

// trimTrailing drops trailing blank lines and trailing spaces, which are the
// difference between "the same screen" and "the same bytes".
func trimTrailing(text string) string {
	lines := strings.Split(text, "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " ")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// The narrow form has a width of its own. "◤ lerian-cli · development build" is
// 34 columns, so on a 20-column terminal the line that exists to avoid wrapping
// wraps. It sheds what it can — the release first, then the marker — and prints
// nothing at all where even the name will not fit, which is better than a
// fragment of one.
func TestTheNarrowBannerFitsANarrowTerminal(t *testing.T) {
	for _, width := range []int{10, 16, 20, 30, 40} {
		painted := renderBanner("development build", width, false)

		for _, line := range strings.Split(painted, "\n") {
			if displayWidth(line) > width {
				t.Errorf("at %d columns a %d-column line was drawn: %q", width, displayWidth(line), line)
			}
		}
	}
}

// What it sheds, it sheds in order: the name is the last thing to go.
func TestTheNameOutlivesTheRestOfTheNarrowBanner(t *testing.T) {
	roomy := renderBanner("v1.7.0", 40, false)
	if !strings.Contains(roomy, "v1.7.0") || !strings.Contains(roomy, "lerian-cli") {
		t.Errorf("40 columns is enough for both:\n%q", roomy)
	}

	tight := renderBanner("development build", 20, false)
	if !strings.Contains(tight, "lerian-cli") {
		t.Errorf("the name was dropped before the release:\n%q", tight)
	}
}

// The sweep has to actually run: the test above only proves where the animation
// LANDS, and it lands on the same screen whether or not anything swept across it.
func TestTheWordmarkIsSweptAfterItArrives(t *testing.T) {
	var raw bytes.Buffer
	revealBanner(&raw, renderBanner("v1.7.0", 100, false), 0, bannerShape(100, false))

	// One cursor-up per frame, each going back over the whole wordmark.
	up := strings.Count(raw.String(), fmt.Sprintf("\x1b[%dA", len(wordmark)))
	if up == 0 {
		t.Fatal("nothing swept across the wordmark")
	}
	if !strings.Contains(raw.String(), "\x1b[1;97m") {
		t.Error("the sweep wrote no highlight")
	}
}

// The narrow banner has no wordmark, so there is nothing to sweep and nothing to
// move the cursor back over — doing it anyway would walk up over whatever the
// terminal had on screen before the command ran.
func TestTheNarrowBannerIsNotSwept(t *testing.T) {
	var raw bytes.Buffer
	revealBanner(&raw, renderBanner("v1.7.0", 40, false), 0, bannerShape(40, false))

	if strings.Contains(raw.String(), "\x1b[") && strings.Contains(raw.String(), "A") &&
		strings.Contains(raw.String(), fmt.Sprintf("\x1b[%dA", len(wordmark))) {
		t.Errorf("the narrow banner moved the cursor back up:\n%q", raw.String())
	}
}

// The mascot stands beside the wordmark where both fit, and gives way before the
// wordmark does: half a wizard is worse than none.
func TestTheMascotAppearsOnlyWhereItFits(t *testing.T) {
	wide := renderBanner("v1.7.0", bannerWidthWithMascot(), false)
	if !strings.Contains(wide, mascot[0]) {
		t.Errorf("no mascot at exactly the width it needs:\n%s", wide)
	}

	// One column short of needing it, the wordmark is still whole and the mascot
	// is gone.
	tight := renderBanner("v1.7.0", bannerWidthWithMascot()-1, false)
	if strings.Contains(tight, strings.TrimSpace(mascot[1])) {
		t.Errorf("the mascot survived into a width that cannot hold it:\n%s", tight)
	}
	if !strings.Contains(tight, wordmark[0]) {
		t.Errorf("the wordmark gave way before the mascot did:\n%s", tight)
	}
}

// Every row of the banner is the same width, or the mascot slides left and right
// down the six rows and reads as a rendering fault.
func TestTheMascotKeepsItsColumn(t *testing.T) {
	banner := renderBanner("v1.7.0", 120, false)

	at := -1
	for _, line := range strings.Split(banner, "\n") {
		cut := strings.IndexAny(line, "▗▟▀◉╎")
		if cut < 0 {
			continue
		}
		column := displayWidth(line[:cut])
		if at >= 0 && column != at {
			// Only the hat's point and the staff start at different columns by
			// design; this catches the whole figure drifting.
			if column < at-4 || column > at+4 {
				t.Errorf("the mascot moved from column %d to %d:\n%s", at, column, banner)
			}
		}
		if at < 0 {
			at = column
		}
	}
	if at < 0 {
		t.Fatal("no mascot was drawn at 120 columns")
	}
}

// Color is an escape sequence, and a banner written into a pipe or a NO_COLOR
// terminal must not carry one.
func TestTheMascotIsPlainWhereColorIsNotWanted(t *testing.T) {
	if strings.Contains(renderBanner("v1.7.0", 120, false), "\x1b[") {
		t.Error("the uncolored banner carries escape sequences")
	}
	if !strings.Contains(renderBanner("v1.7.0", 120, true), "\x1b[38;5;") {
		t.Error("the colored banner has no color")
	}
}

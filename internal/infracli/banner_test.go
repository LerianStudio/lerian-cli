package infracli

import (
	"bytes"
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
	wide := renderBanner("v1.7.0", 100)
	narrow := renderBanner("v1.7.0", 40)

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
		painted := renderBanner("v1.7.0", width)
		if !strings.Contains(painted, "v1.7.0") {
			t.Errorf("no version at %d columns:\n%s", width, painted)
		}
	}
	if !strings.Contains(renderBanner("v1.7.0", 40), "lerian-cli") {
		t.Error("the narrow banner, which has no wordmark, does not name the tool either")
	}
	if !strings.Contains(renderBanner("v1.7.0", 100), "██") {
		t.Error("the wide banner does not draw the name")
	}
}

// A build with no version stamped says so rather than printing a bare separator
// with nothing after it.
func TestAnUnstampedBuildStillReads(t *testing.T) {
	painted := renderBanner("dev", 100)

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

	painted := renderBanner("v1.7.0", measured)
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
	banner := renderBanner("v1.7.0", 100)

	var painted screen
	revealBanner(&painted, banner, 0)

	want := trimTrailing(banner)
	got := trimTrailing(painted.String())
	if got != want {
		t.Errorf("the animation ended somewhere else.\n--- on screen ---\n%s\n--- static ---\n%s", got, want)
	}
}

// And the narrow banner, which has no wordmark to light up, arrives whole just
// the same.
func TestTheNarrowAnimationLandsOnTheNarrowBanner(t *testing.T) {
	banner := renderBanner("v1.7.0", 40)

	var painted screen
	revealBanner(&painted, banner, 0)

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

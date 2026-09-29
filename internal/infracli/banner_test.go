package infracli

import (
	"bytes"
	"strings"
	"testing"
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
func TestBothFormsCarryTheVersion(t *testing.T) {
	for _, width := range []int{100, 40} {
		painted := renderBanner("v1.7.0", width)
		if !strings.Contains(painted, "v1.7.0") {
			t.Errorf("no version at %d columns:\n%s", width, painted)
		}
		if !strings.Contains(painted, "lerian-cli") {
			t.Errorf("the tool is not named at %d columns:\n%s", width, painted)
		}
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

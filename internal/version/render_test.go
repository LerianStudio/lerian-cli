package version

import (
	"encoding/json"
	"strings"
	"testing"
)

func sample() Info {
	return Info{
		Version:   "v1.7.0",
		Commit:    "a1b2c3d",
		Date:      "2026-09-29",
		BuiltBy:   "goreleaser",
		GoVersion: "go1.26.6",
		Platform:  "darwin/arm64",
	}
}

// Every field survives the nicer layout. A version output that drops the commit
// is worse than an ugly one: the commit is what somebody pastes into an issue.
func TestTheLayoutKeepsEveryField(t *testing.T) {
	rendered := sample().Render(false)

	for _, want := range []string{"v1.7.0", "a1b2c3d", "2026-09-29", "goreleaser", "go1.26.6", "darwin/arm64"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("%q is missing:\n%s", want, rendered)
		}
	}
}

// The labels line up, which is the whole point of laying it out at all.
func TestTheValuesShareAColumn(t *testing.T) {
	rendered := sample().Render(false)

	// Trimmed of blank lines only: TrimSpace would take the indentation off the
	// first row and make it look misaligned against the rest.
	column := -1
	for _, line := range strings.Split(strings.Trim(rendered, "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		at := strings.Index(line, fields[1])
		if column == -1 {
			column = at
			continue
		}
		if at != column {
			t.Errorf("value starts at column %d, the one before it at %d: %q", at, column, line)
		}
	}
	if column == -1 {
		t.Fatalf("nothing was rendered:\n%s", rendered)
	}
}

// --json is what scripts read. It is the one output shape that may not change,
// and it must stay free of the escape sequences the styled form may carry.
func TestTheJSONIsUnchangedAndUnstyled(t *testing.T) {
	raw, err := sample().JSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "\x1b") {
		t.Errorf("an escape sequence reached the JSON:\n%s", raw)
	}

	var back Info
	if err := json.Unmarshal([]byte(raw), &back); err != nil {
		t.Fatalf("the JSON no longer parses: %v", err)
	}
	if back != sample() {
		t.Errorf("round trip changed the value:\n%+v\n%+v", back, sample())
	}
}

// Styled output is opt-in. Rendered for a file or a pipe it carries no escapes,
// so `lerian version > version.txt` stays a file somebody can read.
func TestNoEscapesWhenNotStyled(t *testing.T) {
	if strings.Contains(sample().Render(false), "\x1b") {
		t.Error("an escape sequence was emitted with styling off")
	}
	if !strings.Contains(sample().Render(true), "\x1b") {
		t.Error("styling was requested and nothing was styled")
	}
}

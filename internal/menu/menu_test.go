package menu

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// Ctrl-D, or a stream that simply ended, is "I am done here" — not "run the
// first option". Falling through to the default turned an abandoned session
// into a plan on the first environment.
func TestEOFWithNothingTypedCancels(t *testing.T) {
	_, err := Select(NewReader(strings.NewReader("")), io.Discard, "pick", []Option{
		{Name: "plan"}, {Name: "apply"},
	})

	if !errors.Is(err, ErrCanceled) {
		t.Errorf("EOF returned %v, want ErrCanceled", err)
	}
}

// A bare Enter still takes the default. That answer arrives with a newline, not
// with EOF, which is what separates the two.
func TestBareEnterStillTakesTheDefault(t *testing.T) {
	chosen, err := Select(NewReader(strings.NewReader("\n")), io.Discard, "pick", []Option{
		{Name: "plan"}, {Name: "apply"},
	})

	if err != nil {
		t.Fatalf("Select = %v", err)
	}
	if chosen.Name != "plan" {
		t.Errorf("Select = %q, want the first option", chosen.Name)
	}
}

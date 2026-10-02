package infracli

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// ctrl-c is what everybody already presses, and in raw mode it is not a signal:
// the line editor reports it as the end of input. So is ctrl-d, and so is a pipe
// that ran out. All three mean there is no answer coming, and none of them is a
// malfunction — this used to say "cannot read the answer: EOF", which reads as
// the tool breaking at the moment somebody asked it to stop.
func TestTheEndOfInputIsLeaving(t *testing.T) {
	var out bytes.Buffer
	ask := &prompter{
		interactive: true,
		in:          bufio.NewReader(strings.NewReader("")),
		out:         &out,
	}

	_, err := ask.ask("Where is it?", "", "", "--repo")

	if !errors.Is(err, infra.ErrAborted) {
		t.Errorf("ask = %v, want ErrAborted", err)
	}
	if strings.Contains(out.String(), "cannot read") {
		t.Errorf("leaving was reported as a failure to read:\n%s", out.String())
	}
}

// A prompt that does not say how to leave it is one somebody stares at
// wondering whether the command has hung.
func TestTheTextPromptSaysHowToLeave(t *testing.T) {
	for _, test := range []struct{ name, fallback string }{
		{"with no default", ""},
		{"with a default", "/some/path"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			ask := &prompter{
				interactive: true,
				in:          bufio.NewReader(strings.NewReader("answer\n")),
				out:         &out,
			}

			if _, err := ask.ask("Where is it?", "", test.fallback, "--repo"); err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(out.String(), "ctrl-c quit") {
				t.Errorf("the prompt does not offer the key everybody presses:\n%s", out.String())
			}
			if !strings.Contains(out.String(), "q cancel") {
				t.Errorf("the prompt does not offer declining the question:\n%s", out.String())
			}
		})
	}
}

// Reporting somebody's own decision back to them in the format reserved for
// things that went wrong is what "error: aborted by the operator" did.
func TestLeavingIsNotPrintedAsAnError(t *testing.T) {
	message, status := exitLine(infra.ErrAborted)

	if strings.Contains(message, "error") {
		t.Errorf("leaving is printed as an error: %q", message)
	}
	if !strings.Contains(message, "canceled") {
		t.Errorf("leaving says %q", message)
	}
	// And still fails: `lerian infra apply && deploy` must not read a
	// confirmation nobody gave as a successful apply.
	if status == nil {
		t.Error("leaving reports success")
	}
}

func TestARealFailureIsStillPrintedAsOne(t *testing.T) {
	message, status := exitLine(errors.New("the backend is unreachable"))

	if !strings.Contains(message, "error the backend is unreachable") {
		t.Errorf("exitLine = %q", message)
	}
	if status == nil {
		t.Error("a failure reports success")
	}
}

func TestSuccessAndHelpPrintNothing(t *testing.T) {
	for _, err := range []error{nil, flag.ErrHelp} {
		message, status := exitLine(err)
		if message != "" || status != nil {
			t.Errorf("exitLine(%v) = %q, %v", err, message, status)
		}
	}
}

// Package menu is the one-question picker the interactive entry points share.
//
// It is deliberately small. A full-screen TUI would own the terminal, redraw on
// resize and need its own escape hatch; this prints a numbered list, reads a
// line, and leaves the scrollback intact — so what the operator chose stays
// visible above whatever the command then prints.
package menu

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// ErrCanceled is returned when the operator asks to leave instead of choosing.
// Callers treat it as a clean exit, not a failure: backing out of a menu is not
// an error the way a bad flag is.
var ErrCanceled = errors.New("menu: canceled")

// Option is one row. Name is what the caller matches on afterwards; Description
// is the half-line that says what it does.
type Option struct {
	Name        string
	Description string
}

// Interactive reports whether a menu can be shown at all: both ends have to be
// a terminal.
//
// This is the guard that keeps the interactive entry points from changing what
// automation sees. A script that runs the CLI with its output piped, or with no
// stdin, must get exactly what it got before — a prompt there waits forever for
// input that is never coming, and a job that used to fail in seconds with a
// readable error hangs until something kills it.
func Interactive(in, out any) bool {
	return isTerminal(in) && isTerminal(out)
}

func isTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

// Select prints the options and returns the one chosen. An empty line takes the
// first option, so the common path is a single keypress.
//
// It takes a *bufio.Reader rather than an io.Reader on purpose: a buffered
// reader created per question reads a chunk and drops whatever it did not
// return, so the second question in a session would silently lose its answer
// and fall through to the default. One reader has to span the whole session.
func Select(in *bufio.Reader, out io.Writer, question string, options []Option) (Option, error) {
	if len(options) == 0 {
		return Option{}, errors.New("menu: nothing to choose from")
	}

	width := 0
	for _, option := range options {
		if len(option.Name) > width {
			width = len(option.Name)
		}
	}

	fmt.Fprintf(out, "\n%s\n\n", question)
	for i, option := range options {
		fmt.Fprintf(out, "  %d) %-*s  %s\n", i+1, width, option.Name, option.Description)
	}
	fmt.Fprintf(out, "  q) quit\n\n")
	fmt.Fprintf(out, "Choice [1]: ")

	line, err := in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return Option{}, fmt.Errorf("menu: cannot read the answer: %w", err)
	}

	answer := strings.TrimSpace(line)

	// EOF with nothing typed is Ctrl-D, or a stream that ended. Falling through
	// to the default would turn "I am done here" into running the first option —
	// plan on the first environment, or the first command on the root menu.
	// A bare Enter is still the default; that one arrives without EOF.
	if answer == "" && errors.Is(err, io.EOF) {
		return Option{}, ErrCanceled
	}

	switch {
	case answer == "":
		return options[0], nil
	case strings.EqualFold(answer, "q"), strings.EqualFold(answer, "quit"):
		return Option{}, ErrCanceled
	}

	// A name is accepted as well as a number, so a menu can be answered with what
	// the operator already knows to type.
	for _, option := range options {
		if strings.EqualFold(answer, option.Name) {
			return option, nil
		}
	}

	choice, err := strconv.Atoi(answer)
	if err != nil || choice < 1 || choice > len(options) {
		return Option{}, fmt.Errorf("menu: %q is not one of the %d options", answer, len(options))
	}
	return options[choice-1], nil
}

// NewReader returns the reader a session shares across its questions.
func NewReader(in io.Reader) *bufio.Reader { return bufio.NewReader(in) }

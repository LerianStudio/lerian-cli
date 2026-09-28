package infracli

import (
	"io"
	"os"
)

// The selector is the CLI's one way of asking a question with a known set of
// answers, and it lives here because that is where it was written. Nothing about
// it is specific to infrastructure, and `lerian` with no arguments needs the same
// thing, so this file is the smallest surface that lets a caller outside the
// package use it without a second implementation appearing.
//
// It is deliberately narrow: one type and two functions. The selector, its typed
// fallback and the style it paints with belong in a package of their own, and
// moving them there is a mechanical change of 36 call sites in code that landed
// hours ago — worth doing, worth doing on its own. When that happens this file
// disappears and its callers import the new package directly.

// Choice is one answer offered to the operator.
type Choice struct {
	// Value is what Choose returns when this row is picked.
	Value string
	// Label is the row as the operator reads it.
	Label string
	// Note says what choosing it means, in a few words.
	Note string
}

// CanAsk reports whether there is somebody at the other end to answer, and
// somewhere they can see the question.
//
// Both ends matter. Stdin alone is not enough: with output redirected the menu
// is painted into the file while the terminal waits for a keypress, so the
// operator sees a command that has stopped and no way to know why.
//
// Callers check this before offering a menu rather than letting Choose fail:
// with either end missing, the answer is to do whatever the command did before,
// not to report that a question could not be asked.
func CanAsk(out io.Writer) bool { return isTerminal(os.Stdin) && writerIsTerminal(out) }

// writerIsTerminal reports whether out is a terminal. Anything that is not an
// *os.File — a buffer, a pipe wrapper, a tee — is not one.
func writerIsTerminal(out io.Writer) bool {
	file, ok := out.(*os.File)
	return ok && isTerminal(file)
}

// Choose asks question and returns the value of the row picked. purpose is the
// line under it saying what the answer decides; it may be empty.
//
// The error is infra.ErrAborted when the operator leaves without choosing, which
// callers treat as a clean exit rather than a failure.
func Choose(out io.Writer, question, purpose string, choices []Choice) (string, error) {
	options := make([]option, 0, len(choices))
	for _, choice := range choices {
		options = append(options, option{
			value: choice.Value,
			label: choice.Label,
			note:  choice.Note,
		})
	}
	return newPrompter(out).pick(question, purpose, "", options, "")
}

package infracli

// prompter is the entire interactive layer, and it is deliberately thin.
//
// It holds no decisions of its own: every question it asks corresponds to a flag,
// and when there is no terminal it refuses by naming that flag instead of guessing.
// That is what keeps the two front ends honest — a graphical client that never sees
// this file can still reach every capability, because the capability lives in
// pkg/infra and the flag, not in the question.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

type prompter struct {
	interactive bool
	in          *bufio.Reader
	out         io.Writer

	// editor is the line editor, built on first use and kept for the rest of the
	// run: it buffers whatever arrived past the current answer, and a new one per
	// question would discard it.
	editor *term.Terminal
}

// newPrompter returns a prompter that asks only when stdin is a terminal.
func newPrompter(out io.Writer) *prompter {
	return &prompter{
		interactive: isTerminal(os.Stdin),
		in:          bufio.NewReader(os.Stdin),
		out:         out,
	}
}

// isTerminal reports whether a human can answer on f.
//
// This is a real isatty, not a check for a character device: /dev/null is a
// character device too, so `init </dev/null` in CI would have been mistaken for an
// interactive session and asked questions nobody was there to answer. The shell
// spelling of this same test, `[ -t 0 ]`, is what deploy.sh used to use.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// selecting reports whether questions will be rendered as a list rather than
// typed. Callers use it to avoid printing a table the selector is about to draw.
func (p *prompter) selecting() bool {
	return p.interactive && !plainSelection() && !narrowTerminal(p.out)
}

// ask puts a question with its purpose.
//
// The shape is fixed on purpose: the question in bold on its own line, one line of
// plain prose saying what the answer decides, then the input on the line below.
// Question and input on the same line works for someone who already knows the
// tool; it reads as a demand for a password to everyone else, and this CLI is
// meant to be usable by somebody touching Terraform for the first time.
func (p *prompter) ask(question, purpose, fallback, flagName string) (string, error) {
	if !p.interactive {
		if fallback != "" {
			return fallback, nil
		}
		return "", fmt.Errorf("%s is required and there is no terminal to ask\n"+
			"Pass %s explicitly.", question, flagName)
	}

	theme := newStyle(p.out)
	fmt.Fprintf(p.out, "\n  %s\n", theme.bold(question))
	if purpose != "" {
		fmt.Fprintf(p.out, "  %s\n", theme.dim(purpose))
	}
	// The same line the selector shows, for the same reason: a prompt that does
	// not say how to leave it is one somebody stares at wondering whether the
	// command has hung. Both ways out are named — q declines this question, ctrl-c
	// ends the run — because the keys mean different things and only one of them
	// is a habit everybody already has.
	hint := "q cancel · ctrl-c quit"
	if fallback != "" {
		hint = "enter takes " + fallback + " · q cancel · ctrl-c quit"
	}
	fmt.Fprintf(p.out, "  %s\n", theme.dim(hint))

	answer, edited, err := p.editableLine(p.out, "  > ")
	if err != nil {
		return "", p.leftTheQuestion(question, err)
	}
	if !edited {
		// No terminal to edit on: read the line the plain way. The prompt is
		// printed here rather than above, because editableLine prints its own.
		fmt.Fprint(p.out, "  > ")
		line, readErr := p.in.ReadString('\n')
		if readErr != nil && strings.TrimSpace(line) == "" {
			return "", p.leftTheQuestion(question, readErr)
		}
		answer = strings.TrimSpace(line)
	}
	// Consistent with the selector, where q is how an operator backs out. A path
	// of exactly "q" becomes unreachable, which is a trade worth making: no
	// checkout is called that, and an inconsistent escape is worse than an
	// unreachable answer.
	if strings.EqualFold(answer, "q") {
		return "", infra.ErrAborted
	}
	if answer == "" {
		if fallback == "" {
			return "", fmt.Errorf("%s is required", question)
		}
		return fallback, nil
	}
	return answer, nil
}

// leftTheQuestion turns the end of input into leaving, and anything else into
// an error.
//
// ctrl-c arrives here as io.EOF rather than as a signal: the line editor runs in
// raw mode, where the terminal generates none, so x/term reports the keypress
// instead. ctrl-d on an empty line arrives the same way, and so does a pipe that
// ran out. All three mean the same thing — there is no answer coming — and all
// three used to be reported as "cannot read the answer: EOF", which reads as a
// malfunction when somebody has just pressed the key every terminal program has
// taught them means stop.
func (p *prompter) leftTheQuestion(question string, err error) error {
	if errors.Is(err, io.EOF) {
		// The keypress leaves the cursor mid-line, and whatever is printed next
		// would start there.
		fmt.Fprintln(p.out)
		return infra.ErrAborted
	}
	return fmt.Errorf("cannot read the answer to %q: %w", question, err)
}

// drain discards anything queued on the terminal and hands back a reader
// positioned after the flush.
//
// Behind a variable for the same reason rawMode is one: a test has no terminal
// to flush, and the ioctl fails on anything that is not one — so without this
// the confirmation could only ever be exercised by hand.
//
// The reader is returned rather than the flush happening in place because the
// two are one operation: bytes already pulled into the old buffer are beyond the
// reach of an ioctl, so a flush that does not rebuild the reader does not
// actually discard them. A stub returns nil and the caller keeps its own input.
var drain = func() (*bufio.Reader, error) {
	if err := drainStdin(); err != nil {
		return nil, err
	}
	return bufio.NewReader(os.Stdin), nil
}

// readLineOrCancel reads one line and gives up when ctx is canceled.
//
// This is what ctrl-c at a confirmation needs. The run installs a signal handler
// — it exists so an interrupt can stop terraform cleanly rather than orphaning a
// lock — and that handler consumes SIGINT and cancels ctx. A blocking Read does
// not notice a canceled context, so the prompt sat there: the key that ends
// every other program did nothing at all, at the one prompt standing in front of
// writing files.
//
// The goroutine is left parked in Read when ctx wins. There is no way to
// interrupt a blocking read on stdin portably, and the process is on its way out;
// the channel is buffered so the send cannot block forever when it does return.
func readLineOrCancel(ctx context.Context, in *bufio.Reader) (string, error) {
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := in.ReadString('\n')
		done <- result{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		return "", infra.ErrAborted
	case got := <-done:
		return got.line, got.err
	}
}

// confirm requires the word "yes", the same bar the apply confirmation uses.
func (p *prompter) confirm(ctx context.Context, errOut io.Writer, question string) error {
	if !p.interactive {
		return fmt.Errorf("this needs a confirmation but stdin is not a terminal\n"+
			"Pending: %s\n\n"+
			"Re-run from a terminal, or pass --auto-approve. The list above is exactly\n"+
			"what would be written.", question)
	}
	// Discard anything queued before the question. Unlike the prompts above, where
	// typing ahead through a known sequence is legitimate, this one guards a write
	// and must be answered deliberately.
	//
	// p.in has its own buffer, so it is rebuilt after the flush: bytes already
	// pulled out of the descriptor are beyond the reach of a terminal ioctl.
	// A drain that cannot run is refused, not ignored. This is the confirmation for
	// an apply or a destroy, and the queue is exactly where a stray Enter typed
	// during a long-running stage is waiting. --auto-approve remains available: an
	// explicit decision rather than an accident of typing.
	fresh, err := drain()
	if err != nil {
		return fmt.Errorf("cannot make sure the confirmation is answered deliberately: %w\n"+
			"Anything typed while the previous stage ran may still be queued, and would\n"+
			"answer this prompt. Re-run with --auto-approve if you mean to skip it.", err)
	}
	if fresh != nil {
		p.in = fresh
	}

	prompt := fmt.Sprintf("\n  %s [type yes to continue · ctrl-c cancels]: ", question)
	line, readErr := p.confirmLine(ctx, prompt)
	err = readErr
	switch {
	case errors.Is(err, infra.ErrAborted):
		fmt.Fprintln(errOut)
		return infra.ErrAborted
	// End of input is not a malfunction either: ctrl-d, or a pipe that ran out,
	// both mean no answer is coming — and no answer is not yes.
	case errors.Is(err, io.EOF) && strings.TrimSpace(line) == "":
		fmt.Fprintln(errOut)
		return infra.ErrAborted
	case err != nil && strings.TrimSpace(line) == "":
		return fmt.Errorf("cannot read the confirmation: %w", err)
	}
	if strings.TrimSpace(line) != "yes" {
		fmt.Fprintln(errOut)
		return infra.ErrAborted
	}
	return nil
}

// confirmLine reads the answer with the editing a shell gives — arrows, home and
// end — falling back to the plain read when there is no terminal to edit on.
//
// It is the same editor the other prompts use, which is the point: this one
// asked for a word and then rendered a left arrow as "^[[D", so correcting a
// typo in "yes" meant deleting it and starting over. A prompt that takes typed
// input and cannot be edited is a prompt that teaches people to type carefully
// rather than one that lets them correct.
//
// The editor is thrown away first. Reusing it would carry whatever it had
// already pulled out of the descriptor past the flush above — and that flush is
// the entire reason this question can be trusted to have been answered
// deliberately.
func (p *prompter) confirmLine(ctx context.Context, prompt string) (string, error) {
	p.editor = nil
	defer func() { p.editor = nil }()

	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)

	go func() {
		line, handled, err := p.editableLine(p.out, prompt)
		if handled {
			done <- result{line: line, err: err}
			return
		}
		// No terminal to edit on. The prompt has not been printed by the editor,
		// so it is printed here and read the plain way.
		fmt.Fprint(p.out, prompt)
		plain, plainErr := p.in.ReadString('\n')
		done <- result{line: plain, err: plainErr}
	}()

	// ctrl-c has to end the wait even though the read is blocked in a goroutine
	// that cannot be interrupted. The goroutine leaks until the process exits,
	// which is the price of a cancellable read on a descriptor that does not
	// support one — and this is the last question before a run ends either way.
	select {
	case <-ctx.Done():
		return "", infra.ErrAborted
	case got := <-done:
		return got.line, got.err
	}
}

// printProfiles shows what each profile reaches, which is the question an operator
// actually has: not "which profiles exist" but "which one is the right account".
func (p *prompter) printProfiles(resolved []infra.ResolvedProfile) {
	if !p.interactive {
		return
	}
	fmt.Fprintf(p.out, "\n%s\n\n", newStyle(p.out).bold("==> AWS profiles"))
	writer := tabwriter.NewWriter(p.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "  PROFILE\tACCOUNT\tREGION\tSTATUS")

	failed := 0
	for _, entry := range resolved {
		account, status := "-", "ok"
		if !entry.Usable() {
			// The per-row status stays short. Repeating the remedy on every row turns
			// a dozen profiles behind one expired SSO session into a dozen lines that
			// each look like a separate problem; the fix is printed once, below.
			status = "expired"
			failed++
		} else {
			account = entry.Caller.Account
		}
		region := entry.Profile.Region
		if region == "" {
			region = "-"
		}
		fmt.Fprintf(writer, "  %s\t%s\t%s\t%s\n", entry.Profile.Name, account, region, status)
	}
	_ = writer.Flush()

	if hint := infra.LoginHint(resolved); failed > 0 && hint != "" {
		fmt.Fprintf(p.out, "\n  %d profile(s) need a login:\n  %s\n", failed, hint)
	}
	fmt.Fprintln(p.out)
}

package infracli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// ctrl-c at the confirmation reaches the signal handler the run installs, which
// cancels the context — and then nothing happened, because a blocking read does
// not notice a canceled context. The key that ends every other program did
// nothing at all, at the one prompt standing in front of writing files.
//
// blockingReader stands in for a terminal nobody is typing at.
type blockingReader struct{ release chan struct{} }

func (b blockingReader) Read([]byte) (int, error) {
	<-b.release
	return 0, io.EOF
}

// noDrain stands in for the terminal flush, which has no terminal to work on
// under go test.
func noDrain(t *testing.T) {
	t.Helper()
	previous := drain
	// nil keeps the caller's own reader, which is the test's input.
	drain = func() (*bufio.Reader, error) { return nil, nil }
	t.Cleanup(func() { drain = previous })
}

func TestACanceledContextEndsTheConfirmation(t *testing.T) {
	noDrain(t)
	reader := blockingReader{release: make(chan struct{})}
	defer close(reader.release)

	ctx, cancel := context.WithCancel(context.Background())
	ask := &prompter{interactive: true, in: bufio.NewReader(reader), out: &bytes.Buffer{}}

	go func() {
		// Whatever the scheduler does, the read is already parked or about to be;
		// either way the cancel has to be what ends it.
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() { done <- ask.confirm(ctx, &bytes.Buffer{}, "Write 4 file(s)?") }()

	select {
	case err := <-done:
		if !errors.Is(err, infra.ErrAborted) {
			t.Errorf("confirm = %v, want ErrAborted", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("confirm ignored the canceled context and is still waiting")
	}
}

// Same for the apply confirmation, which is a separate function reading stdin
// directly.
func TestACanceledContextEndsTheApplyConfirmation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- confirmOnStdin(ctx, &bytes.Buffer{}, "type yes: ") }()

	select {
	case err := <-done:
		// Under go test stdin is not a terminal, so this returns the
		// "not a terminal" error before it ever reads. Both outcomes are fine; what
		// must not happen is waiting forever.
		if err == nil {
			t.Error("a canceled run was confirmed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("confirmOnStdin ignored the canceled context")
	}
}

// The prompt has to say that ctrl-c is the way out, because at this one the
// answer is a word rather than a keypress and there is no hint line.
func TestTheConfirmationSaysHowToCancel(t *testing.T) {
	noDrain(t)

	var out bytes.Buffer
	ask := &prompter{
		interactive: true,
		in:          bufio.NewReader(strings.NewReader("yes\n")),
		out:         &out,
	}

	if err := ask.confirm(context.Background(), &bytes.Buffer{}, "Write 4 file(s)?"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ctrl-c") {
		t.Errorf("the confirmation does not say how to cancel:\n%s", out.String())
	}
}

// End of input is not yes. ctrl-d, or a pipe that ran out, both mean no answer
// is coming.
func TestTheEndOfInputDoesNotConfirm(t *testing.T) {
	noDrain(t)

	ask := &prompter{
		interactive: true,
		in:          bufio.NewReader(strings.NewReader("")),
		out:         &bytes.Buffer{},
	}

	err := ask.confirm(context.Background(), &bytes.Buffer{}, "Write 4 file(s)?")

	if !errors.Is(err, infra.ErrAborted) {
		t.Errorf("confirm = %v, want ErrAborted", err)
	}
}

// And a word that is not "yes" still cancels, which is the bar this prompt has
// always had.
func TestOnlyTheWholeWordConfirms(t *testing.T) {
	noDrain(t)

	for _, answer := range []string{"y", "Yes", "no", ""} {
		t.Run(answer, func(t *testing.T) {
			ask := &prompter{
				interactive: true,
				in:          bufio.NewReader(strings.NewReader(answer + "\n")),
				out:         &bytes.Buffer{},
			}

			if err := ask.confirm(context.Background(), &bytes.Buffer{}, "Write?"); !errors.Is(err, infra.ErrAborted) {
				t.Errorf("%q was taken as a confirmation: %v", answer, err)
			}
		})
	}
}

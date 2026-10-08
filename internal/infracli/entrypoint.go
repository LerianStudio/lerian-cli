package infracli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// ErrFailed reports that the command already explained itself on stderr. The
// caller only has to turn it into a non-zero exit status.
var ErrFailed = errors.New("infra command failed")

// Run is the entry point behind `lerian infra`. It owns what the standalone
// lerian-infra binary used to own in main(): interrupt handling, the silent exit
// on --help, and the error line. Keeping the rendering here rather than letting
// cobra print it is what makes the ported command byte-identical to the binary
// it replaces.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	message, status := exitLine(run(ctx, args, stdout, stderr))
	if message != "" {
		fmt.Fprint(stderr, message)
	}
	return status
}

// exitLine decides what a finished run prints and what it returns.
//
// Separated from the printing so the decision can be tested: the difference
// between "canceled" and "error" is the difference between reporting somebody's
// own decision back to them and reporting a malfunction.
func exitLine(err error) (string, error) {
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return "", nil

	// Leaving is not a malfunction. q, ctrl-c and a declined confirmation all
	// arrive here, and the error format is reserved for things that went wrong.
	//
	// The status stays non-zero: `lerian infra apply && deploy` must not treat a
	// confirmation nobody gave as a successful apply.
	case errors.Is(err, infra.ErrAborted):
		return "\n  canceled.\n\n", ErrFailed

	default:
		return fmt.Sprintf("\nerror %v\n\n", err), ErrFailed
	}
}

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

	err := run(ctx, args, stdout, stderr)
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return nil
	default:
		fmt.Fprintf(stderr, "\nerror %v\n\n", err)
		return ErrFailed
	}
}

package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	infrapkg "github.com/lerian-studio/lerian-cli/internal/infra"
	"github.com/lerian-studio/lerian-cli/internal/infracli"
	"github.com/lerian-studio/lerian-cli/internal/version"
)

// session runs the menu until the operator leaves it, and reports the exit code.
//
// The menu is a session rather than a launcher. Running what was picked and
// exiting means a second command costs a second start-up — and the commands here
// come in sequences: check the machine, then init; init fails, then read what it
// says and try again.
//
// A failed command does not end it. The operator is sitting right there, and the
// usual next step after a failure is another command, not another `lerian`.
//
// The code is the last command's, not the worst one seen: a session that ended on
// success ended in success, and `lerian && something` should believe it.
func session(ask func() (string, error), dispatch func(string) error) int {
	code := 0
	for {
		chosen, err := ask()
		if errors.Is(err, infrapkg.ErrAborted) {
			return code
		}
		if err != nil {
			// The menu itself broke — the terminal went away mid-question. There is
			// nothing left to ask into.
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}

		code = 0
		if err := dispatch(chosen); err != nil {
			// cobra has already printed it. Repeating it here would double every
			// failure message in the one mode where the operator is watching.
			code = 1
		}
	}
}

// menuAsk is the session's question: it offers the commands and returns the one
// picked. It does not run it — a child's Execute walks up to the root, so
// running the choice from inside the asking would re-enter the root's RunE and
// open a second session, forever.
//
// The list is read off cobra rather than written out, so a command added later
// appears without anyone having to remember to add it twice.
func menuAsk(root *cobra.Command) func() (string, error) {
	return func() (string, error) {
		return infracli.Choose(root.OutOrStdout(), "What do you want to do?", "", menuChoices(root))
	}
}

// interactive is the session wired to the real menu and the real commands.
func interactive(root *cobra.Command) int {
	infracli.Banner(root.OutOrStdout(), version.GetVersion())

	return session(
		menuAsk(root),
		func(chosen string) error {
			// A fresh argument list each time: cobra keeps the last one, so without
			// this the second command inherits the first one's flags.
			root.SetArgs([]string{chosen})
			return root.Execute()
		},
	)
}

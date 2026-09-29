package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
		chosen, err := infracli.Choose(root.OutOrStdout(), "What do you want to do?", "", menuChoices(root))
		if err != nil {
			return "", err
		}

		// A command with no handler of its own does nothing when dispatched by
		// name: cobra prints its help. auth is that — its work is in login and
		// logout — so the menu goes one level down rather than answering a choice
		// with a reference page.
		if !needsChild(root, chosen) {
			return chosen, nil
		}

		child, err := infracli.Choose(root.OutOrStdout(),
			"Which "+chosen+" command?", "", childChoices(root, chosen))
		if err != nil {
			return "", err
		}
		return chosen + " " + child, nil
	}
}

// needsChild reports whether name is a command that only groups others.
func needsChild(root *cobra.Command, name string) bool {
	command := findChild(root, name)
	return command != nil && !command.Runnable() && command.HasAvailableSubCommands()
}

// childChoices is the menu for a command's subcommands.
func childChoices(root *cobra.Command, name string) []infracli.Choice {
	parent := findChild(root, name)
	if parent == nil {
		return nil
	}

	choices := make([]infracli.Choice, 0, len(parent.Commands()))
	for _, child := range parent.Commands() {
		if child.Hidden || !child.IsAvailableCommand() {
			continue
		}
		if child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		choices = append(choices, infracli.Choice{
			Value: child.Name(),
			Label: child.Name(),
			Note:  child.Short,
		})
	}
	return choices
}

func findChild(root *cobra.Command, name string) *cobra.Command {
	for _, child := range root.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

// interactive is the session wired to the real menu and the real commands.
func interactive(root *cobra.Command) int {
	infracli.Banner(root.OutOrStdout(), version.GetVersion())

	return session(
		menuAsk(root),
		func(chosen string) error {
			// A fresh argument list each time: cobra keeps the last one, so without
			// this the second command inherits the first one's flags. Split, because
			// a choice may be a path — "auth login" — rather than a single name.
			root.SetArgs(strings.Fields(chosen))
			return root.Execute()
		},
	)
}

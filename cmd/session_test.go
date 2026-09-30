package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"

	infrapkg "github.com/lerian-studio/lerian-cli/internal/infra"
)

// The menu is a session, not a launcher: running the thing that was picked and
// exiting means choosing a second command costs a second start-up, banner and
// all. It asks again once the command is done.
func TestTheMenuComesBackAfterACommandRuns(t *testing.T) {
	answers := []string{"version", "infra"}
	var ran []string

	code := session(
		func() (string, error) {
			if len(answers) == 0 {
				return "", infrapkg.ErrAborted
			}
			next := answers[0]
			answers = answers[1:]
			return next, nil
		},
		func(name string) error {
			ran = append(ran, name)
			return nil
		},
	)

	if len(ran) != 2 || ran[0] != "version" || ran[1] != "infra" {
		t.Errorf("ran %v, want both commands", ran)
	}
	if code != 0 {
		t.Errorf("exit code %d for a session that ended cleanly", code)
	}
}

// Leaving is what q does, and it is not a failure.
func TestLeavingTheMenuExitsCleanly(t *testing.T) {
	code := session(
		func() (string, error) { return "", infrapkg.ErrAborted },
		func(string) error { t.Fatal("nothing was chosen and something ran"); return nil },
	)

	if code != 0 {
		t.Errorf("exit code %d for leaving the menu", code)
	}
}

// A command that fails does not end the session. The operator is sitting there,
// and the usual next step after a failure is another command — reading the logs,
// checking the environment — not starting the CLI again.
//
// The failure it ends on is still the answer the shell gets, so `lerian && deploy`
// does not run deploy.
func TestAFailedCommandKeepsTheSessionOpen(t *testing.T) {
	answers := []string{"version", "infra"}
	var ran []string

	code := session(
		func() (string, error) {
			if len(answers) == 0 {
				return "", infrapkg.ErrAborted
			}
			next := answers[0]
			answers = answers[1:]
			return next, nil
		},
		func(name string) error {
			ran = append(ran, name)
			if name == "infra" {
				return errors.New("terraform is not installed")
			}
			return nil
		},
	)

	if len(ran) != 2 {
		t.Errorf("the session ended before the second command: ran %v", ran)
	}
	if code == 0 {
		t.Error("a session whose last command failed reported success")
	}
}

// A failure followed by a command that works is a session that succeeded: the
// code reports the last thing that happened, not the worst.
func TestTheCodeIsTheLastCommandsCode(t *testing.T) {
	answers := []string{"infra", "version"}

	code := session(
		func() (string, error) {
			if len(answers) == 0 {
				return "", infrapkg.ErrAborted
			}
			next := answers[0]
			answers = answers[1:]
			return next, nil
		},
		func(name string) error {
			if name == "infra" {
				return errors.New("terraform is not installed")
			}
			return nil
		},
	)

	if code != 0 {
		t.Errorf("exit code %d after a successful last command", code)
	}
}

// Anything other than a clean exit from the menu itself — a terminal that went
// away mid-question — ends the session rather than asking into the void.
func TestABrokenMenuEndsTheSession(t *testing.T) {
	var ran []string
	code := session(
		func() (string, error) { return "", errors.New("the terminal went away") },
		func(name string) error { ran = append(ran, name); return nil },
	)

	if len(ran) != 0 {
		t.Errorf("ran %v after the menu failed", ran)
	}
	if code == 0 {
		t.Error("a broken menu reported success")
	}
}

// auth has no handler of its own — its work is in login and logout — so
// dispatching the bare name lands the operator on a help page instead of on
// something they chose to do. That is the same defect that kept midaz out of the
// menu; the difference is that auth is worth reaching, so the menu goes one level
// down instead.
func TestAParentCommandOffersItsChildren(t *testing.T) {
	root := &cobra.Command{Use: "lerian"}
	auth := &cobra.Command{Use: "auth", Short: "Authentication commands"}
	auth.AddCommand(&cobra.Command{Use: "login", Short: "Configure credentials", Run: func(*cobra.Command, []string) {}})
	auth.AddCommand(&cobra.Command{Use: "logout", Short: "Remove credentials", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(auth)
	root.AddCommand(&cobra.Command{Use: "version", Short: "Print version", Run: func(*cobra.Command, []string) {}})

	children := childChoices(root, "auth")
	offered := make([]string, 0, len(children))
	for _, choice := range children {
		offered = append(offered, choice.Value)
	}

	if len(offered) != 2 || offered[0] != "login" || offered[1] != "logout" {
		t.Errorf("offered %v, want the two subcommands", offered)
	}
}

// A command that does its own work is dispatched, not drilled into.
func TestARunnableCommandIsNotDrilledInto(t *testing.T) {
	root := &cobra.Command{Use: "lerian"}
	root.AddCommand(&cobra.Command{Use: "version", Short: "Print version", Run: func(*cobra.Command, []string) {}})

	if needsChild(root, "version") {
		t.Error("a runnable command was treated as a menu")
	}
	if !needsChild(root, "missing") == false {
		t.Error("an unknown name was treated as a parent")
	}
}

// And the path that comes back is what cobra is handed, so the child actually
// runs rather than the parent printing help again.
func TestTheChosenChildIsDispatchedAsAPath(t *testing.T) {
	var ran []string

	code := session(
		func() (string, error) {
			if len(ran) > 0 {
				return "", infrapkg.ErrAborted
			}
			return "auth login", nil
		},
		func(name string) error { ran = append(ran, name); return nil },
	)

	if code != 0 {
		t.Errorf("exit code %d", code)
	}
	if len(ran) != 1 || ran[0] != "auth login" {
		t.Errorf("dispatched %v, want the full path", ran)
	}
}

// A command that does something of its own AND groups others still offers the
// group.
//
// config prints the configuration and owns reset. The rule as written only
// drilled into commands with no handler, so picking config from the menu printed
// the configuration and came back — and reset, the only other thing it can do,
// was unreachable from the menu entirely.
//
// The parent is not listed beside its children: `lerian config` and
// `lerian config show` are the same action, and two rows for one action is a
// choice that decides nothing.
func TestARunnableParentStillOffersItsChildren(t *testing.T) {
	root := &cobra.Command{Use: "lerian"}
	config := &cobra.Command{
		Use:   "config",
		Short: "Show or reset what this tool remembers",
		Run:   func(*cobra.Command, []string) {},
	}
	config.AddCommand(&cobra.Command{Use: "show", Short: "Print it", Run: func(*cobra.Command, []string) {}})
	config.AddCommand(&cobra.Command{Use: "reset", Short: "Forget everything", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(config)

	if !needsChild(root, "config") {
		t.Fatal("a command with subcommands did not offer them")
	}

	children := childChoices(root, "config")
	offered := make([]string, 0, len(children))
	for _, choice := range children {
		offered = append(offered, choice.Value)
	}

	if len(offered) != 2 {
		t.Errorf("offered %v, want one row per subcommand and no duplicate of the parent", offered)
	}
	for _, want := range []string{"show", "reset"} {
		found := false
		for _, value := range offered {
			if value == want {
				found = true
			}
		}
		if !found {
			t.Errorf("offered %v, want %q among them", offered, want)
		}
	}
}

// A command with no children is dispatched, not turned into a menu of one.
func TestACommandWithNoChildrenIsJustRun(t *testing.T) {
	root := &cobra.Command{Use: "lerian"}
	root.AddCommand(&cobra.Command{Use: "version", Short: "Print version", Run: func(*cobra.Command, []string) {}})

	if needsChild(root, "version") {
		t.Error("a command with no subcommands was turned into a menu")
	}
}

// The cursor starts on the first row, so a submenu that opens on the destructive
// command makes the most likely keypress the one that removes things. Cobra sorts
// its subcommands alphabetically, and "reset" sorts before "show".
func TestTheDestructiveChildIsNotWhereTheCursorOpens(t *testing.T) {
	root := &cobra.Command{Use: "lerian"}
	config := &cobra.Command{Use: "config", Short: "Show or reset", Run: func(*cobra.Command, []string) {}}
	reset := &cobra.Command{
		Use:         "reset",
		Short:       "Forget everything",
		Annotations: map[string]string{menuAnnotation: menuLast},
		Run:         func(*cobra.Command, []string) {},
	}
	config.AddCommand(&cobra.Command{Use: "show", Short: "Print it", Run: func(*cobra.Command, []string) {}}, reset)
	root.AddCommand(config)

	choices := childChoices(root, "config")

	if len(choices) != 2 {
		t.Fatalf("got %d rows", len(choices))
	}
	if choices[0].Value == "reset" {
		t.Error("the submenu opens on the command that removes things")
	}
	if choices[len(choices)-1].Value != "reset" {
		t.Errorf("reset is not last: %+v", choices)
	}
}

// A command whose only subcommands are the ones cobra generates has nothing to
// offer, and opening a menu with no rows in it fails with "has no options to
// choose from" — an error about the menu rather than about the command.
//
// HasAvailableSubCommands is cobra's answer and childChoices is this file's, and
// they filter different things: cobra keeps completion, this drops it. Where they
// disagree, the command is simply run.
func TestACommandWhoseChildrenAreAllFilteredIsJustRun(t *testing.T) {
	root := &cobra.Command{Use: "lerian"}
	lonely := &cobra.Command{Use: "lonely", Short: "Does something", Run: func(*cobra.Command, []string) {}}
	lonely.AddCommand(&cobra.Command{Use: "completion", Short: "Generated", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(lonely)

	if needsChild(root, "lonely") {
		t.Errorf("a command with nothing to offer opens a menu: %+v", childChoices(root, "lonely"))
	}
}

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func fakeRoot(t *testing.T) *cobra.Command {
	t.Helper()

	root := &cobra.Command{Use: "lerian"}
	root.AddCommand(&cobra.Command{Use: "auth", Short: "Authentication commands", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "infra", Short: "Deploy the AWS stacks", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "hidden", Hidden: true, Run: func(*cobra.Command, []string) {}})

	root.SetOut(&bytes.Buffer{})
	return root
}

// Asking must not run what it picked. A child's Execute walks up to the root and
// starts from there, so running the choice from inside the root's own RunE
// re-enters it and never returns. The session runs the answer afterwards, from
// outside that RunE, which is what breaks the cycle.
func TestAskingDoesNotRunTheChoice(t *testing.T) {
	t.Cleanup(func() { wantsSession = false })

	ran := false
	root := &cobra.Command{Use: "lerian"}
	root.AddCommand(&cobra.Command{
		Use:   "infra",
		Short: "Deploy the AWS stacks",
		Run:   func(*cobra.Command, []string) { ran = true },
	})
	root.SetOut(&bytes.Buffer{})

	// No terminal here, so the selector refuses rather than reading a key. What
	// is under test is that nothing was executed either way.
	_, _ = menuAsk(root)()

	if ran {
		t.Error("asking executed the chosen command, which re-enters the root and loops")
	}
}

// The list is read off cobra so a command added later shows up without anyone
// registering it twice, and the ones cobra generates for itself stay out.
//
// Asserted on the choices rather than on the painted menu: without a terminal
// the selector refuses before drawing anything, so the output says nothing about
// what would have been offered.
func TestTheMenuOffersTheRealCommandsOnly(t *testing.T) {
	root := fakeRoot(t)

	choices := menuChoices(root)
	names := make([]string, 0, len(choices))
	for _, choice := range choices {
		names = append(names, choice.Value)
	}
	offered := strings.Join(names, " ")

	for _, want := range []string{"auth", "infra"} {
		if !strings.Contains(offered, want) {
			t.Errorf("the menu does not offer %q: %v", want, names)
		}
	}
	for _, unwanted := range []string{"completion", "hidden", "help"} {
		if strings.Contains(offered, unwanted) {
			t.Errorf("the menu offers %q, which is not a command an operator picks: %v", unwanted, names)
		}
	}
}

// Each row carries the command's own one-line description, so the menu says what
// the choices mean rather than only naming them.
func TestEachChoiceCarriesItsDescription(t *testing.T) {
	root := fakeRoot(t)

	for _, choice := range menuChoices(root) {
		if choice.Note == "" {
			t.Errorf("%q is offered with no description", choice.Value)
		}
	}
}

// Giving the root a RunE makes cobra treat an unrecognized command as an
// argument to it, so without an explicit rejection `lerian nonexistent` prints
// help and exits 0 — reporting success for a typo.
func TestAnUnknownCommandIsStillRejected(t *testing.T) {
	err := rootCmd.RunE(rootCmd, []string{"nonexistent"})

	if err == nil {
		t.Fatal("an unknown command returned nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("the error does not name the command: %v", err)
	}
}

// The menu is a shorter list than the command set. A command can be worth having
// and not worth offering — midaz is the whole ledger surface, reached with
// arguments the menu cannot ask for, so offering it lands the operator in a help
// page rather than in anything they chose to do.
//
// Marked on the command rather than filtered by name here, so the two places do
// not have to be kept in agreement.
func TestACommandCanBeKeptOutOfTheMenu(t *testing.T) {
	root := fakeRoot(t)
	root.AddCommand(&cobra.Command{
		Use:         "midaz",
		Short:       "Midaz ledger management commands",
		Annotations: map[string]string{menuAnnotation: menuSkip},
		Run:         func(*cobra.Command, []string) {},
	})

	for _, choice := range menuChoices(root) {
		if choice.Value == "midaz" {
			t.Error("a command marked as not offered was offered")
		}
	}

	// Still a command. Kept out of the menu is not removed from the CLI, and
	// `lerian midaz ledger list` has to keep working.
	found := false
	for _, child := range root.Commands() {
		if child.Name() == "midaz" {
			found = true
		}
	}
	if !found {
		t.Error("the command was removed from the CLI, not just from the menu")
	}
}

// And the menu still offers what it should — read off cobra, so a command added
// later appears here without anyone registering it twice. config arrived that
// way, and this list is the only place it had to be written down again.
func TestTheMenuOffersAuthConfigInfraAndVersion(t *testing.T) {
	choices := menuChoices(rootCmd)
	offered := make([]string, 0, len(choices))
	for _, choice := range choices {
		offered = append(offered, choice.Value)
	}

	want := []string{"auth", "config", "infra", "version"}
	if len(offered) != len(want) {
		t.Fatalf("the menu offers %v, want exactly %v", offered, want)
	}
	for index, name := range want {
		if offered[index] != name {
			t.Errorf("offered[%d] = %q, want %q (%v)", index, offered[index], name, offered)
		}
	}
}

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

// The menu must not run what it picked. A child's Execute walks up to the root
// and starts from there, so running the choice from inside the root's own RunE
// re-enters the menu and never returns. Recording the answer is what breaks that
// cycle, and Execute dispatches it afterwards.
func TestChooseCommandDoesNotRunTheChoice(t *testing.T) {
	t.Cleanup(func() { chosenCommand = "" })

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
	_ = chooseCommand(root)

	if ran {
		t.Error("chooseCommand executed the chosen command, which re-enters the root and loops")
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

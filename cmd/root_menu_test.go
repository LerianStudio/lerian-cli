package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func fakeRoot(t *testing.T, in string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	root := &cobra.Command{Use: "lerian"}
	root.AddCommand(&cobra.Command{Use: "auth", Short: "Authentication commands", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "infra", Short: "Deploy the AWS stacks", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "hidden", Hidden: true, Run: func(*cobra.Command, []string) {}})

	var out bytes.Buffer
	root.SetIn(strings.NewReader(in))
	root.SetOut(&out)
	return root, &out
}

func TestChooseCommandRecordsTheAnswer(t *testing.T) {
	t.Cleanup(func() { chosenCommand = "" })
	root, _ := fakeRoot(t, "2\n")

	if err := chooseCommand(root); err != nil {
		t.Fatalf("chooseCommand = %v", err)
	}

	if chosenCommand != "infra" {
		t.Errorf("chosenCommand = %q, want %q", chosenCommand, "infra")
	}
}

// chooseCommand must not run the choice itself: a child's Execute walks up to
// the root and starts there, so running it from inside the root's RunE re-enters
// the menu and never returns. Recording the answer is what breaks that cycle.
func TestChooseCommandDoesNotRunTheChoice(t *testing.T) {
	t.Cleanup(func() { chosenCommand = "" })

	ran := false
	root := &cobra.Command{Use: "lerian"}
	root.AddCommand(&cobra.Command{
		Use:   "infra",
		Short: "Deploy the AWS stacks",
		Run:   func(*cobra.Command, []string) { ran = true },
	})
	root.SetIn(strings.NewReader("1\n"))
	root.SetOut(&bytes.Buffer{})

	if err := chooseCommand(root); err != nil {
		t.Fatalf("chooseCommand = %v", err)
	}
	if ran {
		t.Error("chooseCommand executed the chosen command, which re-enters the root and loops")
	}
}

// The list is read off cobra so a command added later shows up without anyone
// remembering to register it twice — and the ones cobra generates stay out.
func TestMenuListsTheRealCommandsAndHidesTheGeneratedOnes(t *testing.T) {
	t.Cleanup(func() { chosenCommand = "" })
	root, out := fakeRoot(t, "1\n")

	if err := chooseCommand(root); err != nil {
		t.Fatalf("chooseCommand = %v", err)
	}

	report := out.String()
	for _, name := range []string{"auth", "infra"} {
		if !strings.Contains(report, name) {
			t.Errorf("the menu does not offer %q:\n%s", name, report)
		}
	}
	for _, name := range []string{"completion", "hidden"} {
		if strings.Contains(report, name) {
			t.Errorf("the menu offers %q, which is not a command an operator picks:\n%s", name, report)
		}
	}
}

func TestQuittingTheMenuRecordsNothing(t *testing.T) {
	t.Cleanup(func() { chosenCommand = "" })
	root, _ := fakeRoot(t, "q\n")

	if err := chooseCommand(root); err != nil {
		t.Fatalf("quitting returned %v, want nil: backing out is not a failure", err)
	}
	if chosenCommand != "" {
		t.Errorf("quitting still recorded %q", chosenCommand)
	}
}

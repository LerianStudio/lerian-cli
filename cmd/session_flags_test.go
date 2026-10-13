package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// One process runs every command of a session, so a flag is parsed into the
// same variable over and over. Without a reset, "install --wait=false" followed
// by plain "install" installs without waiting a second time — and the operator
// typed nothing of the sort.
func TestFlagsDoNotCarryBetweenCommands(t *testing.T) {
	var wait bool
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}}
	child.Flags().BoolVar(&wait, "wait", true, "")
	root.AddCommand(child)

	root.SetArgs([]string{"child", "--wait=false"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if wait {
		t.Fatal("the flag was not parsed at all")
	}

	// The second run names no flag, so it must see the default.
	resetFlags(root)
	root.SetArgs([]string{"child"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if !wait {
		t.Error("the previous run's --wait=false survived into a command that did not pass it")
	}
	if child.Flags().Changed("wait") {
		t.Error("the flag still reports Changed, so code asking whether it was typed is misled")
	}
}

// Persistent flags live on the parent and are just as sticky.
func TestPersistentFlagsAreResetToo(t *testing.T) {
	var profile string
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().StringVar(&profile, "profile", "default", "")
	root.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})

	root.SetArgs([]string{"child", "--profile", "work"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	resetFlags(root)

	if profile != "default" {
		t.Errorf("profile = %q after reset, want the registered default", profile)
	}
}

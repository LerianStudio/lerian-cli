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

// A slice flag is replaced, never Set: Set appends, and pflag renders an empty
// slice's default as the literal "[]". Resetting with Set therefore added an
// element called "[]" on every pass through the menu — and three passes put
// three of them in a Kubernetes Deployment, where the API server rejected the
// duplicate keys.
func TestResettingASliceDoesNotAppendItsOwnDefault(t *testing.T) {
	var namespaces []string
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}}
	child.Flags().StringSliceVar(&namespaces, "namespaces", nil, "")
	root.AddCommand(child)

	// Three passes, the way a session does it.
	for range 3 {
		resetFlags(root)
		root.SetArgs([]string{"child"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	}

	if len(namespaces) != 0 {
		t.Errorf("the reset invented %d values: %q", len(namespaces), namespaces)
	}
}

// A slice with a real default must come back to that default, not to empty.
func TestASliceWithADefaultIsRestoredToIt(t *testing.T) {
	var names []string
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}}
	child.Flags().StringSliceVar(&names, "names", []string{"one", "two"}, "")
	root.AddCommand(child)

	root.SetArgs([]string{"child", "--names", "other"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	resetFlags(root)

	if len(names) != 2 || names[0] != "one" || names[1] != "two" {
		t.Errorf("the registered default was not restored: %q", names)
	}
}

// The parsing this depends on, since pflag offers no way back to the original.
func TestTheRenderedDefaultIsReadBack(t *testing.T) {
	for rendered, want := range map[string][]string{
		"[]":         nil,
		"[one]":      {"one"},
		"[one,two]":  {"one", "two"},
		"[one, two]": {"one", "two"},
		"":           nil,
	} {
		got := defaultSlice(rendered)
		if len(got) != len(want) {
			t.Errorf("%q -> %q, want %q", rendered, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q -> %q, want %q", rendered, got, want)
				break
			}
		}
	}
}

// pflag's slice values replace on the first Set and append on every one after,
// and the field recording which is unexported. Replace changes the value and
// not that bit — so a flag parsed once kept appending for the life of the
// process, and this session runs every command in one process.
//
// This also fails loudly if pflag renames the field, rather than letting the
// appending come back unnoticed.
func TestASliceParsedTwiceDoesNotAppendToItsDefault(t *testing.T) {
	var namespaces []string
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}}
	child.Flags().StringSliceVar(&namespaces, "ns", []string{"the-default"}, "")
	root.AddCommand(child)

	root.SetArgs([]string{"child", "--ns", "first"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	resetFlags(root)

	root.SetArgs([]string{"child", "--ns", "second"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if len(namespaces) != 1 || namespaces[0] != "second" {
		t.Errorf("the second parse appended instead of replacing: %q", namespaces)
	}
}

// `lerian --profile work` is parsed before the menu opens, and resetting to the
// registered default afterwards threw away the one thing the operator had said.
// A flag given on the way in describes the session; one given to a menu command
// belongs to that command.
func TestFlagsGivenWhenTheSessionOpensSurvive(t *testing.T) {
	var profile string
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().StringVar(&profile, "profile", "default", "")
	root.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})

	// The run that opens the session.
	root.SetArgs([]string{"child", "--profile", "work"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	opening := openingFlags(root)

	// A later menu choice, with no flags of its own.
	resetFlags(root)
	restoreFlags(root, opening)
	root.SetArgs([]string{"child"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if profile != "work" {
		t.Errorf("profile = %q, want the value the session was started with", profile)
	}
}

// And a flag that was never given stays at its default rather than being
// pinned to whatever it happened to hold.
func TestFlagsNotGivenAreNotPinned(t *testing.T) {
	var profile string
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().StringVar(&profile, "profile", "default", "")
	root.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})

	root.SetArgs([]string{"child"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	opening := openingFlags(root)

	if len(opening) != 0 {
		t.Errorf("a flag nobody passed was recorded as the session's: %v", opening)
	}
}

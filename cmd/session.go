package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"reflect"
	"unsafe"

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
		if errors.Is(err, infracli.ErrBack) {
			// Nowhere to go back to from the first menu, and it is not a way out:
			// ending the session on the key that means "the previous thing" is the
			// opposite of what was asked for.
			continue
		}
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
		if errors.Is(err, infracli.ErrBack) {
			// Back out of the submenu and into the menu it came from.
			return "", err
		}
		if err != nil {
			return "", err
		}
		return chosen + " " + child, nil
	}
}

// needsChild reports whether picking this command should offer a second menu.
//
// Any command with subcommands does, whether or not it does something itself.
// config does both — it prints the configuration and owns reset — and the earlier
// rule, which drilled in only when there was no handler, left reset unreachable
// from the menu entirely.
func needsChild(root *cobra.Command, name string) bool {
	// Asked of childChoices rather than of cobra: the two filter different things
	// — cobra keeps completion, this does not — and a menu built from a list this
	// function disagreed with would open with no rows in it, failing with an error
	// about the menu instead of running the command.
	return len(childChoices(root, name)) > 0
}

// childChoices is the menu for a command's subcommands.
func childChoices(root *cobra.Command, name string) []infracli.Choice {
	parent := findChild(root, name)
	if parent == nil {
		return nil
	}

	// The children only. A runnable parent is not listed beside them: `lerian
	// config` and `lerian config show` do the same thing, and two rows for one
	// action is a choice that decides nothing.
	//
	// The assumption is that a parent's own action has a child that covers it —
	// true here, and the shape to keep: a command whose bare form does something no
	// subcommand does would be unreachable from this menu.
	choices := make([]infracli.Choice, 0, len(parent.Commands()))

	var first, last []infracli.Choice
	for _, child := range parent.Commands() {
		if child.Hidden || !child.IsAvailableCommand() {
			continue
		}
		if child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		choice := infracli.Choice{
			Value: child.Name(),
			Label: child.Name(),
			Note:  child.Short,
		}
		switch child.Annotations[menuAnnotation] {
		case menuFirst:
			first = append(first, choice)
		case menuLast:
			last = append(last, choice)
		default:
			choices = append(choices, choice)
		}
	}
	return append(append(first, choices...), last...)
}

func findChild(root *cobra.Command, name string) *cobra.Command {
	for _, child := range root.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

// openingFlags records the persistent flags the session was started with.
//
// Persistent only: a subcommand's own flags belong to the command that was run,
// and carrying "lerian agent install --wait=false" into the next choice is the
// leak the reset exists to stop. A flag given on the way in is different — it
// describes the session, not one command in it.
func openingFlags(root *cobra.Command) map[string]string {
	given := map[string]string{}
	// VisitAll and Changed, not Visit: Visit walks what was parsed into this
	// flag set, and cobra parses a subcommand's line into the subcommand's own.
	// The persistent flag carries Changed either way, which is the fact worth
	// reading.
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			given[f.Name] = f.Value.String()
		}
	})
	return given
}

// restoreFlags puts the session's opening values back after a reset.
func restoreFlags(root *cobra.Command, opening map[string]string) {
	for name, value := range opening {
		flag := root.PersistentFlags().Lookup(name)
		if flag == nil {
			continue
		}
		_ = flag.Value.Set(value)
		flag.Changed = true
	}
}

// resetFlags returns every flag in the tree to the value it was defined with.
//
// One process runs every command of a session, so a flag is parsed into the
// same variable over and over. Cobra has no hook between one Execute and the
// next, and no notion of a run being over, so this is the seam.
func resetFlags(cmd *cobra.Command) {
	reset := func(flags *pflag.FlagSet) {
		flags.VisitAll(func(f *pflag.Flag) {
			// A slice is replaced, never Set. Set appends on a slice flag, and
			// DefValue renders an empty one as the literal "[]" — so resetting
			// with Set added an element called "[]" every time through the menu.
			// Three passes produced three of them, and they reached a cluster.
			if slice, ok := f.Value.(pflag.SliceValue); ok {
				_ = slice.Replace(defaultSlice(f.DefValue))
				// And the parser's own state, which Replace does not touch. A
				// slice value remembers that it has been Set, and appends from
				// then on — so after a reset, "--ns second" produced
				// ["the-default", "second"] rather than ["second"].
				forgetSliceParse(f.Value)
				f.Changed = false
				return
			}
			// DefValue is the flag's value as it was registered, which is what a
			// command invoked with nothing should see.
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
	reset(cmd.Flags())
	reset(cmd.PersistentFlags())
	for _, child := range cmd.Commands() {
		resetFlags(child)
	}
}

// forgetSliceParse clears the "I have been Set" bit a slice flag keeps.
//
// pflag's slice values replace on the first Set and append on every one after,
// and the field recording which is unexported with no accessor. SliceValue
// offers Replace, which changes the value and not that bit — so a flag parsed
// once keeps appending for the life of the process, and this session runs every
// command in one process.
//
// Reaching for an unexported field is worth saying out loud: it is wrong if
// pflag renames it, and the test below fails loudly in that case rather than
// letting the appending come back unnoticed.
func forgetSliceParse(value pflag.Value) {
	field := reflect.ValueOf(value)
	if field.Kind() != reflect.Pointer || field.IsNil() {
		return
	}
	changed := field.Elem().FieldByName("changed")
	if !changed.IsValid() || changed.Kind() != reflect.Bool {
		return
	}
	// #nosec G103 -- the address is of a field in a struct this process owns,
	// reached to clear a bool pflag provides no other way to clear. See above.
	reflect.NewAt(changed.Type(), unsafe.Pointer(changed.UnsafeAddr())).Elem().SetBool(false)
}

// defaultSlice reads a slice flag's registered default back out of the string
// pflag renders it as.
//
// pflag prints an empty slice as "[]" and a populated one as "[a,b]". There is
// no accessor for the original, so this is the only way back — and getting it
// wrong is how "[]" became a value.
func defaultSlice(rendered string) []string {
	rendered = strings.TrimSpace(rendered)
	rendered = strings.TrimPrefix(rendered, "[")
	rendered = strings.TrimSuffix(rendered, "]")
	if rendered == "" {
		return nil
	}
	parts := strings.Split(rendered, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			values = append(values, part)
		}
	}
	return values
}

// interactive is the session wired to the real menu and the real commands.
func interactive(root *cobra.Command) int {
	infracli.Banner(root.OutOrStdout(), version.GetVersion())

	// What the session was started with. `lerian --profile work` parses that
	// before the menu opens, and resetting to the registered default afterwards
	// threw away the one thing the operator had said — along with --config and
	// --output.
	//
	// Taken once, here, because every later value comes from a menu choice and
	// belongs to that command alone.
	opening := openingFlags(root)

	return session(
		menuAsk(root),
		func(chosen string) error {
			// A fresh argument list each time: cobra keeps the last one, so without
			// this the second command inherits the first one's flags. Split, because
			// a choice may be a path — "auth login" — rather than a single name.
			root.SetArgs(strings.Fields(chosen))
			// And fresh flag values, for the same reason one step deeper. SetArgs
			// replaces what will be parsed; it does not undo the last parse. The
			// variables flags are bound to still hold the previous run's values and
			// still report Changed, so "install --wait=false" followed by plain
			// "install" installs without waiting a second time — the operator typed
			// nothing of the sort.
			//
			// Before Execute, because cobra parses during it: resetting afterwards
			// would discard what was just typed.
			resetFlags(root)
			restoreFlags(root, opening)
			return root.Execute()
		},
	)
}

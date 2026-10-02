// Package cmd — the config command.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/infra"
	"github.com/lerian-studio/lerian-cli/internal/infracli"
)

var configResetYes bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show or reset what this tool remembers",
	Long: `What the CLI has written down about this machine, and how to clear it.

The configuration is one file, ~/.lerian/config.yaml. It holds where the
templates checkout is and the profiles 'lerian auth login' creates — nothing
else, and nothing belonging to another tool.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		}
		return showConfig(cmd.OutOrStdout())
	},
	SilenceUsage: true,
}

var configTemplatesCmd = &cobra.Command{
	Use:   "templates <path>",
	Short: "Record which lerian-terraform-foundation checkout to use",
	Long: `Writes the path into ~/.lerian/config.yaml, so every later run finds it
without a flag or a variable.

It beats the managed path at ~/lerian/lerian-terraform-foundation — one is a
decision, the other a directory that happens to exist somewhere conventional —
and loses to standing inside a checkout, which is the one you are looking at.

  lerian config templates /path/to/lerian-terraform-foundation
  lerian config templates --clear`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runSetTemplates,
	SilenceUsage: true,
}

var configTemplatesClear bool

func runSetTemplates(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	if configTemplatesClear {
		return clearTemplates(out)
	}
	if len(args) == 0 {
		// Reached off the menu, where there is no command line to put a path on.
		// The usage text of a command nobody typed is not an answer to that.
		if infracli.CanAsk(out) {
			return askThenRecord(out, infracli.AskForTemplates)
		}
		return fmt.Errorf("give the path to a lerian-terraform-foundation checkout\n" +
			"  lerian config templates /path/to/lerian-terraform-foundation\n" +
			"  lerian config templates --clear")
	}
	return setTemplates(out, args[0])
}

// askThenRecord asks and then does what the answer says.
//
// Leaving the prompt is not a failure: r and q mean "not this", and the session
// is about to redraw the menu that was left. Reporting an error there would put
// a red line under a decision not to decide.
// The prompt arrives as an argument so a test can answer it. A test cannot
// reach the real one: it needs a terminal on both ends, and under go test there
// is none — which is also why the CanAsk line above has no test of its own.
func askThenRecord(out io.Writer, ask func(io.Writer) (infracli.TemplatesAnswer, error)) error {
	answer, err := ask(out)
	switch {
	case errors.Is(err, infracli.ErrBack), errors.Is(err, infra.ErrAborted):
		return nil
	case err != nil:
		return err
	case answer.Forget:
		return clearTemplates(out)
	}
	return setTemplates(out, answer.Path)
}

// setTemplates records a checkout, after checking it is one.
//
// Checked here rather than at the next run: a path accepted now and rejected
// later fails naming a missing file, several steps from the moment somebody could
// have fixed the typo.
func setTemplates(out io.Writer, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve %q: %w", path, err)
	}
	if !infra.IsCheckout(absolute) {
		return fmt.Errorf("no lerian-terraform-foundation checkout at %s\n"+
			"A checkout is recognized by the directories examples/aws/_modules and\n"+
			"examples/aws/backend; at least one of them is missing there", absolute)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.TemplatesCheckout = absolute
	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Fprintf(out, "\n  templates  %s\n", absolute)
	fmt.Fprintf(out, "  Recorded. Later runs use it without a flag.\n\n")
	return nil
}

// clearTemplates forgets the path and leaves the directory alone: it is a clone
// somebody made, possibly with work in it.
func clearTemplates(out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.TemplatesCheckout == "" {
		fmt.Fprintf(out, "\n  Nothing was recorded.\n\n")
		return nil
	}

	previous := cfg.TemplatesCheckout
	cfg.TemplatesCheckout = ""
	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Fprintf(out, "\n  Forgot %s\n", previous)
	fmt.Fprintf(out, "  The directory is untouched. Later runs discover a checkout again.\n\n")
	return nil
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the configuration and where it lives",
	// Nothing here takes an argument, and without this cobra accepts and ignores
	// them — so `lerian config reset production`, which reads like "reset the
	// production profile", would quietly remove everything instead.
	Args:         cobra.NoArgs,
	RunE:         func(cmd *cobra.Command, _ []string) error { return showConfig(cmd.OutOrStdout()) },
	SilenceUsage: true,
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Forget everything, as if the CLI had never run here",
	// Last in the submenu: the cursor starts on the first row, and this is the one
	// that removes things.
	Annotations: map[string]string{menuAnnotation: menuLast},
	Long: `Removes ~/.lerian/config.yaml, so the next run asks what it asked the first
time: where the templates are, which account to deploy into.

It takes that file and nothing else. ~/.aws belongs to the AWS CLI and every
tool on this machine reads it; a templates checkout is a git clone you made,
possibly with work in it. Neither is this command's to delete.`,
	Args:         cobra.NoArgs,
	RunE:         func(cmd *cobra.Command, _ []string) error { return resetConfig(cmd.OutOrStdout()) },
	SilenceUsage: true,
}

// noteManagedCheckout says what reset will not change.
//
// reset forgets what the config records. A checkout sitting in the managed path
// is not recorded — it is found by convention, which is the whole point of that
// path — so the next run keeps using it. Somebody who has just been told the tool
// forgot everything, and then watches it carry on with a checkout, is owed the
// sentence that explains why.
func noteManagedCheckout(out io.Writer) bool {
	managed, err := infra.ManagedCheckoutPath("")
	if err != nil || !infra.IsCheckout(managed) {
		return false
	}

	fmt.Fprintf(out, "  The checkout at %s stays.\n", managed)
	fmt.Fprintf(out, "  It is found by convention rather than recorded here, so the next run\n")
	fmt.Fprintf(out, "  still uses it. Remove the directory yourself if that is what you want.\n\n")
	return true
}

func showConfig(out io.Writer) error {
	infracli.DescribeMachine(context.Background(), out)
	return nil
}

// resetConfig asks before removing, because "as if it had never run" is not
// something to do to somebody by accident. --yes is the way to mean it in a
// script, and outside a terminal there is nobody to ask.
//
//nolint:nilerr // see the comment on the declining branch below
func resetConfig(out io.Writer) error {
	described, err := config.Describe()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%s\n", described)
	staying := noteManagedCheckout(out)

	if !configResetYes {
		if !infracli.CanAsk(out) {
			return fmt.Errorf("this removes the configuration above and there is no terminal to ask\n" +
				"Pass --yes if that is what you want")
		}
		answer, err := infracli.Choose(out, "Forget this configuration?",
			"The next run asks what it asked the first time. Nothing outside the file above is touched.",
			[]infracli.Choice{
				{Value: "no", Label: "keep it", Note: "changes nothing"},
				{Value: "yes", Label: "forget it", Note: "removes ~/.lerian/config.yaml"},
			})
		// Declining is not a failure, and neither is a selector that could not draw:
		// either way nothing was removed, which is the safe outcome and the one the
		// operator can see. Reporting "the prompt failed" instead would be alarming
		// about a file that is still exactly where it was.
		if err != nil || answer != "yes" {
			fmt.Fprintf(out, "  kept.\n\n")
			return nil
		}
	}

	removed, err := config.Reset()
	if err != nil {
		return err
	}
	if len(removed) == 0 {
		fmt.Fprintf(out, "  nothing to forget — there was no configuration.\n\n")
		return nil
	}
	for _, path := range removed {
		fmt.Fprintf(out, "  removed %s\n", path)
	}
	// Not "from nothing" when something is about to be picked up again — that
	// sentence would sit three lines under the note saying otherwise.
	if staying {
		fmt.Fprintf(out, "\n  The next run asks again, and finds the checkout above.\n\n")
		return nil
	}
	fmt.Fprintf(out, "\n  The next run starts from nothing.\n\n")
	return nil
}

func init() {
	configResetCmd.Flags().BoolVar(&configResetYes, "yes", false, "do not ask")
	configTemplatesCmd.Flags().BoolVar(&configTemplatesClear, "clear", false, "forget the recorded path")
	configCmd.AddCommand(configShowCmd, configTemplatesCmd, configResetCmd)
	rootCmd.AddCommand(configCmd)
}

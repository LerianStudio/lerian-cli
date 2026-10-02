// Package cmd — the config command.
package cmd

import (
	"context"
	"fmt"
	"io"

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
	configCmd.AddCommand(configShowCmd, configResetCmd)
	rootCmd.AddCommand(configCmd)
}

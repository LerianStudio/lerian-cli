// Package cmd provides the root command and CLI initialization.
//
// The cmd package is the entry point for the Lerian CLI application.
// It sets up the root command and registers all subcommands including
// authentication (auth) and Midaz management (midaz) commands.
//
// Configuration is managed through flags and configuration files,
// supporting multiple profiles for different environments.
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/cmd/auth"
	"github.com/lerian-studio/lerian-cli/cmd/infra"
	"github.com/lerian-studio/lerian-cli/cmd/midaz"
	infrapkg "github.com/lerian-studio/lerian-cli/internal/infra"
	"github.com/lerian-studio/lerian-cli/internal/infracli"
	"github.com/lerian-studio/lerian-cli/internal/version"
)

var (
	// cfgFile holds the path to the configuration file.
	// Can be set via --config flag or LERIAN_CONFIG environment variable.
	cfgFile string

	// profile specifies which configuration profile to use.
	// Profiles allow managing multiple environments (dev, staging, prod).
	profile string

	// output defines the output format for command results.
	// Supported formats: json, yaml, table (default).
	output string

	// chosenCommand carries what the menu picked from the root's RunE out to
	// Execute, which is the only place that can dispatch it without re-entering
	// the menu.
	chosenCommand string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "lerian",
	Short: "Lerian CLI - Manage your Lerian platform services",
	Long: `Lerian CLI is a unified command-line interface for managing
your Lerian platform services including Midaz, Finflow, and Finbase.

Use this CLI to interact with your ledgers, manage deployments,
and access your data.`,
	Version: version.GetVersion(),

	// `lerian` with nothing after it offers the commands rather than printing the
	// reference text and leaving. Only on a terminal: piped, redirected or in CI
	// it prints what it always printed, because a prompt there waits for an
	// answer that is never coming.
	RunE: func(cmd *cobra.Command, args []string) error {
		// Giving the root a RunE makes cobra treat an unrecognized command as an
		// argument to it rather than an error, so the rejection has to be restated
		// here — otherwise `lerian nonexistent` prints help and exits 0.
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		}
		if !infracli.CanAsk(cmd.OutOrStdout()) {
			return cmd.Help()
		}
		return chooseCommand(cmd)
	},

	// The usage block is suppressed but the error is not: a mistyped command
	// needs the one line saying what was wrong, not the whole reference under it.
	SilenceUsage: true,
}

// menuChoices is the command list the menu offers: what cobra knows, minus the
// two it generates for itself and anything hidden. Reading it off cobra rather
// than writing it out means a command added later appears without anyone having
// to remember to add it twice.
func menuChoices(root *cobra.Command) []infracli.Choice {
	var choices []infracli.Choice
	for _, child := range root.Commands() {
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

// chooseCommand asks which command to run and records the answer. It does not
// run it: a child's Execute walks up to the root and starts there, so executing
// the choice from inside the root's own RunE re-enters this menu, forever.
// Execute dispatches the answer once this call has returned.
//
// The list is read off cobra rather than written out, so a command added later
// appears without anyone having to remember to add it twice.
func chooseCommand(root *cobra.Command) error {
	chosen, err := infracli.Choose(root.OutOrStdout(), "What do you want to do?", "", menuChoices(root))
	if errors.Is(err, infrapkg.ErrAborted) {
		return nil
	}
	if err != nil {
		return err
	}

	chosenCommand = chosen
	return nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
// It returns no value but will exit with code 1 on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}

	// The menu records a choice rather than running it; dispatching here, after
	// the first Execute has returned, is what keeps the root out of its own RunE.
	if chosenCommand == "" {
		return
	}
	rootCmd.SetArgs([]string{chosenCommand})
	chosenCommand = ""
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.lerian/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&profile, "profile", "p", "default", "profile to use")
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "table", "output format (table, json, yaml)")

	// Add subcommands
	rootCmd.AddCommand(auth.AuthCmd)
	rootCmd.AddCommand(midaz.MidazCmd)
	rootCmd.AddCommand(infra.InfraCmd)
}

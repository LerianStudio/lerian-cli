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
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/cmd/auth"
	"github.com/lerian-studio/lerian-cli/cmd/infra"
	"github.com/lerian-studio/lerian-cli/cmd/midaz"
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

	// wantsSession carries the decision to open the menu from the root's RunE out
	// to Execute, which is the only place that can run commands without
	// re-entering that RunE for each one.
	wantsSession bool
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

	// `lerian` with nothing after it opens a session rather than printing the
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
		// Recorded, not run: a child's Execute walks up to the root and starts
		// there, so opening the session from inside the root's own RunE would
		// re-enter this function for every command the session runs. Execute opens
		// it once this call has returned.
		wantsSession = true
		return nil
	},

	// The usage block is suppressed but the error is not: a mistyped command
	// needs the one line saying what was wrong, not the whole reference under it.
	SilenceUsage: true,
}

// menuAnnotation marks a command that the session's menu does not offer.
//
// A command can be worth having and not worth offering. midaz is the whole
// ledger surface — its subcommands take ledger ids, regions and sizes the menu
// has no way to ask for — so picking it from a list lands the operator on a help
// page rather than on something they chose to do. It stays a command: `lerian
// midaz ledger list` is unaffected.
//
// Marked on the command rather than filtered by name here, so the decision lives
// next to the thing it describes and there is only one place to change.
const (
	menuAnnotation = "menu"
	menuSkip       = "skip"
	// menuLast puts a command at the end of its submenu. The cursor starts on the
	// first row, and a list that opens on the command that removes things makes the
	// most likely keypress the destructive one — cobra sorts alphabetically, which
	// is how "reset" ended up above "show".
	menuLast = "last"
)

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
		if child.Annotations[menuAnnotation] == menuSkip {
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

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
// It returns no value but will exit with code 1 on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
	if !wantsSession {
		return
	}

	// Opened here, after the first Execute has returned, which is what keeps the
	// root out of its own RunE while the session runs commands through it.
	wantsSession = false
	if code := interactive(rootCmd); code != 0 {
		os.Exit(code)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.lerian/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&profile, "profile", "p", "default", "profile to use")
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "table", "output format (table, json, yaml)")

	// Add subcommands
	rootCmd.AddCommand(auth.AuthCmd)
	// A command, but not one the menu offers — see menuAnnotation.
	midaz.MidazCmd.Annotations = map[string]string{menuAnnotation: menuSkip}
	rootCmd.AddCommand(midaz.MidazCmd)
	rootCmd.AddCommand(infra.InfraCmd)
}

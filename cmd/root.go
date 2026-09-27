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
	"github.com/lerian-studio/lerian-cli/internal/menu"
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

	// chosenCommand carries what the interactive menu picked from the root's RunE
	// out to Execute, which is the only place that can dispatch it without
	// re-entering the menu.
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

	// Bare `lerian` on a terminal offers the commands instead of printing help
	// and leaving. Anywhere else — piped, redirected, in CI — it keeps printing
	// help, because a prompt there waits for input that never arrives.
	RunE: func(cmd *cobra.Command, args []string) error {
		// Giving the root a RunE makes cobra treat an unrecognized command as an
		// argument to it rather than an error, so the rejection has to be restated
		// here — otherwise `lerian nonexistent` prints help and exits 0.
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		}
		if !menu.Interactive(cmd.InOrStdin(), cmd.OutOrStdout()) {
			return cmd.Help()
		}
		return chooseCommand(cmd)
	},

	// The usage block is suppressed but the error is not: a mistyped command or a
	// bad menu answer needs the one line saying what was wrong, not the whole
	// reference text under it.
	SilenceUsage: true,
}

// chooseCommand asks which command to run and records the answer. It does not
// run it: a child's Execute walks up to the root and starts there, so executing
// the choice from inside the root's own RunE re-enters the menu, forever.
// Execute picks the answer up instead and dispatches once this call has
// returned.
func chooseCommand(root *cobra.Command) error {
	var options []menu.Option
	for _, child := range root.Commands() {
		if child.Hidden || !child.IsAvailableCommand() {
			continue
		}
		if child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		options = append(options, menu.Option{Name: child.Name(), Description: child.Short})
	}

	chosen, err := menu.Select(menu.NewReader(root.InOrStdin()), root.OutOrStdout(),
		"What do you want to do?", options)
	if errors.Is(err, menu.ErrCanceled) {
		return nil
	}
	if err != nil {
		return err
	}

	chosenCommand = chosen.Name
	return nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
// It returns no value but will exit with code 1 on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}

	// The interactive menu records a choice rather than running it; dispatching
	// here, after the first Execute has returned, is what keeps the root from
	// re-entering its own RunE.
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

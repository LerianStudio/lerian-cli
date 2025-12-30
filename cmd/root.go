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
	"os"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/cmd/auth"
	"github.com/lerian-studio/lerian-cli/cmd/midaz"
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
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
// It returns no value but will exit with code 1 on error.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
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
}

package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/cmd/auth"
	"github.com/lerian-studio/lerian-cli/cmd/midaz"
	"github.com/lerian-studio/lerian-cli/internal/version"
)

var (
	// Used for flags
	cfgFile string
	profile string
	output  string
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

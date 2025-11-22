package cmd

import (
	"fmt"
	"os"

	"github.com/lerian-studio/lerian-cli/internal/version"
	"github.com/spf13/cobra"
)

var (
	versionJSON bool
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long: `Print detailed version information including version number,
commit hash, build date, and platform details.`,
	Example: `  # Print short version
  lerian version

  # Print detailed version information
  lerian version --verbose

  # Print version as JSON
  lerian version --json`,
	RunE: runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "Output version information as JSON")
}

func runVersion(cmd *cobra.Command, args []string) error {
	info := version.GetInfo()

	// Output as JSON if requested
	if versionJSON {
		jsonOutput, err := info.JSON()
		if err != nil {
			return fmt.Errorf("failed to generate JSON output: %w", err)
		}
		fmt.Fprintln(os.Stdout, jsonOutput)
		return nil
	}

	// Default: print full version information
	fmt.Fprintln(os.Stdout, info.String())
	return nil
}

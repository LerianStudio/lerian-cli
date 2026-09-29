package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lerian-studio/lerian-cli/internal/version"
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
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), jsonOutput)
		return nil
	}

	// Laid out for a person, styled only where somebody is watching: redirected
	// into a file or a bug report it must carry no escape sequences.
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprint(out, info.Render(styles(out)))
	return nil
}

// styles reports whether this destination may carry escape sequences. The same
// question the rest of the CLI asks: a terminal, no NO_COLOR, and not the dumb
// terminal an editor's shell reports.
func styles(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return false
	}
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		return false
	}
	return os.Getenv("TERM") != "dumb"
}

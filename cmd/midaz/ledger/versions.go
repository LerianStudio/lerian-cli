package ledger

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
)

var versionsCmd = &cobra.Command{
	Use:   "versions",
	Short: "List available ledger versions",
	Long: `List all available Midaz ledger versions with their app and Helm chart versions.

Use this command to see which versions you can deploy using the --app-version
and --chart-version flags in the create command.

Examples:
  # List all available versions
  lerian ledger versions

  # Show only the latest version
  lerian ledger versions --latest`,
	SilenceUsage: true,
	RunE:         runVersions,
}

var (
	showLatestOnly bool
)

func init() {
	versionsCmd.Flags().BoolVar(&showLatestOnly, "latest", false, "Show only the latest version")
}

func runVersions(cmd *cobra.Command, args []string) error {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get current profile
	profile, err := cfg.GetProfile("")
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}

	// Create API client
	apiClient := client.NewClient(profile.APIURL, profile.APIKey, profile.TenantID)

	// Get versions
	versions, err := apiClient.ListVersions()
	if err != nil {
		return fmt.Errorf("failed to list versions: %w", err)
	}

	if len(versions) == 0 {
		fmt.Println("No versions available")
		return nil
	}

	// Filter to latest if requested
	if showLatestOnly {
		for _, v := range versions {
			if v.IsLatest {
				displayVersions([]*client.LedgerVersion{v})
				return nil
			}
		}
		fmt.Println("No latest version found")
		return nil
	}

	displayVersions(versions)
	return nil
}

func displayVersions(versions []*client.LedgerVersion) {
	color.Green("\nAvailable Ledger Versions\n")
	fmt.Println(strings.Repeat("=", 100))

	for i, v := range versions {
		// Header with version info
		latestMarker := ""
		if v.IsLatest {
			latestMarker = " [LATEST]"
		}

		fmt.Printf("\n%d. App: %s | Chart: %s%s\n",
			i+1, v.AppVersion, v.ChartVersion, latestMarker)

		// Support tier
		fmt.Printf("   Support: %s\n", strings.ToUpper(v.SupportTier))

		// Release date
		fmt.Printf("   Released: %s\n", v.ReleaseDate.Format("2006-01-02"))

		// Changelog
		if v.Changelog != "" {
			fmt.Printf("   Changes: %s\n", v.Changelog)
		}

		// Usage example
		fmt.Printf("\n   To use this version:\n")
		fmt.Printf("   lerian ledger create --name my-ledger \\\n")
		fmt.Printf("     --app-version %s --chart-version %s \\\n", v.AppVersion, v.ChartVersion)
		fmt.Printf("     --region us-east-1 --env dev\n")

		if i < len(versions)-1 {
			fmt.Println(strings.Repeat("-", 100))
		}
	}

	fmt.Println(strings.Repeat("=", 100))
	fmt.Println()
}

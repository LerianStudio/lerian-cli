package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/config"
)

var (
	apiURL   string
	apiKey   string
	tenantID string
	profile  string
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Configure authentication credentials",
	Long:  `Configure authentication credentials for accessing the Lerian platform.`,
	RunE:  runLogin,
}

func init() {
	loginCmd.Flags().StringVar(&apiURL, "api-url", "", "API URL (e.g., http://localhost:8080)")
	loginCmd.Flags().StringVar(&apiKey, "api-key", "", "API Key")
	loginCmd.Flags().StringVar(&tenantID, "tenant-id", "", "Tenant ID")
	loginCmd.Flags().StringVar(&profile, "profile", "default", "Profile name")

	_ = loginCmd.MarkFlagRequired("api-url")
	_ = loginCmd.MarkFlagRequired("api-key")
	_ = loginCmd.MarkFlagRequired("tenant-id")
}

func runLogin(_ *cobra.Command, args []string) error {
	// Load existing config or create new one
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set the profile
	cfg.SetProfile(profile, config.Profile{
		APIURL:   apiURL,
		APIKey:   apiKey,
		TenantID: tenantID,
	})

	// Set as current profile
	cfg.CurrentProfile = profile

	// Save config
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Successfully logged in to profile '%s'\n", profile)
	fmt.Printf("API URL: %s\n", apiURL)
	fmt.Printf("Tenant ID: %s\n", tenantID)

	configPath, _ := config.GetConfigPath()
	fmt.Printf("\nConfiguration saved to: %s\n", configPath)

	return nil
}

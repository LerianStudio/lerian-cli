package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/config"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove authentication credentials",
	Long:  `Remove authentication credentials for the current or specified profile.`,
	RunE:  runLogout,
}

func init() {
	logoutCmd.Flags().StringVar(&profile, "profile", "default", "Profile name to logout")
}

func runLogout(_ *cobra.Command, _ []string) error {
	// Load existing config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if profile exists
	if _, err := cfg.GetProfile(profile); err != nil {
		return fmt.Errorf("profile '%s' not found", profile)
	}

	// Delete the profile
	if err := cfg.DeleteProfile(profile); err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}

	// Save config
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Successfully logged out from profile '%s'\n", profile)

	return nil
}

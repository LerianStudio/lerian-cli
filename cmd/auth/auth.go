package auth

import (
	"github.com/spf13/cobra"
)

// AuthCmd represents the auth command
var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication commands",
	Long:  `Manage authentication and credentials for the Lerian CLI.`,
}

func init() {
	AuthCmd.AddCommand(loginCmd)
	AuthCmd.AddCommand(logoutCmd)
}

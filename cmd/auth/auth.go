package auth

import (
	"github.com/spf13/cobra"
)

// AuthCmd represents the auth command
var AuthCmd = &cobra.Command{
	Use: "auth",
	// Says what it authenticates with. On a menu whose other entry deploys AWS
	// infrastructure, "Authentication commands" invites exactly the wrong guess —
	// and AWS credentials are not managed here at all, they are read from ~/.aws.
	Short: "Sign in to the Lerian platform (not AWS)",
	Long: `Manage the credentials this CLI uses to reach the Lerian platform: the
API URL, the API key and the tenant. They are kept in ~/.lerian/config.yaml.

AWS credentials are not managed here. They live in ~/.aws, which the AWS CLI
owns, and 'lerian infra' reads them from there.`,
}

func init() {
	AuthCmd.AddCommand(loginCmd)
	AuthCmd.AddCommand(logoutCmd)
}

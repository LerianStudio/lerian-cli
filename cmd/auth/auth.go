package auth

import (
	"github.com/spf13/cobra"
)

// AuthCmd represents the auth command
// AuthCmd is in the menu but not selectable: signing in to the platform is not
// working yet, and a row that leads somewhere broken costs more than a row that
// says so.
var AuthCmd = &cobra.Command{
	Annotations: map[string]string{
		"menu": "disabled",
		// Still says which credentials it is about: a disabled row is read by
		// somebody deciding where to sign in, and on a menu whose other entry
		// deploys AWS infrastructure the obvious guess is the wrong one.
		"menu-disabled-reason": "not available yet — Sign in to the Lerian platform (not AWS)",
	},
	Use: "auth",
	// Says what it authenticates with. On a menu whose other entry deploys AWS
	// infrastructure, "Authentication commands" invites exactly the wrong guess —
	// and AWS credentials are not managed here at all, they are read from ~/.aws.
	Short: "Sign in to the Lerian platform (not AWS)",
	Long: `Manage the credentials this CLI uses to reach the Lerian platform: the
API URL, the API key and the tenant. They are kept in ~/.lerian/config.yaml.

AWS credentials are not managed here. 'lerian infra' reads them the way every
AWS tool does: a profile in ~/.aws, or the credentials already in the
environment — a CI runner has the second and no ~/.aws at all.`,
}

func init() {
	AuthCmd.AddCommand(loginCmd)
	AuthCmd.AddCommand(logoutCmd)
}

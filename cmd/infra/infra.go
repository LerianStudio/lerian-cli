// Package infra exposes the Terraform deployment commands as `lerian infra`.
package infra

import (
	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/infracli"
)

// InfraCmd represents the infra command
var InfraCmd = &cobra.Command{
	Use:   "infra",
	Short: "Deploy the AWS stacks of lerian-terraform-foundation",
	Long: `Drive the Terraform roots of lerian-terraform-foundation: bootstrap the
state backend, stand up the VPC and the cluster, apply the shared datastore tier
and the per-product services, and read their helm values back.

This command takes flags rather than subcommands. Run 'lerian infra --help' for
the full reference, 'lerian infra check' to verify this machine, and
'lerian infra init --env <env>' to write the configuration.`,

	// The infra command line predates this CLI and is parsed by its own flag
	// set, so cobra must hand the arguments over untouched — including -h, which
	// otherwise never reaches the reference text above.
	DisableFlagParsing: true,
	SilenceUsage:       true,
	SilenceErrors:      true,

	RunE: func(cmd *cobra.Command, args []string) error {
		return infracli.Run(cmd.Context(), args, cmd.OutOrStdout(), cmd.ErrOrStderr())
	},
}

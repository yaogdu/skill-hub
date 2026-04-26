package shub

import (
	"github.com/agentregistry-dev/agentregistry/internal/client"
	"github.com/spf13/cobra"
)

var apiClient *client.Client

func SetAPIClient(client *client.Client) {
	apiClient = client
}

var shubHome string

var ShubCmd = &cobra.Command{
	Use:   "shub",
	Short: "SHUB local asset distribution commands",
	Long:  `Commands for publishing, installing, selecting, and repairing SHUB-managed AI assets.`,
	Args:  cobra.ArbitraryArgs,
	Example: `arctl shub lint ./skills/java-analyzer
arctl shub package ./skills/java-analyzer
arctl shub deploy ./dist/java-analyzer-1.2.0.tar.gz
arctl shub add arch/java-analyzer
arctl shub use arch/java-analyzer@1.2.0
arctl shub doctor`,
}

func init() {
	ShubCmd.PersistentFlags().StringVar(&shubHome, "home", "", "Override SHUB home directory (defaults to $SHUB_HOME or ~/.shub)")
	ShubCmd.AddCommand(LintCmd)
	ShubCmd.AddCommand(PackageCmd)
	ShubCmd.AddCommand(DeployCmd)
	ShubCmd.AddCommand(AddCmd)
	ShubCmd.AddCommand(SourceCmd)
	ShubCmd.AddCommand(SearchCmd)
	ShubCmd.AddCommand(UseCmd)
	ShubCmd.AddCommand(SyncCmd)
	ShubCmd.AddCommand(DoctorCmd)
}

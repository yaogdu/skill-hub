package shub

import (
	"fmt"

	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	"github.com/spf13/cobra"
)

var SourceCmd = &cobra.Command{
	Use:   "source",
	Short: "Manage backend SHUB fallback sources",
}

var SourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured SHUB fallback sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireSHUBTokenForRegistryRead(); err != nil {
			return err
		}
		sources, err := apiClient.GetSHUBSources()
		if err != nil {
			return err
		}
		if len(sources) == 0 {
			printer.PrintInfo("No SHUB fallback sources configured")
			return nil
		}
		for _, source := range sources {
			if source == nil {
				continue
			}
			printer.PrintInfo(fmt.Sprintf("%s -> %s", source.Name, source.Address))
		}
		return nil
	},
}

var SourceSetCmd = &cobra.Command{
	Use:   "set <name> <address>",
	Short: "Create or update a SHUB fallback source",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireSHUBTokenForPublish(); err != nil {
			return err
		}
		source, err := apiClient.SetSHUBSource(args[0], args[1])
		if err != nil {
			return err
		}
		printer.PrintSuccess(fmt.Sprintf("Saved SHUB fallback source %s", source.Name))
		printer.PrintInfo(fmt.Sprintf("Address: %s", source.Address))
		return nil
	},
}

var SourceDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a SHUB fallback source",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireSHUBTokenForPublish(); err != nil {
			return err
		}
		if err := apiClient.DeleteSHUBSource(args[0]); err != nil {
			return err
		}
		printer.PrintSuccess(fmt.Sprintf("Deleted SHUB fallback source %s", args[0]))
		return nil
	},
}

func init() {
	SourceCmd.AddCommand(SourceListCmd)
	SourceCmd.AddCommand(SourceSetCmd)
	SourceCmd.AddCommand(SourceDeleteCmd)
}

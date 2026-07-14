package shub

import (
	"fmt"

	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	"github.com/spf13/cobra"
)

var SyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync installed SHUB assets with the latest registry metadata",
	Args:  cobra.NoArgs,
	RunE:  runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	if err := requireSHUBTokenForRegistryRead(); err != nil {
		return err
	}
	manager, err := NewManager(shubHome, apiClient, defaultSourceInstaller(), apiClient.BaseURL)
	if err != nil {
		return err
	}

	result, err := manager.Sync()
	if err != nil {
		return err
	}

	printer.PrintSuccess(fmt.Sprintf("Sync complete. Checked %d assets, installed %d new versions.", result.Checked, result.Installed))
	return nil
}

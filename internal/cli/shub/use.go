package shub

import (
	"fmt"

	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	"github.com/spf13/cobra"
)

var useVersion string

var UseCmd = &cobra.Command{
	Use:   "use <asset-id[@version]>",
	Short: "Switch the active installed version for a SHUB asset",
	Args:  cobra.ExactArgs(1),
	RunE:  runUse,
}

func init() {
	UseCmd.Flags().StringVar(&useVersion, "version", "", "Explicit version to activate")
}

func runUse(cmd *cobra.Command, args []string) error {
	manager, err := NewManager(shubHome, apiClient, DefaultSourceInstaller{}, baseURLFromClient())
	if err != nil {
		return err
	}

	installed, err := manager.Use(args[0], useVersion)
	if err != nil {
		return err
	}

	printer.PrintSuccess(fmt.Sprintf("Activated %s (%s)", installed.AssetID, installed.Version))
	for _, exportPath := range installed.ExportPaths {
		printer.PrintInfo(fmt.Sprintf("Export: %s", exportPath))
	}
	return nil
}

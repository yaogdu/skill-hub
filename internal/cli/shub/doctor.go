package shub

import (
	"fmt"

	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	"github.com/spf13/cobra"
)

var DoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose and repair local SHUB home state",
	Long: `Diagnose and repair local SHUB home state, including installed assets,
runtime environments, exported files, and SHUB-managed third-party tool configs
such as Claude Code and Aider workspace integration files.`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

func runDoctor(cmd *cobra.Command, args []string) error {
	manager, err := NewManager(shubHome, apiClient, defaultSourceInstaller(), baseURLFromClient())
	if err != nil {
		return err
	}

	result, err := manager.Doctor()
	if err != nil {
		return err
	}

	printer.PrintSuccess(fmt.Sprintf("Doctor complete. Checked %d installed assets, repaired %d issues.", result.Checked, result.Repaired))
	return nil
}

func baseURLFromClient() string {
	if apiClient == nil {
		return ""
	}
	return apiClient.BaseURL
}

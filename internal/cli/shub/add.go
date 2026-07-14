package shub

import (
	"fmt"

	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	"github.com/spf13/cobra"
)

var addVersion string
var addFallbackSources []string
var addGitHub bool

var AddCmd = &cobra.Command{
	Use:   "add <skill-name>",
	Short: "Install a registry skill into the local SHUB home",
	Long: `Resolve a skill from the registry, install it into ~/.shub/hub,
materialize a local runtime under ~/.shub/envs, and export prompt files into ~/.shub/exports.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func init() {
	AddCmd.Flags().StringVar(&addVersion, "version", "", "Specific version to install (defaults to latest)")
	AddCmd.Flags().StringArrayVar(&addFallbackSources, "fallback-source", nil, "If the registry misses, ask the server to pull from this named fallback source and mirror it into the registry")
	AddCmd.Flags().BoolVarP(&addGitHub, "github", "g", false, "If the registry misses, try the built-in GitHub fallback source pool and mirror the first resolved asset into the registry")
}

func runAdd(cmd *cobra.Command, args []string) error {
	if err := requireSHUBTokenForRegistryRead(); err != nil {
		return err
	}
	manager, err := NewManager(shubHome, apiClient, defaultSourceInstaller(), apiClient.BaseURL)
	if err != nil {
		return err
	}

	result, err := manager.AddWithOptions(args[0], AddOptions{
		Version:         addVersion,
		FallbackSources: addFallbackSources,
		GitHub:          addGitHub,
	})
	if err != nil {
		return err
	}

	printer.PrintSuccess(fmt.Sprintf("Installed %s (%s)", result.Asset.ID, result.Asset.Version))
	printer.PrintInfo(fmt.Sprintf("Hub path: %s", result.InstallDir))
	printer.PrintInfo(fmt.Sprintf("Env path: %s", result.EnvDir))
	for _, exportPath := range result.ExportPaths {
		printer.PrintInfo(fmt.Sprintf("Export: %s", exportPath))
	}
	return nil
}

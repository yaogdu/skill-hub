package shub

import (
	"fmt"
	"strings"

	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	"github.com/spf13/cobra"
)

var (
	searchLocalOnly bool
	searchOutput    string
)

var SearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search SHUB assets from the registry or local state",
	Args:  cobra.ExactArgs(1),
	RunE:  runSearch,
}

func init() {
	SearchCmd.Flags().BoolVar(&searchLocalOnly, "local-only", false, "Search only local installed assets")
	SearchCmd.Flags().StringVarP(&searchOutput, "output", "o", "text", "Output format (text, json)")
}

func runSearch(cmd *cobra.Command, args []string) error {
	if !searchLocalOnly {
		if err := requireSHUBTokenForRegistryRead(); err != nil {
			return err
		}
	}
	manager, err := NewManager(shubHome, apiClient, DefaultSourceInstaller{}, baseURLFromClient())
	if err != nil {
		return err
	}

	results, err := manager.Search(args[0], searchLocalOnly)
	if err != nil {
		return err
	}

	switch searchOutput {
	case "json":
		p := printer.New(printer.OutputTypeJSON, false)
		p.SetOutput(cmd.OutOrStdout())
		return p.PrintJSON(results)
	case "text", "":
		if len(results) == 0 {
			printer.PrintInfo("No matching assets found")
			return nil
		}
		for _, result := range results {
			installedMarker := ""
			if result.Installed {
				installedMarker = " [installed]"
			}
			printer.PrintInfo(fmt.Sprintf("- %s (%s)%s", result.DisplayID(), result.Version, installedMarker))
			if strings.TrimSpace(result.Description) != "" {
				printer.PrintInfo(fmt.Sprintf("  %s", result.Description))
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", searchOutput)
	}
}

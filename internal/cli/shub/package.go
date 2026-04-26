package shub

import (
	"fmt"
	"path/filepath"

	"github.com/agentregistry-dev/agentregistry/internal/cli/common"
	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
	"github.com/spf13/cobra"
)

var (
	packageFilePath string
	packageFormat   string
)

var PackageCmd = &cobra.Command{
	Use:   "package <asset-dir>",
	Short: "Build a SHUB package archive",
	Long: `Build a publishable .tar.gz artifact from a SKILL.md-rooted SHUB package.

The resulting archive preserves the package contents and embeds the derived
internal manifest under .shub/ for downstream deploy and indexing steps.`,
	Args:         cobra.ExactArgs(1),
	RunE:         runPackage,
	SilenceUsage: true,
	Example: `  arctl shub package ./my-skill
  arctl shub package ./my-skill --file ./dist/my-skill.tar.gz
  arctl shub package ./my-skill --format json`,
}

func init() {
	PackageCmd.Flags().StringVarP(&packageFilePath, "file", "f", "", "Output archive path (.tar.gz)")
	PackageCmd.Flags().StringVar(&packageFormat, "format", "text", "Output format (text, json)")
}

func runPackage(cmd *cobra.Command, args []string) error {
	assetDir := args[0]
	if err := common.ValidateProjectDir(assetDir); err != nil {
		return err
	}

	absPath, err := filepath.Abs(assetDir)
	if err != nil {
		return fmt.Errorf("failed to resolve path %q: %w", assetDir, err)
	}

	asset, err := shubskills.LoadAssetDir(absPath)
	if err != nil {
		return fmt.Errorf("load SHUB asset: %w", err)
	}

	outputPath := packageFilePath
	if outputPath == "" {
		outputPath = filepath.Join(absPath, "dist", fmt.Sprintf("%s-%s.tar.gz", flattenAssetID(asset.ID), asset.Version))
	}
	outputPath, err = filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}

	result, err := shubskills.BuildPackage(absPath, outputPath)
	if err != nil {
		return fmt.Errorf("build SHUB package: %w", err)
	}

	switch packageFormat {
	case "json":
		p := printer.New(printer.OutputTypeJSON, false)
		p.SetOutput(cmd.OutOrStdout())
		return p.PrintJSON(result)
	case "text", "":
		printer.PrintSuccess(fmt.Sprintf("Packaged %s (%s)", result.Asset.ID, result.Asset.Version))
		printer.PrintInfo(fmt.Sprintf("Archive: %s", result.OutputPath))
		printer.PrintInfo(fmt.Sprintf("SHA256: %s", result.SHA256))
		printer.PrintInfo(fmt.Sprintf("Files: %d", len(result.Files)))
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", packageFormat)
	}
}

package skill

import (
	"fmt"
	"path/filepath"

	"github.com/agentregistry-dev/agentregistry/internal/cli/common"
	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
	"github.com/spf13/cobra"
)

var lintOutputFormat string

var LintCmd = &cobra.Command{
	Use:   "lint <skill-folder-path>",
	Short: "Validate a SKILL.md package for SHUB compatibility",
	Long: `Validate a local skill package against the SHUB SKILL.md-first package model.

This command parses SKILL.md, validates the shub frontmatter extension,
checks package-relative file references, and emits the normalized asset manifest.`,
	Args:          cobra.ExactArgs(1),
	RunE:          runLint,
	SilenceUsage:  true,
	SilenceErrors: false,
	Example: `  arctl skill lint ./my-skill
  arctl skill lint ./my-skill --output json`,
}

func init() {
	LintCmd.Flags().StringVarP(&lintOutputFormat, "output", "o", "text", "Output format (text, json)")
}

func runLint(cmd *cobra.Command, args []string) error {
	skillDir := args[0]
	if err := common.ValidateProjectDir(skillDir); err != nil {
		return err
	}

	absPath, err := filepath.Abs(skillDir)
	if err != nil {
		return fmt.Errorf("failed to resolve path %q: %w", skillDir, err)
	}

	result, err := shubskills.ValidateDir(absPath)
	if err != nil {
		return fmt.Errorf("skill validation failed: %w", err)
	}

	switch lintOutputFormat {
	case "text", "":
		printer.PrintSuccess(fmt.Sprintf("SKILL.md is valid for SHUB: %s (%s)", result.Asset.ID, common.FormatVersionForDisplay(result.Asset.Version)))
		printer.PrintInfo(fmt.Sprintf("Category: %s", result.Asset.Category))
		printer.PrintInfo(fmt.Sprintf("Entry: %s -> %s", result.Asset.Manifest.Entry.Kind, result.Asset.Manifest.Entry.Path))
		return nil
	case "json":
		p := printer.New(printer.OutputTypeJSON, false)
		p.SetOutput(cmd.OutOrStdout())
		return p.PrintJSON(result.Asset.Manifest)
	default:
		return fmt.Errorf("unsupported output format: %s", lintOutputFormat)
	}
}

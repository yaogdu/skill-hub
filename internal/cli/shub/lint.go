package shub

import (
	"fmt"
	"path/filepath"

	"github.com/agentregistry-dev/agentregistry/internal/cli/common"
	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
	"github.com/spf13/cobra"
)

var lintFormat string

var LintCmd = &cobra.Command{
	Use:   "lint <asset-dir>",
	Short: "Validate a SHUB SKILL.md package",
	Long: `Validate a local SHUB package rooted by SKILL.md.

This command parses SKILL.md, validates the shub frontmatter extension,
checks package-relative references, and emits the normalized derived manifest.`,
	Args:         cobra.ExactArgs(1),
	RunE:         runLint,
	SilenceUsage: true,
	Example: `  arctl shub lint ./my-skill
  arctl shub lint ./my-skill --output json`,
}

func init() {
	LintCmd.Flags().StringVarP(&lintFormat, "output", "o", "text", "Output format (text, json)")
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

	switch lintFormat {
	case "text", "":
		printer.PrintSuccess(fmt.Sprintf("SKILL.md is valid for SHUB: %s (%s)", result.Asset.ID, result.Asset.Version))
		printer.PrintInfo(fmt.Sprintf("Category: %s", result.Asset.Category))
		printer.PrintInfo(fmt.Sprintf("Entry: %s -> %s", result.Asset.Manifest.Entry.Kind, result.Asset.Manifest.Entry.Path))
		printLintAuditFindings(result.Audit)
		return nil
	case "json":
		p := printer.New(printer.OutputTypeJSON, false)
		p.SetOutput(cmd.OutOrStdout())
		return p.PrintJSON(result)
	default:
		return fmt.Errorf("unsupported output format: %s", lintFormat)
	}
}

func printLintAuditFindings(report *shubskills.AuditReport) {
	if report == nil || len(report.Findings) == 0 {
		printer.PrintInfo("Static audit: clean")
		return
	}

	printer.PrintInfo(fmt.Sprintf("Static audit warnings: %d", report.WarningCount()))
	for _, finding := range report.Findings {
		printer.PrintInfo(fmt.Sprintf("- [%s] %s: %s", finding.Severity, finding.Rule, finding.Message))
	}
}

package shub

import (
	"fmt"

	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
	"github.com/spf13/cobra"
)

var (
	deployGitRepository string
	deployGitProvider   string
	deployDockerImage   string
	deployPackageURL    string
	deployDryRun        bool
	deployOutput        string
)

var DeployCmd = &cobra.Command{
	Use:   "deploy <asset-dir-or-package>",
	Short: "Publish a SHUB asset through the compatibility registry API",
	Long: `Publish a SHUB asset derived from SKILL.md.

Today this command bridges the new SHUB package model onto the existing skill API,
so teams can use shub lint/package/deploy now while the Hub converges on the
unified asset API described in the PRD.`,
	Args:         cobra.ExactArgs(1),
	RunE:         runDeploy,
	SilenceUsage: true,
	Example: `  arctl shub deploy ./my-skill --package-url https://gitlab.example.com/pkg/my-skill-1.0.0.tar.gz
  arctl shub deploy ./dist/my-skill-1.0.0.tar.gz
  arctl shub deploy ./my-skill --git https://gitlab.com/acme/my-skill --dry-run --output json`,
}

func init() {
	DeployCmd.Flags().StringVar(&deployGitRepository, "git", "", "Git repository URL to publish alongside the SHUB asset")
	DeployCmd.Flags().StringVar(&deployGitProvider, "git-provider", "", "Optional provider hint for ambiguous self-hosted git hosts: github, gitlab, or bitbucket")
	DeployCmd.Flags().StringVar(&deployDockerImage, "docker-image", "", "Docker image reference to publish alongside the SHUB asset")
	DeployCmd.Flags().StringVar(&deployPackageURL, "package-url", "", "Package archive URL (http(s):// or file://) for tarball-based installs")
	DeployCmd.Flags().BoolVar(&deployDryRun, "dry-run", false, "Render the compatibility payload without publishing it")
	DeployCmd.Flags().StringVarP(&deployOutput, "output", "o", "text", "Output format (text, json)")
}

func runDeploy(cmd *cobra.Command, args []string) error {
	if err := requireSHUBTokenForPublish(); err != nil {
		return err
	}
	result, err := DeployAsset(args[0], apiClient, DeployOptions{
		GitRepository: deployGitRepository,
		GitProvider:   deployGitProvider,
		DockerImage:   deployDockerImage,
		PackageURL:    deployPackageURL,
		DryRun:        deployDryRun,
	})
	if err != nil {
		return err
	}

	switch deployOutput {
	case "json":
		p := printer.New(printer.OutputTypeJSON, false)
		p.SetOutput(cmd.OutOrStdout())
		return p.PrintJSON(result)
	case "text", "":
		if deployDryRun {
			printer.PrintSuccess(fmt.Sprintf("Prepared SHUB deploy payload for %s (%s)", result.Asset.ID, result.Asset.Version))
		} else {
			printer.PrintSuccess(fmt.Sprintf("Published %s (%s)", result.Asset.ID, result.Asset.Version))
		}
		if result.PackageURL != "" {
			printer.PrintInfo(fmt.Sprintf("Package URL: %s", result.PackageURL))
		}
		if result.Payload.Repository != nil {
			printer.PrintInfo(fmt.Sprintf("Repository: %s", result.Payload.Repository.URL))
		}
		if len(result.Payload.Packages) > 0 {
			printer.PrintInfo(fmt.Sprintf("Packages: %d", len(result.Payload.Packages)))
		}
		printDeployAuditFindings(result.Audit)
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", deployOutput)
	}
}

func printDeployAuditFindings(report *shubskills.AuditReport) {
	if report == nil || len(report.Findings) == 0 {
		printer.PrintInfo("Static audit: clean")
		return
	}

	printer.PrintInfo(fmt.Sprintf("Static audit warnings: %d", report.WarningCount()))
	for _, finding := range report.Findings {
		printer.PrintInfo(fmt.Sprintf("- [%s] %s: %s", finding.Severity, finding.Rule, finding.Message))
	}
}

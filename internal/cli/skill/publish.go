package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentregistry-dev/agentregistry/internal/cli/common"
	"github.com/agentregistry-dev/agentregistry/internal/cli/common/gitutil"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
	"github.com/spf13/cobra"
)

var (
	// Flags for skill publish command
	versionFlag     string
	dryRunFlag      bool
	gitRepository   string
	gitProviderFlag string
	dockerImageFlag string
	publishDesc     string
)

var PublishCmd = &cobra.Command{
	Use:   "publish <skill-name|skill-folder-path>",
	Short: "Publish a skill to the registry",
	Long: `Publish a skill to the agent registry.

This command supports three modes:

1. From a local skill folder (with SKILL.md):
   arctl skill publish ./my-skill --git https://github.com/org/repo --version 1.0.0
   arctl skill publish ./my-skill --docker-image docker.io/myorg/my-skill:v1.0.0 --version 1.0.0

2. Direct registration with Git repository:
   arctl skill publish my-skill \
     --git https://github.com/org/repo/tree/main/skills/my-skill \
     --version 1.0.0 \
     --description "My remote skill"

3. Direct registration with a pre-built Docker image:
   arctl skill publish my-skill \
     --docker-image docker.io/myorg/my-skill:v1.0.0 \
     --version 1.0.0 \
     --description "My Docker skill"

For Git modes, --git accepts GitHub, GitLab, and Bitbucket repository/tree URLs, including compatible self-hosted instances.
In folder mode, the local skill folder must contain a SKILL.md file with proper YAML frontmatter.
In direct mode, the referenced Git path is validated structurally and should contain SKILL.md.

To build a skill as a Docker image, use "arctl skill build" instead.`,
	Args: cobra.ExactArgs(1),
	RunE: runPublish,
}

func init() {
	// Common flags
	PublishCmd.Flags().StringVar(&versionFlag, "version", "", "Version to publish (required for --git or --docker-image)")
	PublishCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Show what would be done without actually doing it")
	PublishCmd.Flags().StringVar(&publishDesc, "description", "", "Skill description (optional, used with direct registration)")
	PublishCmd.Flags().StringVar(&gitRepository, "git", "", "Git repository URL (alternative to --docker-image). Supports GitHub/GitLab/Bitbucket and compatible self-hosted tree URLs")
	PublishCmd.Flags().StringVar(&gitProviderFlag, "git-provider", "", "Optional provider hint for ambiguous self-hosted git hosts: github, gitlab, or bitbucket")

	// Docker-only flags
	PublishCmd.Flags().StringVar(&dockerImageFlag, "docker-image", "", "Docker image URL. For example: docker.io/myorg/my-skill:v1.0.0")

	PublishCmd.MarkFlagsOneRequired("git", "docker-image")
}

func runPublish(cmd *cobra.Command, args []string) error {
	input := args[0]

	if apiClient == nil {
		return fmt.Errorf("API client not initialized")
	}

	// Detect whether input is a skill folder or a skill name.
	// If it's a directory that contains (or has subdirectories with) SKILL.md, use folder mode.
	// Otherwise, treat it as a skill name for direct registration.
	absPath, err := filepath.Abs(input)
	if err != nil {
		return fmt.Errorf("failed to resolve path %q: %w", input, err)
	}
	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		isValid := isValidSkillDir(absPath)
		if !isValid {
			return fmt.Errorf("no valid skills found at path: %s", absPath)
		}
		return runPublishFromFolder(absPath)
	}

	return runPublishDirect(input)
}

// runPublishFromFolder publishes the pre-detected skills from the given directory.
func runPublishFromFolder(skillFolderPath string) error {
	printer.PrintInfo(fmt.Sprintf("Publishing skill from: %s", skillFolderPath))

	var skillJson *models.SkillJSON
	var err error
	switch {
	case gitRepository != "":
		skillJson, err = buildSkillFromGit(skillFolderPath)
	case dockerImageFlag != "":
		skillJson, err = buildSkillFromDocker(skillFolderPath)
	default:
		return fmt.Errorf("--git or --docker-image is required")
	}
	if err != nil {
		return fmt.Errorf("failed to build skill '%s': %w", skillFolderPath, err)
	}

	if err := publishSkillJSON(skillJson); err != nil {
		return err
	}

	return nil
}

// runPublishDirect publishes a skill by name using --git or --docker-image flags
// without requiring a local SKILL.md.
func runPublishDirect(skillName string) error {
	var skillJson *models.SkillJSON
	var err error

	switch {
	case gitRepository != "":
		skillJson, err = buildSkillDirectGit(skillName)
	case dockerImageFlag != "":
		skillJson, err = buildSkillDirectDocker(skillName)
	default:
		return fmt.Errorf("--git or --docker-image is required")
	}
	if err != nil {
		return err
	}

	if err := publishSkillJSON(skillJson); err != nil {
		return err
	}

	if !dryRunFlag {
		printer.PrintSuccess(fmt.Sprintf("Published: %s (%s)", skillJson.Name, common.FormatVersionForDisplay(skillJson.Version)))
	}

	return nil
}

// publishSkillJSON publishes or dry-runs a single SkillJSON.
func publishSkillJSON(skillJson *models.SkillJSON) error {
	if dryRunFlag {
		j, _ := json.Marshal(skillJson)
		printer.PrintInfo("[DRY RUN] Would publish skill to registry " + apiClient.BaseURL + ": " + string(j))
		return nil
	}

	_, err := apiClient.CreateSkill(skillJson)
	if err != nil {
		return fmt.Errorf("failed to publish skill '%s': %w", skillJson.Name, err)
	}
	return nil
}

// buildSkillDirectGit builds SkillJSON from --git flags without a local SKILL.md.
func buildSkillDirectGit(skillName string) (*models.SkillJSON, error) {
	skillName = strings.ToLower(skillName)

	if gitRepository == "" {
		return nil, fmt.Errorf("--git is required when publishing without SKILL.md")
	}
	if versionFlag == "" {
		return nil, fmt.Errorf("--version is required when publishing without SKILL.md")
	}

	if err := validateGitRepository(gitRepository); err != nil {
		return nil, fmt.Errorf("--git validation failed: %w", err)
	}

	return &models.SkillJSON{
		Name:        skillName,
		Description: publishDesc,
		Version:     versionFlag,
		Repository: &models.SkillRepository{
			URL:    gitRepository,
			Source: "git",
		},
	}, nil
}

// buildSkillDirectDocker builds SkillJSON from --docker-image flags without a local SKILL.md.
func buildSkillDirectDocker(skillName string) (*models.SkillJSON, error) {
	skillName = strings.ToLower(skillName)

	if dockerImageFlag == "" {
		return nil, fmt.Errorf("--docker-image is required")
	}
	if versionFlag == "" {
		return nil, fmt.Errorf("--version is required when publishing with --docker-image")
	}

	skill := &models.SkillJSON{
		Name:        skillName,
		Description: publishDesc,
		Version:     versionFlag,
	}

	pkg := models.SkillPackageInfo{
		RegistryType: "docker",
		Identifier:   dockerImageFlag,
		Version:      versionFlag,
	}
	pkg.Transport.Type = "docker"
	skill.Packages = append(skill.Packages, pkg)

	return skill, nil
}

type skillFrontmatter struct {
	Name        string
	Description string
}

// parseSkillFrontmatter reads and parses the YAML frontmatter from a SKILL.md file.
func parseSkillFrontmatter(skillPath string) (*skillFrontmatter, error) {
	document, err := shubskills.ParseDir(skillPath)
	if err != nil {
		return nil, err
	}

	return &skillFrontmatter{
		Name:        document.Frontmatter.Name,
		Description: document.Frontmatter.Description,
	}, nil
}

// resolveSkillMeta parses SKILL.md frontmatter and returns the skill name and description.
func resolveSkillMeta(skillPath string) (name, description string, err error) {
	fm, err := parseSkillFrontmatter(skillPath)
	if err != nil {
		return "", "", err
	}
	return fm.Name, fm.Description, nil
}

// resolveGitVersion returns the version for a git-based publish.
// Requires --version to be set.
func resolveGitVersion() (string, error) {
	if versionFlag == "" {
		return "", fmt.Errorf("--version is required when publishing with --git")
	}
	return versionFlag, nil
}

// validateGitRepository checks that the provided git repository URL uses a
// supported provider and has the required repository/tree shape.
func validateGitRepository(rawURL string) error {
	_, _, _, err := gitutil.ParseGitURLWithProvider(rawURL, gitProviderFlag)
	if err != nil {
		return err
	}
	return nil
}

// buildSkillFromGit reads SKILL.md frontmatter and registers the skill with a git repository.
func buildSkillFromGit(skillPath string) (*models.SkillJSON, error) {
	name, description, err := resolveSkillMeta(skillPath)
	if err != nil {
		return nil, err
	}

	ver, err := resolveGitVersion()
	if err != nil {
		return nil, err
	}

	if err := validateGitRepository(gitRepository); err != nil {
		return nil, fmt.Errorf("--git validation failed: %w", err)
	}

	skill := &models.SkillJSON{
		Name:        name,
		Description: description,
		Version:     ver,
		Repository: &models.SkillRepository{
			URL:    gitRepository,
			Source: "git",
		},
	}

	return skill, nil
}

// buildSkillFromDocker reads SKILL.md frontmatter and registers the skill with a Docker image reference.
func buildSkillFromDocker(skillPath string) (*models.SkillJSON, error) {
	name, description, err := resolveSkillMeta(skillPath)
	if err != nil {
		return nil, err
	}

	if versionFlag == "" {
		return nil, fmt.Errorf("--version is required when publishing with --docker-image")
	}

	skill := &models.SkillJSON{
		Name:        name,
		Description: description,
		Version:     versionFlag,
	}

	pkg := models.SkillPackageInfo{
		RegistryType: "docker",
		Identifier:   dockerImageFlag,
		Version:      versionFlag,
	}
	pkg.Transport.Type = "docker"
	skill.Packages = append(skill.Packages, pkg)

	return skill, nil
}

// isValidSkillDir checks whether a directory contains a SKILL.md with valid YAML frontmatter.
func isValidSkillDir(dir string) bool {
	if !hasSkillMd(dir) {
		return false
	}
	_, err := parseSkillFrontmatter(dir)
	return err == nil
}

// hasSkillMd checks whether a directory contains a SKILL.md file.
func hasSkillMd(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	return err == nil
}

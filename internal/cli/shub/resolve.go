package shub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentregistry-dev/agentregistry/internal/cli/common"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/printer"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
	"github.com/spf13/cobra"
)

const lockfileVersion = 1

var (
	resolveOutputPath string
	resolveFormat     string
	resolveCheckOnly  bool
)

var ResolveCmd = &cobra.Command{
	Use:   "resolve <asset-dir>",
	Short: "Resolve SHUB asset dependencies into a lockfile",
	Long: `Resolve dependencies declared under shub.dependencies in SKILL.md.

The command writes a shub.lock file with exact asset versions, categories,
package references, digests when available, and source commits when available.
Dependency lookup uses the same registry target as other SHUB commands:
--registry-url / ARCTL_API_BASE_URL / SHUB_API_BASE_URL plus
--registry-token / ARCTL_API_TOKEN / SHUB_API_TOKEN for authentication.`,
	Args:         cobra.ExactArgs(1),
	RunE:         runResolve,
	SilenceUsage: true,
	Example: `  arctl shub resolve ./my-agent
  arctl shub resolve ./my-agent --output shub.lock
  arctl shub resolve ./my-agent --check`,
}

func init() {
	ResolveCmd.Flags().StringVarP(&resolveOutputPath, "output", "o", "shub.lock", "Lockfile output path")
	ResolveCmd.Flags().StringVar(&resolveFormat, "format", "text", "Output format (text, json)")
	ResolveCmd.Flags().BoolVar(&resolveCheckOnly, "check", false, "Fail if the existing lockfile differs from the resolved result")
}

func runResolve(cmd *cobra.Command, args []string) error {
	if err := requireSHUBTokenForRegistryRead(); err != nil {
		return err
	}
	if err := common.ValidateProjectDir(args[0]); err != nil {
		return err
	}
	absPath, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve asset directory: %w", err)
	}
	manager, err := NewManager(shubHome, apiClient, DefaultSourceInstaller{}, baseURLFromClient())
	if err != nil {
		return err
	}
	lock, err := manager.Resolve(absPath)
	if err != nil {
		return err
	}

	outputPath := resolveOutputPath
	if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(absPath, outputPath)
	}
	if resolveCheckOnly {
		if err := checkLockfile(outputPath, lock); err != nil {
			return err
		}
		printer.PrintSuccess("shub.lock is up to date")
		return nil
	}
	if err := writeLockfile(outputPath, lock); err != nil {
		return err
	}

	switch resolveFormat {
	case "json":
		p := printer.New(printer.OutputTypeJSON, false)
		p.SetOutput(cmd.OutOrStdout())
		return p.PrintJSON(lock)
	case "text", "":
		printer.PrintSuccess(fmt.Sprintf("Resolved %d dependencies into %s", len(lock.ResolvedAssets), outputPath))
		for _, dep := range lock.ResolvedAssets {
			printer.PrintInfo(fmt.Sprintf("- %s@%s (%s)", dep.ID, dep.Version, dep.Category))
		}
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", resolveFormat)
	}
}

type Lockfile struct {
	LockfileVersion int                 `json:"lockfileVersion"`
	Asset           LockfileRootAsset   `json:"asset"`
	GeneratedAt     time.Time           `json:"generatedAt"`
	ResolvedAssets  []ResolvedLockAsset `json:"resolvedAssets"`
}

type LockfileRootAsset struct {
	ID       string               `json:"id"`
	Version  string               `json:"version"`
	Category models.AssetCategory `json:"category"`
}

type ResolvedLockAsset struct {
	ID           string               `json:"id"`
	Version      string               `json:"version"`
	Category     models.AssetCategory `json:"category"`
	Name         string               `json:"name,omitempty"`
	Digest       string               `json:"digest,omitempty"`
	SourceCommit string               `json:"sourceCommit,omitempty"`
	PackageRef   string               `json:"packageRef,omitempty"`
}

func (manager *Manager) Resolve(assetDir string) (*Lockfile, error) {
	asset, err := shubskills.LoadAssetDir(assetDir)
	if err != nil {
		return nil, fmt.Errorf("load asset package: %w", err)
	}
	resolver := manager.assetRegistry()
	if resolver == nil {
		return nil, fmt.Errorf("registry client does not support SHUB assets")
	}

	refs := flattenDependencies(asset.Manifest.Dependencies)
	resolved := make([]ResolvedLockAsset, 0, len(refs))
	for _, ref := range refs {
		assetResp, err := resolver.GetAssetVersion(ref.ID, ref.Version)
		if err != nil {
			return nil, fmt.Errorf("resolve dependency %s@%s: %w", ref.ID, ref.Version, err)
		}
		if assetResp == nil {
			return nil, fmt.Errorf("dependency %s@%s not found", ref.ID, ref.Version)
		}
		if ref.Category.IsValid() && assetResp.Asset.Category != ref.Category {
			return nil, fmt.Errorf("dependency %s@%s category mismatch: manifest requires %s, registry returned %s", ref.ID, ref.Version, ref.Category, assetResp.Asset.Category)
		}
		resolved = append(resolved, lockAssetFromResponse(assetResp))
	}
	sort.Slice(resolved, func(i, j int) bool {
		if resolved[i].ID == resolved[j].ID {
			return resolved[i].Version < resolved[j].Version
		}
		return resolved[i].ID < resolved[j].ID
	})

	return &Lockfile{
		LockfileVersion: lockfileVersion,
		Asset: LockfileRootAsset{
			ID:       asset.ID,
			Version:  asset.Version,
			Category: asset.Category,
		},
		GeneratedAt:    time.Now().UTC(),
		ResolvedAssets: resolved,
	}, nil
}

func flattenDependencies(dependencies models.AssetDependencies) []models.AssetDependencyRef {
	type dependencyGroup struct {
		category models.AssetCategory
		refs     []models.AssetDependencyRef
	}
	groups := []dependencyGroup{
		{category: models.AssetCategoryPrompt, refs: dependencies.Prompts},
		{refs: dependencies.Skills},
		{category: models.AssetCategoryMCP, refs: dependencies.MCPs},
		{category: models.AssetCategoryAgent, refs: dependencies.Agents},
	}
	refs := make([]models.AssetDependencyRef, 0)
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, ref := range group.refs {
			if strings.TrimSpace(ref.ID) == "" || strings.TrimSpace(ref.Version) == "" {
				continue
			}
			if !ref.Category.IsValid() {
				ref.Category = group.category
			}
			key := strings.TrimSpace(ref.ID) + "@" + strings.TrimSpace(ref.Version)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			refs = append(refs, ref)
		}
	}
	return refs
}

func lockAssetFromResponse(response *models.AssetResponse) ResolvedLockAsset {
	resolved := ResolvedLockAsset{
		ID:       response.Asset.ID,
		Version:  response.Asset.Version,
		Category: response.Asset.Category,
		Name:     response.Asset.Name,
	}
	if response.Asset.Source != nil {
		resolved.SourceCommit = response.Asset.Source.Commit
		resolved.PackageRef = response.Asset.Source.PackageRef
	}
	if response.Asset.Source != nil && strings.EqualFold(strings.TrimSpace(response.Asset.Source.PackageType), "sha256") {
		resolved.Digest = strings.TrimSpace(response.Asset.Source.PackageRef)
	}
	return resolved
}

func writeLockfile(path string, lock *Lockfile) error {
	payload, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lockfile: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create lockfile directory: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write lockfile: %w", err)
	}
	return nil
}

func checkLockfile(path string, lock *Lockfile) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read existing lockfile: %w", err)
	}
	var existing Lockfile
	if err := json.Unmarshal(current, &existing); err != nil {
		return fmt.Errorf("parse existing lockfile: %w", err)
	}
	existing.GeneratedAt = time.Time{}
	expected := *lock
	expected.GeneratedAt = time.Time{}
	if !equalLockfiles(existing, expected) {
		return fmt.Errorf("shub.lock is out of date; run shub resolve")
	}
	return nil
}

func equalLockfiles(a, b Lockfile) bool {
	aBytes, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bBytes, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aBytes) == string(bBytes)
}

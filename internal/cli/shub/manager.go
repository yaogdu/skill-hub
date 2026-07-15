package shub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentregistry-dev/agentregistry/internal/cli/common/docker"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
	"golang.org/x/mod/semver"
	yaml "gopkg.in/yaml.v3"
)

const (
	defaultHomeDirName = ".shub"
	stateVersion       = 1
	claudeMCPPrefix    = "shub__"
)

type SkillRegistry interface {
	GetSkill(name string) (*models.SkillResponse, error)
	GetSkillVersion(name, version string) (*models.SkillResponse, error)
	GetSkillVersions(name string) ([]*models.SkillResponse, error)
	GetSkills() ([]*models.SkillResponse, error)
}

type AssetRegistry interface {
	GetAsset(id string) (*models.AssetResponse, error)
	GetAssetVersion(id, version string) (*models.AssetResponse, error)
	GetAssetVersions(id string) ([]*models.AssetResponse, error)
	GetAssets() ([]*models.AssetResponse, error)
}

type IncrementalAssetRegistry interface {
	ListAssetsUpdatedSince(updatedSince, cursor string, limit int) ([]*models.AssetResponse, string, error)
}

type SourceInstaller interface {
	Install(skill *models.SkillResponse, targetDir string) error
}

type AssetSourcePuller interface {
	PullAssetFromSource(sourceName, assetID, version string) (*models.AssetResponse, error)
}

type SHUBSourceRegistry interface {
	GetSHUBSources() ([]*models.SHUBSource, error)
}

const (
	fallbackSourceGitHubDirect     = "github-direct"
	fallbackSourceGitHubSkillsMain = "github-skills-main"
	fallbackSourceGitHubPluginMain = "github-plugin-skills-main"
	fallbackSourceOpenAISkills     = "openai-skills"
	fallbackSourceAnthropicSkills  = "anthropic-skills"
)

var defaultFallbackSourcePool = []string{
	fallbackSourceGitHubDirect,
	fallbackSourceGitHubSkillsMain,
	fallbackSourceGitHubPluginMain,
	fallbackSourceOpenAISkills,
	fallbackSourceAnthropicSkills,
}

type Manager struct {
	homeRoot  string
	registry  SkillRegistry
	installer SourceInstaller
	baseURL   string
}

type Config struct {
	RegistryURL string `json:"registryUrl,omitempty"`
}

type State struct {
	Version   int               `json:"version"`
	Installed []InstalledAsset  `json:"installed"`
	Active    map[string]string `json:"active"`
	Sync      SyncState         `json:"sync,omitempty"`
}

type SyncState struct {
	Cursor       string    `json:"cursor,omitempty"`
	LastSyncedAt time.Time `json:"lastSyncedAt,omitempty"`
}

type InstalledAsset struct {
	RegistryName string         `json:"registryName"`
	AssetID      string         `json:"assetId"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	SearchText   string         `json:"searchText,omitempty"`
	Version      string         `json:"version"`
	Category     string         `json:"category"`
	RegistryHost string         `json:"registryHost"`
	InstallDir   string         `json:"installDir"`
	EnvDir       string         `json:"envDir"`
	ExportPaths  []string       `json:"exportPaths,omitempty"`
	Runtime      RuntimeStatus  `json:"runtime,omitempty"`
	Exports      []ExportRecord `json:"exports,omitempty"`
	InstalledAt  time.Time      `json:"installedAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

type RuntimeStatus struct {
	Type         string    `json:"type,omitempty"`
	Status       string    `json:"status,omitempty"`
	CheckedAt    time.Time `json:"checkedAt,omitempty"`
	MetadataPath string    `json:"metadataPath,omitempty"`
}

type ExportMetadata struct {
	Exports []ExportRecord `json:"exports"`
}

type ExportRecord struct {
	AssetID    string    `json:"assetId"`
	Version    string    `json:"version"`
	Target     string    `json:"target"`
	Mode       string    `json:"mode"`
	ExportPath string    `json:"exportPath"`
	SourcePath string    `json:"sourcePath"`
	Status     string    `json:"status,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt,omitempty"`
}

type AddResult struct {
	Asset       *models.Asset
	InstallDir  string
	EnvDir      string
	ExportPaths []string
}

type AddOptions struct {
	Version         string
	FallbackSources []string
	GitHub          bool
}

type DoctorResult struct {
	Checked  int `json:"checked"`
	Repaired int `json:"repaired"`
}

type SyncResult struct {
	Checked   int `json:"checked"`
	Installed int `json:"installed"`
}

type SearchResult struct {
	AssetID      string `json:"assetId,omitempty"`
	RegistryName string `json:"registryName,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Version      string `json:"version"`
	Installed    bool   `json:"installed"`
}

func (result SearchResult) DisplayID() string {
	if strings.TrimSpace(result.AssetID) != "" {
		return result.AssetID
	}
	return result.RegistryName
}

func NewManager(homeRoot string, registry SkillRegistry, installer SourceInstaller, baseURL string) (*Manager, error) {
	resolved, err := resolveHomeRoot(homeRoot)
	if err != nil {
		return nil, err
	}
	if installer == nil {
		installer = DefaultSourceInstaller{}
	}
	return &Manager{homeRoot: resolved, registry: registry, installer: installer, baseURL: baseURL}, nil
}

func (manager *Manager) Add(skillName, version string) (*AddResult, error) {
	return manager.AddWithOptions(skillName, AddOptions{Version: version})
}

func (manager *Manager) AddWithOptions(skillName string, opts AddOptions) (*AddResult, error) {
	var result *AddResult
	err := manager.withStateLock(func() error {
		added, err := manager.addLocked(skillName, opts)
		if err != nil {
			return err
		}
		result = added
		return nil
	})
	return result, err
}

func (manager *Manager) addLocked(skillName string, opts AddOptions) (*AddResult, error) {
	if manager.registry == nil {
		return nil, fmt.Errorf("registry client not configured")
	}
	if strings.TrimSpace(skillName) == "" {
		return nil, fmt.Errorf("skill name is required")
	}
	if err := manager.ensureLayout(); err != nil {
		return nil, err
	}

	registryName, skillResp, err := manager.resolveRemoteInstallSource(skillName, opts)
	if err != nil {
		return nil, err
	}
	if skillResp == nil {
		return nil, fmt.Errorf("asset or skill '%s' not found", skillName)
	}

	stagingDir, err := os.MkdirTemp("", "shub-stage-*")
	if err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	if err := manager.installer.Install(skillResp, stagingDir); err != nil {
		return nil, fmt.Errorf("install skill source: %w", err)
	}

	asset, err := shubskills.LoadAssetDir(stagingDir)
	if err != nil {
		return nil, fmt.Errorf("validate installed skill package: %w", err)
	}

	installDir := manager.installDirForAsset(asset)
	if err := ensureParentDir(installDir); err != nil {
		return nil, err
	}
	if _, err := os.Stat(installDir); err == nil {
		asset, err = shubskills.LoadAssetDir(installDir)
		if err != nil {
			return nil, fmt.Errorf("existing install at %s is invalid: %w", installDir, err)
		}
	} else {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat install dir: %w", err)
		}
		if err := os.Rename(stagingDir, installDir); err != nil {
			return nil, fmt.Errorf("move staged package into hub: %w", err)
		}
	}

	envDir, runtimeStatus, err := manager.materializeRuntime(asset, installDir)
	if err != nil {
		return nil, err
	}
	exportPaths, exportRecords, err := manager.writeExports(asset, installDir)
	if err != nil {
		return nil, err
	}
	if err := manager.updateState(registryName, asset, installDir, envDir, exportPaths, runtimeStatus, exportRecords); err != nil {
		return nil, err
	}
	if _, err := manager.refreshManagedClientConfigsLocked(); err != nil {
		return nil, err
	}
	if err := manager.updateConfig(); err != nil {
		return nil, err
	}

	return &AddResult{Asset: asset, InstallDir: installDir, EnvDir: envDir, ExportPaths: exportPaths}, nil
}

func (manager *Manager) Use(reference, version string) (*InstalledAsset, error) {
	var installed *InstalledAsset
	err := manager.withStateLock(func() error {
		current, err := manager.useLocked(reference, version)
		if err != nil {
			return err
		}
		installed = current
		return nil
	})
	return installed, err
}

func (manager *Manager) useLocked(reference, version string) (*InstalledAsset, error) {
	if err := manager.ensureLayout(); err != nil {
		return nil, err
	}
	state, err := manager.loadState()
	if err != nil {
		return nil, err
	}

	assetID, resolvedVersion := parseAssetRef(reference, version)
	installed := findInstalledAsset(state, assetID, resolvedVersion)
	if installed == nil {
		return nil, fmt.Errorf("asset '%s' version '%s' is not installed", assetID, resolvedVersion)
	}

	asset, err := manager.loadOrRepairInstalledAssetLocked(installed)
	if err != nil {
		return nil, fmt.Errorf("load installed asset: %w", err)
	}
	paths, exportRecords, err := manager.writeExports(asset, installed.InstallDir)
	if err != nil {
		return nil, err
	}
	_, runtimeStatus, err := manager.materializeRuntime(asset, installed.InstallDir)
	if err != nil {
		return nil, err
	}
	installed.ExportPaths = paths
	installed.Exports = exportRecords
	installed.Runtime = runtimeStatus
	updateInstalledSummary(installed, asset)
	installed.UpdatedAt = time.Now().UTC()
	if state.Active == nil {
		state.Active = make(map[string]string)
	}
	state.Active[installed.AssetID] = installed.Version
	if err := manager.saveState(state); err != nil {
		return nil, err
	}
	if _, err := manager.refreshManagedClientConfigsLocked(); err != nil {
		return nil, err
	}
	return installed, nil
}

func (manager *Manager) Search(query string, localOnly bool) ([]SearchResult, error) {
	if err := manager.ensureLayout(); err != nil {
		return nil, err
	}
	state, err := manager.loadState()
	if err != nil {
		return nil, err
	}

	if localOnly || manager.registry == nil {
		return manager.searchLocal(state, query)
	}

	if assetRegistry := manager.assetRegistry(); assetRegistry != nil {
		assets, err := assetRegistry.GetAssets()
		if err == nil && len(assets) > 0 {
			return manager.searchRemoteAssets(state, assets, query), nil
		}
	}

	skills, err := manager.registry.GetSkills()
	if err != nil {
		return manager.searchLocal(state, query)
	}
	if len(skills) == 0 {
		return manager.searchLocal(state, query)
	}

	results := make([]SearchResult, 0)
	needle := strings.ToLower(strings.TrimSpace(query))
	for _, skill := range skills {
		if skill == nil {
			continue
		}
		if !matchesSearch(needle, skill.Skill.Name, skill.Skill.Description) {
			continue
		}
		installed := findInstalledByRegistryName(state, skill.Skill.Name, "")
		result := SearchResult{
			RegistryName: skill.Skill.Name,
			Name:         skill.Skill.Name,
			Description:  skill.Skill.Description,
			Version:      skill.Skill.Version,
			Installed:    installed != nil,
		}
		if installed != nil {
			result.AssetID = installed.AssetID
		}
		results = append(results, result)
	}
	return results, nil
}

func (manager *Manager) searchLocal(state *State, query string) ([]SearchResult, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	results := make([]SearchResult, 0)
	seen := make(map[string]struct{})
	for _, installed := range state.Installed {
		key := installed.AssetID + "@" + installed.Version
		if _, ok := seen[key]; ok {
			continue
		}
		name := installed.Name
		description := installed.Description
		searchText := installed.SearchText
		if asset, err := shubskills.LoadAssetDir(installed.InstallDir); err == nil {
			if strings.TrimSpace(name) == "" {
				name = asset.Name
			}
			if strings.TrimSpace(description) == "" {
				description = asset.Description
			}
			if strings.TrimSpace(searchText) == "" {
				searchText = buildInstalledSearchText(installed.RegistryName, asset.ID, asset.Name, asset.Description, string(asset.Category), asset.Version)
			}
		}
		if !matchesSearch(needle, installed.AssetID, installed.RegistryName, name, description, searchText, installed.Category, installed.Version) {
			continue
		}
		if strings.TrimSpace(name) == "" {
			name = installed.AssetID
		}
		results = append(results, SearchResult{
			AssetID:      installed.AssetID,
			RegistryName: installed.RegistryName,
			Name:         name,
			Description:  description,
			Version:      installed.Version,
			Installed:    true,
		})
		seen[key] = struct{}{}
	}
	return results, nil
}

func (manager *Manager) Sync() (*SyncResult, error) {
	var result *SyncResult
	err := manager.withStateLock(func() error {
		synced, err := manager.syncLocked()
		if err != nil {
			return err
		}
		result = synced
		return nil
	})
	return result, err
}

func (manager *Manager) syncLocked() (*SyncResult, error) {
	if manager.registry == nil {
		return nil, fmt.Errorf("registry client not configured")
	}
	if err := manager.ensureLayout(); err != nil {
		return nil, err
	}
	state, err := manager.loadState()
	if err != nil {
		return nil, err
	}
	syncStart := time.Now().UTC()

	if incremental := manager.incrementalAssetRegistry(); incremental != nil {
		return manager.syncIncrementalLocked(state, incremental)
	}

	seen := make(map[string]struct{})
	result := &SyncResult{}
	for _, installed := range state.Installed {
		if installed.RegistryName == "" {
			continue
		}
		if _, ok := seen[installed.RegistryName]; ok {
			continue
		}
		seen[installed.RegistryName] = struct{}{}
		result.Checked++

		latestVersion, err := manager.resolveLatestRemoteVersion(installed.RegistryName)
		if err != nil {
			return nil, err
		}
		if latestVersion == "" {
			continue
		}
		if findInstalledByRegistryName(state, installed.RegistryName, latestVersion) != nil {
			continue
		}

		previousAssetID := installed.AssetID
		previousVersion := state.Active[installed.AssetID]
		added, err := manager.addLocked(installed.RegistryName, AddOptions{Version: latestVersion})
		if err != nil {
			return nil, err
		}
		result.Installed++

		if previousAssetID != "" && previousVersion != "" && added.Asset.ID == previousAssetID && added.Asset.Version != previousVersion {
			if _, err := manager.useLocked(previousAssetID+"@"+previousVersion, ""); err != nil {
				return nil, err
			}
		}
	}
	latestState, err := manager.loadState()
	if err != nil {
		return nil, err
	}
	latestState.Sync.Cursor = syncStart.Format(time.RFC3339Nano)
	latestState.Sync.LastSyncedAt = syncStart
	if err := manager.saveState(latestState); err != nil {
		return nil, err
	}
	return result, nil
}

func (manager *Manager) Doctor() (*DoctorResult, error) {
	var result *DoctorResult
	err := manager.withStateLock(func() error {
		repaired, err := manager.doctorLocked()
		if err != nil {
			return err
		}
		result = repaired
		return nil
	})
	return result, err
}

func (manager *Manager) doctorLocked() (*DoctorResult, error) {
	if err := manager.ensureLayout(); err != nil {
		return nil, err
	}
	state, err := manager.loadState()
	if err != nil {
		return nil, err
	}
	result := &DoctorResult{}
	loadedAssets := make(map[string]*models.Asset, len(state.Installed))

	for index := range state.Installed {
		installed := &state.Installed[index]
		result.Checked++

		installRepairNeeded := installedAssetRepairRequired(installed.InstallDir)
		asset, err := manager.loadOrRepairInstalledAssetLocked(installed)
		if err != nil {
			return nil, fmt.Errorf("doctor load asset %s@%s: %w", installed.AssetID, installed.Version, err)
		}
		if installRepairNeeded {
			result.Repaired++
		}
		loadedAssets[installed.AssetID+"@"+installed.Version] = asset

		runtimeRepairNeeded := runtimeRepairRequired(asset, installed.EnvDir, installed.Runtime)
		envDir, runtimeStatus, err := manager.materializeRuntime(asset, installed.InstallDir)
		if err != nil {
			return nil, err
		}
		if installed.EnvDir != envDir {
			installed.EnvDir = envDir
			result.Repaired++
		}
		if runtimeRepairNeeded || runtimeStatusSubstantiveChanged(installed.Runtime, runtimeStatus) {
			installed.Runtime = runtimeStatus
			result.Repaired++
		} else {
			installed.Runtime.CheckedAt = runtimeStatus.CheckedAt
		}
		if updateInstalledSummary(installed, asset) {
			result.Repaired++
		}
		installed.UpdatedAt = time.Now().UTC()
	}
	result.Repaired += reconcileActiveSelections(state)

	for index := range state.Installed {
		installed := &state.Installed[index]
		if state.Active[installed.AssetID] != installed.Version {
			continue
		}

		asset := loadedAssets[installed.AssetID+"@"+installed.Version]
		if asset == nil {
			reloaded, err := manager.loadOrRepairInstalledAssetLocked(installed)
			if err != nil {
				return nil, fmt.Errorf("doctor refresh exports for %s@%s: %w", installed.AssetID, installed.Version, err)
			}
			asset = reloaded
		}

		result.Repaired += countMissingExportPaths(installed.ExportPaths)
		exportPaths, exportRecords, err := manager.writeExports(asset, installed.InstallDir)
		if err != nil {
			return nil, err
		}
		if !equalStringSlices(installed.ExportPaths, exportPaths) {
			installed.ExportPaths = exportPaths
			result.Repaired++
		}
		if !equalExportRecords(installed.Exports, exportRecords) {
			installed.Exports = exportRecords
			result.Repaired++
		}
	}

	if err := manager.saveState(state); err != nil {
		return nil, err
	}
	clientConfigRepairs, err := manager.refreshManagedClientConfigsLocked()
	if err != nil {
		return nil, err
	}
	result.Repaired += clientConfigRepairs
	return result, nil
}

func (manager *Manager) resolveSkill(skillName, version string) (*models.SkillResponse, error) {
	if strings.TrimSpace(version) == "" {
		skillResp, err := manager.registry.GetSkill(skillName)
		if err != nil {
			return nil, fmt.Errorf("fetch latest skill metadata: %w", err)
		}
		return skillResp, nil
	}

	skillResp, err := manager.registry.GetSkillVersion(skillName, version)
	if err != nil {
		return nil, fmt.Errorf("fetch skill version metadata: %w", err)
	}
	return skillResp, nil
}

func (manager *Manager) assetRegistry() AssetRegistry {
	if manager.registry == nil {
		return nil
	}
	assetRegistry, ok := manager.registry.(AssetRegistry)
	if !ok {
		return nil
	}
	return assetRegistry
}

func (manager *Manager) incrementalAssetRegistry() IncrementalAssetRegistry {
	if manager.registry == nil {
		return nil
	}
	incremental, ok := manager.registry.(IncrementalAssetRegistry)
	if !ok {
		return nil
	}
	return incremental
}

func (manager *Manager) assetSourcePuller() AssetSourcePuller {
	if manager.registry == nil {
		return nil
	}
	puller, ok := manager.registry.(AssetSourcePuller)
	if !ok {
		return nil
	}
	return puller
}

func (manager *Manager) shubSourceRegistry() SHUBSourceRegistry {
	if manager.registry == nil {
		return nil
	}
	sourceRegistry, ok := manager.registry.(SHUBSourceRegistry)
	if !ok {
		return nil
	}
	return sourceRegistry
}

func (manager *Manager) pullFromFallbackSources(reference, version string, opts AddOptions) (*models.AssetResponse, error) {
	sources, err := manager.resolveFallbackSourcePool(opts)
	if err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		return nil, nil
	}
	puller := manager.assetSourcePuller()
	if puller == nil {
		return nil, fmt.Errorf("registry client does not support SHUB fallback sources")
	}

	failures := make([]string, 0, len(sources))
	for _, sourceName := range sources {
		assetResp, err := puller.PullAssetFromSource(sourceName, reference, version)
		if err == nil && assetResp != nil {
			return assetResp, nil
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", sourceName, err))
			continue
		}
		failures = append(failures, fmt.Sprintf("%s: empty response", sourceName))
	}

	if len(failures) == 0 {
		return nil, nil
	}
	return nil, fmt.Errorf("asset or skill %q not found in registry; fallback failed (%s)", reference, strings.Join(failures, "; "))
}

func (manager *Manager) resolveFallbackSourcePool(opts AddOptions) ([]string, error) {
	explicitSources := normalizeFallbackSources(opts.FallbackSources)
	if len(explicitSources) > 0 {
		return explicitSources, nil
	}

	candidates := append([]string{}, defaultFallbackSourcePool...)
	sourceRegistry := manager.shubSourceRegistry()
	if sourceRegistry == nil {
		if opts.GitHub {
			return normalizeFallbackSources(candidates), nil
		}
		return normalizeFallbackSources(candidates), nil
	}

	sources, err := sourceRegistry.GetSHUBSources()
	if err != nil {
		return nil, fmt.Errorf("list SHUB fallback sources: %w", err)
	}
	for _, source := range sources {
		if source == nil {
			continue
		}
		if opts.GitHub && !isGitHubFallbackSource(source) {
			continue
		}
		candidates = append(candidates, source.Name)
	}
	return normalizeFallbackSources(candidates), nil
}

func normalizeFallbackSources(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func isGitHubFallbackSource(source *models.SHUBSource) bool {
	if source == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(source.Provider), "github") {
		return true
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(source.Address)), "github.com/")
}

func normalizeManagerAssetResponse(asset *models.AssetResponse, baseURL string) {
	if asset == nil || asset.Asset.Source == nil {
		return
	}
	ref := strings.TrimSpace(asset.Asset.Source.PackageRef)
	if !strings.HasPrefix(ref, "/") {
		return
	}
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return
	}
	relative, err := url.Parse(ref)
	if err != nil {
		return
	}
	asset.Asset.Source.PackageRef = base.ResolveReference(relative).String()
}

func (manager *Manager) syncIncrementalLocked(state *State, registry IncrementalAssetRegistry) (*SyncResult, error) {
	result := &SyncResult{}
	installedByAssetID := make(map[string]*InstalledAsset)
	for index := range state.Installed {
		installed := &state.Installed[index]
		if strings.TrimSpace(installed.AssetID) == "" {
			continue
		}
		if _, ok := installedByAssetID[installed.AssetID]; !ok {
			installedByAssetID[installed.AssetID] = installed
		}
	}

	syncStart := time.Now().UTC()
	currentCursor := ""
	maxUpdatedAt := parseSyncCursor(state.Sync.Cursor)
	if maxUpdatedAt.IsZero() {
		maxUpdatedAt = syncStart
	}

	for {
		assets, nextCursor, err := registry.ListAssetsUpdatedSince(state.Sync.Cursor, currentCursor, 100)
		if err != nil {
			return nil, err
		}
		if len(assets) == 0 && nextCursor == "" {
			break
		}

		for _, asset := range assets {
			if asset == nil {
				continue
			}
			result.Checked++
			if asset.Meta.Official != nil && asset.Meta.Official.UpdatedAt.After(maxUpdatedAt) {
				maxUpdatedAt = asset.Meta.Official.UpdatedAt
			}
			installed := installedByAssetID[asset.Asset.ID]
			if installed == nil || findInstalledAsset(state, asset.Asset.ID, asset.Asset.Version) != nil {
				continue
			}

			previousVersion := state.Active[installed.AssetID]
			added, err := manager.addLocked(asset.Asset.ID, AddOptions{Version: asset.Asset.Version})
			if err != nil {
				return nil, err
			}
			result.Installed++
			if previousVersion != "" && added.Asset.ID == installed.AssetID && added.Asset.Version != previousVersion {
				if _, err := manager.useLocked(installed.AssetID+"@"+previousVersion, ""); err != nil {
					return nil, err
				}
			}

			latestState, err := manager.loadState()
			if err != nil {
				return nil, err
			}
			state = latestState
		}

		if nextCursor == "" {
			break
		}
		currentCursor = nextCursor
	}

	latestState, err := manager.loadState()
	if err != nil {
		return nil, err
	}
	if maxUpdatedAt.Before(syncStart) {
		maxUpdatedAt = syncStart
	}
	latestState.Sync.Cursor = maxUpdatedAt.Format(time.RFC3339Nano)
	latestState.Sync.LastSyncedAt = syncStart
	if err := manager.saveState(latestState); err != nil {
		return nil, err
	}
	return result, nil
}

func runtimeRepairRequired(asset *models.Asset, envDir string, current RuntimeStatus) bool {
	trimmedEnvDir := strings.TrimSpace(envDir)
	if trimmedEnvDir == "" {
		return true
	}
	if _, err := os.Stat(trimmedEnvDir); err != nil {
		return true
	}

	metadataPath := strings.TrimSpace(current.MetadataPath)
	if metadataPath == "" {
		metadataPath = filepath.Join(trimmedEnvDir, "metadata.json")
	}
	if _, err := os.Stat(metadataPath); err != nil {
		return true
	}

	runtimeType := strings.TrimSpace(asset.Manifest.Runtime.Type)
	switch runtimeType {
	case "python":
		return !pythonRuntimeHealthy(filepath.Join(trimmedEnvDir, "venv"))
	default:
		return false
	}
}

func installedAssetRepairRequired(installDir string) bool {
	trimmed := strings.TrimSpace(installDir)
	if trimmed == "" {
		return true
	}
	if _, err := shubskills.LoadAssetDir(trimmed); err != nil {
		return true
	}
	return false
}

func runtimeStatusSubstantiveChanged(current, next RuntimeStatus) bool {
	return current.Type != next.Type ||
		current.Status != next.Status ||
		current.MetadataPath != next.MetadataPath
}

func countMissingExportPaths(paths []string) int {
	missing := 0
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		if _, err := os.Stat(trimmed); err != nil {
			if os.IsNotExist(err) {
				missing++
			}
		}
	}
	return missing
}

func (manager *Manager) loadOrRepairInstalledAssetLocked(installed *InstalledAsset) (*models.Asset, error) {
	asset, err := shubskills.LoadAssetDir(installed.InstallDir)
	if err == nil {
		return asset, nil
	}
	if manager.registry == nil {
		return nil, err
	}

	var lastErr error
	for _, reference := range repairReferencesForInstalled(installed) {
		registryName, skillResp, resolveErr := manager.resolveRemoteInstallSource(reference, AddOptions{Version: installed.Version})
		if resolveErr != nil {
			lastErr = resolveErr
			continue
		}
		if skillResp == nil {
			continue
		}

		stagingDir, stageErr := os.MkdirTemp("", "shub-repair-*")
		if stageErr != nil {
			return nil, fmt.Errorf("create repair staging directory: %w", stageErr)
		}
		func() {
			defer func() { _ = os.RemoveAll(stagingDir) }()
			if stageErr = manager.installer.Install(skillResp, stagingDir); stageErr != nil {
				lastErr = fmt.Errorf("install repair source: %w", stageErr)
				return
			}
			repairedAsset, loadErr := shubskills.LoadAssetDir(stagingDir)
			if loadErr != nil {
				lastErr = fmt.Errorf("validate repaired asset: %w", loadErr)
				return
			}
			if err := ensureParentDir(installed.InstallDir); err != nil {
				lastErr = err
				return
			}
			if err := os.RemoveAll(installed.InstallDir); err != nil {
				lastErr = fmt.Errorf("clear broken install dir: %w", err)
				return
			}
			if err := os.Rename(stagingDir, installed.InstallDir); err != nil {
				lastErr = fmt.Errorf("move repaired install into place: %w", err)
				return
			}
			installed.RegistryName = registryName
			installed.AssetID = repairedAsset.ID
			asset = repairedAsset
			lastErr = nil
		}()
		if lastErr == nil {
			return asset, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, err
}

func repairReferencesForInstalled(installed *InstalledAsset) []string {
	references := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, reference := range []string{installed.AssetID, installed.RegistryName} {
		trimmed := strings.TrimSpace(reference)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		references = append(references, trimmed)
	}
	return references
}

func parseSyncCursor(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func (manager *Manager) resolveAsset(assetID, version string) (*models.AssetResponse, error) {
	assetRegistry := manager.assetRegistry()
	if assetRegistry == nil {
		return nil, nil
	}
	if strings.TrimSpace(version) == "" {
		assetResp, err := assetRegistry.GetAsset(assetID)
		if err != nil {
			return nil, fmt.Errorf("fetch latest asset metadata: %w", err)
		}
		return assetResp, nil
	}
	assetResp, err := assetRegistry.GetAssetVersion(assetID, version)
	if err != nil {
		return nil, fmt.Errorf("fetch asset version metadata: %w", err)
	}
	return assetResp, nil
}

func (manager *Manager) resolveRemoteInstallSource(reference string, opts AddOptions) (string, *models.SkillResponse, error) {
	if assetResp, err := manager.resolveAsset(reference, opts.Version); err != nil {
		return "", nil, err
	} else if assetResp != nil {
		normalizeManagerAssetResponse(assetResp, manager.baseURL)
		skillResp, err := skillResponseFromAssetResponse(assetResp)
		if err != nil {
			return "", nil, err
		}
		return assetResp.Asset.ID, skillResp, nil
	}

	skillResp, err := manager.resolveSkill(reference, opts.Version)
	if err != nil {
		return "", nil, err
	}
	if skillResp == nil {
		pulledAsset, err := manager.pullFromFallbackSources(reference, opts.Version, opts)
		if err != nil {
			return "", nil, err
		}
		if pulledAsset != nil {
			normalizeManagerAssetResponse(pulledAsset, manager.baseURL)
			skillResp, err = skillResponseFromAssetResponse(pulledAsset)
			if err != nil {
				return "", nil, err
			}
			return pulledAsset.Asset.ID, skillResp, nil
		}
	}
	return reference, skillResp, nil
}

func (manager *Manager) resolveLatestRemoteVersion(reference string) (string, error) {
	if assetResp, err := manager.resolveAsset(reference, ""); err != nil {
		return "", fmt.Errorf("fetch latest asset metadata for %s: %w", reference, err)
	} else if assetResp != nil {
		return assetResp.Asset.Version, nil
	}

	latest, err := manager.registry.GetSkill(reference)
	if err != nil {
		return "", fmt.Errorf("fetch latest skill metadata for %s: %w", reference, err)
	}
	if latest == nil {
		return "", nil
	}
	return latest.Skill.Version, nil
}

func (manager *Manager) searchRemoteAssets(state *State, assets []*models.AssetResponse, query string) []SearchResult {
	needle := strings.ToLower(strings.TrimSpace(query))
	results := make([]SearchResult, 0)
	for _, asset := range assets {
		if asset == nil {
			continue
		}
		if !matchesSearch(needle, asset.Asset.ID, asset.Asset.Name, asset.Asset.Description) {
			continue
		}
		installed := findInstalledAsset(state, asset.Asset.ID, "")
		result := SearchResult{
			AssetID:      asset.Asset.ID,
			RegistryName: asset.Asset.ID,
			Name:         asset.Asset.Name,
			Description:  asset.Asset.Description,
			Version:      asset.Asset.Version,
			Installed:    installed != nil,
		}
		if installed != nil && installed.RegistryName != "" {
			result.RegistryName = installed.RegistryName
		}
		results = append(results, result)
	}
	return results
}

func skillResponseFromAssetResponse(assetResp *models.AssetResponse) (*models.SkillResponse, error) {
	if assetResp == nil {
		return nil, nil
	}
	response := &models.SkillResponse{Skill: models.SkillJSON{
		Name:        assetResp.Asset.Name,
		Title:       assetResp.Asset.ID,
		Category:    string(assetResp.Asset.Category),
		Description: assetResp.Asset.Description,
		Version:     assetResp.Asset.Version,
		SHUB: &models.SkillSHUBMetadata{
			SchemaVersion: models.ShubAssetSchemaVersion,
			AssetID:       assetResp.Asset.ID,
			Category:      assetResp.Asset.Category,
			Manifest:      cloneManagerAssetManifest(assetResp.Asset.Manifest),
			Source:        cloneManagerAssetSource(assetResp.Asset.Source),
		},
	}}
	if assetResp.Meta.Official != nil {
		response.Meta.Official = &models.SkillRegistryExtensions{
			Status:      assetResp.Meta.Official.Status,
			PublishedAt: assetResp.Meta.Official.PublishedAt,
			UpdatedAt:   assetResp.Meta.Official.UpdatedAt,
			IsLatest:    assetResp.Meta.Official.IsLatest,
		}
	}
	if assetResp.Asset.Source != nil {
		if assetResp.Asset.Source.RepositoryURL != "" {
			response.Skill.Repository = &models.SkillRepository{URL: assetResp.Asset.Source.RepositoryURL, Source: "git"}
		}
		if assetResp.Asset.Source.PackageRef != "" {
			pkg := models.SkillPackageInfo{
				RegistryType: assetResp.Asset.Source.PackageType,
				Identifier:   assetResp.Asset.Source.PackageRef,
				Version:      assetResp.Asset.Version,
			}
			switch assetResp.Asset.Source.PackageType {
			case "tarball", "archive":
				pkg.Transport.Type = "streamable-http"
			case "docker", "oci":
				pkg.Transport.Type = "docker"
			}
			response.Skill.Packages = append(response.Skill.Packages, pkg)
		}
	}
	if response.Skill.Repository == nil && len(response.Skill.Packages) == 0 {
		return nil, fmt.Errorf("asset '%s' is missing install source metadata", assetResp.Asset.ID)
	}
	return response, nil
}

func cloneManagerAssetSource(source *models.AssetSource) *models.AssetSource {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}

func cloneManagerAssetManifest(manifest models.AssetManifest) *models.AssetManifest {
	cloned := manifest
	cloned.AllowedTools = append([]string(nil), manifest.AllowedTools...)
	if len(manifest.Exports) > 0 {
		cloned.Exports = append([]models.AssetExport(nil), manifest.Exports...)
	}
	if manifest.Metadata != nil {
		cloned.Metadata = make(map[string]any, len(manifest.Metadata))
		maps.Copy(cloned.Metadata, manifest.Metadata)
	}
	if manifest.Runtime.Install != nil {
		install := *manifest.Runtime.Install
		cloned.Runtime.Install = &install
	}
	if manifest.Hooks.PostInstall != nil {
		command := *manifest.Hooks.PostInstall
		command.Run = append([]string(nil), manifest.Hooks.PostInstall.Run...)
		cloned.Hooks.PostInstall = &command
	}
	if manifest.Hooks.PostPull != nil {
		command := *manifest.Hooks.PostPull
		command.Run = append([]string(nil), manifest.Hooks.PostPull.Run...)
		cloned.Hooks.PostPull = &command
	}
	return &cloned
}

func (manager *Manager) installDirForAsset(asset *models.Asset) string {
	parts := strings.Split(asset.ID, "/")
	segments := []string{manager.homeRoot, "hub", manager.registryHost()}
	segments = append(segments, parts...)
	segments = append(segments, asset.Version)
	return filepath.Join(segments...)
}

func (manager *Manager) envDirForAsset(asset *models.Asset) string {
	hash := shortHash(asset.ID + "@" + asset.Version)
	return filepath.Join(manager.homeRoot, "envs", hash)
}

func (manager *Manager) writeExports(asset *models.Asset, installDir string) ([]string, []ExportRecord, error) {
	exportDir := filepath.Join(manager.homeRoot, "exports")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create exports directory: %w", err)
	}

	records := make([]ExportRecord, 0)
	paths := make([]string, 0)
	now := time.Now().UTC()
	exports := asset.Manifest.Exports
	if len(exports) == 0 && (asset.Category == models.AssetCategoryPrompt || asset.Category == models.AssetCategorySkill || asset.Category == models.AssetCategoryAgent) {
		exports = []models.AssetExport{{Target: "codex", Mode: "prompt-file", Source: models.SkillFileName}}
	}

	for _, export := range exports {
		exportPath, sourcePath, err := manager.renderExport(asset, installDir, exportDir, export)
		if err != nil {
			return nil, nil, err
		}
		if exportPath == "" {
			continue
		}
		paths = append(paths, exportPath)
		records = append(records, ExportRecord{
			AssetID:    asset.ID,
			Version:    asset.Version,
			Target:     export.Target,
			Mode:       export.Mode,
			ExportPath: exportPath,
			SourcePath: sourcePath,
			Status:     "ready",
			UpdatedAt:  now,
		})
	}

	sort.Strings(paths)
	if err := manager.writeExportMetadata(asset, records); err != nil {
		return nil, nil, err
	}
	return paths, records, nil
}

func (manager *Manager) renderExport(asset *models.Asset, installDir, exportDir string, export models.AssetExport) (string, string, error) {
	exportBaseDir := exportDir
	switch {
	case export.Target == "codex" && export.Mode == "skill-dir":
		codexDir, err := resolveCodexSkillsDir()
		if err != nil {
			return "", "", err
		}
		exportBaseDir = codexDir
	case export.Target == "claude-code" && export.Mode == "prompt-file":
		claudeCommandsDir, err := resolveClaudeCommandsDir()
		if err != nil {
			return "", "", err
		}
		exportBaseDir = claudeCommandsDir
	case export.Target == "claude-code" && export.Mode == "skill-dir":
		claudeSkillsDir, err := resolveClaudeSkillsDir()
		if err != nil {
			return "", "", err
		}
		exportBaseDir = claudeSkillsDir
	case export.Target == "claude-code" && export.Mode == "mcp-config":
		claudeMCPDir, err := resolveClaudeMCPSourceDir(exportDir)
		if err != nil {
			return "", "", err
		}
		exportBaseDir = claudeMCPDir
	case export.Target == "aider" && export.Mode == "rules-file":
		aiderDir, err := resolveAiderRulesDir(exportDir)
		if err != nil {
			return "", "", err
		}
		exportBaseDir = aiderDir
	case export.Target == "cursor" && export.Mode == "rules-file":
		cursorDir, err := resolveCursorRulesDir(exportDir)
		if err != nil {
			return "", "", err
		}
		exportBaseDir = cursorDir
	}

	exportPath, err := resolveExportPath(exportBaseDir, asset, export)
	if err != nil {
		return "", "", err
	}
	if exportPath == "" {
		return "", "", nil
	}

	sourcePath := filepath.Clean(filepath.Join(installDir, exportSourceOrDefault(export)))
	switch export.Mode {
	case "prompt-file", "rules-file":
		var data []byte
		if export.Source == "" || export.Source == models.SkillFileName {
			data = []byte(asset.SourceSkill.Body)
			sourcePath = filepath.Join(installDir, models.SkillFileName)
		} else {
			data, err = os.ReadFile(sourcePath)
			if err != nil {
				return "", "", fmt.Errorf("read export source %s: %w", export.Source, err)
			}
		}
		if err := removeExistingExportPath(exportPath); err != nil {
			return "", "", fmt.Errorf("clear export path %s: %w", exportPath, err)
		}
		if err := ensureParentDir(exportPath); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(exportPath, data, 0o644); err != nil {
			return "", "", fmt.Errorf("write export file: %w", err)
		}
		return exportPath, sourcePath, nil
	case "mcp-config":
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return "", "", fmt.Errorf("read export source %s: %w", export.Source, err)
		}
		if err := removeExistingExportPath(exportPath); err != nil {
			return "", "", fmt.Errorf("clear export path %s: %w", exportPath, err)
		}
		if err := ensureParentDir(exportPath); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(exportPath, data, 0o644); err != nil {
			return "", "", fmt.Errorf("write export file: %w", err)
		}
		return exportPath, sourcePath, nil
	case "skill-dir":
		if err := removeExistingExportPath(exportPath); err != nil {
			return "", "", fmt.Errorf("clear export path %s: %w", exportPath, err)
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			return "", "", fmt.Errorf("stat export source %s: %w", export.Source, err)
		}
		if info.IsDir() {
			if err := copyDirContents(sourcePath, exportPath); err != nil {
				return "", "", err
			}
			return exportPath, sourcePath, nil
		}
		if err := os.MkdirAll(exportPath, 0o755); err != nil {
			return "", "", fmt.Errorf("create skill-dir export %s: %w", exportPath, err)
		}
		targetFile := filepath.Join(exportPath, filepath.Base(sourcePath))
		if err := docker.CopyFile(sourcePath, targetFile); err != nil {
			return "", "", fmt.Errorf("copy skill-dir export file: %w", err)
		}
		return exportPath, sourcePath, nil
	default:
		return "", "", nil
	}
}

func (manager *Manager) writeExportMetadata(asset *models.Asset, newRecords []ExportRecord) error {
	metadataPath := filepath.Join(manager.homeRoot, "exports", ".metadata.json")
	metadata := ExportMetadata{}
	if data, err := os.ReadFile(metadataPath); err == nil {
		if err := json.Unmarshal(data, &metadata); err != nil {
			return fmt.Errorf("parse export metadata: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read export metadata: %w", err)
	}

	previous := make([]ExportRecord, 0)
	filtered := metadata.Exports[:0]
	for _, record := range metadata.Exports {
		if record.AssetID == asset.ID {
			previous = append(previous, record)
			continue
		}
		filtered = append(filtered, record)
	}

	current := make(map[string]struct{}, len(newRecords))
	for _, record := range newRecords {
		current[record.ExportPath] = struct{}{}
	}
	for _, record := range previous {
		if _, ok := current[record.ExportPath]; ok {
			continue
		}
		if err := removeExistingExportPath(record.ExportPath); err != nil {
			return fmt.Errorf("remove stale export %s: %w", record.ExportPath, err)
		}
	}

	metadata.Exports = append(filtered, newRecords...)
	sort.Slice(metadata.Exports, func(i, j int) bool {
		if metadata.Exports[i].AssetID == metadata.Exports[j].AssetID {
			if metadata.Exports[i].Version == metadata.Exports[j].Version {
				return metadata.Exports[i].ExportPath < metadata.Exports[j].ExportPath
			}
			return metadata.Exports[i].Version < metadata.Exports[j].Version
		}
		return metadata.Exports[i].AssetID < metadata.Exports[j].AssetID
	})
	return writeJSONFile(metadataPath, metadata)
}

func (manager *Manager) materializeRuntime(asset *models.Asset, installDir string) (string, RuntimeStatus, error) {
	envDir := manager.envDirForAsset(asset)
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		return "", RuntimeStatus{}, fmt.Errorf("create env directory: %w", err)
	}

	metadata := map[string]any{
		"assetId": asset.ID,
		"version": asset.Version,
		"runtime": asset.Manifest.Runtime,
	}
	metadataPath := filepath.Join(envDir, "metadata.json")
	if err := writeJSONFile(metadataPath, metadata); err != nil {
		return "", RuntimeStatus{}, err
	}

	status := RuntimeStatus{
		Type:         asset.Manifest.Runtime.Type,
		Status:       "ready",
		CheckedAt:    time.Now().UTC(),
		MetadataPath: metadataPath,
	}
	runtimeType := asset.Manifest.Runtime.Type
	switch runtimeType {
	case "", "none", "node", "binary":
		if status.Type == "" {
			status.Type = "none"
		}
		return envDir, status, nil
	case "python":
		venvDir := filepath.Join(envDir, "venv")
		if pythonRuntimeHealthy(venvDir) {
			return envDir, status, nil
		}
		if err := os.RemoveAll(venvDir); err != nil {
			return "", RuntimeStatus{}, fmt.Errorf("clear unhealthy python runtime: %w", err)
		}
		if err := createPythonVenv(venvDir, installDir, asset.Manifest.Runtime); err != nil {
			return "", RuntimeStatus{}, err
		}
		return envDir, status, nil
	default:
		return "", RuntimeStatus{}, fmt.Errorf("unsupported runtime type: %s", runtimeType)
	}
}

func (manager *Manager) updateState(registryName string, asset *models.Asset, installDir, envDir string, exportPaths []string, runtimeStatus RuntimeStatus, exportRecords []ExportRecord) error {
	state, err := manager.loadState()
	if err != nil {
		return err
	}
	if state.Active == nil {
		state.Active = make(map[string]string)
	}

	now := time.Now().UTC()
	updated := false
	for index := range state.Installed {
		entry := &state.Installed[index]
		if entry.AssetID == asset.ID && entry.Version == asset.Version {
			entry.RegistryName = registryName
			updateInstalledSummary(entry, asset)
			entry.Category = string(asset.Category)
			entry.RegistryHost = manager.registryHost()
			entry.InstallDir = installDir
			entry.EnvDir = envDir
			entry.ExportPaths = exportPaths
			entry.Runtime = runtimeStatus
			entry.Exports = exportRecords
			entry.UpdatedAt = now
			updated = true
			break
		}
	}
	if !updated {
		entry := InstalledAsset{
			RegistryName: registryName,
			AssetID:      asset.ID,
			Version:      asset.Version,
			Category:     string(asset.Category),
			RegistryHost: manager.registryHost(),
			InstallDir:   installDir,
			EnvDir:       envDir,
			ExportPaths:  exportPaths,
			Runtime:      runtimeStatus,
			Exports:      exportRecords,
			InstalledAt:  now,
			UpdatedAt:    now,
		}
		updateInstalledSummary(&entry, asset)
		state.Installed = append(state.Installed, entry)
	}
	state.Active[asset.ID] = asset.Version
	sort.Slice(state.Installed, func(i, j int) bool {
		if state.Installed[i].AssetID == state.Installed[j].AssetID {
			return compareInstalledVersionPreference(&state.Installed[i], &state.Installed[j]) < 0
		}
		return state.Installed[i].AssetID < state.Installed[j].AssetID
	})
	return manager.saveState(state)
}

func (manager *Manager) updateConfig() error {
	configPath := filepath.Join(manager.homeRoot, "config.json")
	config := Config{RegistryURL: manager.baseURL}
	return writeJSONFile(configPath, config)
}

func (manager *Manager) refreshManagedClientConfigsLocked() (int, error) {
	repaired := 0
	if changed, err := manager.refreshClaudeCodeMCPConfigLocked(); err != nil {
		return repaired, err
	} else if changed {
		repaired++
	}
	if changed, err := manager.refreshClaudeCodeSettingsLocked(); err != nil {
		return repaired, err
	} else if changed {
		repaired++
	}
	if changed, err := manager.refreshAiderConfigLocked(); err != nil {
		return repaired, err
	} else if changed {
		repaired++
	}
	return repaired, nil
}

func (manager *Manager) refreshClaudeCodeMCPConfigLocked() (bool, error) {
	state, err := manager.loadState()
	if err != nil {
		return false, err
	}

	exportDir := filepath.Join(manager.homeRoot, "exports")
	configPath, err := resolveClaudeMCPConfigPath(exportDir)
	if err != nil {
		return false, err
	}
	managedServers, err := collectActiveClaudeMCPServers(state)
	if err != nil {
		return false, err
	}
	return writeClaudeMCPConfig(configPath, managedServers)
}

func (manager *Manager) refreshClaudeCodeSettingsLocked() (bool, error) {
	state, err := manager.loadState()
	if err != nil {
		return false, err
	}

	exportDir := filepath.Join(manager.homeRoot, "exports")
	settingsPath, err := resolveClaudeSettingsPath(exportDir)
	if err != nil {
		return false, err
	}
	managedServers, err := collectActiveClaudeMCPServers(state)
	if err != nil {
		return false, err
	}
	return writeClaudeSettings(settingsPath, mapsKeys(managedServers))
}

func (manager *Manager) refreshAiderConfigLocked() (bool, error) {
	state, err := manager.loadState()
	if err != nil {
		return false, err
	}

	exportDir := filepath.Join(manager.homeRoot, "exports")
	rulesDir, err := resolveAiderRulesDir(exportDir)
	if err != nil {
		return false, err
	}
	configPath, err := resolveAiderConfigPath(exportDir)
	if err != nil {
		return false, err
	}

	managedReads := collectActiveAiderReads(state, configPath)
	return writeAiderConfig(configPath, rulesDir, managedReads)
}

func (manager *Manager) ensureLayout() error {
	directories := []string{
		manager.homeRoot,
		filepath.Join(manager.homeRoot, "hub"),
		filepath.Join(manager.homeRoot, "envs"),
		filepath.Join(manager.homeRoot, "exports"),
	}
	for _, dir := range directories {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return nil
}

func (manager *Manager) loadState() (*State, error) {
	path := filepath.Join(manager.homeRoot, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Version: stateVersion, Active: make(map[string]string)}, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if state.Active == nil {
		state.Active = make(map[string]string)
	}
	if state.Version == 0 {
		state.Version = stateVersion
	}
	return &state, nil
}

func (manager *Manager) saveState(state *State) error {
	state.Version = stateVersion
	return writeJSONFile(filepath.Join(manager.homeRoot, "state.json"), state)
}

func (manager *Manager) registryHost() string {
	parsed, err := url.Parse(manager.baseURL)
	if err != nil || parsed.Host == "" {
		return "default"
	}
	host := parsed.Host
	host = strings.ReplaceAll(host, ":", "_")
	return host
}

func resolveHomeRoot(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return filepath.Abs(override)
	}
	if fromEnv := strings.TrimSpace(os.Getenv("SHUB_HOME")); fromEnv != "" {
		return filepath.Abs(fromEnv)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(homeDir, defaultHomeDirName), nil
}

func resolveCodexSkillsDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("SHUB_CODEX_SKILLS_DIR")); override != "" {
		return filepath.Abs(override)
	}
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		absHome, err := filepath.Abs(codexHome)
		if err != nil {
			return "", fmt.Errorf("resolve CODEX_HOME: %w", err)
		}
		return filepath.Join(absHome, "skills"), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve codex home: %w", err)
	}
	return filepath.Join(homeDir, ".codex", "skills"), nil
}

func resolveClaudeCommandsDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("SHUB_CLAUDE_COMMANDS_DIR")); override != "" {
		return filepath.Abs(override)
	}
	if workspaceDir := strings.TrimSpace(os.Getenv("SHUB_CLAUDE_WORKSPACE_DIR")); workspaceDir != "" {
		absWorkspace, err := filepath.Abs(workspaceDir)
		if err != nil {
			return "", fmt.Errorf("resolve SHUB_CLAUDE_WORKSPACE_DIR: %w", err)
		}
		return filepath.Join(absWorkspace, ".claude", "commands"), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve claude code home: %w", err)
	}
	return filepath.Join(homeDir, ".claude", "commands"), nil
}

func resolveClaudeSkillsDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("SHUB_CLAUDE_SKILLS_DIR")); override != "" {
		return filepath.Abs(override)
	}
	if workspaceDir := strings.TrimSpace(os.Getenv("SHUB_CLAUDE_WORKSPACE_DIR")); workspaceDir != "" {
		absWorkspace, err := filepath.Abs(workspaceDir)
		if err != nil {
			return "", fmt.Errorf("resolve SHUB_CLAUDE_WORKSPACE_DIR: %w", err)
		}
		return filepath.Join(absWorkspace, ".claude", "skills"), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve claude code home: %w", err)
	}
	return filepath.Join(homeDir, ".claude", "skills"), nil
}

func resolveClaudeMCPSourceDir(defaultExportDir string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("SHUB_CLAUDE_MCP_DIR")); override != "" {
		return filepath.Abs(override)
	}
	if workspaceDir := strings.TrimSpace(os.Getenv("SHUB_CLAUDE_WORKSPACE_DIR")); workspaceDir != "" {
		absWorkspace, err := filepath.Abs(workspaceDir)
		if err != nil {
			return "", fmt.Errorf("resolve SHUB_CLAUDE_WORKSPACE_DIR: %w", err)
		}
		return filepath.Join(absWorkspace, ".claude", "mcp", "shub"), nil
	}
	return filepath.Join(defaultExportDir, "claude-code", "mcp", "shub"), nil
}

func resolveClaudeMCPConfigPath(defaultExportDir string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("SHUB_CLAUDE_MCP_CONFIG_PATH")); override != "" {
		return filepath.Abs(override)
	}
	if workspaceDir := strings.TrimSpace(os.Getenv("SHUB_CLAUDE_WORKSPACE_DIR")); workspaceDir != "" {
		absWorkspace, err := filepath.Abs(workspaceDir)
		if err != nil {
			return "", fmt.Errorf("resolve SHUB_CLAUDE_WORKSPACE_DIR: %w", err)
		}
		return filepath.Join(absWorkspace, ".mcp.json"), nil
	}
	return filepath.Join(defaultExportDir, "claude-code", ".mcp.json"), nil
}

func resolveClaudeSettingsPath(defaultExportDir string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("SHUB_CLAUDE_SETTINGS_PATH")); override != "" {
		return filepath.Abs(override)
	}
	if workspaceDir := strings.TrimSpace(os.Getenv("SHUB_CLAUDE_WORKSPACE_DIR")); workspaceDir != "" {
		absWorkspace, err := filepath.Abs(workspaceDir)
		if err != nil {
			return "", fmt.Errorf("resolve SHUB_CLAUDE_WORKSPACE_DIR: %w", err)
		}
		return filepath.Join(absWorkspace, ".claude", "settings.local.json"), nil
	}
	return filepath.Join(defaultExportDir, "claude-code", "settings.local.json"), nil
}

func resolveAiderRulesDir(defaultExportDir string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("SHUB_AIDER_RULES_DIR")); override != "" {
		return filepath.Abs(override)
	}
	if workspaceDir := strings.TrimSpace(os.Getenv("SHUB_AIDER_WORKSPACE_DIR")); workspaceDir != "" {
		absWorkspace, err := filepath.Abs(workspaceDir)
		if err != nil {
			return "", fmt.Errorf("resolve SHUB_AIDER_WORKSPACE_DIR: %w", err)
		}
		return filepath.Join(absWorkspace, ".aider", "shub"), nil
	}
	return filepath.Join(defaultExportDir, "aider", "shub"), nil
}

func resolveAiderConfigPath(defaultExportDir string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("SHUB_AIDER_CONFIG_PATH")); override != "" {
		return filepath.Abs(override)
	}
	if workspaceDir := strings.TrimSpace(os.Getenv("SHUB_AIDER_WORKSPACE_DIR")); workspaceDir != "" {
		absWorkspace, err := filepath.Abs(workspaceDir)
		if err != nil {
			return "", fmt.Errorf("resolve SHUB_AIDER_WORKSPACE_DIR: %w", err)
		}
		return filepath.Join(absWorkspace, ".aider.conf.yml"), nil
	}
	return filepath.Join(defaultExportDir, "aider", ".aider.conf.yml"), nil
}

func resolveCursorRulesDir(defaultExportDir string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("SHUB_CURSOR_RULES_DIR")); override != "" {
		return filepath.Abs(override)
	}
	if workspaceDir := strings.TrimSpace(os.Getenv("SHUB_CURSOR_WORKSPACE_DIR")); workspaceDir != "" {
		absWorkspace, err := filepath.Abs(workspaceDir)
		if err != nil {
			return "", fmt.Errorf("resolve SHUB_CURSOR_WORKSPACE_DIR: %w", err)
		}
		return filepath.Join(absWorkspace, ".cursor", "rules"), nil
	}
	return filepath.Join(defaultExportDir, "cursor", "rules"), nil
}

func parseAssetRef(reference, version string) (string, string) {
	if version != "" {
		return reference, version
	}
	parts := strings.Split(reference, "@")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return reference, version
}

func findInstalledByRegistryName(state *State, registryName, version string) *InstalledAsset {
	for index := range state.Installed {
		installed := &state.Installed[index]
		if installed.RegistryName != registryName {
			continue
		}
		if version != "" && installed.Version != version {
			continue
		}
		return installed
	}
	return nil
}

func findInstalledAsset(state *State, assetID, version string) *InstalledAsset {
	var selected *InstalledAsset
	for index := range state.Installed {
		installed := &state.Installed[index]
		if installed.AssetID != assetID && installed.RegistryName != assetID {
			continue
		}
		if version != "" && installed.Version != version {
			continue
		}
		if version == "" {
			activeVersion := state.Active[installed.AssetID]
			if activeVersion != "" {
				if installed.Version == activeVersion {
					return installed
				}
				continue
			}
			if selected == nil || compareInstalledVersionPreference(installed, selected) > 0 {
				selected = installed
			}
			continue
		}
		return installed
	}
	return selected
}

func matchesSearch(needle string, fields ...string) bool {
	if needle == "" {
		return true
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}

func flattenAssetID(assetID string) string {
	replacer := strings.NewReplacer("/", "-", "_", "-")
	return replacer.Replace(assetID)
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func ensureParentDir(path string) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create parent directory %s: %w", parent, err)
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}
	return nil
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalExportRecords(left, right []ExportRecord) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].AssetID != right[index].AssetID ||
			left[index].Version != right[index].Version ||
			left[index].Target != right[index].Target ||
			left[index].Mode != right[index].Mode ||
			left[index].ExportPath != right[index].ExportPath ||
			left[index].SourcePath != right[index].SourcePath ||
			left[index].Status != right[index].Status {
			return false
		}
	}
	return true
}

func updateInstalledSummary(installed *InstalledAsset, asset *models.Asset) bool {
	if installed == nil || asset == nil {
		return false
	}

	updated := false
	if installed.AssetID != asset.ID {
		installed.AssetID = asset.ID
		updated = true
	}
	if installed.Name != asset.Name {
		installed.Name = asset.Name
		updated = true
	}
	if installed.Description != asset.Description {
		installed.Description = asset.Description
		updated = true
	}
	if installed.Category != string(asset.Category) {
		installed.Category = string(asset.Category)
		updated = true
	}
	searchText := buildInstalledSearchText(installed.RegistryName, asset.ID, asset.Name, asset.Description, string(asset.Category), asset.Version)
	if installed.SearchText != searchText {
		installed.SearchText = searchText
		updated = true
	}
	return updated
}

func buildInstalledSearchText(fields ...string) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " ")
}

func reconcileActiveSelections(state *State) int {
	if state == nil {
		return 0
	}
	if state.Active == nil {
		state.Active = make(map[string]string)
	}

	repaired := 0
	installedByAssetID := make(map[string][]*InstalledAsset)
	for index := range state.Installed {
		installed := &state.Installed[index]
		if strings.TrimSpace(installed.AssetID) == "" {
			continue
		}
		installedByAssetID[installed.AssetID] = append(installedByAssetID[installed.AssetID], installed)
	}

	for assetID, versions := range installedByAssetID {
		activeVersion := state.Active[assetID]
		if activeVersion != "" {
			if installed := findInstalledAssetExact(state, assetID, activeVersion); installed != nil {
				continue
			}
		}

		preferred := preferredInstalledAsset(versions)
		if preferred == nil {
			if _, ok := state.Active[assetID]; ok {
				delete(state.Active, assetID)
				repaired++
			}
			continue
		}
		if state.Active[assetID] != preferred.Version {
			state.Active[assetID] = preferred.Version
			repaired++
		}
	}

	for assetID := range state.Active {
		if _, ok := installedByAssetID[assetID]; ok {
			continue
		}
		delete(state.Active, assetID)
		repaired++
	}
	return repaired
}

func findInstalledAssetExact(state *State, assetID, version string) *InstalledAsset {
	if state == nil {
		return nil
	}
	for index := range state.Installed {
		installed := &state.Installed[index]
		if installed.AssetID == assetID && installed.Version == version {
			return installed
		}
	}
	return nil
}

func preferredInstalledAsset(installed []*InstalledAsset) *InstalledAsset {
	var preferred *InstalledAsset
	for _, candidate := range installed {
		if candidate == nil {
			continue
		}
		if preferred == nil || compareInstalledVersionPreference(candidate, preferred) > 0 {
			preferred = candidate
		}
	}
	return preferred
}

func compareInstalledVersionPreference(left, right *InstalledAsset) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}

	leftSemver := normalizeInstalledSemver(left.Version)
	rightSemver := normalizeInstalledSemver(right.Version)
	switch {
	case leftSemver != "" && rightSemver != "":
		if cmp := semver.Compare(leftSemver, rightSemver); cmp != 0 {
			return cmp
		}
	case leftSemver != "":
		return 1
	case rightSemver != "":
		return -1
	}

	if !left.UpdatedAt.Equal(right.UpdatedAt) {
		if left.UpdatedAt.Before(right.UpdatedAt) {
			return -1
		}
		return 1
	}
	if !left.InstalledAt.Equal(right.InstalledAt) {
		if left.InstalledAt.Before(right.InstalledAt) {
			return -1
		}
		return 1
	}
	return strings.Compare(left.Version, right.Version)
}

func normalizeInstalledSemver(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "v") {
		trimmed = "v" + trimmed
	}
	if !semver.IsValid(trimmed) {
		return ""
	}
	return trimmed
}

func exportSourceOrDefault(export models.AssetExport) string {
	if strings.TrimSpace(export.Source) != "" {
		return export.Source
	}
	return models.SkillFileName
}

func resolveExportPath(exportDir string, asset *models.Asset, export models.AssetExport) (string, error) {
	if strings.TrimSpace(export.TargetPath) != "" {
		if filepath.IsAbs(export.TargetPath) {
			return "", fmt.Errorf("export targetPath must be relative: %s", export.TargetPath)
		}
		absBase, err := filepath.Abs(exportDir)
		if err != nil {
			return "", fmt.Errorf("resolve exports directory: %w", err)
		}
		absTarget, err := filepath.Abs(filepath.Join(absBase, export.TargetPath))
		if err != nil {
			return "", fmt.Errorf("resolve export targetPath %s: %w", export.TargetPath, err)
		}
		if !isWithinBasePath(absBase, absTarget) && absBase != absTarget {
			return "", fmt.Errorf("export targetPath must stay within the exports directory: %s", export.TargetPath)
		}
		return absTarget, nil
	}

	slug := flattenAssetID(asset.ID)
	switch export.Mode {
	case "prompt-file", "rules-file":
		extension := ".md"
		if export.Mode == "rules-file" && export.Target == "cursor" {
			extension = ".mdc"
		}
		return filepath.Join(exportDir, slug+extension), nil
	case "mcp-config":
		return filepath.Join(exportDir, slug+".json"), nil
	case "skill-dir":
		return filepath.Join(exportDir, slug), nil
	default:
		return "", nil
	}
}

func isWithinBasePath(baseDir, target string) bool {
	rel, err := filepath.Rel(baseDir, target)
	if err != nil {
		return false
	}
	if rel == ".." {
		return false
	}
	prefix := ".." + string(filepath.Separator)
	return !strings.HasPrefix(rel, prefix)
}

func removeExistingExportPath(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func copyDirContents(sourceDir, targetDir string) error {
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("resolve export copy path: %w", err)
		}
		if rel == "." {
			return os.MkdirAll(targetDir, 0o755)
		}
		if rel == ".shub" || strings.HasPrefix(filepath.ToSlash(rel), ".shub/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		dstPath := filepath.Join(targetDir, rel)
		if info.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill-dir export does not support symlinks: %s", rel)
		}
		if err := docker.CopyFile(path, dstPath); err != nil {
			return fmt.Errorf("copy export file %s: %w", rel, err)
		}
		return nil
	})
}

func collectActiveClaudeMCPServers(state *State) (map[string]any, error) {
	if state == nil {
		return map[string]any{}, nil
	}

	servers := make(map[string]any)
	for _, installed := range state.Installed {
		if state.Active[installed.AssetID] != installed.Version {
			continue
		}
		for _, record := range installed.Exports {
			if record.Target != "claude-code" || record.Mode != "mcp-config" || strings.TrimSpace(record.ExportPath) == "" {
				continue
			}
			exportedServers, err := readClaudeMCPServers(record.ExportPath)
			if err != nil {
				return nil, err
			}
			for name, config := range exportedServers {
				servers[managedClaudeMCPServerName(installed.AssetID, name)] = config
			}
		}
	}
	return servers, nil
}

func managedClaudeMCPServerName(assetID, serverName string) string {
	assetSlug := flattenAssetID(assetID)
	serverSlug := flattenAssetID(serverName)
	if strings.TrimSpace(serverSlug) == "" {
		serverSlug = "default"
	}
	return claudeMCPPrefix + assetSlug + "__" + serverSlug
}

func managedClaudeMCPPermission(serverName string) string {
	return "mcp__" + serverName
}

func writeClaudeMCPConfig(configPath string, managedServers map[string]any) (bool, error) {
	config, err := readClaudeMCPConfig(configPath)
	if err != nil {
		return false, err
	}

	merged := make(map[string]any)
	for name, server := range readClaudeMCPServerEntries(config["mcpServers"]) {
		if strings.HasPrefix(name, claudeMCPPrefix) {
			continue
		}
		merged[name] = server
	}
	maps.Copy(merged, managedServers)

	if len(merged) == 0 {
		delete(config, "mcpServers")
		if len(config) == 0 {
			removed, err := removeFileIfExists(configPath)
			if err != nil {
				return false, fmt.Errorf("remove claude code mcp config: %w", err)
			}
			return removed, nil
		}
	} else {
		config["mcpServers"] = merged
	}

	if err := ensureParentDir(configPath); err != nil {
		return false, err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal claude code mcp config: %w", err)
	}
	data = append(data, '\n')
	changed, err := writeFileIfChanged(configPath, data, 0o644)
	if err != nil {
		return false, fmt.Errorf("write claude code mcp config: %w", err)
	}
	return changed, nil
}

func readClaudeMCPConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read claude code mcp config: %w", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse claude code mcp config: %w", err)
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}

func writeClaudeSettings(settingsPath string, managedServerNames []string) (bool, error) {
	config, err := readClaudeSettings(settingsPath)
	if err != nil {
		return false, err
	}

	enabled := make([]string, 0, len(managedServerNames))
	permissionsAllow := make([]string, 0, len(managedServerNames))
	for _, serverName := range managedServerNames {
		if strings.TrimSpace(serverName) == "" {
			continue
		}
		enabled = append(enabled, serverName)
		permissionsAllow = append(permissionsAllow, managedClaudeMCPPermission(serverName))
	}

	existingEnabled := readStringList(config["enabledMcpjsonServers"])
	config["enabledMcpjsonServers"] = mergeManagedStringList(existingEnabled, enabled, func(entry string) bool {
		return strings.HasPrefix(entry, claudeMCPPrefix)
	})
	if len(readStringList(config["enabledMcpjsonServers"])) == 0 {
		delete(config, "enabledMcpjsonServers")
	}

	permissions := readJSONObject(config["permissions"])
	allowRules := readStringList(permissions["allow"])
	permissions["allow"] = mergeManagedStringList(allowRules, permissionsAllow, func(entry string) bool {
		return strings.HasPrefix(entry, managedClaudeMCPPermission(claudeMCPPrefix))
	})
	if len(readStringList(permissions["allow"])) == 0 {
		delete(permissions, "allow")
	}
	if len(permissions) == 0 {
		delete(config, "permissions")
	} else {
		config["permissions"] = permissions
	}

	if len(config) == 0 {
		removed, err := removeFileIfExists(settingsPath)
		if err != nil {
			return false, fmt.Errorf("remove claude settings: %w", err)
		}
		return removed, nil
	}

	if err := ensureParentDir(settingsPath); err != nil {
		return false, err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal claude settings: %w", err)
	}
	data = append(data, '\n')
	changed, err := writeFileIfChanged(settingsPath, data, 0o644)
	if err != nil {
		return false, fmt.Errorf("write claude settings: %w", err)
	}
	return changed, nil
}

func readClaudeSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read claude settings: %w", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse claude settings: %w", err)
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}

func readClaudeMCPServers(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read claude code mcp export: %w", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse claude code mcp export: %w", err)
	}
	servers := readClaudeMCPServerEntries(config["mcpServers"])
	if len(servers) == 0 {
		return nil, fmt.Errorf("claude code mcp export %s does not define any mcpServers", path)
	}
	return servers, nil
}

func readClaudeMCPServerEntries(value any) map[string]any {
	entries, ok := value.(map[string]any)
	if !ok || len(entries) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(entries))
	for name, entry := range entries {
		if strings.TrimSpace(name) == "" {
			continue
		}
		result[name] = entry
	}
	return result
}

func readJSONObject(value any) map[string]any {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return map[string]any{}
	}
	return object
}

func readStringList(value any) []string {
	switch entries := value.(type) {
	case []string:
		result := make([]string, 0, len(entries))
		for _, entry := range entries {
			trimmed := strings.TrimSpace(entry)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	case []any:
		result := make([]string, 0, len(entries))
		for _, entry := range entries {
			text, ok := entry.(string)
			if !ok {
				continue
			}
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	default:
		return nil
	}
}

func mergeManagedStringList(existing, managed []string, isManaged func(string) bool) []string {
	result := make([]string, 0, len(existing)+len(managed))
	seen := make(map[string]struct{}, len(existing)+len(managed))
	for _, entry := range existing {
		if isManaged(entry) {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		result = append(result, entry)
	}
	for _, entry := range managed {
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		result = append(result, entry)
	}
	sort.Strings(result)
	return result
}

func mapsKeys(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func collectActiveAiderReads(state *State, configPath string) []string {
	if state == nil {
		return nil
	}

	reads := make([]string, 0)
	seen := make(map[string]struct{})
	for _, installed := range state.Installed {
		if state.Active[installed.AssetID] != installed.Version {
			continue
		}
		for _, record := range installed.Exports {
			if record.Target != "aider" || record.Mode != "rules-file" || strings.TrimSpace(record.ExportPath) == "" {
				continue
			}
			readPath := aiderConfigReadPath(configPath, record.ExportPath)
			if _, ok := seen[readPath]; ok {
				continue
			}
			seen[readPath] = struct{}{}
			reads = append(reads, readPath)
		}
	}
	sort.Strings(reads)
	return reads
}

func aiderConfigReadPath(configPath, exportPath string) string {
	configDir := filepath.Dir(configPath)
	absConfigDir, err := filepath.Abs(configDir)
	if err != nil {
		return filepath.ToSlash(exportPath)
	}
	absExportPath, err := filepath.Abs(exportPath)
	if err != nil {
		return filepath.ToSlash(exportPath)
	}
	rel, err := filepath.Rel(absConfigDir, absExportPath)
	if err != nil {
		return filepath.ToSlash(absExportPath)
	}
	return filepath.ToSlash(rel)
}

func writeAiderConfig(configPath, managedRulesDir string, managedReads []string) (bool, error) {
	config, err := readAiderConfig(configPath)
	if err != nil {
		return false, err
	}

	existingReads := aiderConfigReadEntries(config["read"])
	filteredReads := make([]string, 0, len(existingReads)+len(managedReads))
	seen := make(map[string]struct{}, len(existingReads)+len(managedReads))
	for _, entry := range existingReads {
		if isManagedAiderReadEntry(configPath, managedRulesDir, entry) {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		filteredReads = append(filteredReads, entry)
	}
	for _, entry := range managedReads {
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		filteredReads = append(filteredReads, entry)
	}

	if len(filteredReads) == 0 {
		delete(config, "read")
		if len(config) == 0 {
			removed, err := removeFileIfExists(configPath)
			if err != nil {
				return false, fmt.Errorf("remove aider config: %w", err)
			}
			return removed, nil
		}
	} else {
		sort.Strings(filteredReads)
		config["read"] = filteredReads
	}

	if err := ensureParentDir(configPath); err != nil {
		return false, err
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return false, fmt.Errorf("marshal aider config: %w", err)
	}
	changed, err := writeFileIfChanged(configPath, data, 0o644)
	if err != nil {
		return false, fmt.Errorf("write aider config: %w", err)
	}
	return changed, nil
}

func readAiderConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read aider config: %w", err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse aider config: %w", err)
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}

func writeFileIfChanged(path string, data []byte, perm os.FileMode) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == string(data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return false, err
	}
	return true, nil
}

func removeFileIfExists(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func aiderConfigReadEntries(value any) []string {
	switch entries := value.(type) {
	case []string:
		result := make([]string, 0, len(entries))
		for _, entry := range entries {
			trimmed := strings.TrimSpace(entry)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	case []any:
		result := make([]string, 0, len(entries))
		for _, entry := range entries {
			text, ok := entry.(string)
			if !ok {
				continue
			}
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	default:
		return nil
	}
}

func isManagedAiderReadEntry(configPath, managedRulesDir, entry string) bool {
	absManagedDir, err := filepath.Abs(managedRulesDir)
	if err != nil {
		return false
	}
	resolvedEntry := entry
	if !filepath.IsAbs(entry) {
		resolvedEntry = filepath.Join(filepath.Dir(configPath), entry)
	}
	absEntry, err := filepath.Abs(resolvedEntry)
	if err != nil {
		return false
	}
	return isWithinBasePath(absManagedDir, absEntry) || absManagedDir == absEntry
}

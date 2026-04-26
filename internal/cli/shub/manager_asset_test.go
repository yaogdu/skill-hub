package shub

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
)

type assetBackedRegistry struct {
	assets        map[string]*models.AssetResponse
	assetVersions map[string]map[string]*models.AssetResponse
	sources       []*models.SHUBSource
	pullFn        func(sourceName, assetID, version string) (*models.AssetResponse, error)
}

func (registry *assetBackedRegistry) GetAsset(id string) (*models.AssetResponse, error) {
	return registry.assets[id], nil
}

func (registry *assetBackedRegistry) GetAssetVersion(id, version string) (*models.AssetResponse, error) {
	if registry.assetVersions[id] == nil {
		return nil, nil
	}
	return registry.assetVersions[id][version], nil
}

func (registry *assetBackedRegistry) GetAssetVersions(id string) ([]*models.AssetResponse, error) {
	versions := registry.assetVersions[id]
	result := make([]*models.AssetResponse, 0, len(versions))
	for _, asset := range versions {
		result = append(result, asset)
	}
	return result, nil
}

func (registry *assetBackedRegistry) GetAssets() ([]*models.AssetResponse, error) {
	result := make([]*models.AssetResponse, 0, len(registry.assets))
	for _, asset := range registry.assets {
		result = append(result, asset)
	}
	return result, nil
}

func (registry *assetBackedRegistry) GetSHUBSources() ([]*models.SHUBSource, error) {
	return registry.sources, nil
}

func (registry *assetBackedRegistry) ListAssetsUpdatedSince(updatedSince, cursor string, limit int) ([]*models.AssetResponse, string, error) {
	result := make([]*models.AssetResponse, 0)
	updatedAfter := parseSyncCursor(updatedSince)
	for _, versions := range registry.assetVersions {
		for _, asset := range versions {
			if asset == nil {
				continue
			}
			if asset.Meta.Official != nil && !updatedAfter.IsZero() && !asset.Meta.Official.UpdatedAt.After(updatedAfter) {
				continue
			}
			result = append(result, asset)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Asset.ID == result[j].Asset.ID {
			return result[i].Asset.Version < result[j].Asset.Version
		}
		return result[i].Asset.ID < result[j].Asset.ID
	})

	start := 0
	if cursor != "" {
		for start < len(result) && compareTestAssetWithCursor(result[start], cursor) <= 0 {
			start++
		}
	}
	if start >= len(result) {
		return nil, "", nil
	}
	if limit <= 0 {
		limit = len(result)
	}
	end := start + limit
	if end > len(result) {
		end = len(result)
	}
	page := result[start:end]
	nextCursor := ""
	if end < len(result) {
		last := page[len(page)-1]
		nextCursor = last.Asset.ID + ":" + last.Asset.Version
	}
	return page, nextCursor, nil
}

func (registry *assetBackedRegistry) PullAssetFromSource(sourceName, assetID, version string) (*models.AssetResponse, error) {
	if registry.pullFn != nil {
		return registry.pullFn(sourceName, assetID, version)
	}
	return nil, nil
}

func (registry *assetBackedRegistry) GetSkill(string) (*models.SkillResponse, error) { return nil, nil }
func (registry *assetBackedRegistry) GetSkillVersion(string, string) (*models.SkillResponse, error) {
	return nil, nil
}
func (registry *assetBackedRegistry) GetSkillVersions(string) ([]*models.SkillResponse, error) {
	return nil, nil
}
func (registry *assetBackedRegistry) GetSkills() ([]*models.SkillResponse, error) { return nil, nil }

type recordingInstaller struct {
	sourceDir      string
	seenPackageRef string
}

func (installer *recordingInstaller) Install(skill *models.SkillResponse, targetDir string) error {
	for _, pkg := range skill.Skill.Packages {
		if strings.TrimSpace(pkg.Identifier) != "" {
			installer.seenPackageRef = pkg.Identifier
			break
		}
	}
	return copyDir(installer.sourceDir, targetDir)
}

func TestManagerAddViaAssetRegistry(t *testing.T) {
	homeDir := t.TempDir()
	fixture := createSkillFixture(t, "1.0.0", "arch/java-analyzer", "# Demo v1\n")
	archivePath := filepath.Join(t.TempDir(), "java-analyzer-1.0.0.tar.gz")
	build, err := shubskills.BuildPackage(fixture, archivePath)
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}

	registry := &assetBackedRegistry{
		assets: map[string]*models.AssetResponse{
			"arch/java-analyzer": assetRegistryResponse(build.Asset, fileURL(archivePath), time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)),
		},
		assetVersions: map[string]map[string]*models.AssetResponse{
			"arch/java-analyzer": {
				"1.0.0": assetRegistryResponse(build.Asset, fileURL(archivePath), time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)),
			},
		},
	}
	manager, err := NewManager(homeDir, registry, DefaultSourceInstaller{}, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	result, err := manager.Add("arch/java-analyzer", "")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if result.Asset.ID != "arch/java-analyzer" {
		t.Fatalf("Asset.ID = %q, want %q", result.Asset.ID, "arch/java-analyzer")
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "arch-java-analyzer.md"), "# Demo v1")
	state := readState(t, filepath.Join(homeDir, "state.json"))
	if state.Active["arch/java-analyzer"] != "1.0.0" {
		t.Fatalf("active version = %q, want %q", state.Active["arch/java-analyzer"], "1.0.0")
	}
	if findInstalledByRegistryName(state, "arch/java-analyzer", "1.0.0") == nil {
		t.Fatal("installed asset missing registryName=assetID mapping")
	}
}

func TestManagerSyncUsesIncrementalAssetCursor(t *testing.T) {
	homeDir := t.TempDir()
	fixtureV1 := createSkillFixture(t, "1.0.0", "arch/java-analyzer", "# Demo v1\n")
	archiveV1 := filepath.Join(t.TempDir(), "java-analyzer-1.0.0.tar.gz")
	buildV1, err := shubskills.BuildPackage(fixtureV1, archiveV1)
	if err != nil {
		t.Fatalf("BuildPackage(v1) error = %v", err)
	}

	fixtureV2 := createSkillFixture(t, "1.1.0", "arch/java-analyzer", "# Demo v2\n")
	archiveV2 := filepath.Join(t.TempDir(), "java-analyzer-1.1.0.tar.gz")
	buildV2, err := shubskills.BuildPackage(fixtureV2, archiveV2)
	if err != nil {
		t.Fatalf("BuildPackage(v2) error = %v", err)
	}

	updatedV1 := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	updatedV2 := updatedV1.Add(time.Hour)
	registry := &assetBackedRegistry{
		assets: map[string]*models.AssetResponse{
			"arch/java-analyzer": assetRegistryResponse(buildV2.Asset, fileURL(archiveV2), updatedV2),
		},
		assetVersions: map[string]map[string]*models.AssetResponse{
			"arch/java-analyzer": {
				"1.0.0": assetRegistryResponse(buildV1.Asset, fileURL(archiveV1), updatedV1),
				"1.1.0": assetRegistryResponse(buildV2.Asset, fileURL(archiveV2), updatedV2),
			},
		},
	}

	manager, err := NewManager(homeDir, registry, DefaultSourceInstaller{}, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if _, err := manager.Add("arch/java-analyzer", "1.0.0"); err != nil {
		t.Fatalf("Add(v1) error = %v", err)
	}

	syncResult, err := manager.Sync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if syncResult.Installed != 1 {
		t.Fatalf("Installed = %d, want 1", syncResult.Installed)
	}

	state := readState(t, filepath.Join(homeDir, "state.json"))
	if state.Sync.Cursor == "" {
		t.Fatal("sync cursor should be persisted")
	}
	if findInstalledByRegistryName(state, "arch/java-analyzer", "1.1.0") == nil {
		t.Fatal("incremental sync did not install the newer asset version")
	}
	if state.Active["arch/java-analyzer"] != "1.0.0" {
		t.Fatalf("active version = %q, want 1.0.0", state.Active["arch/java-analyzer"])
	}
}

func TestManagerAddPullsFromFallbackSourceOnRegistryMiss(t *testing.T) {
	homeDir := t.TempDir()
	fixture := createSkillFixture(t, "1.0.0", "arch/java-analyzer", "# Demo fallback\n")
	registry := &assetBackedRegistry{
		assets:        map[string]*models.AssetResponse{},
		assetVersions: map[string]map[string]*models.AssetResponse{},
	}
	registry.pullFn = func(sourceName, assetID, version string) (*models.AssetResponse, error) {
		if sourceName != "github-main" {
			t.Fatalf("sourceName = %q, want github-main", sourceName)
		}
		if assetID != "arch/java-analyzer" {
			t.Fatalf("assetID = %q, want arch/java-analyzer", assetID)
		}
		if version != "1.0.0" {
			t.Fatalf("version = %q, want 1.0.0", version)
		}
		asset, err := shubskills.LoadAssetDir(fixture)
		if err != nil {
			t.Fatalf("LoadAssetDir() error = %v", err)
		}
		asset.Source = &models.AssetSource{
			PackageType: "tarball",
			PackageRef:  "/v0/assets/arch%2Fjava-analyzer/versions/1.0.0/package",
		}
		return &models.AssetResponse{
			Asset: *asset,
			Meta: models.AssetResponseMeta{Official: &models.AssetRegistryExtensions{
				Status:      "active",
				PublishedAt: time.Date(2026, time.April, 25, 0, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2026, time.April, 25, 0, 0, 0, 0, time.UTC),
				IsLatest:    true,
			}},
		}, nil
	}

	installer := &recordingInstaller{sourceDir: fixture}
	manager, err := NewManager(homeDir, registry, installer, "https://registry.example.com/v0")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	result, err := manager.AddWithOptions("arch/java-analyzer", AddOptions{
		Version:         "1.0.0",
		FallbackSources: []string{"github-main"},
	})
	if err != nil {
		t.Fatalf("AddWithOptions() error = %v", err)
	}
	if result.Asset.ID != "arch/java-analyzer" {
		t.Fatalf("Asset.ID = %q, want arch/java-analyzer", result.Asset.ID)
	}
	wantPackageRef := "https://registry.example.com/v0/assets/arch%2Fjava-analyzer/versions/1.0.0/package"
	if installer.seenPackageRef != wantPackageRef {
		t.Fatalf("package ref = %q, want %q", installer.seenPackageRef, wantPackageRef)
	}
}

func TestManagerAddPullsFromAutomaticFallbackPoolOnRegistryMiss(t *testing.T) {
	homeDir := t.TempDir()
	fixture := createSkillFixture(t, "1.0.0", "arch/java-analyzer", "# Demo automatic fallback\n")
	pullAttempts := make([]string, 0, 4)
	registry := &assetBackedRegistry{
		assets:        map[string]*models.AssetResponse{},
		assetVersions: map[string]map[string]*models.AssetResponse{},
		sources: []*models.SHUBSource{
			{Name: "company-gitlab", Address: "https://gitlab.example.com/acme/skills/-/tree/main/skills/{name}"},
			{Name: fallbackSourceAnthropicSkills, Address: "https://github.com/anthropics/skills/tree/main/skills/{name}", Provider: "github", BuiltIn: true},
			{Name: fallbackSourceGitHubPluginMain, Address: "https://github.com/{asset}/tree/main/plugins/{name}/skills/{name}", Provider: "github", BuiltIn: true},
			{Name: fallbackSourceGitHubSkillsMain, Address: "https://github.com/{asset}/tree/main/skills/{name}", Provider: "github", BuiltIn: true},
			{Name: fallbackSourceGitHubDirect, Address: "https://github.com/{asset}", Provider: "github", BuiltIn: true},
		},
	}
	registry.pullFn = func(sourceName, assetID, version string) (*models.AssetResponse, error) {
		pullAttempts = append(pullAttempts, sourceName)
		if sourceName != fallbackSourceGitHubPluginMain {
			return nil, nil
		}
		asset, err := shubskills.LoadAssetDir(fixture)
		if err != nil {
			t.Fatalf("LoadAssetDir() error = %v", err)
		}
		asset.Source = &models.AssetSource{
			PackageType: "tarball",
			PackageRef:  "/v0/assets/arch%2Fjava-analyzer/versions/1.0.0/package",
		}
		return &models.AssetResponse{
			Asset: *asset,
			Meta: models.AssetResponseMeta{Official: &models.AssetRegistryExtensions{
				Status:      "active",
				PublishedAt: time.Date(2026, time.April, 26, 0, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2026, time.April, 26, 0, 0, 0, 0, time.UTC),
				IsLatest:    true,
			}},
		}, nil
	}

	installer := &recordingInstaller{sourceDir: fixture}
	manager, err := NewManager(homeDir, registry, installer, "https://registry.example.com/v0")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	result, err := manager.AddWithOptions("arch/java-analyzer", AddOptions{Version: "1.0.0"})
	if err != nil {
		t.Fatalf("AddWithOptions() error = %v", err)
	}
	if result.Asset.ID != "arch/java-analyzer" {
		t.Fatalf("Asset.ID = %q, want arch/java-analyzer", result.Asset.ID)
	}
	if len(pullAttempts) != 3 {
		t.Fatalf("pull attempts = %#v, want 3 attempts", pullAttempts)
	}
	if pullAttempts[0] != fallbackSourceGitHubDirect || pullAttempts[1] != fallbackSourceGitHubSkillsMain || pullAttempts[2] != fallbackSourceGitHubPluginMain {
		t.Fatalf("pull attempts = %#v, want github-direct then github-skills-main then github-plugin-skills-main", pullAttempts)
	}
}

func TestManagerAddWithGitHubFlagFiltersNonGitHubFallbackSources(t *testing.T) {
	homeDir := t.TempDir()
	fixture := createSkillFixture(t, "1.0.0", "arch/java-analyzer", "# Demo github fallback\n")
	pullAttempts := make([]string, 0, 4)
	registry := &assetBackedRegistry{
		assets:        map[string]*models.AssetResponse{},
		assetVersions: map[string]map[string]*models.AssetResponse{},
		sources: []*models.SHUBSource{
			{Name: "company-gitlab", Address: "https://gitlab.example.com/acme/skills/-/tree/main/skills/{name}"},
			{Name: fallbackSourceGitHubPluginMain, Address: "https://github.com/{asset}/tree/main/plugins/{name}/skills/{name}", Provider: "github", BuiltIn: true},
			{Name: fallbackSourceGitHubSkillsMain, Address: "https://github.com/{asset}/tree/main/skills/{name}", Provider: "github", BuiltIn: true},
			{Name: fallbackSourceGitHubDirect, Address: "https://github.com/{asset}", Provider: "github", BuiltIn: true},
		},
	}
	registry.pullFn = func(sourceName, assetID, version string) (*models.AssetResponse, error) {
		pullAttempts = append(pullAttempts, sourceName)
		if sourceName != fallbackSourceGitHubDirect {
			return nil, nil
		}
		asset, err := shubskills.LoadAssetDir(fixture)
		if err != nil {
			t.Fatalf("LoadAssetDir() error = %v", err)
		}
		asset.Source = &models.AssetSource{
			PackageType: "tarball",
			PackageRef:  "/v0/assets/arch%2Fjava-analyzer/versions/1.0.0/package",
		}
		return &models.AssetResponse{
			Asset: *asset,
			Meta: models.AssetResponseMeta{Official: &models.AssetRegistryExtensions{
				Status:      "active",
				PublishedAt: time.Date(2026, time.April, 26, 0, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2026, time.April, 26, 0, 0, 0, 0, time.UTC),
				IsLatest:    true,
			}},
		}, nil
	}

	installer := &recordingInstaller{sourceDir: fixture}
	manager, err := NewManager(homeDir, registry, installer, "https://registry.example.com/v0")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.AddWithOptions("arch/java-analyzer", AddOptions{Version: "1.0.0", GitHub: true}); err != nil {
		t.Fatalf("AddWithOptions() error = %v", err)
	}
	if len(pullAttempts) == 0 {
		t.Fatal("expected GitHub fallback attempts")
	}
	for _, sourceName := range pullAttempts {
		if sourceName == "company-gitlab" {
			t.Fatalf("pull attempts = %#v, did not expect gitlab source when GitHub mode is enabled", pullAttempts)
		}
	}
}

func TestManagerDoctorRepairsMissingInstallDirFromAssetRegistry(t *testing.T) {
	homeDir := t.TempDir()
	fixture := createSkillFixture(t, "1.0.0", "arch/java-analyzer", "# Demo v1\n")
	archivePath := filepath.Join(t.TempDir(), "java-analyzer-1.0.0.tar.gz")
	build, err := shubskills.BuildPackage(fixture, archivePath)
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}

	registry := &assetBackedRegistry{
		assets: map[string]*models.AssetResponse{
			"arch/java-analyzer": assetRegistryResponse(build.Asset, fileURL(archivePath), time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)),
		},
		assetVersions: map[string]map[string]*models.AssetResponse{
			"arch/java-analyzer": {
				"1.0.0": assetRegistryResponse(build.Asset, fileURL(archivePath), time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)),
			},
		},
	}
	manager, err := NewManager(homeDir, registry, DefaultSourceInstaller{}, "http://localhost:12121")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	result, err := manager.Add("arch/java-analyzer", "")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := os.RemoveAll(result.InstallDir); err != nil {
		t.Fatalf("remove install dir: %v", err)
	}

	doctorResult, err := manager.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if doctorResult.Repaired == 0 {
		t.Fatal("Doctor() should report a repaired installation")
	}
	assertFileContains(t, filepath.Join(homeDir, "exports", "arch-java-analyzer.md"), "# Demo v1")
	if _, err := os.Stat(result.InstallDir); err != nil {
		t.Fatalf("install dir not repaired: %v", err)
	}
}

func assetRegistryResponse(asset *models.Asset, packageURL string, updatedAt time.Time) *models.AssetResponse {
	cloned := *asset
	cloned.Source = &models.AssetSource{PackageType: "tarball", PackageRef: packageURL}
	return &models.AssetResponse{
		Asset: cloned,
		Meta: models.AssetResponseMeta{Official: &models.AssetRegistryExtensions{
			Status:      "active",
			PublishedAt: updatedAt,
			UpdatedAt:   updatedAt,
			IsLatest:    true,
		}},
	}
}

func compareTestAssetWithCursor(asset *models.AssetResponse, cursor string) int {
	if asset == nil {
		return -1
	}
	parts := strings.SplitN(cursor, ":", 2)
	cursorID := cursor
	cursorVersion := ""
	if len(parts) == 2 {
		cursorID = parts[0]
		cursorVersion = parts[1]
	}
	if cmp := strings.Compare(asset.Asset.ID, cursorID); cmp != 0 {
		return cmp
	}
	return strings.Compare(asset.Asset.Version, cursorVersion)
}

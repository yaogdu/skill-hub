package asset

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	skillsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/skill"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
)

type fakeSkillsRegistry struct {
	listSkillsFn       func(ctx context.Context, filter *database.SkillFilter, cursor string, limit int) ([]*models.SkillResponse, string, error)
	getSkillFn         func(ctx context.Context, skillName string) (*models.SkillResponse, error)
	getSkillVersionFn  func(ctx context.Context, skillName, version string) (*models.SkillResponse, error)
	getSkillVersionsFn func(ctx context.Context, skillName string) ([]*models.SkillResponse, error)
	publishSkillFn     func(ctx context.Context, skill *models.SkillJSON) (*models.SkillResponse, error)
	applySkillFn       func(ctx context.Context, skill *models.SkillJSON) (*models.SkillResponse, error)
}

var _ skillsvc.Registry = (*fakeSkillsRegistry)(nil)

func (registry *fakeSkillsRegistry) ListSkills(ctx context.Context, filter *database.SkillFilter, cursor string, limit int) ([]*models.SkillResponse, string, error) {
	if registry.listSkillsFn != nil {
		return registry.listSkillsFn(ctx, filter, cursor, limit)
	}
	return nil, "", nil
}

func (registry *fakeSkillsRegistry) GetSkill(ctx context.Context, skillName string) (*models.SkillResponse, error) {
	if registry.getSkillFn != nil {
		return registry.getSkillFn(ctx, skillName)
	}
	return nil, nil
}

func (registry *fakeSkillsRegistry) GetSkillVersion(ctx context.Context, skillName, version string) (*models.SkillResponse, error) {
	if registry.getSkillVersionFn != nil {
		return registry.getSkillVersionFn(ctx, skillName, version)
	}
	return nil, nil
}

func (registry *fakeSkillsRegistry) GetSkillVersions(ctx context.Context, skillName string) ([]*models.SkillResponse, error) {
	if registry.getSkillVersionsFn != nil {
		return registry.getSkillVersionsFn(ctx, skillName)
	}
	return nil, nil
}

func (registry *fakeSkillsRegistry) PublishSkill(ctx context.Context, skill *models.SkillJSON) (*models.SkillResponse, error) {
	if registry.publishSkillFn != nil {
		return registry.publishSkillFn(ctx, skill)
	}
	return nil, nil
}

func (registry *fakeSkillsRegistry) ApplySkill(ctx context.Context, skill *models.SkillJSON) (*models.SkillResponse, error) {
	if registry.applySkillFn != nil {
		return registry.applySkillFn(ctx, skill)
	}
	return nil, nil
}

func (registry *fakeSkillsRegistry) DeleteSkill(context.Context, string, string) error {
	return nil
}

type fakeAgentsReader struct {
	listAgentsFn                func(ctx context.Context, filter *database.AgentFilter, cursor string, limit int) ([]*models.AgentResponse, string, error)
	getAgentFn                  func(ctx context.Context, agentName string) (*models.AgentResponse, error)
	getAgentVersionFn           func(ctx context.Context, agentName, version string) (*models.AgentResponse, error)
	getAgentVersionsFn          func(ctx context.Context, agentName string) ([]*models.AgentResponse, error)
	getAgentEmbeddingMetadataFn func(ctx context.Context, agentName, version string) (*database.SemanticEmbeddingMetadata, error)
}

func (reader *fakeAgentsReader) ListAgents(ctx context.Context, filter *database.AgentFilter, cursor string, limit int) ([]*models.AgentResponse, string, error) {
	if reader.listAgentsFn != nil {
		return reader.listAgentsFn(ctx, filter, cursor, limit)
	}
	return nil, "", nil
}

func (reader *fakeAgentsReader) GetAgent(ctx context.Context, agentName string) (*models.AgentResponse, error) {
	if reader.getAgentFn != nil {
		return reader.getAgentFn(ctx, agentName)
	}
	return nil, database.ErrNotFound
}

func (reader *fakeAgentsReader) GetAgentVersion(ctx context.Context, agentName, version string) (*models.AgentResponse, error) {
	if reader.getAgentVersionFn != nil {
		return reader.getAgentVersionFn(ctx, agentName, version)
	}
	return nil, database.ErrNotFound
}

func (reader *fakeAgentsReader) GetAgentVersions(ctx context.Context, agentName string) ([]*models.AgentResponse, error) {
	if reader.getAgentVersionsFn != nil {
		return reader.getAgentVersionsFn(ctx, agentName)
	}
	return nil, database.ErrNotFound
}

func (reader *fakeAgentsReader) GetAgentEmbeddingMetadata(ctx context.Context, agentName, version string) (*database.SemanticEmbeddingMetadata, error) {
	if reader.getAgentEmbeddingMetadataFn != nil {
		return reader.getAgentEmbeddingMetadataFn(ctx, agentName, version)
	}
	return nil, nil
}

type fakePromptsReader struct {
	listPromptsFn       func(ctx context.Context, filter *database.PromptFilter, cursor string, limit int) ([]*models.PromptResponse, string, error)
	getPromptFn         func(ctx context.Context, promptName string) (*models.PromptResponse, error)
	getPromptVersionFn  func(ctx context.Context, promptName, version string) (*models.PromptResponse, error)
	getPromptVersionsFn func(ctx context.Context, promptName string) ([]*models.PromptResponse, error)
}

func (reader *fakePromptsReader) ListPrompts(ctx context.Context, filter *database.PromptFilter, cursor string, limit int) ([]*models.PromptResponse, string, error) {
	if reader.listPromptsFn != nil {
		return reader.listPromptsFn(ctx, filter, cursor, limit)
	}
	return nil, "", nil
}

func (reader *fakePromptsReader) GetPrompt(ctx context.Context, promptName string) (*models.PromptResponse, error) {
	if reader.getPromptFn != nil {
		return reader.getPromptFn(ctx, promptName)
	}
	return nil, database.ErrNotFound
}

func (reader *fakePromptsReader) GetPromptVersion(ctx context.Context, promptName, version string) (*models.PromptResponse, error) {
	if reader.getPromptVersionFn != nil {
		return reader.getPromptVersionFn(ctx, promptName, version)
	}
	return nil, database.ErrNotFound
}

func (reader *fakePromptsReader) GetPromptVersions(ctx context.Context, promptName string) ([]*models.PromptResponse, error) {
	if reader.getPromptVersionsFn != nil {
		return reader.getPromptVersionsFn(ctx, promptName)
	}
	return nil, database.ErrNotFound
}

type fakeAssetStore struct {
	listAssetsFn              func(ctx context.Context, filter *database.AssetFilter, cursor string, limit int) ([]*models.AssetResponse, string, error)
	getAssetFn                func(ctx context.Context, assetID string) (*models.AssetResponse, error)
	getAssetVersionFn         func(ctx context.Context, assetID, version string) (*models.AssetResponse, error)
	getAssetVersionsFn        func(ctx context.Context, assetID string) ([]*models.AssetResponse, error)
	createAssetFn             func(ctx context.Context, asset *models.Asset, officialMeta *models.AssetRegistryExtensions) (*models.AssetResponse, error)
	updateAssetFn             func(ctx context.Context, assetID, version string, asset *models.Asset, officialMeta *models.AssetRegistryExtensions) (*models.AssetResponse, error)
	deleteAssetFn             func(ctx context.Context, assetID, version string) error
	getLatestAssetFn          func(ctx context.Context, assetID string) (*models.AssetResponse, error)
	countAssetVersionsFn      func(ctx context.Context, assetID string) (int, error)
	checkAssetVersionExistsFn func(ctx context.Context, assetID, version string) (bool, error)
	unmarkAssetAsLatestFn     func(ctx context.Context, assetID string) error
}

type fakePackageStore struct {
	putFn func(ctx context.Context, assetID, version string, content []byte, uploadedAt time.Time) (*models.AssetPackage, error)
	getFn func(ctx context.Context, assetID, version string) (*models.AssetPackageDownload, error)
}

func (store *fakePackageStore) Put(ctx context.Context, assetID, version string, content []byte, uploadedAt time.Time) (*models.AssetPackage, error) {
	if store.putFn != nil {
		return store.putFn(ctx, assetID, version, content, uploadedAt)
	}
	return nil, fmt.Errorf("not implemented")
}

func (store *fakePackageStore) Get(ctx context.Context, assetID, version string) (*models.AssetPackageDownload, error) {
	if store.getFn != nil {
		return store.getFn(ctx, assetID, version)
	}
	return nil, database.ErrNotFound
}

func (store *fakeAssetStore) ListAssets(ctx context.Context, filter *database.AssetFilter, cursor string, limit int) ([]*models.AssetResponse, string, error) {
	if store.listAssetsFn != nil {
		return store.listAssetsFn(ctx, filter, cursor, limit)
	}
	return nil, "", nil
}

func (store *fakeAssetStore) GetAsset(ctx context.Context, assetID string) (*models.AssetResponse, error) {
	if store.getAssetFn != nil {
		return store.getAssetFn(ctx, assetID)
	}
	return nil, database.ErrNotFound
}

func (store *fakeAssetStore) GetAssetVersion(ctx context.Context, assetID, version string) (*models.AssetResponse, error) {
	if store.getAssetVersionFn != nil {
		return store.getAssetVersionFn(ctx, assetID, version)
	}
	return nil, database.ErrNotFound
}

func (store *fakeAssetStore) GetAssetVersions(ctx context.Context, assetID string) ([]*models.AssetResponse, error) {
	if store.getAssetVersionsFn != nil {
		return store.getAssetVersionsFn(ctx, assetID)
	}
	return nil, database.ErrNotFound
}

func (store *fakeAssetStore) CreateAsset(ctx context.Context, asset *models.Asset, officialMeta *models.AssetRegistryExtensions) (*models.AssetResponse, error) {
	if store.createAssetFn != nil {
		return store.createAssetFn(ctx, asset, officialMeta)
	}
	return nil, nil
}

func (store *fakeAssetStore) UpdateAsset(ctx context.Context, assetID, version string, asset *models.Asset, officialMeta *models.AssetRegistryExtensions) (*models.AssetResponse, error) {
	if store.updateAssetFn != nil {
		return store.updateAssetFn(ctx, assetID, version, asset, officialMeta)
	}
	return nil, database.ErrNotFound
}

func (store *fakeAssetStore) DeleteAsset(ctx context.Context, assetID, version string) error {
	if store.deleteAssetFn != nil {
		return store.deleteAssetFn(ctx, assetID, version)
	}
	return database.ErrNotFound
}

func (store *fakeAssetStore) GetLatestAsset(ctx context.Context, assetID string) (*models.AssetResponse, error) {
	if store.getLatestAssetFn != nil {
		return store.getLatestAssetFn(ctx, assetID)
	}
	return nil, database.ErrNotFound
}

func (store *fakeAssetStore) CountAssetVersions(ctx context.Context, assetID string) (int, error) {
	if store.countAssetVersionsFn != nil {
		return store.countAssetVersionsFn(ctx, assetID)
	}
	return 0, nil
}

func (store *fakeAssetStore) CheckAssetVersionExists(ctx context.Context, assetID, version string) (bool, error) {
	if store.checkAssetVersionExistsFn != nil {
		return store.checkAssetVersionExistsFn(ctx, assetID, version)
	}
	return false, nil
}

func (store *fakeAssetStore) UnmarkAssetAsLatest(ctx context.Context, assetID string) error {
	if store.unmarkAssetAsLatestFn != nil {
		return store.unmarkAssetAsLatestFn(ctx, assetID)
	}
	return nil
}

func TestListAssets_ReturnsOnlySHUBSkills(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	service := New(Dependencies{Skills: &fakeSkillsRegistry{
		listSkillsFn: func(context.Context, *database.SkillFilter, string, int) ([]*models.SkillResponse, string, error) {
			return []*models.SkillResponse{
				legacySkillResponse("legacy-skill", "1.0.0"),
				shubSkillResponse("java-analyzer", "arch/java-analyzer", "1.2.0", now),
			}, "", nil
		},
	}})

	search := "java"
	assets, next, err := service.ListAssets(context.Background(), &Filter{Search: &search}, "", 30)
	if err != nil {
		t.Fatalf("ListAssets() error = %v", err)
	}
	if next != "" {
		t.Fatalf("NextCursor = %q, want empty", next)
	}
	if len(assets) != 1 {
		t.Fatalf("len(assets) = %d, want 1", len(assets))
	}
	if assets[0].Asset.ID != "arch/java-analyzer" {
		t.Fatalf("Asset.ID = %q, want %q", assets[0].Asset.ID, "arch/java-analyzer")
	}
}

func TestListAssets_FiltersNativeSkillCategory(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	service := New(Dependencies{Skills: &fakeSkillsRegistry{
		listSkillsFn: func(context.Context, *database.SkillFilter, string, int) ([]*models.SkillResponse, string, error) {
			return []*models.SkillResponse{
				shubSkillResponseWithCategory("java-analyzer", "arch/java-analyzer", "1.2.0", models.AssetCategorySkill, now),
				shubSkillResponse("prompt-helper", "arch/prompt-helper", "1.0.0", now),
			}, "", nil
		},
	}})

	category := models.AssetCategorySkill
	assets, next, err := service.ListAssets(context.Background(), &Filter{Category: &category}, "", 30)
	if err != nil {
		t.Fatalf("ListAssets() error = %v", err)
	}
	if next != "" {
		t.Fatalf("NextCursor = %q, want empty", next)
	}
	if len(assets) != 1 {
		t.Fatalf("len(assets) = %d, want 1", len(assets))
	}
	if assets[0].Asset.Category != models.AssetCategorySkill {
		t.Fatalf("asset category = %q, want %q", assets[0].Asset.Category, models.AssetCategorySkill)
	}
}

func TestUploadAssetPackage_ValidatesAndStoresArchive(t *testing.T) {
	archivePath := buildTestPackageArchive(t, "arch/java-analyzer", "1.2.0")
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read package archive: %v", err)
	}

	service := New(Dependencies{
		Packages: &fakePackageStore{putFn: func(_ context.Context, assetID, version string, content []byte, uploadedAt time.Time) (*models.AssetPackage, error) {
			if assetID != "arch/java-analyzer" {
				t.Fatalf("assetID = %q, want arch/java-analyzer", assetID)
			}
			if version != "1.2.0" {
				t.Fatalf("version = %q, want 1.2.0", version)
			}
			if string(content) != string(archiveBytes) {
				t.Fatal("stored package content mismatch")
			}
			if uploadedAt.IsZero() {
				t.Fatal("uploadedAt should be set")
			}
			return &models.AssetPackage{AssetID: assetID, Version: version, ContentType: "application/gzip", SizeBytes: len(content)}, nil
		}},
	})

	response, err := service.UploadAssetPackage(context.Background(), "arch/java-analyzer", "1.2.0", archiveBytes, "application/gzip")
	if err != nil {
		t.Fatalf("UploadAssetPackage() error = %v", err)
	}
	if response.Package.AssetID != "arch/java-analyzer" {
		t.Fatalf("AssetID = %q, want arch/java-analyzer", response.Package.AssetID)
	}
}

func TestUploadAssetPackage_RejectsMismatchedArchiveMetadata(t *testing.T) {
	archivePath := buildTestPackageArchive(t, "arch/java-analyzer", "1.2.0")
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read package archive: %v", err)
	}

	service := New(Dependencies{Packages: &fakePackageStore{}})
	_, err = service.UploadAssetPackage(context.Background(), "arch/other", "1.2.0", archiveBytes, "application/gzip")
	if err == nil {
		t.Fatal("expected metadata mismatch error, got nil")
	}
	if got := err.Error(); got == "" || !containsAll(got, "does not match request", "arch/other") {
		t.Fatalf("error = %q, want mismatch details", got)
	}
}

func TestGetAssetPackage_ResolvesLatestVersion(t *testing.T) {
	service := New(Dependencies{
		Assets: &fakeAssetStore{getAssetFn: func(_ context.Context, assetID string) (*models.AssetResponse, error) {
			return &models.AssetResponse{Asset: models.Asset{ID: assetID, Version: "1.2.0"}}, nil
		}},
		Packages: &fakePackageStore{getFn: func(_ context.Context, assetID, version string) (*models.AssetPackageDownload, error) {
			if version != "1.2.0" {
				t.Fatalf("version = %q, want resolved latest 1.2.0", version)
			}
			return &models.AssetPackageDownload{
				Package: models.AssetPackage{AssetID: assetID, Version: version, ContentType: "application/gzip", SizeBytes: len("pkg")},
				Content: []byte("pkg"),
			}, nil
		}},
	})

	pkg, err := service.GetAssetPackage(context.Background(), "arch/java-analyzer", "latest")
	if err != nil {
		t.Fatalf("GetAssetPackage() error = %v", err)
	}
	if string(pkg.Content) != "pkg" {
		t.Fatalf("content = %q, want pkg", string(pkg.Content))
	}
}

func buildTestPackageArchive(t *testing.T, assetID, version string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(fmt.Sprintf(`---
name: java-analyzer
description: Analyze Java services
version: %s
allowed-tools:
  - Read
shub:
  schemaVersion: shub.skill/v1alpha1
  id: %s
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
---
# Java Analyzer
`, version, assetID)), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "package.tar.gz")
	if _, err := shubskills.BuildPackage(dir, archivePath); err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	return archivePath
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func TestGetAssetVersion_FindsByAssetID(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	service := New(Dependencies{Skills: &fakeSkillsRegistry{
		listSkillsFn: func(context.Context, *database.SkillFilter, string, int) ([]*models.SkillResponse, string, error) {
			return []*models.SkillResponse{
				shubSkillResponse("java-analyzer", "arch/java-analyzer", "1.1.0", now.Add(-time.Hour)),
				shubSkillResponse("java-analyzer", "arch/java-analyzer", "1.2.0", now),
			}, "", nil
		},
	}})

	asset, err := service.GetAssetVersion(context.Background(), "arch/java-analyzer", "1.2.0")
	if err != nil {
		t.Fatalf("GetAssetVersion() error = %v", err)
	}
	if asset.Asset.Version != "1.2.0" {
		t.Fatalf("Version = %q, want %q", asset.Asset.Version, "1.2.0")
	}
}

func TestPublishAsset_UsesAssetStoreWhenConfigured(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	var created *models.Asset
	service := New(Dependencies{Assets: &fakeAssetStore{
		countAssetVersionsFn:      func(context.Context, string) (int, error) { return 0, nil },
		checkAssetVersionExistsFn: func(context.Context, string, string) (bool, error) { return false, nil },
		getLatestAssetFn:          func(context.Context, string) (*models.AssetResponse, error) { return nil, database.ErrNotFound },
		createAssetFn: func(_ context.Context, asset *models.Asset, officialMeta *models.AssetRegistryExtensions) (*models.AssetResponse, error) {
			created = asset
			return &models.AssetResponse{Asset: *asset, Meta: models.AssetResponseMeta{Official: &models.AssetRegistryExtensions{Status: officialMeta.Status, PublishedAt: now, UpdatedAt: now, IsLatest: true}}}, nil
		},
	}})

	response, err := service.PublishAsset(context.Background(), &models.AssetPublishRequest{
		Manifest: models.AssetManifest{
			SchemaVersion: models.ShubAssetSchemaVersion,
			ID:            "arch/java-analyzer",
			Category:      models.AssetCategoryPrompt,
			Name:          "java-analyzer",
			Description:   "Analyze Java services",
			Version:       "1.2.0",
			SourceSkill:   models.AssetSourceSkill{Path: models.SkillFileName, Body: "# Java Analyzer", BodyFormat: "markdown"},
			Entry:         models.AssetEntry{Kind: "skill-body", Path: models.SkillFileName},
			Runtime:       models.AssetRuntime{Type: "none"},
		},
	})
	if err != nil {
		t.Fatalf("PublishAsset() error = %v", err)
	}
	if created == nil || created.ID != "arch/java-analyzer" {
		t.Fatalf("created asset = %#v, want persisted asset", created)
	}
	if response.Asset.ID != "arch/java-analyzer" {
		t.Fatalf("response asset id = %q, want %q", response.Asset.ID, "arch/java-analyzer")
	}
}

func TestPublishAsset_PublishesCompatibilitySkill(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	var published *models.SkillJSON
	service := New(Dependencies{Skills: &fakeSkillsRegistry{
		publishSkillFn: func(_ context.Context, skill *models.SkillJSON) (*models.SkillResponse, error) {
			published = skill
			return &models.SkillResponse{
				Skill: *skill,
				Meta:  models.SkillResponseMeta{Official: &models.SkillRegistryExtensions{Status: "active", PublishedAt: now, UpdatedAt: now, IsLatest: true}},
			}, nil
		},
	}})

	response, err := service.PublishAsset(context.Background(), &models.AssetPublishRequest{
		Manifest: models.AssetManifest{
			SchemaVersion: models.ShubAssetSchemaVersion,
			ID:            "arch/java-analyzer",
			Category:      models.AssetCategoryPrompt,
			Name:          "java-analyzer",
			Description:   "Analyze Java services",
			Version:       "1.2.0",
			SourceSkill:   models.AssetSourceSkill{Path: models.SkillFileName, Body: "# Java Analyzer", BodyFormat: "markdown"},
			Entry:         models.AssetEntry{Kind: "skill-body", Path: models.SkillFileName},
			Runtime:       models.AssetRuntime{Type: "none"},
		},
		Source: &models.AssetSource{RepositoryURL: "https://gitlab.example.com/arch/java-analyzer", PackageType: "tarball", PackageRef: "https://gitlab.example.com/pkg/java-analyzer-1.2.0.tgz"},
	})
	if err != nil {
		t.Fatalf("PublishAsset() error = %v", err)
	}
	if published == nil || published.SHUB == nil || published.SHUB.AssetID != "arch/java-analyzer" {
		t.Fatalf("published skill payload = %#v, want SHUB metadata", published)
	}
	if response.Asset.ID != "arch/java-analyzer" {
		t.Fatalf("response asset id = %q, want %q", response.Asset.ID, "arch/java-analyzer")
	}
}

func legacySkillResponse(name, version string) *models.SkillResponse {
	return &models.SkillResponse{Skill: models.SkillJSON{Name: name, Description: name, Version: version}}
}

func shubSkillResponse(name, assetID, version string, updatedAt time.Time) *models.SkillResponse {
	return shubSkillResponseWithCategory(name, assetID, version, models.AssetCategoryPrompt, updatedAt)
}

func shubSkillResponseWithCategory(name, assetID, version string, category models.AssetCategory, updatedAt time.Time) *models.SkillResponse {
	return &models.SkillResponse{
		Skill: models.SkillJSON{
			Name:        name,
			Description: "Analyze Java services",
			Version:     version,
			SHUB: &models.SkillSHUBMetadata{
				SchemaVersion: models.ShubAssetSchemaVersion,
				AssetID:       assetID,
				Category:      category,
				Manifest: &models.AssetManifest{
					SchemaVersion: models.ShubAssetSchemaVersion,
					ID:            assetID,
					Category:      category,
					Name:          name,
					Description:   "Analyze Java services",
					Version:       version,
					SourceSkill:   models.AssetSourceSkill{Path: models.SkillFileName, Body: "# Java Analyzer", BodyFormat: "markdown"},
					Entry:         models.AssetEntry{Kind: "skill-body", Path: models.SkillFileName},
					Runtime:       models.AssetRuntime{Type: "none"},
				},
				Source: &models.AssetSource{PackageType: "tarball", PackageRef: "file:///tmp/demo.tar.gz"},
			},
		},
		Meta: models.SkillResponseMeta{Official: &models.SkillRegistryExtensions{Status: "active", UpdatedAt: updatedAt, PublishedAt: updatedAt, IsLatest: true}},
	}
}

func TestPublishAsset_MirrorsCompatibilitySkillWhenAssetStoreIsPrimary(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	var mirrored *models.SkillJSON
	service := New(Dependencies{
		Assets: &fakeAssetStore{
			countAssetVersionsFn:      func(context.Context, string) (int, error) { return 0, nil },
			checkAssetVersionExistsFn: func(context.Context, string, string) (bool, error) { return false, nil },
			getLatestAssetFn:          func(context.Context, string) (*models.AssetResponse, error) { return nil, database.ErrNotFound },
			createAssetFn: func(_ context.Context, asset *models.Asset, officialMeta *models.AssetRegistryExtensions) (*models.AssetResponse, error) {
				return &models.AssetResponse{Asset: *asset, Meta: models.AssetResponseMeta{Official: &models.AssetRegistryExtensions{Status: officialMeta.Status, PublishedAt: now, UpdatedAt: now, IsLatest: true}}}, nil
			},
		},
		Skills: &fakeSkillsRegistry{
			applySkillFn: func(_ context.Context, skill *models.SkillJSON) (*models.SkillResponse, error) {
				mirrored = skill
				return &models.SkillResponse{Skill: *skill}, nil
			},
		},
	})

	response, err := service.PublishAsset(context.Background(), &models.AssetPublishRequest{
		Manifest: models.AssetManifest{
			SchemaVersion: models.ShubAssetSchemaVersion,
			ID:            "arch/java-analyzer",
			Category:      models.AssetCategoryPrompt,
			Name:          "java-analyzer",
			Description:   "Analyze Java services",
			Version:       "1.2.0",
			SourceSkill:   models.AssetSourceSkill{Path: models.SkillFileName, Body: "# Java Analyzer", BodyFormat: "markdown"},
			Entry:         models.AssetEntry{Kind: "skill-body", Path: models.SkillFileName},
			Runtime:       models.AssetRuntime{Type: "none"},
		},
	})
	if err != nil {
		t.Fatalf("PublishAsset() error = %v", err)
	}
	if mirrored == nil || mirrored.SHUB == nil || mirrored.SHUB.AssetID != "arch/java-analyzer" {
		t.Fatalf("mirrored skill payload = %#v, want SHUB metadata", mirrored)
	}
	if response.Asset.ID != "arch/java-analyzer" {
		t.Fatalf("response asset id = %q, want %q", response.Asset.ID, "arch/java-analyzer")
	}
}

func legacyAgentResponse(name, version string, updatedAt time.Time, isLatest bool) *models.AgentResponse {
	return &models.AgentResponse{
		Agent: models.AgentJSON{
			AgentManifest: models.AgentManifest{
				Name:          name,
				Image:         "ghcr.io/acme/" + name + ":" + version,
				Language:      "python",
				Framework:     "adk",
				ModelProvider: "openai",
				ModelName:     "gpt-4o",
				Description:   "Legacy agent " + name,
			},
			Title:   name + " title",
			Version: version,
		},
		Meta: models.AgentResponseMeta{Official: &models.AgentRegistryExtensions{Status: "active", UpdatedAt: updatedAt, PublishedAt: updatedAt, IsLatest: isLatest}},
	}
}

func TestListAssets_FallsBackToLegacyAgents(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	service := New(Dependencies{
		Agents: &fakeAgentsReader{
			listAgentsFn: func(_ context.Context, filter *database.AgentFilter, cursor string, limit int) ([]*models.AgentResponse, string, error) {
				if filter == nil || filter.SubstringName == nil || *filter.SubstringName != "agent" {
					t.Fatalf("unexpected filter = %#v", filter)
				}
				if cursor != "" {
					return nil, "", nil
				}
				return []*models.AgentResponse{legacyAgentResponse("assistant-agent", "1.0.0", now, true)}, "", nil
			},
		},
	})

	search := "agent"
	category := models.AssetCategoryAgent
	assets, nextCursor, err := service.ListAssets(context.Background(), &Filter{Search: &search, Category: &category}, "", 10)
	if err != nil {
		t.Fatalf("ListAssets() error = %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("nextCursor = %q, want empty", nextCursor)
	}
	if len(assets) != 1 {
		t.Fatalf("len(assets) = %d, want 1", len(assets))
	}
	if assets[0].Asset.Category != models.AssetCategoryAgent {
		t.Fatalf("asset category = %q, want %q", assets[0].Asset.Category, models.AssetCategoryAgent)
	}
	if assets[0].Asset.ID != "assistant-agent" {
		t.Fatalf("asset id = %q, want %q", assets[0].Asset.ID, "assistant-agent")
	}
}

func TestGetAssetVersion_FallsBackToLegacyAgent(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	service := New(Dependencies{
		Agents: &fakeAgentsReader{
			listAgentsFn: func(_ context.Context, filter *database.AgentFilter, cursor string, limit int) ([]*models.AgentResponse, string, error) {
				if cursor != "" {
					return nil, "", nil
				}
				return []*models.AgentResponse{legacyAgentResponse("assistant-agent", "1.0.0", now, true)}, "", nil
			},
		},
	})

	asset, err := service.GetAssetVersion(context.Background(), "assistant-agent", "1.0.0")
	if err != nil {
		t.Fatalf("GetAssetVersion() error = %v", err)
	}
	if asset.Asset.ID != "assistant-agent" {
		t.Fatalf("asset id = %q, want %q", asset.Asset.ID, "assistant-agent")
	}
	if asset.Asset.Manifest.Entry.Kind != "image" {
		t.Fatalf("entry kind = %q, want %q", asset.Asset.Manifest.Entry.Kind, "image")
	}
}

func TestGetAssetVersions_FallsBackToLegacyAgentVersions(t *testing.T) {
	older := time.Date(2026, time.January, 1, 3, 4, 5, 0, time.UTC)
	newer := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	service := New(Dependencies{
		Agents: &fakeAgentsReader{
			listAgentsFn: func(_ context.Context, filter *database.AgentFilter, cursor string, limit int) ([]*models.AgentResponse, string, error) {
				if cursor != "" {
					return nil, "", nil
				}
				return []*models.AgentResponse{
					legacyAgentResponse("assistant-agent", "1.0.0", older, false),
					legacyAgentResponse("assistant-agent", "1.1.0", newer, true),
				}, "", nil
			},
		},
	})

	versions, err := service.GetAssetVersions(context.Background(), "assistant-agent")
	if err != nil {
		t.Fatalf("GetAssetVersions() error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
	if versions[0].Asset.Version != "1.0.0" {
		t.Fatalf("versions[0] = %q, want %q", versions[0].Asset.Version, "1.0.0")
	}
	if versions[1].Asset.Version != "1.1.0" {
		t.Fatalf("versions[1] = %q, want %q", versions[1].Asset.Version, "1.1.0")
	}
}

func legacyPromptResponse(name, version string, updatedAt time.Time, isLatest bool) *models.PromptResponse {
	return &models.PromptResponse{
		Prompt: models.PromptJSON{
			Name:        name,
			Description: "Legacy prompt " + name,
			Version:     version,
			Content:     "Prompt content for " + name + "@" + version,
		},
		Meta: models.PromptResponseMeta{Official: &models.PromptRegistryExtensions{Status: "active", UpdatedAt: updatedAt, PublishedAt: updatedAt, IsLatest: isLatest}},
	}
}

func TestListAssets_FallsBackToLegacyPrompts(t *testing.T) {
	now := time.Date(2026, time.January, 3, 3, 4, 5, 0, time.UTC)
	service := New(Dependencies{
		Prompts: &fakePromptsReader{
			listPromptsFn: func(_ context.Context, filter *database.PromptFilter, cursor string, limit int) ([]*models.PromptResponse, string, error) {
				if filter == nil || filter.SubstringName == nil || *filter.SubstringName != "prompt" {
					t.Fatalf("unexpected filter = %#v", filter)
				}
				if cursor != "" {
					return nil, "", nil
				}
				return []*models.PromptResponse{legacyPromptResponse("welcome-prompt", "1.0.0", now, true)}, "", nil
			},
		},
	})

	search := "prompt"
	category := models.AssetCategoryPrompt
	assets, nextCursor, err := service.ListAssets(context.Background(), &Filter{Search: &search, Category: &category}, "", 10)
	if err != nil {
		t.Fatalf("ListAssets() error = %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("nextCursor = %q, want empty", nextCursor)
	}
	if len(assets) != 1 {
		t.Fatalf("len(assets) = %d, want 1", len(assets))
	}
	if assets[0].Asset.Category != models.AssetCategoryPrompt {
		t.Fatalf("asset category = %q, want %q", assets[0].Asset.Category, models.AssetCategoryPrompt)
	}
	if assets[0].Asset.ID != "welcome-prompt" {
		t.Fatalf("asset id = %q, want %q", assets[0].Asset.ID, "welcome-prompt")
	}
}

func TestGetAssetVersion_FallsBackToLegacyPrompt(t *testing.T) {
	now := time.Date(2026, time.January, 3, 3, 4, 5, 0, time.UTC)
	service := New(Dependencies{
		Prompts: &fakePromptsReader{
			listPromptsFn: func(_ context.Context, filter *database.PromptFilter, cursor string, limit int) ([]*models.PromptResponse, string, error) {
				if cursor != "" {
					return nil, "", nil
				}
				return []*models.PromptResponse{legacyPromptResponse("welcome-prompt", "1.0.0", now, true)}, "", nil
			},
		},
	})

	asset, err := service.GetAssetVersion(context.Background(), "welcome-prompt", "1.0.0")
	if err != nil {
		t.Fatalf("GetAssetVersion() error = %v", err)
	}
	if asset.Asset.ID != "welcome-prompt" {
		t.Fatalf("asset id = %q, want %q", asset.Asset.ID, "welcome-prompt")
	}
	if asset.Asset.Manifest.Entry.Kind != "skill-body" {
		t.Fatalf("entry kind = %q, want %q", asset.Asset.Manifest.Entry.Kind, "skill-body")
	}
}

func TestGetAssetVersions_FallsBackToLegacyPromptVersions(t *testing.T) {
	older := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	newer := time.Date(2026, time.January, 3, 3, 4, 5, 0, time.UTC)
	service := New(Dependencies{
		Prompts: &fakePromptsReader{
			listPromptsFn: func(_ context.Context, filter *database.PromptFilter, cursor string, limit int) ([]*models.PromptResponse, string, error) {
				if cursor != "" {
					return nil, "", nil
				}
				return []*models.PromptResponse{
					legacyPromptResponse("welcome-prompt", "1.0.0", older, false),
					legacyPromptResponse("welcome-prompt", "1.1.0", newer, true),
				}, "", nil
			},
		},
	})

	versions, err := service.GetAssetVersions(context.Background(), "welcome-prompt")
	if err != nil {
		t.Fatalf("GetAssetVersions() error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
	if versions[0].Asset.Version != "1.0.0" {
		t.Fatalf("versions[0] = %q, want %q", versions[0].Asset.Version, "1.0.0")
	}
	if versions[1].Asset.Version != "1.1.0" {
		t.Fatalf("versions[1] = %q, want %q", versions[1].Asset.Version, "1.1.0")
	}
}

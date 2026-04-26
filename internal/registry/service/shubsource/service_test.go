package shubsource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
)

type fakeSourceStore struct {
	listFn   func(context.Context) ([]*models.SHUBSource, error)
	getFn    func(context.Context, string) (*models.SHUBSource, error)
	putFn    func(context.Context, *models.SHUBSource) (*models.SHUBSource, error)
	deleteFn func(context.Context, string) error
}

func (f *fakeSourceStore) ListSHUBSources(ctx context.Context) ([]*models.SHUBSource, error) {
	if f.listFn != nil {
		return f.listFn(ctx)
	}
	return nil, nil
}

func (f *fakeSourceStore) GetSHUBSource(ctx context.Context, name string) (*models.SHUBSource, error) {
	if f.getFn != nil {
		return f.getFn(ctx, name)
	}
	return nil, database.ErrNotFound
}

func (f *fakeSourceStore) PutSHUBSource(ctx context.Context, source *models.SHUBSource) (*models.SHUBSource, error) {
	if f.putFn != nil {
		return f.putFn(ctx, source)
	}
	return source, nil
}

func (f *fakeSourceStore) DeleteSHUBSource(ctx context.Context, name string) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, name)
	}
	return nil
}

type fakeAssetRegistry struct {
	getAssetFn        func(context.Context, string) (*models.AssetResponse, error)
	getAssetVersionFn func(context.Context, string, string) (*models.AssetResponse, error)
	publishFn         func(context.Context, *models.AssetPublishRequest) (*models.AssetResponse, error)
	uploadPackageFn   func(context.Context, string, string, []byte, string) (*models.AssetPackageResponse, error)
}

func (f *fakeAssetRegistry) GetAsset(ctx context.Context, assetID string) (*models.AssetResponse, error) {
	if f.getAssetFn != nil {
		return f.getAssetFn(ctx, assetID)
	}
	return nil, database.ErrNotFound
}

func (f *fakeAssetRegistry) GetAssetVersion(ctx context.Context, assetID, version string) (*models.AssetResponse, error) {
	if f.getAssetVersionFn != nil {
		return f.getAssetVersionFn(ctx, assetID, version)
	}
	return nil, database.ErrNotFound
}

func (f *fakeAssetRegistry) PublishAsset(ctx context.Context, request *models.AssetPublishRequest) (*models.AssetResponse, error) {
	if f.publishFn != nil {
		return f.publishFn(ctx, request)
	}
	asset, err := request.ToAsset()
	if err != nil {
		return nil, err
	}
	return &models.AssetResponse{Asset: *asset}, nil
}

func (f *fakeAssetRegistry) UploadAssetPackage(ctx context.Context, assetID, version string, content []byte, contentType string) (*models.AssetPackageResponse, error) {
	if f.uploadPackageFn != nil {
		return f.uploadPackageFn(ctx, assetID, version, content, contentType)
	}
	return &models.AssetPackageResponse{Package: models.AssetPackage{AssetID: assetID, Version: version, SizeBytes: len(content), ContentType: contentType}}, nil
}

type fakeFetcher struct {
	fetchFn func(context.Context, *models.SHUBSource, string, string, string) (string, error)
}

func (f fakeFetcher) Fetch(ctx context.Context, source *models.SHUBSource, assetID, version, targetDir string) (string, error) {
	return f.fetchFn(ctx, source, assetID, version, targetDir)
}

func TestSetSource_ValidatesAndStores(t *testing.T) {
	var stored *models.SHUBSource
	service := New(Dependencies{Sources: &fakeSourceStore{putFn: func(_ context.Context, source *models.SHUBSource) (*models.SHUBSource, error) {
		stored = source
		cloned := *source
		cloned.CreatedAt = time.Now().UTC()
		cloned.UpdatedAt = cloned.CreatedAt
		return &cloned, nil
	}}})

	result, err := service.SetSource(context.Background(), " github-main ", " https://github.com/acme/skills/tree/main/skills ")
	if err != nil {
		t.Fatalf("SetSource() error = %v", err)
	}
	if stored == nil {
		t.Fatal("source was not stored")
	}
	if stored.Name != "github-main" || stored.Address != "https://github.com/acme/skills/tree/main/skills" {
		t.Fatalf("stored source = %#v", stored)
	}
	if result.Name != stored.Name {
		t.Fatalf("result = %#v, want stored source", result)
	}
}

func TestListSources_IncludesBuiltInCatalog(t *testing.T) {
	service := New(Dependencies{Sources: &fakeSourceStore{listFn: func(context.Context) ([]*models.SHUBSource, error) {
		return []*models.SHUBSource{{Name: "company-main", Address: "https://gitlab.example.com/acme/skills/-/tree/main/skills/{asset}"}}, nil
	}}})

	sources, err := service.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) < 6 {
		t.Fatalf("expected built-in sources plus custom source, got %#v", sources)
	}
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Name)
	}
	for _, want := range []string{"anthropic-skills", "github-direct", "github-skills-main", "github-plugin-skills-main", "openai-skills", "company-main"} {
		if !containsString(names, want) {
			t.Fatalf("built-in source %q missing from %#v", want, names)
		}
	}
}

func TestGetSource_ReturnsBuiltInSource(t *testing.T) {
	service := New(Dependencies{})

	source, err := service.GetSource(context.Background(), "openai-skills")
	if err != nil {
		t.Fatalf("GetSource() error = %v", err)
	}
	if !source.BuiltIn || source.Provider != "github" {
		t.Fatalf("source = %#v, want built-in github provider", source)
	}
}

func TestSetSource_RejectsBuiltInName(t *testing.T) {
	service := New(Dependencies{Sources: &fakeSourceStore{}})

	_, err := service.SetSource(context.Background(), "github-direct", "https://example.com")
	if err == nil {
		t.Fatal("expected built-in rejection error, got nil")
	}
	if !errors.Is(err, database.ErrInvalidInput) {
		t.Fatalf("error = %v, want database.ErrInvalidInput", err)
	}
}

func TestSetSource_RejectsInvalidAddress(t *testing.T) {
	service := New(Dependencies{Sources: &fakeSourceStore{}})
	_, err := service.SetSource(context.Background(), "github-main", "ssh://github.com/acme/skills")
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !errors.Is(err, database.ErrInvalidInput) {
		t.Fatalf("error = %v, want database.ErrInvalidInput", err)
	}
}

func TestPullAsset_FetchesPackagesAndPublishes(t *testing.T) {
	fixture := createSourceFixture(t, "arch/java-analyzer", "1.2.0")
	var uploadAssetID string
	var uploadVersion string
	var uploadedBytes int
	var published *models.AssetPublishRequest
	service := New(Dependencies{
		Sources: &fakeSourceStore{getFn: func(_ context.Context, name string) (*models.SHUBSource, error) {
			return &models.SHUBSource{Name: name, Address: "https://github.com/acme/skills/tree/main/skills"}, nil
		}},
		Assets: &fakeAssetRegistry{
			getAssetFn:        func(context.Context, string) (*models.AssetResponse, error) { return nil, database.ErrNotFound },
			getAssetVersionFn: func(context.Context, string, string) (*models.AssetResponse, error) { return nil, database.ErrNotFound },
			uploadPackageFn: func(_ context.Context, assetID, version string, content []byte, _ string) (*models.AssetPackageResponse, error) {
				uploadAssetID = assetID
				uploadVersion = version
				uploadedBytes = len(content)
				return &models.AssetPackageResponse{Package: models.AssetPackage{AssetID: assetID, Version: version, SizeBytes: len(content)}}, nil
			},
			publishFn: func(_ context.Context, request *models.AssetPublishRequest) (*models.AssetResponse, error) {
				published = request
				asset, err := request.ToAsset()
				if err != nil {
					return nil, err
				}
				return &models.AssetResponse{Asset: *asset}, nil
			},
		},
		Fetcher: fakeFetcher{fetchFn: func(_ context.Context, _ *models.SHUBSource, _ string, _ string, targetDir string) (string, error) {
			if err := copyFixture(fixture, targetDir); err != nil {
				return "", err
			}
			return "https://github.com/acme/skills/tree/main/skills/java-analyzer", nil
		}},
	})

	asset, err := service.PullAsset(context.Background(), "github-main", "arch/java-analyzer", "1.2.0")
	if err != nil {
		t.Fatalf("PullAsset() error = %v", err)
	}
	if asset.Asset.ID != "arch/java-analyzer" {
		t.Fatalf("asset id = %q, want arch/java-analyzer", asset.Asset.ID)
	}
	if uploadAssetID != "arch/java-analyzer" || uploadVersion != "1.2.0" {
		t.Fatalf("uploaded package = %s@%s", uploadAssetID, uploadVersion)
	}
	if uploadedBytes == 0 {
		t.Fatal("expected non-empty uploaded package")
	}
	if published == nil || published.Source == nil {
		t.Fatalf("published request = %#v, want source metadata", published)
	}
	if published.Source.RepositoryURL != "https://github.com/acme/skills/tree/main/skills/java-analyzer" {
		t.Fatalf("repository url = %q", published.Source.RepositoryURL)
	}
	if published.Source.PackageRef != "/v0/assets/arch%2Fjava-analyzer/versions/1.2.0/package" {
		t.Fatalf("package ref = %q", published.Source.PackageRef)
	}
}

func TestPullAsset_UsesBuiltInSourceCatalog(t *testing.T) {
	fixture := createSourceFixture(t, "unfallenwill/supercoder", "1.2.0")
	service := New(Dependencies{
		Assets: &fakeAssetRegistry{
			uploadPackageFn: func(context.Context, string, string, []byte, string) (*models.AssetPackageResponse, error) {
				return &models.AssetPackageResponse{}, nil
			},
		},
		Fetcher: fakeFetcher{fetchFn: func(_ context.Context, source *models.SHUBSource, assetID, version, targetDir string) (string, error) {
			if source == nil {
				t.Fatal("expected built-in source")
			}
			if source.Name != "github-direct" {
				t.Fatalf("source.Name = %q, want github-direct", source.Name)
			}
			if source.Address != "https://github.com/{asset}" {
				t.Fatalf("source.Address = %q, want https://github.com/{asset}", source.Address)
			}
			if assetID != "unfallenwill/supercoder" {
				t.Fatalf("assetID = %q, want unfallenwill/supercoder", assetID)
			}
			if version != "1.2.0" {
				t.Fatalf("version = %q, want 1.2.0", version)
			}
			if err := copyFixture(fixture, targetDir); err != nil {
				t.Fatalf("copyFixture() error = %v", err)
			}
			return "https://github.com/unfallenwill/supercoder", nil
		}},
	})

	asset, err := service.PullAsset(context.Background(), "github-direct", "unfallenwill/supercoder", "1.2.0")
	if err != nil {
		t.Fatalf("PullAsset() error = %v", err)
	}
	if asset.Asset.ID != "unfallenwill/supercoder" {
		t.Fatalf("asset id = %q, want unfallenwill/supercoder", asset.Asset.ID)
	}
}

func TestPullAsset_ImportsPlainSkillRepoIntoSHUBAsset(t *testing.T) {
	fixture := createPlainSourceFixture(t)
	var uploadedAssetID string
	var uploadedVersion string
	var published *models.AssetPublishRequest
	service := New(Dependencies{
		Assets: &fakeAssetRegistry{
			uploadPackageFn: func(_ context.Context, assetID, version string, _ []byte, _ string) (*models.AssetPackageResponse, error) {
				uploadedAssetID = assetID
				uploadedVersion = version
				return &models.AssetPackageResponse{}, nil
			},
			publishFn: func(_ context.Context, request *models.AssetPublishRequest) (*models.AssetResponse, error) {
				published = request
				asset, err := request.ToAsset()
				if err != nil {
					return nil, err
				}
				return &models.AssetResponse{Asset: *asset}, nil
			},
		},
		Fetcher: fakeFetcher{fetchFn: func(_ context.Context, source *models.SHUBSource, assetID, version, targetDir string) (string, error) {
			if source.Name != "github-plugin-skills-main" {
				t.Fatalf("source.Name = %q, want github-plugin-skills-main", source.Name)
			}
			if err := copyFixture(fixture, targetDir); err != nil {
				t.Fatalf("copyFixture() error = %v", err)
			}
			return "https://github.com/unfallenwill/supercoder/tree/main/plugins/supercoder/skills/supercoder", nil
		}},
	})

	asset, err := service.PullAsset(context.Background(), "github-plugin-skills-main", "unfallenwill/supercoder", "")
	if err != nil {
		t.Fatalf("PullAsset() error = %v", err)
	}
	if asset.Asset.ID != "unfallenwill/supercoder" {
		t.Fatalf("asset id = %q, want unfallenwill/supercoder", asset.Asset.ID)
	}
	if asset.Asset.Version != importedAssetFallbackVersion {
		t.Fatalf("asset version = %q, want %q", asset.Asset.Version, importedAssetFallbackVersion)
	}
	if uploadedAssetID != "unfallenwill/supercoder" || uploadedVersion != importedAssetFallbackVersion {
		t.Fatalf("uploaded asset = %s@%s, want unfallenwill/supercoder@%s", uploadedAssetID, uploadedVersion, importedAssetFallbackVersion)
	}
	if published == nil {
		t.Fatal("expected publish request")
	}
	if published.Manifest.Entry.Kind != "skill-body" || published.Manifest.Entry.Path != "SKILL.md" {
		t.Fatalf("entry = %#v, want skill-body/SKILL.md", published.Manifest.Entry)
	}
	if published.Manifest.Runtime.Type != "none" {
		t.Fatalf("runtime = %#v, want type none", published.Manifest.Runtime)
	}
	if len(published.Manifest.Exports) != 2 {
		t.Fatalf("exports = %#v, want 2 default skill-dir exports", published.Manifest.Exports)
	}
	if published.Manifest.Exports[0].Target != "codex" || published.Manifest.Exports[0].Mode != "skill-dir" || published.Manifest.Exports[0].Source != "." {
		t.Fatalf("first export = %#v, want codex skill-dir from .", published.Manifest.Exports[0])
	}
	if published.Manifest.Exports[1].Target != "claude-code" || published.Manifest.Exports[1].Mode != "skill-dir" || published.Manifest.Exports[1].Source != "." {
		t.Fatalf("second export = %#v, want claude-code skill-dir from .", published.Manifest.Exports[1])
	}
	if published.Source == nil || published.Source.RepositoryURL != "https://github.com/unfallenwill/supercoder/tree/main/plugins/supercoder/skills/supercoder" {
		t.Fatalf("source = %#v, want repository URL for imported skill", published.Source)
	}
}

func TestPullAsset_ReturnsExistingRegistryAssetBeforeFetching(t *testing.T) {
	service := New(Dependencies{
		Sources: &fakeSourceStore{},
		Assets: &fakeAssetRegistry{getAssetVersionFn: func(_ context.Context, assetID, version string) (*models.AssetResponse, error) {
			return &models.AssetResponse{Asset: models.Asset{ID: assetID, Version: version}}, nil
		}},
		Fetcher: fakeFetcher{fetchFn: func(context.Context, *models.SHUBSource, string, string, string) (string, error) {
			t.Fatal("fetcher should not be called when asset already exists")
			return "", nil
		}},
	})

	asset, err := service.PullAsset(context.Background(), "github-main", "arch/java-analyzer", "1.2.0")
	if err != nil {
		t.Fatalf("PullAsset() error = %v", err)
	}
	if asset.Asset.Version != "1.2.0" {
		t.Fatalf("version = %q, want 1.2.0", asset.Asset.Version)
	}
}

func TestResolveSourceAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		assetID string
		version string
		want    string
		wantErr string
	}{
		{
			name:    "appends basename when no template",
			address: "https://github.com/acme/skills/tree/main/skills",
			assetID: "arch/java-analyzer",
			want:    "https://github.com/acme/skills/tree/main/skills/java-analyzer",
		},
		{
			name:    "expands templates",
			address: "https://gitlab.com/acme/skills/-/tree/{version}/skills/{name}",
			assetID: "arch/java-analyzer",
			version: "1.2.0",
			want:    "https://gitlab.com/acme/skills/-/tree/1.2.0/skills/java-analyzer",
		},
		{
			name:    "requires version when template uses it",
			address: "https://gitlab.com/acme/skills/-/tree/{version}/skills/{name}",
			assetID: "arch/java-analyzer",
			wantErr: "requires a version",
		},
		{
			name:    "supports full asset template for github direct",
			address: "https://github.com/{asset}",
			assetID: "unfallenwill/supercoder",
			want:    "https://github.com/unfallenwill/supercoder",
		},
		{
			name:    "supports plugin skill layout on github",
			address: "https://github.com/{asset}/tree/main/plugins/{name}/skills/{name}",
			assetID: "unfallenwill/supercoder",
			want:    "https://github.com/unfallenwill/supercoder/tree/main/plugins/supercoder/skills/supercoder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSourceAddress(&models.SHUBSource{Name: "test", Address: tt.address}, tt.assetID, tt.version)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveSourceAddress() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveSourceAddress() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolved = %q, want %q", got, tt.want)
			}
		})
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func createSourceFixture(t *testing.T, assetID, version string) string {
	t.Helper()
	dir := t.TempDir()
	body := `---
name: java-analyzer
description: Analyze Java services
version: ` + version + `
allowed-tools:
  - Read
shub:
  schemaVersion: shub.skill/v1alpha1
  id: ` + assetID + `
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
---
# Java Analyzer
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	return dir
}

func createPlainSourceFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body := `---
name: supercoder
description: Plain plugin skill imported from GitHub.
allowed-tools:
  - Read
  - Write
argument-hint: <requirement>
---
# Supercoder

Imported plain skill fixture.
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatalf("mkdir references: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "discover-phase.md"), []byte("discover"), 0o644); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	return dir
}

func copyFixture(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyFixture(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

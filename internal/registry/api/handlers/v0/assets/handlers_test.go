package assets_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v0assets "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/assets"
	assetsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/asset"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/stretchr/testify/assert"
)

type fakeAssetService struct {
	listFn        func(ctx context.Context, filter *assetsvc.Filter, cursor string, limit int) ([]*models.AssetResponse, string, error)
	getFn         func(ctx context.Context, assetID string) (*models.AssetResponse, error)
	getVersionFn  func(ctx context.Context, assetID, version string) (*models.AssetResponse, error)
	getVersionsFn func(ctx context.Context, assetID string) ([]*models.AssetResponse, error)
	publishFn     func(ctx context.Context, request *models.AssetPublishRequest) (*models.AssetResponse, error)
	uploadFn      func(ctx context.Context, assetID, version string, content []byte, contentType string) (*models.AssetPackageResponse, error)
	getPackageFn  func(ctx context.Context, assetID, version string) (*models.AssetPackageDownload, error)
}

func (service *fakeAssetService) ListAssets(ctx context.Context, filter *assetsvc.Filter, cursor string, limit int) ([]*models.AssetResponse, string, error) {
	if service.listFn != nil {
		return service.listFn(ctx, filter, cursor, limit)
	}
	return nil, "", nil
}

func (service *fakeAssetService) GetAsset(ctx context.Context, assetID string) (*models.AssetResponse, error) {
	if service.getFn != nil {
		return service.getFn(ctx, assetID)
	}
	return nil, database.ErrNotFound
}

func (service *fakeAssetService) GetAssetVersion(ctx context.Context, assetID, version string) (*models.AssetResponse, error) {
	if service.getVersionFn != nil {
		return service.getVersionFn(ctx, assetID, version)
	}
	return nil, database.ErrNotFound
}

func (service *fakeAssetService) GetAssetVersions(ctx context.Context, assetID string) ([]*models.AssetResponse, error) {
	if service.getVersionsFn != nil {
		return service.getVersionsFn(ctx, assetID)
	}
	return nil, database.ErrNotFound
}

func (service *fakeAssetService) PublishAsset(ctx context.Context, request *models.AssetPublishRequest) (*models.AssetResponse, error) {
	if service.publishFn != nil {
		return service.publishFn(ctx, request)
	}
	return nil, database.ErrNotFound
}

func (service *fakeAssetService) UploadAssetPackage(ctx context.Context, assetID, version string, content []byte, contentType string) (*models.AssetPackageResponse, error) {
	if service.uploadFn != nil {
		return service.uploadFn(ctx, assetID, version, content, contentType)
	}
	return nil, database.ErrNotFound
}

func (service *fakeAssetService) GetAssetPackage(ctx context.Context, assetID, version string) (*models.AssetPackageDownload, error) {
	if service.getPackageFn != nil {
		return service.getPackageFn(ctx, assetID, version)
	}
	return nil, database.ErrNotFound
}

func TestListAssetsEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test API", "1.0.0"))
	v0assets.RegisterAssetsEndpoints(api, "/v0", &fakeAssetService{listFn: func(context.Context, *assetsvc.Filter, string, int) ([]*models.AssetResponse, string, error) {
		return []*models.AssetResponse{testAssetResponse("arch/java-analyzer", "1.2.0")}, "", nil
	}})

	req := httptest.NewRequest(http.MethodGet, "/v0/assets?search=java", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"asset":{"id":"arch/java-analyzer"`)
	assert.Contains(t, w.Body.String(), `"count":1`)
}

func TestGetAssetLatestEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test API", "1.0.0"))
	v0assets.RegisterAssetsEndpoints(api, "/v0", &fakeAssetService{getFn: func(_ context.Context, assetID string) (*models.AssetResponse, error) {
		if assetID != "arch/java-analyzer" {
			return nil, database.ErrNotFound
		}
		return testAssetResponse(assetID, "1.2.0"), nil
	}})

	req := httptest.NewRequest(http.MethodGet, "/v0/assets/arch%2Fjava-analyzer", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"version":"1.2.0"`)
}

func testAssetResponse(assetID, version string) *models.AssetResponse {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	return &models.AssetResponse{
		Asset: models.Asset{
			ID:          assetID,
			Name:        "java-analyzer",
			Description: "Analyze Java services",
			Version:     version,
			Category:    models.AssetCategoryPrompt,
			SourceSkill: models.AssetSourceSkill{Path: models.SkillFileName, Body: "# Java Analyzer", BodyFormat: "markdown"},
			Manifest: models.AssetManifest{
				SchemaVersion: models.ShubAssetSchemaVersion,
				ID:            assetID,
				Category:      models.AssetCategoryPrompt,
				Name:          "java-analyzer",
				Description:   "Analyze Java services",
				Version:       version,
				SourceSkill:   models.AssetSourceSkill{Path: models.SkillFileName, Body: "# Java Analyzer", BodyFormat: "markdown"},
				Entry:         models.AssetEntry{Kind: "skill-body", Path: models.SkillFileName},
				Runtime:       models.AssetRuntime{Type: "none"},
			},
			Source: &models.AssetSource{PackageType: "tarball", PackageRef: "file:///tmp/demo.tar.gz"},
		},
		Meta: models.AssetResponseMeta{Official: &models.AssetRegistryExtensions{Status: "active", UpdatedAt: now, PublishedAt: now, IsLatest: true}},
	}
}

func TestCreateAssetEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test API", "1.0.0"))
	v0assets.RegisterAssetsCreateEndpoint(api, "/v0", &fakeAssetService{publishFn: func(_ context.Context, request *models.AssetPublishRequest) (*models.AssetResponse, error) {
		if request.Manifest.ID != "arch/java-analyzer" {
			return nil, database.ErrNotFound
		}
		return testAssetResponse(request.Manifest.ID, request.Manifest.Version), nil
	}})

	body := `{"manifest":{"schemaVersion":"shub.asset/v1alpha1","id":"arch/java-analyzer","category":"prompt","name":"java-analyzer","description":"Analyze Java services","version":"1.2.0","sourceSkill":{"path":"SKILL.md","body":"# Java Analyzer","bodyFormat":"markdown"},"entry":{"kind":"skill-body","path":"SKILL.md"},"runtime":{"type":"none"}},"source":{"packageType":"tarball","packageRef":"https://example.com/java-analyzer-1.2.0.tgz"}}`
	req := httptest.NewRequest(http.MethodPost, "/v0/assets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"asset":{"id":"arch/java-analyzer"`)
}

func TestUploadAssetPackageEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test API", "1.0.0"))
	v0assets.RegisterAssetsCreateEndpoint(api, "/v0", &fakeAssetService{uploadFn: func(_ context.Context, assetID, version string, content []byte, contentType string) (*models.AssetPackageResponse, error) {
		assert.Equal(t, "arch/java-analyzer", assetID)
		assert.Equal(t, "1.2.0", version)
		assert.Equal(t, "application/gzip", contentType)
		assert.Equal(t, []byte("package-bytes"), content)
		return &models.AssetPackageResponse{
			Package: models.AssetPackage{
				AssetID:     assetID,
				Version:     version,
				ContentType: "application/gzip",
				SizeBytes:   len(content),
				SHA256:      "abc123",
			},
		}, nil
	}})

	req := httptest.NewRequest(http.MethodPut, "/v0/assets/arch%2Fjava-analyzer/versions/1.2.0/package", bytes.NewReader([]byte("package-bytes")))
	req.Header.Set("Content-Type", "application/gzip")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"downloadUrl":"/v0/assets/arch%2Fjava-analyzer/versions/1.2.0/package"`)
	assert.Contains(t, w.Body.String(), `"sha256":"abc123"`)
}

func TestGetAssetPackageEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test API", "1.0.0"))
	v0assets.RegisterAssetsEndpoints(api, "/v0", &fakeAssetService{getPackageFn: func(_ context.Context, assetID, version string) (*models.AssetPackageDownload, error) {
		assert.Equal(t, "arch/java-analyzer", assetID)
		assert.Equal(t, "1.2.0", version)
		return &models.AssetPackageDownload{
			Package: models.AssetPackage{
				AssetID:     assetID,
				Version:     version,
				ContentType: "application/gzip",
				SizeBytes:   len("package-bytes"),
			},
			Content: []byte("package-bytes"),
		}, nil
	}})

	req := httptest.NewRequest(http.MethodGet, "/v0/assets/arch%2Fjava-analyzer/versions/1.2.0/package", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/gzip", w.Header().Get("Content-Type"))
	assert.Equal(t, "package-bytes", w.Body.String())
}

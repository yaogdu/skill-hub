package shubsources_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v0shubsources "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/shubsources"
	shubsourcesvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/shubsource"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSourceService struct {
	listFn   func(context.Context) ([]*models.SHUBSource, error)
	getFn    func(context.Context, string) (*models.SHUBSource, error)
	setFn    func(context.Context, string, string) (*models.SHUBSource, error)
	deleteFn func(context.Context, string) error
	pullFn   func(context.Context, string, string, string) (*models.AssetResponse, error)
}

func (f *fakeSourceService) ListSources(ctx context.Context) ([]*models.SHUBSource, error) {
	if f.listFn != nil {
		return f.listFn(ctx)
	}
	return nil, nil
}

func (f *fakeSourceService) GetSource(ctx context.Context, name string) (*models.SHUBSource, error) {
	if f.getFn != nil {
		return f.getFn(ctx, name)
	}
	return nil, database.ErrNotFound
}

func (f *fakeSourceService) SetSource(ctx context.Context, name, address string) (*models.SHUBSource, error) {
	if f.setFn != nil {
		return f.setFn(ctx, name, address)
	}
	return &models.SHUBSource{Name: name, Address: address}, nil
}

func (f *fakeSourceService) DeleteSource(ctx context.Context, name string) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, name)
	}
	return nil
}

func (f *fakeSourceService) PullAsset(ctx context.Context, sourceName, assetID, version string) (*models.AssetResponse, error) {
	if f.pullFn != nil {
		return f.pullFn(ctx, sourceName, assetID, version)
	}
	return nil, database.ErrNotFound
}

func TestRegisterSHUBSourceEndpoints_List(t *testing.T) {
	mux, api := newSourceTestAPI(t)
	now := time.Date(2026, time.April, 25, 9, 0, 0, 0, time.UTC)
	service := &fakeSourceService{listFn: func(context.Context) ([]*models.SHUBSource, error) {
		return []*models.SHUBSource{{Name: "github-main", Address: "https://github.com/acme/skills/tree/main/skills", CreatedAt: now, UpdatedAt: now}}, nil
	}}
	v0shubsources.RegisterSHUBSourceEndpoints(api, "/v0", service)

	req := httptest.NewRequest(http.MethodGet, "/v0/shub/sources", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"name":"github-main"`)
	assert.Contains(t, w.Body.String(), `"count":1`)
}

func TestRegisterSHUBSourceEndpoints_PutAndPull(t *testing.T) {
	mux, api := newSourceTestAPI(t)
	service := &fakeSourceService{}
	service.setFn = func(_ context.Context, name, address string) (*models.SHUBSource, error) {
		return &models.SHUBSource{Name: name, Address: address}, nil
	}
	service.pullFn = func(_ context.Context, sourceName, assetID, version string) (*models.AssetResponse, error) {
		return &models.AssetResponse{Asset: models.Asset{ID: assetID, Version: version}}, nil
	}
	v0shubsources.RegisterSHUBSourceEndpoints(api, "/v0", service)

	putBody, _ := json.Marshal(models.SHUBSourceUpsertRequest{Address: "https://github.com/acme/skills/tree/main/skills"})
	putReq := httptest.NewRequest(http.MethodPut, "/v0/shub/sources/github-main", bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	mux.ServeHTTP(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", putW.Code, putW.Body.String())
	}
	assert.Contains(t, putW.Body.String(), `"address":"https://github.com/acme/skills/tree/main/skills"`)

	pullBody, _ := json.Marshal(models.SHUBSourcePullRequest{AssetID: "arch/java-analyzer", Version: "1.2.0"})
	pullReq := httptest.NewRequest(http.MethodPost, "/v0/shub/sources/github-main/pull", bytes.NewReader(pullBody))
	pullReq.Header.Set("Content-Type", "application/json")
	pullW := httptest.NewRecorder()
	mux.ServeHTTP(pullW, pullReq)
	if pullW.Code != http.StatusOK {
		t.Fatalf("POST status = %d body=%s", pullW.Code, pullW.Body.String())
	}
	assert.Contains(t, pullW.Body.String(), `"id":"arch/java-analyzer"`)
}

func TestRegisterSHUBSourceEndpoints_MapsNotFound(t *testing.T) {
	mux, api := newSourceTestAPI(t)
	service := &fakeSourceService{getFn: func(context.Context, string) (*models.SHUBSource, error) {
		return nil, database.ErrNotFound
	}}
	v0shubsources.RegisterSHUBSourceEndpoints(api, "/v0", service)

	req := httptest.NewRequest(http.MethodGet, "/v0/shub/sources/missing", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), `SHUB source not found`)
}

func TestMapSourceErrorPreservesInvalidInput(t *testing.T) {
	err := errors.New("x")
	assert.False(t, errors.Is(err, database.ErrInvalidInput))
}

func newSourceTestAPI(t *testing.T) (*http.ServeMux, huma.API) {
	t.Helper()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("test", "1.0.0"))
	return mux, api
}

var _ shubsourcesvc.Registry = (*fakeSourceService)(nil)

package userauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	userauthsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/userauth"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRegistryService struct {
	loginResp        *models.LoginResponse
	loginErr         error
	createUserResp   *models.RegistryUser
	createUserErr    error
	createAPIKeyResp *models.CreateAPIKeyResponse
	createAPIKeyErr  error
	settingsResp     *models.RegistryAuthSettings
	settingsErr      error
}

var _ userauthsvc.Registry = (*fakeRegistryService)(nil)

func (f *fakeRegistryService) BootstrapAdmin(context.Context, string, string) error {
	return nil
}

func (f *fakeRegistryService) Login(context.Context, *models.LoginRequest) (*models.LoginResponse, error) {
	return f.loginResp, f.loginErr
}

func (f *fakeRegistryService) Me(context.Context) (*models.RegistryUser, error) {
	return nil, nil
}

func (f *fakeRegistryService) ListUsers(context.Context) ([]*models.RegistryUser, error) {
	return nil, nil
}

func (f *fakeRegistryService) CreateUser(context.Context, *models.CreateUserRequest) (*models.RegistryUser, error) {
	return f.createUserResp, f.createUserErr
}

func (f *fakeRegistryService) ListAPIKeys(context.Context) ([]*models.APIKey, error) {
	return nil, nil
}

func (f *fakeRegistryService) CreateAPIKey(context.Context, *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	return f.createAPIKeyResp, f.createAPIKeyErr
}

func (f *fakeRegistryService) DeleteAPIKey(context.Context, string) error {
	return nil
}

func (f *fakeRegistryService) GetSettings(context.Context) (*models.RegistryAuthSettings, error) {
	return f.settingsResp, f.settingsErr
}

func (f *fakeRegistryService) UpdateSettings(context.Context, *models.UpdateRegistryAuthSettingsRequest) (*models.RegistryAuthSettings, error) {
	return f.settingsResp, f.settingsErr
}

func newAuthHandler(t *testing.T, svc userauthsvc.Registry) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("test", "1.0.0"))
	RegisterAuthEndpoints(api, "/v0", svc)
	return mux
}

func TestLoginEndpoint(t *testing.T) {
	handler := newAuthHandler(t, &fakeRegistryService{
		loginResp: &models.LoginResponse{
			Token:     "token-123",
			ExpiresAt: 1234567890,
			User:      models.RegistryUser{ID: "user-1", Username: "admin", Role: auth.RoleAdmin},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v0/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp models.LoginResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "token-123", resp.Token)
	assert.Equal(t, "admin", resp.User.Username)
}

func TestGetRegistryAuthSettingsEndpointIsPublic(t *testing.T) {
	handler := newAuthHandler(t, &fakeRegistryService{
		settingsResp: &models.RegistryAuthSettings{APIKeyValidationEnabled: true},
	})

	req := httptest.NewRequest(http.MethodGet, "/v0/auth/settings", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp models.RegistryAuthSettings
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.True(t, resp.APIKeyValidationEnabled)
}

func TestCreateUserEndpointMapsForbidden(t *testing.T) {
	handler := newAuthHandler(t, &fakeRegistryService{
		createUserErr: auth.ErrForbidden,
	})

	req := httptest.NewRequest(http.MethodPost, "/v0/auth/users", bytes.NewBufferString(`{"username":"bob","password":"secret","role":"user"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Forbidden")
}

func TestCreateAPIKeyEndpointMapsBadRequest(t *testing.T) {
	handler := newAuthHandler(t, &fakeRegistryService{
		createAPIKeyErr: fmt.Errorf("API key name is required"),
	})

	req := httptest.NewRequest(http.MethodPost, "/v0/auth/api-keys", bytes.NewBufferString(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "API key name is required")
}

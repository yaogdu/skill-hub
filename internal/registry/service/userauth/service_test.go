package userauth

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agentregistry-dev/agentregistry/internal/registry/config"
	internaldb "github.com/agentregistry-dev/agentregistry/internal/registry/database"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sessionStub struct {
	principal auth.Principal
}

func (s sessionStub) Principal() auth.Principal {
	return s.principal
}

type fakeUserStore struct {
	usersByUsername map[string]*auth.UserRecord
	usersByID       map[string]*auth.UserRecord
	list            []*models.RegistryUser
	adminCount      int
	createCalls     []struct {
		username     string
		passwordHash string
		role         string
	}
}

func (s *fakeUserStore) GetAuthUserByUsername(_ context.Context, username string) (*auth.UserRecord, error) {
	return s.usersByUsername[username], nil
}

func (s *fakeUserStore) GetAuthUserByID(_ context.Context, userID string) (*auth.UserRecord, error) {
	return s.usersByID[userID], nil
}

func (s *fakeUserStore) ListRegistryUsers(context.Context) ([]*models.RegistryUser, error) {
	return s.list, nil
}

func (s *fakeUserStore) CreateRegistryUser(_ context.Context, username, passwordHash, role string) (*models.RegistryUser, error) {
	s.createCalls = append(s.createCalls, struct {
		username     string
		passwordHash string
		role         string
	}{username: username, passwordHash: passwordHash, role: role})
	id := fmt.Sprintf("user-%d", len(s.createCalls))
	record := &auth.UserRecord{ID: id, Username: username, PasswordHash: passwordHash, Role: role}
	if s.usersByUsername == nil {
		s.usersByUsername = map[string]*auth.UserRecord{}
	}
	if s.usersByID == nil {
		s.usersByID = map[string]*auth.UserRecord{}
	}
	s.usersByUsername[username] = record
	s.usersByID[id] = record
	user := &models.RegistryUser{
		ID:        id,
		Username:  username,
		Role:      role,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	s.list = append(s.list, user)
	if role == auth.RoleAdmin {
		s.adminCount++
	}
	return user, nil
}

func (s *fakeUserStore) CountRegistryAdmins(context.Context) (int, error) {
	return s.adminCount, nil
}

type fakeAPIKeyStore struct {
	keysByUser map[string][]*models.APIKey
}

func (s *fakeAPIKeyStore) GetAuthAPIKeyBySecret(context.Context, string) (*auth.APIKeyRecord, error) {
	return nil, nil
}

func (s *fakeAPIKeyStore) TouchAuthAPIKey(context.Context, string) error {
	return nil
}

func (s *fakeAPIKeyStore) ListRegistryAPIKeysByUser(_ context.Context, userID string) ([]*models.APIKey, error) {
	return append([]*models.APIKey(nil), s.keysByUser[userID]...), nil
}

func (s *fakeAPIKeyStore) CreateRegistryAPIKey(_ context.Context, userID, name, secret string) (*models.APIKey, error) {
	if s.keysByUser == nil {
		s.keysByUser = map[string][]*models.APIKey{}
	}
	key := &models.APIKey{
		ID:        fmt.Sprintf("key-%d", len(s.keysByUser[userID])+1),
		Name:      name,
		Prefix:    secret[:12],
		CreatedAt: time.Now().UTC(),
	}
	s.keysByUser[userID] = append(s.keysByUser[userID], key)
	return key, nil
}

func (s *fakeAPIKeyStore) DeleteRegistryAPIKey(_ context.Context, userID, keyID string) error {
	keys := s.keysByUser[userID]
	filtered := keys[:0]
	for _, key := range keys {
		if key.ID != keyID {
			filtered = append(filtered, key)
		}
	}
	s.keysByUser[userID] = filtered
	return nil
}

type fakeSettingsStore struct {
	settings *models.RegistryAuthSettings
}

func (s *fakeSettingsStore) GetRegistryAuthSettings(context.Context) (*models.RegistryAuthSettings, error) {
	if s.settings == nil {
		return &models.RegistryAuthSettings{APIKeyValidationEnabled: true}, nil
	}
	return s.settings, nil
}

func (s *fakeSettingsStore) UpdateRegistryAuthSettings(_ context.Context, enabled bool) (*models.RegistryAuthSettings, error) {
	s.settings = &models.RegistryAuthSettings{
		APIKeyValidationEnabled: enabled,
		UpdatedAt:               time.Now().UTC(),
	}
	return s.settings, nil
}

func testJWTManager(t *testing.T) *auth.JWTManager {
	t.Helper()
	return auth.NewJWTManager(&config.Config{JWTPrivateKey: strings.Repeat("0", 64)})
}

func testContext(userID, username, role string) context.Context {
	return auth.AuthSessionTo(context.Background(), sessionStub{
		principal: auth.Principal{
			User: auth.User{
				ID:       userID,
				Username: username,
				Role:     role,
			},
		},
	})
}

func TestBootstrapAdminCreatesDefaultAdminOnce(t *testing.T) {
	users := &fakeUserStore{}
	svc := New(users, &fakeAPIKeyStore{}, &fakeSettingsStore{}, testJWTManager(t))

	err := svc.BootstrapAdmin(context.Background(), " admin ", " secret ")
	require.NoError(t, err)
	require.Len(t, users.createCalls, 1)
	assert.Equal(t, "admin", users.createCalls[0].username)
	assert.Equal(t, auth.RoleAdmin, users.createCalls[0].role)
	assert.NotEqual(t, "secret", users.createCalls[0].passwordHash)
	require.NoError(t, internaldb.CheckPassword(users.createCalls[0].passwordHash, "secret"))

	err = svc.BootstrapAdmin(context.Background(), "admin", "secret")
	require.NoError(t, err)
	assert.Len(t, users.createCalls, 1)
}

func TestLoginReturnsSignedTokenWithRoleClaims(t *testing.T) {
	passwordHash, err := internaldb.HashPassword("secret")
	require.NoError(t, err)

	users := &fakeUserStore{
		usersByUsername: map[string]*auth.UserRecord{
			"alice": {ID: "user-1", Username: "alice", PasswordHash: passwordHash, Role: auth.RoleAdmin},
		},
		usersByID: map[string]*auth.UserRecord{
			"user-1": {ID: "user-1", Username: "alice", PasswordHash: passwordHash, Role: auth.RoleAdmin},
		},
	}
	jwtManager := testJWTManager(t)
	svc := New(users, &fakeAPIKeyStore{}, &fakeSettingsStore{}, jwtManager)

	resp, err := svc.Login(context.Background(), &models.LoginRequest{Username: "alice", Password: "secret"})
	require.NoError(t, err)
	assert.Equal(t, "alice", resp.User.Username)
	assert.Equal(t, auth.RoleAdmin, resp.User.Role)
	assert.NotEmpty(t, resp.Token)

	claims, err := jwtManager.ValidateToken(context.Background(), resp.Token)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.UserID)
	assert.Equal(t, "alice", claims.Username)
	assert.Equal(t, auth.RoleAdmin, claims.Role)
	assert.NotEmpty(t, claims.Permissions)
}

func TestCreateUserRequiresAdmin(t *testing.T) {
	svc := New(&fakeUserStore{}, &fakeAPIKeyStore{}, &fakeSettingsStore{}, testJWTManager(t))

	_, err := svc.CreateUser(testContext("user-1", "alice", auth.RoleUser), &models.CreateUserRequest{
		Username: "bob",
		Password: "secret",
		Role:     auth.RoleUser,
	})
	require.ErrorIs(t, err, auth.ErrForbidden)
}

func TestAPIKeyLifecycleUsesCurrentUser(t *testing.T) {
	keys := &fakeAPIKeyStore{}
	svc := New(&fakeUserStore{}, keys, &fakeSettingsStore{}, testJWTManager(t))
	ctx := testContext("user-1", "alice", auth.RoleUser)

	created, err := svc.CreateAPIKey(ctx, &models.CreateAPIKeyRequest{Name: "default-cli"})
	require.NoError(t, err)
	assert.Equal(t, "default-cli", created.APIKey.Name)
	assert.True(t, strings.HasPrefix(created.Secret, "ar_sk_"))

	listed, err := svc.ListAPIKeys(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, created.APIKey.ID, listed[0].ID)

	err = svc.DeleteAPIKey(ctx, created.APIKey.ID)
	require.NoError(t, err)

	listed, err = svc.ListAPIKeys(ctx)
	require.NoError(t, err)
	assert.Empty(t, listed)
}

func TestUpdateSettingsRequiresAdminAndPersists(t *testing.T) {
	settings := &fakeSettingsStore{}
	svc := New(&fakeUserStore{}, &fakeAPIKeyStore{}, settings, testJWTManager(t))

	_, err := svc.UpdateSettings(testContext("user-1", "alice", auth.RoleUser), &models.UpdateRegistryAuthSettingsRequest{
		APIKeyValidationEnabled: false,
	})
	require.ErrorIs(t, err, auth.ErrForbidden)

	updated, err := svc.UpdateSettings(testContext("admin-1", "admin", auth.RoleAdmin), &models.UpdateRegistryAuthSettingsRequest{
		APIKeyValidationEnabled: false,
	})
	require.NoError(t, err)
	assert.False(t, updated.APIKeyValidationEnabled)
	require.NotNil(t, settings.settings)
	assert.False(t, settings.settings.APIKeyValidationEnabled)
}

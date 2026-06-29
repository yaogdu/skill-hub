package userauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	internaldb "github.com/agentregistry-dev/agentregistry/internal/registry/database"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
)

const sessionDuration = 12 * time.Hour

type UserStore interface {
	auth.UserCredentialStore
	ListRegistryUsers(ctx context.Context) ([]*models.RegistryUser, error)
	CreateRegistryUser(ctx context.Context, username, passwordHash, role string) (*models.RegistryUser, error)
	CountRegistryAdmins(ctx context.Context) (int, error)
}

type APIKeyStore interface {
	auth.APIKeyCredentialStore
	ListRegistryAPIKeysByUser(ctx context.Context, userID string) ([]*models.APIKey, error)
	CreateRegistryAPIKey(ctx context.Context, userID, name, secret string) (*models.APIKey, error)
	DeleteRegistryAPIKey(ctx context.Context, userID, keyID string) error
}

type SettingsStore interface {
	GetRegistryAuthSettings(ctx context.Context) (*models.RegistryAuthSettings, error)
	UpdateRegistryAuthSettings(ctx context.Context, enabled bool) (*models.RegistryAuthSettings, error)
}

type Registry interface {
	BootstrapAdmin(ctx context.Context, username, password string) error
	Login(ctx context.Context, request *models.LoginRequest) (*models.LoginResponse, error)
	Me(ctx context.Context) (*models.RegistryUser, error)
	ListUsers(ctx context.Context) ([]*models.RegistryUser, error)
	CreateUser(ctx context.Context, request *models.CreateUserRequest) (*models.RegistryUser, error)
	ListAPIKeys(ctx context.Context) ([]*models.APIKey, error)
	CreateAPIKey(ctx context.Context, request *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	DeleteAPIKey(ctx context.Context, keyID string) error
	GetSettings(ctx context.Context) (*models.RegistryAuthSettings, error)
	UpdateSettings(ctx context.Context, request *models.UpdateRegistryAuthSettingsRequest) (*models.RegistryAuthSettings, error)
}

type service struct {
	users    UserStore
	apiKeys  APIKeyStore
	settings SettingsStore
	jwt      *auth.JWTManager
}

func New(users UserStore, apiKeys APIKeyStore, settings SettingsStore, jwt *auth.JWTManager) Registry {
	return &service{users: users, apiKeys: apiKeys, settings: settings, jwt: jwt}
}

func (s *service) BootstrapAdmin(ctx context.Context, username, password string) error {
	if s.users == nil {
		return nil
	}
	count, err := s.users.CountRegistryAdmins(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := internaldb.HashPassword(strings.TrimSpace(password))
	if err != nil {
		return err
	}
	_, err = s.users.CreateRegistryUser(ctx, strings.TrimSpace(username), hash, auth.RoleAdmin)
	if err != nil {
		return fmt.Errorf("bootstrap admin user: %w", err)
	}
	return nil
}

func (s *service) Login(ctx context.Context, request *models.LoginRequest) (*models.LoginResponse, error) {
	if s.users == nil || s.jwt == nil {
		return nil, fmt.Errorf("authentication is not configured")
	}
	if request == nil {
		return nil, fmt.Errorf("login request is required")
	}
	username := strings.TrimSpace(request.Username)
	password := strings.TrimSpace(request.Password)
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}
	user, err := s.users.GetAuthUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if user == nil || internaldb.CheckPassword(user.PasswordHash, password) != nil {
		return nil, auth.ErrUnauthenticated
	}
	claims := auth.JWTClaims{
		AuthMethod:        auth.MethodPassword,
		AuthMethodSubject: user.Username,
		UserID:            user.ID,
		Username:          user.Username,
		Role:              user.Role,
		Permissions:       permissionsForRole(user.Role),
	}
	response, err := s.jwt.GenerateTokenResponseWithDuration(claims, sessionDuration)
	if err != nil {
		return nil, fmt.Errorf("generate login token: %w", err)
	}
	return &models.LoginResponse{
		Token:     response.RegistryToken,
		ExpiresAt: int64(response.ExpiresAt),
		User: models.RegistryUser{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
	}, nil
}

func (s *service) Me(ctx context.Context) (*models.RegistryUser, error) {
	user, err := s.currentUser(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.users.GetAuthUserByID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, auth.ErrUnauthenticated
	}
	return &models.RegistryUser{ID: record.ID, Username: record.Username, Role: record.Role}, nil
}

func (s *service) ListUsers(ctx context.Context) ([]*models.RegistryUser, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	return s.users.ListRegistryUsers(ctx)
}

func (s *service) CreateUser(ctx context.Context, request *models.CreateUserRequest) (*models.RegistryUser, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("create user request is required")
	}
	username := strings.TrimSpace(request.Username)
	password := strings.TrimSpace(request.Password)
	role := strings.ToLower(strings.TrimSpace(request.Role))
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}
	if role == "" {
		role = auth.RoleUser
	}
	if role != auth.RoleUser && role != auth.RoleAdmin {
		return nil, fmt.Errorf("unsupported role %q", request.Role)
	}
	hash, err := internaldb.HashPassword(password)
	if err != nil {
		return nil, err
	}
	return s.users.CreateRegistryUser(ctx, username, hash, role)
}

func (s *service) ListAPIKeys(ctx context.Context) ([]*models.APIKey, error) {
	user, err := s.currentUser(ctx)
	if err != nil {
		return nil, err
	}
	return s.apiKeys.ListRegistryAPIKeysByUser(ctx, user.ID)
}

func (s *service) CreateAPIKey(ctx context.Context, request *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	user, err := s.currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if request == nil || strings.TrimSpace(request.Name) == "" {
		return nil, fmt.Errorf("API key name is required")
	}
	secret, err := generateAPIKeySecret()
	if err != nil {
		return nil, err
	}
	key, err := s.apiKeys.CreateRegistryAPIKey(ctx, user.ID, request.Name, secret)
	if err != nil {
		return nil, err
	}
	return &models.CreateAPIKeyResponse{APIKey: *key, Secret: secret}, nil
}

func (s *service) DeleteAPIKey(ctx context.Context, keyID string) error {
	user, err := s.currentUser(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(keyID) == "" {
		return fmt.Errorf("API key id is required")
	}
	return s.apiKeys.DeleteRegistryAPIKey(ctx, user.ID, keyID)
}

func (s *service) GetSettings(ctx context.Context) (*models.RegistryAuthSettings, error) {
	if s.settings == nil {
		return &models.RegistryAuthSettings{APIKeyValidationEnabled: true}, nil
	}
	return s.settings.GetRegistryAuthSettings(ctx)
}

func (s *service) UpdateSettings(ctx context.Context, request *models.UpdateRegistryAuthSettingsRequest) (*models.RegistryAuthSettings, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("settings request is required")
	}
	if s.settings == nil {
		return nil, fmt.Errorf("settings store is not configured")
	}
	return s.settings.UpdateRegistryAuthSettings(ctx, request.APIKeyValidationEnabled)
}

func (s *service) currentUser(ctx context.Context) (*auth.User, error) {
	session, ok := auth.AuthSessionFrom(ctx)
	if !ok || session == nil {
		return nil, auth.ErrUnauthenticated
	}
	user := session.Principal().User
	if strings.TrimSpace(user.ID) == "" {
		return nil, auth.ErrUnauthenticated
	}
	return &user, nil
}

func (s *service) requireAdmin(ctx context.Context) error {
	user, err := s.currentUser(ctx)
	if err != nil {
		return err
	}
	if strings.EqualFold(user.Role, auth.RoleAdmin) {
		return nil
	}
	for _, permission := range user.Permissions {
		if permission.ResourcePattern == "*" {
			return nil
		}
	}
	return auth.ErrForbidden
}

func permissionsForRole(role string) []auth.Permission {
	if strings.EqualFold(role, auth.RoleAdmin) {
		return []auth.Permission{{Action: auth.PermissionActionRead, ResourcePattern: "*"}, {Action: auth.PermissionActionPublish, ResourcePattern: "*"}, {Action: auth.PermissionActionEdit, ResourcePattern: "*"}, {Action: auth.PermissionActionDelete, ResourcePattern: "*"}}
	}
	return nil
}

func generateAPIKeySecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate API key: %w", err)
	}
	return "ar_sk_" + hex.EncodeToString(buf), nil
}

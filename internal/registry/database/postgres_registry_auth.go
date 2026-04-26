package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"strings"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
)

type registryUserStore struct {
	repositoryBase
}

type registryAPIKeyStore struct {
	repositoryBase
}

type resourceOwnerStore struct {
	repositoryBase
}

type registryAuthSettingsStore struct {
	repositoryBase
}

type RegistryUserStore interface {
	auth.UserCredentialStore
	ListRegistryUsers(ctx context.Context) ([]*models.RegistryUser, error)
	CreateRegistryUser(ctx context.Context, username, passwordHash, role string) (*models.RegistryUser, error)
	CountRegistryAdmins(ctx context.Context) (int, error)
}

type RegistryAPIKeyStore interface {
	auth.APIKeyCredentialStore
	ListRegistryAPIKeysByUser(ctx context.Context, userID string) ([]*models.APIKey, error)
	CreateRegistryAPIKey(ctx context.Context, userID, name, secret string) (*models.APIKey, error)
	DeleteRegistryAPIKey(ctx context.Context, userID, keyID string) error
}

type ResourceOwnerStore interface {
	auth.ResourceOwnerLookup
	AssignResourceOwner(ctx context.Context, resourceType auth.PermissionArtifactType, resourceName, ownerUserID string) error
}

type RegistryAuthSettingsStore interface {
	GetRegistryAuthSettings(ctx context.Context) (*models.RegistryAuthSettings, error)
	UpdateRegistryAuthSettings(ctx context.Context, enabled bool) (*models.RegistryAuthSettings, error)
}

func hashAPIKey(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hashed), nil
}

func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func (s *registryUserStore) GetAuthUserByUsername(ctx context.Context, username string) (*auth.UserRecord, error) {
	trimmed := strings.TrimSpace(username)
	if trimmed == "" {
		return nil, nil
	}
	var record auth.UserRecord
	if err := s.executor.QueryRow(ctx, `
		SELECT id, username, password_hash, role
		FROM registry_users
		WHERE username = $1
	`, trimmed).Scan(&record.ID, &record.Username, &record.PasswordHash, &record.Role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get registry user by username %q: %w", trimmed, err)
	}
	return &record, nil
}

func (s *registryUserStore) GetAuthUserByID(ctx context.Context, userID string) (*auth.UserRecord, error) {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return nil, nil
	}
	var record auth.UserRecord
	if err := s.executor.QueryRow(ctx, `
		SELECT id, username, password_hash, role
		FROM registry_users
		WHERE id = $1
	`, trimmed).Scan(&record.ID, &record.Username, &record.PasswordHash, &record.Role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get registry user by id %q: %w", trimmed, err)
	}
	return &record, nil
}

func (s *registryUserStore) ListRegistryUsers(ctx context.Context) ([]*models.RegistryUser, error) {
	rows, err := s.executor.Query(ctx, `
		SELECT id, username, role, created_at, updated_at
		FROM registry_users
		ORDER BY username ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list registry users: %w", err)
	}
	defer rows.Close()

	users := make([]*models.RegistryUser, 0)
	for rows.Next() {
		var user models.RegistryUser
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan registry user: %w", err)
		}
		users = append(users, &user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate registry users: %w", err)
	}
	return users, nil
}

func (s *registryUserStore) CreateRegistryUser(ctx context.Context, username, passwordHash, role string) (*models.RegistryUser, error) {
	trimmedUsername := strings.TrimSpace(username)
	trimmedRole := strings.ToLower(strings.TrimSpace(role))
	if trimmedUsername == "" || strings.TrimSpace(passwordHash) == "" {
		return nil, fmt.Errorf("username and password hash are required")
	}
	if trimmedRole == "" {
		trimmedRole = auth.RoleUser
	}
	var user models.RegistryUser
	if err := s.executor.QueryRow(ctx, `
		INSERT INTO registry_users (username, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id, username, role, created_at, updated_at
	`, trimmedUsername, passwordHash, trimmedRole).Scan(&user.ID, &user.Username, &user.Role, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create registry user %q: %w", trimmedUsername, err)
	}
	return &user, nil
}

func (s *registryUserStore) CountRegistryAdmins(ctx context.Context) (int, error) {
	var count int
	if err := s.executor.QueryRow(ctx, `SELECT COUNT(*) FROM registry_users WHERE role = 'admin'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count registry admins: %w", err)
	}
	return count, nil
}

func (s *registryAPIKeyStore) GetAuthAPIKeyBySecret(ctx context.Context, secret string) (*auth.APIKeyRecord, error) {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return nil, nil
	}
	var record auth.APIKeyRecord
	if err := s.executor.QueryRow(ctx, `
		SELECT k.id, u.id, u.username, u.role
		FROM registry_api_keys k
		JOIN registry_users u ON u.id = k.user_id
		WHERE k.key_hash = $1 AND k.revoked_at IS NULL
	`, hashAPIKey(trimmed)).Scan(&record.ID, &record.UserID, &record.Username, &record.Role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get registry api key: %w", err)
	}
	return &record, nil
}

func (s *registryAPIKeyStore) TouchAuthAPIKey(ctx context.Context, keyID string) error {
	if strings.TrimSpace(keyID) == "" {
		return nil
	}
	_, err := s.executor.Exec(ctx, `UPDATE registry_api_keys SET last_used_at = NOW() WHERE id = $1`, keyID)
	if err != nil {
		return fmt.Errorf("touch registry api key %q: %w", keyID, err)
	}
	return nil
}

func (s *registryAPIKeyStore) ListRegistryAPIKeysByUser(ctx context.Context, userID string) ([]*models.APIKey, error) {
	rows, err := s.executor.Query(ctx, `
		SELECT id, name, key_prefix, created_at, last_used_at, revoked_at
		FROM registry_api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list registry api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]*models.APIKey, 0)
	for rows.Next() {
		var key models.APIKey
		if err := rows.Scan(&key.ID, &key.Name, &key.Prefix, &key.CreatedAt, &key.LastUsedAt, &key.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan registry api key: %w", err)
		}
		keys = append(keys, &key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate registry api keys: %w", err)
	}
	return keys, nil
}

func (s *registryAPIKeyStore) CreateRegistryAPIKey(ctx context.Context, userID, name, secret string) (*models.APIKey, error) {
	trimmedName := strings.TrimSpace(name)
	if strings.TrimSpace(userID) == "" || trimmedName == "" || strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("user id, key name, and secret are required")
	}
	prefix := trimmedName
	if len(prefix) > 24 {
		prefix = prefix[:24]
	}
	keyPrefix := secret
	if len(keyPrefix) > 12 {
		keyPrefix = keyPrefix[:12]
	}
	var key models.APIKey
	if err := s.executor.QueryRow(ctx, `
		INSERT INTO registry_api_keys (user_id, name, key_prefix, key_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, key_prefix, created_at, last_used_at, revoked_at
	`, userID, trimmedName, keyPrefix, hashAPIKey(secret)).Scan(&key.ID, &key.Name, &key.Prefix, &key.CreatedAt, &key.LastUsedAt, &key.RevokedAt); err != nil {
		return nil, fmt.Errorf("create registry api key %q: %w", trimmedName, err)
	}
	return &key, nil
}

func (s *registryAPIKeyStore) DeleteRegistryAPIKey(ctx context.Context, userID, keyID string) error {
	result, err := s.executor.Exec(ctx, `DELETE FROM registry_api_keys WHERE id = $1 AND user_id = $2`, keyID, userID)
	if err != nil {
		return fmt.Errorf("delete registry api key %q: %w", keyID, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("registry api key not found")
	}
	return nil
}

func (s *resourceOwnerStore) GetAuthResourceOwner(ctx context.Context, resourceType auth.PermissionArtifactType, resourceName string) (*auth.ResourceOwnerRecord, error) {
	trimmedName := strings.TrimSpace(resourceName)
	if trimmedName == "" {
		return nil, nil
	}
	var record auth.ResourceOwnerRecord
	if err := s.executor.QueryRow(ctx, `
		SELECT resource_type, resource_name, owner_user_id
		FROM registry_resource_owners
		WHERE resource_type = $1 AND resource_name = $2
	`, string(resourceType), trimmedName).Scan(&record.ResourceType, &record.ResourceName, &record.OwnerUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get resource owner %s/%s: %w", resourceType, trimmedName, err)
	}
	return &record, nil
}

func (s *resourceOwnerStore) AssignResourceOwner(ctx context.Context, resourceType auth.PermissionArtifactType, resourceName, ownerUserID string) error {
	trimmedName := strings.TrimSpace(resourceName)
	trimmedOwner := strings.TrimSpace(ownerUserID)
	if resourceType == "" || trimmedName == "" || trimmedOwner == "" {
		return nil
	}
	_, err := s.executor.Exec(ctx, `
		INSERT INTO registry_resource_owners (resource_type, resource_name, owner_user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (resource_type, resource_name) DO NOTHING
	`, string(resourceType), trimmedName, trimmedOwner)
	if err != nil {
		return fmt.Errorf("assign resource owner %s/%s: %w", resourceType, trimmedName, err)
	}
	return nil
}

func (s *registryAuthSettingsStore) GetRegistryAuthSettings(ctx context.Context) (*models.RegistryAuthSettings, error) {
	var settings models.RegistryAuthSettings
	if err := s.executor.QueryRow(ctx, `
		SELECT api_key_validation_enabled, updated_at
		FROM registry_auth_settings
		WHERE singleton = TRUE
	`).Scan(&settings.APIKeyValidationEnabled, &settings.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &models.RegistryAuthSettings{APIKeyValidationEnabled: true}, nil
		}
		return nil, fmt.Errorf("get registry auth settings: %w", err)
	}
	return &settings, nil
}

func (s *registryAuthSettingsStore) UpdateRegistryAuthSettings(ctx context.Context, enabled bool) (*models.RegistryAuthSettings, error) {
	var settings models.RegistryAuthSettings
	if err := s.executor.QueryRow(ctx, `
		INSERT INTO registry_auth_settings (singleton, api_key_validation_enabled)
		VALUES (TRUE, $1)
		ON CONFLICT (singleton) DO UPDATE
		SET api_key_validation_enabled = EXCLUDED.api_key_validation_enabled,
		    updated_at = NOW()
		RETURNING api_key_validation_enabled, updated_at
	`, enabled).Scan(&settings.APIKeyValidationEnabled, &settings.UpdatedAt); err != nil {
		return nil, fmt.Errorf("update registry auth settings: %w", err)
	}
	return &settings, nil
}

package auth

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/danielgtaylor/huma/v2"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

type UserRecord struct {
	ID           string
	Username     string
	PasswordHash string
	Role         string
}

type APIKeyRecord struct {
	ID       string
	UserID   string
	Username string
	Role     string
}

type ResourceOwnerRecord struct {
	ResourceType PermissionArtifactType
	ResourceName string
	OwnerUserID  string
}

type UserCredentialStore interface {
	GetAuthUserByUsername(ctx context.Context, username string) (*UserRecord, error)
	GetAuthUserByID(ctx context.Context, userID string) (*UserRecord, error)
}

type APIKeyCredentialStore interface {
	GetAuthAPIKeyBySecret(ctx context.Context, secret string) (*APIKeyRecord, error)
	TouchAuthAPIKey(ctx context.Context, keyID string) error
}

type ResourceOwnerLookup interface {
	GetAuthResourceOwner(ctx context.Context, resourceType PermissionArtifactType, resourceName string) (*ResourceOwnerRecord, error)
}

type RegistryAuthSettingsLookup interface {
	GetRegistryAuthSettings(ctx context.Context) (*models.RegistryAuthSettings, error)
}

type userSession struct {
	principal Principal
}

func (s *userSession) Principal() Principal {
	return s.principal
}

type RegistryAuthnProvider struct {
	jwt     *JWTManager
	users   UserCredentialStore
	apiKeys APIKeyCredentialStore
}

func NewRegistryAuthnProvider(jwt *JWTManager) *RegistryAuthnProvider {
	return &RegistryAuthnProvider{jwt: jwt}
}

func (p *RegistryAuthnProvider) SetStores(users UserCredentialStore, apiKeys APIKeyCredentialStore) {
	p.users = users
	p.apiKeys = apiKeys
}

func (p *RegistryAuthnProvider) Authenticate(ctx context.Context, reqHeaders func(name string) string, query url.Values) (Session, error) {
	if p == nil {
		return nil, nil
	}
	const bearerPrefix = "Bearer "
	authHeader := strings.TrimSpace(reqHeaders("Authorization"))
	if len(authHeader) < len(bearerPrefix) || !strings.EqualFold(authHeader[:len(bearerPrefix)], bearerPrefix) {
		return nil, nil
	}
	credential := strings.TrimSpace(authHeader[len(bearerPrefix):])
	if credential == "" {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if p.jwt != nil {
		claims, err := p.jwt.ValidateToken(ctx, credential)
		if err == nil {
			return &jwtSession{claims: claims}, nil
		}
	}

	if p.apiKeys != nil {
		record, err := p.apiKeys.GetAuthAPIKeyBySecret(ctx, credential)
		if err != nil {
			return nil, huma.Error401Unauthorized("Invalid API key", err)
		}
		if record != nil {
			_ = p.apiKeys.TouchAuthAPIKey(ctx, record.ID)
			return &userSession{principal: Principal{User: User{
				ID:       record.UserID,
				Username: record.Username,
				Role:     record.Role,
			}}}, nil
		}
	}

	return nil, huma.Error401Unauthorized("Invalid or expired authentication token")
}

type RegistryAuthzProvider struct {
	publicActions map[PermissionAction]bool
	owners        ResourceOwnerLookup
	settings      RegistryAuthSettingsLookup
}

func NewRegistryAuthzProvider(publicActions []PermissionAction) *RegistryAuthzProvider {
	actions := make(map[PermissionAction]bool, len(publicActions))
	for _, action := range publicActions {
		actions[action] = true
	}
	return &RegistryAuthzProvider{publicActions: actions}
}

func (p *RegistryAuthzProvider) SetOwnerLookup(owners ResourceOwnerLookup) {
	p.owners = owners
}

func (p *RegistryAuthzProvider) SetAuthSettingsLookup(settings RegistryAuthSettingsLookup) {
	p.settings = settings
}

func (p *RegistryAuthzProvider) Check(ctx context.Context, s Session, verb PermissionAction, resource Resource) error {
	if p == nil {
		return nil
	}
	if p.IsRegistryAdmin(ctx, s) {
		return nil
	}
	if verb == PermissionActionRead {
		if s != nil {
			return nil
		}
		if p.allowsAnonymousRead(ctx) {
			return nil
		}
		return ErrUnauthenticated
	}
	if p.publicActions[verb] {
		return nil
	}
	if s == nil {
		return ErrUnauthenticated
	}
	if hasPermission(resource.Name, verb, s.Principal().User.Permissions) {
		return nil
	}

	if resource.Type == PermissionArtifactTypeSource || resource.Type == PermissionArtifactTypeProvider {
		return ErrForbidden
	}

	userID := strings.TrimSpace(s.Principal().User.ID)
	if userID == "" || p.owners == nil {
		return ErrForbidden
	}

	owner, err := p.owners.GetAuthResourceOwner(ctx, resource.Type, resource.Name)
	if err != nil {
		return ErrForbidden
	}
	if owner == nil {
		if verb == PermissionActionPublish {
			return nil
		}
		return ErrForbidden
	}
	if strings.TrimSpace(owner.OwnerUserID) == userID {
		return nil
	}
	return ErrForbidden
}

func (p *RegistryAuthzProvider) allowsAnonymousRead(ctx context.Context) bool {
	if !p.publicActions[PermissionActionRead] {
		return false
	}
	if p.settings == nil {
		return true
	}
	settings, err := p.settings.GetRegistryAuthSettings(WithSystemContext(ctx))
	if err != nil || settings == nil {
		return false
	}
	return !settings.APIKeyValidationEnabled
}

func (p *RegistryAuthzProvider) IsRegistryAdmin(ctx context.Context, s Session) bool {
	if s == nil {
		return false
	}
	if IsSystemSession(s) {
		return true
	}
	user := s.Principal().User
	if strings.EqualFold(strings.TrimSpace(user.Role), RoleAdmin) {
		return true
	}
	for _, permission := range user.Permissions {
		if permission.ResourcePattern == "*" {
			return true
		}
	}
	return false
}

func MustRole(role string) string {
	normalized := strings.ToLower(strings.TrimSpace(role))
	switch normalized {
	case RoleAdmin:
		return RoleAdmin
	case "", RoleUser:
		return RoleUser
	default:
		panic(fmt.Sprintf("unsupported role %q", role))
	}
}

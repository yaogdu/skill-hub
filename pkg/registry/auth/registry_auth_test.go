package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
)

type ownerLookupStub struct {
	owner *auth.ResourceOwnerRecord
}

func (s ownerLookupStub) GetAuthResourceOwner(_ context.Context, _ auth.PermissionArtifactType, _ string) (*auth.ResourceOwnerRecord, error) {
	return s.owner, nil
}

type apiKeyStoreStub struct {
	record *auth.APIKeyRecord
}

func (s apiKeyStoreStub) GetAuthAPIKeyBySecret(_ context.Context, secret string) (*auth.APIKeyRecord, error) {
	if secret == "valid-key" {
		return s.record, nil
	}
	return nil, nil
}

func (s apiKeyStoreStub) TouchAuthAPIKey(_ context.Context, _ string) error { return nil }

type sessionStub struct {
	principal auth.Principal
}

func (s sessionStub) Principal() auth.Principal { return s.principal }

type authSettingsStub struct {
	settings *models.RegistryAuthSettings
	err      error
}

func (s authSettingsStub) GetRegistryAuthSettings(context.Context) (*models.RegistryAuthSettings, error) {
	return s.settings, s.err
}

func TestRegistryAuthzProviderAllowsOwnerEdit(t *testing.T) {
	provider := auth.NewRegistryAuthzProvider([]auth.PermissionAction{auth.PermissionActionRead})
	provider.SetOwnerLookup(ownerLookupStub{owner: &auth.ResourceOwnerRecord{OwnerUserID: "user-1"}})
	user := sessionStub{principal: auth.Principal{User: auth.User{ID: "user-1", Username: "alice", Role: auth.RoleUser}}}

	if err := provider.Check(context.Background(), user, auth.PermissionActionEdit, auth.Resource{Name: "arch/java-analyzer", Type: auth.PermissionArtifactTypeAsset}); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
}

func TestRegistryAuthzProviderAllowsAuthenticatedPublishForNewResource(t *testing.T) {
	provider := auth.NewRegistryAuthzProvider([]auth.PermissionAction{auth.PermissionActionRead})
	provider.SetOwnerLookup(ownerLookupStub{})
	user := sessionStub{principal: auth.Principal{User: auth.User{ID: "user-2", Username: "bob", Role: auth.RoleUser}}}

	if err := provider.Check(context.Background(), user, auth.PermissionActionPublish, auth.Resource{Name: "arch/new-skill", Type: auth.PermissionArtifactTypeAsset}); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
}

func TestRegistryAuthzProviderDeniesNonOwnerDelete(t *testing.T) {
	provider := auth.NewRegistryAuthzProvider([]auth.PermissionAction{auth.PermissionActionRead})
	provider.SetOwnerLookup(ownerLookupStub{owner: &auth.ResourceOwnerRecord{OwnerUserID: "owner-1"}})
	user := sessionStub{principal: auth.Principal{User: auth.User{ID: "user-3", Username: "charlie", Role: auth.RoleUser}}}

	err := provider.Check(context.Background(), user, auth.PermissionActionDelete, auth.Resource{Name: "arch/java-analyzer", Type: auth.PermissionArtifactTypeAsset})
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("Check() error = %v, want ErrForbidden", err)
	}
}

func TestRegistryAuthnProviderAuthenticatesAPIKey(t *testing.T) {
	provider := auth.NewRegistryAuthnProvider(nil)
	provider.SetStores(nil, apiKeyStoreStub{record: &auth.APIKeyRecord{ID: "key-1", UserID: "user-1", Username: "alice", Role: auth.RoleUser}})

	session, err := provider.Authenticate(context.Background(), func(name string) string {
		if name == "Authorization" {
			return "Bearer valid-key"
		}
		return ""
	}, nil)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if session == nil {
		t.Fatal("Authenticate() returned nil session")
	}
	if session.Principal().User.Username != "alice" {
		t.Fatalf("username = %q, want alice", session.Principal().User.Username)
	}
}

func TestRegistryAuthzProviderRequiresAuthForReadWhenAPIKeyValidationEnabled(t *testing.T) {
	provider := auth.NewRegistryAuthzProvider([]auth.PermissionAction{auth.PermissionActionRead})
	provider.SetAuthSettingsLookup(authSettingsStub{settings: &models.RegistryAuthSettings{APIKeyValidationEnabled: true}})

	err := provider.Check(context.Background(), nil, auth.PermissionActionRead, auth.Resource{Name: "arch/java-analyzer", Type: auth.PermissionArtifactTypeAsset})
	if !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("Check() error = %v, want ErrUnauthenticated", err)
	}
}

func TestRegistryAuthzProviderAllowsAnonymousReadWhenAPIKeyValidationDisabled(t *testing.T) {
	provider := auth.NewRegistryAuthzProvider([]auth.PermissionAction{auth.PermissionActionRead})
	provider.SetAuthSettingsLookup(authSettingsStub{settings: &models.RegistryAuthSettings{APIKeyValidationEnabled: false}})

	if err := provider.Check(context.Background(), nil, auth.PermissionActionRead, auth.Resource{Name: "arch/java-analyzer", Type: auth.PermissionArtifactTypeAsset}); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
}

func TestRegistryAuthzProviderDeniesNonAdminSourceMutation(t *testing.T) {
	provider := auth.NewRegistryAuthzProvider([]auth.PermissionAction{auth.PermissionActionRead})
	user := sessionStub{principal: auth.Principal{User: auth.User{ID: "user-4", Username: "dora", Role: auth.RoleUser}}}

	err := provider.Check(context.Background(), user, auth.PermissionActionPublish, auth.Resource{Name: "github-main", Type: auth.PermissionArtifactTypeSource})
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("Check() error = %v, want ErrForbidden", err)
	}
}

func TestRegistryAuthzProviderAllowsAdminSourceMutation(t *testing.T) {
	provider := auth.NewRegistryAuthzProvider([]auth.PermissionAction{auth.PermissionActionRead})
	admin := sessionStub{principal: auth.Principal{User: auth.User{ID: "admin-1", Username: "admin", Role: auth.RoleAdmin}}}

	if err := provider.Check(context.Background(), admin, auth.PermissionActionPublish, auth.Resource{Name: "github-main", Type: auth.PermissionArtifactTypeSource}); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
}

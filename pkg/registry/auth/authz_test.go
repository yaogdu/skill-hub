package auth_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
)

type testSession struct {
	principal auth.Principal
}

func (s testSession) Principal() auth.Principal {
	return s.principal
}

func TestResolvePublicActions(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		jwtConfigured bool
		want          []auth.PermissionAction
		wantErr       string
	}{
		{
			name:          "defaults to all actions without jwt",
			jwtConfigured: false,
			want: []auth.PermissionAction{
				auth.PermissionActionRead,
				auth.PermissionActionPublish,
				auth.PermissionActionEdit,
				auth.PermissionActionDelete,
			},
		},
		{
			name:          "defaults to read only with jwt",
			jwtConfigured: true,
			want:          []auth.PermissionAction{auth.PermissionActionRead},
		},
		{
			name:          "parses explicit list",
			raw:           " read , publish , delete ",
			jwtConfigured: true,
			want: []auth.PermissionAction{
				auth.PermissionActionRead,
				auth.PermissionActionPublish,
				auth.PermissionActionDelete,
			},
		},
		{
			name:          "deduplicates explicit list",
			raw:           "read,read,publish",
			jwtConfigured: true,
			want: []auth.PermissionAction{
				auth.PermissionActionRead,
				auth.PermissionActionPublish,
			},
		},
		{
			name:          "supports all alias",
			raw:           "all",
			jwtConfigured: true,
			want: []auth.PermissionAction{
				auth.PermissionActionRead,
				auth.PermissionActionPublish,
				auth.PermissionActionEdit,
				auth.PermissionActionDelete,
			},
		},
		{
			name:          "supports none alias",
			raw:           "none",
			jwtConfigured: true,
			want:          []auth.PermissionAction{},
		},
		{
			name:          "supports uppercase alias with whitespace",
			raw:           "  ALL  ",
			jwtConfigured: true,
			want: []auth.PermissionAction{
				auth.PermissionActionRead,
				auth.PermissionActionPublish,
				auth.PermissionActionEdit,
				auth.PermissionActionDelete,
			},
		},
		{
			name:          "rejects unknown action",
			raw:           "read,admin",
			jwtConfigured: true,
			wantErr:       `unsupported public action "admin"`,
		},
		{
			name:          "rejects empty entry",
			raw:           "read,",
			jwtConfigured: true,
			wantErr:       "public actions contains an empty entry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := auth.ResolvePublicActions(tt.raw, tt.jwtConfigured)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("ResolvePublicActions() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvePublicActions() unexpected error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ResolvePublicActions() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPublicAuthzProviderCheck(t *testing.T) {
	resource := auth.Resource{Name: "arch/java-analyzer", Type: auth.PermissionArtifactTypeSkill}
	ctx := context.Background()

	tests := []struct {
		name     string
		provider *auth.PublicAuthzProvider
		session  auth.Session
		action   auth.PermissionAction
		wantErr  error
	}{
		{
			name:     "no jwt keeps publish public",
			provider: auth.NewPublicAuthzProvider(nil),
			action:   auth.PermissionActionPublish,
		},
		{
			name:     "jwt defaults publish to authenticated",
			provider: auth.NewPublicAuthzProvider(&auth.JWTManager{}),
			action:   auth.PermissionActionPublish,
			wantErr:  auth.ErrUnauthenticated,
		},
		{
			name:     "jwt still leaves read public",
			provider: auth.NewPublicAuthzProvider(&auth.JWTManager{}),
			action:   auth.PermissionActionRead,
		},
		{
			name:     "explicit all override keeps publish public with jwt",
			provider: auth.NewPublicAuthzProvider(&auth.JWTManager{}, auth.AllPublicActions()...),
			action:   auth.PermissionActionPublish,
		},
		{
			name:     "explicit none disables anonymous read",
			provider: auth.NewPublicAuthzProviderWithActions(&auth.JWTManager{}, []auth.PermissionAction{}),
			action:   auth.PermissionActionRead,
			wantErr:  auth.ErrUnauthenticated,
		},
		{
			name:     "authenticated session needs matching permission",
			provider: auth.NewPublicAuthzProvider(&auth.JWTManager{}),
			action:   auth.PermissionActionPublish,
			session: testSession{
				principal: auth.Principal{
					User: auth.User{
						Permissions: []auth.Permission{
							{Action: auth.PermissionActionPublish, ResourcePattern: "arch/*"},
						},
					},
				},
			},
		},
		{
			name:     "authenticated session without permission is forbidden",
			provider: auth.NewPublicAuthzProvider(&auth.JWTManager{}),
			action:   auth.PermissionActionPublish,
			session: testSession{
				principal: auth.Principal{
					User: auth.User{
						Permissions: []auth.Permission{
							{Action: auth.PermissionActionEdit, ResourcePattern: "arch/*"},
						},
					},
				},
			},
			wantErr: auth.ErrForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.provider.Check(ctx, tt.session, tt.action, resource)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Check() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestPublicAuthzProviderIsRegistryAdmin(t *testing.T) {
	ctx := context.Background()
	provider := auth.NewPublicAuthzProvider(&auth.JWTManager{})

	tests := []struct {
		name    string
		session auth.Session
		want    bool
	}{
		{
			name: "nil session",
			want: false,
		},
		{
			name:    "system session",
			session: &auth.SystemSession{},
			want:    true,
		},
		{
			name: "wildcard permission is admin",
			session: testSession{
				principal: auth.Principal{
					User: auth.User{
						Permissions: []auth.Permission{
							{Action: auth.PermissionActionEdit, ResourcePattern: "*"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "scoped permission is not admin",
			session: testSession{
				principal: auth.Principal{
					User: auth.User{
						Permissions: []auth.Permission{
							{Action: auth.PermissionActionEdit, ResourcePattern: "arch/*"},
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := provider.IsRegistryAdmin(ctx, tt.session); got != tt.want {
				t.Fatalf("IsRegistryAdmin() = %v, want %v", got, tt.want)
			}
		})
	}
}

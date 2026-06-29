package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	// ErrUnauthenticated is returned when authentication is required but not provided.
	// This should be mapped to HTTP 401 Unauthorized in handlers.
	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrForbidden is returned when a user is authenticated but lacks permission.
	// This should be mapped to HTTP 403 Forbidden in handlers (or 404 to prevent info leakage).
	ErrForbidden = errors.New("forbidden")
)

// AuthzProvider defines the authorization interface.
type AuthzProvider interface {
	// Check verifies if the session can perform the action on the resource.
	// Used for single-resource operations (get, update, delete).
	Check(ctx context.Context, s Session, verb PermissionAction, resource Resource) error
	// IsRegistryAdmin checks if the session has global permissions (i.e. "*") for the registry
	// Also used by internal operations and database queries that need to bypass filtering.
	IsRegistryAdmin(ctx context.Context, s Session) bool
}

var _ AuthzProvider = &PublicAuthzProvider{}

type Authorizer struct {
	Authz AuthzProvider
}

func (a *Authorizer) Check(ctx context.Context, verb PermissionAction, resource Resource) error {
	if a.Authz == nil {
		return nil
	}
	s, _ := AuthSessionFrom(ctx)
	return a.Authz.Check(ctx, s, verb, resource)
}

func (a *Authorizer) IsRegistryAdmin(ctx context.Context) bool {
	if a.Authz == nil {
		return false
	}
	s, _ := AuthSessionFrom(ctx)
	return a.Authz.IsRegistryAdmin(ctx, s)
}

var allPublicActions = []PermissionAction{
	PermissionActionRead,
	PermissionActionPublish,
	PermissionActionEdit,
	PermissionActionDelete,
}

// PublicAuthzProvider implements AuthzProvider for the public version.
type PublicAuthzProvider struct {
	publicActions map[PermissionAction]bool
}

// NewPublicAuthzProvider creates a new public authorization provider.
// When no explicit public actions are provided, the provider defaults to:
// - all actions public when JWT auth is disabled
// - read-only public access when JWT auth is enabled
func NewPublicAuthzProvider(jwtManager *JWTManager, publicActions ...PermissionAction) *PublicAuthzProvider {
	return newPublicAuthzProvider(jwtManager != nil, publicActions, false)
}

// NewPublicAuthzProviderWithActions creates a provider from a resolved action list.
// A nil slice uses defaults, while an empty slice means no anonymous actions.
func NewPublicAuthzProviderWithActions(jwtManager *JWTManager, publicActions []PermissionAction) *PublicAuthzProvider {
	return newPublicAuthzProvider(jwtManager != nil, publicActions, true)
}

func newPublicAuthzProvider(jwtConfigured bool, publicActions []PermissionAction, explicit bool) *PublicAuthzProvider {
	if len(publicActions) == 0 {
		if !explicit {
			publicActions = DefaultPublicActions(jwtConfigured)
		}
	}
	actions := make(map[PermissionAction]bool, len(publicActions))
	for _, action := range publicActions {
		actions[action] = true
	}
	return &PublicAuthzProvider{
		publicActions: actions,
	}
}

// AllPublicActions returns the canonical list of supported public actions.
func AllPublicActions() []PermissionAction {
	return append([]PermissionAction(nil), allPublicActions...)
}

// DefaultPublicActions returns the default anonymous-access policy.
func DefaultPublicActions(jwtConfigured bool) []PermissionAction {
	if jwtConfigured {
		return []PermissionAction{PermissionActionRead}
	}
	return AllPublicActions()
}

// ResolvePublicActions parses a PUBLIC_ACTIONS-style config override.
// When raw is empty, the default policy depends on whether JWT auth is enabled.
// Supported special values are "all" and "none".
func ResolvePublicActions(raw string, jwtConfigured bool) ([]PermissionAction, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultPublicActions(jwtConfigured), nil
	}

	switch strings.ToLower(trimmed) {
	case "all":
		return AllPublicActions(), nil
	case "none":
		return []PermissionAction{}, nil
	}

	seen := make(map[PermissionAction]bool, len(allPublicActions))
	actions := make([]PermissionAction, 0, len(allPublicActions))
	for token := range strings.SplitSeq(trimmed, ",") {
		action := PermissionAction(strings.TrimSpace(strings.ToLower(token)))
		if action == "" {
			return nil, fmt.Errorf("public actions contains an empty entry")
		}
		if !isKnownPublicAction(action) {
			return nil, fmt.Errorf("unsupported public action %q", action)
		}
		if seen[action] {
			continue
		}
		seen[action] = true
		actions = append(actions, action)
	}
	return actions, nil
}

func isKnownPublicAction(action PermissionAction) bool {
	return slices.Contains(allPublicActions, action)
}

// Check verifies if the session can perform the action on the resource.
func (o *PublicAuthzProvider) Check(ctx context.Context, s Session, verb PermissionAction, resource Resource) error {
	if o.IsRegistryAdmin(ctx, s) {
		return nil
	}

	if o.publicActions[verb] {
		return nil
	}

	if s == nil {
		return ErrUnauthenticated
	}

	if !hasPermission(resource.Name, verb, s.Principal().User.Permissions) {
		return ErrForbidden
	}
	return nil
}

func (o *PublicAuthzProvider) IsRegistryAdmin(ctx context.Context, s Session) bool {
	if s == nil {
		return false
	}

	// the system session is exempt from authz checks and acts as a global admin, similar to the registry admin
	if IsSystemSession(s) {
		return true
	}

	for _, permission := range s.Principal().User.Permissions {
		if permission.ResourcePattern == "*" {
			return true
		}
	}
	return false
}

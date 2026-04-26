package database

import (
	"context"
	"strings"

	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
)

func (base repositoryBase) assignResourceOwner(ctx context.Context, resourceType auth.PermissionArtifactType, resourceName string) error {
	if base.owners == nil {
		return nil
	}
	session, ok := auth.AuthSessionFrom(ctx)
	if !ok || auth.IsSystemSession(session) {
		return nil
	}
	userID := strings.TrimSpace(session.Principal().User.ID)
	if userID == "" {
		return nil
	}
	return base.owners.AssignResourceOwner(ctx, resourceType, resourceName, userID)
}

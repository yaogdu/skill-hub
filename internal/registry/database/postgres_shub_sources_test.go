package database_test

import (
	"context"
	"testing"

	internaldb "github.com/agentregistry-dev/agentregistry/internal/registry/database"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type shubSourceStoreProvider interface {
	Sources() database.SHUBSourceStore
}

func TestPostgreSQL_PutListGetDeleteSHUBSource(t *testing.T) {
	db := internaldb.NewTestDB(t)
	ctx := context.Background()
	provider, ok := db.(shubSourceStoreProvider)
	require.True(t, ok, "test db should expose SHUB sources store")
	sources := provider.Sources()

	stored, err := sources.PutSHUBSource(ctx, &models.SHUBSource{Name: "github-main", Address: "https://github.com/acme/skills/tree/main/skills"})
	require.NoError(t, err)
	assert.Equal(t, "github-main", stored.Name)
	assert.Equal(t, "https://github.com/acme/skills/tree/main/skills", stored.Address)

	updated, err := sources.PutSHUBSource(ctx, &models.SHUBSource{Name: "github-main", Address: "https://gitlab.com/acme/skills/-/tree/main/skills"})
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com/acme/skills/-/tree/main/skills", updated.Address)
	assert.False(t, updated.UpdatedAt.Before(stored.UpdatedAt))

	listed, err := sources.ListSHUBSources(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "github-main", listed[0].Name)

	resolved, err := sources.GetSHUBSource(ctx, "github-main")
	require.NoError(t, err)
	assert.Equal(t, updated.Address, resolved.Address)

	err = sources.DeleteSHUBSource(ctx, "github-main")
	require.NoError(t, err)

	_, err = sources.GetSHUBSource(ctx, "github-main")
	require.ErrorIs(t, err, database.ErrNotFound)
}

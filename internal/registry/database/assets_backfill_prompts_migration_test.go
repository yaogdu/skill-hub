package database

import (
	"context"
	"testing"
	"time"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	pkgdb "github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration014BackfillsLegacyPromptsIntoAssets(t *testing.T) {
	store := NewTestDB(t)
	db, ok := store.(*PostgreSQL)
	require.True(t, ok, "test DB should be PostgreSQL-backed")

	ctx := WithTestSession(context.Background())
	publishedAt := time.Now().Add(-time.Hour).UTC()
	prompt := &models.PromptJSON{
		Name:        "migration-prompt",
		Description: "Legacy migration prompt",
		Version:     "1.0.0",
		Content:     "You are a migration prompt.",
	}

	_, err := db.Prompts().CreatePrompt(ctx, prompt, &models.PromptRegistryExtensions{
		Status:      "active",
		PublishedAt: publishedAt,
		UpdatedAt:   publishedAt,
		IsLatest:    true,
	})
	require.NoError(t, err)

	_, err = db.Assets().GetAssetVersion(ctx, "migration-prompt", "1.0.0")
	require.ErrorIs(t, err, pkgdb.ErrNotFound)

	sqlBytes, err := migrationFiles.ReadFile("migrations/014_backfill_legacy_prompts_into_assets.sql")
	require.NoError(t, err)
	_, err = db.pool.Exec(ctx, string(sqlBytes))
	require.NoError(t, err)

	asset, err := db.Assets().GetAssetVersion(ctx, "migration-prompt", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, asset)
	assert.Equal(t, models.AssetCategoryPrompt, asset.Asset.Category)
	assert.Equal(t, "migration-prompt", asset.Asset.ID)
	assert.Equal(t, "Legacy migration prompt", asset.Asset.Description)
	assert.Equal(t, "You are a migration prompt.", asset.Asset.SourceSkill.Body)
	assert.Equal(t, "skill-body", asset.Asset.Manifest.Entry.Kind)
	require.NotNil(t, asset.Meta.Official)
	assert.Equal(t, publishedAt.Unix(), asset.Meta.Official.PublishedAt.Unix())
}

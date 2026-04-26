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

func TestMigration013BackfillsLegacyAgentsIntoAssets(t *testing.T) {
	store := NewTestDB(t)
	db, ok := store.(*PostgreSQL)
	require.True(t, ok, "test DB should be PostgreSQL-backed")

	ctx := WithTestSession(context.Background())
	publishedAt := time.Now().Add(-time.Hour).UTC()
	agent := &models.AgentJSON{
		AgentManifest: models.AgentManifest{
			Name:          "migration-agent",
			Image:         "ghcr.io/acme/migration-agent:1.0.0",
			Language:      "python",
			Framework:     "adk",
			ModelProvider: "openai",
			ModelName:     "gpt-4o",
			Description:   "Legacy migration agent",
		},
		Title:   "Migration Agent",
		Version: "1.0.0",
	}

	_, err := db.Agents().CreateAgent(ctx, agent, &models.AgentRegistryExtensions{
		Status:      "active",
		PublishedAt: publishedAt,
		UpdatedAt:   publishedAt,
		IsLatest:    true,
	})
	require.NoError(t, err)

	_, err = db.Assets().GetAssetVersion(ctx, "migration-agent", "1.0.0")
	require.ErrorIs(t, err, pkgdb.ErrNotFound)

	sqlBytes, err := migrationFiles.ReadFile("migrations/013_backfill_legacy_agents_into_assets.sql")
	require.NoError(t, err)
	_, err = db.pool.Exec(ctx, string(sqlBytes))
	require.NoError(t, err)

	asset, err := db.Assets().GetAssetVersion(ctx, "migration-agent", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, asset)
	assert.Equal(t, models.AssetCategoryAgent, asset.Asset.Category)
	assert.Equal(t, "migration-agent", asset.Asset.ID)
	assert.Equal(t, "Legacy migration agent", asset.Asset.Description)
	assert.Equal(t, "image", asset.Asset.Manifest.Entry.Kind)
	require.NotNil(t, asset.Meta.Official)
	assert.Equal(t, publishedAt.Unix(), asset.Meta.Official.PublishedAt.Unix())

	legacy, ok := asset.Asset.Manifest.Metadata["legacyAgent"]
	require.True(t, ok, "legacyAgent metadata should be present")
	legacyMap, ok := legacy.(map[string]any)
	require.True(t, ok, "legacyAgent metadata should decode to object")
	_, ok = legacyMap["agent"]
	require.True(t, ok, "legacyAgent metadata should retain original agent payload")
}

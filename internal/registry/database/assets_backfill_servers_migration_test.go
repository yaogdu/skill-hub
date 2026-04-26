package database

import (
	"context"
	"testing"
	"time"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	pkgdb "github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	apiv0 "github.com/modelcontextprotocol/registry/pkg/api/v0"
	"github.com/modelcontextprotocol/registry/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration015BackfillsLegacyServersIntoAssets(t *testing.T) {
	store := NewTestDB(t)
	db, ok := store.(*PostgreSQL)
	require.True(t, ok, "test DB should be PostgreSQL-backed")

	ctx := WithTestSession(context.Background())
	publishedAt := time.Now().Add(-time.Hour).UTC()
	server := &apiv0.ServerJSON{
		Schema:      model.CurrentSchemaURL,
		Name:        "com.example/migration-server",
		Title:       "Migration Server",
		Description: "Legacy migration server",
		Version:     "1.0.0",
		Remotes:     []model.Transport{{Type: model.TransportTypeStreamableHTTP, URL: "https://api.example.com/mcp"}},
	}

	_, err := db.Servers().CreateServer(ctx, server, &apiv0.RegistryExtensions{
		Status:      model.StatusActive,
		PublishedAt: publishedAt,
		UpdatedAt:   publishedAt,
		IsLatest:    true,
	})
	require.NoError(t, err)
	require.NoError(t, db.Servers().UpsertServerReadme(ctx, &pkgdb.ServerReadme{
		ServerName:  server.Name,
		Version:     server.Version,
		Content:     []byte("# Migration Server\n\nREADME"),
		ContentType: "text/markdown",
		SizeBytes:   len("# Migration Server\n\nREADME"),
		FetchedAt:   publishedAt,
	}))

	_, err = db.Assets().GetAssetVersion(ctx, server.Name, server.Version)
	require.ErrorIs(t, err, pkgdb.ErrNotFound)

	sqlBytes, err := migrationFiles.ReadFile("migrations/015_backfill_legacy_servers_into_assets.sql")
	require.NoError(t, err)
	_, err = db.pool.Exec(ctx, string(sqlBytes))
	require.NoError(t, err)

	asset, err := db.Assets().GetAssetVersion(ctx, server.Name, server.Version)
	require.NoError(t, err)
	require.NotNil(t, asset)
	assert.Equal(t, models.AssetCategoryMCP, asset.Asset.Category)
	assert.Equal(t, server.Name, asset.Asset.ID)
	assert.Equal(t, "Migration Server", asset.Asset.Name)
	assert.Equal(t, "Legacy migration server", asset.Asset.Description)
	assert.Equal(t, "# Migration Server\n\nREADME", asset.Asset.SourceSkill.Body)
	assert.Equal(t, "mcp-config", asset.Asset.Manifest.Entry.Kind)
	require.NotNil(t, asset.Meta.Official)
	assert.Equal(t, publishedAt.Unix(), asset.Meta.Official.PublishedAt.Unix())

	legacy, ok := asset.Asset.Manifest.Metadata["legacyServer"]
	require.True(t, ok, "legacyServer metadata should be present")
	legacyMap, ok := legacy.(map[string]any)
	require.True(t, ok, "legacyServer metadata should decode to object")
	_, ok = legacyMap["server"]
	require.True(t, ok, "legacyServer metadata should retain original server payload")
}

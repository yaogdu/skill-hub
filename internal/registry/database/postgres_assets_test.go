package database_test

import (
	"context"
	"testing"
	"time"

	internaldb "github.com/agentregistry-dev/agentregistry/internal/registry/database"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type assetStoreProvider interface {
	Assets() database.AssetStore
}

func TestPostgreSQL_CreateAndReadAsset(t *testing.T) {
	db := internaldb.NewTestDB(t)
	ctx := context.Background()
	provider, ok := db.(assetStoreProvider)
	require.True(t, ok, "test db should expose assets store")
	assets := provider.Assets()

	created, err := assets.CreateAsset(ctx, testAsset("arch/java-analyzer", "1.2.0"), &models.AssetRegistryExtensions{
		Status:      "active",
		PublishedAt: time.Now(),
		UpdatedAt:   time.Now(),
		IsLatest:    true,
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "arch/java-analyzer", created.Asset.ID)

	latest, err := assets.GetAsset(ctx, "arch/java-analyzer")
	require.NoError(t, err)
	assert.Equal(t, "1.2.0", latest.Asset.Version)

	byVersion, err := assets.GetAssetVersion(ctx, "arch/java-analyzer", "1.2.0")
	require.NoError(t, err)
	assert.Equal(t, "java-analyzer", byVersion.Asset.Name)
}

func TestPostgreSQL_UpdateAsset(t *testing.T) {
	db := internaldb.NewTestDB(t)
	ctx := context.Background()
	provider, ok := db.(assetStoreProvider)
	require.True(t, ok, "test db should expose assets store")
	assets := provider.Assets()

	publishedAt := time.Now().Add(-time.Hour).UTC()
	_, err := assets.CreateAsset(ctx, testAsset("arch/java-analyzer", "1.2.0"), &models.AssetRegistryExtensions{
		Status:      "active",
		PublishedAt: publishedAt,
		UpdatedAt:   publishedAt,
		IsLatest:    true,
	})
	require.NoError(t, err)

	updatedAt := time.Now().UTC()
	updatedAsset := testAsset("arch/java-analyzer", "1.2.0")
	updatedAsset.Description = "Updated Java analysis asset"
	updatedAsset.Source.PackageRef = "https://gitlab.example.com/pkg/java-analyzer-1.2.0-updated.tgz"

	updated, err := assets.UpdateAsset(ctx, "arch/java-analyzer", "1.2.0", updatedAsset, &models.AssetRegistryExtensions{
		Status:      "active",
		PublishedAt: publishedAt,
		UpdatedAt:   updatedAt,
		IsLatest:    true,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Updated Java analysis asset", updated.Asset.Description)
	require.NotNil(t, updated.Asset.Source)
	assert.Equal(t, "https://gitlab.example.com/pkg/java-analyzer-1.2.0-updated.tgz", updated.Asset.Source.PackageRef)
	assert.Equal(t, publishedAt.Unix(), updated.Meta.Official.PublishedAt.Unix())
	assert.Equal(t, updatedAt.Unix(), updated.Meta.Official.UpdatedAt.Unix())
}

func TestPostgreSQL_DeleteAssetPromotesNextLatest(t *testing.T) {
	db := internaldb.NewTestDB(t)
	ctx := context.Background()
	provider, ok := db.(assetStoreProvider)
	require.True(t, ok, "test db should expose assets store")
	assets := provider.Assets()

	olderPublishedAt := time.Now().Add(-2 * time.Hour).UTC()
	latestPublishedAt := time.Now().Add(-time.Hour).UTC()
	_, err := assets.CreateAsset(ctx, testAsset("arch/java-analyzer", "1.0.0"), &models.AssetRegistryExtensions{
		Status:      "active",
		PublishedAt: olderPublishedAt,
		UpdatedAt:   olderPublishedAt,
		IsLatest:    false,
	})
	require.NoError(t, err)
	_, err = assets.CreateAsset(ctx, testAsset("arch/java-analyzer", "1.1.0"), &models.AssetRegistryExtensions{
		Status:      "active",
		PublishedAt: latestPublishedAt,
		UpdatedAt:   latestPublishedAt,
		IsLatest:    true,
	})
	require.NoError(t, err)

	err = assets.DeleteAsset(ctx, "arch/java-analyzer", "1.1.0")
	require.NoError(t, err)

	latest, err := assets.GetLatestAsset(ctx, "arch/java-analyzer")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", latest.Asset.Version)

	_, err = assets.GetAssetVersion(ctx, "arch/java-analyzer", "1.1.0")
	require.ErrorIs(t, err, database.ErrNotFound)
}

func TestPostgreSQL_ListAssetsAndHelpers(t *testing.T) {
	db := internaldb.NewTestDB(t)
	ctx := context.Background()
	provider, ok := db.(assetStoreProvider)
	require.True(t, ok, "test db should expose assets store")
	assets := provider.Assets()

	_, err := assets.CreateAsset(ctx, testAsset("arch/java-analyzer", "1.0.0"), &models.AssetRegistryExtensions{
		Status:      "active",
		PublishedAt: time.Now().Add(-time.Hour),
		UpdatedAt:   time.Now().Add(-time.Hour),
		IsLatest:    false,
	})
	require.NoError(t, err)
	_, err = assets.CreateAsset(ctx, testAsset("arch/java-analyzer", "1.1.0"), &models.AssetRegistryExtensions{
		Status:      "active",
		PublishedAt: time.Now(),
		UpdatedAt:   time.Now(),
		IsLatest:    true,
	})
	require.NoError(t, err)

	list, cursor, err := assets.ListAssets(ctx, &database.AssetFilter{Search: assetStringPtr("java"), Category: assetCategoryPtr(models.AssetCategoryPrompt)}, "", 10)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	assert.Empty(t, cursor)

	count, err := assets.CountAssetVersions(ctx, "arch/java-analyzer")
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	exists, err := assets.CheckAssetVersionExists(ctx, "arch/java-analyzer", "1.1.0")
	require.NoError(t, err)
	assert.True(t, exists)

	versions, err := assets.GetAssetVersions(ctx, "arch/java-analyzer")
	require.NoError(t, err)
	assert.Len(t, versions, 2)

	err = assets.UnmarkAssetAsLatest(ctx, "arch/java-analyzer")
	require.NoError(t, err)
	_, err = assets.GetLatestAsset(ctx, "arch/java-analyzer")
	require.ErrorIs(t, err, database.ErrNotFound)
}

func testAsset(assetID, version string) *models.Asset {
	return &models.Asset{
		ID:          assetID,
		Name:        "java-analyzer",
		Description: "Analyze Java services",
		Version:     version,
		Category:    models.AssetCategoryPrompt,
		SourceSkill: models.AssetSourceSkill{Path: models.SkillFileName, Body: "# Java Analyzer", BodyFormat: "markdown"},
		Manifest: models.AssetManifest{
			SchemaVersion: models.ShubAssetSchemaVersion,
			ID:            assetID,
			Category:      models.AssetCategoryPrompt,
			Name:          "java-analyzer",
			Description:   "Analyze Java services",
			Version:       version,
			SourceSkill:   models.AssetSourceSkill{Path: models.SkillFileName, Body: "# Java Analyzer", BodyFormat: "markdown"},
			Entry:         models.AssetEntry{Kind: "skill-body", Path: models.SkillFileName},
			Runtime:       models.AssetRuntime{Type: "none"},
		},
		Source: &models.AssetSource{RepositoryURL: "https://gitlab.example.com/arch/java-analyzer", PackageType: "tarball", PackageRef: "https://gitlab.example.com/pkg/java-analyzer.tgz"},
	}
}

func assetCategoryPtr(category models.AssetCategory) *models.AssetCategory {
	return &category
}

func assetStringPtr(value string) *string {
	return &value
}

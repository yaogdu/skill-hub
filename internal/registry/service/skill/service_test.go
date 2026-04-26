package skill_test

import (
	"context"
	"testing"
	"time"

	internaldb "github.com/agentregistry-dev/agentregistry/internal/registry/database"
	skillsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/skill"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	regdb "github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCtx returns a context with a test auth session embedded, which is
// required by the database layer for write operations.
func testCtx() context.Context {
	return internaldb.WithTestSession(context.Background())
}

// newTestSkillService creates a skill service backed by a real test DB.
func newTestSkillService(t *testing.T) skillsvc.Registry {
	t.Helper()
	testDB := internaldb.NewTestDB(t)
	return skillsvc.New(skillsvc.Dependencies{StoreDB: testDB})
}

func newTestSkillServiceWithAssets(t *testing.T) (skillsvc.Registry, regdb.AssetStore) {
	t.Helper()
	testDB := internaldb.NewTestDB(t)
	provider, ok := testDB.(interface{ Assets() regdb.AssetStore })
	require.True(t, ok, "test DB should expose asset store")
	return skillsvc.New(skillsvc.Dependencies{StoreDB: testDB}), provider.Assets()
}

// minimalSkillJSON returns a minimal valid SkillJSON suitable for testing.
func minimalSkillJSON(name, version, description string) *models.SkillJSON {
	return &models.SkillJSON{
		Name:        name,
		Version:     version,
		Description: description,
	}
}

func minimalSHUBSkillJSON(name, assetID, version, description string) *models.SkillJSON {
	return &models.SkillJSON{
		Name:        name,
		Version:     version,
		Description: description,
		SHUB: &models.SkillSHUBMetadata{
			SchemaVersion: models.ShubAssetSchemaVersion,
			AssetID:       assetID,
			Category:      models.AssetCategoryPrompt,
			Manifest: &models.AssetManifest{
				SchemaVersion: models.ShubAssetSchemaVersion,
				ID:            assetID,
				Category:      models.AssetCategoryPrompt,
				Name:          name,
				Description:   description,
				Version:       version,
				SourceSkill: models.AssetSourceSkill{
					Path:       models.SkillFileName,
					Body:       "# " + name,
					BodyFormat: "markdown",
				},
				Entry:   models.AssetEntry{Kind: "skill-body", Path: models.SkillFileName},
				Runtime: models.AssetRuntime{Type: "none"},
			},
			Source: &models.AssetSource{
				RepositoryURL: "https://example.com/" + name + ".git",
				PackageType:   "tarball",
				PackageRef:    "https://example.com/" + name + "-" + version + ".tgz",
			},
		},
	}
}

func createAssetVersion(t *testing.T, ctx context.Context, store regdb.AssetStore, id, name, version string, publishedAt time.Time, isLatest bool) *models.AssetResponse {
	t.Helper()
	description := "asset-backed skill " + name + "@" + version
	asset := &models.Asset{
		ID:          id,
		Name:        name,
		Description: description,
		Version:     version,
		Category:    models.AssetCategoryPrompt,
		SourceSkill: models.AssetSourceSkill{
			Path:       models.SkillFileName,
			BodyFormat: "markdown",
		},
		Manifest: models.AssetManifest{
			SchemaVersion: models.ShubAssetSchemaVersion,
			ID:            id,
			Category:      models.AssetCategoryPrompt,
			Name:          name,
			Description:   description,
			Version:       version,
			SourceSkill: models.AssetSourceSkill{
				Path:       models.SkillFileName,
				BodyFormat: "markdown",
			},
		},
		Source: &models.AssetSource{
			RepositoryURL: "https://example.com/" + name + ".git",
			PackageType:   "git",
			PackageRef:    "refs/tags/" + version,
		},
	}
	resp, err := store.CreateAsset(ctx, asset, &models.AssetRegistryExtensions{
		Status:      "active",
		PublishedAt: publishedAt,
		UpdatedAt:   publishedAt,
		IsLatest:    isLatest,
	})
	require.NoError(t, err)
	return resp
}

func TestApplySkill_Create(t *testing.T) {
	ctx := testCtx()
	svc := newTestSkillService(t)

	req := minimalSkillJSON("apply-create-skill", "1.0.0", "initial description")

	resp, err := svc.ApplySkill(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, req.Name, resp.Skill.Name)
	assert.Equal(t, req.Version, resp.Skill.Version)
	assert.Equal(t, req.Description, resp.Skill.Description)

	got, err := svc.GetSkillVersion(ctx, req.Name, req.Version)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, req.Version, got.Skill.Version)
}

func TestApplySkill_Update(t *testing.T) {
	ctx := testCtx()
	svc := newTestSkillService(t)

	const (
		name    = "apply-update-skill"
		version = "1.0.0"
	)

	initial := minimalSkillJSON(name, version, "original description")
	created, err := svc.PublishSkill(ctx, initial)
	require.NoError(t, err)
	require.NotNil(t, created)

	originalPublishedAt := created.Meta.Official.PublishedAt

	updated := minimalSkillJSON(name, version, "updated description")
	resp, err := svc.ApplySkill(ctx, updated)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "updated description", resp.Skill.Description)

	got, err := svc.GetSkillVersion(ctx, name, version)
	require.NoError(t, err)
	assert.Equal(t, "updated description", got.Skill.Description)

	require.NotNil(t, got.Meta.Official)
	assert.Equal(t, originalPublishedAt.Unix(), got.Meta.Official.PublishedAt.Unix(),
		"published_at should be preserved after update")
	assert.False(t, got.Meta.Official.UpdatedAt.IsZero(),
		"updated_at should be set after an update")
}

func TestApplySkill_Idempotent(t *testing.T) {
	ctx := testCtx()
	svc := newTestSkillService(t)

	req := minimalSkillJSON("apply-idempotent-skill", "1.0.0", "idempotent description")

	resp1, err := svc.ApplySkill(ctx, req)
	require.NoError(t, err, "first apply should succeed")
	require.NotNil(t, resp1)

	resp2, err := svc.ApplySkill(ctx, req)
	require.NoError(t, err, "second apply should succeed (idempotent)")
	require.NotNil(t, resp2)

	assert.Equal(t, resp1.Skill.Name, resp2.Skill.Name)
	assert.Equal(t, resp1.Skill.Version, resp2.Skill.Version)
	assert.Equal(t, resp1.Skill.Description, resp2.Skill.Description)

	versions, err := svc.GetSkillVersions(ctx, req.Name)
	require.NoError(t, err)
	assert.Len(t, versions, 1, "idempotent apply should not create duplicate versions")
}

func TestPublishSkill_MirrorsNativeAsset(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestSkillServiceWithAssets(t)

	req := minimalSHUBSkillJSON("publish-asset-skill", "acme/publish-asset-skill", "1.0.0", "publish mirrored asset")
	resp, err := svc.PublishSkill(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	asset, err := assets.GetAssetVersion(ctx, "acme/publish-asset-skill", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, asset)
	assert.Equal(t, "publish-asset-skill", asset.Asset.Name)
	assert.Equal(t, "publish mirrored asset", asset.Asset.Description)
	require.NotNil(t, asset.Asset.Source)
	assert.Equal(t, "https://example.com/publish-asset-skill-1.0.0.tgz", asset.Asset.Source.PackageRef)
	require.NotNil(t, asset.Meta.Official)
	assert.True(t, asset.Meta.Official.IsLatest)
}

func TestApplySkill_UpdatesMirroredNativeAsset(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestSkillServiceWithAssets(t)

	initial := minimalSHUBSkillJSON("apply-asset-skill", "acme/apply-asset-skill", "1.0.0", "initial mirrored asset")
	_, err := svc.PublishSkill(ctx, initial)
	require.NoError(t, err)

	before, err := assets.GetAssetVersion(ctx, "acme/apply-asset-skill", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, before.Meta.Official)
	beforePublishedAt := before.Meta.Official.PublishedAt

	updated := minimalSHUBSkillJSON("apply-asset-skill", "acme/apply-asset-skill", "1.0.0", "updated mirrored asset")
	updated.SHUB.Source.PackageRef = "https://example.com/apply-asset-skill-updated.tgz"
	resp, err := svc.ApplySkill(ctx, updated)
	require.NoError(t, err)
	require.NotNil(t, resp)

	after, err := assets.GetAssetVersion(ctx, "acme/apply-asset-skill", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "updated mirrored asset", after.Asset.Description)
	require.NotNil(t, after.Asset.Source)
	assert.Equal(t, "https://example.com/apply-asset-skill-updated.tgz", after.Asset.Source.PackageRef)
	require.NotNil(t, after.Meta.Official)
	assert.Equal(t, beforePublishedAt.Unix(), after.Meta.Official.PublishedAt.Unix())
	assert.False(t, after.Meta.Official.UpdatedAt.IsZero())
}

func TestDeleteSkill_DeletesMirroredNativeAsset(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestSkillServiceWithAssets(t)

	req := minimalSHUBSkillJSON("delete-asset-skill", "acme/delete-asset-skill", "1.0.0", "delete mirrored asset")
	_, err := svc.PublishSkill(ctx, req)
	require.NoError(t, err)

	err = svc.DeleteSkill(ctx, "delete-asset-skill", "1.0.0")
	require.NoError(t, err)

	_, err = assets.GetAssetVersion(ctx, "acme/delete-asset-skill", "1.0.0")
	require.ErrorIs(t, err, regdb.ErrNotFound)
	_, err = svc.GetSkillVersion(ctx, "delete-asset-skill", "1.0.0")
	require.ErrorIs(t, err, regdb.ErrNotFound)
}

func TestDeleteSkill_DoesNotDeleteUnrelatedAssetForLegacySkill(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestSkillServiceWithAssets(t)

	_, err := svc.PublishSkill(ctx, minimalSkillJSON("shared-name", "1.0.0", "legacy skill only"))
	require.NoError(t, err)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createAssetVersion(t, ctx, assets, "acme/shared-name", "shared-name", "1.0.0", publishedAt, true)

	err = svc.DeleteSkill(ctx, "shared-name", "1.0.0")
	require.NoError(t, err)

	got, err := svc.GetSkillVersion(ctx, "shared-name", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "shared-name", got.Skill.Name)
	assert.Equal(t, "1.0.0", got.Skill.Version)
	assert.Equal(t, "acme/shared-name", got.Skill.SHUB.AssetID)
	asset, err := assets.GetAssetVersion(ctx, "acme/shared-name", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, asset)
	assert.Equal(t, "shared-name", asset.Asset.Name)
}

func TestDeleteSkill_FallsBackToDeleteAssetOnlyByName(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestSkillServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createAssetVersion(t, ctx, assets, "acme/delete-only-asset", "delete-only-asset", "1.0.0", publishedAt, true)

	err := svc.DeleteSkill(ctx, "delete-only-asset", "1.0.0")
	require.NoError(t, err)

	_, err = assets.GetAssetVersion(ctx, "acme/delete-only-asset", "1.0.0")
	require.ErrorIs(t, err, regdb.ErrNotFound)
}

func TestGetSkill_FallsBackToAssetByID(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestSkillServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	created := createAssetVersion(t, ctx, assets, "acme/supercoder", "supercoder", "1.0.0", publishedAt, true)

	got, err := svc.GetSkill(ctx, "acme/supercoder")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.Asset.Name, got.Skill.Name)
	assert.Equal(t, created.Asset.Version, got.Skill.Version)
	require.NotNil(t, got.Skill.SHUB)
	assert.Equal(t, created.Asset.ID, got.Skill.SHUB.AssetID)
}

func TestGetSkillVersion_FallsBackToAssetByName(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestSkillServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createAssetVersion(t, ctx, assets, "acme/supercoder", "supercoder", "1.0.0", publishedAt, true)

	got, err := svc.GetSkillVersion(ctx, "supercoder", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "supercoder", got.Skill.Name)
	assert.Equal(t, "1.0.0", got.Skill.Version)
	require.NotNil(t, got.Skill.SHUB)
	assert.Equal(t, "acme/supercoder", got.Skill.SHUB.AssetID)
}

func TestGetSkillVersions_FallsBackToAssetVersionsByName(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestSkillServiceWithAssets(t)

	older := time.Now().Add(-10 * time.Minute).UTC()
	newer := time.Now().Add(-1 * time.Minute).UTC()
	createAssetVersion(t, ctx, assets, "acme/supercoder", "supercoder", "1.0.0", older, false)
	createAssetVersion(t, ctx, assets, "acme/supercoder", "supercoder", "1.1.0", newer, true)

	versions, err := svc.GetSkillVersions(ctx, "supercoder")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, "1.1.0", versions[0].Skill.Version)
	assert.Equal(t, "1.0.0", versions[1].Skill.Version)
	assert.Equal(t, "supercoder", versions[0].Skill.Name)
	assert.Equal(t, "supercoder", versions[1].Skill.Name)
}

func TestListSkills_FallsBackToAssets(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestSkillServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createAssetVersion(t, ctx, assets, "acme/supercoder", "supercoder", "1.0.0", publishedAt, true)

	search := "super"
	latest := true
	skills, nextCursor, err := svc.ListSkills(ctx, &regdb.SkillFilter{SubstringName: &search, IsLatest: &latest}, "", 10)
	require.NoError(t, err)
	assert.Empty(t, nextCursor)
	require.Len(t, skills, 1)
	assert.Equal(t, "supercoder", skills[0].Skill.Name)
	require.NotNil(t, skills[0].Skill.SHUB)
	assert.Equal(t, "acme/supercoder", skills[0].Skill.SHUB.AssetID)
}

package prompt_test

import (
	"context"
	"testing"
	"time"

	internaldb "github.com/agentregistry-dev/agentregistry/internal/registry/database"
	promptsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/prompt"
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

// newTestPromptService creates a prompt service backed by a real test DB.
func newTestPromptService(t *testing.T) promptsvc.Registry {
	t.Helper()
	testDB := internaldb.NewTestDB(t)
	return promptsvc.New(promptsvc.Dependencies{StoreDB: testDB})
}

func newTestPromptServiceWithAssets(t *testing.T) (promptsvc.Registry, regdb.AssetStore) {
	t.Helper()
	testDB := internaldb.NewTestDB(t)
	provider, ok := testDB.(interface{ Assets() regdb.AssetStore })
	require.True(t, ok, "test DB should expose asset store")
	return promptsvc.New(promptsvc.Dependencies{StoreDB: testDB}), provider.Assets()
}

// minimalPromptJSON returns a minimal valid PromptJSON suitable for testing.
func minimalPromptJSON(name, version, content string) *models.PromptJSON {
	return &models.PromptJSON{
		Name:    name,
		Version: version,
		Content: content,
	}
}

func createPromptAssetVersion(t *testing.T, ctx context.Context, store regdb.AssetStore, id, name, version, content string, publishedAt time.Time, isLatest bool) *models.AssetResponse {
	t.Helper()
	description := "prompt asset " + name + "@" + version
	asset := &models.Asset{
		ID:          id,
		Name:        name,
		Description: description,
		Version:     version,
		Category:    models.AssetCategoryPrompt,
		SourceSkill: models.AssetSourceSkill{Path: models.SkillFileName, Body: content, BodyFormat: "markdown"},
		Manifest: models.AssetManifest{
			SchemaVersion: models.ShubAssetSchemaVersion,
			ID:            id,
			Category:      models.AssetCategoryPrompt,
			Name:          name,
			Description:   description,
			Version:       version,
			SourceSkill:   models.AssetSourceSkill{Path: models.SkillFileName, Body: content, BodyFormat: "markdown"},
			Entry:         models.AssetEntry{Kind: "skill-body", Path: models.SkillFileName},
			Runtime:       models.AssetRuntime{Type: "none"},
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

func TestApplyPrompt_Create(t *testing.T) {
	ctx := testCtx()
	svc := newTestPromptService(t)

	req := minimalPromptJSON("apply-create-prompt", "1.0.0", "initial content")

	resp, err := svc.ApplyPrompt(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, req.Name, resp.Prompt.Name)
	assert.Equal(t, req.Version, resp.Prompt.Version)
	assert.Equal(t, req.Content, resp.Prompt.Content)

	got, err := svc.GetPromptVersion(ctx, req.Name, req.Version)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, req.Version, got.Prompt.Version)
}

func TestApplyPrompt_Update(t *testing.T) {
	ctx := testCtx()
	svc := newTestPromptService(t)

	const (
		name    = "apply-update-prompt"
		version = "1.0.0"
	)

	initial := minimalPromptJSON(name, version, "original content")
	created, err := svc.PublishPrompt(ctx, initial)
	require.NoError(t, err)
	require.NotNil(t, created)

	originalPublishedAt := created.Meta.Official.PublishedAt

	updated := minimalPromptJSON(name, version, "updated content")
	resp, err := svc.ApplyPrompt(ctx, updated)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "updated content", resp.Prompt.Content)

	got, err := svc.GetPromptVersion(ctx, name, version)
	require.NoError(t, err)
	assert.Equal(t, "updated content", got.Prompt.Content)

	require.NotNil(t, got.Meta.Official)
	assert.Equal(t, originalPublishedAt.Unix(), got.Meta.Official.PublishedAt.Unix(),
		"published_at should be preserved after update")
	assert.False(t, got.Meta.Official.UpdatedAt.IsZero(),
		"updated_at should be set after an update")
}

func TestApplyPrompt_Idempotent(t *testing.T) {
	ctx := testCtx()
	svc := newTestPromptService(t)

	req := minimalPromptJSON("apply-idempotent-prompt", "1.0.0", "idempotent content")

	resp1, err := svc.ApplyPrompt(ctx, req)
	require.NoError(t, err, "first apply should succeed")
	require.NotNil(t, resp1)

	resp2, err := svc.ApplyPrompt(ctx, req)
	require.NoError(t, err, "second apply should succeed (idempotent)")
	require.NotNil(t, resp2)

	assert.Equal(t, resp1.Prompt.Name, resp2.Prompt.Name)
	assert.Equal(t, resp1.Prompt.Version, resp2.Prompt.Version)
	assert.Equal(t, resp1.Prompt.Content, resp2.Prompt.Content)

	versions, err := svc.GetPromptVersions(ctx, req.Name)
	require.NoError(t, err)
	assert.Len(t, versions, 1, "idempotent apply should not create duplicate versions")
}

func TestPublishPrompt_MirrorsNativeAsset(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestPromptServiceWithAssets(t)

	req := minimalPromptJSON("welcome-prompt", "1.0.0", "You are helpful.")
	req.Description = "Welcome system prompt"
	resp, err := svc.PublishPrompt(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	asset, err := assets.GetAssetVersion(ctx, "welcome-prompt", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, asset)
	assert.Equal(t, "welcome-prompt", asset.Asset.Name)
	assert.Equal(t, "Welcome system prompt", asset.Asset.Description)
	assert.Equal(t, "You are helpful.", asset.Asset.SourceSkill.Body)
	require.NotNil(t, asset.Meta.Official)
	assert.True(t, asset.Meta.Official.IsLatest)
}

func TestApplyPrompt_UpdatesMirroredNativeAsset(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestPromptServiceWithAssets(t)

	initial := minimalPromptJSON("update-prompt", "1.0.0", "Old content")
	initial.Description = "Old description"
	_, err := svc.PublishPrompt(ctx, initial)
	require.NoError(t, err)

	before, err := assets.GetAssetVersion(ctx, "update-prompt", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, before.Meta.Official)
	beforePublishedAt := before.Meta.Official.PublishedAt

	updated := minimalPromptJSON("update-prompt", "1.0.0", "New content")
	updated.Description = "New description"
	resp, err := svc.ApplyPrompt(ctx, updated)
	require.NoError(t, err)
	require.NotNil(t, resp)

	after, err := assets.GetAssetVersion(ctx, "update-prompt", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "New description", after.Asset.Description)
	assert.Equal(t, "New content", after.Asset.SourceSkill.Body)
	require.NotNil(t, after.Meta.Official)
	assert.Equal(t, beforePublishedAt.Unix(), after.Meta.Official.PublishedAt.Unix())
	assert.False(t, after.Meta.Official.UpdatedAt.IsZero())
}

func TestDeletePrompt_DeletesMirroredNativeAsset(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestPromptServiceWithAssets(t)

	req := minimalPromptJSON("delete-prompt", "1.0.0", "Delete me")
	_, err := svc.PublishPrompt(ctx, req)
	require.NoError(t, err)

	err = svc.DeletePrompt(ctx, "delete-prompt", "1.0.0")
	require.NoError(t, err)

	_, err = assets.GetAssetVersion(ctx, "delete-prompt", "1.0.0")
	require.ErrorIs(t, err, regdb.ErrNotFound)
	_, err = svc.GetPromptVersion(ctx, "delete-prompt", "1.0.0")
	require.ErrorIs(t, err, regdb.ErrNotFound)
}

func TestDeletePrompt_DoesNotDeleteUnrelatedAssetForLegacyPrompt(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestPromptServiceWithAssets(t)

	_, err := svc.PublishPrompt(ctx, minimalPromptJSON("shared-prompt", "1.0.0", "Legacy prompt only"))
	require.NoError(t, err)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createPromptAssetVersion(t, ctx, assets, "acme/shared-prompt", "shared-prompt", "1.0.0", "Asset prompt", publishedAt, true)

	err = svc.DeletePrompt(ctx, "shared-prompt", "1.0.0")
	require.NoError(t, err)

	got, err := svc.GetPromptVersion(ctx, "shared-prompt", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "shared-prompt", got.Prompt.Name)
	assert.Equal(t, "1.0.0", got.Prompt.Version)
	assert.Equal(t, "Asset prompt", got.Prompt.Content)
	asset, err := assets.GetAssetVersion(ctx, "acme/shared-prompt", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, asset)
	assert.Equal(t, "shared-prompt", asset.Asset.Name)
}

func TestDeletePrompt_FallsBackToDeleteAssetOnlyByName(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestPromptServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createPromptAssetVersion(t, ctx, assets, "acme/delete-only-prompt", "delete-only-prompt", "1.0.0", "Delete asset only", publishedAt, true)

	err := svc.DeletePrompt(ctx, "delete-only-prompt", "1.0.0")
	require.NoError(t, err)

	_, err = assets.GetAssetVersion(ctx, "acme/delete-only-prompt", "1.0.0")
	require.ErrorIs(t, err, regdb.ErrNotFound)
}

func TestGetPrompt_FallsBackToAssetByID(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestPromptServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	created := createPromptAssetVersion(t, ctx, assets, "acme/welcome-prompt", "welcome-prompt", "1.0.0", "You are helpful.", publishedAt, true)

	got, err := svc.GetPrompt(ctx, "acme/welcome-prompt")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.Asset.Name, got.Prompt.Name)
	assert.Equal(t, created.Asset.Version, got.Prompt.Version)
	assert.Equal(t, "You are helpful.", got.Prompt.Content)
}

func TestGetPromptVersion_FallsBackToAssetByName(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestPromptServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createPromptAssetVersion(t, ctx, assets, "acme/welcome-prompt", "welcome-prompt", "1.0.0", "You are helpful.", publishedAt, true)

	got, err := svc.GetPromptVersion(ctx, "welcome-prompt", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "welcome-prompt", got.Prompt.Name)
	assert.Equal(t, "1.0.0", got.Prompt.Version)
	assert.Equal(t, "You are helpful.", got.Prompt.Content)
}

func TestGetPromptVersions_FallsBackToAssetVersionsByName(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestPromptServiceWithAssets(t)

	older := time.Now().Add(-10 * time.Minute).UTC()
	newer := time.Now().Add(-1 * time.Minute).UTC()
	createPromptAssetVersion(t, ctx, assets, "acme/welcome-prompt", "welcome-prompt", "1.0.0", "You are helpful.", older, false)
	createPromptAssetVersion(t, ctx, assets, "acme/welcome-prompt", "welcome-prompt", "1.1.0", "You are extra helpful.", newer, true)

	versions, err := svc.GetPromptVersions(ctx, "welcome-prompt")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, "1.1.0", versions[0].Prompt.Version)
	assert.Equal(t, "1.0.0", versions[1].Prompt.Version)
	assert.Equal(t, "You are extra helpful.", versions[0].Prompt.Content)
}

func TestListPrompts_FallsBackToAssets(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestPromptServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createPromptAssetVersion(t, ctx, assets, "acme/welcome-prompt", "welcome-prompt", "1.0.0", "You are helpful.", publishedAt, true)

	search := "welcome"
	latest := true
	prompts, nextCursor, err := svc.ListPrompts(ctx, &regdb.PromptFilter{SubstringName: &search, IsLatest: &latest}, "", 10)
	require.NoError(t, err)
	assert.Empty(t, nextCursor)
	require.Len(t, prompts, 1)
	assert.Equal(t, "welcome-prompt", prompts[0].Prompt.Name)
	assert.Equal(t, "You are helpful.", prompts[0].Prompt.Content)
}

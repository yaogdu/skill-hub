package agent_test

import (
	"context"
	"testing"
	"time"

	internaldb "github.com/agentregistry-dev/agentregistry/internal/registry/database"
	agentsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/agent"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	regdb "github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCtx returns a context with a test auth session embedded, which is
// required by the database layer for write operations (UpdateAgent etc.).
func testCtx() context.Context {
	return internaldb.WithTestSession(context.Background())
}

// newTestAgentService creates an agent service backed by a real test DB.
func newTestAgentService(t *testing.T) agentsvc.Registry {
	t.Helper()
	testDB := internaldb.NewTestDB(t)
	return agentsvc.New(agentsvc.Dependencies{StoreDB: testDB})
}

func newTestAgentServiceWithAssets(t *testing.T) (agentsvc.Registry, regdb.AssetStore) {
	t.Helper()
	testDB := internaldb.NewTestDB(t)
	provider, ok := testDB.(interface{ Assets() regdb.AssetStore })
	require.True(t, ok, "test DB should expose asset store")
	return agentsvc.New(agentsvc.Dependencies{StoreDB: testDB}), provider.Assets()
}

// minimalAgentJSON returns a minimal valid AgentJSON suitable for testing.
func minimalAgentJSON(name, version, description string) *models.AgentJSON {
	return &models.AgentJSON{
		AgentManifest: models.AgentManifest{
			Name:          name,
			Image:         "ghcr.io/test/agent:v1",
			Language:      "python",
			Framework:     "adk",
			ModelProvider: "openai",
			ModelName:     "gpt-4o",
			Description:   description,
		},
		Title:   name + " title",
		Version: version,
	}
}

func createAgentAssetVersion(t *testing.T, ctx context.Context, store regdb.AssetStore, id, name, version, description string, publishedAt time.Time, isLatest bool) *models.AssetResponse {
	t.Helper()
	agent := minimalAgentJSON(name, version, description)
	response, err := models.AssetResponseFromAgentResponse(&models.AgentResponse{
		Agent: *agent,
		Meta: models.AgentResponseMeta{Official: &models.AgentRegistryExtensions{
			Status:      "active",
			PublishedAt: publishedAt,
			UpdatedAt:   publishedAt,
			IsLatest:    isLatest,
		}},
	})
	require.NoError(t, err)
	response.Asset.ID = id
	response.Asset.Manifest.ID = id
	response.Meta.Official = &models.AssetRegistryExtensions{
		Status:      "active",
		PublishedAt: publishedAt,
		UpdatedAt:   publishedAt,
		IsLatest:    isLatest,
	}
	stored, err := store.CreateAsset(ctx, &response.Asset, response.Meta.Official)
	require.NoError(t, err)
	return stored
}

func TestApplyAgent_Create(t *testing.T) {
	ctx := testCtx()
	svc := newTestAgentService(t)

	req := minimalAgentJSON("apply-create-agent", "1.0.0", "initial description")

	resp, err := svc.ApplyAgent(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, req.Name, resp.Agent.Name)
	assert.Equal(t, req.Version, resp.Agent.Version)
	assert.Equal(t, req.Description, resp.Agent.Description)

	// Verify the resource persists in the DB.
	got, err := svc.GetAgentVersion(ctx, req.Name, req.Version)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, req.Version, got.Agent.Version)
}

func TestApplyAgent_Update(t *testing.T) {
	ctx := testCtx()
	svc := newTestAgentService(t)

	const (
		name    = "apply-update-agent"
		version = "1.0.0"
	)

	// Setup: publish an initial version.
	initial := minimalAgentJSON(name, version, "original description")
	created, err := svc.PublishAgent(ctx, initial)
	require.NoError(t, err)
	require.NotNil(t, created)

	originalPublishedAt := created.Meta.Official.PublishedAt

	// Action: apply same name+version with updated description.
	updated := minimalAgentJSON(name, version, "updated description")
	resp, err := svc.ApplyAgent(ctx, updated)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Assert: description is updated.
	assert.Equal(t, "updated description", resp.Agent.Description)

	// Verify persisted state.
	got, err := svc.GetAgentVersion(ctx, name, version)
	require.NoError(t, err)
	assert.Equal(t, "updated description", got.Agent.Description)

	// published_at must be preserved; updated_at must be set (not zero).
	require.NotNil(t, got.Meta.Official)
	assert.Equal(t, originalPublishedAt.Unix(), got.Meta.Official.PublishedAt.Unix(),
		"published_at should be preserved after update")
	assert.False(t, got.Meta.Official.UpdatedAt.IsZero(),
		"updated_at should be set after an update")
}

func TestApplyAgent_Idempotent(t *testing.T) {
	ctx := testCtx()
	svc := newTestAgentService(t)

	req := minimalAgentJSON("apply-idempotent-agent", "1.0.0", "idempotent description")

	// First apply — creates the resource.
	resp1, err := svc.ApplyAgent(ctx, req)
	require.NoError(t, err, "first apply should succeed")
	require.NotNil(t, resp1)

	// Second apply — same payload, should update (no error).
	resp2, err := svc.ApplyAgent(ctx, req)
	require.NoError(t, err, "second apply should succeed (idempotent)")
	require.NotNil(t, resp2)

	// Both responses describe the same resource.
	assert.Equal(t, resp1.Agent.Name, resp2.Agent.Name)
	assert.Equal(t, resp1.Agent.Version, resp2.Agent.Version)
	assert.Equal(t, resp1.Agent.Description, resp2.Agent.Description)

	// Only one version in the DB.
	versions, err := svc.GetAgentVersions(ctx, req.Name)
	require.NoError(t, err)
	assert.Len(t, versions, 1, "idempotent apply should not create duplicate versions")
}

func TestPublishAgent_CreatesMirroredNativeAsset(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestAgentServiceWithAssets(t)

	req := minimalAgentJSON("publish-agent", "1.0.0", "Agent description")
	_, err := svc.PublishAgent(ctx, req)
	require.NoError(t, err)

	asset, err := assets.GetAssetVersion(ctx, "publish-agent", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, models.AssetCategoryAgent, asset.Asset.Category)
	assert.Equal(t, "publish-agent", asset.Asset.Name)
	assert.Equal(t, "Agent description", asset.Asset.Description)
	assert.Equal(t, "image", asset.Asset.Manifest.Entry.Kind)
	assert.Contains(t, asset.Asset.SourceSkill.Body, "Agent description")
}

func TestApplyAgent_UpdatesMirroredNativeAsset(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestAgentServiceWithAssets(t)

	initial := minimalAgentJSON("update-agent", "1.0.0", "Old description")
	_, err := svc.PublishAgent(ctx, initial)
	require.NoError(t, err)

	before, err := assets.GetAssetVersion(ctx, "update-agent", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, before.Meta.Official)
	beforePublishedAt := before.Meta.Official.PublishedAt

	updated := minimalAgentJSON("update-agent", "1.0.0", "New description")
	resp, err := svc.ApplyAgent(ctx, updated)
	require.NoError(t, err)
	require.NotNil(t, resp)

	after, err := assets.GetAssetVersion(ctx, "update-agent", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "New description", after.Asset.Description)
	assert.Contains(t, after.Asset.SourceSkill.Body, "New description")
	require.NotNil(t, after.Meta.Official)
	assert.Equal(t, beforePublishedAt.Unix(), after.Meta.Official.PublishedAt.Unix())
	assert.False(t, after.Meta.Official.UpdatedAt.IsZero())
}

func TestDeleteAgent_DeletesMirroredNativeAsset(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestAgentServiceWithAssets(t)

	req := minimalAgentJSON("delete-agent", "1.0.0", "Delete me")
	_, err := svc.PublishAgent(ctx, req)
	require.NoError(t, err)

	err = svc.DeleteAgent(ctx, "delete-agent", "1.0.0")
	require.NoError(t, err)

	_, err = assets.GetAssetVersion(ctx, "delete-agent", "1.0.0")
	require.ErrorIs(t, err, regdb.ErrNotFound)
	_, err = svc.GetAgentVersion(ctx, "delete-agent", "1.0.0")
	require.ErrorIs(t, err, regdb.ErrNotFound)
}

func TestDeleteAgent_DoesNotDeleteUnrelatedAssetForLegacyAgent(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestAgentServiceWithAssets(t)

	_, err := svc.PublishAgent(ctx, minimalAgentJSON("shared-agent", "1.0.0", "Legacy agent only"))
	require.NoError(t, err)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createAgentAssetVersion(t, ctx, assets, "acme/shared-agent", "shared-agent", "1.0.0", "Asset agent", publishedAt, true)

	err = svc.DeleteAgent(ctx, "shared-agent", "1.0.0")
	require.NoError(t, err)

	got, err := svc.GetAgentVersion(ctx, "shared-agent", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "shared-agent", got.Agent.Name)
	assert.Equal(t, "1.0.0", got.Agent.Version)
	assert.Equal(t, "Asset agent", got.Agent.Description)
	asset, err := assets.GetAssetVersion(ctx, "acme/shared-agent", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, asset)
	assert.Equal(t, "shared-agent", asset.Asset.Name)
}

func TestDeleteAgent_FallsBackToDeleteAssetOnlyByName(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestAgentServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createAgentAssetVersion(t, ctx, assets, "acme/delete-only-agent", "delete-only-agent", "1.0.0", "Delete asset only", publishedAt, true)

	err := svc.DeleteAgent(ctx, "delete-only-agent", "1.0.0")
	require.NoError(t, err)

	_, err = assets.GetAssetVersion(ctx, "acme/delete-only-agent", "1.0.0")
	require.ErrorIs(t, err, regdb.ErrNotFound)
}

func TestGetAgent_FallsBackToAssetByID(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestAgentServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	created := createAgentAssetVersion(t, ctx, assets, "acme/assistant-agent", "assistant-agent", "1.0.0", "Helpful agent", publishedAt, true)

	got, err := svc.GetAgent(ctx, "acme/assistant-agent")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.Asset.Name, got.Agent.Name)
	assert.Equal(t, created.Asset.Version, got.Agent.Version)
	assert.Equal(t, "Helpful agent", got.Agent.Description)
}

func TestGetAgentVersion_FallsBackToAssetByName(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestAgentServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createAgentAssetVersion(t, ctx, assets, "acme/assistant-agent", "assistant-agent", "1.0.0", "Helpful agent", publishedAt, true)

	got, err := svc.GetAgentVersion(ctx, "assistant-agent", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "assistant-agent", got.Agent.Name)
	assert.Equal(t, "1.0.0", got.Agent.Version)
	assert.Equal(t, "Helpful agent", got.Agent.Description)
}

func TestGetAgentVersions_FallsBackToAssetVersionsByName(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestAgentServiceWithAssets(t)

	older := time.Now().Add(-10 * time.Minute).UTC()
	newer := time.Now().Add(-1 * time.Minute).UTC()
	createAgentAssetVersion(t, ctx, assets, "acme/assistant-agent", "assistant-agent", "1.0.0", "Helpful agent", older, false)
	createAgentAssetVersion(t, ctx, assets, "acme/assistant-agent", "assistant-agent", "1.1.0", "More helpful agent", newer, true)

	versions, err := svc.GetAgentVersions(ctx, "assistant-agent")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, "1.1.0", versions[0].Agent.Version)
	assert.Equal(t, "1.0.0", versions[1].Agent.Version)
	assert.Equal(t, "More helpful agent", versions[0].Agent.Description)
}

func TestListAgents_FallsBackToAssets(t *testing.T) {
	ctx := testCtx()
	svc, assets := newTestAgentServiceWithAssets(t)

	publishedAt := time.Now().Add(-5 * time.Minute).UTC()
	createAgentAssetVersion(t, ctx, assets, "acme/assistant-agent", "assistant-agent", "1.0.0", "Helpful agent", publishedAt, true)

	search := "assistant"
	latest := true
	agents, nextCursor, err := svc.ListAgents(ctx, &regdb.AgentFilter{SubstringName: &search, IsLatest: &latest}, "", 10)
	require.NoError(t, err)
	assert.Empty(t, nextCursor)
	require.Len(t, agents, 1)
	assert.Equal(t, "assistant-agent", agents[0].Agent.Name)
	assert.Equal(t, "Helpful agent", agents[0].Agent.Description)
}

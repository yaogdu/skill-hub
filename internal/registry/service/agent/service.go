package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/agentregistry-dev/agentregistry/internal/registry/config"
	"github.com/agentregistry-dev/agentregistry/internal/registry/embeddings"
	platformtypes "github.com/agentregistry-dev/agentregistry/internal/registry/platforms/types"
	"github.com/agentregistry-dev/agentregistry/internal/registry/service/internal/embeddingutil"
	"github.com/agentregistry-dev/agentregistry/internal/registry/service/internal/versionutil"
	"github.com/agentregistry-dev/agentregistry/internal/registry/validators"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/modelcontextprotocol/registry/pkg/model"
)

const (
	maxVersionsPerAgent   = 10000
	assetFallbackPageSize = 100
)

type Dependencies struct {
	StoreDB            database.Store
	Agents             database.AgentStore
	Assets             database.AssetStore
	Skills             database.SkillStore
	Prompts            database.PromptStore
	Tx                 database.Transactor
	Config             *config.Config
	EmbeddingsProvider embeddings.Provider
	Logger             *slog.Logger
}

type Registry interface {
	database.AgentReader
	PublishAgent(ctx context.Context, req *models.AgentJSON) (*models.AgentResponse, error)
	ApplyAgent(ctx context.Context, req *models.AgentJSON) (*models.AgentResponse, error)
	DeleteAgent(ctx context.Context, agentName, version string) error
	SetAgentEmbedding(ctx context.Context, agentName, version string, embedding *database.SemanticEmbedding) error
	ResolveAgentManifestSkills(ctx context.Context, manifest *models.AgentManifest) ([]platformtypes.AgentSkillRef, error)
	ResolveAgentManifestPrompts(ctx context.Context, manifest *models.AgentManifest) ([]platformtypes.ResolvedPrompt, error)
}

type registry struct {
	database.AgentStore
	assets             database.AssetStore
	skills             database.SkillStore
	prompts            database.PromptStore
	tx                 database.Transactor
	cfg                *config.Config
	embeddingsProvider embeddings.Provider
	logger             *slog.Logger
}

var _ Registry = (*registry)(nil)

type assetStoreProvider interface {
	Assets() database.AssetStore
}

func New(deps Dependencies) Registry {
	if deps.Agents == nil && deps.StoreDB != nil {
		deps.Agents = deps.StoreDB.Agents()
	}
	if deps.Assets == nil && deps.StoreDB != nil {
		if provider, ok := deps.StoreDB.(assetStoreProvider); ok {
			deps.Assets = provider.Assets()
		}
	}
	if deps.Skills == nil && deps.StoreDB != nil {
		deps.Skills = deps.StoreDB.Skills()
	}
	if deps.Prompts == nil && deps.StoreDB != nil {
		deps.Prompts = deps.StoreDB.Prompts()
	}
	if deps.Tx == nil {
		deps.Tx = deps.StoreDB
	}

	logger := deps.Logger
	if logger == nil {
		logger = slog.Default().With("component", "registry.agent")
	}

	return &registry{
		AgentStore:         deps.Agents,
		assets:             deps.Assets,
		skills:             deps.Skills,
		prompts:            deps.Prompts,
		tx:                 deps.Tx,
		cfg:                deps.Config,
		embeddingsProvider: deps.EmbeddingsProvider,
		logger:             logger,
	}
}

func (s *registry) ListAgents(ctx context.Context, filter *database.AgentFilter, cursor string, limit int) ([]*models.AgentResponse, string, error) {
	if limit <= 0 {
		limit = 30
	}
	if filter != nil && filter.Semantic != nil {
		if err := embeddingutil.EnsureQueryEmbedding(ctx, s.cfg, s.embeddingsProvider, filter.Semantic); err != nil {
			return nil, "", err
		}
	}
	if s.AgentStore != nil {
		agents, nextCursor, err := s.AgentStore.ListAgents(ctx, filter, cursor, limit)
		if err == nil && (len(agents) > 0 || s.assets == nil || !canFallbackToAssets(filter)) {
			return agents, nextCursor, nil
		}
		if err != nil && (s.assets == nil || !canFallbackToAssets(filter)) {
			return nil, "", err
		}
	}
	if s.assets == nil || !canFallbackToAssets(filter) {
		return []*models.AgentResponse{}, "", nil
	}
	return s.listAssetBackedAgents(ctx, filter, cursor, limit)
}

func (s *registry) GetAgent(ctx context.Context, agentName string) (*models.AgentResponse, error) {
	if s.AgentStore != nil {
		agent, err := s.AgentStore.GetAgent(ctx, agentName)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return agent, err
		}
	}
	return s.findAssetBackedAgent(ctx, agentName, "latest")
}

func (s *registry) GetAgentVersion(ctx context.Context, agentName, version string) (*models.AgentResponse, error) {
	if strings.TrimSpace(version) == "" || strings.EqualFold(version, "latest") {
		return s.GetAgent(ctx, agentName)
	}
	if s.AgentStore != nil {
		agent, err := s.AgentStore.GetAgentVersion(ctx, agentName, version)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return agent, err
		}
	}
	return s.findAssetBackedAgent(ctx, agentName, version)
}

func (s *registry) GetAgentVersions(ctx context.Context, agentName string) ([]*models.AgentResponse, error) {
	if s.AgentStore != nil {
		agents, err := s.AgentStore.GetAgentVersions(ctx, agentName)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return agents, err
		}
	}
	if s.assets == nil {
		return nil, database.ErrNotFound
	}

	selected, err := s.findAssetBackedAsset(ctx, agentName, "latest")
	if err != nil {
		return nil, err
	}

	versions, err := s.assets.GetAssetVersions(ctx, selected.Asset.ID)
	if err != nil {
		return nil, err
	}

	agents := make([]*models.AgentResponse, 0, len(versions))
	for _, versionedAsset := range versions {
		if versionedAsset == nil || versionedAsset.Asset.Category != models.AssetCategoryAgent {
			continue
		}
		agent, err := models.AgentResponseFromAssetResponse(versionedAsset)
		if err != nil {
			return nil, fmt.Errorf("convert asset %s@%s to agent response: %w", versionedAsset.Asset.ID, versionedAsset.Asset.Version, err)
		}
		agents = append(agents, agent)
	}
	if len(agents) == 0 {
		return nil, database.ErrNotFound
	}
	sortAgentResponsesByRecency(agents)
	return agents, nil
}

func (s *registry) PublishAgent(ctx context.Context, req *models.AgentJSON) (*models.AgentResponse, error) {
	return database.InTransactionT(ctx, s.tx, func(txCtx context.Context, scope database.Scope) (*models.AgentResponse, error) {
		return s.createAgentInTransaction(txCtx, scope.Agents(), s.assetStoreFromScope(scope), req)
	})
}

func (s *registry) ApplyAgent(ctx context.Context, req *models.AgentJSON) (*models.AgentResponse, error) {
	if req == nil || req.Name == "" || req.Version == "" {
		return nil, fmt.Errorf("invalid agent payload: name and version are required")
	}
	return database.InTransactionT(ctx, s.tx, func(txCtx context.Context, scope database.Scope) (*models.AgentResponse, error) {
		return s.applyAgentInTransaction(txCtx, scope.Agents(), s.assetStoreFromScope(scope), req)
	})
}

func (s *registry) applyAgentInTransaction(ctx context.Context, agents database.AgentStore, assets database.AssetStore, req *models.AgentJSON) (*models.AgentResponse, error) {
	exists, err := agents.CheckAgentVersionExists(ctx, req.Name, req.Version)
	if err != nil {
		return nil, err
	}
	if exists {
		if err := s.validateNoDuplicateRemoteURLs(ctx, agents, *req); err != nil {
			return nil, err
		}
		result, err := agents.UpdateAgent(ctx, req.Name, req.Version, req)
		if err != nil {
			return nil, err
		}
		if err := s.mirrorAssetInStore(ctx, assets, result); err != nil {
			return nil, err
		}
		// Trigger async embedding regeneration (spec may have changed)
		s.triggerAgentEmbeddingRegeneration(req)
		return result, nil
	}
	return s.createAgentInTransaction(ctx, agents, assets, req)
}

func (s *registry) DeleteAgent(ctx context.Context, agentName, version string) error {
	return database.InTransaction(ctx, s.tx, func(txCtx context.Context, scope database.Scope) error {
		agents := scope.Agents()
		assets := s.assetStoreFromScope(scope)

		agentExisted := false
		if assets != nil {
			_, err := agents.GetAgentVersion(txCtx, agentName, version)
			switch {
			case err == nil:
				agentExisted = true
			case errors.Is(err, database.ErrNotFound):
			default:
				return err
			}
		}

		agentErr := agents.DeleteAgent(txCtx, agentName, version)
		if agentErr != nil && !errors.Is(agentErr, database.ErrNotFound) {
			return agentErr
		}

		assetErr := database.ErrNotFound
		if assets != nil {
			switch {
			case agentExisted:
				assetErr = assets.DeleteAsset(txCtx, agentName, version)
			case errors.Is(agentErr, database.ErrNotFound):
				assetErr = s.deleteMirroredAssetInStore(txCtx, assets, agentName, version)
			}
			if assetErr != nil && !errors.Is(assetErr, database.ErrNotFound) {
				return assetErr
			}
		}

		if agentErr == nil || assetErr == nil {
			return nil
		}
		return database.ErrNotFound
	})
}

func (s *registry) SetAgentEmbedding(ctx context.Context, agentName, version string, embedding *database.SemanticEmbedding) error {
	return database.InTransaction(ctx, s.tx, func(txCtx context.Context, scope database.Scope) error {
		return scope.Agents().SetAgentEmbedding(txCtx, agentName, version, embedding)
	})
}

func (s *registry) ResolveAgentManifestSkills(ctx context.Context, manifest *models.AgentManifest) ([]platformtypes.AgentSkillRef, error) {
	if manifest == nil || len(manifest.Skills) == 0 {
		return nil, nil
	}

	resolved := make([]platformtypes.AgentSkillRef, 0, len(manifest.Skills))
	for _, skill := range manifest.Skills {
		ref, err := s.resolveSkillRef(ctx, skill)
		if err != nil {
			return nil, fmt.Errorf("resolve skill %q: %w", skill.Name, err)
		}
		resolved = append(resolved, ref)
	}
	return resolved, nil
}

func (s *registry) ResolveAgentManifestPrompts(ctx context.Context, manifest *models.AgentManifest) ([]platformtypes.ResolvedPrompt, error) {
	if manifest == nil || len(manifest.Prompts) == 0 {
		return nil, nil
	}

	resolved := make([]platformtypes.ResolvedPrompt, 0, len(manifest.Prompts))
	for _, ref := range manifest.Prompts {
		promptName := strings.TrimSpace(ref.RegistryPromptName)
		if promptName == "" {
			return nil, fmt.Errorf("prompt name is required")
		}

		version := strings.TrimSpace(ref.RegistryPromptVersion)

		var promptResp *models.PromptResponse
		var err error
		if version == "" || version == "latest" {
			promptResp, err = s.prompts.GetPrompt(ctx, promptName)
		} else {
			promptResp, err = s.prompts.GetPromptVersion(ctx, promptName, version)
		}
		if err != nil {
			return nil, fmt.Errorf("resolve prompt %q version %q: %w", promptName, version, err)
		}

		displayName := ref.Name
		if displayName == "" {
			displayName = promptName
		}
		resolved = append(resolved, platformtypes.ResolvedPrompt{
			Name:    displayName,
			Content: promptResp.Prompt.Content,
		})
	}

	return resolved, nil
}

func (s *registry) assetStoreFromScope(scope database.Scope) database.AssetStore {
	store := s.assets
	if provider, ok := scope.(assetStoreProvider); ok && provider.Assets() != nil {
		store = provider.Assets()
	}
	return store
}

func canFallbackToAssets(filter *database.AgentFilter) bool {
	if filter == nil {
		return true
	}
	return filter.RemoteURL == nil && filter.Semantic == nil
}

func toAssetFallbackFilter(filter *database.AgentFilter) *database.AssetFilter {
	category := models.AssetCategoryAgent
	if filter == nil {
		return &database.AssetFilter{Category: &category}
	}

	assetFilter := &database.AssetFilter{
		UpdatedSince: filter.UpdatedSince,
		Version:      filter.Version,
		IsLatest:     filter.IsLatest,
		Category:     &category,
	}
	if filter.SubstringName != nil {
		assetFilter.Search = filter.SubstringName
	} else if filter.Name != nil {
		assetFilter.Search = filter.Name
	}
	return assetFilter
}

func (s *registry) listAssetBackedAgents(ctx context.Context, filter *database.AgentFilter, cursor string, limit int) ([]*models.AgentResponse, string, error) {
	pageSize := limit
	if pageSize < assetFallbackPageSize {
		pageSize = assetFallbackPageSize
	}

	assetFilter := toAssetFallbackFilter(filter)
	collected := make([]*models.AgentResponse, 0, limit)
	currentCursor := cursor

	for {
		assets, nextCursor, err := s.assets.ListAssets(ctx, assetFilter, currentCursor, pageSize)
		if err != nil {
			return nil, "", err
		}
		for index, asset := range assets {
			if !matchesAgentFallbackFilter(asset, filter) {
				continue
			}
			agent, err := models.AgentResponseFromAssetResponse(asset)
			if err != nil {
				return nil, "", fmt.Errorf("convert asset %s@%s to agent response: %w", asset.Asset.ID, asset.Asset.Version, err)
			}
			collected = append(collected, agent)
			if len(collected) == limit {
				if index < len(assets)-1 || nextCursor != "" {
					return collected, assetCursorKey(asset), nil
				}
				return collected, "", nil
			}
		}
		if nextCursor == "" {
			return collected, "", nil
		}
		currentCursor = nextCursor
	}
}

func matchesAgentFallbackFilter(asset *models.AssetResponse, filter *database.AgentFilter) bool {
	if asset == nil || asset.Asset.Category != models.AssetCategoryAgent {
		return false
	}
	if filter == nil {
		return true
	}
	if filter.Name != nil && asset.Asset.ID != *filter.Name && asset.Asset.Name != *filter.Name {
		return false
	}
	if filter.SubstringName != nil {
		needle := strings.TrimSpace(*filter.SubstringName)
		if needle != "" && !containsFold(asset.Asset.ID, needle) && !containsFold(asset.Asset.Name, needle) {
			return false
		}
	}
	if filter.Version != nil && asset.Asset.Version != *filter.Version {
		return false
	}
	if filter.IsLatest != nil {
		isLatest := asset.Meta.Official != nil && asset.Meta.Official.IsLatest
		if isLatest != *filter.IsLatest {
			return false
		}
	}
	if filter.UpdatedSince != nil {
		if asset.Meta.Official == nil || !asset.Meta.Official.UpdatedAt.After(*filter.UpdatedSince) {
			return false
		}
	}
	return true
}

func containsFold(value, needle string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}

func assetCursorKey(asset *models.AssetResponse) string {
	if asset == nil {
		return ""
	}
	return asset.Asset.ID + ":" + asset.Asset.Version
}

func (s *registry) findAssetBackedAgent(ctx context.Context, agentName, version string) (*models.AgentResponse, error) {
	asset, err := s.findAssetBackedAsset(ctx, agentName, version)
	if err != nil {
		return nil, err
	}
	agent, err := models.AgentResponseFromAssetResponse(asset)
	if err != nil {
		return nil, fmt.Errorf("convert asset %s@%s to agent response: %w", asset.Asset.ID, asset.Asset.Version, err)
	}
	return agent, nil
}

func (s *registry) findAssetBackedAsset(ctx context.Context, agentName, version string) (*models.AssetResponse, error) {
	return s.findAssetBackedAssetInStore(ctx, s.assets, agentName, version)
}

func (s *registry) findAssetBackedAssetInStore(ctx context.Context, assets database.AssetStore, agentName, version string) (*models.AssetResponse, error) {
	if assets == nil {
		return nil, database.ErrNotFound
	}
	trimmedVersion := strings.TrimSpace(version)
	if trimmedVersion == "" || strings.EqualFold(trimmedVersion, "latest") {
		asset, err := assets.GetAsset(ctx, agentName)
		if err == nil {
			if asset.Asset.Category == models.AssetCategoryAgent {
				return asset, nil
			}
		} else if !errors.Is(err, database.ErrNotFound) {
			return nil, err
		}
		return s.findAssetByExactNameInStore(ctx, assets, agentName, "")
	}

	asset, err := assets.GetAssetVersion(ctx, agentName, trimmedVersion)
	if err == nil {
		if asset.Asset.Category == models.AssetCategoryAgent {
			return asset, nil
		}
	} else if !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}
	return s.findAssetByExactNameInStore(ctx, assets, agentName, trimmedVersion)
}

func (s *registry) findAssetByExactNameInStore(ctx context.Context, assets database.AssetStore, agentName, version string) (*models.AssetResponse, error) {
	if assets == nil {
		return nil, database.ErrNotFound
	}
	category := models.AssetCategoryAgent
	search := agentName
	filter := &database.AssetFilter{Search: &search, Category: &category}
	if strings.TrimSpace(version) == "" || strings.EqualFold(version, "latest") {
		latest := true
		filter.IsLatest = &latest
	} else {
		filter.Version = &version
	}

	candidates := make([]*models.AssetResponse, 0)
	cursor := ""
	for {
		listedAssets, nextCursor, err := assets.ListAssets(ctx, filter, cursor, assetFallbackPageSize)
		if err != nil {
			return nil, err
		}
		for _, asset := range listedAssets {
			if asset == nil || asset.Asset.Name != agentName || asset.Asset.Category != models.AssetCategoryAgent {
				continue
			}
			candidates = append(candidates, asset)
		}
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}
	if len(candidates) == 0 {
		return nil, database.ErrNotFound
	}
	preferNewerAssets(candidates)
	return candidates[0], nil
}

func preferNewerAssets(assets []*models.AssetResponse) {
	sort.SliceStable(assets, func(i, j int) bool {
		left := assets[i]
		right := assets[j]

		leftLatest := left.Meta.Official != nil && left.Meta.Official.IsLatest
		rightLatest := right.Meta.Official != nil && right.Meta.Official.IsLatest
		if leftLatest != rightLatest {
			return leftLatest
		}

		var leftPublishedAt, rightPublishedAt time.Time
		if left.Meta.Official != nil {
			leftPublishedAt = left.Meta.Official.PublishedAt
		}
		if right.Meta.Official != nil {
			rightPublishedAt = right.Meta.Official.PublishedAt
		}
		if cmp := versionutil.CompareVersions(left.Asset.Version, right.Asset.Version, leftPublishedAt, rightPublishedAt); cmp != 0 {
			return cmp > 0
		}
		if !leftPublishedAt.Equal(rightPublishedAt) {
			return leftPublishedAt.After(rightPublishedAt)
		}
		return left.Asset.ID < right.Asset.ID
	})
}

func sortAgentResponsesByRecency(agents []*models.AgentResponse) {
	sort.SliceStable(agents, func(i, j int) bool {
		left := agents[i]
		right := agents[j]

		leftLatest := left.Meta.Official != nil && left.Meta.Official.IsLatest
		rightLatest := right.Meta.Official != nil && right.Meta.Official.IsLatest
		if leftLatest != rightLatest {
			return leftLatest
		}

		var leftPublishedAt, rightPublishedAt time.Time
		if left.Meta.Official != nil {
			leftPublishedAt = left.Meta.Official.PublishedAt
		}
		if right.Meta.Official != nil {
			rightPublishedAt = right.Meta.Official.PublishedAt
		}
		if cmp := versionutil.CompareVersions(left.Agent.Version, right.Agent.Version, leftPublishedAt, rightPublishedAt); cmp != 0 {
			return cmp > 0
		}
		if !leftPublishedAt.Equal(rightPublishedAt) {
			return leftPublishedAt.After(rightPublishedAt)
		}
		return left.Agent.Name < right.Agent.Name
	})
}

func (s *registry) deleteMirroredAssetInStore(ctx context.Context, assets database.AssetStore, agentName, version string) error {
	if assets == nil {
		return database.ErrNotFound
	}
	asset, err := s.findAssetBackedAssetInStore(ctx, assets, agentName, version)
	if err != nil {
		return err
	}
	return assets.DeleteAsset(ctx, asset.Asset.ID, asset.Asset.Version)
}

func (s *registry) mirrorAssetInStore(ctx context.Context, assets database.AssetStore, agentResponse *models.AgentResponse) error {
	if assets == nil || agentResponse == nil {
		return nil
	}

	assetResponse, err := models.AssetResponseFromAgentResponse(agentResponse)
	if err != nil {
		return fmt.Errorf("convert agent response to asset response: %w", err)
	}

	assetMeta := assetResponse.Meta.Official
	if assetMeta == nil {
		assetMeta = &models.AssetRegistryExtensions{}
	}
	normalizedMeta := *assetMeta
	now := time.Now()
	if strings.TrimSpace(normalizedMeta.Status) == "" {
		normalizedMeta.Status = string(model.StatusActive)
	}
	if normalizedMeta.PublishedAt.IsZero() {
		normalizedMeta.PublishedAt = now
	}
	if normalizedMeta.UpdatedAt.IsZero() {
		normalizedMeta.UpdatedAt = normalizedMeta.PublishedAt
	}

	exists, err := assets.CheckAssetVersionExists(ctx, assetResponse.Asset.ID, assetResponse.Asset.Version)
	if err != nil {
		return fmt.Errorf("check mirrored asset version: %w", err)
	}
	if exists {
		if normalizedMeta.IsLatest {
			currentLatest, err := assets.GetLatestAsset(ctx, assetResponse.Asset.ID)
			if err != nil && !errors.Is(err, database.ErrNotFound) {
				return fmt.Errorf("get mirrored asset latest version: %w", err)
			}
			if err == nil && currentLatest != nil && currentLatest.Asset.Version != assetResponse.Asset.Version {
				if err := assets.UnmarkAssetAsLatest(ctx, assetResponse.Asset.ID); err != nil {
					return fmt.Errorf("unmark mirrored asset latest version: %w", err)
				}
			}
		}
		if _, err := assets.UpdateAsset(ctx, assetResponse.Asset.ID, assetResponse.Asset.Version, &assetResponse.Asset, &normalizedMeta); err != nil {
			return fmt.Errorf("update mirrored asset: %w", err)
		}
		return nil
	}

	currentLatest, err := assets.GetLatestAsset(ctx, assetResponse.Asset.ID)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return fmt.Errorf("get mirrored asset latest version: %w", err)
	}

	isNewLatest := normalizedMeta.IsLatest
	if currentLatest != nil {
		var existingPublishedAt time.Time
		if currentLatest.Meta.Official != nil {
			existingPublishedAt = currentLatest.Meta.Official.PublishedAt
		}
		if versionutil.CompareVersions(assetResponse.Asset.Version, currentLatest.Asset.Version, normalizedMeta.PublishedAt, existingPublishedAt) <= 0 {
			isNewLatest = false
		} else {
			isNewLatest = true
		}
	}

	if isNewLatest && currentLatest != nil {
		if err := assets.UnmarkAssetAsLatest(ctx, assetResponse.Asset.ID); err != nil {
			return fmt.Errorf("unmark mirrored asset latest version: %w", err)
		}
	}

	normalizedMeta.IsLatest = isNewLatest
	if _, err := assets.CreateAsset(ctx, &assetResponse.Asset, &normalizedMeta); err != nil {
		return fmt.Errorf("create mirrored asset: %w", err)
	}
	return nil
}

// validateNoDuplicateRemoteURLs ensures none of the requested remote URLs are
// already owned by a different agent. Used by both the create and apply paths
// to enforce the same uniqueness invariant.
// triggerAgentEmbeddingRegeneration kicks off async embedding regeneration if
// embedding-on-publish is enabled. The agent value is copied into the closure
// to avoid races with callers that may mutate the request after this returns.
func (s *registry) triggerAgentEmbeddingRegeneration(req *models.AgentJSON) {
	if !embeddingutil.EnabledOnPublish(s.cfg, s.embeddingsProvider) {
		return
	}
	agentCopy := *req
	go func() {
		bgCtx := context.Background()
		payload := embeddings.BuildAgentEmbeddingPayload(&agentCopy)
		if strings.TrimSpace(payload) == "" {
			return
		}
		embedding, embErr := embeddings.GenerateSemanticEmbedding(bgCtx, s.embeddingsProvider, payload, s.cfg.Embeddings.Dimensions)
		if embErr != nil {
			s.logger.Warn("failed to generate embedding for agent", "name", agentCopy.Name, "version", agentCopy.Version, "error", embErr)
			return
		}
		if embedding == nil {
			return
		}
		if storeErr := s.SetAgentEmbedding(bgCtx, agentCopy.Name, agentCopy.Version, embedding); storeErr != nil {
			s.logger.Warn("failed to store embedding for agent", "name", agentCopy.Name, "version", agentCopy.Version, "error", storeErr)
		}
	}()
}

func (s *registry) validateNoDuplicateRemoteURLs(ctx context.Context, agents database.AgentStore, agentDetail models.AgentJSON) error {
	for _, remote := range agentDetail.Remotes {
		remoteURL := remote.URL
		filter := &database.AgentFilter{RemoteURL: &remoteURL}
		cursor := ""

		for {
			conflictingAgents, nextCursor, err := agents.ListAgents(ctx, filter, cursor, 1000)
			if err != nil {
				return fmt.Errorf("failed to check remote URL conflict: %w", err)
			}
			for _, conflictingAgent := range conflictingAgents {
				if conflictingAgent.Agent.Name != agentDetail.Name {
					return fmt.Errorf("remote URL %s is already used by agent %s", remoteURL, conflictingAgent.Agent.Name)
				}
			}
			if nextCursor == "" {
				break
			}
			cursor = nextCursor
		}
	}
	return nil
}

func (s *registry) createAgentInTransaction(ctx context.Context, agents database.AgentStore, assets database.AssetStore, req *models.AgentJSON) (*models.AgentResponse, error) {
	if req == nil || req.Name == "" || req.Version == "" {
		return nil, fmt.Errorf("invalid agent payload: name and version are required")
	}

	publishTime := time.Now()
	agentJSON := *req

	if err := s.validateNoDuplicateRemoteURLs(ctx, agents, agentJSON); err != nil {
		return nil, err
	}

	versionCount, err := agents.CountAgentVersions(ctx, agentJSON.Name)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}
	if versionCount >= maxVersionsPerAgent {
		return nil, database.ErrMaxVersionsReached
	}

	exists, err := agents.CheckAgentVersionExists(ctx, agentJSON.Name, agentJSON.Version)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, database.ErrInvalidVersion
	}

	currentLatest, err := agents.GetLatestAgent(ctx, agentJSON.Name)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	isNewLatest := true
	if currentLatest != nil {
		var existingPublishedAt time.Time
		if currentLatest.Meta.Official != nil {
			existingPublishedAt = currentLatest.Meta.Official.PublishedAt
		}
		if versionutil.CompareVersions(agentJSON.Version, currentLatest.Agent.Version, publishTime, existingPublishedAt) <= 0 {
			isNewLatest = false
		}
	}

	if isNewLatest && currentLatest != nil {
		if err := agents.UnmarkAgentAsLatest(ctx, agentJSON.Name); err != nil {
			return nil, err
		}
	}

	officialMeta := &models.AgentRegistryExtensions{
		Status:      string(model.StatusActive),
		PublishedAt: publishTime,
		UpdatedAt:   publishTime,
		IsLatest:    isNewLatest,
	}

	result, err := agents.CreateAgent(ctx, &agentJSON, officialMeta)
	if err != nil {
		return nil, err
	}
	if err := s.mirrorAssetInStore(ctx, assets, result); err != nil {
		return nil, err
	}

	s.triggerAgentEmbeddingRegeneration(&agentJSON)

	return result, nil
}

func (s *registry) resolveSkillRef(ctx context.Context, skill models.SkillRef) (platformtypes.AgentSkillRef, error) {
	image := strings.TrimSpace(skill.Image)
	registrySkillName := strings.TrimSpace(skill.RegistrySkillName)
	hasImage := image != ""
	hasRegistry := registrySkillName != ""

	if !hasImage && !hasRegistry {
		return platformtypes.AgentSkillRef{}, fmt.Errorf("one of image or registrySkillName is required")
	}
	if hasImage && hasRegistry {
		return platformtypes.AgentSkillRef{}, fmt.Errorf("only one of image or registrySkillName may be set")
	}

	if hasImage {
		return platformtypes.AgentSkillRef{Name: skill.Name, Image: image}, nil
	}

	version := strings.TrimSpace(skill.RegistrySkillVersion)
	if version == "" {
		version = "latest"
	}

	skillResp, err := s.skills.GetSkillVersion(ctx, registrySkillName, version)
	if err != nil {
		return platformtypes.AgentSkillRef{}, fmt.Errorf("fetch skill %q version %q: %w", registrySkillName, version, err)
	}

	for _, pkg := range skillResp.Skill.Packages {
		typ := strings.ToLower(strings.TrimSpace(pkg.RegistryType))
		if (typ == "docker" || typ == "oci") && strings.TrimSpace(pkg.Identifier) != "" {
			return platformtypes.AgentSkillRef{Name: skill.Name, Image: strings.TrimSpace(pkg.Identifier)}, nil
		}
	}

	if skillResp.Skill.Repository != nil &&
		strings.EqualFold(skillResp.Skill.Repository.Source, string(validators.SourceGit)) &&
		strings.TrimSpace(skillResp.Skill.Repository.URL) != "" {
		return platformtypes.AgentSkillRef{
			Name:    skill.Name,
			RepoURL: strings.TrimSpace(skillResp.Skill.Repository.URL),
		}, nil
	}

	return platformtypes.AgentSkillRef{}, fmt.Errorf(
		"skill %q (version %s): no docker/oci package or git repository found",
		registrySkillName,
		version,
	)
}

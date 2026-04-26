package server

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
	"github.com/agentregistry-dev/agentregistry/internal/registry/service/internal/embeddingutil"
	"github.com/agentregistry-dev/agentregistry/internal/registry/service/internal/versionutil"
	"github.com/agentregistry-dev/agentregistry/internal/registry/validators"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	apiv0 "github.com/modelcontextprotocol/registry/pkg/api/v0"
	"github.com/modelcontextprotocol/registry/pkg/model"
)

const (
	maxVersionsPerServer  = 10000
	assetFallbackPageSize = 100
)

type Dependencies struct {
	StoreDB            database.Store
	Servers            database.ServerStore
	Assets             database.AssetStore
	Tx                 database.Transactor
	Config             *config.Config
	EmbeddingsProvider embeddings.Provider
	Logger             *slog.Logger
}

type Registry interface {
	database.ServerReader
	PublishServer(ctx context.Context, req *apiv0.ServerJSON) (*apiv0.ServerResponse, error)
	ApplyServer(ctx context.Context, req *apiv0.ServerJSON) (*apiv0.ServerResponse, error)
	UpdateServer(ctx context.Context, serverName, version string, req *apiv0.ServerJSON, newStatus *string) (*apiv0.ServerResponse, error)
	SetServerReadme(ctx context.Context, serverName, version string, content []byte, contentType string) error
	DeleteServer(ctx context.Context, serverName, version string) error
	SetServerEmbedding(ctx context.Context, serverName, version string, embedding *database.SemanticEmbedding) error
}

type registry struct {
	database.ServerStore
	assets             database.AssetStore
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
	if deps.Servers == nil && deps.StoreDB != nil {
		deps.Servers = deps.StoreDB.Servers()
	}
	if deps.Assets == nil && deps.StoreDB != nil {
		if provider, ok := deps.StoreDB.(assetStoreProvider); ok {
			deps.Assets = provider.Assets()
		}
	}
	if deps.Tx == nil {
		deps.Tx = deps.StoreDB
	}

	logger := deps.Logger
	if logger == nil {
		logger = slog.Default().With("component", "registry.server")
	}

	return &registry{
		ServerStore:        deps.Servers,
		assets:             deps.Assets,
		tx:                 deps.Tx,
		cfg:                deps.Config,
		embeddingsProvider: deps.EmbeddingsProvider,
		logger:             logger,
	}
}

func (s *registry) ListServers(ctx context.Context, filter *database.ServerFilter, cursor string, limit int) ([]*apiv0.ServerResponse, string, error) {
	if limit <= 0 {
		limit = 30
	}
	if filter != nil {
		if err := embeddingutil.EnsureQueryEmbedding(ctx, s.cfg, s.embeddingsProvider, filter.Semantic); err != nil {
			return nil, "", err
		}
	}
	if s.ServerStore != nil {
		servers, nextCursor, err := s.ServerStore.ListServers(ctx, filter, cursor, limit)
		if err == nil && (len(servers) > 0 || s.assets == nil || !canFallbackToAssets(filter)) {
			return servers, nextCursor, nil
		}
		if err != nil && (s.assets == nil || !canFallbackToAssets(filter)) {
			return nil, "", err
		}
	}
	if s.assets == nil || !canFallbackToAssets(filter) {
		return []*apiv0.ServerResponse{}, "", nil
	}
	return s.listAssetBackedServers(ctx, filter, cursor, limit)
}

func (s *registry) GetServer(ctx context.Context, serverName string) (*apiv0.ServerResponse, error) {
	if s.ServerStore != nil {
		server, err := s.ServerStore.GetServer(ctx, serverName)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return server, err
		}
	}
	return s.findAssetBackedServer(ctx, serverName, "latest")
}

func (s *registry) GetServerVersion(ctx context.Context, serverName, version string) (*apiv0.ServerResponse, error) {
	if strings.TrimSpace(version) == "" || strings.EqualFold(version, "latest") {
		return s.GetServer(ctx, serverName)
	}
	if s.ServerStore != nil {
		server, err := s.ServerStore.GetServerVersion(ctx, serverName, version)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return server, err
		}
	}
	return s.findAssetBackedServer(ctx, serverName, version)
}

func (s *registry) GetServerVersions(ctx context.Context, serverName string) ([]*apiv0.ServerResponse, error) {
	if s.ServerStore != nil {
		servers, err := s.ServerStore.GetServerVersions(ctx, serverName)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return servers, err
		}
	}
	if s.assets == nil {
		return nil, database.ErrNotFound
	}

	selected, err := s.findAssetBackedAsset(ctx, serverName, "latest")
	if err != nil {
		return nil, err
	}
	versions, err := s.assets.GetAssetVersions(ctx, selected.Asset.ID)
	if err != nil {
		return nil, err
	}

	servers := make([]*apiv0.ServerResponse, 0, len(versions))
	for _, versionedAsset := range versions {
		if versionedAsset == nil || versionedAsset.Asset.Category != models.AssetCategoryMCP {
			continue
		}
		server, err := models.ServerResponseFromAssetResponse(versionedAsset)
		if err != nil {
			return nil, fmt.Errorf("convert asset %s@%s to server response: %w", versionedAsset.Asset.ID, versionedAsset.Asset.Version, err)
		}
		servers = append(servers, server)
	}
	if len(servers) == 0 {
		return nil, database.ErrNotFound
	}
	sort.SliceStable(servers, func(i, j int) bool {
		left := servers[i]
		right := servers[j]
		leftPublishedAt := time.Time{}
		rightPublishedAt := time.Time{}
		if left != nil && left.Meta.Official != nil {
			leftPublishedAt = left.Meta.Official.PublishedAt
		}
		if right != nil && right.Meta.Official != nil {
			rightPublishedAt = right.Meta.Official.PublishedAt
		}
		if !leftPublishedAt.Equal(rightPublishedAt) {
			return leftPublishedAt.After(rightPublishedAt)
		}
		return versionutil.CompareVersions(left.Server.Version, right.Server.Version, leftPublishedAt, rightPublishedAt) > 0
	})
	return servers, nil
}

func (s *registry) GetServerReadme(ctx context.Context, serverName, version string) (*database.ServerReadme, error) {
	if strings.TrimSpace(version) == "" || strings.EqualFold(version, "latest") {
		return s.GetLatestServerReadme(ctx, serverName)
	}
	if s.ServerStore != nil {
		readme, err := s.ServerStore.GetServerReadme(ctx, serverName, version)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return readme, err
		}
	}
	return s.findAssetBackedReadme(ctx, serverName, version)
}

func (s *registry) GetLatestServerReadme(ctx context.Context, serverName string) (*database.ServerReadme, error) {
	if s.ServerStore != nil {
		readme, err := s.ServerStore.GetLatestServerReadme(ctx, serverName)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return readme, err
		}
	}
	return s.findAssetBackedReadme(ctx, serverName, "latest")
}

func (s *registry) PublishServer(ctx context.Context, req *apiv0.ServerJSON) (*apiv0.ServerResponse, error) {
	return database.InTransactionT(ctx, s.tx, func(txCtx context.Context, scope database.Scope) (*apiv0.ServerResponse, error) {
		assets := s.assets
		if provider, ok := scope.(assetStoreProvider); ok && provider.Assets() != nil {
			assets = provider.Assets()
		}
		return s.createServerInTransaction(txCtx, scope.Servers(), assets, req)
	})
}

func (s *registry) ApplyServer(ctx context.Context, req *apiv0.ServerJSON) (*apiv0.ServerResponse, error) {
	if req == nil || req.Name == "" || req.Version == "" {
		return nil, fmt.Errorf("invalid server payload: name and version are required")
	}
	return database.InTransactionT(ctx, s.tx, func(txCtx context.Context, scope database.Scope) (*apiv0.ServerResponse, error) {
		assets := s.assets
		if provider, ok := scope.(assetStoreProvider); ok && provider.Assets() != nil {
			assets = provider.Assets()
		}
		return s.applyServerInTransaction(txCtx, scope.Servers(), assets, req)
	})
}

func (s *registry) applyServerInTransaction(ctx context.Context, servers database.ServerStore, assets database.AssetStore, req *apiv0.ServerJSON) (*apiv0.ServerResponse, error) {
	exists, err := servers.CheckVersionExists(ctx, req.Name, req.Version)
	if err != nil {
		return nil, err
	}
	if exists {
		if err := s.validateNoDuplicateRemoteURLs(ctx, servers, *req); err != nil {
			return nil, err
		}
		result, err := servers.UpdateServer(ctx, req.Name, req.Version, req)
		if err != nil {
			return nil, err
		}
		if err := s.mirrorAssetInStore(ctx, assets, result, s.currentServerReadmeBody(ctx, servers, req.Name, req.Version)); err != nil {
			return nil, err
		}
		s.triggerServerEmbeddingRegeneration(req)
		return result, nil
	}
	if assets != nil {
		if _, err := s.findAssetBackedAssetInStore(ctx, assets, req.Name, req.Version); err == nil {
			return s.updateAssetBackedServerInStore(ctx, assets, req.Name, req.Version, req, nil)
		} else if !errors.Is(err, database.ErrNotFound) {
			return nil, err
		}
	}
	return s.createServerInTransaction(ctx, servers, assets, req)
}

func (s *registry) UpdateServer(ctx context.Context, serverName, version string, req *apiv0.ServerJSON, newStatus *string) (*apiv0.ServerResponse, error) {
	return database.InTransactionT(ctx, s.tx, func(txCtx context.Context, scope database.Scope) (*apiv0.ServerResponse, error) {
		assets := s.assets
		if provider, ok := scope.(assetStoreProvider); ok && provider.Assets() != nil {
			assets = provider.Assets()
		}
		return s.updateServerInTransaction(txCtx, scope.Servers(), assets, serverName, version, req, newStatus)
	})
}

func (s *registry) SetServerReadme(ctx context.Context, serverName, version string, content []byte, contentType string) error {
	if len(content) == 0 {
		return nil
	}
	if contentType == "" {
		contentType = "text/markdown"
	}

	return database.InTransaction(ctx, s.tx, func(txCtx context.Context, scope database.Scope) error {
		servers := scope.Servers()
		assets := s.assets
		if provider, ok := scope.(assetStoreProvider); ok && provider.Assets() != nil {
			assets = provider.Assets()
		}
		if _, err := servers.GetServerVersion(txCtx, serverName, version); err != nil {
			if assets != nil && errors.Is(err, database.ErrNotFound) {
				return s.updateAssetReadmeInStore(txCtx, assets, serverName, version, string(content))
			}
			return err
		}

		readme := &database.ServerReadme{
			ServerName:  serverName,
			Version:     version,
			Content:     append([]byte(nil), content...),
			ContentType: contentType,
			SizeBytes:   len(content),
			FetchedAt:   time.Now(),
		}
		if err := servers.UpsertServerReadme(txCtx, readme); err != nil {
			return err
		}
		if assets == nil {
			return nil
		}
		server, err := servers.GetServerVersion(txCtx, serverName, version)
		if err != nil {
			return err
		}
		return s.mirrorAssetInStore(txCtx, assets, server, string(content))
	})
}

func (s *registry) DeleteServer(ctx context.Context, serverName, version string) error {
	return database.InTransaction(ctx, s.tx, func(txCtx context.Context, scope database.Scope) error {
		assets := s.assets
		if provider, ok := scope.(assetStoreProvider); ok && provider.Assets() != nil {
			assets = provider.Assets()
		}
		serverErr := scope.Servers().DeleteServer(txCtx, serverName, version)
		if serverErr == nil {
			if assets == nil {
				return nil
			}
			return s.deleteMirroredAssetInStore(txCtx, assets, serverName, version)
		}
		if !errors.Is(serverErr, database.ErrNotFound) || assets == nil {
			return serverErr
		}
		assetErr := s.deleteMirroredAssetInStore(txCtx, assets, serverName, version)
		if errors.Is(assetErr, database.ErrNotFound) {
			return serverErr
		}
		return assetErr
	})
}

func (s *registry) SetServerEmbedding(ctx context.Context, serverName, version string, embedding *database.SemanticEmbedding) error {
	return database.InTransaction(ctx, s.tx, func(txCtx context.Context, scope database.Scope) error {
		return scope.Servers().SetServerEmbedding(txCtx, serverName, version, embedding)
	})
}

func canFallbackToAssets(filter *database.ServerFilter) bool {
	if filter == nil {
		return true
	}
	return filter.RemoteURL == nil && filter.Semantic == nil
}

func toAssetFallbackFilter(filter *database.ServerFilter) *database.AssetFilter {
	category := models.AssetCategoryMCP
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

func (s *registry) listAssetBackedServers(ctx context.Context, filter *database.ServerFilter, cursor string, limit int) ([]*apiv0.ServerResponse, string, error) {
	pageSize := max(limit, assetFallbackPageSize)
	assetFilter := toAssetFallbackFilter(filter)
	collected := make([]*apiv0.ServerResponse, 0, limit)
	currentCursor := cursor

	for {
		assets, nextCursor, err := s.assets.ListAssets(ctx, assetFilter, currentCursor, pageSize)
		if err != nil {
			return nil, "", err
		}
		for index, asset := range assets {
			if !matchesServerFallbackFilter(asset, filter) {
				continue
			}
			server, err := models.ServerResponseFromAssetResponse(asset)
			if err != nil {
				return nil, "", fmt.Errorf("convert asset %s@%s to server response: %w", asset.Asset.ID, asset.Asset.Version, err)
			}
			collected = append(collected, server)
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

func matchesServerFallbackFilter(asset *models.AssetResponse, filter *database.ServerFilter) bool {
	if asset == nil || asset.Asset.Category != models.AssetCategoryMCP {
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

func (s *registry) findAssetBackedServer(ctx context.Context, serverName, version string) (*apiv0.ServerResponse, error) {
	asset, err := s.findAssetBackedAsset(ctx, serverName, version)
	if err != nil {
		return nil, err
	}
	server, err := models.ServerResponseFromAssetResponse(asset)
	if err != nil {
		return nil, fmt.Errorf("convert asset %s@%s to server response: %w", asset.Asset.ID, asset.Asset.Version, err)
	}
	return server, nil
}

func (s *registry) findAssetBackedReadme(ctx context.Context, serverName, version string) (*database.ServerReadme, error) {
	asset, err := s.findAssetBackedAsset(ctx, serverName, version)
	if err != nil {
		return nil, err
	}
	body := strings.TrimSpace(asset.Asset.SourceSkill.Body)
	if body == "" {
		body = strings.TrimSpace(asset.Asset.Manifest.SourceSkill.Body)
	}
	if body == "" {
		return nil, database.ErrNotFound
	}
	return &database.ServerReadme{
		ServerName:  asset.Asset.ID,
		Version:     asset.Asset.Version,
		Content:     []byte(body),
		ContentType: "text/markdown",
		SizeBytes:   len(body),
		FetchedAt:   readmeFetchedAt(asset),
	}, nil
}

func readmeFetchedAt(asset *models.AssetResponse) time.Time {
	if asset != nil && asset.Meta.Official != nil {
		if !asset.Meta.Official.UpdatedAt.IsZero() {
			return asset.Meta.Official.UpdatedAt
		}
		return asset.Meta.Official.PublishedAt
	}
	return time.Now()
}

func (s *registry) findAssetBackedAsset(ctx context.Context, serverName, version string) (*models.AssetResponse, error) {
	return s.findAssetBackedAssetInStore(ctx, s.assets, serverName, version)
}

func (s *registry) findAssetBackedAssetInStore(ctx context.Context, assets database.AssetStore, serverName, version string) (*models.AssetResponse, error) {
	if assets == nil {
		return nil, database.ErrNotFound
	}
	trimmedVersion := strings.TrimSpace(version)
	if trimmedVersion == "" || strings.EqualFold(trimmedVersion, "latest") {
		asset, err := assets.GetAsset(ctx, serverName)
		if err == nil {
			if asset.Asset.Category == models.AssetCategoryMCP {
				return asset, nil
			}
		} else if !errors.Is(err, database.ErrNotFound) {
			return nil, err
		}
		return s.findAssetByExactNameInStore(ctx, assets, serverName, "")
	}

	asset, err := assets.GetAssetVersion(ctx, serverName, trimmedVersion)
	if err == nil {
		if asset.Asset.Category == models.AssetCategoryMCP {
			return asset, nil
		}
	} else if !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}
	return s.findAssetByExactNameInStore(ctx, assets, serverName, trimmedVersion)
}

func (s *registry) findAssetByExactNameInStore(ctx context.Context, assets database.AssetStore, serverName, version string) (*models.AssetResponse, error) {
	if assets == nil {
		return nil, database.ErrNotFound
	}
	category := models.AssetCategoryMCP
	search := serverName
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
			if asset == nil || asset.Asset.Category != models.AssetCategoryMCP {
				continue
			}
			if asset.Asset.ID != serverName && asset.Asset.Name != serverName {
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
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		leftPublishedAt := time.Time{}
		rightPublishedAt := time.Time{}
		if left.Meta.Official != nil {
			leftPublishedAt = left.Meta.Official.PublishedAt
		}
		if right.Meta.Official != nil {
			rightPublishedAt = right.Meta.Official.PublishedAt
		}
		return versionutil.CompareVersions(left.Asset.Version, right.Asset.Version, leftPublishedAt, rightPublishedAt) > 0
	})
	return candidates[0], nil
}

func (s *registry) currentServerReadmeBody(ctx context.Context, servers database.ServerStore, serverName, version string) string {
	if servers == nil {
		return ""
	}
	readme, err := servers.GetServerReadme(ctx, serverName, version)
	if err != nil || readme == nil {
		return ""
	}
	return string(readme.Content)
}

func (s *registry) updateAssetBackedServerInStore(ctx context.Context, assets database.AssetStore, serverName, version string, req *apiv0.ServerJSON, newStatus *string) (*apiv0.ServerResponse, error) {
	currentAsset, err := s.findAssetBackedAssetInStore(ctx, assets, serverName, version)
	if err != nil {
		return nil, err
	}
	current, err := models.ServerResponseFromAssetResponse(currentAsset)
	if err != nil {
		return nil, fmt.Errorf("convert current asset %s@%s to server response: %w", currentAsset.Asset.ID, currentAsset.Asset.Version, err)
	}
	updated := *req
	response := &apiv0.ServerResponse{Server: updated}
	assetMeta := currentAsset.Meta.Official
	if assetMeta == nil {
		assetMeta = &models.AssetRegistryExtensions{}
	}
	meta := &apiv0.RegistryExtensions{
		Status:      model.Status(assetMeta.Status),
		PublishedAt: assetMeta.PublishedAt,
		UpdatedAt:   time.Now(),
		IsLatest:    assetMeta.IsLatest,
	}
	if meta.Status == "" {
		meta.Status = model.StatusActive
	}
	if meta.PublishedAt.IsZero() {
		meta.PublishedAt = meta.UpdatedAt
	}
	if newStatus != nil {
		meta.Status = model.Status(*newStatus)
	}
	response.Meta.Official = meta
	readme := strings.TrimSpace(currentAsset.Asset.SourceSkill.Body)
	if readme == "" {
		readme = strings.TrimSpace(currentAsset.Asset.Manifest.SourceSkill.Body)
	}
	if current.Server.Title != "" && response.Server.Title == "" {
		response.Server.Title = current.Server.Title
	}
	if current.Server.Schema != "" && response.Server.Schema == "" {
		response.Server.Schema = current.Server.Schema
	}
	if response.Server.Name == "" {
		response.Server.Name = current.Server.Name
	}
	if response.Server.Version == "" {
		response.Server.Version = current.Server.Version
	}
	if response.Server.Description == "" {
		response.Server.Description = current.Server.Description
	}
	if err := s.mirrorAssetInStore(ctx, assets, response, readme); err != nil {
		return nil, err
	}
	asset, err := assets.GetAssetVersion(ctx, currentAsset.Asset.ID, currentAsset.Asset.Version)
	if err != nil {
		return nil, err
	}
	return models.ServerResponseFromAssetResponse(asset)
}

func (s *registry) updateAssetReadmeInStore(ctx context.Context, assets database.AssetStore, serverName, version, readme string) error {
	asset, err := s.findAssetBackedAssetInStore(ctx, assets, serverName, version)
	if err != nil {
		return err
	}
	updatedAsset := asset.Asset
	updatedAsset.SourceSkill.Body = readme
	updatedAsset.Manifest.SourceSkill.Body = readme
	meta := asset.Meta.Official
	if meta == nil {
		meta = &models.AssetRegistryExtensions{}
	}
	normalizedMeta := *meta
	if normalizedMeta.PublishedAt.IsZero() {
		normalizedMeta.PublishedAt = time.Now()
	}
	normalizedMeta.UpdatedAt = time.Now()
	if normalizedMeta.Status == "" {
		normalizedMeta.Status = string(model.StatusActive)
	}
	_, err = assets.UpdateAsset(ctx, updatedAsset.ID, updatedAsset.Version, &updatedAsset, &normalizedMeta)
	if err != nil {
		return fmt.Errorf("update mirrored asset readme: %w", err)
	}
	return nil
}

func (s *registry) deleteMirroredAssetInStore(ctx context.Context, assets database.AssetStore, serverName, version string) error {
	asset, err := s.findAssetBackedAssetInStore(ctx, assets, serverName, version)
	if err != nil {
		return err
	}
	return assets.DeleteAsset(ctx, asset.Asset.ID, asset.Asset.Version)
}

func (s *registry) mirrorAssetInStore(ctx context.Context, assets database.AssetStore, serverResponse *apiv0.ServerResponse, readme string) error {
	if assets == nil || serverResponse == nil {
		return nil
	}
	assetResponse, err := models.AssetResponseFromServerResponse(serverResponse, readme)
	if err != nil {
		return fmt.Errorf("convert server response to asset response: %w", err)
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

// triggerServerEmbeddingRegeneration kicks off async embedding regeneration if
// embedding-on-publish is enabled. The server value is copied into the closure
// to avoid races with callers that may mutate the request after this returns.
func (s *registry) triggerServerEmbeddingRegeneration(req *apiv0.ServerJSON) {
	if !embeddingutil.EnabledOnPublish(s.cfg, s.embeddingsProvider) {
		return
	}
	serverCopy := *req
	go func() {
		bgCtx := context.Background()
		payload := embeddings.BuildServerEmbeddingPayload(&serverCopy)
		if strings.TrimSpace(payload) == "" {
			return
		}
		embedding, embErr := embeddings.GenerateSemanticEmbedding(bgCtx, s.embeddingsProvider, payload, s.cfg.Embeddings.Dimensions)
		if embErr != nil {
			s.logger.Warn("failed to generate embedding for server", "name", serverCopy.Name, "version", serverCopy.Version, "error", embErr)
			return
		}
		if embedding == nil {
			return
		}
		if storeErr := s.SetServerEmbedding(bgCtx, serverCopy.Name, serverCopy.Version, embedding); storeErr != nil {
			s.logger.Warn("failed to store embedding for server", "name", serverCopy.Name, "version", serverCopy.Version, "error", storeErr)
		}
	}()
}

func (s *registry) validateNoDuplicateRemoteURLs(ctx context.Context, servers database.ServerStore, serverDetail apiv0.ServerJSON) error {
	for _, remote := range serverDetail.Remotes {
		remoteURL := remote.URL
		filter := &database.ServerFilter{RemoteURL: &remoteURL}
		cursor := ""

		for {
			conflictingServers, nextCursor, err := servers.ListServers(ctx, filter, cursor, 1000)
			if err != nil {
				return fmt.Errorf("failed to check remote URL conflict: %w", err)
			}
			for _, conflictingServer := range conflictingServers {
				if conflictingServer.Server.Name != serverDetail.Name {
					return fmt.Errorf("remote URL %s is already used by server %s", remoteURL, conflictingServer.Server.Name)
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

func (s *registry) createServerInTransaction(ctx context.Context, servers database.ServerStore, assets database.AssetStore, req *apiv0.ServerJSON) (*apiv0.ServerResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request body is required", database.ErrInvalidInput)
	}
	if err := validators.ValidatePublishRequest(ctx, *req, s.cfg); err != nil {
		return nil, err
	}

	publishTime := time.Now()
	serverJSON := *req

	if err := servers.AcquireServerCreateLock(ctx, serverJSON.Name); err != nil {
		return nil, err
	}
	if err := s.validateNoDuplicateRemoteURLs(ctx, servers, serverJSON); err != nil {
		return nil, err
	}

	versionCount, err := servers.CountServerVersions(ctx, serverJSON.Name)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}
	if versionCount >= maxVersionsPerServer {
		return nil, database.ErrMaxVersionsReached
	}

	versionExists, err := servers.CheckVersionExists(ctx, serverJSON.Name, serverJSON.Version)
	if err != nil {
		return nil, err
	}
	if versionExists {
		return nil, database.ErrInvalidVersion
	}

	currentLatest, err := servers.GetLatestServer(ctx, serverJSON.Name)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	isNewLatest := true
	if currentLatest != nil {
		var existingPublishedAt time.Time
		if currentLatest.Meta.Official != nil {
			existingPublishedAt = currentLatest.Meta.Official.PublishedAt
		}
		isNewLatest = versionutil.CompareVersions(serverJSON.Version, currentLatest.Server.Version, publishTime, existingPublishedAt) > 0
	}
	if isNewLatest && currentLatest != nil {
		if err := servers.UnmarkAsLatest(ctx, serverJSON.Name); err != nil {
			return nil, err
		}
	}

	officialMeta := &apiv0.RegistryExtensions{
		Status:      model.StatusActive,
		PublishedAt: publishTime,
		UpdatedAt:   publishTime,
		IsLatest:    isNewLatest,
	}
	result, err := servers.CreateServer(ctx, &serverJSON, officialMeta)
	if err != nil {
		return nil, err
	}
	if err := s.mirrorAssetInStore(ctx, assets, result, ""); err != nil {
		return nil, err
	}
	s.triggerServerEmbeddingRegeneration(&serverJSON)
	return result, nil
}

func (s *registry) updateServerInTransaction(ctx context.Context, servers database.ServerStore, assets database.AssetStore, serverName, version string, req *apiv0.ServerJSON, newStatus *string) (*apiv0.ServerResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request body is required", database.ErrInvalidInput)
	}
	currentServer, err := servers.GetServerVersion(ctx, serverName, version)
	if err != nil {
		if assets != nil && errors.Is(err, database.ErrNotFound) {
			return s.updateAssetBackedServerInStore(ctx, assets, serverName, version, req, newStatus)
		}
		return nil, err
	}

	currentlyDeleted := currentServer.Meta.Official != nil && currentServer.Meta.Official.Status == model.StatusDeleted
	beingDeleted := newStatus != nil && *newStatus == string(model.StatusDeleted)
	skipRegistryValidation := currentlyDeleted || beingDeleted
	if err := s.validateUpdateRequest(ctx, *req, skipRegistryValidation); err != nil {
		return nil, err
	}

	updatedServer := *req
	if err := s.validateNoDuplicateRemoteURLs(ctx, servers, updatedServer); err != nil {
		return nil, err
	}
	updatedServerResponse, err := servers.UpdateServer(ctx, serverName, version, &updatedServer)
	if err != nil {
		return nil, err
	}
	if newStatus != nil {
		updatedServerResponse, err = servers.SetServerStatus(ctx, serverName, version, *newStatus)
		if err != nil {
			return nil, err
		}
	}
	if err := s.mirrorAssetInStore(ctx, assets, updatedServerResponse, s.currentServerReadmeBody(ctx, servers, serverName, version)); err != nil {
		return nil, err
	}
	return updatedServerResponse, nil
}

func (s *registry) validateUpdateRequest(ctx context.Context, req apiv0.ServerJSON, skipRegistryValidation bool) error {
	if err := validators.ValidateServerJSON(&req); err != nil {
		return err
	}
	if skipRegistryValidation || s.cfg == nil || !s.cfg.EnableRegistryValidation {
		return nil
	}
	for idx, pkg := range req.Packages {
		if err := validators.ValidatePackage(ctx, pkg, req.Name); err != nil {
			return fmt.Errorf("registry validation failed for package %d (%s): %w", idx, pkg.Identifier, err)
		}
	}
	return nil
}

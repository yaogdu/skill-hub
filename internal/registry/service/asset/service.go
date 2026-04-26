package asset

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agentregistry-dev/agentregistry/internal/registry/service/internal/versionutil"
	skillsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/skill"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
	apiv0 "github.com/modelcontextprotocol/registry/pkg/api/v0"
	"github.com/modelcontextprotocol/registry/pkg/model"
)

const (
	scanPageSize        = 1000
	maxVersionsPerAsset = 10000
)

type Dependencies struct {
	StoreDB  database.Store
	Assets   database.AssetStore
	Skills   skillsvc.Registry
	Packages PackageStore
	Agents   database.AgentReader
	Prompts  database.PromptReader
	Servers  database.ServerStore
	Tx       database.Transactor
}

type Filter struct {
	UpdatedSince *time.Time
	Search       *string
	Version      *string
	IsLatest     *bool
	Category     *models.AssetCategory
}

type Registry interface {
	ListAssets(ctx context.Context, filter *Filter, cursor string, limit int) ([]*models.AssetResponse, string, error)
	GetAsset(ctx context.Context, assetID string) (*models.AssetResponse, error)
	GetAssetVersion(ctx context.Context, assetID, version string) (*models.AssetResponse, error)
	GetAssetVersions(ctx context.Context, assetID string) ([]*models.AssetResponse, error)
	PublishAsset(ctx context.Context, request *models.AssetPublishRequest) (*models.AssetResponse, error)
	UploadAssetPackage(ctx context.Context, assetID, version string, content []byte, contentType string) (*models.AssetPackageResponse, error)
	GetAssetPackage(ctx context.Context, assetID, version string) (*models.AssetPackageDownload, error)
}

type registry struct {
	assets   database.AssetStore
	skills   skillsvc.Registry
	packages PackageStore
	agents   database.AgentReader
	prompts  database.PromptReader
	servers  database.ServerStore
	tx       database.Transactor
}

var _ Registry = (*registry)(nil)

type assetStoreProvider interface {
	Assets() database.AssetStore
}

func New(deps Dependencies) Registry {
	if deps.Assets == nil && deps.StoreDB != nil {
		if provider, ok := deps.StoreDB.(assetStoreProvider); ok {
			deps.Assets = provider.Assets()
		}
	}
	if deps.Agents == nil && deps.StoreDB != nil {
		deps.Agents = deps.StoreDB.Agents()
	}
	if deps.Prompts == nil && deps.StoreDB != nil {
		deps.Prompts = deps.StoreDB.Prompts()
	}
	if deps.Servers == nil && deps.StoreDB != nil {
		deps.Servers = deps.StoreDB.Servers()
	}
	if deps.Tx == nil {
		deps.Tx = deps.StoreDB
	}
	return &registry{assets: deps.Assets, skills: deps.Skills, packages: deps.Packages, agents: deps.Agents, prompts: deps.Prompts, servers: deps.Servers, tx: deps.Tx}
}

func (registry *registry) ListAssets(ctx context.Context, filter *Filter, cursor string, limit int) ([]*models.AssetResponse, string, error) {
	if limit <= 0 {
		limit = 30
	}
	if registry.assets != nil {
		assets, nextCursor, err := registry.assets.ListAssets(ctx, toAssetStoreFilter(filter), cursor, limit)
		if err == nil && (len(assets) > 0 || !registry.hasCompatibilityReaders()) {
			return assets, nextCursor, nil
		}
		if err != nil && !registry.hasCompatibilityReaders() {
			return nil, "", err
		}
	}
	return registry.listCompatibilityAssets(ctx, filter, cursor, limit)
}

func (registry *registry) GetAsset(ctx context.Context, assetID string) (*models.AssetResponse, error) {
	if registry.assets != nil {
		asset, err := registry.assets.GetAsset(ctx, assetID)
		if err == nil || !registry.hasCompatibilityReaders() || !errors.Is(err, database.ErrNotFound) {
			return asset, err
		}
	}
	latest := true
	return registry.findCompatibilityAsset(ctx, assetID, &Filter{IsLatest: &latest})
}

func (registry *registry) GetAssetVersion(ctx context.Context, assetID, version string) (*models.AssetResponse, error) {
	if strings.TrimSpace(version) == "" || version == "latest" {
		return registry.GetAsset(ctx, assetID)
	}
	if registry.assets != nil {
		asset, err := registry.assets.GetAssetVersion(ctx, assetID, version)
		if err == nil || !registry.hasCompatibilityReaders() || !errors.Is(err, database.ErrNotFound) {
			return asset, err
		}
	}
	return registry.findCompatibilityAsset(ctx, assetID, &Filter{Version: &version})
}

func (registry *registry) PublishAsset(ctx context.Context, request *models.AssetPublishRequest) (*models.AssetResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("asset publish request is nil")
	}

	if registry.assets != nil {
		asset, err := request.ToAsset()
		if err != nil {
			return nil, fmt.Errorf("convert asset publish request: %w", err)
		}
		if registry.tx != nil {
			return database.InTransactionT(ctx, registry.tx, func(txCtx context.Context, scope database.Scope) (*models.AssetResponse, error) {
				store := registry.assets
				if provider, ok := scope.(assetStoreProvider); ok && provider.Assets() != nil {
					store = provider.Assets()
				}
				createdAsset, err := registry.publishAssetToStore(txCtx, store, asset)
				if err != nil {
					return nil, err
				}
				if registry.skills != nil {
					skillPayload, err := request.ToSkillJSON()
					if err != nil {
						return nil, fmt.Errorf("convert asset publish request: %w", err)
					}
					if err := registry.mirrorCompatibilitySkillInStore(txCtx, scope.Skills(), skillPayload, createdAsset.Meta.Official); err != nil {
						return nil, err
					}
				}
				return createdAsset, nil
			})
		}

		createdAsset, err := registry.publishAssetToStore(ctx, registry.assets, asset)
		if err != nil {
			return nil, err
		}
		if registry.skills != nil {
			skillPayload, err := request.ToSkillJSON()
			if err != nil {
				return nil, fmt.Errorf("convert asset publish request: %w", err)
			}
			if err := registry.mirrorCompatibilitySkillViaRegistry(ctx, skillPayload); err != nil {
				return nil, err
			}
		}
		return createdAsset, nil
	}

	if request.Manifest.Category == models.AssetCategoryMCP && registry.servers != nil {
		if registry.tx != nil {
			return database.InTransactionT(ctx, registry.tx, func(txCtx context.Context, scope database.Scope) (*models.AssetResponse, error) {
				return registry.publishCompatibilityServerToAsset(txCtx, scope.Servers(), request)
			})
		}
		return registry.publishCompatibilityServerToAsset(ctx, registry.servers, request)
	}

	if registry.skills == nil {
		return nil, fmt.Errorf("asset service requires asset, server, or skill registry")
	}

	skillPayload, err := request.ToSkillJSON()
	if err != nil {
		return nil, fmt.Errorf("convert asset publish request: %w", err)
	}
	skillResponse, err := registry.skills.PublishSkill(ctx, skillPayload)
	if err != nil {
		return nil, err
	}
	assetResponse, err := models.AssetResponseFromSkillResponse(skillResponse)
	if err != nil {
		return nil, fmt.Errorf("convert published skill response: %w", err)
	}
	return assetResponse, nil
}

func (registry *registry) UploadAssetPackage(ctx context.Context, assetID, version string, content []byte, contentType string) (*models.AssetPackageResponse, error) {
	if registry.packages == nil {
		return nil, fmt.Errorf("asset package store is not configured")
	}
	if strings.TrimSpace(assetID) == "" || strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("asset id and version are required")
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("asset package content is empty")
	}
	if err := validateUploadedAssetPackage(content, assetID, version); err != nil {
		return nil, err
	}
	uploadedAt := time.Now().UTC()
	pkg, err := registry.packages.Put(ctx, assetID, version, content, uploadedAt)
	if err != nil {
		return nil, err
	}
	if pkg.ContentType == "" {
		pkg.ContentType = normalizeAssetPackageContentType(contentType)
	}
	return &models.AssetPackageResponse{Package: *pkg}, nil
}

func (registry *registry) GetAssetPackage(ctx context.Context, assetID, version string) (*models.AssetPackageDownload, error) {
	if registry.packages == nil {
		return nil, fmt.Errorf("asset package store is not configured")
	}
	if strings.TrimSpace(assetID) == "" {
		return nil, fmt.Errorf("asset id is required")
	}
	if strings.TrimSpace(version) == "" || strings.EqualFold(strings.TrimSpace(version), "latest") {
		latest, err := registry.GetAsset(ctx, assetID)
		if err != nil {
			return nil, err
		}
		version = latest.Asset.Version
	}
	return registry.packages.Get(ctx, assetID, version)
}

func (registry *registry) GetAssetVersions(ctx context.Context, assetID string) ([]*models.AssetResponse, error) {
	if registry.assets != nil {
		versions, err := registry.assets.GetAssetVersions(ctx, assetID)
		if err == nil || !registry.hasCompatibilityReaders() || !errors.Is(err, database.ErrNotFound) {
			return versions, err
		}
	}

	assets, err := registry.scanCompatibilityAssets(ctx, nil)
	if err != nil {
		return nil, err
	}

	versions := make([]*models.AssetResponse, 0)
	for _, asset := range assets {
		if asset == nil || asset.Asset.ID != assetID {
			continue
		}
		versions = append(versions, asset)
	}
	if len(versions) == 0 {
		return nil, database.ErrNotFound
	}
	sort.SliceStable(versions, func(i, j int) bool {
		return versions[i].Asset.Version < versions[j].Asset.Version
	})
	return versions, nil
}

func validateUploadedAssetPackage(content []byte, assetID, version string) error {
	tempDir, err := os.MkdirTemp("", "agentregistry-asset-package-*")
	if err != nil {
		return fmt.Errorf("create asset package validation directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	if err := shubskills.ExtractPackageReader(bytes.NewReader(content), tempDir); err != nil {
		return fmt.Errorf("extract uploaded SHUB package: %w", err)
	}
	asset, err := shubskills.LoadAssetDir(tempDir)
	if err != nil {
		return fmt.Errorf("load uploaded SHUB package: %w", err)
	}
	if asset.ID != assetID {
		return fmt.Errorf("uploaded SHUB package asset id %q does not match request %q", asset.ID, assetID)
	}
	if asset.Version != version {
		return fmt.Errorf("uploaded SHUB package version %q does not match request %q", asset.Version, version)
	}
	return nil
}

func normalizeAssetPackageContentType(contentType string) string {
	if strings.TrimSpace(contentType) == "" {
		return assetPackageContentType
	}
	return contentType
}

func (registry *registry) publishAssetToStore(ctx context.Context, store database.AssetStore, asset *models.Asset) (*models.AssetResponse, error) {
	if store == nil {
		return nil, fmt.Errorf("asset store is not configured")
	}
	publishTime := time.Now()

	versionCount, err := store.CountAssetVersions(ctx, asset.ID)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}
	if versionCount >= maxVersionsPerAsset {
		return nil, database.ErrMaxVersionsReached
	}

	exists, err := store.CheckAssetVersionExists(ctx, asset.ID, asset.Version)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, database.ErrInvalidVersion
	}

	currentLatest, err := store.GetLatestAsset(ctx, asset.ID)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	isNewLatest := true
	if currentLatest != nil {
		var existingPublishedAt time.Time
		if currentLatest.Meta.Official != nil {
			existingPublishedAt = currentLatest.Meta.Official.PublishedAt
		}
		if versionutil.CompareVersions(asset.Version, currentLatest.Asset.Version, publishTime, existingPublishedAt) <= 0 {
			isNewLatest = false
		}
	}

	if isNewLatest && currentLatest != nil {
		if err := store.UnmarkAssetAsLatest(ctx, asset.ID); err != nil {
			return nil, err
		}
	}

	officialMeta := &models.AssetRegistryExtensions{
		Status:      "active",
		PublishedAt: publishTime,
		UpdatedAt:   publishTime,
		IsLatest:    isNewLatest,
	}
	return store.CreateAsset(ctx, asset, officialMeta)
}

func (registry *registry) publishCompatibilityServerToAsset(ctx context.Context, servers database.ServerStore, request *models.AssetPublishRequest) (*models.AssetResponse, error) {
	serverPayload, readme, err := request.ToServerJSON()
	if err != nil {
		return nil, fmt.Errorf("convert asset publish request to server payload: %w", err)
	}
	createdServer, err := registry.publishCompatibilityServerInStore(ctx, servers, serverPayload, readme)
	if err != nil {
		return nil, err
	}
	assetResponse, err := models.AssetResponseFromServerResponse(createdServer, readme)
	if err != nil {
		return nil, fmt.Errorf("convert published server response: %w", err)
	}
	return assetResponse, nil
}

func (registry *registry) publishCompatibilityServerInStore(ctx context.Context, servers database.ServerStore, serverPayload *apiv0.ServerJSON, readme string) (*apiv0.ServerResponse, error) {
	if servers == nil {
		return nil, fmt.Errorf("server store is not configured")
	}
	if serverPayload == nil {
		return nil, fmt.Errorf("server payload is nil")
	}

	publishTime := time.Now()
	if err := servers.AcquireServerCreateLock(ctx, serverPayload.Name); err != nil {
		return nil, err
	}
	if err := registry.validateNoDuplicateServerRemoteURLs(ctx, servers, *serverPayload); err != nil {
		return nil, err
	}

	versionCount, err := servers.CountServerVersions(ctx, serverPayload.Name)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}
	if versionCount >= maxVersionsPerAsset {
		return nil, database.ErrMaxVersionsReached
	}

	exists, err := servers.CheckVersionExists(ctx, serverPayload.Name, serverPayload.Version)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, database.ErrInvalidVersion
	}

	currentLatest, err := servers.GetLatestServer(ctx, serverPayload.Name)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	isNewLatest := true
	if currentLatest != nil {
		var existingPublishedAt time.Time
		if currentLatest.Meta.Official != nil {
			existingPublishedAt = currentLatest.Meta.Official.PublishedAt
		}
		if versionutil.CompareVersions(serverPayload.Version, currentLatest.Server.Version, publishTime, existingPublishedAt) <= 0 {
			isNewLatest = false
		}
	}

	if isNewLatest && currentLatest != nil {
		if err := servers.UnmarkAsLatest(ctx, serverPayload.Name); err != nil {
			return nil, err
		}
	}

	officialMeta := &apiv0.RegistryExtensions{
		Status:      model.StatusActive,
		PublishedAt: publishTime,
		UpdatedAt:   publishTime,
		IsLatest:    isNewLatest,
	}
	created, err := servers.CreateServer(ctx, serverPayload, officialMeta)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(readme) != "" {
		if err := servers.UpsertServerReadme(ctx, &database.ServerReadme{
			ServerName:  serverPayload.Name,
			Version:     serverPayload.Version,
			Content:     []byte(readme),
			ContentType: "text/markdown",
			SizeBytes:   len(readme),
			FetchedAt:   publishTime,
		}); err != nil {
			return nil, fmt.Errorf("store compatibility server readme: %w", err)
		}
	}
	return created, nil
}

func (registry *registry) validateNoDuplicateServerRemoteURLs(ctx context.Context, servers database.ServerStore, server apiv0.ServerJSON) error {
	for _, remote := range server.Remotes {
		remoteURL := strings.TrimSpace(remote.URL)
		if remoteURL == "" {
			continue
		}
		filter := &database.ServerFilter{RemoteURL: &remoteURL}
		cursor := ""
		for {
			conflicts, nextCursor, err := servers.ListServers(ctx, filter, cursor, scanPageSize)
			if err != nil {
				return fmt.Errorf("check compatibility server remote URL conflict: %w", err)
			}
			for _, conflict := range conflicts {
				if conflict != nil && conflict.Server.Name != server.Name {
					return fmt.Errorf("remote URL %s is already used by server %s", remoteURL, conflict.Server.Name)
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

func (registry *registry) mirrorCompatibilitySkillViaRegistry(ctx context.Context, skillPayload *models.SkillJSON) error {
	if registry.skills == nil || skillPayload == nil {
		return nil
	}
	if _, err := registry.skills.ApplySkill(ctx, skillPayload); err != nil {
		return fmt.Errorf("mirror asset into compatibility skill registry: %w", err)
	}
	return nil
}

func (registry *registry) mirrorCompatibilitySkillInStore(ctx context.Context, store database.SkillStore, skillPayload *models.SkillJSON, assetMeta *models.AssetRegistryExtensions) error {
	if store == nil || skillPayload == nil || assetMeta == nil {
		return nil
	}

	exists, err := store.CheckSkillVersionExists(ctx, skillPayload.Name, skillPayload.Version)
	if err != nil {
		return fmt.Errorf("check compatibility skill version: %w", err)
	}
	if exists {
		if _, err := store.UpdateSkill(ctx, skillPayload.Name, skillPayload.Version, skillPayload); err != nil {
			return fmt.Errorf("update compatibility skill: %w", err)
		}
		return nil
	}
	if assetMeta.IsLatest {
		if err := store.UnmarkSkillAsLatest(ctx, skillPayload.Name); err != nil {
			return fmt.Errorf("unmark compatibility skill latest version: %w", err)
		}
	}
	if _, err := store.CreateSkill(ctx, skillPayload, &models.SkillRegistryExtensions{
		Status:      assetMeta.Status,
		PublishedAt: assetMeta.PublishedAt,
		UpdatedAt:   assetMeta.UpdatedAt,
		IsLatest:    assetMeta.IsLatest,
	}); err != nil {
		return fmt.Errorf("create compatibility skill: %w", err)
	}
	return nil
}

func (registry *registry) listCompatibilityAssets(ctx context.Context, filter *Filter, cursor string, limit int) ([]*models.AssetResponse, string, error) {
	assets, err := registry.scanCompatibilityAssets(ctx, filter)
	if err != nil {
		return nil, "", err
	}
	sortCompatibilityAssets(assets)
	return paginateCompatibilityAssets(assets, cursor, limit), nextCompatibilityCursor(assets, cursor, limit), nil
}

func (registry *registry) findCompatibilityAsset(ctx context.Context, assetID string, filter *Filter) (*models.AssetResponse, error) {
	assets, err := registry.scanCompatibilityAssets(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, asset := range assets {
		if asset == nil || asset.Asset.ID != assetID {
			continue
		}
		return asset, nil
	}
	return nil, database.ErrNotFound
}

func (registry *registry) scanCompatibilityAssets(ctx context.Context, filter *Filter) ([]*models.AssetResponse, error) {
	if !registry.hasCompatibilityReaders() {
		return nil, fmt.Errorf("asset service requires asset store or compatibility readers")
	}

	results := make([]*models.AssetResponse, 0)

	skillAssets, err := registry.scanSkillCompatibilityAssets(ctx, filter)
	if err != nil {
		return nil, err
	}
	results = append(results, skillAssets...)

	agentAssets, err := registry.scanAgentCompatibilityAssets(ctx, filter)
	if err != nil {
		return nil, err
	}
	results = append(results, agentAssets...)

	promptAssets, err := registry.scanPromptCompatibilityAssets(ctx, filter)
	if err != nil {
		return nil, err
	}
	results = append(results, promptAssets...)

	serverAssets, err := registry.scanServerCompatibilityAssets(ctx, filter)
	if err != nil {
		return nil, err
	}
	results = append(results, serverAssets...)

	return results, nil
}

func (registry *registry) scanSkillCompatibilityAssets(ctx context.Context, filter *Filter) ([]*models.AssetResponse, error) {
	if registry.skills == nil {
		return nil, nil
	}

	results := make([]*models.AssetResponse, 0)
	cursor := ""
	for {
		skills, nextCursor, err := registry.skills.ListSkills(ctx, toSkillFilter(filter), cursor, scanPageSize)
		if err != nil {
			return nil, err
		}
		for _, skill := range skills {
			asset, ok := convertSkillToAsset(skill)
			if !ok || !matchesFilter(asset, filter) {
				continue
			}
			results = append(results, asset)
		}
		if nextCursor == "" {
			return results, nil
		}
		cursor = nextCursor
	}
}

func (registry *registry) scanAgentCompatibilityAssets(ctx context.Context, filter *Filter) ([]*models.AssetResponse, error) {
	if registry.agents == nil {
		return nil, nil
	}
	if filter != nil && filter.Category != nil && *filter.Category != models.AssetCategoryAgent {
		return nil, nil
	}

	results := make([]*models.AssetResponse, 0)
	cursor := ""
	for {
		agents, nextCursor, err := registry.agents.ListAgents(ctx, toAgentFilter(filter), cursor, scanPageSize)
		if err != nil {
			return nil, err
		}
		for _, agent := range agents {
			asset, ok := convertAgentToAsset(agent)
			if !ok || !matchesFilter(asset, filter) {
				continue
			}
			results = append(results, asset)
		}
		if nextCursor == "" {
			return results, nil
		}
		cursor = nextCursor
	}
}

func (registry *registry) scanServerCompatibilityAssets(ctx context.Context, filter *Filter) ([]*models.AssetResponse, error) {
	if registry.servers == nil {
		return nil, nil
	}
	if filter != nil && filter.Category != nil && *filter.Category != models.AssetCategoryMCP {
		return nil, nil
	}

	results := make([]*models.AssetResponse, 0)
	cursor := ""
	for {
		servers, nextCursor, err := registry.servers.ListServers(ctx, toServerFilter(filter), cursor, scanPageSize)
		if err != nil {
			return nil, err
		}
		for _, server := range servers {
			asset, ok := registry.convertServerToAsset(ctx, server)
			if !ok || !matchesFilter(asset, filter) {
				continue
			}
			results = append(results, asset)
		}
		if nextCursor == "" {
			return results, nil
		}
		cursor = nextCursor
	}
}

func (registry *registry) scanPromptCompatibilityAssets(ctx context.Context, filter *Filter) ([]*models.AssetResponse, error) {
	if registry.prompts == nil {
		return nil, nil
	}
	if filter != nil && filter.Category != nil && *filter.Category != models.AssetCategoryPrompt {
		return nil, nil
	}

	results := make([]*models.AssetResponse, 0)
	cursor := ""
	for {
		prompts, nextCursor, err := registry.prompts.ListPrompts(ctx, toPromptFilter(filter), cursor, scanPageSize)
		if err != nil {
			return nil, err
		}
		for _, prompt := range prompts {
			asset, ok := convertPromptToAsset(prompt)
			if !ok || !matchesFilter(asset, filter) {
				continue
			}
			results = append(results, asset)
		}
		if nextCursor == "" {
			return results, nil
		}
		cursor = nextCursor
	}
}

func (registry *registry) hasCompatibilityReaders() bool {
	return registry.skills != nil || registry.agents != nil || registry.prompts != nil || registry.servers != nil
}

func toAssetStoreFilter(filter *Filter) *database.AssetFilter {
	if filter == nil {
		return &database.AssetFilter{}
	}
	assetFilter := &database.AssetFilter{}
	if filter.UpdatedSince != nil {
		assetFilter.UpdatedSince = filter.UpdatedSince
	}
	if filter.Version != nil {
		assetFilter.Version = filter.Version
	}
	if filter.IsLatest != nil {
		assetFilter.IsLatest = filter.IsLatest
	}
	if filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
		assetFilter.Search = filter.Search
	}
	if filter.Category != nil {
		assetFilter.Category = filter.Category
	}
	return assetFilter
}

func toSkillFilter(filter *Filter) *database.SkillFilter {
	if filter == nil {
		return &database.SkillFilter{}
	}
	skillFilter := &database.SkillFilter{}
	if filter.UpdatedSince != nil {
		skillFilter.UpdatedSince = filter.UpdatedSince
	}
	if filter.Version != nil {
		skillFilter.Version = filter.Version
	}
	if filter.IsLatest != nil {
		skillFilter.IsLatest = filter.IsLatest
	}
	if filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
		skillFilter.SubstringName = filter.Search
	}
	return skillFilter
}

func toAgentFilter(filter *Filter) *database.AgentFilter {
	if filter == nil {
		return &database.AgentFilter{}
	}
	agentFilter := &database.AgentFilter{}
	if filter.UpdatedSince != nil {
		agentFilter.UpdatedSince = filter.UpdatedSince
	}
	if filter.Version != nil {
		agentFilter.Version = filter.Version
	}
	if filter.IsLatest != nil {
		agentFilter.IsLatest = filter.IsLatest
	}
	if filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
		agentFilter.SubstringName = filter.Search
	}
	return agentFilter
}

func toServerFilter(filter *Filter) *database.ServerFilter {
	if filter == nil {
		return &database.ServerFilter{}
	}
	serverFilter := &database.ServerFilter{}
	if filter.UpdatedSince != nil {
		serverFilter.UpdatedSince = filter.UpdatedSince
	}
	if filter.Version != nil {
		serverFilter.Version = filter.Version
	}
	if filter.IsLatest != nil {
		serverFilter.IsLatest = filter.IsLatest
	}
	if filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
		serverFilter.SubstringName = filter.Search
	}
	return serverFilter
}

func toPromptFilter(filter *Filter) *database.PromptFilter {
	if filter == nil {
		return &database.PromptFilter{}
	}
	promptFilter := &database.PromptFilter{}
	if filter.UpdatedSince != nil {
		promptFilter.UpdatedSince = filter.UpdatedSince
	}
	if filter.Version != nil {
		promptFilter.Version = filter.Version
	}
	if filter.IsLatest != nil {
		promptFilter.IsLatest = filter.IsLatest
	}
	if filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
		promptFilter.SubstringName = filter.Search
	}
	return promptFilter
}

func convertSkillToAsset(skill *models.SkillResponse) (*models.AssetResponse, bool) {
	asset, err := models.AssetResponseFromSkillResponse(skill)
	if err != nil {
		return nil, false
	}
	return asset, true
}

func convertAgentToAsset(agent *models.AgentResponse) (*models.AssetResponse, bool) {
	asset, err := models.AssetResponseFromAgentResponse(agent)
	if err != nil {
		return nil, false
	}
	return asset, true
}

func convertPromptToAsset(prompt *models.PromptResponse) (*models.AssetResponse, bool) {
	asset, err := models.AssetResponseFromPromptResponse(prompt)
	if err != nil {
		return nil, false
	}
	return asset, true
}

func (registry *registry) convertServerToAsset(ctx context.Context, server *apiv0.ServerResponse) (*models.AssetResponse, bool) {
	if server == nil {
		return nil, false
	}
	readme := registry.compatibilityServerReadme(ctx, server)
	asset, err := models.AssetResponseFromServerResponse(server, readme)
	if err != nil {
		return nil, false
	}
	return asset, true
}

func (registry *registry) compatibilityServerReadme(ctx context.Context, server *apiv0.ServerResponse) string {
	if registry.servers == nil || server == nil {
		return ""
	}
	var (
		readme *database.ServerReadme
		err    error
	)
	if server.Meta.Official != nil && server.Meta.Official.IsLatest {
		readme, err = registry.servers.GetLatestServerReadme(ctx, server.Server.Name)
	} else {
		readme, err = registry.servers.GetServerReadme(ctx, server.Server.Name, server.Server.Version)
	}
	if err != nil || readme == nil {
		return ""
	}
	return strings.TrimSpace(string(readme.Content))
}

func matchesFilter(asset *models.AssetResponse, filter *Filter) bool {
	if asset == nil {
		return false
	}
	if filter == nil {
		return true
	}
	if filter.Category != nil && asset.Asset.Category != *filter.Category {
		return false
	}
	if filter.Search != nil {
		needle := strings.ToLower(strings.TrimSpace(*filter.Search))
		if needle != "" &&
			!strings.Contains(strings.ToLower(asset.Asset.ID), needle) &&
			!strings.Contains(strings.ToLower(asset.Asset.Name), needle) &&
			!strings.Contains(strings.ToLower(asset.Asset.Description), needle) {
			return false
		}
	}
	if filter.Version != nil && strings.TrimSpace(*filter.Version) != "" && asset.Asset.Version != *filter.Version {
		return false
	}
	if filter.IsLatest != nil {
		if asset.Meta.Official == nil || asset.Meta.Official.IsLatest != *filter.IsLatest {
			return false
		}
	}
	if filter.UpdatedSince != nil {
		if asset.Meta.Official == nil || asset.Meta.Official.UpdatedAt.Before(*filter.UpdatedSince) {
			return false
		}
	}
	return true
}

func sortCompatibilityAssets(assets []*models.AssetResponse) {
	sort.SliceStable(assets, func(i, j int) bool {
		left := assets[i]
		right := assets[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		if left.Asset.ID != right.Asset.ID {
			return left.Asset.ID < right.Asset.ID
		}
		return left.Asset.Version < right.Asset.Version
	})
}

func paginateCompatibilityAssets(assets []*models.AssetResponse, cursor string, limit int) []*models.AssetResponse {
	if limit <= 0 {
		limit = 30
	}
	start := 0
	if cursor != "" {
		for start < len(assets) && compareAssetWithCursor(assets[start], cursor) <= 0 {
			start++
		}
	}
	if start >= len(assets) {
		return []*models.AssetResponse{}
	}
	end := min(start+limit, len(assets))
	return assets[start:end]
}

func nextCompatibilityCursor(assets []*models.AssetResponse, cursor string, limit int) string {
	page := paginateCompatibilityAssets(assets, cursor, limit)
	if len(page) == 0 {
		return ""
	}
	start := 0
	if cursor != "" {
		for start < len(assets) && compareAssetWithCursor(assets[start], cursor) <= 0 {
			start++
		}
	}
	if start+len(page) >= len(assets) {
		return ""
	}
	last := page[len(page)-1]
	return last.Asset.ID + ":" + last.Asset.Version
}

func compareAssetWithCursor(asset *models.AssetResponse, cursor string) int {
	if asset == nil {
		return -1
	}
	parts := strings.SplitN(cursor, ":", 2)
	cursorID := cursor
	cursorVersion := ""
	if len(parts) == 2 {
		cursorID = parts[0]
		cursorVersion = parts[1]
	}
	if cmp := strings.Compare(asset.Asset.ID, cursorID); cmp != 0 {
		return cmp
	}
	return strings.Compare(asset.Asset.Version, cursorVersion)
}

package prompt

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agentregistry-dev/agentregistry/internal/registry/service/internal/versionutil"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/modelcontextprotocol/registry/pkg/model"
)

const (
	maxVersionsPerPrompt  = 10000
	assetFallbackPageSize = 100
)

type Dependencies struct {
	StoreDB database.Store
	Prompts database.PromptStore
	Assets  database.AssetStore
	Tx      database.Transactor
}

type Registry interface {
	database.PromptReader
	PublishPrompt(ctx context.Context, req *models.PromptJSON) (*models.PromptResponse, error)
	ApplyPrompt(ctx context.Context, req *models.PromptJSON) (*models.PromptResponse, error)
	DeletePrompt(ctx context.Context, promptName, version string) error
}

type registry struct {
	database.PromptStore
	assets database.AssetStore
	tx     database.Transactor
}

var _ Registry = (*registry)(nil)

type assetStoreProvider interface {
	Assets() database.AssetStore
}

func New(deps Dependencies) Registry {
	if deps.Prompts == nil && deps.StoreDB != nil {
		deps.Prompts = deps.StoreDB.Prompts()
	}
	if deps.Assets == nil && deps.StoreDB != nil {
		if provider, ok := deps.StoreDB.(assetStoreProvider); ok {
			deps.Assets = provider.Assets()
		}
	}
	if deps.Tx == nil {
		deps.Tx = deps.StoreDB
	}

	return &registry{
		PromptStore: deps.Prompts,
		assets:      deps.Assets,
		tx:          deps.Tx,
	}
}

func (s *registry) ListPrompts(ctx context.Context, filter *database.PromptFilter, cursor string, limit int) ([]*models.PromptResponse, string, error) {
	if limit <= 0 {
		limit = 30
	}
	if s.PromptStore != nil {
		prompts, nextCursor, err := s.PromptStore.ListPrompts(ctx, filter, cursor, limit)
		if err == nil && (len(prompts) > 0 || s.assets == nil) {
			return prompts, nextCursor, nil
		}
		if err != nil && s.assets == nil {
			return nil, "", err
		}
	}
	if s.assets == nil {
		return []*models.PromptResponse{}, "", nil
	}
	return s.listAssetBackedPrompts(ctx, filter, cursor, limit)
}

func (s *registry) GetPrompt(ctx context.Context, promptName string) (*models.PromptResponse, error) {
	if s.PromptStore != nil {
		prompt, err := s.PromptStore.GetPrompt(ctx, promptName)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return prompt, err
		}
	}
	return s.findAssetBackedPrompt(ctx, promptName, "latest")
}

func (s *registry) GetPromptVersion(ctx context.Context, promptName, version string) (*models.PromptResponse, error) {
	if strings.TrimSpace(version) == "" || strings.EqualFold(version, "latest") {
		return s.GetPrompt(ctx, promptName)
	}
	if s.PromptStore != nil {
		prompt, err := s.PromptStore.GetPromptVersion(ctx, promptName, version)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return prompt, err
		}
	}
	return s.findAssetBackedPrompt(ctx, promptName, version)
}

func (s *registry) GetPromptVersions(ctx context.Context, promptName string) ([]*models.PromptResponse, error) {
	if s.PromptStore != nil {
		prompts, err := s.PromptStore.GetPromptVersions(ctx, promptName)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return prompts, err
		}
	}
	if s.assets == nil {
		return nil, database.ErrNotFound
	}

	selected, err := s.findAssetBackedAsset(ctx, promptName, "latest")
	if err != nil {
		return nil, err
	}
	versions, err := s.assets.GetAssetVersions(ctx, selected.Asset.ID)
	if err != nil {
		return nil, err
	}

	prompts := make([]*models.PromptResponse, 0, len(versions))
	for _, versionedAsset := range versions {
		prompt, err := models.PromptResponseFromAssetResponse(versionedAsset)
		if err != nil {
			return nil, fmt.Errorf("convert asset %s@%s to prompt response: %w", versionedAsset.Asset.ID, versionedAsset.Asset.Version, err)
		}
		prompts = append(prompts, prompt)
	}
	sortPromptResponsesByRecency(prompts)
	return prompts, nil
}

func (s *registry) PublishPrompt(ctx context.Context, req *models.PromptJSON) (*models.PromptResponse, error) {
	return database.InTransactionT(ctx, s.tx, func(txCtx context.Context, scope database.Scope) (*models.PromptResponse, error) {
		return s.createPromptInTransaction(txCtx, scope.Prompts(), s.assetStoreFromScope(scope), req)
	})
}

func (s *registry) ApplyPrompt(ctx context.Context, req *models.PromptJSON) (*models.PromptResponse, error) {
	if req == nil || req.Name == "" || req.Version == "" {
		return nil, fmt.Errorf("invalid prompt payload: name and version are required")
	}
	return database.InTransactionT(ctx, s.tx, func(txCtx context.Context, scope database.Scope) (*models.PromptResponse, error) {
		return s.applyPromptInTransaction(txCtx, scope.Prompts(), s.assetStoreFromScope(scope), req)
	})
}

func (s *registry) applyPromptInTransaction(ctx context.Context, prompts database.PromptStore, assets database.AssetStore, req *models.PromptJSON) (*models.PromptResponse, error) {
	exists, err := prompts.CheckPromptVersionExists(ctx, req.Name, req.Version)
	if err != nil {
		return nil, err
	}
	if exists {
		updatedPrompt, err := prompts.UpdatePrompt(ctx, req.Name, req.Version, req)
		if err != nil {
			return nil, err
		}
		if err := s.mirrorAssetInStore(ctx, assets, updatedPrompt); err != nil {
			return nil, err
		}
		return updatedPrompt, nil
	}
	return s.createPromptInTransaction(ctx, prompts, assets, req)
}

func (s *registry) DeletePrompt(ctx context.Context, promptName, version string) error {
	return database.InTransaction(ctx, s.tx, func(txCtx context.Context, scope database.Scope) error {
		prompts := scope.Prompts()
		assets := s.assetStoreFromScope(scope)

		promptExisted := false
		if assets != nil {
			_, err := prompts.GetPromptVersion(txCtx, promptName, version)
			switch {
			case err == nil:
				promptExisted = true
			case errors.Is(err, database.ErrNotFound), errors.Is(err, auth.ErrForbidden), errors.Is(err, auth.ErrUnauthenticated):
			default:
				return err
			}
		}

		promptErr := prompts.DeletePrompt(txCtx, promptName, version)
		if promptErr != nil && !errors.Is(promptErr, database.ErrNotFound) {
			return promptErr
		}

		assetErr := database.ErrNotFound
		if assets != nil {
			switch {
			case promptExisted:
				assetErr = assets.DeleteAsset(txCtx, promptName, version)
			case errors.Is(promptErr, database.ErrNotFound):
				assetErr = s.deleteMirroredAssetInStore(txCtx, assets, promptName, version)
			}
			if assetErr != nil && !errors.Is(assetErr, database.ErrNotFound) {
				return assetErr
			}
		}

		if promptErr == nil || assetErr == nil {
			return nil
		}
		return database.ErrNotFound
	})
}

func (s *registry) assetStoreFromScope(scope database.Scope) database.AssetStore {
	store := s.assets
	if provider, ok := scope.(assetStoreProvider); ok && provider.Assets() != nil {
		store = provider.Assets()
	}
	return store
}

func (s *registry) createPromptInTransaction(ctx context.Context, prompts database.PromptStore, assets database.AssetStore, req *models.PromptJSON) (*models.PromptResponse, error) {
	if req == nil || req.Name == "" || req.Version == "" {
		return nil, fmt.Errorf("invalid prompt payload: name and version are required")
	}

	publishTime := time.Now()
	promptJSON := *req

	versionCount, err := prompts.CountPromptVersions(ctx, promptJSON.Name)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}
	if versionCount >= maxVersionsPerPrompt {
		return nil, database.ErrMaxVersionsReached
	}

	exists, err := prompts.CheckPromptVersionExists(ctx, promptJSON.Name, promptJSON.Version)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, database.ErrInvalidVersion
	}

	currentLatest, err := prompts.GetLatestPrompt(ctx, promptJSON.Name)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	isNewLatest := true
	if currentLatest != nil {
		var existingPublishedAt time.Time
		if currentLatest.Meta.Official != nil {
			existingPublishedAt = currentLatest.Meta.Official.PublishedAt
		}
		if versionutil.CompareVersions(promptJSON.Version, currentLatest.Prompt.Version, publishTime, existingPublishedAt) <= 0 {
			isNewLatest = false
		}
	}

	if isNewLatest && currentLatest != nil {
		if err := prompts.UnmarkPromptAsLatest(ctx, promptJSON.Name); err != nil {
			return nil, err
		}
	}

	officialMeta := &models.PromptRegistryExtensions{
		Status:      string(model.StatusActive),
		PublishedAt: publishTime,
		UpdatedAt:   publishTime,
		IsLatest:    isNewLatest,
	}

	createdPrompt, err := prompts.CreatePrompt(ctx, &promptJSON, officialMeta)
	if err != nil {
		return nil, err
	}
	if err := s.mirrorAssetInStore(ctx, assets, createdPrompt); err != nil {
		return nil, err
	}
	return createdPrompt, nil
}

func toAssetFallbackFilter(filter *database.PromptFilter) *database.AssetFilter {
	category := models.AssetCategoryPrompt
	assetFilter := &database.AssetFilter{Category: &category}
	if filter == nil {
		return assetFilter
	}
	assetFilter.UpdatedSince = filter.UpdatedSince
	assetFilter.Version = filter.Version
	assetFilter.IsLatest = filter.IsLatest
	if filter.SubstringName != nil {
		assetFilter.Search = filter.SubstringName
	} else if filter.Name != nil {
		assetFilter.Search = filter.Name
	}
	return assetFilter
}

func matchesPromptFallbackFilter(asset *models.AssetResponse, filter *database.PromptFilter) bool {
	if asset == nil || asset.Asset.Category != models.AssetCategoryPrompt {
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

func (s *registry) listAssetBackedPrompts(ctx context.Context, filter *database.PromptFilter, cursor string, limit int) ([]*models.PromptResponse, string, error) {
	pageSize := max(limit, assetFallbackPageSize)

	assetFilter := toAssetFallbackFilter(filter)
	collected := make([]*models.PromptResponse, 0, limit)
	currentCursor := cursor

	for {
		assets, nextCursor, err := s.assets.ListAssets(ctx, assetFilter, currentCursor, pageSize)
		if err != nil {
			return nil, "", err
		}
		for index, asset := range assets {
			if !matchesPromptFallbackFilter(asset, filter) {
				continue
			}
			prompt, err := models.PromptResponseFromAssetResponse(asset)
			if err != nil {
				return nil, "", fmt.Errorf("convert asset %s@%s to prompt response: %w", asset.Asset.ID, asset.Asset.Version, err)
			}
			collected = append(collected, prompt)
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

func (s *registry) findAssetBackedPrompt(ctx context.Context, promptName, version string) (*models.PromptResponse, error) {
	asset, err := s.findAssetBackedAsset(ctx, promptName, version)
	if err != nil {
		return nil, err
	}
	prompt, err := models.PromptResponseFromAssetResponse(asset)
	if err != nil {
		return nil, fmt.Errorf("convert asset %s@%s to prompt response: %w", asset.Asset.ID, asset.Asset.Version, err)
	}
	return prompt, nil
}

func (s *registry) findAssetBackedAsset(ctx context.Context, promptName, version string) (*models.AssetResponse, error) {
	return s.findAssetBackedAssetInStore(ctx, s.assets, promptName, version)
}

func (s *registry) findAssetBackedAssetInStore(ctx context.Context, assets database.AssetStore, promptName, version string) (*models.AssetResponse, error) {
	if assets == nil {
		return nil, database.ErrNotFound
	}
	trimmedVersion := strings.TrimSpace(version)
	if trimmedVersion == "" || strings.EqualFold(trimmedVersion, "latest") {
		asset, err := assets.GetAsset(ctx, promptName)
		if err == nil && asset.Asset.Category == models.AssetCategoryPrompt {
			return asset, nil
		}
		if err == nil {
			return nil, database.ErrNotFound
		}
		if !errors.Is(err, database.ErrNotFound) {
			return nil, err
		}
		return s.findAssetByExactNameInStore(ctx, assets, promptName, "")
	}

	asset, err := assets.GetAssetVersion(ctx, promptName, trimmedVersion)
	if err == nil && asset.Asset.Category == models.AssetCategoryPrompt {
		return asset, nil
	}
	if err == nil {
		return nil, database.ErrNotFound
	}
	if !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}
	return s.findAssetByExactNameInStore(ctx, assets, promptName, trimmedVersion)
}

func (s *registry) findAssetByExactNameInStore(ctx context.Context, assets database.AssetStore, promptName, version string) (*models.AssetResponse, error) {
	if assets == nil {
		return nil, database.ErrNotFound
	}
	category := models.AssetCategoryPrompt
	search := promptName
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
			if asset == nil || asset.Asset.Name != promptName || asset.Asset.Category != models.AssetCategoryPrompt {
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

func sortPromptResponsesByRecency(prompts []*models.PromptResponse) {
	sort.SliceStable(prompts, func(i, j int) bool {
		left := prompts[i]
		right := prompts[j]

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
		if cmp := versionutil.CompareVersions(left.Prompt.Version, right.Prompt.Version, leftPublishedAt, rightPublishedAt); cmp != 0 {
			return cmp > 0
		}
		if !leftPublishedAt.Equal(rightPublishedAt) {
			return leftPublishedAt.After(rightPublishedAt)
		}
		return left.Prompt.Name < right.Prompt.Name
	})
}

func (s *registry) deleteMirroredAssetInStore(ctx context.Context, assets database.AssetStore, promptName, version string) error {
	if assets == nil {
		return database.ErrNotFound
	}
	asset, err := s.findAssetBackedAssetInStore(ctx, assets, promptName, version)
	if err != nil {
		return err
	}
	return assets.DeleteAsset(ctx, asset.Asset.ID, asset.Asset.Version)
}

func (s *registry) mirrorAssetInStore(ctx context.Context, assets database.AssetStore, promptResponse *models.PromptResponse) error {
	if assets == nil || promptResponse == nil {
		return nil
	}

	assetResponse, err := models.AssetResponseFromPromptResponse(promptResponse)
	if err != nil {
		return fmt.Errorf("convert prompt response to asset response: %w", err)
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

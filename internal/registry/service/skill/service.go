package skill

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
	maxVersionsPerSkill   = 10000
	assetFallbackPageSize = 100
)

type Dependencies struct {
	StoreDB database.Store
	Skills  database.SkillStore
	Assets  database.AssetStore
	Tx      database.Transactor
}

type Registry interface {
	database.SkillReader
	PublishSkill(ctx context.Context, req *models.SkillJSON) (*models.SkillResponse, error)
	ApplySkill(ctx context.Context, req *models.SkillJSON) (*models.SkillResponse, error)
	DeleteSkill(ctx context.Context, skillName, version string) error
}

type registry struct {
	database.SkillStore
	assets database.AssetStore
	tx     database.Transactor
}

var _ Registry = (*registry)(nil)

type assetStoreProvider interface {
	Assets() database.AssetStore
}

func New(deps Dependencies) Registry {
	if deps.Skills == nil && deps.StoreDB != nil {
		deps.Skills = deps.StoreDB.Skills()
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
		SkillStore: deps.Skills,
		assets:     deps.Assets,
		tx:         deps.Tx,
	}
}

func (s *registry) ListSkills(ctx context.Context, filter *database.SkillFilter, cursor string, limit int) ([]*models.SkillResponse, string, error) {
	if limit <= 0 {
		limit = 30
	}
	if s.SkillStore != nil {
		skills, nextCursor, err := s.SkillStore.ListSkills(ctx, filter, cursor, limit)
		if err == nil && (len(skills) > 0 || s.assets == nil || !canFallbackToAssets(filter)) {
			return skills, nextCursor, nil
		}
		if err != nil && (s.assets == nil || !canFallbackToAssets(filter)) {
			return nil, "", err
		}
	}
	if s.assets == nil || !canFallbackToAssets(filter) {
		return []*models.SkillResponse{}, "", nil
	}
	return s.listAssetBackedSkills(ctx, filter, cursor, limit)
}

func (s *registry) GetSkill(ctx context.Context, skillName string) (*models.SkillResponse, error) {
	if s.SkillStore != nil {
		skill, err := s.SkillStore.GetSkill(ctx, skillName)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return skill, err
		}
	}
	return s.findAssetBackedSkill(ctx, skillName, "latest")
}

func (s *registry) GetSkillVersion(ctx context.Context, skillName, version string) (*models.SkillResponse, error) {
	if strings.TrimSpace(version) == "" || strings.EqualFold(version, "latest") {
		return s.GetSkill(ctx, skillName)
	}
	if s.SkillStore != nil {
		skill, err := s.SkillStore.GetSkillVersion(ctx, skillName, version)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return skill, err
		}
	}
	return s.findAssetBackedSkill(ctx, skillName, version)
}

func (s *registry) GetSkillVersions(ctx context.Context, skillName string) ([]*models.SkillResponse, error) {
	if s.SkillStore != nil {
		skills, err := s.SkillStore.GetSkillVersions(ctx, skillName)
		if err == nil || s.assets == nil || !errors.Is(err, database.ErrNotFound) {
			return skills, err
		}
	}
	if s.assets == nil {
		return nil, database.ErrNotFound
	}

	selected, err := s.findAssetBackedAsset(ctx, skillName, "latest")
	if err != nil {
		return nil, err
	}

	versions, err := s.assets.GetAssetVersions(ctx, selected.Asset.ID)
	if err != nil {
		return nil, err
	}

	skills := make([]*models.SkillResponse, 0, len(versions))
	for _, versionedAsset := range versions {
		skill, err := models.SkillResponseFromAssetResponse(versionedAsset)
		if err != nil {
			return nil, fmt.Errorf("convert asset %s@%s to skill response: %w", versionedAsset.Asset.ID, versionedAsset.Asset.Version, err)
		}
		skills = append(skills, skill)
	}
	sortSkillResponsesByRecency(skills)
	return skills, nil
}

func (s *registry) PublishSkill(ctx context.Context, req *models.SkillJSON) (*models.SkillResponse, error) {
	return database.InTransactionT(ctx, s.tx, func(txCtx context.Context, scope database.Scope) (*models.SkillResponse, error) {
		return s.createSkillInTransaction(txCtx, scope.Skills(), s.assetStoreFromScope(scope), req)
	})
}

func (s *registry) ApplySkill(ctx context.Context, req *models.SkillJSON) (*models.SkillResponse, error) {
	if req == nil || req.Name == "" || req.Version == "" {
		return nil, fmt.Errorf("invalid skill payload: name and version are required")
	}
	return database.InTransactionT(ctx, s.tx, func(txCtx context.Context, scope database.Scope) (*models.SkillResponse, error) {
		return s.applySkillInTransaction(txCtx, scope.Skills(), s.assetStoreFromScope(scope), req)
	})
}

func (s *registry) applySkillInTransaction(ctx context.Context, skills database.SkillStore, assets database.AssetStore, req *models.SkillJSON) (*models.SkillResponse, error) {
	exists, err := skills.CheckSkillVersionExists(ctx, req.Name, req.Version)
	if err != nil {
		return nil, err
	}
	if exists {
		if err := s.validateNoDuplicateRemoteURLs(ctx, skills, *req); err != nil {
			return nil, err
		}
		updatedSkill, err := skills.UpdateSkill(ctx, req.Name, req.Version, req)
		if err != nil {
			return nil, err
		}
		if err := s.mirrorAssetInStore(ctx, assets, updatedSkill); err != nil {
			return nil, err
		}
		return updatedSkill, nil
	}
	return s.createSkillInTransaction(ctx, skills, assets, req)
}

func (s *registry) DeleteSkill(ctx context.Context, skillName, version string) error {
	return database.InTransaction(ctx, s.tx, func(txCtx context.Context, scope database.Scope) error {
		skills := scope.Skills()
		assets := s.assetStoreFromScope(scope)

		deleteMirroredAsset := false
		if assets != nil {
			skillResponse, err := skills.GetSkillVersion(txCtx, skillName, version)
			switch {
			case err == nil:
				deleteMirroredAsset = skillResponse != nil && skillResponse.Skill.SHUB != nil && skillResponse.Skill.SHUB.Manifest != nil
			case errors.Is(err, database.ErrNotFound), errors.Is(err, auth.ErrForbidden), errors.Is(err, auth.ErrUnauthenticated):
				// Keep delete semantics permissive; asset-only fallback is handled below when the skill row is absent.
			default:
				return err
			}
		}

		skillErr := skills.DeleteSkill(txCtx, skillName, version)
		if skillErr != nil && !errors.Is(skillErr, database.ErrNotFound) {
			return skillErr
		}

		assetErr := database.ErrNotFound
		if assets != nil && (skillErr != nil || deleteMirroredAsset) {
			assetErr = s.deleteMirroredAssetInStore(txCtx, assets, skillName, version)
			if assetErr != nil && !errors.Is(assetErr, database.ErrNotFound) {
				return assetErr
			}
		}

		if skillErr == nil || assetErr == nil {
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

func canFallbackToAssets(filter *database.SkillFilter) bool {
	if filter == nil {
		return true
	}
	return filter.RemoteURL == nil
}

func toAssetFallbackFilter(filter *database.SkillFilter) *database.AssetFilter {
	if filter == nil {
		return nil
	}

	assetFilter := &database.AssetFilter{
		UpdatedSince: filter.UpdatedSince,
		Version:      filter.Version,
		IsLatest:     filter.IsLatest,
	}
	if filter.SubstringName != nil {
		assetFilter.Search = filter.SubstringName
	} else if filter.Name != nil {
		assetFilter.Search = filter.Name
	}
	return assetFilter
}

func (s *registry) listAssetBackedSkills(ctx context.Context, filter *database.SkillFilter, cursor string, limit int) ([]*models.SkillResponse, string, error) {
	pageSize := max(limit, assetFallbackPageSize)

	assetFilter := toAssetFallbackFilter(filter)
	collected := make([]*models.SkillResponse, 0, limit)
	currentCursor := cursor

	for {
		assets, nextCursor, err := s.assets.ListAssets(ctx, assetFilter, currentCursor, pageSize)
		if err != nil {
			return nil, "", err
		}
		for index, asset := range assets {
			if !matchesSkillFallbackFilter(asset, filter) {
				continue
			}
			skill, err := models.SkillResponseFromAssetResponse(asset)
			if err != nil {
				return nil, "", fmt.Errorf("convert asset %s@%s to skill response: %w", asset.Asset.ID, asset.Asset.Version, err)
			}
			collected = append(collected, skill)
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

func matchesSkillFallbackFilter(asset *models.AssetResponse, filter *database.SkillFilter) bool {
	if asset == nil {
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

func (s *registry) findAssetBackedSkill(ctx context.Context, skillName, version string) (*models.SkillResponse, error) {
	asset, err := s.findAssetBackedAsset(ctx, skillName, version)
	if err != nil {
		return nil, err
	}
	skill, err := models.SkillResponseFromAssetResponse(asset)
	if err != nil {
		return nil, fmt.Errorf("convert asset %s@%s to skill response: %w", asset.Asset.ID, asset.Asset.Version, err)
	}
	return skill, nil
}

func (s *registry) findAssetBackedAsset(ctx context.Context, skillName, version string) (*models.AssetResponse, error) {
	return s.findAssetBackedAssetInStore(ctx, s.assets, skillName, version)
}

func (s *registry) findAssetBackedAssetInStore(ctx context.Context, assets database.AssetStore, skillName, version string) (*models.AssetResponse, error) {
	if assets == nil {
		return nil, database.ErrNotFound
	}
	trimmedVersion := strings.TrimSpace(version)
	if trimmedVersion == "" || strings.EqualFold(trimmedVersion, "latest") {
		asset, err := assets.GetAsset(ctx, skillName)
		if err == nil {
			return asset, nil
		}
		if !errors.Is(err, database.ErrNotFound) {
			return nil, err
		}
		return s.findAssetByExactNameInStore(ctx, assets, skillName, "")
	}

	asset, err := assets.GetAssetVersion(ctx, skillName, trimmedVersion)
	if err == nil {
		return asset, nil
	}
	if !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}
	return s.findAssetByExactNameInStore(ctx, assets, skillName, trimmedVersion)
}

func (s *registry) findAssetByExactNameInStore(ctx context.Context, assets database.AssetStore, skillName, version string) (*models.AssetResponse, error) {
	if assets == nil {
		return nil, database.ErrNotFound
	}
	search := skillName
	filter := &database.AssetFilter{Search: &search}
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
			if asset == nil || asset.Asset.Name != skillName {
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

func sortSkillResponsesByRecency(skills []*models.SkillResponse) {
	sort.SliceStable(skills, func(i, j int) bool {
		left := skills[i]
		right := skills[j]

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
		if cmp := versionutil.CompareVersions(left.Skill.Version, right.Skill.Version, leftPublishedAt, rightPublishedAt); cmp != 0 {
			return cmp > 0
		}
		if !leftPublishedAt.Equal(rightPublishedAt) {
			return leftPublishedAt.After(rightPublishedAt)
		}
		return left.Skill.Name < right.Skill.Name
	})
}

func (s *registry) deleteMirroredAssetInStore(ctx context.Context, assets database.AssetStore, skillName, version string) error {
	if assets == nil {
		return database.ErrNotFound
	}
	asset, err := s.findAssetBackedAssetInStore(ctx, assets, skillName, version)
	if err != nil {
		return err
	}
	return assets.DeleteAsset(ctx, asset.Asset.ID, asset.Asset.Version)
}

func (s *registry) mirrorAssetInStore(ctx context.Context, assets database.AssetStore, skillResponse *models.SkillResponse) error {
	if assets == nil || skillResponse == nil || skillResponse.Skill.SHUB == nil || skillResponse.Skill.SHUB.Manifest == nil {
		return nil
	}

	assetResponse, err := models.AssetResponseFromSkillResponse(skillResponse)
	if err != nil {
		return fmt.Errorf("convert skill response to asset response: %w", err)
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
// already owned by a different skill. Used by both the create and apply paths
// to enforce the same uniqueness invariant.
func (s *registry) validateNoDuplicateRemoteURLs(ctx context.Context, skills database.SkillStore, skillDetail models.SkillJSON) error {
	for _, remote := range skillDetail.Remotes {
		remoteURL := remote.URL
		filter := &database.SkillFilter{RemoteURL: &remoteURL}
		cursor := ""

		for {
			conflictingSkills, nextCursor, err := skills.ListSkills(ctx, filter, cursor, 1000)
			if err != nil {
				return fmt.Errorf("failed to check remote URL conflict: %w", err)
			}
			for _, conflictingSkill := range conflictingSkills {
				if conflictingSkill.Skill.Name != skillDetail.Name {
					return fmt.Errorf("remote URL %s is already used by skill %s", remoteURL, conflictingSkill.Skill.Name)
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

func (s *registry) createSkillInTransaction(ctx context.Context, skills database.SkillStore, assets database.AssetStore, req *models.SkillJSON) (*models.SkillResponse, error) {
	if req == nil || req.Name == "" || req.Version == "" {
		return nil, fmt.Errorf("invalid skill payload: name and version are required")
	}

	publishTime := time.Now()
	skillJSON := *req

	if err := s.validateNoDuplicateRemoteURLs(ctx, skills, skillJSON); err != nil {
		return nil, err
	}

	versionCount, err := skills.CountSkillVersions(ctx, skillJSON.Name)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}
	if versionCount >= maxVersionsPerSkill {
		return nil, database.ErrMaxVersionsReached
	}

	exists, err := skills.CheckSkillVersionExists(ctx, skillJSON.Name, skillJSON.Version)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, database.ErrInvalidVersion
	}

	currentLatest, err := skills.GetLatestSkill(ctx, skillJSON.Name)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	isNewLatest := true
	if currentLatest != nil {
		var existingPublishedAt time.Time
		if currentLatest.Meta.Official != nil {
			existingPublishedAt = currentLatest.Meta.Official.PublishedAt
		}
		if versionutil.CompareVersions(skillJSON.Version, currentLatest.Skill.Version, publishTime, existingPublishedAt) <= 0 {
			isNewLatest = false
		}
	}

	if isNewLatest && currentLatest != nil {
		if err := skills.UnmarkSkillAsLatest(ctx, skillJSON.Name); err != nil {
			return nil, err
		}
	}

	officialMeta := &models.SkillRegistryExtensions{
		Status:      string(model.StatusActive),
		PublishedAt: publishTime,
		UpdatedAt:   publishTime,
		IsLatest:    isNewLatest,
	}

	createdSkill, err := skills.CreateSkill(ctx, &skillJSON, officialMeta)
	if err != nil {
		return nil, err
	}
	if err := s.mirrorAssetInStore(ctx, assets, createdSkill); err != nil {
		return nil, err
	}
	return createdSkill, nil
}

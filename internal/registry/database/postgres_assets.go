package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
)

type assetStore struct {
	repositoryBase
}

var _ database.AssetStore = (*assetStore)(nil)

func (store *assetStore) ListAssets(ctx context.Context, filter *database.AssetFilter, cursor string, limit int) ([]*models.AssetResponse, string, error) {
	if limit <= 0 {
		limit = 10
	}
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}

	var whereConditions []string
	args := []any{}
	argIndex := 1

	if filter != nil { //nolint:nestif
		if filter.ID != nil {
			whereConditions = append(whereConditions, fmt.Sprintf("asset_id = $%d", argIndex))
			args = append(args, *filter.ID)
			argIndex++
		}
		if filter.UpdatedSince != nil {
			whereConditions = append(whereConditions, fmt.Sprintf("updated_at > $%d", argIndex))
			args = append(args, *filter.UpdatedSince)
			argIndex++
		}
		if filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
			whereConditions = append(whereConditions, fmt.Sprintf("(asset_id ILIKE $%d OR value->>'name' ILIKE $%d OR value->>'description' ILIKE $%d)", argIndex, argIndex, argIndex))
			args = append(args, "%"+*filter.Search+"%")
			argIndex++
		}
		if filter.Version != nil {
			whereConditions = append(whereConditions, fmt.Sprintf("version = $%d", argIndex))
			args = append(args, *filter.Version)
			argIndex++
		}
		if filter.IsLatest != nil {
			whereConditions = append(whereConditions, fmt.Sprintf("is_latest = $%d", argIndex))
			args = append(args, *filter.IsLatest)
			argIndex++
		}
		if filter.Category != nil {
			whereConditions = append(whereConditions, fmt.Sprintf("category = $%d", argIndex))
			args = append(args, string(*filter.Category))
			argIndex++
		}
	}

	if cursor != "" {
		parts := strings.SplitN(cursor, ":", 2)
		if len(parts) == 2 {
			cursorID := parts[0]
			cursorVersion := parts[1]
			whereConditions = append(whereConditions, fmt.Sprintf("(asset_id > $%d OR (asset_id = $%d AND version > $%d))", argIndex, argIndex+1, argIndex+2))
			args = append(args, cursorID, cursorID, cursorVersion)
			argIndex += 3
		} else {
			whereConditions = append(whereConditions, fmt.Sprintf("asset_id > $%d", argIndex))
			args = append(args, cursor)
			argIndex++
		}
	}

	whereClause := ""
	if len(whereConditions) > 0 {
		whereClause = "WHERE " + strings.Join(whereConditions, " AND ")
	}

	query := fmt.Sprintf(`
        SELECT asset_id, version, category, status, published_at, updated_at, is_latest, value
        FROM assets
        %s
        ORDER BY asset_id, version
        LIMIT $%d
    `, whereClause, argIndex)
	args = append(args, limit)

	rows, err := store.executor.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query assets: %w", err)
	}
	defer rows.Close()

	var results []*models.AssetResponse
	for rows.Next() {
		assetResponse, err := scanAssetRow(rows)
		if err != nil {
			return nil, "", err
		}
		results = append(results, assetResponse)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("error iterating asset rows: %w", err)
	}

	nextCursor := ""
	if len(results) > 0 && len(results) >= limit {
		last := results[len(results)-1]
		nextCursor = last.Asset.ID + ":" + last.Asset.Version
	}
	return results, nextCursor, nil
}

func (store *assetStore) GetAsset(ctx context.Context, assetID string) (*models.AssetResponse, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := store.authz.Check(ctx, auth.PermissionActionRead, auth.Resource{Name: assetID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return nil, err
	}
	return store.querySingleAsset(ctx, `
        SELECT asset_id, version, category, status, published_at, updated_at, is_latest, value
        FROM assets
        WHERE asset_id = $1 AND is_latest = true
        ORDER BY published_at DESC
        LIMIT 1
    `, assetID)
}

func (store *assetStore) GetAssetVersion(ctx context.Context, assetID, version string) (*models.AssetResponse, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := store.authz.Check(ctx, auth.PermissionActionRead, auth.Resource{Name: assetID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return nil, err
	}
	return store.querySingleAsset(ctx, `
        SELECT asset_id, version, category, status, published_at, updated_at, is_latest, value
        FROM assets
        WHERE asset_id = $1 AND version = $2
        LIMIT 1
    `, assetID, version)
}

func (store *assetStore) GetAssetVersions(ctx context.Context, assetID string) ([]*models.AssetResponse, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := store.authz.Check(ctx, auth.PermissionActionRead, auth.Resource{Name: assetID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return nil, err
	}

	rows, err := store.executor.Query(ctx, `
        SELECT asset_id, version, category, status, published_at, updated_at, is_latest, value
        FROM assets
        WHERE asset_id = $1
        ORDER BY version
    `, assetID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset versions: %w", err)
	}
	defer rows.Close()

	versions := make([]*models.AssetResponse, 0)
	for rows.Next() {
		assetResponse, err := scanAssetRow(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, assetResponse)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating asset rows: %w", err)
	}
	if len(versions) == 0 {
		return nil, database.ErrNotFound
	}
	return versions, nil
}

func (store *assetStore) CreateAsset(ctx context.Context, asset *models.Asset, officialMeta *models.AssetRegistryExtensions) (*models.AssetResponse, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if asset == nil || officialMeta == nil {
		return nil, fmt.Errorf("asset and officialMeta are required")
	}
	if strings.TrimSpace(asset.ID) == "" || strings.TrimSpace(asset.Version) == "" {
		return nil, fmt.Errorf("asset id and version are required")
	}
	if err := store.authz.Check(ctx, auth.PermissionActionPublish, auth.Resource{Name: asset.ID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return nil, err
	}

	stored := *asset
	stored.Status = officialMeta.Status
	valueJSON, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal asset JSON: %w", err)
	}

	query := `
        INSERT INTO assets (asset_id, version, category, status, published_at, updated_at, is_latest, value)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        RETURNING asset_id, version, category, status, published_at, updated_at, is_latest, value
    `
	row := store.executor.QueryRow(ctx, query,
		stored.ID,
		stored.Version,
		string(stored.Category),
		officialMeta.Status,
		officialMeta.PublishedAt,
		officialMeta.UpdatedAt,
		officialMeta.IsLatest,
		valueJSON,
	)
	result, err := scanAssetRow(row)
	if err != nil {
		return nil, err
	}
	if err := store.assignResourceOwner(ctx, auth.PermissionArtifactTypeAsset, asset.ID); err != nil {
		return nil, err
	}
	return result, nil
}

func (store *assetStore) UpdateAsset(ctx context.Context, assetID, version string, asset *models.Asset, officialMeta *models.AssetRegistryExtensions) (*models.AssetResponse, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if asset == nil || officialMeta == nil {
		return nil, fmt.Errorf("asset and officialMeta are required")
	}
	if strings.TrimSpace(assetID) == "" || strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("asset id and version are required")
	}
	if asset.ID != "" && asset.ID != assetID {
		return nil, fmt.Errorf("asset id mismatch: %s != %s", asset.ID, assetID)
	}
	if asset.Version != "" && asset.Version != version {
		return nil, fmt.Errorf("asset version mismatch: %s != %s", asset.Version, version)
	}
	if err := store.authz.Check(ctx, auth.PermissionActionPublish, auth.Resource{Name: assetID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return nil, err
	}

	stored := *asset
	stored.ID = assetID
	stored.Version = version
	stored.Status = officialMeta.Status
	valueJSON, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal asset JSON: %w", err)
	}

	query := `
        UPDATE assets
        SET category = $3,
            status = $4,
            published_at = $5,
            updated_at = $6,
            is_latest = $7,
            value = $8
        WHERE asset_id = $1 AND version = $2
        RETURNING asset_id, version, category, status, published_at, updated_at, is_latest, value
    `
	row := store.executor.QueryRow(ctx, query,
		assetID,
		version,
		string(stored.Category),
		officialMeta.Status,
		officialMeta.PublishedAt,
		officialMeta.UpdatedAt,
		officialMeta.IsLatest,
		valueJSON,
	)
	return scanAssetRow(row)
}

func (store *assetStore) DeleteAsset(ctx context.Context, assetID, version string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := store.authz.Check(ctx, auth.PermissionActionDelete, auth.Resource{Name: assetID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return err
	}

	var wasLatest bool
	if err := store.executor.QueryRow(ctx,
		`SELECT is_latest FROM assets WHERE asset_id = $1 AND version = $2`,
		assetID, version,
	).Scan(&wasLatest); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.ErrNotFound
		}
		return fmt.Errorf("failed to check asset latest status: %w", err)
	}

	result, err := store.executor.Exec(ctx, `DELETE FROM assets WHERE asset_id = $1 AND version = $2`, assetID, version)
	if err != nil {
		return fmt.Errorf("failed to delete asset: %w", err)
	}
	if result.RowsAffected() == 0 {
		return database.ErrNotFound
	}

	if wasLatest {
		promoteQuery := `
			UPDATE assets SET is_latest = true
			WHERE asset_id = $1
			  AND version = (
			    SELECT version FROM assets
			    WHERE asset_id = $1
			    ORDER BY published_at DESC
			    LIMIT 1
			  )
		`
		if _, err := store.executor.Exec(ctx, promoteQuery, assetID); err != nil {
			return fmt.Errorf("failed to promote next latest asset version: %w", err)
		}
	}

	return nil
}

func (store *assetStore) GetLatestAsset(ctx context.Context, assetID string) (*models.AssetResponse, error) {
	return store.GetAsset(ctx, assetID)
}

func (store *assetStore) CountAssetVersions(ctx context.Context, assetID string) (int, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if err := store.authz.Check(ctx, auth.PermissionActionRead, auth.Resource{Name: assetID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return 0, err
	}

	var count int
	if err := store.executor.QueryRow(ctx, `SELECT COUNT(*) FROM assets WHERE asset_id = $1`, assetID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count asset versions: %w", err)
	}
	return count, nil
}

func (store *assetStore) CheckAssetVersionExists(ctx context.Context, assetID, version string) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err := store.authz.Check(ctx, auth.PermissionActionRead, auth.Resource{Name: assetID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return false, err
	}

	var exists bool
	if err := store.executor.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM assets WHERE asset_id = $1 AND version = $2)`, assetID, version).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check asset version existence: %w", err)
	}
	return exists, nil
}

func (store *assetStore) UnmarkAssetAsLatest(ctx context.Context, assetID string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := store.authz.Check(ctx, auth.PermissionActionPublish, auth.Resource{Name: assetID, Type: auth.PermissionArtifactTypeAsset}); err != nil {
		return err
	}
	if _, err := store.executor.Exec(ctx, `UPDATE assets SET is_latest = false WHERE asset_id = $1 AND is_latest = true`, assetID); err != nil {
		return fmt.Errorf("failed to unmark latest asset version: %w", err)
	}
	return nil
}

func (store *assetStore) querySingleAsset(ctx context.Context, query string, args ...any) (*models.AssetResponse, error) {
	return scanAssetRow(store.executor.QueryRow(ctx, query, args...))
}

func scanAssetRow(row interface{ Scan(dest ...any) error }) (*models.AssetResponse, error) {
	var assetID, version, category, status string
	var publishedAt, updatedAt time.Time
	var isLatest bool
	var valueJSON []byte
	if err := row.Scan(&assetID, &version, &category, &status, &publishedAt, &updatedAt, &isLatest, &valueJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, database.ErrNotFound
		}
		return nil, fmt.Errorf("failed to scan asset row: %w", err)
	}

	var asset models.Asset
	if err := json.Unmarshal(valueJSON, &asset); err != nil {
		return nil, fmt.Errorf("failed to unmarshal asset JSON: %w", err)
	}
	if asset.ID == "" {
		asset.ID = assetID
	}
	if asset.Version == "" {
		asset.Version = version
	}
	if !asset.Category.IsValid() {
		asset.Category = models.AssetCategory(category)
	}
	asset.Status = status

	return &models.AssetResponse{
		Asset: asset,
		Meta: models.AssetResponseMeta{
			Official: &models.AssetRegistryExtensions{
				Status:      status,
				PublishedAt: publishedAt,
				UpdatedAt:   updatedAt,
				IsLatest:    isLatest,
			},
		},
	}, nil
}

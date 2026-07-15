package assets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/agentregistry-dev/agentregistry/internal/registry/api/apitypes"
	assetsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/asset"
	assetmodels "github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/agentregistry-dev/agentregistry/pkg/types"
	"github.com/danielgtaylor/huma/v2"
)

type AssetDetailInput struct {
	AssetID string `path:"assetID" json:"assetID" doc:"URL-encoded asset id" example:"arch%2Fjava-analyzer"`
}

type AssetVersionDetailInput struct {
	AssetID string `path:"assetID" json:"assetID" doc:"URL-encoded asset id" example:"arch%2Fjava-analyzer"`
	Version string `path:"version" json:"version" doc:"URL-encoded asset version" example:"1.2.0"`
}

type AssetVersionsInput struct {
	AssetID string `path:"assetID" json:"assetID" doc:"URL-encoded asset id" example:"arch%2Fjava-analyzer"`
}

type CreateAssetInput struct {
	Body assetmodels.AssetPublishRequest `body:""`
}

type UploadAssetPackageInput struct {
	AssetID     string `path:"assetID" json:"assetID" doc:"URL-encoded asset id" example:"arch%2Fjava-analyzer"`
	Version     string `path:"version" json:"version" doc:"URL-encoded asset version" example:"1.2.0"`
	ContentType string `header:"Content-Type" doc:"Package content type"`
	RawBody     []byte `contentType:"application/gzip" required:"true" doc:"Packaged SHUB asset (.tar.gz or .tgz)"`
}

func RegisterAssetsEndpoints(api huma.API, pathPrefix string, assetSvc assetsvc.Registry) {
	tags := []string{"assets"}
	if strings.Contains(pathPrefix, "admin") {
		tags = append(tags, "admin")
	}

	huma.Register(api, huma.Operation{
		OperationID: "list-assets" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodGet,
		Path:        pathPrefix + "/assets",
		Summary:     "List SHUB assets",
		Description: "Get a paginated list of SHUB-compatible assets from the registry compatibility layer",
		Tags:        tags,
	}, func(ctx context.Context, input *apitypes.ListAssetsInput) (*types.Response[assetmodels.AssetListResponse], error) {
		filter := &assetsvc.Filter{}
		if input.UpdatedSince != "" {
			updatedTime, err := time.Parse(time.RFC3339, input.UpdatedSince)
			if err != nil {
				return nil, huma.Error400BadRequest("Invalid updated_since format: expected RFC3339 timestamp (e.g., 2025-08-07T13:15:04.280Z)")
			}
			filter.UpdatedSince = &updatedTime
		}
		if input.Search != "" {
			filter.Search = &input.Search
		}
		if input.Version != "" {
			if input.Version == "latest" {
				latest := true
				filter.IsLatest = &latest
			} else {
				filter.Version = &input.Version
			}
		}
		if input.Category != "" {
			category := assetmodels.AssetCategory(input.Category)
			if !category.IsValid() {
				return nil, huma.Error400BadRequest("Invalid category: expected prompt, skill, agent, or mcp")
			}
			filter.Category = &category
		}

		assets, nextCursor, err := assetSvc.ListAssets(ctx, filter, input.Cursor, input.Limit)
		if err != nil {
			return nil, mapAssetError("Failed to get assets list", err)
		}

		values := make([]assetmodels.AssetResponse, len(assets))
		for index, asset := range assets {
			values[index] = *asset
		}
		return &types.Response[assetmodels.AssetListResponse]{
			Body: assetmodels.AssetListResponse{
				Assets: values,
				Metadata: assetmodels.AssetMetadata{
					NextCursor: nextCursor,
					Count:      len(assets),
				},
			},
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-asset-latest" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodGet,
		Path:        pathPrefix + "/assets/{assetID}",
		Summary:     "Get latest asset version",
		Description: "Get detailed information about the latest version of an asset",
		Tags:        tags,
	}, func(ctx context.Context, input *AssetDetailInput) (*types.Response[assetmodels.AssetResponse], error) {
		assetID, err := url.PathUnescape(input.AssetID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid asset id encoding", err)
		}
		asset, err := assetSvc.GetAsset(ctx, assetID)
		if err != nil {
			return nil, mapAssetError("Failed to get asset details", err)
		}
		return &types.Response[assetmodels.AssetResponse]{Body: *asset}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-asset-version" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodGet,
		Path:        pathPrefix + "/assets/{assetID}/versions/{version}",
		Summary:     "Get specific asset version",
		Description: "Get detailed information about a specific asset version. Use version 'latest' to fetch the latest version.",
		Tags:        tags,
	}, func(ctx context.Context, input *AssetVersionDetailInput) (*types.Response[assetmodels.AssetResponse], error) {
		assetID, err := url.PathUnescape(input.AssetID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid asset id encoding", err)
		}
		version, err := url.PathUnescape(input.Version)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid version encoding", err)
		}
		asset, err := assetSvc.GetAssetVersion(ctx, assetID, version)
		if err != nil {
			return nil, mapAssetError("Failed to get asset details", err)
		}
		return &types.Response[assetmodels.AssetResponse]{Body: *asset}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-asset-versions" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodGet,
		Path:        pathPrefix + "/assets/{assetID}/versions",
		Summary:     "Get all versions of an asset",
		Description: "Get all available versions for a specific asset id",
		Tags:        tags,
	}, func(ctx context.Context, input *AssetVersionsInput) (*types.Response[assetmodels.AssetListResponse], error) {
		assetID, err := url.PathUnescape(input.AssetID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid asset id encoding", err)
		}
		assets, err := assetSvc.GetAssetVersions(ctx, assetID)
		if err != nil {
			return nil, mapAssetError("Failed to get asset versions", err)
		}
		values := make([]assetmodels.AssetResponse, len(assets))
		for index, asset := range assets {
			values[index] = *asset
		}
		return &types.Response[assetmodels.AssetListResponse]{
			Body: assetmodels.AssetListResponse{
				Assets:   values,
				Metadata: assetmodels.AssetMetadata{Count: len(values)},
			},
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-asset-package-version" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodGet,
		Path:        pathPrefix + "/assets/{assetID}/versions/{version}/package",
		Summary:     "Download a published SHUB asset package",
		Description: "Stream the package archive for a specific SHUB asset version hosted by the registry.",
		Tags:        tags,
	}, func(ctx context.Context, input *AssetVersionDetailInput) (*huma.StreamResponse, error) {
		assetID, err := url.PathUnescape(input.AssetID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid asset id encoding", err)
		}
		version, err := url.PathUnescape(input.Version)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid version encoding", err)
		}
		pkg, err := assetSvc.GetAssetPackage(ctx, assetID, version)
		if err != nil {
			return nil, mapAssetError("Failed to get asset package", err)
		}
		return &huma.StreamResponse{
			Body: func(hctx huma.Context) {
				hctx.SetHeader("Content-Type", pkg.Package.ContentType)
				hctx.SetHeader("Content-Length", strconv.Itoa(pkg.Package.SizeBytes))
				writer := hctx.BodyWriter()
				_, _ = writer.Write(pkg.Content)
			},
		}, nil
	})
}

func createAssetHandler(ctx context.Context, input *CreateAssetInput, assetSvc assetsvc.Registry) (*types.Response[assetmodels.AssetResponse], error) {
	createdAsset, err := assetSvc.PublishAsset(ctx, &input.Body)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, huma.Error404NotFound("Asset dependency not found")
		}
		if errors.Is(err, auth.ErrUnauthenticated) {
			return nil, huma.Error401Unauthorized("Authentication required")
		}
		if errors.Is(err, auth.ErrForbidden) {
			return nil, huma.Error403Forbidden("Forbidden")
		}
		return nil, huma.Error400BadRequest("Failed to create asset", err)
	}
	return &types.Response[assetmodels.AssetResponse]{Body: *createdAsset}, nil
}

func RegisterAssetsCreateEndpoint(api huma.API, pathPrefix string, assetSvc assetsvc.Registry) {
	huma.Register(api, huma.Operation{
		OperationID: "create-asset" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodPost,
		Path:        pathPrefix + "/assets",
		Summary:     "Create or publish asset",
		Description: "Create a new SHUB asset version in the registry compatibility layer.",
		Tags:        []string{"assets"},
	}, func(ctx context.Context, input *CreateAssetInput) (*types.Response[assetmodels.AssetResponse], error) {
		return createAssetHandler(ctx, input, assetSvc)
	})

	huma.Register(api, huma.Operation{
		OperationID: "upload-asset-package" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodPut,
		Path:        pathPrefix + "/assets/{assetID}/versions/{version}/package",
		Summary:     "Upload a SHUB asset package blob",
		Description: "Upload a packaged SHUB asset archive for registry-hosted distribution.",
		Tags:        []string{"assets"},
	}, func(ctx context.Context, input *UploadAssetPackageInput) (*types.Response[assetmodels.AssetPackageResponse], error) {
		assetID, err := url.PathUnescape(input.AssetID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid asset id encoding", err)
		}
		version, err := url.PathUnescape(input.Version)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid version encoding", err)
		}
		response, err := assetSvc.UploadAssetPackage(ctx, assetID, version, input.RawBody, input.ContentType)
		if err != nil {
			return nil, mapAssetError("Failed to upload asset package", err)
		}
		if response != nil && strings.TrimSpace(response.DownloadURL) == "" {
			response.DownloadURL = fmt.Sprintf("%s/assets/%s/versions/%s/package", pathPrefix, url.PathEscape(assetID), url.PathEscape(version))
		}
		return &types.Response[assetmodels.AssetPackageResponse]{Body: *response}, nil
	})
}

func mapAssetError(message string, err error) error {
	if errors.Is(err, database.ErrNotFound) {
		return huma.Error404NotFound("Asset not found")
	}
	if errors.Is(err, auth.ErrUnauthenticated) {
		return huma.Error401Unauthorized("Authentication required")
	}
	if errors.Is(err, auth.ErrForbidden) {
		return huma.Error403Forbidden("Forbidden")
	}
	return huma.Error500InternalServerError(message, err)
}

package shubsources

import (
	"context"
	"errors"
	"net/http"
	"strings"

	shubsourcesvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/shubsource"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/agentregistry-dev/agentregistry/pkg/types"
	"github.com/danielgtaylor/huma/v2"
)

type SourceByNameInput struct {
	Name string `path:"name" json:"name" doc:"Configured SHUB source name"`
}

type PutSourceInput struct {
	Name string                         `path:"name" json:"name" doc:"Configured SHUB source name"`
	Body models.SHUBSourceUpsertRequest `body:""`
}

type PullSourceInput struct {
	Name string                       `path:"name" json:"name" doc:"Configured SHUB source name"`
	Body models.SHUBSourcePullRequest `body:""`
}

type SHUBSourcesListResponse struct {
	Body struct {
		Sources []models.SHUBSource `json:"sources"`
		Count   int                 `json:"count"`
	}
}

func RegisterSHUBSourceEndpoints(api huma.API, pathPrefix string, sourceSvc shubsourcesvc.Registry) {
	tags := []string{"shub"}

	huma.Register(api, huma.Operation{
		OperationID: "list-shub-sources" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodGet,
		Path:        pathPrefix + "/shub/sources",
		Summary:     "List SHUB fallback sources",
		Description: "List backend-configured GitHub/GitLab-style SHUB fallback sources.",
		Tags:        tags,
	}, func(ctx context.Context, _ *struct{}) (*SHUBSourcesListResponse, error) {
		sources, err := sourceSvc.ListSources(ctx)
		if err != nil {
			return nil, mapSourceError("Failed to list SHUB sources", err)
		}
		resp := &SHUBSourcesListResponse{}
		resp.Body.Sources = make([]models.SHUBSource, 0, len(sources))
		for _, source := range sources {
			if source == nil {
				continue
			}
			resp.Body.Sources = append(resp.Body.Sources, *source)
		}
		resp.Body.Count = len(resp.Body.Sources)
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-shub-source" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodPut,
		Path:        pathPrefix + "/shub/sources/{name}",
		Summary:     "Create or update a SHUB fallback source",
		Description: "Create or update a named fallback source used by `shub add --fallback-source`.",
		Tags:        tags,
	}, func(ctx context.Context, input *PutSourceInput) (*types.Response[models.SHUBSource], error) {
		source, err := sourceSvc.SetSource(ctx, input.Name, input.Body.Address)
		if err != nil {
			return nil, mapSourceError("Failed to save SHUB source", err)
		}
		return &types.Response[models.SHUBSource]{Body: *source}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-shub-source" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodGet,
		Path:        pathPrefix + "/shub/sources/{name}",
		Summary:     "Get a SHUB fallback source",
		Description: "Return one configured SHUB fallback source by name.",
		Tags:        tags,
	}, func(ctx context.Context, input *SourceByNameInput) (*types.Response[models.SHUBSource], error) {
		source, err := sourceSvc.GetSource(ctx, input.Name)
		if err != nil {
			return nil, mapSourceError("Failed to get SHUB source", err)
		}
		return &types.Response[models.SHUBSource]{Body: *source}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-shub-source" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodDelete,
		Path:        pathPrefix + "/shub/sources/{name}",
		Summary:     "Delete a SHUB fallback source",
		Description: "Delete a named SHUB fallback source.",
		Tags:        tags,
	}, func(ctx context.Context, input *SourceByNameInput) (*struct{}, error) {
		if err := sourceSvc.DeleteSource(ctx, input.Name); err != nil {
			return nil, mapSourceError("Failed to delete SHUB source", err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "pull-shub-source-asset" + strings.ReplaceAll(pathPrefix, "/", "-"),
		Method:      http.MethodPost,
		Path:        pathPrefix + "/shub/sources/{name}/pull",
		Summary:     "Pull an asset from a SHUB fallback source",
		Description: "Fetch an asset from the named fallback source, mirror its package into registry storage, and publish it into the registry.",
		Tags:        tags,
	}, func(ctx context.Context, input *PullSourceInput) (*types.Response[models.AssetResponse], error) {
		asset, err := sourceSvc.PullAsset(ctx, input.Name, input.Body.AssetID, input.Body.Version)
		if err != nil {
			return nil, mapSourceError("Failed to pull SHUB asset from source", err)
		}
		return &types.Response[models.AssetResponse]{Body: *asset}, nil
	})
}

func mapSourceError(message string, err error) error {
	switch {
	case errors.Is(err, database.ErrNotFound):
		return huma.Error404NotFound("SHUB source not found")
	case errors.Is(err, database.ErrInvalidInput):
		return huma.Error400BadRequest(err.Error())
	case errors.Is(err, auth.ErrUnauthenticated):
		return huma.Error401Unauthorized("Authentication required")
	case errors.Is(err, auth.ErrForbidden):
		return huma.Error403Forbidden("Forbidden")
	default:
		return huma.Error500InternalServerError(message, err)
	}
}

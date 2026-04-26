package userauth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	userauthsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/userauth"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/agentregistry-dev/agentregistry/pkg/types"
	"github.com/danielgtaylor/huma/v2"
)

type apiKeyByIDInput struct {
	ID string `path:"id" json:"id"`
}

type loginInput struct {
	Body models.LoginRequest `body:""`
}

type createUserInput struct {
	Body models.CreateUserRequest `body:""`
}

type updateSettingsInput struct {
	Body models.UpdateRegistryAuthSettingsRequest `body:""`
}

type createAPIKeyInput struct {
	Body models.CreateAPIKeyRequest `body:""`
}

type userListResponse struct {
	Body struct {
		Users []models.RegistryUser `json:"users"`
		Count int                   `json:"count"`
	}
}

type apiKeyListResponse struct {
	Body struct {
		APIKeys []models.APIKey `json:"apiKeys"`
		Count   int             `json:"count"`
	}
}

func RegisterAuthEndpoints(api huma.API, pathPrefix string, svc userauthsvc.Registry) {
	tags := []string{"auth"}

	huma.Register(api, huma.Operation{
		OperationID: "login-registry-user",
		Method:      http.MethodPost,
		Path:        pathPrefix + "/auth/login",
		Summary:     "Login with username and password",
		Tags:        tags,
	}, func(ctx context.Context, input *loginInput) (*types.Response[models.LoginResponse], error) {
		resp, err := svc.Login(ctx, &input.Body)
		if err != nil {
			return nil, mapAuthError("Failed to login", err)
		}
		return &types.Response[models.LoginResponse]{Body: *resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-current-user",
		Method:      http.MethodGet,
		Path:        pathPrefix + "/auth/me",
		Summary:     "Get the current authenticated user",
		Tags:        tags,
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*types.Response[models.RegistryUser], error) {
		user, err := svc.Me(ctx)
		if err != nil {
			return nil, mapAuthError("Failed to get current user", err)
		}
		return &types.Response[models.RegistryUser]{Body: *user}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-registry-users",
		Method:      http.MethodGet,
		Path:        pathPrefix + "/auth/users",
		Summary:     "List registry users",
		Tags:        tags,
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*userListResponse, error) {
		users, err := svc.ListUsers(ctx)
		if err != nil {
			return nil, mapAuthError("Failed to list users", err)
		}
		resp := &userListResponse{}
		resp.Body.Users = make([]models.RegistryUser, 0, len(users))
		for _, user := range users {
			if user != nil {
				resp.Body.Users = append(resp.Body.Users, *user)
			}
		}
		resp.Body.Count = len(resp.Body.Users)
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-registry-user",
		Method:      http.MethodPost,
		Path:        pathPrefix + "/auth/users",
		Summary:     "Create a registry user",
		Tags:        tags,
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *createUserInput) (*types.Response[models.RegistryUser], error) {
		user, err := svc.CreateUser(ctx, &input.Body)
		if err != nil {
			return nil, mapAuthError("Failed to create user", err)
		}
		return &types.Response[models.RegistryUser]{Body: *user}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-registry-api-keys",
		Method:      http.MethodGet,
		Path:        pathPrefix + "/auth/api-keys",
		Summary:     "List API keys for the current user",
		Tags:        tags,
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*apiKeyListResponse, error) {
		keys, err := svc.ListAPIKeys(ctx)
		if err != nil {
			return nil, mapAuthError("Failed to list API keys", err)
		}
		resp := &apiKeyListResponse{}
		resp.Body.APIKeys = make([]models.APIKey, 0, len(keys))
		for _, key := range keys {
			if key != nil {
				resp.Body.APIKeys = append(resp.Body.APIKeys, *key)
			}
		}
		resp.Body.Count = len(resp.Body.APIKeys)
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-registry-auth-settings",
		Method:      http.MethodGet,
		Path:        pathPrefix + "/auth/settings",
		Summary:     "Get registry auth settings",
		Tags:        tags,
	}, func(ctx context.Context, _ *struct{}) (*types.Response[models.RegistryAuthSettings], error) {
		settings, err := svc.GetSettings(ctx)
		if err != nil {
			return nil, mapAuthError("Failed to get auth settings", err)
		}
		return &types.Response[models.RegistryAuthSettings]{Body: *settings}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-registry-auth-settings",
		Method:      http.MethodPut,
		Path:        pathPrefix + "/auth/settings",
		Summary:     "Update registry auth settings",
		Tags:        tags,
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *updateSettingsInput) (*types.Response[models.RegistryAuthSettings], error) {
		settings, err := svc.UpdateSettings(ctx, &input.Body)
		if err != nil {
			return nil, mapAuthError("Failed to update auth settings", err)
		}
		return &types.Response[models.RegistryAuthSettings]{Body: *settings}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-registry-api-key",
		Method:      http.MethodPost,
		Path:        pathPrefix + "/auth/api-keys",
		Summary:     "Create an API key for the current user",
		Tags:        tags,
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *createAPIKeyInput) (*types.Response[models.CreateAPIKeyResponse], error) {
		resp, err := svc.CreateAPIKey(ctx, &input.Body)
		if err != nil {
			return nil, mapAuthError("Failed to create API key", err)
		}
		return &types.Response[models.CreateAPIKeyResponse]{Body: *resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-registry-api-key",
		Method:      http.MethodDelete,
		Path:        pathPrefix + "/auth/api-keys/{id}",
		Summary:     "Delete an API key",
		Tags:        tags,
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *apiKeyByIDInput) (*struct{}, error) {
		if err := svc.DeleteAPIKey(ctx, input.ID); err != nil {
			return nil, mapAuthError("Failed to delete API key", err)
		}
		return &struct{}{}, nil
	})
}

func mapAuthError(message string, err error) error {
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		return huma.Error401Unauthorized("Authentication required")
	case errors.Is(err, auth.ErrForbidden):
		return huma.Error403Forbidden("Forbidden")
	case strings.Contains(strings.ToLower(err.Error()), "required"), strings.Contains(strings.ToLower(err.Error()), "unsupported role"):
		return huma.Error400BadRequest(err.Error())
	default:
		return huma.Error500InternalServerError(message, err)
	}
}

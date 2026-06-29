package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	apitypes "github.com/agentregistry-dev/agentregistry/internal/registry/api/apitypes"
	"github.com/agentregistry-dev/agentregistry/internal/registry/kinds"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
	v0 "github.com/modelcontextprotocol/registry/pkg/api/v0"
)

// Client is a lightweight API client replacing the previous SQLite backend
type Client struct {
	BaseURL    string
	httpClient *http.Client
	token      string
}

const (
	defaultBaseURL = "http://localhost:12121/v0"
	DefaultBaseURL = defaultBaseURL

	envARCTLAPIBaseURL = "ARCTL_API_BASE_URL"
	envARCTLAPIToken   = "ARCTL_API_TOKEN"
	envSHUBAPIBaseURL  = "SHUB_API_BASE_URL"
	envSHUBAPIToken    = "SHUB_API_TOKEN"
)

type VersionBody = apitypes.VersionBody

type IndexRequest = apitypes.IndexRequest

type IndexJobResponse = apitypes.IndexJobResponse

type JobProgress = apitypes.JobProgress

type JobResult = apitypes.JobResult

type JobStatusResponse = apitypes.JobStatusResponse

// NewClientFromEnv constructs a client using environment variables
func NewClientFromEnv() (*Client, error) {
	base, token := ResolveEnvConfig(os.Getenv)
	return NewClientWithConfig(base, token)
}

// ResolveEnvConfig resolves client config from environment variables.
// ARCTL_* takes precedence over SHUB_* so the generic CLI override remains authoritative.
func ResolveEnvConfig(getEnv func(string) string) (baseURL, token string) {
	baseURL = strings.TrimSpace(firstNonEmpty(
		getEnv(envARCTLAPIBaseURL),
		getEnv(envSHUBAPIBaseURL),
	))
	token = strings.TrimSpace(firstNonEmpty(
		getEnv(envARCTLAPIToken),
		getEnv(envSHUBAPIToken),
	))
	return baseURL, token
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// NewClient constructs a client with explicit baseURL and token.
// The baseURL can be provided with or without the /v0 API prefix;
// if missing, /v0 is appended automatically.
func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = ensureV0Suffix(baseURL)
	return &Client{
		BaseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ensureV0Suffix appends /v0 to the URL if not already present.
func ensureV0Suffix(u string) string {
	u = strings.TrimRight(u, "/")
	if !strings.HasSuffix(u, "/v0") {
		u += "/v0"
	}
	return u
}

// NewClientWithConfig constructs a client from explicit inputs (flag/env), applies defaults, and verifies connectivity.
func NewClientWithConfig(baseURL, token string) (*Client, error) {
	c := NewClient(baseURL, token)
	if err := c.Ping(); err != nil {
		return nil, err
	}
	return c, nil
}

// Close is a no-op in API mode
func (c *Client) Close() error { return nil }

func (c *Client) newRequest(method, pathWithQuery string) (*http.Request, error) {
	fullURL := strings.TrimRight(c.BaseURL, "/") + pathWithQuery
	req, err := http.NewRequest(method, fullURL, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

func (c *Client) doJSON(req *http.Request, out any) error {
	if out != nil {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if msg := extractAPIErrorMessage(errBody); msg != "" {
			return fmt.Errorf("%s: %s", resp.Status, msg)
		}
		return fmt.Errorf("unexpected status: %s, %s", resp.Status, string(errBody))
	}
	if out == nil {
		return nil
	}
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(out)
}

// extractAPIErrorMessage parses a Huma-style JSON error body and returns a
// human-readable string with just the error messages. Returns "" if the body
// cannot be parsed.
func extractAPIErrorMessage(body []byte) string {
	var apiErr struct {
		Detail string `json:"detail"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &apiErr) != nil || (apiErr.Detail == "" && len(apiErr.Errors) == 0) {
		return ""
	}
	var msgs []string
	for _, e := range apiErr.Errors {
		if e.Message != "" {
			msgs = append(msgs, e.Message)
		}
	}
	if len(msgs) > 0 {
		return strings.Join(msgs, "; ")
	}
	return apiErr.Detail
}

func (c *Client) doJsonRequest(method, pathWithQuery string, in, out any) error {
	req, err := c.newRequest(method, pathWithQuery)
	if err != nil {
		return err
	}
	if in != nil {
		inBytes, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("failed to marshal %T: %w", in, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Body = io.NopCloser(bytes.NewReader(inBytes))
	}
	return c.doJSON(req, out)
}

// Ping checks connectivity to the API
func (c *Client) Ping() error {
	req, err := c.newRequest(http.MethodGet, "/ping")
	if err != nil {
		return err
	}
	return c.doJSON(req, nil)
}

func (c *Client) GetVersion() (*VersionBody, error) {
	req, err := c.newRequest(http.MethodGet, "/version")
	if err != nil {
		return nil, err
	}
	var resp VersionBody
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetPublishedServers returns all published MCP servers
func (c *Client) GetPublishedServers() ([]*v0.ServerResponse, error) {
	// Cursor-based pagination to fetch all servers
	limit := 100
	cursor := ""
	var all []*v0.ServerResponse

	for {
		q := fmt.Sprintf("/servers?limit=%d", limit)
		if cursor != "" {
			q += "&cursor=" + url.QueryEscape(cursor)
		}
		req, err := c.newRequest(http.MethodGet, q)
		if err != nil {
			return nil, err
		}

		var resp v0.ServerListResponse
		if err := c.doJSON(req, &resp); err != nil {
			return nil, err
		}

		for _, s := range resp.Servers {
			all = append(all, &s)
		}

		if resp.Metadata.NextCursor == "" {
			break
		}
		cursor = resp.Metadata.NextCursor
	}

	return all, nil
}

// GetServer returns a server by name (latest version)
func (c *Client) GetServer(name string) (*v0.ServerResponse, error) {
	return c.GetServerVersion(name, "latest")
}

// GetServerVersion returns a specific version of a server
func (c *Client) GetServerVersion(name, version string) (*v0.ServerResponse, error) {
	encName := url.PathEscape(name)
	encVersion := url.PathEscape(version)
	q := "/servers/" + encName + "/versions/" + encVersion
	req, err := c.newRequest(http.MethodGet, q)
	if err != nil {
		return nil, err
	}
	// The endpoint now returns ServerListResponse (even for a single version)
	var resp v0.ServerListResponse
	if err := c.doJSON(req, &resp); err != nil {
		// 404 -> not found returns nil
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get server by name and version: %w", err)
	}

	if len(resp.Servers) == 0 {
		return nil, nil
	}

	return &resp.Servers[0], nil
}

// GetServerVersions returns all versions of a server by name (public endpoint - only published)
func (c *Client) GetServerVersions(name string) ([]v0.ServerResponse, error) {
	encName := url.PathEscape(name)
	req, err := c.newRequest(http.MethodGet, "/servers/"+encName+"/versions")
	if err != nil {
		return nil, err
	}

	var resp v0.ServerListResponse
	if err := c.doJSON(req, &resp); err != nil {
		// 404 -> not found returns empty list
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get server versions: %w", err)
	}

	return resp.Servers, nil
}

// GetSkills returns all skills from connected registries
func (c *Client) GetSkills() ([]*models.SkillResponse, error) {
	limit := 100
	cursor := ""
	var all []*models.SkillResponse

	for {
		q := fmt.Sprintf("/skills?limit=%d", limit)
		if cursor != "" {
			q += "&cursor=" + url.QueryEscape(cursor)
		}
		req, err := c.newRequest(http.MethodGet, q)
		if err != nil {
			return nil, err
		}

		var resp models.SkillListResponse
		if err := c.doJSON(req, &resp); err != nil {
			return nil, err
		}
		for _, sk := range resp.Skills {
			all = append(all, &sk)
		}
		if resp.Metadata.NextCursor == "" {
			break
		}
		cursor = resp.Metadata.NextCursor
	}

	return all, nil
}

// GetSkill returns a skill by name
func (c *Client) GetSkill(name string) (*models.SkillResponse, error) {
	encName := url.PathEscape(name)
	req, err := c.newRequest(http.MethodGet, "/skills/"+encName+"/versions/latest")
	if err != nil {
		return nil, err
	}
	var resp models.SkillResponse
	if err := c.doJSON(req, &resp); err != nil {
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get skill by name: %w", err)
	}
	return &resp, nil
}

// GetSkillVersions returns all versions for a skill by name.
func (c *Client) GetSkillVersions(name string) ([]*models.SkillResponse, error) {
	encName := url.PathEscape(name)
	req, err := c.newRequest(http.MethodGet, "/skills/"+encName+"/versions")
	if err != nil {
		return nil, err
	}

	var resp models.SkillListResponse
	if err := c.doJSON(req, &resp); err != nil {
		// 404 -> not found returns empty list
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get skill versions: %w", err)
	}

	// Convert to pointer slice to match existing client method conventions.
	result := make([]*models.SkillResponse, len(resp.Skills))
	for i := range resp.Skills {
		result[i] = &resp.Skills[i]
	}

	return result, nil
}

// GetSkillVersion returns a specific version of a skill
func (c *Client) GetSkillVersion(name, version string) (*models.SkillResponse, error) {
	encName := url.PathEscape(name)
	encVersion := url.PathEscape(version)

	req, err := c.newRequest(http.MethodGet, "/skills/"+encName+"/versions/"+encVersion)
	if err != nil {
		return nil, err
	}

	var resp models.SkillResponse
	if err := c.doJSON(req, &resp); err != nil {
		// 404 -> not found returns nil
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get skill by name and version: %w", err)
	}

	return &resp, nil
}

// GetAssets returns all SHUB assets exposed by the compatibility asset API.
func (c *Client) GetAssets() ([]*models.AssetResponse, error) {
	assets, _, err := c.listAssets("", "", 100)
	return assets, err
}

// ListAssetsUpdatedSince returns assets changed after the provided RFC3339 timestamp,
// using cursor-based pagination from the compatibility asset API.
func (c *Client) ListAssetsUpdatedSince(updatedSince, cursor string, limit int) ([]*models.AssetResponse, string, error) {
	return c.listAssets(updatedSince, cursor, limit)
}

func (c *Client) listAssets(updatedSince, cursor string, limit int) ([]*models.AssetResponse, string, error) {
	if limit <= 0 {
		limit = 100
	}
	var all []*models.AssetResponse

	for {
		q := fmt.Sprintf("/assets?limit=%d", limit)
		if cursor != "" {
			q += "&cursor=" + url.QueryEscape(cursor)
		}
		if updatedSince != "" {
			q += "&updated_since=" + url.QueryEscape(updatedSince)
		}
		req, err := c.newRequest(http.MethodGet, q)
		if err != nil {
			return nil, "", err
		}

		var resp models.AssetListResponse
		if err := c.doJSON(req, &resp); err != nil {
			if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
				return nil, "", nil
			}
			return nil, "", fmt.Errorf("failed to get assets: %w", err)
		}
		for index := range resp.Assets {
			c.normalizeAssetResponse(&resp.Assets[index])
			all = append(all, &resp.Assets[index])
		}
		if resp.Metadata.NextCursor == "" {
			break
		}
		cursor = resp.Metadata.NextCursor
		if updatedSince != "" {
			return all, cursor, nil
		}
	}

	return all, "", nil
}

// GetAsset returns the latest version of a SHUB asset by canonical asset id.
func (c *Client) GetAsset(id string) (*models.AssetResponse, error) {
	encID := url.PathEscape(id)
	req, err := c.newRequest(http.MethodGet, "/assets/"+encID)
	if err != nil {
		return nil, err
	}
	var resp models.AssetResponse
	if err := c.doJSON(req, &resp); err != nil {
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get asset by id: %w", err)
	}
	c.normalizeAssetResponse(&resp)
	return &resp, nil
}

// GetAssetVersions returns all versions for a SHUB asset by canonical asset id.
func (c *Client) GetAssetVersions(id string) ([]*models.AssetResponse, error) {
	encID := url.PathEscape(id)
	req, err := c.newRequest(http.MethodGet, "/assets/"+encID+"/versions")
	if err != nil {
		return nil, err
	}

	var resp models.AssetListResponse
	if err := c.doJSON(req, &resp); err != nil {
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get asset versions: %w", err)
	}

	result := make([]*models.AssetResponse, len(resp.Assets))
	for index := range resp.Assets {
		c.normalizeAssetResponse(&resp.Assets[index])
		result[index] = &resp.Assets[index]
	}
	return result, nil
}

// GetAssetVersion returns a specific version of a SHUB asset.
func (c *Client) GetAssetVersion(id, version string) (*models.AssetResponse, error) {
	encID := url.PathEscape(id)
	encVersion := url.PathEscape(version)
	req, err := c.newRequest(http.MethodGet, "/assets/"+encID+"/versions/"+encVersion)
	if err != nil {
		return nil, err
	}
	var resp models.AssetResponse
	if err := c.doJSON(req, &resp); err != nil {
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get asset by id and version: %w", err)
	}
	c.normalizeAssetResponse(&resp)
	return &resp, nil
}

func (c *Client) AssetPackageURL(assetID, version string) string {
	base := strings.TrimRight(c.BaseURL, "/")
	return base + "/assets/" + url.PathEscape(assetID) + "/versions/" + url.PathEscape(version) + "/package"
}

func (c *Client) UploadAssetPackage(assetID, version string, content []byte, contentType string) (*models.AssetPackageResponse, error) {
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/gzip"
	}
	req, err := c.newRequest(http.MethodPut, "/assets/"+url.PathEscape(assetID)+"/versions/"+url.PathEscape(version)+"/package")
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Body = io.NopCloser(bytes.NewReader(content))

	var resp models.AssetPackageResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to upload asset package: %w", err)
	}
	if strings.TrimSpace(resp.DownloadURL) == "" {
		resp.DownloadURL = c.AssetPackageURL(assetID, version)
	} else if strings.HasPrefix(resp.DownloadURL, "/") {
		resp.DownloadURL = strings.TrimSuffix(strings.TrimRight(c.BaseURL, "/"), "/v0") + resp.DownloadURL
	}
	return &resp, nil
}

func (c *Client) GetSHUBSources() ([]*models.SHUBSource, error) {
	req, err := c.newRequest(http.MethodGet, "/shub/sources")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Sources []*models.SHUBSource `json:"sources"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get SHUB sources: %w", err)
	}
	return resp.Sources, nil
}

func (c *Client) HasToken() bool {
	return strings.TrimSpace(c.token) != ""
}

func (c *Client) Login(username, password string) (*models.LoginResponse, error) {
	var resp models.LoginResponse
	err := c.doJsonRequest(http.MethodPost, "/auth/login", &models.LoginRequest{
		Username: username,
		Password: password,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to login: %w", err)
	}
	return &resp, nil
}

func (c *Client) GetCurrentUser() (*models.RegistryUser, error) {
	req, err := c.newRequest(http.MethodGet, "/auth/me")
	if err != nil {
		return nil, err
	}
	var resp models.RegistryUser
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	return &resp, nil
}

func (c *Client) ListRegistryUsers() ([]*models.RegistryUser, error) {
	req, err := c.newRequest(http.MethodGet, "/auth/users")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Users []*models.RegistryUser `json:"users"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to list registry users: %w", err)
	}
	return resp.Users, nil
}

func (c *Client) CreateRegistryUser(request *models.CreateUserRequest) (*models.RegistryUser, error) {
	var resp models.RegistryUser
	if err := c.doJsonRequest(http.MethodPost, "/auth/users", request, &resp); err != nil {
		return nil, fmt.Errorf("failed to create registry user: %w", err)
	}
	return &resp, nil
}

func (c *Client) ListAPIKeys() ([]*models.APIKey, error) {
	req, err := c.newRequest(http.MethodGet, "/auth/api-keys")
	if err != nil {
		return nil, err
	}
	var resp struct {
		APIKeys []*models.APIKey `json:"apiKeys"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	return resp.APIKeys, nil
}

func (c *Client) CreateAPIKey(request *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	var resp models.CreateAPIKeyResponse
	if err := c.doJsonRequest(http.MethodPost, "/auth/api-keys", request, &resp); err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}
	return &resp, nil
}

func (c *Client) DeleteAPIKey(keyID string) error {
	req, err := c.newRequest(http.MethodDelete, "/auth/api-keys/"+url.PathEscape(keyID))
	if err != nil {
		return err
	}
	if err := c.doJSON(req, nil); err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}
	return nil
}

func (c *Client) GetRegistryAuthSettings() (*models.RegistryAuthSettings, error) {
	req, err := c.newRequest(http.MethodGet, "/auth/settings")
	if err != nil {
		return nil, err
	}
	var resp models.RegistryAuthSettings
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get auth settings: %w", err)
	}
	return &resp, nil
}

func (c *Client) UpdateRegistryAuthSettings(request *models.UpdateRegistryAuthSettingsRequest) (*models.RegistryAuthSettings, error) {
	var resp models.RegistryAuthSettings
	if err := c.doJsonRequest(http.MethodPut, "/auth/settings", request, &resp); err != nil {
		return nil, fmt.Errorf("failed to update auth settings: %w", err)
	}
	return &resp, nil
}

func (c *Client) GetSHUBSource(name string) (*models.SHUBSource, error) {
	req, err := c.newRequest(http.MethodGet, "/shub/sources/"+url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	var resp models.SHUBSource
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get SHUB source: %w", err)
	}
	return &resp, nil
}

func (c *Client) SetSHUBSource(name, address string) (*models.SHUBSource, error) {
	var resp models.SHUBSource
	err := c.doJsonRequest(http.MethodPut, "/shub/sources/"+url.PathEscape(name), &models.SHUBSourceUpsertRequest{
		Address: address,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to set SHUB source: %w", err)
	}
	return &resp, nil
}

func (c *Client) DeleteSHUBSource(name string) error {
	req, err := c.newRequest(http.MethodDelete, "/shub/sources/"+url.PathEscape(name))
	if err != nil {
		return err
	}
	if err := c.doJSON(req, nil); err != nil {
		return fmt.Errorf("failed to delete SHUB source: %w", err)
	}
	return nil
}

func (c *Client) PullAssetFromSource(sourceName, assetID, version string) (*models.AssetResponse, error) {
	var resp models.AssetResponse
	err := c.doJsonRequest(http.MethodPost, "/shub/sources/"+url.PathEscape(sourceName)+"/pull", &models.SHUBSourcePullRequest{
		AssetID: assetID,
		Version: version,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to pull asset from SHUB source: %w", err)
	}
	c.normalizeAssetResponse(&resp)
	return &resp, nil
}

// GetAgents returns all agents from connected registries
func (c *Client) GetAgents() ([]*models.AgentResponse, error) {
	limit := 100
	cursor := ""
	var all []*models.AgentResponse

	for {
		q := fmt.Sprintf("/agents?limit=%d", limit)
		if cursor != "" {
			q += "&cursor=" + url.QueryEscape(cursor)
		}
		req, err := c.newRequest(http.MethodGet, q)
		if err != nil {
			return nil, err
		}

		var resp models.AgentListResponse
		if err := c.doJSON(req, &resp); err != nil {
			return nil, err
		}
		for _, ag := range resp.Agents {
			all = append(all, &ag)
		}
		if resp.Metadata.NextCursor == "" {
			break
		}
		cursor = resp.Metadata.NextCursor
	}

	return all, nil
}

func (c *Client) GetAgent(name string) (*models.AgentResponse, error) {
	encName := url.PathEscape(name)
	req, err := c.newRequest(http.MethodGet, "/agents/"+encName+"/versions/latest")
	if err != nil {
		return nil, err
	}
	var resp models.AgentResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get agent by name: %w", err)
	}
	return &resp, nil
}

// GetAgentVersion returns a specific version of an agent
func (c *Client) GetAgentVersion(name, version string) (*models.AgentResponse, error) {
	encName := url.PathEscape(name)
	encVersion := url.PathEscape(version)
	req, err := c.newRequest(http.MethodGet, "/agents/"+encName+"/versions/"+encVersion)
	if err != nil {
		return nil, err
	}
	var resp models.AgentResponse
	if err := c.doJSON(req, &resp); err != nil {
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get agent by name and version: %w", err)
	}
	return &resp, nil
}

// GetPrompts returns all prompts from the registry
func (c *Client) GetPrompts() ([]*models.PromptResponse, error) {
	limit := 100
	cursor := ""
	var all []*models.PromptResponse

	for {
		q := fmt.Sprintf("/prompts?limit=%d", limit)
		if cursor != "" {
			q += "&cursor=" + url.QueryEscape(cursor)
		}
		req, err := c.newRequest(http.MethodGet, q)
		if err != nil {
			return nil, err
		}

		var resp models.PromptListResponse
		if err := c.doJSON(req, &resp); err != nil {
			return nil, err
		}
		for _, p := range resp.Prompts {
			all = append(all, &p)
		}
		if resp.Metadata.NextCursor == "" {
			break
		}
		cursor = resp.Metadata.NextCursor
	}

	return all, nil
}

// GetPrompt returns a prompt by name (latest version)
func (c *Client) GetPrompt(name string) (*models.PromptResponse, error) {
	encName := url.PathEscape(name)
	req, err := c.newRequest(http.MethodGet, "/prompts/"+encName+"/versions/latest")
	if err != nil {
		return nil, err
	}
	var resp models.PromptResponse
	if err := c.doJSON(req, &resp); err != nil {
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get prompt by name: %w", err)
	}
	return &resp, nil
}

// GetPromptVersion returns a specific version of a prompt
func (c *Client) GetPromptVersion(name, version string) (*models.PromptResponse, error) {
	encName := url.PathEscape(name)
	encVersion := url.PathEscape(version)
	req, err := c.newRequest(http.MethodGet, "/prompts/"+encName+"/versions/"+encVersion)
	if err != nil {
		return nil, err
	}
	var resp models.PromptResponse
	if err := c.doJSON(req, &resp); err != nil {
		if respErr := asHTTPStatus(err); respErr == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get prompt by name and version: %w", err)
	}
	return &resp, nil
}

// CreatePrompt creates a prompt in the registry (immediately visible)
func (c *Client) CreatePrompt(prompt *models.PromptJSON) (*models.PromptResponse, error) {
	var resp models.PromptResponse
	err := c.doJsonRequest(http.MethodPost, "/prompts", prompt, &resp)
	return &resp, err
}

// DeletePrompt deletes a prompt from the registry
func (c *Client) DeletePrompt(name, version string) error {
	encName := url.PathEscape(name)
	encVersion := url.PathEscape(version)

	req, err := c.newRequest(http.MethodDelete, "/prompts/"+encName+"/versions/"+encVersion)
	if err != nil {
		return err
	}

	return c.doJSON(req, nil)
}

// CreateSkill creates a skill in the registry (immediately visible)
func (c *Client) CreateSkill(skill *models.SkillJSON) (*models.SkillResponse, error) {
	var resp models.SkillResponse
	err := c.doJsonRequest(http.MethodPost, "/skills", skill, &resp)
	return &resp, err
}

// CreateAsset publishes a SHUB asset through the unified compatibility API.
func (c *Client) CreateAsset(request *models.AssetPublishRequest) (*models.AssetResponse, error) {
	var resp models.AssetResponse
	err := c.doJsonRequest(http.MethodPost, "/assets", request, &resp)
	c.normalizeAssetResponse(&resp)
	return &resp, err
}

// CreateAgent creates or updates an agent entry.
func (c *Client) CreateAgent(agent *models.AgentJSON) (*models.AgentResponse, error) {
	var resp models.AgentResponse
	err := c.doJsonRequest(http.MethodPost, "/agents", agent, &resp)
	return &resp, err
}

// CreateMCPServer creates or updates an MCP server entry.
func (c *Client) CreateMCPServer(server *v0.ServerJSON) (*v0.ServerResponse, error) {
	var resp v0.ServerResponse
	err := c.doJsonRequest(http.MethodPost, "/servers", server, &resp)
	return &resp, err
}

// DeleteAgent deletes an agent from the registry
func (c *Client) DeleteAgent(name, version string) error {
	encName := url.PathEscape(name)
	encVersion := url.PathEscape(version)

	req, err := c.newRequest(http.MethodDelete, "/agents/"+encName+"/versions/"+encVersion)
	if err != nil {
		return err
	}

	return c.doJSON(req, nil)
}

// DeleteSkill deletes a skill from the registry
// Note: This uses DELETE HTTP method. If the endpoint doesn't exist, it will return an error.
func (c *Client) DeleteSkill(name, version string) error {
	encName := url.PathEscape(name)
	encVersion := url.PathEscape(version)

	req, err := c.newRequest(http.MethodDelete, "/skills/"+encName+"/versions/"+encVersion)
	if err != nil {
		return err
	}

	return c.doJSON(req, nil)
}

// DeleteMCPServer deletes an MCP server from the registry by setting its status to deleted
func (c *Client) DeleteMCPServer(name, version string) error {
	encName := url.PathEscape(name)
	encVersion := url.PathEscape(version)

	req, err := c.newRequest(http.MethodDelete, "/servers/"+encName+"/versions/"+encVersion)
	if err != nil {
		return err
	}
	return c.doJSON(req, nil)
}

// Helpers to convert API errors
func asHTTPStatus(err error) int {
	if err == nil {
		return 0
	}
	errStr := err.Error()

	// Try "unexpected status: CODE ..." (unparsed JSON fallback)
	if strings.Contains(errStr, "unexpected status:") {
		parts := strings.Split(errStr, "unexpected status: ")
		if len(parts) > 1 {
			statusPart := strings.Split(parts[1], " ")[0]
			if code, parseErr := strconv.Atoi(statusPart); parseErr == nil {
				return code
			}
		}
	}

	// Try "CODE Status Text: message" (parsed API error)
	if code, parseErr := strconv.Atoi(strings.Split(errStr, " ")[0]); parseErr == nil {
		return code
	}

	if strings.Contains(errStr, "404") || strings.Contains(errStr, "Not Found") {
		return http.StatusNotFound
	}
	return 0
}

func (c *Client) normalizeAssetResponse(asset *models.AssetResponse) {
	if asset == nil || asset.Asset.Source == nil {
		return
	}
	ref := strings.TrimSpace(asset.Asset.Source.PackageRef)
	if !strings.HasPrefix(ref, "/") {
		return
	}
	if absolute := resolveRelativeURL(c.BaseURL, ref); absolute != "" {
		asset.Asset.Source.PackageRef = absolute
	}
}

func resolveRelativeURL(baseURL, ref string) string {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	relative, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ""
	}
	return base.ResolveReference(relative).String()
}

// ApplyOpts carries cross-cutting batch options for the POST /v0/apply endpoint.
type ApplyOpts struct {
	Force  bool
	DryRun bool
}

// Apply sends a multi-doc YAML body to POST /v0/apply and returns per-resource results.
// Returns an error only on request-level failures (network, 4xx from server).
// Per-resource errors are encoded in the returned results.
func (c *Client) Apply(ctx context.Context, body []byte, opts ApplyOpts) ([]kinds.Result, error) {
	u := strings.TrimRight(c.BaseURL, "/") + "/apply"
	q := url.Values{}
	if opts.Force {
		q.Set("force", "true")
	}
	if opts.DryRun {
		q.Set("dryRun", "true")
	}
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("POST /v0/apply returned %d: %s", resp.StatusCode, string(b))
	}

	var out struct {
		Results []kinds.Result `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding apply response: %w", err)
	}
	return out.Results, nil
}

// DeleteViaApply sends a DELETE /v0/apply with a YAML body and returns per-resource results.
// Mirrors Apply but uses the DELETE HTTP method.
func (c *Client) DeleteViaApply(ctx context.Context, body []byte) ([]kinds.Result, error) {
	u := strings.TrimRight(c.BaseURL, "/") + "/apply"

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DELETE /v0/apply returned %d: %s", resp.StatusCode, string(b))
	}

	var out struct {
		Results []kinds.Result `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return out.Results, nil
}

// SSEClient returns the HTTP client used for SSE requests.
func (c *Client) SSEClient() *http.Client {
	return &http.Client{
		Transport:     c.httpClient.Transport,
		CheckRedirect: c.httpClient.CheckRedirect,
		Jar:           c.httpClient.Jar,
		Timeout:       0,
	}
}

// NewSSERequest creates a request for streaming embedding indexing events.
func (c *Client) NewSSERequest(ctx context.Context, reqBody IndexRequest) (*http.Request, error) {
	req, err := c.newRequest(http.MethodPost, "/embeddings/index/stream")
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal index request: %w", err)
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Body = io.NopCloser(bytes.NewReader(body))
	return req, nil
}

// StartIndex starts a non-streaming indexing job.
func (c *Client) StartIndex(req IndexRequest) (*IndexJobResponse, error) {
	var resp IndexJobResponse
	if err := c.doJsonRequest(http.MethodPost, "/embeddings/index", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetIndexStatus fetches indexing job status by job ID.
func (c *Client) GetIndexStatus(jobID string) (*JobStatusResponse, error) {
	encJobID := url.PathEscape(jobID)
	req, err := c.newRequest(http.MethodGet, "/embeddings/index/"+encJobID)
	if err != nil {
		return nil, err
	}
	var resp JobStatusResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

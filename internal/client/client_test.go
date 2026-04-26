package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestEnsureV0Suffix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bare URL", "http://localhost:12121", "http://localhost:12121/v0"},
		{"already has /v0", "http://localhost:12121/v0", "http://localhost:12121/v0"},
		{"trailing slash", "http://localhost:12121/", "http://localhost:12121/v0"},
		{"trailing slash with v0", "http://localhost:12121/v0/", "http://localhost:12121/v0"},
		{"https URL", "https://registry.example.com", "https://registry.example.com/v0"},
		{"https with v0", "https://registry.example.com/v0", "https://registry.example.com/v0"},
		{"with port", "http://myhost:8080", "http://myhost:8080/v0"},
		{"empty string", "", "/v0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ensureV0Suffix(tt.input)
			if got != tt.want {
				t.Errorf("ensureV0Suffix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewClient_BaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantURL string
	}{
		{"empty defaults to defaultBaseURL", "", defaultBaseURL},
		{"bare URL gets /v0 appended", "http://localhost:12121", "http://localhost:12121/v0"},
		{"URL with /v0 unchanged", "http://localhost:12121/v0", "http://localhost:12121/v0"},
		{"trailing slash normalized", "http://localhost:12121/", "http://localhost:12121/v0"},
		{"custom host", "https://registry.example.com", "https://registry.example.com/v0"},
		{"custom host with /v0", "https://registry.example.com/v0", "https://registry.example.com/v0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.baseURL, "test-token")
			if c.BaseURL != tt.wantURL {
				t.Errorf("NewClient(%q, ...).BaseURL = %q, want %q", tt.baseURL, c.BaseURL, tt.wantURL)
			}
		})
	}
}

func TestExtractAPIErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			"single error message",
			`{"title":"Bad Request","status":400,"detail":"Failed to create server","errors":[{"message":"name is required"}]}`,
			"name is required",
		},
		{
			"multiple error messages",
			`{"title":"Bad Request","status":400,"detail":"Validation failed","errors":[{"message":"name is required"},{"message":"version is invalid"}]}`,
			"name is required; version is invalid",
		},
		{
			"falls back to detail when no error messages",
			`{"title":"Bad Request","status":400,"detail":"Something went wrong","errors":[]}`,
			"Something went wrong",
		},
		{
			"detail only no errors field",
			`{"title":"Internal Server Error","status":500,"detail":"Unexpected failure"}`,
			"Unexpected failure",
		},
		{
			"skips empty messages",
			`{"title":"Bad Request","status":400,"detail":"fail","errors":[{"message":""},{"message":"real error"}]}`,
			"real error",
		},
		{
			"invalid JSON returns empty",
			`not json at all`,
			"",
		},
		{
			"empty body returns empty",
			``,
			"",
		},
		{
			"no detail or errors returns empty",
			`{"title":"Bad Request","status":400}`,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAPIErrorMessage([]byte(tt.body))
			if got != tt.want {
				t.Errorf("extractAPIErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAsHTTPStatus(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
		want   int
	}{
		{"parsed API error format", "400 Bad Request: name is required", 400},
		{"unparsed fallback format", "unexpected status: 404 Not Found, {\"detail\":\"not found\"}", 404},
		{"500 parsed format", "500 Internal Server Error: something broke", 500},
		{"contains 404", "something 404 happened", 404},
		{"contains Not Found", "resource Not Found", 404},
		{"unknown error", "connection refused", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("%s", tt.errMsg)
			if got := asHTTPStatus(err); got != tt.want {
				t.Errorf("asHTTPStatus(%q) = %d, want %d", tt.errMsg, got, tt.want)
			}
		})
	}
}

func TestNewClient_Token(t *testing.T) {
	c := NewClient("http://localhost:12121", "my-secret-token")
	if c.token != "my-secret-token" {
		t.Errorf("NewClient token = %q, want %q", c.token, "my-secret-token")
	}

	c2 := NewClient("http://localhost:12121", "")
	if c2.token != "" {
		t.Errorf("NewClient empty token = %q, want empty", c2.token)
	}
}

func TestNewClient_HttpClientNotNil(t *testing.T) {
	c := NewClient("", "")
	if c.httpClient == nil {
		t.Error("NewClient httpClient should not be nil")
	}
}

func TestResolveEnvConfig(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{
			name: "prefers arctl variables",
			env: map[string]string{
				envARCTLAPIBaseURL: "http://env.example.com",
				envARCTLAPIToken:   "env-token",
				envSHUBAPIBaseURL:  "http://shub.example.com",
				envSHUBAPIToken:    "shub-token",
			},
			want: []string{"http://env.example.com", "env-token"},
		},
		{
			name: "falls back to shub variables",
			env: map[string]string{
				envSHUBAPIBaseURL: "http://shub.example.com",
				envSHUBAPIToken:   "shub-token",
			},
			want: []string{"http://shub.example.com", "shub-token"},
		},
		{
			name: "uses shub token when only arctl url is set",
			env: map[string]string{
				envARCTLAPIBaseURL: "http://env.example.com",
				envSHUBAPIToken:    "shub-token",
			},
			want: []string{"http://env.example.com", "shub-token"},
		},
		{
			name: "uses shub url when only arctl token is set",
			env: map[string]string{
				envARCTLAPIToken:  "env-token",
				envSHUBAPIBaseURL: "http://shub.example.com",
			},
			want: []string{"http://shub.example.com", "env-token"},
		},
		{
			name: "trims whitespace",
			env: map[string]string{
				envARCTLAPIBaseURL: "  http://env.example.com  ",
				envARCTLAPIToken:   "  env-token  ",
			},
			want: []string{"http://env.example.com", "env-token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getEnv := func(key string) string { return tt.env[key] }
			got := []string{}
			baseURL, token := ResolveEnvConfig(getEnv)
			got = append(got, baseURL, token)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ResolveEnvConfig() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestClientGetAssetNormalizesRelativePackageRef(t *testing.T) {
	client := NewClient("https://registry.example.com", "")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/v0/assets/arch%2Fjava-analyzer" {
			t.Fatalf("path = %q, want /v0/assets/arch%%2Fjava-analyzer", r.URL.EscapedPath())
		}
		body, err := json.Marshal(models.AssetResponse{
			Asset: models.Asset{
				ID:      "arch/java-analyzer",
				Version: "1.2.0",
				Source:  &models.AssetSource{PackageType: "tarball", PackageRef: "/v0/assets/arch%2Fjava-analyzer/versions/1.2.0/package"},
			},
		})
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	})}
	asset, err := client.GetAsset("arch/java-analyzer")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	want := "https://registry.example.com/v0/assets/arch%2Fjava-analyzer/versions/1.2.0/package"
	if got := asset.Asset.Source.PackageRef; got != want {
		t.Fatalf("package ref = %q, want %q", got, want)
	}
}

func TestClientSHUBSourceMethods(t *testing.T) {
	client := NewClient("https://registry.example.com", "")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var payload []byte
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v0/shub/sources/github-main":
			var req models.SHUBSourceUpsertRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode put request: %v", err)
			}
			var err error
			payload, err = json.Marshal(models.SHUBSource{Name: "github-main", Address: req.Address})
			if err != nil {
				t.Fatalf("marshal put response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/v0/shub/sources/github-main/pull":
			var req models.SHUBSourcePullRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode pull request: %v", err)
			}
			var err error
			payload, err = json.Marshal(models.AssetResponse{
				Asset: models.Asset{
					ID:      req.AssetID,
					Version: req.Version,
					Source:  &models.AssetSource{PackageType: "tarball", PackageRef: "/v0/assets/arch%2Fjava-analyzer/versions/1.2.0/package"},
				},
			})
			if err != nil {
				t.Fatalf("marshal pull response: %v", err)
			}
		case r.Method == http.MethodDelete && r.URL.Path == "/v0/shub/sources/github-main":
			payload = []byte(`{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v0/shub/sources":
			var err error
			payload, err = json.Marshal(map[string]any{
				"sources": []models.SHUBSource{{Name: "github-main", Address: "https://github.com/acme/skills/tree/main/skills"}},
			})
			if err != nil {
				t.Fatalf("marshal list response: %v", err)
			}
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"detail":"not found"}`))),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(payload)),
		}, nil
	})}
	source, err := client.SetSHUBSource("github-main", "https://github.com/acme/skills/tree/main/skills")
	if err != nil {
		t.Fatalf("SetSHUBSource() error = %v", err)
	}
	if source.Name != "github-main" {
		t.Fatalf("source name = %q, want github-main", source.Name)
	}

	listed, err := client.GetSHUBSources()
	if err != nil {
		t.Fatalf("GetSHUBSources() error = %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "github-main" {
		t.Fatalf("listed = %#v, want github-main", listed)
	}

	asset, err := client.PullAssetFromSource("github-main", "arch/java-analyzer", "1.2.0")
	if err != nil {
		t.Fatalf("PullAssetFromSource() error = %v", err)
	}
	want := "https://registry.example.com/v0/assets/arch%2Fjava-analyzer/versions/1.2.0/package"
	if got := asset.Asset.Source.PackageRef; got != want {
		t.Fatalf("package ref = %q, want %q", got, want)
	}

	if err := client.DeleteSHUBSource("github-main"); err != nil {
		t.Fatalf("DeleteSHUBSource() error = %v", err)
	}
}

func TestResolveRelativeURL(t *testing.T) {
	got := resolveRelativeURL("https://registry.example.com/v0", "/v0/assets/demo/versions/1.0.0/package")
	want := "https://registry.example.com/v0/assets/demo/versions/1.0.0/package"
	if got != want {
		t.Fatalf("resolveRelativeURL() = %q, want %q", got, want)
	}
}

func TestClientLoginMethod(t *testing.T) {
	client := NewClient("https://registry.example.com", "")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/v0/auth/login" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req models.LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode login request: %v", err)
		}
		if req.Username != "admin" || req.Password != "admin" {
			t.Fatalf("unexpected login payload: %#v", req)
		}
		body, err := json.Marshal(models.LoginResponse{
			Token:     "jwt-token",
			ExpiresAt: 1234567890,
			User:      models.RegistryUser{ID: "user-1", Username: "admin", Role: "admin"},
		})
		if err != nil {
			t.Fatalf("marshal login response: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	})}

	resp, err := client.Login("admin", "admin")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if resp.Token != "jwt-token" || resp.User.Username != "admin" {
		t.Fatalf("Login() = %#v, want token and admin user", resp)
	}
}

func TestClientRegistryAuthMethods(t *testing.T) {
	client := NewClient("https://registry.example.com", "api-token")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer api-token" {
			t.Fatalf("Authorization header = %q, want Bearer api-token", got)
		}

		var payload []byte
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/auth/me":
			var err error
			payload, err = json.Marshal(models.RegistryUser{ID: "user-1", Username: "alice", Role: "user"})
			if err != nil {
				t.Fatalf("marshal me response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/v0/auth/users":
			var err error
			payload, err = json.Marshal(map[string]any{
				"users": []models.RegistryUser{{ID: "user-1", Username: "alice", Role: "user"}},
			})
			if err != nil {
				t.Fatalf("marshal users response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/v0/auth/users":
			var req models.CreateUserRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode create user request: %v", err)
			}
			if req.Username != "bob" || req.Role != "user" {
				t.Fatalf("unexpected create user payload: %#v", req)
			}
			var err error
			payload, err = json.Marshal(models.RegistryUser{ID: "user-2", Username: "bob", Role: "user"})
			if err != nil {
				t.Fatalf("marshal create user response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/v0/auth/api-keys":
			var err error
			payload, err = json.Marshal(map[string]any{
				"apiKeys": []models.APIKey{{ID: "key-1", Name: "default-cli", Prefix: "ar_sk_123456"}},
			})
			if err != nil {
				t.Fatalf("marshal api keys response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/v0/auth/api-keys":
			var req models.CreateAPIKeyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode create api key request: %v", err)
			}
			if req.Name != "default-cli" {
				t.Fatalf("unexpected api key payload: %#v", req)
			}
			var err error
			payload, err = json.Marshal(models.CreateAPIKeyResponse{
				APIKey: models.APIKey{ID: "key-1", Name: "default-cli", Prefix: "ar_sk_123456"},
				Secret: "ar_sk_secret",
			})
			if err != nil {
				t.Fatalf("marshal create api key response: %v", err)
			}
		case r.Method == http.MethodDelete && r.URL.Path == "/v0/auth/api-keys/key-1":
			payload = []byte(`{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v0/auth/settings":
			var err error
			payload, err = json.Marshal(models.RegistryAuthSettings{APIKeyValidationEnabled: true})
			if err != nil {
				t.Fatalf("marshal get settings response: %v", err)
			}
		case r.Method == http.MethodPut && r.URL.Path == "/v0/auth/settings":
			var req models.UpdateRegistryAuthSettingsRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode update settings request: %v", err)
			}
			if req.APIKeyValidationEnabled {
				t.Fatalf("expected updated setting to disable validation")
			}
			var err error
			payload, err = json.Marshal(models.RegistryAuthSettings{APIKeyValidationEnabled: false})
			if err != nil {
				t.Fatalf("marshal update settings response: %v", err)
			}
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"detail":"not found"}`))),
			}, nil
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(payload)),
		}, nil
	})}

	if !client.HasToken() {
		t.Fatal("HasToken() = false, want true")
	}

	me, err := client.GetCurrentUser()
	if err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}
	if me.Username != "alice" {
		t.Fatalf("GetCurrentUser() = %#v, want alice", me)
	}

	users, err := client.ListRegistryUsers()
	if err != nil {
		t.Fatalf("ListRegistryUsers() error = %v", err)
	}
	if len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("ListRegistryUsers() = %#v, want alice", users)
	}

	user, err := client.CreateRegistryUser(&models.CreateUserRequest{Username: "bob", Password: "secret", Role: "user"})
	if err != nil {
		t.Fatalf("CreateRegistryUser() error = %v", err)
	}
	if user.Username != "bob" {
		t.Fatalf("CreateRegistryUser() = %#v, want bob", user)
	}

	keys, err := client.ListAPIKeys()
	if err != nil {
		t.Fatalf("ListAPIKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0].Name != "default-cli" {
		t.Fatalf("ListAPIKeys() = %#v, want default-cli", keys)
	}

	key, err := client.CreateAPIKey(&models.CreateAPIKeyRequest{Name: "default-cli"})
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}
	if key.Secret != "ar_sk_secret" {
		t.Fatalf("CreateAPIKey() = %#v, want ar_sk_secret", key)
	}

	if err := client.DeleteAPIKey("key-1"); err != nil {
		t.Fatalf("DeleteAPIKey() error = %v", err)
	}

	settings, err := client.GetRegistryAuthSettings()
	if err != nil {
		t.Fatalf("GetRegistryAuthSettings() error = %v", err)
	}
	if !settings.APIKeyValidationEnabled {
		t.Fatalf("GetRegistryAuthSettings() = %#v, want enabled", settings)
	}

	updated, err := client.UpdateRegistryAuthSettings(&models.UpdateRegistryAuthSettingsRequest{APIKeyValidationEnabled: false})
	if err != nil {
		t.Fatalf("UpdateRegistryAuthSettings() error = %v", err)
	}
	if updated.APIKeyValidationEnabled {
		t.Fatalf("UpdateRegistryAuthSettings() = %#v, want disabled", updated)
	}
}

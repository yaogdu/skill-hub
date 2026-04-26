package auth

// Method represents the authentication method used
type Method string

const (
	// GitHub OAuth authentication (access token)
	MethodGitHubAT Method = "github-at"
	// GitHub Actions OIDC authentication
	MethodGitHubOIDC Method = "github-oidc"
	// Generic OIDC authentication
	MethodOIDC Method = "oidc"
	// DNS-based public/private key authentication
	MethodDNS Method = "dns"
	// HTTP-based public/private key authentication
	MethodHTTP Method = "http"
	// Username/password login for the built-in registry UI and API.
	MethodPassword Method = "password"
	// Long-lived API key authentication for CLI and automation.
	MethodAPIKey Method = "api-key"
	// No authentication - should only be used for local development and testing
	MethodNone Method = "none"
)

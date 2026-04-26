package models

import "time"

type RegistryUser struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string       `json:"token"`
	ExpiresAt int64        `json:"expiresAt"`
	User      RegistryUser `json:"user"`
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"`
}

type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

type CreateAPIKeyRequest struct {
	Name string `json:"name"`
}

type CreateAPIKeyResponse struct {
	APIKey APIKey `json:"apiKey"`
	Secret string `json:"secret"`
}

type RegistryAuthSettings struct {
	APIKeyValidationEnabled bool      `json:"apiKeyValidationEnabled"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

type UpdateRegistryAuthSettingsRequest struct {
	APIKeyValidationEnabled bool `json:"apiKeyValidationEnabled"`
}

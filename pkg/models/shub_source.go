package models

import "time"

// SHUBSource configures a named remote location that the registry can pull SHUB
// assets from when a client explicitly names a source or when automatic fallback
// source resolution is enabled on registry misses.
type SHUBSource struct {
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	Description string    `json:"description,omitempty"`
	Provider    string    `json:"provider,omitempty"`
	BuiltIn     bool      `json:"builtIn,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// SHUBSourceUpsertRequest updates or creates a named SHUB source.
type SHUBSourceUpsertRequest struct {
	Address string `json:"address"`
}

// SHUBSourcePullRequest asks the registry to pull an asset from a configured
// SHUB source and mirror it into registry-managed storage.
type SHUBSourcePullRequest struct {
	AssetID string `json:"assetId"`
	Version string `json:"version,omitempty"`
}

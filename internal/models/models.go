// Package models holds microstore's plain domain structs. They are
// storage-agnostic — no bbolt (or any persistence) imports live here — and are
// serialized as JSON throughout, both into bbolt and across the MCP surface.
//
// Entities split into two groups:
//   - Live entities are fetched from GitHub on every use and never persisted.
//   - Persisted entities (InstalledApp, Config) are the only state kept in bbolt.
package models

import "time"

// --- Live entities (fetched from GitHub, never persisted) ---

// Catalog is the manifest document fetched from Config.ManifestURL.
type Catalog struct {
	Apps      []ManifestEntry `json:"apps"`
	Templates []Template      `json:"templates"`
}

// ManifestEntry is a minimal catalog listing for an installable micro-app;
// richer metadata is read live from GitHub via RepoInfo/Release.
type ManifestEntry struct {
	Repo        string `json:"repo"` // "owner/name"
	Category    string `json:"category"`
	DisplayName string `json:"display_name,omitempty"`
}

// Template is a catalog-listed starting point for scaffolding a new micro-app.
type Template struct {
	Repo        string `json:"repo"` // "owner/name"
	Ref         string `json:"ref"`  // branch or tag
	Name        string `json:"name"`
	Description string `json:"description"`
}

// RepoInfo is the subset of a GitHub repository we surface.
type RepoInfo struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	Stars       int    `json:"stars"`
}

// Release is a GitHub release with its downloadable assets.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []Asset   `json:"assets"`
}

// Asset is a single downloadable file attached to a Release.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

// --- Persisted entities (bbolt) ---

// InstalledApp records one installed binary, keyed by its repo slug. Field tags
// are stable: old records persist on disk across upgrades, so decoding must stay
// backward-compatible (additive fields, tolerate missing keys).
type InstalledApp struct {
	Repo        string    `json:"repo"` // "owner/name" — the bbolt key
	DisplayName string    `json:"display_name,omitempty"`
	Category    string    `json:"category,omitempty"`
	Version     string    `json:"version"` // installed release tag
	AssetName   string    `json:"asset_name"`
	Path        string    `json:"path"` // absolute path of the placed binary
	SHA256      string    `json:"sha256"`
	Size        int64     `json:"size"`
	InstalledAt time.Time `json:"installed_at"`
	SourceURL   string    `json:"source_url"`
}

// Config is the singleton store configuration persisted under a well-known key.
type Config struct {
	ManifestURL string `json:"manifest_url"`
	InstallDir  string `json:"install_dir"`
}

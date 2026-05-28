// Package github is microstore's outbound HTTPS client for GitHub. It fetches
// the live catalog manifest, repository info, releases/assets, and tarballs, and
// downloads release assets. It performs no persistence — callers receive plain
// domain models. This is client I/O only (like git or go install); microstore
// runs no service of its own.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"techthos.net/microstore/internal/models"
)

// TokenEnv is the environment variable holding an optional GitHub token. When
// set, requests are authenticated (higher rate limits, private repos).
const TokenEnv = "MICROSTORE_GITHUB_TOKEN"

const defaultAPIBase = "https://api.github.com"

// Client talks to GitHub over HTTPS.
type Client struct {
	http    *http.Client
	apiBase string
	token   string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying *http.Client (used in tests).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithBaseURL overrides the GitHub API base URL (used in tests). No trailing slash.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.apiBase = strings.TrimRight(u, "/") }
}

// WithToken sets the auth token explicitly, overriding the environment default.
func WithToken(t string) Option { return func(c *Client) { c.token = t } }

// New builds a Client. By default it reads the token from TokenEnv and uses a
// 30-second timeout.
func New(opts ...Option) *Client {
	c := &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		apiBase: defaultAPIBase,
		token:   os.Getenv(TokenEnv),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// FetchCatalog fetches and decodes the manifest at manifestURL (a raw JSON URL).
// An empty URL is a clear, non-network error.
func (c *Client) FetchCatalog(ctx context.Context, manifestURL string) (models.Catalog, error) {
	if strings.TrimSpace(manifestURL) == "" {
		return models.Catalog{}, fmt.Errorf("manifest URL not set")
	}
	var cat models.Catalog
	if err := c.getJSON(ctx, manifestURL, &cat); err != nil {
		return models.Catalog{}, fmt.Errorf("fetch catalog: %w", err)
	}
	return cat, nil
}

// RepoInfo fetches the repository metadata for repo ("owner/name").
func (c *Client) RepoInfo(ctx context.Context, repo string) (models.RepoInfo, error) {
	owner, name, err := parseRepo(repo)
	if err != nil {
		return models.RepoInfo{}, err
	}
	var dto repoDTO
	url := fmt.Sprintf("%s/repos/%s/%s", c.apiBase, owner, name)
	if err := c.getJSON(ctx, url, &dto); err != nil {
		return models.RepoInfo{}, fmt.Errorf("repo info %s: %w", repo, err)
	}
	return dto.toModel(), nil
}

// Releases fetches all releases for repo, newest-first (GitHub's order).
func (c *Client) Releases(ctx context.Context, repo string) ([]models.Release, error) {
	owner, name, err := parseRepo(repo)
	if err != nil {
		return nil, err
	}
	var dtos []releaseDTO
	url := fmt.Sprintf("%s/repos/%s/%s/releases", c.apiBase, owner, name)
	if err := c.getJSON(ctx, url, &dtos); err != nil {
		return nil, fmt.Errorf("releases %s: %w", repo, err)
	}
	releases := make([]models.Release, len(dtos))
	for i, d := range dtos {
		releases[i] = d.toModel()
	}
	return releases, nil
}

// LatestRelease returns the newest non-prerelease release for repo.
func (c *Client) LatestRelease(ctx context.Context, repo string) (models.Release, error) {
	releases, err := c.Releases(ctx, repo)
	if err != nil {
		return models.Release{}, err
	}
	for _, r := range releases {
		if !r.Prerelease {
			return r, nil
		}
	}
	return models.Release{}, fmt.Errorf("no published (non-prerelease) release for %s", repo)
}

// Download streams the content at url into w, returning the number of bytes written.
func (c *Client) Download(ctx context.Context, url string, w io.Writer) (int64, error) {
	resp, err := c.do(ctx, url, "")
	if err != nil {
		return 0, fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return 0, fmt.Errorf("download %s: %w", url, err)
	}
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, fmt.Errorf("download %s: %w", url, err)
	}
	return n, nil
}

// Tarball opens the source tarball for repo at ref. The caller must close the
// returned reader.
func (c *Client) Tarball(ctx context.Context, repo, ref string) (io.ReadCloser, error) {
	owner, name, err := parseRepo(repo)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("tarball %s: ref is empty", repo)
	}
	url := fmt.Sprintf("%s/repos/%s/%s/tarball/%s", c.apiBase, owner, name, ref)
	resp, err := c.do(ctx, url, "")
	if err != nil {
		return nil, fmt.Errorf("tarball %s@%s: %w", repo, ref, err)
	}
	if err := checkStatus(resp); err != nil {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("tarball %s@%s: %w", repo, ref, err)
	}
	return resp.Body, nil
}

func (c *Client) do(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return c.http.Do(req)
}

func (c *Client) getJSON(ctx context.Context, url string, out any) error {
	resp, err := c.do(ctx, url, "application/vnd.github+json")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if ts, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return fmt.Errorf("github rate limit exceeded; resets at %s", time.Unix(ts, 0).UTC().Format(time.RFC3339))
		}
		return fmt.Errorf("github rate limit exceeded")
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("unexpected status %s: %s", resp.Status, bytes.TrimSpace(body))
}

func parseRepo(repo string) (owner, name string, err error) {
	parts := strings.Split(strings.TrimSpace(repo), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo slug %q, want \"owner/name\"", repo)
	}
	return parts[0], parts[1], nil
}

// --- GitHub API DTOs (wire shapes mapped into clean domain models) ---

type repoDTO struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	Stars       int    `json:"stargazers_count"`
}

func (d repoDTO) toModel() models.RepoInfo {
	return models.RepoInfo{
		FullName:    d.FullName,
		Description: d.Description,
		Homepage:    d.Homepage,
		Stars:       d.Stars,
	}
}

type assetDTO struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

type releaseDTO struct {
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	PublishedAt time.Time  `json:"published_at"`
	Prerelease  bool       `json:"prerelease"`
	Assets      []assetDTO `json:"assets"`
}

func (d releaseDTO) toModel() models.Release {
	assets := make([]models.Asset, len(d.Assets))
	for i, a := range d.Assets {
		assets[i] = models.Asset{
			Name:        a.Name,
			DownloadURL: a.DownloadURL,
			Size:        a.Size,
			ContentType: a.ContentType,
		}
	}
	return models.Release{
		TagName:     d.TagName,
		Name:        d.Name,
		Body:        d.Body,
		PublishedAt: d.PublishedAt,
		Prerelease:  d.Prerelease,
		Assets:      assets,
	}
}

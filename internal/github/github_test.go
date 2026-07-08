package github_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"techthos.net/binzaar/internal/github"
)

const (
	repoJSON = `{"full_name":"techthos/binzaar","description":"a local store","homepage":"https://example.test","stargazers_count":7}`

	releasesJSON = `[
	 {"tag_name":"v2.0.0-rc1","name":"rc","body":"pre","published_at":"2026-05-10T00:00:00Z","prerelease":true,"assets":[]},
	 {"tag_name":"v1.2.0","name":"stable","body":"notes","published_at":"2026-05-01T00:00:00Z","prerelease":false,
	  "assets":[{"name":"binzaar_linux_amd64","browser_download_url":"https://example.test/dl/bin","size":1234,"content_type":"application/octet-stream"}]}
	]`

	catalogJSON = `{"apps":[{"repo":"techthos/binzaar","category":"tools","display_name":"binzaar"}],
	                "templates":[{"repo":"techthos/template","ref":"main","name":"base","description":"bare"}]}`
)

// newClient wires a github.Client to a test server handling the GitHub API
// shapes binzaar uses.
func newClient(t *testing.T, opts ...github.Option) (*github.Client, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/techthos/binzaar", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, repoJSON)
	})
	mux.HandleFunc("/repos/techthos/binzaar/releases", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, releasesJSON)
	})
	mux.HandleFunc("/repos/techthos/template/tarball/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "TARBALL-BYTES")
	})
	mux.HandleFunc("/catalog.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, catalogJSON)
	})
	mux.HandleFunc("/dl/bin", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "BINARY")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	allOpts := append([]github.Option{
		github.WithBaseURL(srv.URL),
		github.WithHTTPClient(srv.Client()),
		github.WithToken(""),
	}, opts...)
	return github.New(allOpts...), srv
}

func TestFetchCatalog(t *testing.T) {
	t.Parallel()
	c, srv := newClient(t)
	cat, err := c.FetchCatalog(context.Background(), srv.URL+"/catalog.json")
	if err != nil {
		t.Fatalf("FetchCatalog: %v", err)
	}
	if len(cat.Apps) != 1 || cat.Apps[0].Repo != "techthos/binzaar" || cat.Apps[0].DisplayName != "binzaar" {
		t.Errorf("apps = %+v", cat.Apps)
	}
	if len(cat.Templates) != 1 || cat.Templates[0].Ref != "main" {
		t.Errorf("templates = %+v", cat.Templates)
	}
}

func TestFetchCatalogEmptyURL(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t)
	_, err := c.FetchCatalog(context.Background(), "  ")
	if err == nil || !strings.Contains(err.Error(), "manifest URL not set") {
		t.Fatalf("err = %v, want \"manifest URL not set\"", err)
	}
}

func TestRepoInfo(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t)
	info, err := c.RepoInfo(context.Background(), "techthos/binzaar")
	if err != nil {
		t.Fatalf("RepoInfo: %v", err)
	}
	if info.FullName != "techthos/binzaar" || info.Stars != 7 || info.Homepage != "https://example.test" {
		t.Errorf("info = %+v", info)
	}
}

func TestRepoInfoInvalidSlug(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t)
	if _, err := c.RepoInfo(context.Background(), "not-a-slug"); err == nil {
		t.Fatal("expected error for invalid slug")
	}
}

func TestReleasesNewestFirst(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t)
	rels, err := c.Releases(context.Background(), "techthos/binzaar")
	if err != nil {
		t.Fatalf("Releases: %v", err)
	}
	if len(rels) != 2 || rels[0].TagName != "v2.0.0-rc1" || !rels[0].Prerelease {
		t.Fatalf("releases = %+v", rels)
	}
	if rels[1].PublishedAt != time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC) {
		t.Errorf("published = %v", rels[1].PublishedAt)
	}
	if len(rels[1].Assets) != 1 || rels[1].Assets[0].DownloadURL != "https://example.test/dl/bin" {
		t.Errorf("assets = %+v", rels[1].Assets)
	}
}

func TestLatestReleaseSkipsPrerelease(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t)
	rel, err := c.LatestRelease(context.Background(), "techthos/binzaar")
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if rel.TagName != "v1.2.0" || rel.Prerelease {
		t.Errorf("latest = %+v, want v1.2.0 non-prerelease", rel)
	}
}

func TestDownload(t *testing.T) {
	t.Parallel()
	c, srv := newClient(t)
	var buf strings.Builder
	n, err := c.Download(context.Background(), srv.URL+"/dl/bin", &buf)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if buf.String() != "BINARY" || n != int64(len("BINARY")) {
		t.Errorf("got %q (%d bytes)", buf.String(), n)
	}
}

func TestTarball(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t)
	rc, err := c.Tarball(context.Background(), "techthos/template", "main")
	if err != nil {
		t.Fatalf("Tarball: %v", err)
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read tarball: %v", err)
	}
	if string(b) != "TARBALL-BYTES" {
		t.Errorf("tarball = %q", b)
	}
}

func TestRateLimitError(t *testing.T) {
	t.Parallel()
	reset := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	c := github.New(github.WithBaseURL(srv.URL), github.WithHTTPClient(srv.Client()), github.WithToken(""))
	_, err := c.RepoInfo(context.Background(), "techthos/binzaar")
	if err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("err = %v, want rate-limit error", err)
	}
}

func TestAuthHeaderSentWhenTokenSet(t *testing.T) {
	t.Parallel()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, repoJSON)
	}))
	t.Cleanup(srv.Close)
	c := github.New(github.WithBaseURL(srv.URL), github.WithHTTPClient(srv.Client()), github.WithToken("secret"))
	if _, err := c.RepoInfo(context.Background(), "techthos/binzaar"); err != nil {
		t.Fatalf("RepoInfo: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q, want \"Bearer secret\"", gotAuth)
	}
}

package registry_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"techthos.net/binzaar/internal/registry"
)

const validCatalog = `{"apps":[{"repo":"acme/widget","category":"tools"}],"templates":[]}`

func writeCatalog(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	return path
}

func TestLoad(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string // "" means: point at a missing file
		wantErr string // substring; "" means success
	}{
		{name: "valid catalog", content: validCatalog},
		{name: "empty object is a valid catalog", content: `{}`},
		{name: "missing file", content: "", wantErr: "read catalog"},
		{name: "invalid JSON", content: `{nope`, wantErr: "not a valid catalog document"},
		{name: "wrong shape", content: `{"apps":"not-a-list"}`, wantErr: "not a valid catalog document"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "absent.json")
			if tc.content != "" {
				path = writeCatalog(t, tc.content)
			}
			data, err := registry.Load(path)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if string(data) != tc.content {
				t.Errorf("data = %q, want the raw file bytes %q", data, tc.content)
			}
		})
	}
}

func TestHandler(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
		wantBody   string // substring; "" means don't check
	}{
		{name: "GET /catalog.json", method: http.MethodGet, target: "/catalog.json", wantStatus: http.StatusOK, wantBody: "acme/widget"},
		{name: "GET / serves the same document", method: http.MethodGet, target: "/", wantStatus: http.StatusOK, wantBody: "acme/widget"},
		{name: "unknown path", method: http.MethodGet, target: "/other", wantStatus: http.StatusNotFound},
		{name: "non-GET method", method: http.MethodPost, target: "/catalog.json", wantStatus: http.StatusMethodNotAllowed},
	}
	path := writeCatalog(t, validCatalog)
	h := registry.Handler(path)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body = %q, want substring %q", rec.Body.String(), tc.wantBody)
			}
			if tc.wantStatus == http.StatusOK {
				if got := rec.Header().Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", got)
				}
			}
		})
	}
}

func TestHandlerRereadsFilePerRequest(t *testing.T) {
	t.Parallel()
	path := writeCatalog(t, validCatalog)
	srv := httptest.NewServer(registry.Handler(path))
	defer srv.Close()

	get := func() (int, string) {
		t.Helper()
		resp, err := http.Get(srv.URL + "/catalog.json")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		return resp.StatusCode, string(body)
	}

	if code, body := get(); code != http.StatusOK || !strings.Contains(body, "acme/widget") {
		t.Fatalf("first GET = %d %q, want 200 with acme/widget", code, body)
	}

	// An edit is visible on the next request, without a restart.
	updated := `{"apps":[{"repo":"acme/gadget","category":"tools"}]}`
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("rewrite catalog: %v", err)
	}
	if code, body := get(); code != http.StatusOK || !strings.Contains(body, "acme/gadget") {
		t.Fatalf("GET after edit = %d %q, want 200 with acme/gadget", code, body)
	}

	// A file that becomes invalid after startup yields 500.
	if err := os.WriteFile(path, []byte(`{broken`), 0o644); err != nil {
		t.Fatalf("break catalog: %v", err)
	}
	if code, _ := get(); code != http.StatusInternalServerError {
		t.Fatalf("GET after corruption = %d, want 500", code)
	}
}

func TestServeRejectsInvalidCatalogBeforeListening(t *testing.T) {
	t.Parallel()
	path := writeCatalog(t, `{broken`)
	// An invalid file must fail fast; Serve returns before ever listening, so
	// the bogus address is never bound (binding it would be a different error).
	err := registry.Serve("127.0.0.1:0", path)
	if err == nil || !strings.Contains(err.Error(), "not a valid catalog document") {
		t.Fatalf("err = %v, want catalog validation error", err)
	}
}

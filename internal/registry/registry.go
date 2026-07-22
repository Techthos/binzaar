// Package registry implements UC 15: serving a local catalog JSON file over
// plain HTTP so anyone can host a custom registry that other binzaar
// instances consume via Config.ManifestURL. It is a leaf service — it depends
// only on internal/models and touches neither bbolt nor GitHub.
package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"techthos.net/binzaar/internal/models"
)

// Load reads and validates the catalog file at path: the file must exist and
// parse as a models.Catalog. It returns the raw bytes ready to serve.
func Load(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog %q: %w", path, err)
	}
	var c models.Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("catalog %q is not a valid catalog document: %w", path, err)
	}
	return data, nil
}

// Handler serves the catalog file at path on GET /catalog.json and GET /.
// The file is re-read on every request so edits are picked up without a
// restart; a file that becomes unreadable or invalid yields HTTP 500. Other
// paths return 404 and non-GET methods 405.
func Handler(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/catalog.json" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := Load(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
}

// Serve validates the catalog file once (fail fast before listening), then
// serves it on addr until the listener fails. It blocks for the life of the
// server.
func Serve(addr, path string) error {
	if _, err := Load(path); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "serving %s on %s (GET /catalog.json)\n", path, addr)
	//nolint:gosec // a local registry server; operators pick the listen addr.
	return http.ListenAndServe(addr, Handler(path))
}

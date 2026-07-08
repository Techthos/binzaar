package models_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"techthos.net/binzaar/internal/models"
)

// roundTrip marshals a value, unmarshals it back into the same type, and
// re-marshals — asserting the JSON is byte-identical. This exercises the json
// tags without reflect.DeepEqual or an external diff dependency.
func roundTrip[T any](t *testing.T, in T) {
	t.Helper()
	first, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out T
	if err := json.Unmarshal(first, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	second, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("round-trip mismatch:\n first: %s\nsecond: %s", first, second)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	published := time.Date(2026, 5, 1, 12, 30, 0, 0, time.UTC)
	installed := time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)

	asset := models.Asset{
		Name:        "binzaar_linux_amd64",
		DownloadURL: "https://example.test/dl/binzaar_linux_amd64",
		Size:        1234567,
		ContentType: "application/octet-stream",
	}

	t.Run("Catalog", func(t *testing.T) {
		t.Parallel()
		roundTrip(t, models.Catalog{
			Apps: []models.ManifestEntry{
				{Repo: "techthos/binzaar", Category: "tools", DisplayName: "binzaar", Bin: "store"},
				{Repo: "techthos/foo", Category: "tools"},
			},
			Templates: []models.Template{
				{Repo: "techthos/template", Ref: "main", Name: "base", Description: "bare scaffold"},
			},
		})
	})

	t.Run("Release", func(t *testing.T) {
		t.Parallel()
		roundTrip(t, models.Release{
			TagName:     "v1.2.0",
			Name:        "v1.2.0",
			Body:        "notes",
			PublishedAt: published,
			Prerelease:  false,
			Assets:      []models.Asset{asset},
		})
	})

	t.Run("RepoInfo", func(t *testing.T) {
		t.Parallel()
		roundTrip(t, models.RepoInfo{
			FullName:    "techthos/binzaar",
			Description: "a local store",
			Homepage:    "https://example.test",
			Stars:       42,
		})
	})

	t.Run("InstalledApp", func(t *testing.T) {
		t.Parallel()
		roundTrip(t, models.InstalledApp{
			Repo:        "techthos/binzaar",
			DisplayName: "binzaar",
			Category:    "tools",
			Bin:         "store",
			Version:     "v1.2.0",
			AssetName:   asset.Name,
			Path:        "/home/op/.local/share/binzaar/bin/binzaar",
			SHA256:      "deadbeef",
			Size:        asset.Size,
			InstalledAt: installed,
			SourceURL:   asset.DownloadURL,
		})
	})

	t.Run("Config", func(t *testing.T) {
		t.Parallel()
		roundTrip(t, models.Config{
			ManifestURL:      "https://example.test/catalog.json",
			InstallDir:       "/home/op/.local/share/binzaar/bin",
			LastSection:      "installed",
			SidebarCollapsed: true,
		})
	})
}

// TestPersistedJSONTags locks the on-disk wire format for the persisted
// entities: these tags must stay stable so records written by older builds keep
// decoding after an upgrade.
func TestPersistedJSONTags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   any
		want string
	}{
		{
			name: "InstalledApp",
			in: models.InstalledApp{
				Repo:        "techthos/binzaar",
				Version:     "v1.0.0",
				AssetName:   "a",
				Path:        "/p",
				SHA256:      "h",
				Size:        1,
				InstalledAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				SourceURL:   "u",
			},
			want: `{"repo":"techthos/binzaar","version":"v1.0.0","asset_name":"a","path":"/p","sha256":"h","size":1,"installed_at":"2026-01-01T00:00:00Z","source_url":"u"}`,
		},
		{
			name: "Config",
			in:   models.Config{ManifestURL: "u", InstallDir: "d"},
			want: `{"manifest_url":"u","install_dir":"d"}`,
		},
		{
			// View-prefs are additive and omitempty, so old records (without them)
			// keep decoding; when set, they carry these stable tags.
			name: "ConfigWithUIPrefs",
			in:   models.Config{ManifestURL: "u", InstallDir: "d", LastSection: "config", SidebarCollapsed: true},
			want: `{"manifest_url":"u","install_dir":"d","last_section":"config","sidebar_collapsed":true}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("JSON =\n  %s\nwant\n  %s", got, tc.want)
			}
		})
	}
}

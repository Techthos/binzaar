package db_test

import (
	"path/filepath"
	"testing"

	"techthos.net/microstore/internal/db"
	"techthos.net/microstore/internal/models"
)

func newStore(t *testing.T) *db.Store {
	t.Helper()
	s, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestConfigLoadDefaultsOnFreshDB(t *testing.T) {
	t.Parallel()
	got, err := newStore(t).Config().Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if want := db.DefaultConfig(); got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
	if got.ManifestURL == "" {
		t.Error("default ManifestURL is empty, want the curated catalog URL")
	}
	if got.InstallDir == "" {
		t.Error("default InstallDir is empty, want a managed directory")
	}
}

func TestConfigSaveThenLoad(t *testing.T) {
	t.Parallel()
	repo := newStore(t).Config()
	want := models.Config{
		ManifestURL: "https://example.test/catalog.json",
		InstallDir:  "/opt/microstore/bin",
	}
	if err := repo.Save(want); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	got, err := repo.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestConfigSaveOverwrites(t *testing.T) {
	t.Parallel()
	repo := newStore(t).Config()
	if err := repo.Save(models.Config{ManifestURL: "a", InstallDir: "x"}); err != nil {
		t.Fatalf("first Save(): %v", err)
	}
	want := models.Config{ManifestURL: "b", InstallDir: "y"}
	if err := repo.Save(want); err != nil {
		t.Fatalf("second Save(): %v", err)
	}
	got, err := repo.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

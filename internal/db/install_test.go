package db_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"techthos.net/microstore/internal/db"
	"techthos.net/microstore/internal/models"
)

func sampleApp(repo string) models.InstalledApp {
	return models.InstalledApp{
		Repo:        repo,
		DisplayName: repo,
		Category:    "tools",
		Version:     "v1.0.0",
		AssetName:   "bin_linux_amd64",
		Path:        "/install/" + repo,
		SHA256:      "deadbeef",
		Size:        1024,
		InstalledAt: time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC),
		SourceURL:   "https://example.test/" + repo,
	}
}

// equalJSON compares two values by their JSON encoding, sidestepping time.Time
// equality pitfalls.
func equalJSON(t *testing.T, got, want any) {
	t.Helper()
	g, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	w, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	if string(g) != string(w) {
		t.Errorf("mismatch:\n got: %s\nwant: %s", g, w)
	}
}

func TestInstallGetNotFound(t *testing.T) {
	t.Parallel()
	_, err := newStore(t).Installs().Get("techthos/missing")
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("Get() err = %v, want ErrNotFound", err)
	}
}

func TestInstallSaveThenGet(t *testing.T) {
	t.Parallel()
	repo := newStore(t).Installs()
	want := sampleApp("techthos/microstore")
	if err := repo.Save(want); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	got, err := repo.Get(want.Repo)
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	equalJSON(t, *got, want)
}

func TestInstallSaveOverwrites(t *testing.T) {
	t.Parallel()
	repo := newStore(t).Installs()
	if err := repo.Save(sampleApp("techthos/microstore")); err != nil {
		t.Fatalf("first Save(): %v", err)
	}
	want := sampleApp("techthos/microstore")
	want.Version = "v2.0.0"
	if err := repo.Save(want); err != nil {
		t.Fatalf("second Save(): %v", err)
	}
	got, err := repo.Get(want.Repo)
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.Version != "v2.0.0" {
		t.Errorf("Version = %q, want v2.0.0", got.Version)
	}
}

func TestInstallListAlphabetical(t *testing.T) {
	t.Parallel()
	repo := newStore(t).Installs()
	for _, slug := range []string{"zeta/z", "alpha/a", "mid/m"} {
		if err := repo.Save(sampleApp(slug)); err != nil {
			t.Fatalf("Save(%q): %v", slug, err)
		}
	}
	got, err := repo.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	want := []string{"alpha/a", "mid/m", "zeta/z"}
	if len(got) != len(want) {
		t.Fatalf("List() len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Repo != w {
			t.Errorf("List()[%d].Repo = %q, want %q", i, got[i].Repo, w)
		}
	}
}

func TestInstallListEmpty(t *testing.T) {
	t.Parallel()
	got, err := newStore(t).Installs().List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List() = %v, want empty", got)
	}
}

func TestInstallDelete(t *testing.T) {
	t.Parallel()
	repo := newStore(t).Installs()
	app := sampleApp("techthos/microstore")
	if err := repo.Save(app); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	if err := repo.Delete(app.Repo); err != nil {
		t.Fatalf("Delete(): %v", err)
	}
	if _, err := repo.Get(app.Repo); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("Get() after delete err = %v, want ErrNotFound", err)
	}
}

func TestInstallDeleteUnknown(t *testing.T) {
	t.Parallel()
	err := newStore(t).Installs().Delete("techthos/missing")
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("Delete() err = %v, want ErrNotFound", err)
	}
}

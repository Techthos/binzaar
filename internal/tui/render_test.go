package tui

import (
	"strings"
	"testing"
	"time"

	"techthos.net/microstore/internal/app"
	"techthos.net/microstore/internal/models"
)

func TestCatalogRow(t *testing.T) {
	t.Parallel()
	got := catalogRow(models.ManifestEntry{Repo: "o/a", Category: "tools", DisplayName: "Alpha"})
	// Name is plain; repo and category carry color-tag markup around the value.
	if got[0] != "Alpha" {
		t.Errorf("name = %q, want Alpha", got[0])
	}
	if !strings.Contains(got[1], "o/a") || !strings.Contains(got[2], "tools") {
		t.Errorf("catalogRow = %v, want repo+category present", got)
	}
	// Falls back to repo when DisplayName is empty.
	got = catalogRow(models.ManifestEntry{Repo: "o/b", Category: "x"})
	if got[0] != "o/b" {
		t.Errorf("name fallback = %q, want o/b", got[0])
	}
}

func TestInstalledRow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	ia := models.InstalledApp{Repo: "o/a", Version: "v1", InstalledAt: now.Add(-2 * time.Hour)}
	got := installedRow(ia, "", now)
	// Lists show the timestamp relative to now (absolute lives in the detail pane).
	if got[0] != "o/a" || got[1] != "v1" || got[2] != "2h ago" {
		t.Errorf("installedRow = %v", got)
	}
	// Empty status renders the muted "not checked" cell; "ok" renders verified.
	if !strings.Contains(got[3], "not checked") {
		t.Errorf("empty verify = %q, want not checked", got[3])
	}
	if !strings.Contains(installedRow(ia, "ok", now)[3], "verified") {
		t.Errorf("ok verify not rendered: %q", installedRow(ia, "ok", now)[3])
	}
}

func TestRelTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, emDash},
		{"just now", now.Add(-10 * time.Second), "just now"},
		{"minutes", now.Add(-5 * time.Minute), "5m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-48 * time.Hour), "2d ago"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := relTime(tc.t, now); got != tc.want {
				t.Errorf("relTime = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInstalledDetailText(t *testing.T) {
	t.Parallel()
	ia := models.InstalledApp{
		Repo: "o/a", Version: "v1", AssetName: "bin-linux", Path: "/bin/x",
		InstalledAt: time.Date(2026, 5, 28, 9, 30, 0, 0, time.UTC),
	}
	txt := installedDetailText(ia, "ok")
	// The detail pane carries the absolute timestamp and the full record.
	for _, want := range []string{"o/a", "v1", "2026-05-28 09:30", "bin-linux", "/bin/x", "verified"} {
		if !strings.Contains(txt, want) {
			t.Errorf("installedDetailText missing %q in:\n%s", want, txt)
		}
	}
}

func TestVerifyCell(t *testing.T) {
	t.Parallel()
	cases := map[string]string{"ok": "verified", "mismatch": "mismatch", "missing": "missing", "": "not checked"}
	for status, want := range cases {
		if got := verifyCell(status); !strings.Contains(got, want) {
			t.Errorf("verifyCell(%q) = %q, want it to contain %q", status, got, want)
		}
	}
}

func TestFilterApps(t *testing.T) {
	t.Parallel()
	apps := []models.ManifestEntry{
		{Repo: "o/alpha", Category: "tools", DisplayName: "Alpha"},
		{Repo: "o/beta", Category: "games", DisplayName: "Beta"},
		{Repo: "o/altimeter", Category: "tools", DisplayName: "Altimeter"},
	}
	if got := filterApps(apps, "", ""); len(got) != 3 {
		t.Errorf("no filter = %d, want 3", len(got))
	}
	if got := filterApps(apps, "", "tools"); len(got) != 2 {
		t.Errorf("category = %d, want 2", len(got))
	}
	if got := filterApps(apps, "alt", ""); len(got) != 1 || got[0].Repo != "o/altimeter" {
		t.Errorf("query = %v", got)
	}
	if got := filterApps(apps, "al", "tools"); len(got) != 2 {
		t.Errorf("combined = %d, want 2", len(got))
	}
}

func TestDistinctCategories(t *testing.T) {
	t.Parallel()
	apps := []models.ManifestEntry{
		{Category: "tools"}, {Category: "games"}, {Category: "tools"}, {Category: ""},
	}
	got := distinctCategories(apps)
	if strings.Join(got, ",") != "games,tools" {
		t.Errorf("distinctCategories = %v, want [games tools]", got)
	}
}

func TestSidebarItems(t *testing.T) {
	t.Parallel()
	items := sidebarItems(
		map[string]int{pageCatalog: 12, pageInstalled: 3},
		map[string]bool{pageInstalled: true},
	)
	if len(items) != len(sectionOrder) {
		t.Fatalf("got %d items, want %d", len(items), len(sectionOrder))
	}
	// Numbered shortcuts and the count appear; Installed carries the attention badge.
	if !strings.Contains(items[0], "1") || !strings.Contains(items[0], "Catalog") || !strings.Contains(items[0], "12") {
		t.Errorf("catalog item = %q", items[0])
	}
	if !strings.Contains(items[1], "Installed") || !strings.Contains(items[1], "●") {
		t.Errorf("installed item missing badge: %q", items[1])
	}
	// No badge where none requested.
	if strings.Contains(items[2], "●") {
		t.Errorf("new-app item should have no badge: %q", items[2])
	}
}

func TestStatusHints(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		pageCatalog:   "install",
		pageInstalled: "uninstall",
		pageNew:       "scaffold",
		pageConfig:    "save",
	}
	for page, want := range cases {
		h := statusHints(page)
		if !strings.Contains(h, want) {
			t.Errorf("statusHints(%q) missing %q in %q", page, want, h)
		}
		if !strings.Contains(h, "help") {
			t.Errorf("statusHints(%q) must end in the help hint: %q", page, h)
		}
	}
}

func TestHelpText(t *testing.T) {
	t.Parallel()
	got := helpText()
	for _, want := range []string{"jump to section", "toggle sidebar", "filter", "uninstall", "save", "quit"} {
		if !strings.Contains(got, want) {
			t.Errorf("helpText missing %q", want)
		}
	}
}

func TestPathWarningText(t *testing.T) {
	t.Parallel()
	st := app.PathStatus{
		InstallDir:  "/home/u/.local/share/microstore/bin",
		ProfilePath: "/home/u/.bashrc",
		ExportLine:  `export PATH="$PATH:/home/u/.local/share/microstore/bin"`,
	}
	got := pathWarningText(st)
	for _, want := range []string{st.InstallDir, st.ProfilePath, st.ExportLine, "not on your PATH"} {
		if !strings.Contains(got, want) {
			t.Errorf("pathWarningText missing %q in:\n%s", want, got)
		}
	}
}

func TestSections(t *testing.T) {
	t.Parallel()
	// The navigable sections are the four sidebar entries; Detail is not one.
	if isSection(pageCatalog) != true || isSection("detail") != false {
		t.Errorf("isSection wrong: catalog=%v detail=%v", isSection(pageCatalog), isSection("detail"))
	}
	if sectionIndex(pageConfig) != 3 {
		t.Errorf("sectionIndex(config) = %d, want 3", sectionIndex(pageConfig))
	}
	if sectionIndex("nope") != -1 {
		t.Errorf("unknown section index should be -1")
	}
	if screenTitle(pageNew) != "New App" {
		t.Errorf("screenTitle(new) = %q", screenTitle(pageNew))
	}
}

func TestFilterInstalled(t *testing.T) {
	t.Parallel()
	apps := []models.InstalledApp{
		{Repo: "o/alpha", Version: "v1"},
		{Repo: "o/beta", Version: "v2"},
	}
	if got := filterInstalled(apps, ""); len(got) != 2 {
		t.Errorf("empty query = %d, want 2", len(got))
	}
	if got := filterInstalled(apps, "alph"); len(got) != 1 || got[0].Repo != "o/alpha" {
		t.Errorf("repo query = %v", got)
	}
	if got := filterInstalled(apps, "V2"); len(got) != 1 || got[0].Repo != "o/beta" {
		t.Errorf("version query (case-insensitive) = %v", got)
	}
}

func TestDetailText(t *testing.T) {
	t.Parallel()
	d := app.AppDetails{
		Repo:   models.RepoInfo{FullName: "o/app", Description: "desc", Stars: 5},
		Latest: models.Release{TagName: "v1.0.0", Assets: []models.Asset{{Name: "bin", Size: 2048}}},
	}
	txt := detailText(d)
	for _, want := range []string{"o/app", "desc", "v1.0.0", "bin", "Not installed"} {
		if !strings.Contains(txt, want) {
			t.Errorf("detailText missing %q in:\n%s", want, txt)
		}
	}
	d.Installed = &models.InstalledApp{Version: "v0.9.0"}
	if !strings.Contains(detailText(d), "v0.9.0") {
		t.Errorf("detailText should show installed version")
	}
}

func TestHumanSize(t *testing.T) {
	t.Parallel()
	tests := map[int64]string{0: "0 B", 512: "512 B", 2048: "2.0 KB", 1048576: "1.0 MB"}
	for in, want := range tests {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

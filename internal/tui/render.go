// Package tui is microstore's terminal UI, built on rivo/tview. It owns the one
// *tview.Application and is a thin view layer: all data flows through the
// injected Service (the internal/app use-case layer). Network and disk work runs
// off the event loop and is funnelled back via QueueUpdateDraw; render logic
// lives in the pure helpers below so it is testable without the Application.
package tui

import (
	"fmt"
	"sort"
	"strings"

	"techthos.net/microstore/internal/app"
	"techthos.net/microstore/internal/models"
)

// Page identifiers within the root Pages primitive.
const (
	pageCatalog   = "catalog"
	pageDetail    = "detail"
	pageInstalled = "installed"
	pageNew       = "new"
	pageConfig    = "config"
	pageAssetPick = "assetpick"
	pageConfirm   = "confirm"
)

// pageOrder is the Tab-cycle order of the primary screens.
var pageOrder = []string{pageCatalog, pageDetail, pageInstalled, pageNew, pageConfig}

func nextPage(cur string) string { return cyclePage(cur, +1) }
func prevPage(cur string) string { return cyclePage(cur, -1) }

func cyclePage(cur string, delta int) string {
	for i, p := range pageOrder {
		if p == cur {
			n := (i + delta + len(pageOrder)) % len(pageOrder)
			return pageOrder[n]
		}
	}
	return pageCatalog
}

var catalogHeader = []string{"Name", "Repo", "Category"}

func catalogRow(e models.ManifestEntry) []string {
	name := e.DisplayName
	if name == "" {
		name = e.Repo
	}
	return []string{name, e.Repo, e.Category}
}

var installedHeader = []string{"Repo", "Version", "Installed", "Verify"}

func installedRow(a models.InstalledApp, verify string) []string {
	if verify == "" {
		verify = "-"
	}
	return []string{a.Repo, a.Version, a.InstalledAt.Format("2006-01-02"), verify}
}

// filterApps applies the catalog's in-memory search: free-text on name/repo
// (case-insensitive) and/or an exact category. Empty filters match everything.
func filterApps(apps []models.ManifestEntry, query, category string) []models.ManifestEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []models.ManifestEntry
	for _, e := range apps {
		if category != "" && !strings.EqualFold(e.Category, category) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.DisplayName), q) && !strings.Contains(strings.ToLower(e.Repo), q) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// distinctCategories returns the sorted unique categories present in apps.
func distinctCategories(apps []models.ManifestEntry) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range apps {
		if e.Category != "" && !seen[e.Category] {
			seen[e.Category] = true
			out = append(out, e.Category)
		}
	}
	sort.Strings(out)
	return out
}

// detailText renders an app's details as dynamic-color markup for the TextView.
func detailText(d app.AppDetails) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[::b]%s[::-]", orDash(d.Repo.FullName))
	if d.Repo.Stars > 0 {
		fmt.Fprintf(&b, "   [yellow]★ %d[-]", d.Repo.Stars)
	}
	b.WriteString("\n")
	if d.Repo.Description != "" {
		fmt.Fprintf(&b, "%s\n", d.Repo.Description)
	}
	if d.Repo.Homepage != "" {
		fmt.Fprintf(&b, "[blue]%s[-]\n", d.Repo.Homepage)
	}
	b.WriteString("\n")

	if d.Latest.TagName == "" {
		b.WriteString("[gray]No published release.[-]\n")
	} else {
		fmt.Fprintf(&b, "[::b]Latest:[::-] %s", d.Latest.TagName)
		if !d.Latest.PublishedAt.IsZero() {
			fmt.Fprintf(&b, "  (%s)", d.Latest.PublishedAt.Format("2006-01-02"))
		}
		b.WriteString("\n[::b]Assets:[::-]\n")
		if len(d.Latest.Assets) == 0 {
			b.WriteString("  [gray](none)[-]\n")
		}
		for _, a := range d.Latest.Assets {
			fmt.Fprintf(&b, "  - %s [gray](%s)[-]\n", a.Name, humanSize(a.Size))
		}
	}

	b.WriteString("\n")
	if d.Installed != nil {
		fmt.Fprintf(&b, "[green]Installed:[-] %s\n", d.Installed.Version)
	} else {
		b.WriteString("[gray]Not installed.[-]\n")
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

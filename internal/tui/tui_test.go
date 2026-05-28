package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"techthos.net/microstore/internal/app"
	"techthos.net/microstore/internal/install"
	"techthos.net/microstore/internal/models"
)

type fakeSvc struct {
	apps []models.ManifestEntry
}

func (f fakeSvc) ListCatalog(context.Context) ([]models.ManifestEntry, error) { return f.apps, nil }
func (fakeSvc) AppDetails(context.Context, string) (app.AppDetails, error) {
	return app.AppDetails{}, nil
}
func (fakeSvc) ListInstalled() ([]models.InstalledApp, error) { return nil, nil }
func (fakeSvc) Install(context.Context, string, string, string, bool) (models.InstalledApp, error) {
	return models.InstalledApp{}, nil
}
func (fakeSvc) Update(context.Context, string) (app.UpdateResult, error) {
	return app.UpdateResult{}, nil
}
func (fakeSvc) Uninstall(string) error { return nil }
func (fakeSvc) Verify(string) (install.VerifyStatus, error) {
	return install.VerifyOK, nil
}
func (fakeSvc) ListTemplates(context.Context) ([]models.Template, error) { return nil, nil }
func (fakeSvc) Scaffold(context.Context, string, string, string, bool) (app.ScaffoldResult, error) {
	return app.ScaffoldResult{}, nil
}
func (fakeSvc) GetConfig() (models.Config, error) { return models.Config{}, nil }
func (fakeSvc) SetConfig(models.Config) error     { return nil }

func TestNewBuildsPages(t *testing.T) {
	t.Parallel()
	a := New(fakeSvc{})
	for _, p := range []string{pageCatalog, pageDetail, pageInstalled, pageNew, pageConfig} {
		if !a.pages.HasPage(p) {
			t.Errorf("missing page %q", p)
		}
	}
	if front, _ := a.pages.GetFrontPage(); front != pageCatalog {
		t.Errorf("front page = %q, want catalog", front)
	}
}

func TestNavigationCyclesAcrossPages(t *testing.T) {
	t.Parallel()
	a := New(fakeSvc{})
	a.switchTo(nextPage(pageCatalog))
	if front, _ := a.pages.GetFrontPage(); front != pageDetail {
		t.Errorf("after Tab from catalog, front = %q, want detail", front)
	}
}

func screenText(sim tcell.SimulationScreen) string {
	cells, w, h := sim.GetContents()
	var b strings.Builder
	for i := 0; i < w*h; i++ {
		for _, r := range cells[i].Runes {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestRendersAndQuitsHeadless(t *testing.T) {
	a := New(fakeSvc{apps: []models.ManifestEntry{{Repo: "o/a", Category: "tools", DisplayName: "Alpha"}}})

	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatalf("sim init: %v", err)
	}
	sim.SetSize(120, 40)
	a.Application().SetScreen(sim)

	// Capture each rendered frame on the event-loop goroutine (inside the
	// after-draw hook); never read the screen from the test goroutine. The
	// first frames can be blank (pre-layout), so drain until content appears.
	frames := make(chan string, 64)
	a.Application().SetAfterDrawFunc(func(tcell.Screen) {
		select {
		case frames <- screenText(sim):
		default:
		}
	})

	done := make(chan error, 1)
	go func() { done <- a.Run() }()

	deadline := time.After(5 * time.Second)
	for found := false; !found; {
		select {
		case txt := <-frames:
			if strings.Contains(txt, "Search") {
				found = true
			}
		case <-deadline:
			a.Application().Stop()
			t.Fatal("no catalog 'Search' label rendered within 5s")
		}
	}

	a.Application().Stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Stop")
	}
}

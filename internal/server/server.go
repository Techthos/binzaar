// Package server implements binzaar's Model Context Protocol server using
// github.com/mark3labs/mcp-go. It is transport-agnostic: construction and
// registration never reference a transport (cmd selects stdio). Every tool and
// resource delegates to the internal/app use-case layer. User/input failures are
// returned as tool error results (nil error); only unexpected failures bubble up
// as protocol errors.
//
// CRUD tool results additionally carry an interactive gadget widget as a
// per-call embedded ui:// HTML resource (community mcp-ui convention — see
// widgets.go); hosts that don't render it ignore the extra content block.
package server

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"techthos.net/binzaar/internal/app"
	"techthos.net/binzaar/internal/models"
)

type handler struct {
	app *app.Service
}

// New builds the binzaar MCP server with all tools and resources registered.
func New(svc *app.Service, name, version string) *server.MCPServer {
	s := server.NewMCPServer(
		name, version,
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, false),
		server.WithRecovery(),
		server.WithLogging(),
	)
	h := &handler{app: svc}
	h.registerTools(s)
	h.registerResources(s)
	return s
}

// --- tool inputs (json tags bind request arguments via NewTypedToolHandler;
// the advertised schema is declared explicitly at registration) ---

type repoInput struct {
	Repo string `json:"repo"`
}

type searchInput struct {
	Query    string `json:"query"`
	Category string `json:"category"`
}

type installInput struct {
	Repo            string `json:"repo"`
	Version         string `json:"version"`
	Asset           string `json:"asset"`
	AllowUnverified bool   `json:"allow_unverified"`
}

type scaffoldInput struct {
	TemplateRepo string `json:"template_repo"`
	TargetDir    string `json:"target_dir"`
	Ref          string `json:"ref"`
	Force        bool   `json:"force"`
}

type configureMCPInput struct {
	Repo string `json:"repo"`
	Dir  string `json:"dir"`
}

// --- tool outputs ---

// catalogRow is a ManifestEntry decorated with install state — the catalog
// table widget renders it as the "status" badge, and the model can read it to
// know what is already installed.
type catalogRow struct {
	models.ManifestEntry
	Status string `json:"status"` // "installed" | "available"
}

type catalogOutput struct {
	Apps []catalogRow `json:"apps"`
}

type releasesOutput struct {
	Releases []models.Release `json:"releases"`
}

type installedListOutput struct {
	Installed []models.InstalledApp `json:"installed"`
}

type installOutput struct {
	Installed models.InstalledApp `json:"installed"`
}

type removedOutput struct {
	Removed bool `json:"removed"`
}

type verifyOutput struct {
	Status string `json:"status"`
}

type templatesOutput struct {
	Templates []models.Template `json:"templates"`
}

type configureMCPOutput struct {
	Result app.MCPConfigResult `json:"result"`
}

type configInput struct {
	ManifestURL string `json:"manifest_url"`
	InstallDir  string `json:"install_dir"`
}

type configOutput struct {
	Config models.Config `json:"config"`
}

// configErrorsOutput carries field-level validation errors for the config form
// widget (rendered inline under the offending fields).
type configErrorsOutput struct {
	Errors map[string]string `json:"errors"`
}

// toolErr surfaces a use-case error to the model as a tool error result so it can
// react (set the manifest URL, pick another asset, etc.).
func toolErr(err error) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(err.Error()), nil
}

// nz coalesces a nil slice to an empty one so JSON output is [] not null.
func nz[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// decorateApps joins catalog entries with the tracked installs so each row
// carries its install status.
func (h *handler) decorateApps(apps []models.ManifestEntry) ([]catalogRow, error) {
	installed, err := h.app.ListInstalled()
	if err != nil {
		return nil, err
	}
	have := make(map[string]struct{}, len(installed))
	for _, ia := range installed {
		have[ia.Repo] = struct{}{}
	}
	rows := make([]catalogRow, 0, len(apps))
	for _, e := range apps {
		status := "available"
		if _, ok := have[e.Repo]; ok {
			status = "installed"
		}
		rows = append(rows, catalogRow{ManifestEntry: e, Status: status})
	}
	return rows, nil
}

// catalogRows fetches the live catalog decorated with install status.
func (h *handler) catalogRows(ctx context.Context) ([]catalogRow, error) {
	apps, err := h.app.ListCatalog(ctx)
	if err != nil {
		return nil, err
	}
	return h.decorateApps(apps)
}

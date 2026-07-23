// Interactive UI: gadget-built widgets embedded per call in tool results,
// following the community mcp-ui convention rendered by hosts like LibreChat —
// each result carries a self-contained HTML document as an embedded ui://
// resource with a unique URI and the call's data baked in (InitialData).
// Actions target the normal model-visible tools; in an mcp-ui host the click
// becomes a follow-up turn where the model runs the tool, and gadget's runtime
// falls back to the mcp-ui postMessage protocol automatically when no MCP Apps
// host answers — posting each action as a prompt-type message carrying the
// \uievent envelope (UI Interaction Protocol v1), which protocol-aware hosts
// render as an event chip ("You clicked: …") instead of a fake user message.
// Hosts that render neither simply ignore the extra content block.
package server

import (
	"fmt"
	"log"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/techthos/gadget"
	"techthos.net/binzaar/internal/models"
)

// uiURI returns a fresh widget URI — mcp-ui hosts key renders by URI, so every
// render must be unique.
func uiURI(kind string) string {
	return fmt.Sprintf("ui://binzaar/%s/%d", kind, time.Now().UnixNano())
}

// embedUI appends w's rendered document to res as an embedded ui:// resource.
// The UI is progressive enhancement: the JSON result stands alone, so widget
// build/render failures are logged (stderr — stdout is the protocol channel)
// and never fail the tool.
func embedUI(res *mcp.CallToolResult, w gadget.Widget, err error) *mcp.CallToolResult {
	if err != nil {
		log.Printf("server: build widget: %v", err)
		return res
	}
	doc, err := w.Document()
	if err != nil {
		log.Printf("server: render widget: %v", err)
		return res
	}
	res.Content = append(res.Content, mcp.NewEmbeddedResource(mcp.TextResourceContents{
		URI:      w.Descriptor().URI,
		MIMEType: "text/html",
		Text:     doc,
	}))
	return res
}

// catalogWidget builds the catalog table with this call's rows baked in.
func catalogWidget(rows []catalogRow) (gadget.Widget, error) {
	data, err := gadget.RowsOf(rows)
	if err != nil {
		return nil, err
	}
	return &gadget.Table{
		URI:         uiURI("catalog"),
		Title:       "Catalog",
		RowsKey:     "apps", // matches catalogOutput's JSON key
		RowID:       "repo",
		Filterable:  true,
		PageSize:    10,
		InitialData: map[string]any{"apps": data},
		Columns: []gadget.Column{
			gadget.Text("display_name", "Name"),
			gadget.Text("repo", "Repository"),
			gadget.Text("category", "Category"),
			gadget.Text("description", "Description"),
			gadget.Badge("status", "Status", map[string]gadget.BadgeVariant{
				"installed": gadget.BadgeSuccess,
				"available": gadget.BadgeNeutral,
			}),
			gadget.ActionsColumn(gadget.Action{
				Label:   "Install",
				Tool:    "install_app",
				Args:    map[string]gadget.ArgSource{"repo": gadget.FromRow("repo")},
				Variant: gadget.VariantPrimary,
			}),
		},
		Empty: gadget.EmptyState{Title: "No apps", Body: "The catalog manifest lists no apps."},
	}, nil
}

// installedWidget builds the installed table with this call's rows baked in.
func installedWidget(installed []models.InstalledApp) (gadget.Widget, error) {
	data, err := gadget.RowsOf(installed)
	if err != nil {
		return nil, err
	}
	return &gadget.Table{
		URI:         uiURI("installed"),
		Title:       "Installed",
		RowsKey:     "installed", // matches installedListOutput's JSON key
		RowID:       "repo",
		Filterable:  true,
		InitialData: map[string]any{"installed": data},
		Columns: []gadget.Column{
			gadget.Text("repo", "Repository"),
			gadget.Text("version", "Version"),
			gadget.Text("category", "Category"),
			gadget.Date("installed_at", "Installed", "date"),
			gadget.Text("path", "Path"),
			gadget.ActionsColumn(
				gadget.Action{
					Label:   "Update",
					Tool:    "update_app",
					Args:    map[string]gadget.ArgSource{"repo": gadget.FromRow("repo")},
					Variant: gadget.VariantPrimary,
				},
				gadget.Action{
					Label:   "Uninstall",
					Tool:    "uninstall_app",
					Args:    map[string]gadget.ArgSource{"repo": gadget.FromRow("repo")},
					Confirm: "Remove the binary and its install record?",
					Variant: gadget.VariantDanger,
				},
			),
		},
		Empty: gadget.EmptyState{Title: "Nothing installed", Body: "Install apps from the catalog."},
	}, nil
}

// templatesWidget builds the templates table with this call's rows baked in.
func templatesWidget(templates []models.Template) (gadget.Widget, error) {
	data, err := gadget.RowsOf(templates)
	if err != nil {
		return nil, err
	}
	return &gadget.Table{
		URI:         uiURI("templates"),
		Title:       "Templates",
		RowsKey:     "templates", // matches templatesOutput's JSON key
		RowID:       "repo",
		InitialData: map[string]any{"templates": data},
		Columns: []gadget.Column{
			gadget.Text("name", "Name"),
			gadget.Text("repo", "Repository"),
			gadget.Text("ref", "Ref"),
			gadget.Text("description", "Description"),
		},
		Empty: gadget.EmptyState{Title: "No templates", Body: "The catalog manifest lists no templates."},
	}, nil
}

// configWidget builds the store-configuration form prefilled with the current
// values baked in.
func configWidget(cfg models.Config) (gadget.Widget, error) {
	return &gadget.Form{
		URI:   uiURI("config"),
		Title: "Store configuration",
		InitialData: map[string]any{"values": map[string]any{
			"manifest_url": cfg.ManifestURL,
			"install_dir":  cfg.InstallDir,
		}},
		Fields: []gadget.Field{
			{
				Name:        "manifest_url",
				Label:       "Manifest URL",
				Description: "Raw JSON URL of the catalog manifest. Empty leaves the current value unchanged.",
				Placeholder: "https://raw.githubusercontent.com/owner/repo/main/catalog.json",
			},
			{
				Name:        "install_dir",
				Label:       "Install directory",
				Description: "Directory where installed binaries are placed. Empty leaves the current value unchanged.",
			},
		},
		Submit: gadget.SubmitSpec{
			Tool:           "set_config",
			Label:          "Save",
			SuccessMessage: "Configuration saved.",
		},
	}, nil
}

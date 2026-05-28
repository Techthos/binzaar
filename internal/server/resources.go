package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *handler) registerResources(s *server.MCPServer) {
	s.AddResource(mcp.NewResource("catalog://list", "Catalog apps",
		mcp.WithResourceDescription("The current catalog app entries (live)."),
		mcp.WithMIMEType("application/json")),
		h.catalogResource)

	s.AddResource(mcp.NewResource("installed://list", "Installed apps",
		mcp.WithResourceDescription("The tracked installs (from bbolt)."),
		mcp.WithMIMEType("application/json")),
		h.installedResource)

	s.AddResource(mcp.NewResource("templates://list", "Templates",
		mcp.WithResourceDescription("The manifest's templates (live)."),
		mcp.WithMIMEType("application/json")),
		h.templatesResource)
}

func (h *handler) catalogResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	apps, err := h.app.ListCatalog(ctx)
	if err != nil {
		return nil, err
	}
	return jsonResource(req.Params.URI, catalogOutput{Apps: nz(apps)})
}

func (h *handler) installedResource(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	list, err := h.app.ListInstalled()
	if err != nil {
		return nil, err
	}
	return jsonResource(req.Params.URI, installedListOutput{Installed: nz(list)})
}

func (h *handler) templatesResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	tmpls, err := h.app.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	return jsonResource(req.Params.URI, templatesOutput{Templates: nz(tmpls)})
}

func jsonResource(uri string, v any) ([]mcp.ResourceContents, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal resource %s: %w", uri, err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{URI: uri, MIMEType: "application/json", Text: string(data)},
	}, nil
}

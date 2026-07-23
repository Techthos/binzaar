---
description: How to build the MCP server in internal/server using github.com/mark3labs/mcp-go — server construction, tool/resource/prompt registration, transports, middleware, conventions, and the interactive UI (gadget widget) requirement for CRUD tools.
paths: internal/server/**
---

# MCP Server — `mark3labs/mcp-go`

Code under `internal/server/` implements this project's [Model Context Protocol](https://modelcontextprotocol.io) server using **`github.com/mark3labs/mcp-go`**. Follow these conventions.

Two import paths only:

```go
import (
    "github.com/mark3labs/mcp-go/mcp"     // protocol types + tool/result builders
    "github.com/mark3labs/mcp-go/server"  // MCPServer, transports, options, middleware
)
```

## Construction

Build the server with `server.NewMCPServer(name, version, opts...)`. It is **transport-agnostic** — construction and registration never reference a transport. Enable only the capabilities the server actually uses, and always enable recovery so a panicking handler can't crash the process:

```go
func New(name, version string) *server.MCPServer {
    return server.NewMCPServer(name, version,
        server.WithToolCapabilities(true),       // we expose tools
        server.WithResourceCapabilities(true, true), // (subscribe, listChanged) — only if used
        server.WithPromptCapabilities(true),     // only if used
        server.WithRecovery(),                   // recover panics in handlers
        server.WithLogging(),
    )
}
```

Keep construction and registration in `internal/server`; keep transport selection and process lifecycle in `cmd/`. `main` stays thin.

## Tools

### Schema-based tools (simple args)

Define the tool with `mcp.NewTool` + `mcp.With*` option builders, then register with `s.AddTool(tool, handler)`. Mark required params with `mcp.Required()`; constrain with `mcp.Enum(...)`, defaults with `mcp.DefaultBool(...)`, etc.

```go
tool := mcp.NewTool("calculate",
    mcp.WithDescription("Perform basic arithmetic"),
    mcp.WithString("operation", mcp.Required(),
        mcp.Enum("add", "subtract", "multiply", "divide"),
        mcp.Description("The operation to perform")),
    mcp.WithNumber("x", mcp.Required(), mcp.Description("First number")),
    mcp.WithNumber("y", mcp.Required(), mcp.Description("Second number")),
)
s.AddTool(tool, handleCalculate)
```

Handler signature is `func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)`. Extract args with the typed accessors — `req.RequireString("operation")` / `req.RequireFloat("x")` / `req.RequireBool(...)` (return an error if missing/wrong type), or `req.GetString("k", default)` for optional values.

### Prefer typed handlers for non-trivial input

When a tool takes more than one or two args, define an input struct with `jsonschema` tags and use `mcp.WithInputSchema[T]()` / `mcp.WithOutputSchema[T]()`, then wrap the handler:

- `mcp.NewStructuredToolHandler(fn)` — `fn(ctx, req, args T) (R, error)`; input is validated and bound, output `R` is auto-serialized.
- `mcp.NewTypedToolHandler(fn)` — `fn(ctx, req, args T) (*mcp.CallToolResult, error)`; build the result yourself.

```go
type SearchRequest struct {
    Query      string   `json:"query" jsonschema:"required,description=Search text"`
    Categories []string `json:"categories"`
    Limit      int      `json:"limit" jsonschema:"description=Max results"`
}

searchTool := mcp.NewTool("search_products",
    mcp.WithDescription("Search the product catalog"),
    mcp.WithInputSchema[SearchRequest](),
    mcp.WithOutputSchema[SearchResponse](),
)
s.AddTool(searchTool, mcp.NewStructuredToolHandler(searchProductsHandler))

func searchProductsHandler(ctx context.Context, req mcp.CallToolRequest, args SearchRequest) (SearchResponse, error) {
    // args is already validated; return a typed value.
}
```

### Results & error semantics — important

Distinguish the two failure modes:

- **Tool-level / user-facing failures** (bad input, business-rule failure): return `mcp.NewToolResultError("message"), nil`. Return value, **`nil` error** — this surfaces to the model as an error result it can react to.
- **Protocol/transport failures** (something the model can't fix): return `nil, err`.

Build success results with: `mcp.NewToolResultText(...)`, `mcp.NewToolResultJSON(v)`, or `mcp.NewToolResultStructured(v, fallbackText)` (structured output + plain-text fallback for older clients).

## Interactive UI for CRUD tools — embedded gadget widgets

Every CRUD tool (create/read/update/delete over a domain entity) also ships an **interactive UI
version** — a widget built with `github.com/techthos/gadget` — not just a text/JSON result.
**Invoke the `gadget-mcp-ui` skill before writing any widget code**; the skill and its
`reference.md` are the source of truth for the gadget API — do not restate or improvise it here.

Widgets follow the **MCP Apps extension** ([`io.modelcontextprotocol/ui`](https://modelcontextprotocol.io/extensions/apps/overview),
spec version `2026-01-26`): a self-contained HTML document tagged with the **MCP Apps HTML profile**
(`text/html;profile=mcp-app`) that the host renders in a sandboxed iframe and drives through the
standard **App Bridge** (`@modelcontextprotocol/ext-apps`) over `postMessage`. Spec-canonically the
document is a **pre-registered `ui://` template resource** that tools link via
`_meta.ui.resourceUri` and the host fetches with `resources/read`; **this server uses gadget's
embedded-per-call variant** — each tool result carries the document inline as an embedded `ui://`
resource with the call's data baked in — which gadget drives through the same App Bridge. Either
way, do **not** invent a custom UI-event channel and do **not** inject chat prompts (no
`postMessage`-a-prompt fallback): interactions flow back only as the standard MCP Apps JSON-RPC
methods below.

- Build the widget **per call** with the data baked in (`InitialData`) and a **unique `ui://` URI
  per render**; append the rendered `Document()` to the tool result's `content` as an embedded
  resource (`mcp.NewEmbeddedResource(mcp.TextResourceContents{URI, MIMEType: "text/html;profile=mcp-app", Text})`)
  **after** the text content block. The non-UI result (text block + `structuredContent`) must always
  stand alone; widget build/render failures are logged to stderr and never fail the tool.
- **Table** for list/read output, **Form** for create/update input (prefill via baked `values`,
  inline field errors under `errors` keyed by field name).
- Actions and submits target the **normal model-visible tools**; a widget click/submit dispatches a
  standard **`tools/call`** over the App Bridge that the host runs directly against this server
  (links use **`ui/open-link`**, iframe height is applied via **`ui/notifications/size-changed`**).
- **In-place refresh is the standard tool-result push — not a chat prompt, not a static snapshot.**
  When a widget's `tools/call` completes, the host sends that result back to the same widget via
  **`ui/notifications/tool-result`**, and the gadget runtime re-renders the widget from the result's
  `structuredContent`: a table repaints its rows when `structuredContent` carries that table's
  **`rowsKey`** (the fresh rows array), a form re-applies fields from its **`prefillKey`** (and
  inline errors from its `errorsKey`). So a mutating tool must return the **refreshed collection
  under the target widget's key** (e.g. an install action surfaced on a `rowsKey: "apps"` catalog
  table returns `{"apps": <refreshed rows>}`), or the visible widget goes stale even though the tool
  succeeded. The host drives the re-render off the returned result — the iframe is neither mutated by
  our code nor left as a frozen snapshot, and it never re-posts a prompt. Also embed a freshly
  rendered widget in the result as a fallback for hosts that render result widgets rather than
  patching in place.
- **Mutating results carry a human status line as the text block, not raw JSON.** The gadget runtime
  shows a table/form's first text content block as its status banner, so build mutating results with
  `mcp.NewToolResultStructured(payload, "Installed owner/name v1.2.3")`: `payload` (carrying the
  widget key above) lands in `structuredContent`, and the short sentence is what the user sees.
  **Read/list tools that embed a widget use the same status-line form** (e.g.
  `mcp.NewToolResultStructured(catalogOutput{Apps: rows}, "5 apps in the catalog.")`): a raw-JSON
  text block flashes visibly in an MCP Apps host until the widget paints over it, so keep the JSON in
  `structuredContent` (what the model reads) and make the text block a short sentence. Reserve
  `mcp.NewToolResultJSON` for tools with **no** embedded widget (e.g. `app_details`, `list_releases`),
  where the JSON text block is the only output.
- **Destructive actions (delete)**: table row actions with `Action.Confirm` — the sandboxed
  iframe has no native `confirm()`/`alert()`.
- New or changed widgets and tools are product-surface changes → update `docs/SPECIFICATIONS.md`
  in the **same commit** (`specification-rules.md`).

Configure widget actions declaratively through gadget's `Action`/`Submit` API (which name the
target tool and its argument sources) — the gadget runtime speaks the MCP Apps App Bridge protocol
for you. Never hand-author postMessage payloads or text-sentinel envelopes in widget HTML. The full
host-side contract (method surface, rendering, refresh loop) is documented in
`docs/mcp-apps-host-guide.md`.

## Resources

Register with `s.AddResource(resource, handler)`. Use URI templates for parameterized resources:

```go
s.AddResource(
    mcp.NewResource("file://{path}", "File Content",
        mcp.WithResourceDescription("Read file contents"),
        mcp.WithMIMEType("text/plain")),
    handleFileContent,
)

func handleFileContent(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
    // return []mcp.ResourceContents{ mcp.TextResourceContents{URI, MIMEType, Text} }
}
```

Validate and sanitize any path/URI input (clean the path, reject `..` traversal, confine to an allowed base dir) before touching the filesystem.

## Prompts

Register with `s.AddPrompt(prompt, handler)` using `mcp.NewPrompt(...)` and return `mcp.NewGetPromptResult(...)` from the handler.

## Transports

The server object is shared across transports. Select transport in `cmd/` (e.g. via an `MCP_TRANSPORT` env var), not in `internal/server`:

- **stdio** (default for local/CLI use): `server.ServeStdio(s)`. Never write logs to **stdout** on stdio — that stream is the protocol channel. Log to **stderr**.
- **Streamable HTTP** (preferred network transport): `server.NewStreamableHTTPServer(s, opts...).Start(":8080")`. Options: `server.WithEndpointPath("/mcp")`, `server.WithHeartbeatInterval(30*time.Second)`, `server.WithStateLess(bool)`, `server.WithSessionIdleTTL(...)`.
- **SSE** (legacy): `server.NewSSEServer(s).Start(":8080")`. Prefer Streamable HTTP for new work.
- **In-process** (`client.NewInProcessClient(s)`): use this in tests instead of spawning a subprocess.

## Middleware & context

- Cross-cutting concerns (auth, rate limiting, caching, metrics) go in middleware, not in handlers: `s.AddToolMiddleware(func(next server.ToolHandler) server.ToolHandler { ... })` and `s.AddResourceMiddleware(...)`. Each wraps `next` and may short-circuit by returning early.
- Inside a handler, reach the server via `server.ServerFromContext(ctx)`; push async updates with `mcpServer.SendNotificationToClient(ctx, "event", payload)`.
- Always thread the incoming `ctx` through downstream calls (DB, HTTP) for cancellation.

## Testing

Test handlers through the in-process client (`client.NewInProcessClient(s)`) so the full registration + (de)serialization path is exercised without a transport. Initialize, then `CallTool`, then assert on `result.Content` (extract text via `mcp.AsTextContent(result.Content[0])`).

## Checklist for new functionality

1. One tool/resource/prompt per file (or a small cohesive group) under `internal/server`.
2. Always set `mcp.WithDescription` and describe every parameter — the model relies on these.
3. Use typed handlers + `jsonschema`-tagged structs once input is non-trivial.
4. User/input errors → `NewToolResultError(...), nil`; infrastructure errors → `nil, err`.
5. Enable the matching capability in `NewMCPServer` and confirm `WithRecovery()` is set.
6. Add an in-process client test.
7. CRUD tool? Ship its gadget widget UI (per-call embedded Table/Form, actions targeting the
   model-visible tools) via the `gadget-mcp-ui` skill, and update `docs/SPECIFICATIONS.md` in the
   same commit.
8. Widget interactions flow through the standard MCP Apps App Bridge (`tools/call`, `ui/open-link`,
   `ui/notifications/size-changed`) — configure them via gadget's `Action`/`Submit` API, never a
   hand-authored postMessage payload or text-sentinel envelope.

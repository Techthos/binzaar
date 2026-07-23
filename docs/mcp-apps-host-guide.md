# How an AI chat host renders MCP Apps micro-app widgets

This guide is for implementers of an AI chat application (an MCP **host**) that connects to these
micro-apps and wants their interactive widgets to render and behave correctly. It applies to **every
micro-app built from this template** (binzaar itself, and every app it scaffolds or installs): a
single Go binary whose MCP server embeds interactive HTML widgets in its tool results, following the
**MCP Apps** extension (`io.modelcontextprotocol/ui`, spec version `2026-01-26`). It describes the
contract from the host's side: what a server sends, how to render it, and how widget interactions
flow back through the standard **App Bridge**.

The protocol here is the one defined by the MCP Apps specification — not a project-specific
invention. See the overview at <https://modelcontextprotocol.io/extensions/apps/overview> and the
specification and `@modelcontextprotocol/ext-apps` SDK at
<https://github.com/modelcontextprotocol/ext-apps>. A host that implements MCP Apps needs no bespoke
message plumbing: attach the standard App Bridge and everything below is handled.

## 1. What a server sends

Every CRUD tool result (list/read tools, and mutating tools such as install/update/delete/save)
carries, in order:

1. a JSON **text** content block (and matching `structuredContent`) that stands alone, and
2. one extra **embedded resource** content block:

```json
{
  "type": "resource",
  "resource": {
    "uri": "ui://binzaar/catalog/1753260000000000000",
    "mimeType": "text/html;profile=mcp-app",
    "text": "<!doctype html>... complete self-contained document ..."
  }
}
```

Properties the host can rely on:

- **`ui://` scheme + `mimeType: "text/html;profile=mcp-app"`** identifies a renderable MCP Apps
  widget, regardless of which micro-app sent it. The `profile=mcp-app` token is what the host keys
  on to attach its App Bridge.
- **The document is fully self-contained** (inline CSS and JS, data baked in). It needs no network
  access, no `resources/read` round-trip, and no CSP allowances beyond a locked-down default.
- **The URI is unique per render** (`ui://<app>/<kind>/<unixnano>`). Key your renders by URI: two
  calls to the same tool produce two distinct widgets, each painting the data of its own call.
  Never dedupe or cache by URI prefix.
- The JSON block always stands alone. A host that renders no UI can ignore the resource block and
  lose nothing; a host that renders UI should still keep the JSON visible to the model (it is what
  the model reasons over).

Widgets come in two kinds: **tables** (list/read output; optionally filterable, paginated, sortable,
with per-row and bulk action buttons) and **forms** (create/update input; prefilled values, inline
per-field server-side errors). The host does not need to distinguish them; both are self-contained
documents obeying the same protocol.

## 2. Rendering the widget and attaching the App Bridge

- Render the document in a **sandboxed iframe**. The MCP Apps **App Bridge** from
  `@modelcontextprotocol/ext-apps` owns the iframe lifecycle, the `ui/initialize` capability
  handshake, and message passing over `postMessage`; the
  [`basic-host`](https://github.com/modelcontextprotocol/ext-apps/tree/main/examples/basic-host)
  example shows the integration, and `@mcp-ui/client` provides ready-made React components. Do not
  grant the iframe `allow-same-origin`, popups, or downloads.
- The widgets never call `alert()`/`confirm()`/`prompt()` — destructive actions use an inline
  two-phase confirm button — so the sandbox can stay dialog-free.
- **Auto-resize.** The widget reports its content height and the bridge applies it via the standard
  **`ui/notifications/size-changed`** mechanism. Set the iframe height from it; let the iframe fill
  the available width and the widget's responsive CSS adapt. A host that honors this never shows an
  internally scrolled or clipped widget.
- **Theming.** Widgets use a `--gadget-*` design-token system with built-in light/dark fallbacks;
  they look correct with zero configuration. A host may inject `hostContext.styles.variables`
  (delivered via `ui/notifications/host-context-changed`) to align them with its theme, but this is
  optional.
- Render the widget **once, in place, in the assistant turn that carried it**. When a later tool
  call embeds a refreshed widget (see below), that is a new render with a new URI; do not try to
  patch data into an old iframe.

## 3. How interactions flow back (MCP Apps JSON-RPC)

Widget buttons, form submits, and links do not mutate anything inside the iframe. Each interaction
is a standard MCP Apps request the widget sends to the host over the `postMessage` channel, and the
App Bridge routes it. Every message is **JSON-RPC 2.0** — there is no text sentinel, prompt
injection, or bespoke envelope.

- **Tool actions** (row buttons, bulk actions, form submit) → **`tools/call`** with the target tool
  name and arguments. The App Bridge runs it **directly** against the same micro-app's MCP server
  and returns the result to the widget. In binzaar these targets are normal tools (`install_app`,
  `update_app`, `uninstall_app`, `set_config`).
- **Links** → **`ui/open-link`** with the target URL. Open it in a new browser tab; never navigate
  the iframe or the chat page.
- **Sizing** ← the host applies `ui/notifications/size-changed` (see section 2).

The full method surface, for a host implementing the bridge directly rather than using the SDK:

| Direction | Method | Purpose |
|---|---|---|
| widget → host | `ui/initialize` | capability handshake |
| widget → host | `tools/call` | run a tool on the MCP server |
| widget → host | `resources/read` | read a resource |
| widget → host | `ui/open-link` | open an external URL |
| widget → host | `ui/message` | add a message to the conversation |
| widget → host | `ui/request-display-mode` | request a display-mode change |
| widget → host | `ui/update-model-context` | update the model's context |
| host → widget | `ui/notifications/tool-input` / `…-partial` | (streaming) tool input |
| host → widget | `ui/notifications/tool-result` | tool result pushed to the widget |
| host → widget | `ui/notifications/tool-cancelled` | tool execution cancelled |
| host → widget | `ui/notifications/size-changed` | iframe size update |
| host → widget | `ui/notifications/host-context-changed` | host state (theme, etc.) |

## 4. The refresh loop (why every mutation embeds a new widget)

There is no in-place data push to a live widget. The loop, illustrated with binzaar's catalog:

1. User: "show me the catalog" → model calls `list_catalog` → host renders the catalog table.
2. User clicks **Install** on a row → the widget sends
   `tools/call {"name":"install_app","arguments":{"repo":…}}` → the App Bridge runs it against the
   server.
3. The `install_app` result carries the install record **and a freshly rendered catalog widget**
   with the row's status badge now `installed`.
4. Host renders that new widget (new URI). The old one stays as a historical snapshot.

Every mutating tool of a micro-app behaves the same: its result embeds the refreshed dataset's
table, or the refreshed form (with inline field errors under `errors` keyed by field name when
server-side validation fails). The host needs no special handling; the loop is just steps 1–4
repeating.

## 5. Host checklist

- [ ] Render `type: "resource"` blocks with a `ui://` URI and
      `mimeType: "text/html;profile=mcp-app"` in a sandboxed iframe (no `allow-same-origin`), keyed
      by URI, via the App Bridge.
- [ ] Honor `ui/notifications/size-changed`; never fix the iframe height.
- [ ] Route widget `tools/call` to the MCP server and return the result to the widget.
- [ ] Handle `ui/open-link` by opening the URL externally.
- [ ] Keep the JSON text/`structuredContent` in the model's context; the widget is presentation
      only.
- [ ] Ignore unknown methods and unknown content blocks.

## 6. Security notes for the host

- The document executes arbitrary JS from the MCP server: the sandbox (no same-origin, no
  top-navigation, no popups) is the boundary. Treat every message as untrusted input; validate its
  shape and enforce the bridge's capability policy (which tools a widget may call, whether
  `ui/open-link` is permitted).
- A `tools/call` from a widget can have side effects (downloads, filesystem writes, deletions). The
  host's capability policy and user-consent model govern which calls are permitted.
- Widgets never need network egress; a CSP that blocks all external fetches from the iframe is
  correct and expected.

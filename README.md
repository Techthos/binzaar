# microstore

A single-binary local store for Go micro-apps: browse a GitHub-hosted catalog, install the right
release binary for your machine (with SHA-256 verification), manage what you've installed, and
scaffold new micro-apps from templates. It exposes the same domain through both a **tview TUI** and
an **MCP stdio server**, backed by an embedded **bbolt** database.

See [`docs/SPECIFICATIONS.md`](docs/SPECIFICATIONS.md) for the full product contract.

## Prerequisites

- **Go** (toolchain that satisfies `go.mod`).
- Optional, for `make fmt` / `make lint`:
  ```sh
  go install mvdan.cc/gofumpt@latest
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
  ```
  `make` reports which are missing rather than failing silently.

## Getting started

```sh
make build    # go build ./...
make run      # go run .
make test     # go test ./... -race -cover
make fmt      # gofumpt -w .
make lint     # golangci-lint run
make tidy     # go mod tidy
make check    # fmt + tidy + lint + test, in sequence
```

Run a single test:

```sh
go test ./... -run TestName -race -v
```

## Configuration

- `MICROSTORE_GITHUB_TOKEN` — optional GitHub token; raises rate limits and enables private repos.
  Anonymous access is used when unset.

## Layout

This repo starts flat (`main.go`) and grows `internal/` packages as the spec is implemented:

```
models  ←  db  ←  server
            ↑
           tui
```

`internal/models` is storage-agnostic; `internal/db` is the only package that touches bbolt; both
`internal/server` (MCP) and `internal/tui` go through `internal/db`. See `CLAUDE.md` and
`.claude/rules/` for the layer rules.

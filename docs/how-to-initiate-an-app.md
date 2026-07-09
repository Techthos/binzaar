# How to initiate and create a Binzaar micro-app with Claude Code

This guide walks you from an empty directory to a working, released Go micro-app in the **Binzaar
shape**: a single local binary that presents one shared domain through up to three faces — a
**tview terminal UI**, an **MCP stdio server**, and an **embedded bbolt database**.

It uses the **Claude Code starter kit** that Binzaar ships and places for you. You do not write the
kit yourself — Binzaar drops it into your project, and Claude Code then drives a spec-first workflow
on top of it.

---

## The mental model

Binzaar micro-apps are built **spec-first**. Nothing is coded until there is an agreed contract in
`docs/SPECIFICATIONS.md`, and from then on the spec and the code change together. The kit encodes
this as three slash commands run in order:

```
/product-idea  →  /app-init <module-path>  →  /app-spec-sync
   (write the         (scaffold the             (reconcile code
    contract)          Go project)               with the spec, in phases)
```

- **`/product-idea`** — an interactive discovery session that turns a raw idea into
  `docs/SPECIFICATIONS.md`. Writes only the spec, no code.
- **`/app-init <module-path>`** — scaffolds a minimal, idiomatic Go project (go.mod, `main.go`,
  Makefile, linting, CI). **Refuses to run until the spec exists.**
- **`/app-spec-sync`** — audits code against the spec, finds gaps/drift, and implements them in
  small, test-covered phases. Run it repeatedly as the app grows.

Two supporting pieces close the loop: the **`build-and-release`** skill generates the release
workflow, and **`/release`** tags and pushes a version so the binary ships.

Everything must fit the **local-only envelope**: one executable, no external runtime dependencies,
no web server, no cloud APIs, no second process. The commands enforce this envelope for you.

---

## Prerequisites

- **Claude Code** installed and authenticated.
- **Go** toolchain that satisfies `go.mod` (the kit pins one Go version in CI).
- A GitHub repo (or the intent to create one) if you want releases.
- Optional but recommended tooling — the Makefile reports these as missing rather than failing:
  ```sh
  go install mvdan.cc/gofumpt@latest
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
  ```

---

## Step 0 — Place the starter kit

Create and enter a fresh directory, then let Binzaar drop the Claude Code kit into it:

```sh
mkdir my-microapp && cd my-microapp
binzaar init
```

`binzaar init` places the embedded `.claude/` kit (commands, rules, skills) and `.github/`
workflows into the current directory and prints the phase guide. It **opens no database and needs
no network** — it is a pure file placement.

After it runs you have:

```
.claude/
  commands/     product-idea.md · app-init.md · app-spec-sync.md · release.md
  rules/        db-rules.md · mcp-server.md · tui-rules.md · go-testing.md ·
                github-actions.md · specification-rules.md
  skills/       build-and-release/
.github/
  workflows/    ci.yml · build-and-release.yml
```

The **rules** in `.claude/rules/` load automatically whenever you edit a matching path, so Claude
Code applies the right per-layer conventions without you asking. Now open Claude Code in this
directory.

---

## Step 1 — Write the spec: `/product-idea`

```
/product-idea a local bookmark manager with tags
```

The argument (a one-line idea) is optional — you can also run `/product-idea` bare and describe the
idea in conversation.

This is a **collaborative session**, not a form. Claude Code will:

1. Reflect your problem and goal back in a sentence and confirm it.
2. Pin down scope and **non-goals** (the smallest useful v1).
3. Work through the **domain model** — entities, attributes, relationships, and what uniquely
   identifies each one (this becomes `internal/models` + bbolt buckets).
4. Design **persistence** — buckets, key encoding (so lexical order matches logical order),
   secondary indexes, serialization (JSON by default).
5. Enumerate **use-cases**, **user stories**, the **MCP surface** (tools/resources/prompts), the
   **TUI surface** (screens + navigation), and **acceptance criteria**.

It enforces the local-only envelope as it goes: if your idea implies a web server, cloud API, or a
second process, it surfaces the conflict and helps you reshape or scope it out.

**Output:** `docs/SPECIFICATIONS.md` — the implementation contract. Nothing is written until the
plan is coherent and you confirm it. Re-running `/product-idea` later treats the existing spec as a
revision session and shows you what changed.

> The spec is the source of truth for *what the app is*. From here on, **spec and code change
> together** (`specification-rules.md`) — a behavior change is not done until the spec reflects it.

---

## Step 2 — Scaffold the project: `/app-init`

```
/app-init github.com/your-org/my-microapp
```

Pass your **Go module path** as the argument (it will ask if you omit it — it never guesses).

Guardrails it checks first:

- **Spec gate:** if `docs/SPECIFICATIONS.md` is missing, it **stops** and tells you to run
  `/product-idea` first. Scaffolding only happens against an agreed spec.
- If Go isn't installed, or `go.mod` already exists, it reports the state instead of clobbering
  anything.

What it creates (deliberately **minimal** — flat, no `cmd/` or `internal/` until needed):

- `main.go` + `main_test.go` (one green test so `go test ./...` passes from day one)
- `.gitignore`, `README.md`
- `.golangci.yml` (golangci-lint **v2**), `gofumpt` formatting
- `Makefile` with `fmt` · `lint` · `test` · `build` · `tidy` · `check` · `run`
- `.github/workflows/ci.yml`
- initializes git if needed and stages the files (it won't commit unless you ask)

It finishes by running `go build ./...` and `go test ./...` to prove the scaffold is green, then
**offers the release workflow** — say yes to have the `build-and-release` skill generate
`.github/workflows/build-and-release.yml`.

---

## Step 3 — Implement in phases: `/app-spec-sync`

This is the workhorse you run repeatedly. It reconciles **what the spec says** with **what the code
does** in three movements:

1. **Detect drift** via git-diff — what in the spec recently moved (highest-risk areas first).
2. **Audit coverage** — parse the spec into concrete elements (entities, buckets, use-cases, MCP
   tools, TUI screens, acceptance criteria) and produce a **coverage matrix**:

   | Status | Meaning |
   |---|---|
   | ✅ done | implemented in the right layer **and** covered by a passing `_test.go` |
   | 🟡 untested | code exists but no real test |
   | 🔴 missing | spec requires it, no code yet |
   | 🟠 drift | code and spec disagree |

3. **Plan & implement** small, **test-forced** phases — one entity, one repository op, one MCP tool,
   or one screen at a time. Every phase ships with a sibling `*_test.go` and a green
   `go test ./... -race`. Drift (🟠) is resolved before new gaps (🔴).

Scope it to a focus area if you like:

```
/app-spec-sync bookmarks         # audit + implement just the Bookmark entity/use-cases
/app-spec-sync                   # whole-spec audit
```

It respects the **dependency rule** throughout — `models` → `db` → (`server` | `tui`) — and never
plans a server/TUI phase before the repository method it needs exists. It stops for review after
each phase unless you tell it to run straight through.

Repeat `/app-spec-sync` until the coverage matrix is all ✅. Whenever you change the spec, run it
again to reconcile.

---

## The architecture your app grows into

`/app-spec-sync` builds toward the standard Binzaar layering. The key rule: **`internal/app` is the
use-case layer, and `server`/`tui` depend on it — never directly on `db`, `github`, `install`, or
`scaffold`.**

```
models ← db ─────┐
(leaf services)  ┼─ app  ─┬─ server (MCP)
                 ┘        └─ tui
```

- `internal/models` — plain domain structs; **never** imports bbolt. Separates live entities from
  persisted ones.
- `internal/db` — the **only** package that touches `go.etcd.io/bbolt`; hands out repositories that
  return domain models, never `*bolt.Tx`/`*bolt.Bucket`.
- `internal/app` — the `Service` that wires everything behind use-case methods.
- `internal/server` / `internal/tui` — each defines a narrow interface that `*app.Service`
  satisfies and calls **only** through it; no business logic in a handler or draw call.
- `cmd/` opens the single bbolt DB once, builds the `Service`, and dispatches by mode.

Two cross-cutting constraints the rules enforce:

1. **bbolt takes a process-wide write lock** — only one writer process at a time; design modes as
   alternatives (or use a read-only consumer).
2. **MCP stdio uses stdout as the protocol channel** — in `serve` mode, **all logging goes to
   stderr**, never stdout.

You don't have to memorize these — the matching rule file loads automatically when you edit that
layer.

---

## Step 4 — Build, run, and test locally

The Makefile is your day-to-day loop:

```sh
make build    # go build ./...
make run      # go run .
make test     # go test ./... -race -cover
make fmt      # gofumpt -w .
make lint     # golangci-lint run
make check    # fmt + tidy + lint + test, in sequence
```

Run a single test: `go test ./internal/db -run TestName -race -v`.

Once modes exist, the binary selects a mode in `cmd/` — typically the TUI by default, `serve`/`mcp`
for the MCP stdio server, and `init` to place the kit. A `--db <path>` flag overrides the DB
location.

---

## Step 5 — Release

If you accepted the release workflow during `/app-init` (or generated it later with the
`build-and-release` skill), tagging a version triggers a cross-platform build with SHA-256
checksums uploaded to a GitHub Release:

```sh
git tag v0.1.0 && git push origin v0.1.0
```

The kit also includes a **`/release`** command that commits outstanding work, picks the next
semantic version based on the changes, tags it, and pushes to origin with tags.

The release job builds a matrix of `linux/darwin/windows × amd64/arm64` static binaries
(`CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w -X main.version=…"`) plus a `.sha256` sidecar for
each — exactly the shape Binzaar's own installer verifies against.

---

## Quick reference

| Command / skill | When | Produces |
|---|---|---|
| `binzaar init` | empty dir, before Claude Code | places `.claude/` + `.github/` kit |
| `/product-idea [idea]` | first | `docs/SPECIFICATIONS.md` (the contract) |
| `/app-init <module-path>` | after the spec exists | scaffolded Go project + CI |
| `/app-spec-sync [focus]` | repeatedly, as the app grows | tested implementation phases |
| `build-and-release` skill | once, offered by `/app-init` | `build-and-release.yml` |
| `/release` | to ship | version tag pushed to origin |

**Golden rules:** spec before code · spec and code change in the same commit · every phase ships
tests · `app` is the only orchestration layer · local-only envelope, always.

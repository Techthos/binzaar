---
description: Conventions for the Makefile. Applies when creating or editing Makefile build targets.
paths:
  - Makefile
---

# Makefile rules

## Build output always goes under `./bin/`

- The `build` target must **fully build the binary** and place it at **`./bin/<binary-name>`**,
  where `<binary-name>` is the project's binary name (normally the repo/module directory name).
  This is always the `go build` output location:

  ```make
  build:
  	$(GO) build -o bin/<binary-name> .
  ```

  (`go build -o` creates `bin/` itself; no `mkdir` needed.)

- Never use a bare `go build ./...` as the `build` target — it only compiles and drops nothing
  (or, for a root `main` package, drops the binary in the repo root). The placed binary under
  `./bin/` is the contract.
- Never let any target write a binary to the repo root; every target that produces a binary
  (cross-compile helpers included) writes under `./bin/`.
- Keep `./bin/` gitignored (`/bin/` in `.gitignore`) — it is build output, never committed.

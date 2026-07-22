# Serving a custom registry with Docker Compose

`binzaar serve-catalog` (UC 15) is a deliberately minimal HTTP file server: it validates a local
catalog JSON file at startup, then serves it on `GET /catalog.json` (and `GET /`), re-reading the
file on every request. It opens no database and contacts no GitHub — which makes it trivial to run
in a container.

## 1. Build a static Linux binary

binzaar is pure Go (no cgo), so a static binary that runs on `alpine` is one build away:

```sh
CGO_ENABLED=0 GOOS=linux go build -o bin/binzaar .
```

Set `GOARCH` explicitly (`amd64` or `arm64`) if the machine running Docker differs from the one
building. Alternatively, download the matching release binary instead of building.

## 2. Compose file

With this layout on the host:

```
.
├── compose.yaml
├── bin/
│   └── binzaar          # the linux binary from step 1
└── registry/
    └── catalog.json     # the catalog to serve
```

```yaml
# compose.yaml
services:
  registry:
    image: alpine:3.20
    ports:
      - "8080:8080"
    working_dir: /app
    volumes:
      - ./bin/binzaar:/app/binzaar:ro
      - ./registry:/app/registry:ro
    command: ["./binzaar", "serve-catalog", "--catalog", "registry/catalog.json", "--addr", ":8080"]
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/catalog.json"]
      interval: 30s
      timeout: 3s
```

```sh
docker compose up -d
curl http://localhost:8080/catalog.json
```

Note that the catalog is mounted as a **directory** (`./registry`), not as a single file. A
single-file bind mount pins the file's inode: editors and tools that save by writing a new file and
renaming it over the old one (as `mv` does) would leave the container serving the stale copy.
Mounting the parent directory avoids that, so `serve-catalog`'s per-request re-read keeps edits
visible without a restart.

## 3. Point binzaar instances at it

Consumers set their manifest URL to the served endpoint:

- **TUI:** Config screen → set the manifest URL to `http://<host>:8080/catalog.json`.
- **MCP:** call `set_config` with `{ "manifest_url": "http://<host>:8080/catalog.json" }`.

Browse/search then reads this catalog, while each listed app's releases, assets, and template
tarballs are still fetched from GitHub as usual.

## Notes

- `serve-catalog` speaks **plain HTTP**. For anything beyond localhost or a trusted LAN, put it
  behind a TLS-terminating reverse proxy (Caddy, nginx, Traefik) and hand out the `https://` URL.
- The container needs no database volume and no `BINZAAR_GITHUB_TOKEN` — this mode uses neither.
- An unreadable or invalid catalog file yields HTTP 500 on request; at startup an invalid file makes
  the server refuse to start (fail fast).

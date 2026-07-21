---
description: Rules and conventions for the bbolt embedded key/value store and the domain models persisted in it.
paths:
  - internal/db/**
  - internal/models/**
---

# bbolt persistence rules (`internal/db`, `internal/models`)

These rules apply when working in `internal/db` (the storage layer) and `internal/models`
(domain structs that get serialized into bbolt).

## Library

- **Package:** `go.etcd.io/bbolt` — an embedded, ACID-compliant, single-file key/value store. Pure Go, no server, no cgo.
- **Version:** pin the `v1.4.x` line in `go.mod` (latest stable as of 2026). Treat the module path `go.etcd.io/bbolt` as canonical — **never** use the deprecated `github.com/boltdb/bolt`.
- **Import alias:** always alias as `bolt`:
  ```go
  import bolt "go.etcd.io/bbolt"
  ```
- **Docs:** README https://github.com/etcd-io/bbolt · GoDoc https://pkg.go.dev/go.etcd.io/bbolt

## Hard constraints (enforced by bbolt — design around them)

- A key must be **non-empty** and **≤ 32,768 bytes**; a value must be **< 2 GiB**.
- `Get` returns `nil` for a missing key (no error, no sentinel). Always nil-check.
- **Byte slices returned by `Get`, `Cursor`, and `ForEach` are only valid inside the
  transaction.** To use them later, copy them (`append([]byte(nil), v...)`) or unmarshal
  before the txn closes. Never return or store a raw bbolt slice past `View`/`Update`.
- Keys are stored in **byte-sorted order**. Exploit this for range/prefix scans; design
  key encodings (e.g. zero-padded numbers, RFC3339 timestamps) so lexical order == logical order.
- A read-write transaction takes a **process-wide exclusive lock**; only one writer at a time.
  Many concurrent readers are fine. Never open the same file from two processes read-write.

## Database location — one resolver, env-var overridable

The DB path is **resolved in exactly one place** — a `DefaultPath()` function in `internal/db` — and
every caller (each `cmd/` mode, tests, tooling) goes through it. Never hardcode a path at a call site
and never rebuild the `filepath.Join(home, ...)` expression in a second package: a second copy is how
the TUI and the MCP server end up on different files.

**Precedence, highest first:**

1. **An explicit `--db <path>` flag** — the operator said exactly what they meant; nothing overrides it.
2. **The `<APP>_DB` environment variable** — uppercased binary name plus `_DB` (e.g. `BINZAAR_DB`).
   An **empty value counts as unset** and falls through to the default.
3. **The XDG default** — `$XDG_DATA_HOME/<app>/<app>.db`, falling back to
   `~/.local/share/<app>/<app>.db` when `XDG_DATA_HOME` is empty.

The env-var tier is **not optional**. It is what makes the app usable without flag plumbing:
separate profiles/datasets, a scratch DB in tests and CI, a mounted volume in a container, and a way
to point an MCP client at the same file the TUI uses when the launcher gives you no argv control.

- **Resolution is pure and side-effect free.** `DefaultPath()` computes a string — it must not create
  directories, touch the file, or open bbolt. Directory creation (`os.MkdirAll` on
  `filepath.Dir(path)`, `0o755`) belongs to whatever opens the DB.
- **Fail loudly if there is no home.** If `os.UserHomeDir()` errors and neither a flag nor the env
  var was given, return that error. Do **not** silently fall back to a relative path — that quietly
  creates a stray DB under whatever directory the user happened to `cd` into, splitting their data.
- **Don't expand `~` yourself.** A flag or env-var value is used verbatim; the shell already expands
  `~`, and a literal `~` from a non-shell caller is a real (ugly) directory name, not a home reference.
- **Tests never touch the resolved path.** Use `t.TempDir()` and pass the path in explicitly. A test
  that reads the operator's real DB is a bug; one that writes it is data loss.
- **Document it.** The env var belongs in the README's environment table and in
  `docs/SPECIFICATIONS.md` alongside the flag — adding or renaming it is a spec change.

```go
// DefaultPath reports where the database lives, honouring <APP>_DB.
// An explicit --db flag takes precedence and is applied by the caller in cmd/.
func DefaultPath() (string, error) {
    if p := os.Getenv(dbPathEnv); p != "" { // dbPathEnv = "<APP>_DB"
        return p, nil
    }
    if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
        return filepath.Join(dir, appName, appName+".db"), nil
    }
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("resolve home directory for default db path: %w", err)
    }
    return filepath.Join(home, ".local", "share", appName, appName+".db"), nil
}
```

## Opening the database

Open once at startup, keep the `*bolt.DB` for the process lifetime, and always set a `Timeout`
so a stale lock fails fast instead of blocking forever.

```go
db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
if err != nil {
    return fmt.Errorf("open bbolt at %q: %w", path, err)
}
// defer db.Close() at the owning scope
```

- Use `ReadOnly: true` for shared read-only access (no exclusive lock).
- Tune only when justified: `NoFreelistSync: true` + `FreelistType: bolt.FreelistMapType`
  for write-heavy/large DBs; `InitialMmapSize` for read-heavy low-latency.

## Transactions — the only rules that matter

- **Reads:** `db.View(func(tx *bolt.Tx) error { ... })`. No mutations allowed.
- **Writes:** `db.Update(func(tx *bolt.Tx) error { ... })`. Return `nil` to commit, return a
  non-nil error to roll back the **entire** transaction atomically.
- Prefer the managed `View`/`Update` closures. Use manual `db.Begin(writable)` only when you
  must span control flow — and then `defer tx.Rollback()` (safe after `Commit`) and explicitly
  `Commit()`. A leaked read txn causes **unbounded file growth**.
- Keep transactions short; never do network I/O or block inside a write txn (it holds the lock).
- Batch many independent writes with `db.Batch(...)` — but its fn may run **more than once**,
  so it must be **idempotent and side-effect free** apart from bbolt operations.

## Buckets & keys

- Create buckets idempotently with `tx.CreateBucketIfNotExists([]byte("name"))`; pre-create all
  required top-level buckets in a single migration/`Update` at startup.
- Define bucket names as **package-level `[]byte` constants** in `internal/db`, never inline literals.
- Nest buckets (`b.CreateBucketIfNotExists`) for hierarchical data; a key and a sub-bucket cannot
  share the same name.

## Domain models (`internal/models`)

- `internal/models` holds plain domain structs. Keep them **free of bbolt imports** — persistence
  concerns live in `internal/db`, models stay storage-agnostic.
- Choose an explicit serialization and apply it consistently (JSON via `encoding/json` is the
  simple default; switch to a faster/compact codec only with a reason). Document the choice here
  when made.
- Each model needs a stable, deterministic key strategy (e.g. an ID field encoded to bytes).
  Marshal in `internal/db` right before `Put`, unmarshal right after `Get`/inside the cursor loop.
- For surrogate IDs, use per-bucket `Bucket.NextSequence()` (persistent monotonic `uint64`) and
  encode the key **big-endian** (`binary.BigEndian.PutUint64`) so numeric IDs sort correctly:
  ```go
  id, _ := b.NextSequence()
  key := make([]byte, 8)
  binary.BigEndian.PutUint64(key, id)
  ```
- When evolving a struct, keep decoding **backward compatible** (additive fields, tolerate missing
  keys) since old records remain on disk.

## Iteration

- `Cursor` (`Seek`/`First`/`Last`/`Next`/`Prev`) for prefix/range scans and reverse walks — reposition the cursor after any mutation.
- `Bucket.ForEach(func(k, v []byte) error)` to walk every pair in lexicographical order. **`v == nil` means the entry is a nested bucket, not a value** — check it. Iteration stops on the first non-nil error. Do **not** mutate the bucket during `ForEach`.
- `tx.ForEach(func(name []byte, b *bolt.Bucket) error)` walks **top-level buckets**; combine with `b.Stats().KeyN` for per-bucket key counts (useful for diagnostics/migrations).

## Errors

- bbolt exposes stable sentinel errors; match with `errors.Is`, never on string text. Common ones:
  - Buckets: `bolt.ErrBucketNotFound`, `bolt.ErrBucketExists`, `bolt.ErrBucketNameRequired`, `bolt.ErrIncompatibleValue`.
  - Keys/values: `bolt.ErrKeyRequired`, `bolt.ErrKeyTooLarge`, `bolt.ErrValueTooLarge`.
  - Transactions/DB: `bolt.ErrTxClosed`, `bolt.ErrTxNotWritable`, `bolt.ErrDatabaseNotOpen`, `bolt.ErrDatabaseReadOnly`, `bolt.ErrTimeout` (lock-acquire timeout from `Open`).
- A `nil` from `Get` is **not** an error — it means the key is absent. Handle it explicitly.

## Durability & performance

- Default: every `Update`/`Commit` does an `fsync` — durable but ~one synchronous write per txn. Group many small writes to amortize that cost.
- `db.Batch(fn)` coalesces concurrent writes from multiple goroutines into one transaction (tuned via `DB.MaxBatchSize` / `DB.MaxBatchDelay`). The fn must be **idempotent** (it can run more than once).
- Bulk loads: set `db.NoSync = true`, run the writes, then call `db.Sync()` (fdatasync) once and reset `NoSync = false`. Crash-before-Sync loses the unsynced data — use only for rebuildable/import data.
- `db.Stats()` returns cumulative counters; snapshot twice and use `Stats.Sub` to get a delta for monitoring (txn counts, freelist pages, etc.).
- Concurrency model: one read-write txn at a time (process-wide), unlimited concurrent read txns; reads never block writes (MVCC via mmap). bbolt is well-suited to read-heavy workloads.
- **Long-lived read txns are costly**: an open read txn pins the old pages, so the file can't reclaim freed space and grows while it's held. Keep `View` closures short; don't hold a read txn across requests.
- `Bucket.FillPercent` (default `bolt.DefaultFillPercent` = 0.5) tunes page split density. For **append-only / monotonically increasing keys**, raise it toward `1.0` before bulk inserts for denser pages; leave the default for random-key inserts (high values + random inserts give poor page utilization).

## Backups & maintenance

- **Hot backup** from a read-only txn while the DB is live:
  - `tx.WriteTo(w io.Writer)` — stream a consistent snapshot (e.g. over HTTP; set `Content-Length` from `tx.Size()`).
  - `tx.CopyFile(path, mode)` — write the snapshot straight to a file.
- **Compaction** — bbolt never shrinks its file on its own; deleted space is reused, not released. Reclaim fragmented free space periodically with `bolt.Compact(dst, src, txMaxSize)` (pass a non-zero `txMaxSize`, e.g. 64 MiB, to cap per-txn memory), then atomically replace the old file.

## Operability — the `bbolt` CLI

Install once (`go install go.etcd.io/bbolt/cmd/bbolt@latest`) for inspecting/repairing a DB file out-of-band. **Read commands still need the exclusive lock**, so run them against a copy/backup of a live DB, not the in-use file.

- `bbolt inspect <db>` — hierarchical bucket tree with key counts (`keyN`).
- `bbolt stats <db>` — page-usage and tree-structure statistics.
- `bbolt check <db>` — exhaustive integrity check (page reachability, double references); prints `ok` if intact.
- `bbolt get [--format=hex] <db> <bucket> <key>` / `bbolt keys <db> <bucket>` — dump individual values/keys.
- `bbolt surgery ...` — recovery tools (e.g. `surgery freelist rebuild <db> --output rebuilt.db`, `revert-meta-page`). Last resort on a corrupted file; always operate on a copy.

## Reference snippets

```go
// Write + read in one txn
err := db.Update(func(tx *bolt.Tx) error {
    b, err := tx.CreateBucketIfNotExists(productsBucket)
    if err != nil {
        return err
    }
    return b.Put(key, encoded) // encoded = json.Marshal(model)
})

// Prefix / range scan (keys are byte-sorted)
err = db.View(func(tx *bolt.Tx) error {
    c := tx.Bucket(productsBucket).Cursor()
    prefix := []byte("2026-")
    for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
        // copy or unmarshal v here — do not retain it past this loop
    }
    return nil
})
```

## Do / Don't

- ✅ Wrap every bbolt error with `%w` and context (`path`, bucket, key).
- ✅ Centralize all bbolt access behind a repository type in `internal/db`; callers get domain
  models, never `*bolt.Tx`.
- ❌ Don't leak `*bolt.Tx`, `*bolt.Bucket`, or transaction-scoped byte slices outside their txn.
- ❌ Don't run long/blocking work inside `Update`.
- ❌ Don't use `db.Batch` for non-idempotent logic.

## References

- Context7 (source of these rules, up-to-date API + snippets): https://context7.com/etcd-io/bbolt — library ID `/etcd-io/bbolt`. Re-query via the context7 MCP for the latest examples.
- Upstream README: https://github.com/etcd-io/bbolt/blob/main/README.md
- API reference (GoDoc): https://pkg.go.dev/go.etcd.io/bbolt
- `bbolt` CLI docs: https://github.com/etcd-io/bbolt/blob/main/cmd/bbolt/README.md
- Releases / changelog: https://github.com/etcd-io/bbolt/releases

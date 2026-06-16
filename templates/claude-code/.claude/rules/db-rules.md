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
  Many concurrent readers are fine. Don't hold two long-lived handles on the same file from two
  processes read-write — if you genuinely need multiple writer processes, use the
  **connection-per-operation strategy** documented at the end of this file instead.

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

## Where the database file lives

Each micro-app keeps its store under its own per-app directory:

```
~/.local/microapp/<name>/store.db
```

`<name>` is the micro-app's name. Create the directory (`0700`) before opening the file, and let
`--db <path>` (or the equivalent flag/env) override the location. Keeping one DB per app under its
own directory is what makes the connection-per-operation strategy below safe across an app's own
processes (e.g. its TUI and MCP server) without colliding with other apps' files.

# bbolt concurrent-access strategy: connection-per-operation

A portable strategy for letting **two or more independent processes** read and write the same
[bbolt](https://github.com/etcd-io/bbolt) file, without the "open at startup, hold the handle for
the process lifetime" model that makes bbolt single-process. Application-agnostic; copy it anywhere.

> **TL;DR** — bbolt's file lock is taken at `Open`, not per-transaction, and a held *read* lock
> blocks *all* writers. The only way two processes can both write is for **neither to hold any handle
> while idle**: open for one operation, then close. Wrap the open in a timeout + backoff retry so a
> collision becomes a sub-second wait. Detect external writes by polling bbolt's monotonic txid.

## 1. Why the default model is single-process

bbolt's lock is an OS file lock (`flock` on Unix, `LockFileEx` on Windows). Two facts decide
everything:

1. **The lock is acquired at `Open`, not per transaction.** There is no "lock only while writing";
   opening read-write locks the file until `Close`. The usual `bolt.Open(...); defer db.Close()`
   therefore holds a process-wide lock for the whole process lifetime. A second process that opens
   read-write blocks until its `Timeout`, then fails with `ErrTimeout`.
2. **Two lock modes conflict in the direction that bites:** read-write takes `LOCK_EX` (exclusive);
   `ReadOnly: true` takes `LOCK_SH` (shared). `LOCK_EX` cannot be granted while **any** `LOCK_SH` is
   held — even one held by the same process on a different descriptor.

So the tempting design — keep a read-only handle open for fast reads, open a second RW handle only
to write — **deadlocks**: the persistent `LOCK_SH` blocks the writer's `LOCK_EX`, including its own.
Two long-lived processes each holding an idle read handle means *no write ever succeeds*.

**Consequence:** to let two processes both write, *no process may hold any handle while idle.*

## 2. The algorithm: connection-per-operation

Open the database for the duration of **one operation** and close it immediately. Idle processes
hold no lock, so any process is free to grab the (exclusive, brief) write lock when it needs it.
Reads and writes stay serialized at the file level, but each lock lasts milliseconds — for
low-contention workloads it behaves as if parallel.

```
idle      → no handle, no lock            (any process may write)
read op   → Open(ReadOnly) → View   → Close   (LOCK_SH, milliseconds)
write op  → Open(RW)       → Update → Close   (LOCK_EX, milliseconds)
```

Three rules make it correct and robust:

- **Per-open `Timeout` + backoff retry.** Keep the per-attempt timeout short (~75ms) and retry with
  growing backoff up to a total budget (~3s). On collision the loser waits, not fails. Retry **only**
  on `bolt.ErrTimeout`; any other error is fatal. Because retry uses `time.Sleep` (blocking), never
  call an operation on a UI/event-loop goroutine — run it on a worker and hand the result back.
- **One-time bootstrap.** `ReadOnly` opens require the file and buckets to already exist (bbolt
  can't create a file read-only). At startup, open read-write **once**, run an idempotent migration
  (`CreateBucketIfNotExists` for every bucket), and close. Every later operation assumes the schema.
- **Each operation is its own transaction.** Keep every cross-entity, must-be-atomic use-case inside
  a **single** `update(func(tx){...})` so it commits or rolls back as a unit — you can no longer span
  multiple `view`/`update` calls in one transaction.

The `Store` holds only the **path**, never a live `*bolt.DB`. An `open(readOnly bool)` helper runs
the timeout/retry loop; thin `view`/`update` wrappers call `open`, run the `bolt.Tx` function, and
`defer db.Close()`. (Reference Go implementation: see the bbolt GoDoc for `Options.Timeout`,
`Options.ReadOnly`; the wrapper is ~40 lines.)

## 3. Cross-process freshness: detecting "someone else wrote"

The data layer is never stale — every read opens fresh and sees the latest committed state. But a
**long-lived UI** caches a *rendered snapshot* in its widgets that must refresh when another process
writes.

bbolt stamps a **monotonically increasing transaction ID** in its meta page on every committed
write; a read transaction's `tx.ID()` returns the latest committed ID. Comparing it across reads is
a near-free "did anything change?" probe (one `Open` + empty `View`, no data scan). Wire it into a
long-lived reader as a background poll:

```
every 1–2s on a worker goroutine:
    now := store.txid()            // one Open + empty View
    if now != lastSeen:
        data := store.fetchAll()             // re-query off the UI thread
        queueUIUpdate(func(){ render(data) }) // tiny mutation on the UI thread
        lastSeen = now
```

Pair it with a **manual refresh key**. Notes: a no-op write still bumps the txid (occasional
identical re-render — harmless); `tx.ID()` is authoritative while file `mtime`/size are not (bbolt
writes via `mmap` + `fsync`); `fsnotify` works too but emits several events per commit and still
needs a txid confirm, so the poll is simpler.

## 4. Trade-offs & decision guide

Connection-per-operation costs a per-op `Open` (mmap + meta read, a few ms), a cold page cache each
op, and lower write throughput (one fsync + open per op) — in exchange for multiple writer processes
and always-fresh cross-process reads. **Read-your-own-writes across processes is eventual**: between
two operations another process may have written (identical to alt-tabbing between two windows).

```
Do multiple OS processes need to write the same bbolt file?
├─ No → use the standard single-handle model. This doc doesn't apply.
└─ Yes
   ├─ Can you instead run ONE process hosting all surfaces?
   │     → Prefer that. One handle, no lock dance, full cache & throughput.
   ├─ Low-contention & low-throughput (desktop tool, single user, UI + helper)?
   │     → Use connection-per-operation (this doc).
   └─ Write-heavy or genuinely concurrent (many writers, high TPS)?
         → Wrong tool. Use a client/server DB (SQLite+WAL; Postgres for true concurrency).
```

## 5. Correctness checklist

- [ ] `Store` holds the **path**, never a live `*bolt.DB`; no handle/`*bolt.Tx`/tx-scoped slice
      retained past its helper (copy or unmarshal before returning — the handle closes right after).
- [ ] One-time **bootstrap** opens RW, runs the idempotent migration, closes.
- [ ] `open(readOnly)` retries **only** on `bolt.ErrTimeout`, with short per-attempt `Timeout` +
      backoff up to a budget matched to your contention (documented if non-default).
- [ ] All reads via `view` (ReadOnly), all writes via `update` (RW); each atomic use-case is a
      **single** `update` transaction; operations are short (no network/blocking/user I/O inside).
- [ ] `view`/`update` are **never** called on a UI/event-loop goroutine.
- [ ] Long-lived readers refresh via a **txid poll** (`tx.ID()`) plus a **manual refresh** key.

## References

- bbolt README — https://github.com/etcd-io/bbolt/blob/main/README.md
- `Options.Timeout`, `Options.ReadOnly`, `Tx.ID()` — https://pkg.go.dev/go.etcd.io/bbolt#Options
- `flock(2)` lock semantics — https://man7.org/linux/man-pages/man2/flock.2.html

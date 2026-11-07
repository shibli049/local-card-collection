# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Card Atlas (Go module `cardatlas`, repo name soft-inventory) is a single-binary local web app for browsing and managing photo collections by album/page/row/column. Go server with the plain-JS UI embedded; no database, no frontend framework, no runtime network dependencies. The only Go dependency is Goldmark (admin guide rendering).

For feature work, follow `.agents/skills/card-atlas-feature/SKILL.md`. It holds the product contracts, guidance and tour follow-through rules, error and logging conventions, and the delivery checklist. Keep it accurate when architecture, storage, logging or build behavior changes.

## Commands

```sh
go run . --no-browser                 # run from source (default http://localhost:8765, data in ~/.soft-inventory)
go run . --no-browser --data "$(mktemp -d)" --port 0   # isolated data dir, any free port
go test -race ./...                   # full Go suite (what CI runs, on Linux/macOS/Windows)
go test -run '^TestUnit' ./...        # unit tests
go test -run '^TestIntegration' .     # real HTTP server tests
go test -run 'TestDelete' .           # single test or group by name pattern
go test -run '^$' -bench '^BenchmarkLargeCatalog' -benchmem .
node --test tests/frontend.test.cjs   # client tests, Node 22+, no npm packages
git diff --check
go run ./cmd/build                    # macOS arm64/amd64 + Windows amd64/arm64 executables and ZIPs in dist/
```

Static files are embedded with `//go:embed static/*` (`server.go`). A running app or built executable won't pick up source edits until it is restarted or rebuilt.

## Architecture

Everything is in `package main` at the repo root. `cmd/build` is the cross-compile/packaging tool.

**Catalog state (`catalog.go`).** One `Catalog` holds the whole `CatalogData` (images keyed by relative path, albums, root) in memory under `c.mu` (RWMutex). It is persisted as `catalog.json` inside the image root. `~/.soft-inventory/library-location.json` (or the `--data` dir) only remembers where that root is. Mutations follow one pattern: take the lock, `snapshot()` a copy, modify the copy, then `commit(copy)`. `commit` writes atomically (temp file + rename) *before* swapping in-memory state, and clears `c.index`. Don't mutate `c.data` directly. Don't call `c.logBase()` while holding `c.mu`, because it takes a read lock.

**Index (`index.go`).** A lazily rebuilt cache of sorted metadata, album counts and normalized search text. `commit` invalidates it. Scaling tests (`scaling_test.go`, 50k cards) cover response bounds and invalidation.

**HTTP (`server.go`, `auth.go`).** `handlerForHost` is one handler that checks host, `Sec-Fetch-Site` and `Origin` on every request. Then every POST except login/logout requires an admin session, and every POST except `/api/import` (multipart) must be JSON. API routes are dispatched with a `switch` on the path inside this function, not a mux, so add new routes there. GETs are public read-only. Credentials are hardcoded in `auth.go`, and sessions are in memory, so they don't survive a restart. `--host` is `127.0.0.1` by default; `0.0.0.0` is opt-in (`listen.go`).

**Filesystem mutations.** `import.go` (review/plan conflicts, then apply with rollback), `rearrange.go` (positions/layouts), `transfer.go` (moves between albums), `delete_album.go` / `delete_selection.go` (move to `.card-atlas-trash`). They all validate the full selection first, guard against path escapes and symlinks (`within`), and restore files if the catalog save fails. Positions come from filenames ending in `-page-row-column` (`positionPattern`).

**Hidden dirs under the image root** are excluded from scans and import fingerprints: `.card-atlas-cache/thumbnails` (`thumbnail*.go`), `.card-atlas-trash`, `.soft-inventory-import-*` (import recovery backups), and `.log` (daily debug/error logs with 5-day retention, `logging.go`).

**Errors (`errors.go`).** `userError` / `badRequest` turn internal errors into friendly messages without paths. Technical details go to the error log through `recordError`/`logEvent`. Wrap errors with `%w`.

**Client (`static/`).** `app.js` is the whole UI with no build step. `admin-tour.js` is the interactive admin walkthrough. `admin-guide.md` is rendered server-side (`admin_guide.go`) into `admin-guide.html` for signed-in admins. `tests/frontend.test.cjs` loads `app.js` into a `vm` context with a minimal fake DOM, so it tests handlers but not layout. Element ID changes have to be reflected in the tour targets and these tests.

## Tests

Tests use temp directories and `httptest`/real localhost listeners, never a real collection. Name prefixes select groups: `TestUnit*`, `TestIntegration*`, plus feature-named tests (`TestDelete*`, `TestTransfer*`, etc.). Update `README.md` when startup, storage, export, dependency or packaging behavior changes.

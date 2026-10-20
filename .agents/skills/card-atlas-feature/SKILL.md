---
name: card-atlas-feature
description: Add or change features in Card Atlas, the soft-inventory Go project, including implementation, regression tests, user guidance, local logging, and executable builds. Use for feature requests and behavior changes in this project; do not apply its project conventions to unrelated repositories.
---

# Card Atlas feature development

Deliver the working feature together with the guidance and failure handling a nontechnical administrator needs to use it.

## Establish the current behavior

Work from the Card Atlas checkout. Confirm `go.mod` declares `module cardatlas` and inspect any applicable `AGENTS.md`. Read the current source and relevant tests before relying on this skill's file map. Inspect `git status` and preserve unrelated work.

Turn the request into concrete acceptance criteria: who can use it, where the control belongs, its successful result, and what happens when it fails. Make routine implementation choices without unnecessary questions. Clarify only decisions that materially affect the user's data or the intended behavior.

Useful starting points, relative to the repository root:

| Concern | Source |
| --- | --- |
| Routes, authorization and response handling | `server.go`, `auth.go` |
| Catalog persistence, folder scans and metadata | `catalog.go`, `index.go` |
| Imports and conflict previews | `import.go` |
| Moves, layouts and album deletion | `rearrange.go`, `transfer.go`, `delete_album.go` |
| Image responses and preview cache | `thumbnail.go`, `thumbnail_cache.go` |
| Controls, dialogs and client state | `static/index.html`, `static/app.js`, `static/style.css` |
| Interactive getting-started tour | `static/admin-tour.js` |
| Folder-import admin guide | `static/admin-guide.md` |
| Markdown rendering and full-page guide shell | `admin_guide.go`, `static/admin-guide.html` |
| Friendly errors and local logs | `errors.go`, `logging.go` |
| Startup and executable packaging | `main.go`, `cmd/build/main.go` |

## Implement through the existing application

Use the current Go server and plain JavaScript interface. Do not introduce a frontend framework, external service, or runtime network dependency for a routine feature. Read `go.mod` before adding dependencies; document any genuine dependency or build requirement change.

Preserve these product contracts unless the user explicitly changes them:

- Standard users browse and search. Admin actions require server-side authentication as well as appropriate UI visibility. A hidden button is not authorization. Handle expired sessions without leaving usable edit controls behind.
- The view order is Grid, Table, Organized, with Grid as the default. Keep physical album pages distinct from result pagination. Read the current page sizes rather than assuming one size for every view.
- Album and box types stay fixed once saved. New unnumbered imports default to boxes; numbered imports can use paged albums. Existing destination types and an explicit import choice take precedence over automatic detection.
- Catalog data lives beside the collection's photos in `catalog.json`; settings remember the collection location. Changes must survive restart and rescan, including names, prices, tags and saved positions.
- Multi-photo transfers and album deletion require a clear confirmation. Deletion retains files in `.card-atlas-trash`. Do not turn recoverable deletion into permanent deletion as an incidental implementation change.

For mutations, follow the catalog's locking, snapshot and commit conventions. `commit` saves before replacing in-memory state and invalidates the index. Do not edit the live catalog directly to bypass those guarantees. Preserve typed errors with `%w` when adding context.

For filesystem operations, validate the full selection before changing files, prevent overwrites and path escapes, respect symlink boundaries, and restore files if saving fails. Never discard the only surviving recovery copy. Check both stored metadata and actual files when deciding whether a destination is occupied.

If data structures or inferred defaults change, inspect existing catalog loading and scanning. Add compatibility handling and a realistic old-catalog fixture when needed. Do not silently reclassify saved album types or lose metadata. A control rename, route change or response-shape change must be reflected in every affected caller and test.

## Keep both kinds of help accurate

Review this impact table for every user-visible change. Update affected guidance in the same feature; do not rewrite unrelated material just to touch every file.

| Change | Required follow-through |
| --- | --- |
| Folder connection, filename inference, scan behavior or import-folder errors | Update `static/admin-guide.md` and the corresponding tour steps |
| Another admin feature, control, workflow or confirmation | Update the getting-started tour; keep the folder guide focused on importing an existing folder |
| Element IDs, selectors, dialogs or navigation | Update tour targets, readiness checks and client tests |
| Startup, storage, support, exports, dependencies or packaging | Update the relevant `README.md` instructions |
| New error category, log destination or retention behavior | Update error handling, logging tests and affected support wording |

The admin guide's source is Markdown. End users read its rendered full-page view, not raw Markdown or a popup. Preserve authenticated access, `no-store`, safe rendering and working topic links. Do not duplicate guide content into the HTML shell. Its topic menu is generated from headings.

The getting-started tour is an interactive walkthrough, not another long manual. Add or revise steps near the relevant task:

- Name the actual control and explain the next action in plain language.
- Point `target` at a visible control. Use a target function when opening a dialog changes which element is relevant.
- Use a side-effect-free `done` predicate for steps that wait for a real action. The tour must not import, save, move or delete on the user's behalf.
- Keep Back, Skip, topic jumping and End tour usable. An empty collection, a box without a page layout, or an unavailable control must not trap the user.
- Keep tips usable inside open dialogs and on smaller screens. Stop the tour when admin access is lost. Escape should end the tour without discarding current work.

Write for an administrator who may not be technical: short steps, exact button labels, familiar words, and a clear result. Explain what happens to files before consequential actions. Avoid jargon, promotional wording and invented behavior. Use optional writing skills only if available; this workflow must remain usable without them.

## Friendly errors and diagnostic logs

Reuse `userError`, `badRequest`, `recordError`, `logEvent` and the request-logging middleware as appropriate. Preserve specific validation messages that help the user correct an input. Translate filesystem, parser and connection failures into a useful next step instead of displaying raw technical errors or absolute paths.

In the client, reuse `fetchResponse`, `responseJSON` and `userErrorMessage`. Keep intentionally canceled requests quiet. Restore disabled controls after failure, retain unsaved input or selection for retry, and prevent duplicate submissions while a mutation is running.

Logging conventions:

- Daily error and debug logs go in `.log` beside the collection catalog. Before connection, the settings directory is the fallback.
- Technical details belong in error logs; request debug entries record method, route, status and duration. Add a focused event for a new background failure when request logging cannot capture it.
- Do not log passwords, cookies, request bodies, full query strings, photo contents or entire metadata records. Avoid logging the same failure repeatedly at every layer.
- Preserve five-day retention for the logger's own files, with startup and hourly cleanup. Never let log cleanup delete photos, recovery files or unrelated files.
- Keep `.log`, caches, staging and recovery directories out of photo scans and relevant import fingerprints.
- Logging failures must not prevent normal app use. Do not call `c.logBase()` while holding `c.mu`; it takes a read lock. Record request errors and let middleware log after handler locks are released, or use a base path already captured safely.

## Verify the behavior

Add meaningful regression tests for the feature's success case and affected failure paths. Match risk to the change; do not create tests that merely assert wording or copy the implementation.

For catalog or file mutations, cover admin authorization, invalid or stale input, filename/position conflicts, save failure and rollback, restart/rescan persistence, and relevant path or symlink boundaries. Use temporary collections, never the user's collection. Test older data when compatibility behavior changes.

For UI changes, cover the actual handlers, admin/viewer state, cancellation, retry and duplicate-submit behavior. The current Node tests model DOM operations; they do not prove CSS layout. For layout or tour changes, inspect the real app in a temporary preview when permitted. Check a normal laptop viewport and relevant smaller-screen behavior. Acknowledge any browser check that could not be completed; never claim it passed based on mocked DOM tests. Respect tool approvals and do not work around a rejected browser sign-in.

Use focused tests during development, then run the relevant existing checks before delivery:

```sh
node --test tests/frontend.test.cjs
go test -race ./...
git diff --check
```

These commands are the current project checks; re-read CI if it changes. Run logging/retention tests for new error or logging paths. Run Markdown/authentication tests for guide changes and tour progression tests for modified walkthrough behavior.

If the feature changes the architecture, help split, storage, logging policy or build process, update this skill's affected guidance too. Prefer a narrow correction over accumulating stale rules. Keep skill instructions aligned with the actual project and the user's latest decisions.

## Build and deliver

The interface and guide are embedded in the executable. Editing source files does not update an already running app. When delivering a feature for the user's packaged app, complete the checks and rebuild:

```sh
go run ./cmd/build
```

The builder produces macOS and Windows executables and ZIPs in `dist`. Keep the atomic `copyExecutable` replacement: overwriting the macOS alias in place previously caused immediate termination before startup. For launch/build changes, smoke-test the actual rebuilt executable with an isolated temporary data directory and stop the preview afterward. Do not change OS security settings to make a build launch.

Tell the user what changed, where to click, how it was checked and whether to restart Card Atlas. State material limitations without a technical checklist. Do not claim a test or rebuild ran when it did not. Commit or publish only when requested; a feature request does not itself authorize either action.

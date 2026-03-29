# CLAUDE.md — simple-filestore

Context for future AI work sessions on this codebase.

## What this is

A simple self-hosted file sharing web app. The authentication model is unusual:
the folder name itself is the access key — users enter a folder name on the login
page, and if it exists in the config, they get access. There is no username.
Groups (e.g. a sports team) get one shared folder they all access with the same
folder name.

## Architecture

```
cmd/server/main.go          — entrypoint: flags, wiring, embed, ListenAndServe
internal/assets/            — embed.FS (templates + static files compiled in)
  assets.go                 — //go:embed all:templates all:static
  templates/                — HTML templates (component-based, not inherited)
    base.html               — named partials: user-header (used by browse/trash)
    browser.html            — file browser + {{define "file-list"}} HTMX partial
    trash.html              — + {{define "trash-list"}} HTMX partial
    admin.html              — + {{define "admin-folder-list"}} + {{define "admin-error"}}
    login.html, admin_login.html, preview.html, error.html
  static/
    css/output.css          — built by tailwindcss (placeholder committed)
    js/htmx.min.js          — vendored HTMX 2.0.4
    js/alpine.min.js        — vendored Alpine.js 3.14.9
internal/config/config.go   — JSON config, atomic save, auto-generates secret_key
internal/storage/storage.go — ALL filesystem ops; safeJoin path traversal guard
internal/middleware/auth.go — gorilla/securecookie session cookies
internal/handlers/          — HTTP handlers (auth, browse, trash, preview, admin)
```

## Critical design decisions

### Template system
Go's `{{block}}` inheritance doesn't work across files in a shared template set
(last-parsed definition wins). Instead, templates are self-contained HTML pages
that call named partials from base.html. HTMX partial fragments use
`{{define "name"}}...{{end}}` blocks within their page template, with globally
unique names (file-list, trash-list, admin-folder-list, admin-error).

### Path safety
Every storage operation goes through `safeJoin(root, folder, ...parts)` which
validates the folder name, joins paths, cleans, then verifies the result starts
with `folderRoot + "/"`. No storage function accepts a raw user path directly.

### Trash
Items moved to trash are stored at `workspace/folders/<folder>/.trash/<id>` with
a companion `.trash/<id>.meta` JSON file containing original path and deletion
time. `List()` always filters entries starting with `.` so trash is invisible
during normal browsing.

### Sessions
- User cookie `sf_user`: gorilla/securecookie encoded `{"f":"foldername"}`
- Admin cookie `sf_admin`: gorilla/securecookie encoded `{"a":true}`
- Both signed+encrypted with the secret_key from config.json
- `RequireUser` middleware injects folder into request context
- Handlers get folder via `middleware.FolderFromContext(ctx)`, never read cookies

### File serving
`/files/` handler uses `http.ServeContent` (supports Range requests for video
seeking). `?inline=1` sets `Content-Disposition: inline` for PDF/preview embedding.

### HTMX patterns
- Mutating actions (upload, mkdir, delete, rename, restore) respond with an HTML
  fragment of the refreshed file/trash list when `HX-Request: true`
- Full page redirect for non-HTMX requests (form fallback)
- Alpine.js for client state: upload/mkdir panels toggle, inline rename form

## Running locally

```bash
nix develop          # enter devShell with Go, tailwindcss, air
make css             # start tailwind watcher (terminal 1)
make dev             # start air hot-reload (terminal 2)
# OR without air:
go run ./cmd/server --workspace ./workspace
```

First run creates `workspace/config.json` with defaults. Change `admin_password`
in that file. Use `/admin/login` to create folders.

## Building

```bash
nix build            # produces result/bin/server
# or
make build
```

The Nix build runs `tailwindcss` in `preBuild` before `go build`.

## Key file locations

- `workspace/config.json` — admin password, port, secret key, folder list
- `workspace/folders/<name>/` — per-folder files
- `workspace/folders/<name>/.trash/` — per-folder trash
- `workspace/deleted/` — soft-deleted folders (via admin panel)

## Extending

**Adding a new page**: add handler in `internal/handlers/`, register route in
`RegisterRoutes()`, add template as self-contained HTML in
`internal/assets/templates/`. Add a `{{define "xxx"}}` block if it needs HTMX
partial refresh.

**Adding a storage operation**: add to `internal/storage/storage.go`, always
use `safeJoin` or verify paths start with the folder root.

**Changing the config schema**: update `Config` struct in `internal/config/`,
update `workspace/config.json` manually or delete it to regenerate defaults.

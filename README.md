# simple-filestore

A simple self-hosted file sharing web application. No accounts, just folder names
as access keys. Give a group a folder name, they browse and upload files.

I made this only to solve a problem I had, it's a vibe-coded application, not ready to be used by any capacity by anyone else. 

## Features

- **Folder-based access**: enter a folder name on the login page to access it
- **File browser**: browse, upload, create folders, rename, delete
- **Per-folder trash** with restore and permanent delete
- **File preview**: images, video, audio, PDF, text/code
- **Admin panel**: create and delete folders
- **Single binary**: everything embedded, no external file dependencies at runtime
- **Mobile-friendly**: responsive UI that works on phones and tablets

## Quick start

```bash
# With Nix
nix run . -- --workspace ./workspace

# Without Nix (build first)
make build
./result/simple-filestore --workspace ./workspace
```

First run creates `workspace/config.json` with default settings. Visit
`http://localhost:8080/admin/login` (default password: `changeme`) to create
your first folder.

## Configuration (`workspace/config.json`)

```json
{
  "admin_password": "your-secret-admin-password",
  "port": 8080,
  "secret_key": "auto-generated",
  "folders": ["team-name", "photos"]
}
```

Change `admin_password` before exposing the service. The `secret_key` is
auto-generated on first run and used to sign session cookies.

## Development

```bash
nix develop          # enter dev shell
make css             # start tailwind CSS watcher (terminal 1)
make dev             # start server with hot-reload via air (terminal 2)
make test            # run tests
make fmt             # format code
```

## Deployment on NixOS

Add to your NixOS configuration:

```nix
{
  inputs.simple-filestore.url = "github:artogahr/simple-filestore";

  # In your configuration:
  imports = [ inputs.simple-filestore.nixosModules.default ];

  services.simple-filestore = {
    enable = true;
    port = 8080;
    workspaceDir = "/var/lib/simple-filestore";
  };
}
```

The workspace directory is where config and folder data are stored. Back up by
copying the entire workspace directory.

## Project structure

```
cmd/server/         — main entrypoint
internal/
  assets/           — embedded templates + static files (CSS, HTMX, Alpine.js)
  config/           — config loading/saving
  handlers/         — HTTP handlers
  middleware/       — session cookie auth
  storage/          — filesystem operations with path traversal protection
workspace/          — runtime data (gitignored)
  config.json
  folders/          — user folders
  deleted/          — soft-deleted folders
```

See `CLAUDE.md` for architecture notes (primarily for AI-assisted development).

## Tech stack

- **Go 1.22+** — backend, single binary
- **HTMX 2.0** — server-side interactivity without a JS framework (vendored)
- **Alpine.js 3** — small client-side interactions (vendored)
- **Tailwind CSS** — utility-first styling (built at compile time via standalone CLI)
- **gorilla/securecookie** — signed+encrypted session cookies

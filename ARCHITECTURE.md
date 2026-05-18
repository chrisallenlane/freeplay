# Architecture

Freeplay is a single Go binary that embeds all frontend and emulator assets
at compile time. There is no database -- the filesystem is the only source of
truth.

## How it works

On startup, Freeplay:

1. Reads `freeplay.toml` from the data directory
2. Scans each configured ROM directory and builds an in-memory game catalog
3. Starts an HTTP server that serves the frontend, emulator, ROMs, and API

The initial scan completes synchronously before the HTTP listener starts,
so handlers never observe an empty catalog. If cover art is configured,
missing covers are fetched from IGDB in the background after each scan,
and a follow-up scan picks up the new images.

## Data flow

```
Browser ──► HTTP Server ──► Embedded frontend (HTML/JS/CSS)
                │
                ├──► /api/games          ──► In-memory catalog (built by scanner)
                ├──► /api/game-details   ──► Filesystem: <data>/cache/igdb/
                ├──► /api/saves/...      ──► Filesystem: <data>/saves/
                ├──► /api/rescan         ──► Triggers scanner
                ├──► /roms/...           ──► Filesystem: configured ROM directories
                ├──► /bios/...           ──► Filesystem: configured BIOS files
                ├──► /covers/...         ──► Filesystem: <data>/covers/
                ├──► /cache/igdb/...     ──► Filesystem: <data>/cache/igdb/
                ├──► /manuals/...        ──► Filesystem: <data>/manuals/
                └──► /emulatorjs/...     ──► Embedded EmulatorJS assets
```

Everything the server needs is either embedded in the binary or read from the
data directory. There are no background processes, task queues, or external
service dependencies (IGDB is optional and only used during scans).

## Embedded assets

The `frontend/` and `emulatorjs/` directories are embedded into the binary at
compile time via Go's `embed` package (see `embed.go`). This means the
compiled binary is fully self-contained -- no runtime file dependencies
beyond the data directory.

The `emulatorjs/` tree is fetched at build time from the upstream
[EmulatorJS release](https://github.com/EmulatorJS/EmulatorJS/releases)
pinned in the `Makefile` (tag and SHA-256). The pinned version contains
the controller-port-device patches that power lightgun support
(EmulatorJS PR #1182, RetroArch PR #38), so the player page loads
the standard `emulator.min.js` bundle.

## API

All API routes are internal to the frontend. They are not versioned and may
change without notice.

| Method | Path                                 | Cache-Control              | Description                                              |
|--------|--------------------------------------|----------------------------|----------------------------------------------------------|
| `GET`  | `/api/health`                        | `no-store`                 | Health check -- returns `{"status":"ok"}`                |
| `GET`  | `/api/games`                         | `no-store`                 | Full game catalog (consoles + games list)                |
| `GET`  | `/api/status`                        | `no-store`                 | IGDB fetch status (`{"fetchingDetails":bool}`)           |
| `GET`  | `/api/game-details`                  | `private, max-age=300`     | IGDB metadata for a single game (`?console=&rom=`)       |
| `POST` | `/api/rescan`                        | `no-store`                 | Trigger a ROM directory rescan                           |
| `GET`  | `/api/saves/{console}/{game}/{type}` | `no-store`                 | Download a save file (`type`: `state` or `sram`); 404 for missing, 5xx for existing-but-unreadable |
| `POST` | `/api/saves/{console}/{game}/{type}` | `no-store`                 | Upload a save file (64 MiB max)                          |

## Static file routes

| Path                      | Source                         | Cache-Control                              |
|---------------------------|--------------------------------|--------------------------------------------|
| `/roms/{console}/{file}`  | Configured ROM directory       | `public, max-age=31536000, immutable`      |
| `/bios/{console}`         | Configured BIOS file           | `public, max-age=31536000, immutable`      |
| `/covers/{rest...}`       | `<data>/covers/`               | `public, max-age=31536000`                 |
| `/cache/igdb/{rest...}`   | `<data>/cache/igdb/`           | `public, max-age=31536000`                 |
| `/manuals/{rest...}`      | `<data>/manuals/`              | `public, max-age=31536000`                 |
| `/emulatorjs/...`         | Embedded EmulatorJS assets     | `public, max-age=31536000, immutable`      |
| `/details`                | Embedded details page          | `no-cache`                                 |
| `/play`                   | Embedded player page           | `no-cache`                                 |
| `/`                       | Embedded frontend (catch-all)  | `no-cache`                                 |

ROMs, BIOS files, and EmulatorJS assets use `immutable` because their URLs
are stable and the bytes never change behind a given path. Covers, cached
IGDB images, and manuals use a long max-age without `immutable` so browsers
revalidate via `If-Modified-Since` once the TTL expires — these files can be
rewritten after an IGDB rescan or a manual update without changing their URLs.
The frontend, details page, and player page use `no-cache` so that
redeployments are picked up immediately.

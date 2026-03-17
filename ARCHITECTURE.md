# Architecture

Freeplay is a single Go binary that embeds all frontend and emulator assets
at compile time. There is no database -- the filesystem is the only source of
truth.

## How it works

On startup, Freeplay:

1. Reads `freeplay.toml` from the data directory
2. Scans each configured ROM directory and builds an in-memory game catalog
3. Starts an HTTP server that serves the frontend, emulator, ROMs, and API

The scan runs asynchronously. If cover art is configured, missing covers are
fetched from IGDB after each scan, and a follow-up scan picks up the new
images.

## Data flow

```
Browser ──► HTTP Server ──► Embedded frontend (HTML/JS/CSS)
                │
                ├──► /api/games       ──► In-memory catalog (built by scanner)
                ├──► /api/saves/...   ──► Filesystem: <data>/saves/
                ├──► /api/rescan      ──► Triggers scanner
                ├──► /roms/...        ──► Filesystem: configured ROM directories
                ├──► /bios/...        ──► Filesystem: configured BIOS files
                ├──► /covers/...      ──► Filesystem: <data>/covers/
                └──► /emulatorjs/...  ──► Embedded EmulatorJS assets
```

Everything the server needs is either embedded in the binary or read from the
data directory. There are no background processes, task queues, or external
service dependencies (IGDB is optional and only used during scans).

## Embedded assets

The `frontend/` and `emulatorjs/` directories are embedded into the binary at
compile time via Go's `embed` package (see `embed.go`). This means the
compiled binary is fully self-contained -- no runtime file dependencies
beyond the data directory.

The player page sets `EJS_DEBUG_XX = true`, which tells EmulatorJS to load
its unminified source files instead of `emulator.min.js`. This is required
because the vendored `emulator.min.js` does not include the controller port
device patches (lightgun support). The individual source files in
`emulatorjs/data/src/` do contain these patches.

## API

All API routes are internal to the frontend. They are not versioned and may
change without notice.

| Method | Path                                 | Description                                       |
|--------|--------------------------------------|---------------------------------------------------|
| `GET`  | `/api/health`                        | Health check -- returns `{"status":"ok"}`         |
| `GET`  | `/api/games`                         | Full game catalog (consoles + games list)         |
| `GET`  | `/api/status`                        | Cover art fetch status (`{"fetchingCovers":bool}`) |
| `POST` | `/api/rescan`                        | Trigger a ROM directory rescan                    |
| `GET`  | `/api/saves/{console}/{game}/{type}` | Download a save file (`type`: `state` or `sram`)  |
| `POST` | `/api/saves/{console}/{game}/{type}` | Upload a save file (64 MB max)                    |

## Static file routes

| Path                     | Source                        |
|--------------------------|-------------------------------|
| `/roms/{console}/{file}` | Configured ROM directory      |
| `/bios/{console}`        | Configured BIOS file          |
| `/covers/{rest...}`      | `<data>/covers/`              |
| `/emulatorjs/...`        | Embedded EmulatorJS assets    |
| `/play`                  | Embedded player page          |
| `/`                      | Embedded frontend (catch-all) |

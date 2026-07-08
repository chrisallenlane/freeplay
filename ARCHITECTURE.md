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

## EmulatorJS volume workaround

The player page ships a frontend module, `frontend/masterVolume.js`, that
works around a bug in EmulatorJS's volume control. It is worth understanding
before touching anything audio-related.

**The bug.** EmulatorJS's volume slider and mute button both funnel through
`emulator.setVolume()`, which changes volume only by walking the Emscripten
OpenAL source list (`Module.AL.currentCtx.sources`). But the RetroArch cores
EmulatorJS ships do not use OpenAL at runtime — they use RetroArch's
`rwebaudio` driver, which streams `AudioBufferSourceNode`s connected directly
to `AudioContext.destination`, and `Module.AL.currentCtx` stays null. So
`setVolume` iterates an empty list: the slider moves, mute toggles its icon,
and the audio never changes.

**Why we work around it instead of fixing it upstream.** This was chased to
ground: the latest EmulatorJS release (`v4.3.0-pre`, pinned in the `Makefile`)
and upstream `main` both leave `setVolume` OpenAL-only; the historical volume
fix (PR #903, `window.AL` → `Module.AL`) is already present; and forcing
`audio_driver = "openal"` in `retroarch.cfg` does not make the core engage
OpenAL — it stays on `rwebaudio` (tested live). There is no configuration or
version bump that fixes it.

**How the workaround works.** A browser has exactly one audio sink,
`AudioContext.destination`, and every driver must connect to it to make sound.
`masterVolume.js` patches `AudioNode.prototype.connect` (at module load, before
the core boots) so any connection to `destination` is rerouted through a single
per-context "master" `GainNode`:

```
source ─► destination      becomes      source ─► masterGain ─► destination
```

A gain of 1.0 is a transparent passthrough, so the insertion cannot break any
driver's audio — an unknown future driver simply gains a working volume
control. `setVolume` is then wrapped so the slider and mute button drive the
master gain.

**Startup timing.** The wrap is not applied the instant `emulator.setVolume`
exists. EmulatorJS's `setVolume` dereferences `this.Module.AL` *without*
optional chaining, and the Emscripten `Module` can attach a beat after
`setVolume` is created (inside `createBottomMenuBar`, after the `start` event).
Wrapping too early — specifically the initial gain-sync the wrapper performs to
align the master node with the slider's starting position — would call through
to `setVolume` before `Module` exists and throw `Cannot read properties of
undefined (reading 'AL')`. So `wrapSetVolume` no-ops until *both*
`emulator.setVolume` and `emulator.Module` are present, and the bounded poll in
`installMasterVolume` keeps retrying until they are. This is a guard against a
startup-ordering crash, distinct from the double-attenuation guard below.

**The one correctness guard.** The only real hazard is double attenuation: a core that
*did* use OpenAL would have its gains scaled by EmulatorJS *and* by our master,
giving volume². The wrapper guards against this behaviorally — it applies the
master gain only when no active OpenAL context is present
(`Module.AL.currentCtx`), otherwise it leaves the master at unity and lets
EmulatorJS keep control. This is safe for unknown drivers (transparent when
idle, working when needed) and avoids a brittle `is-rwebaudio` whitelist that
would fail closed for any third driver.

The graph interception is browser-only and verified live; the decision and
wiring logic are unit-tested in `frontend/masterVolume_test.js`.

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

| Path                     | Source                        | Cache-Control           |
|--------------------------|-------------------------------|-------------------------|
| `/roms/{console}/{file}` | Configured ROM directory      | `public, max-age=86400` |
| `/bios/{console}`        | Configured BIOS file          | `public, max-age=86400` |
| `/covers/{rest...}`      | `<data>/covers/`              | `public, max-age=86400` |
| `/cache/igdb/{rest...}`  | `<data>/cache/igdb/`          | `public, max-age=86400` |
| `/manuals/{rest...}`     | `<data>/manuals/`             | `public, max-age=86400` |
| `/emulatorjs/...`        | Embedded EmulatorJS assets    | `public, max-age=86400` |
| `/details`               | Embedded details page         | `no-cache`              |
| `/play`                  | Embedded player page          | `no-cache`              |
| `/`                      | Embedded frontend (catch-all) | `no-cache`              |

All long-cache static routes share `public, max-age=86400` (24 hours).
`immutable` is intentionally absent project-wide: it blocks revalidation
even on hard refresh, so a broken release that ships malformed bytes at a
stable URL would pin browsers on the broken copy until cache eviction. The
24-hour TTL bounds worst-case staleness for any file behind a stable URL
that legitimately changes — cover rescans, BIOS swaps, an EmulatorJS bump
— while still keeping repeat-navigation traffic off the wire on a LAN.

`/emulatorjs/*` additionally carries a version-stamped `ETag`, so a
Freeplay release invalidates cached client copies via `If-None-Match`
(returning 304 when the bundle hasn't changed) without forcing a full
re-download.

The frontend, details page, and player page use `no-cache` so that
redeployments are picked up immediately. The panic-recovery 500 path
strips `Cache-Control`, `ETag`, `Last-Modified`, and `Content-Encoding`
before emitting the error, so a transient 500 on a long-cached route is
not itself cached for the full max-age window.

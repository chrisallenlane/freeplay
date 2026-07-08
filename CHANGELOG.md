# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Changed
- The emulator's "Save State Location" setting (Download / Keep in
  Browser) is now hidden. Freeplay routes the bottom-bar Save/Load
  buttons to the server via the `EJS_onSaveState` / `EJS_onLoadState`
  hooks, which short-circuit before that setting is ever consulted — so
  the dropdown had no effect on where a state was stored and only
  implied a "Keep in Browser" choice that no longer existed (it was also
  the trigger for the v1.1.3 load-state crash). Hidden via
  `EJS_hideSettings`; the still-functional "Save State Slot" setting,
  which the quick-save hotkeys and context menu read, is retained.

### Fixed
- The master-volume workaround no longer throws an uncaught
  `TypeError` at startup. `wrapSetVolume` did its initial gain-sync as
  soon as `emulator.setVolume` existed, but EmulatorJS's `setVolume`
  dereferences `this.Module.AL` without optional chaining and
  `Module` can attach a beat later — so the sync threw "Cannot read
  properties of undefined (reading 'AL')". It was benign (the wrap was
  already in place, later slider calls worked, and startup is at unity
  gain), but noisy. `wrapSetVolume` now also waits for `emulator.Module`
  before wrapping, and the bounded poll keeps retrying until it lands.

## [1.1.4] - 2026-07-08

### Fixed
- CI: the Gitea build now publishes `:latest` from the **tag** build
  instead of the master build. Previously `:latest` tracked master HEAD
  and was pushed by the master-branch build, which — during a release,
  when the release commit and the tag are pushed moments apart — races a
  concurrent same-SHA tag build for the runner and has intermittently
  died, leaving `:latest` stale (e.g. it did not advance to v1.1.3). It
  now rides the single, reliable tag build and tracks the newest release
  rather than master HEAD. Plain master pushes publish only `:sha-<short>`.

## [1.1.3] - 2026-07-08

### Changed
- The bottom-bar "save state" button now shows a confirmation message.
  It always posted to the server, but EmulatorJS suppresses its own
  "SAVED STATE" message once a `saveState` listener is registered, so
  the button appeared to do nothing. Save and load now both surface a
  transient status message via the emulator's overlay.

### Fixed
- The volume slider and mute button now work. EmulatorJS's `setVolume`
  changes volume only through the Emscripten OpenAL driver, but the
  RetroArch cores it ships use the `rwebaudio` driver instead — so the
  slider moved and mute toggled its icon while the audio never changed.
  There is no upstream or config fix (forcing `audio_driver = "openal"`
  does not engage OpenAL). A new frontend module (`frontend/masterVolume.js`)
  splices a master `GainNode` into the WebAudio graph and drives it from
  the existing slider + mute, guarded to stay transparent for any core
  that does use OpenAL. See ARCHITECTURE.md § "EmulatorJS volume workaround".
- The bottom-bar "load state" button no longer crashes the emulator.
  Freeplay routes save states to the server (via `EJS_onSaveState` /
  `EJS_loadStateURL`) but never registered a `loadState` listener, so
  the button fell through to EmulatorJS's built-in behaviour. With the
  "Save State Location" setting on "Keep in Browser" — but the state
  living on the server, not in browser storage — that path called
  `gameManager.loadState(undefined)`, which aborts the WASM runtime
  (`Aborted(Unsupported data type)`) and required a page reload to
  recover. A new `EJS_onLoadState` handler (`frontend/loadState.js`)
  restores the state from the server, mirroring the save direction, and
  registering it also short-circuits the crashing built-in path.

## [1.1.2] - 2026-05-19

### Changed
- `Cache-Control: immutable` has been removed from all static file
  routes (`/roms/`, `/bios/`, `/covers/`, `/manuals/`, `/cache/igdb/`,
  `/emulatorjs/`). Browsers can now recover from cached-but-broken
  assets via a hard refresh; previously, `immutable` blocked
  revalidation even on hard refresh and a broken bundle could trap
  clients on stale bytes. The long-cache `max-age` is also cut from
  365 days to 24 hours to bound staleness.
- All long-cache static routes now share a single `longCache`
  directive (`public, max-age=86400`). The previous distinction
  between `longCacheImmutable` (ROMs, BIOS, EmulatorJS) and
  `longCacheMutable` (covers, IGDB cache, manuals) is gone.
- `/api/game-details` is now wrapped in `noStore` so its error
  responses (404 for missing console/rom, 400 for invalid
  parameters) carry `Cache-Control: no-store` rather than no
  header. The success path retains its `private, max-age=300`
  override.

### Fixed
- Panic-recovery 500 responses now strip `Cache-Control`, `ETag`,
  `Last-Modified`, and `Content-Encoding` headers before emitting
  the error. Pre-fix, a transient panic on a route wrapped in
  long-cache middleware would cache the 500 for the full max-age
  window — Go's `http.Error` does not clear these headers (only the
  unexported `http.serveError` used by `FileServerFS` does), so the
  fix is to clear them explicitly via a shared
  `clearLongCacheHeaders` helper.

## [1.1.1] - 2026-05-18

### Changed
- EmulatorJS dependency switched from the `chrisallenlane/EmulatorJS`
  fork to upstream `EmulatorJS/EmulatorJS@v4.3.0-pre`. The
  controller-port-device patches that power lightgun support were
  merged upstream (EmulatorJS PR #1182, RetroArch PR #38), so Freeplay
  no longer needs to ship a fork or force unminified asset loading via
  `EJS_DEBUG_XX`. The player page now loads the standard
  `emulator.min.js` bundle.

### Fixed
- `/emulatorjs/*` responses now carry a version-stamped `ETag` and no
  longer use the `immutable` Cache-Control directive. Browsers
  revalidate via `If-None-Match` on every Freeplay release, so an
  upstream EmulatorJS bump can change file bytes at stable URLs
  without trapping clients on stale cached copies.

## [1.1.0] - 2026-05-12

### Added
- `-port` CLI flag overrides the port from the config file (`0` = use
  config value)
- `LOG_LEVEL` environment variable for slog level
  (`debug`/`info`/`warn`/`error`, case-insensitive; default `info`;
  unrecognised values fall back to `info` with a one-time warning)
- Request-log middleware emits one `slog.Info` per response (method,
  path, status, bytes, ms)
- Panic-recovery middleware normalises handler panics to a 500 response
  with a structured `slog.Error` (method, path, panic value, stack)
- Integration test suite under `internal/integration` (build-tagged,
  run via `make integration`)
- Frontend↔server contract test for the save round-trip
  (`frontend/contract_test.js`)
- Fuzz coverage for `datadir.PathInside` and `igdb.safeIGDBInfoURL`
- `CONTRIBUTING.md`

### Changed
- `GET /api/saves/...` now returns 5xx (was 404) when a save file
  exists but cannot be read. The frontend uses this distinction to
  refuse periodic-auto-save registration when a real save is present
  but unreadable, rather than overwriting it
- `library.Start` performs the first scan synchronously before HTTP
  serves traffic, closing a cold-start race window where legitimate
  save GETs 404'd and POSTs were silently dropped before
  `MaxBytesReader`
- Frontend SRAM and state probes now branch on response status:
  404 registers the periodic save (legitimate fresh game); any other
  non-2xx leaves it unregistered and emits `console.error` so
  operators can correlate against the server-side `slog.Warn`
- `frontend/postSave` no-ops on empty buffers instead of overwriting,
  closing a save-loss path
- Save-endpoint gate aligns with the URL slug convention via
  `HasGameSlug`

### Fixed
- `details.writeNotFound` and `ensureCoverThumbnail` log
  `atomicfile.Write` failures at warn level instead of swallowing
  them silently. Without these logs, persistent disk-full or
  permission errors caused unbounded IGDB re-fetches and permanent
  missing covers with no operator-visible diagnostic
- `noDirListing` clears `Cache-Control`, `Etag`, `Last-Modified`, and
  `Content-Encoding` headers before returning a 404 for a directory
  without `index.html`. Pre-fix, browsers cached the 404 immutably
  for a year because the outer `cacheControl(longCacheImmutable, ...)`
  middleware stamped the header before `http.NotFound` ran
- `gameDetailsFromIGDB` skips `InvolvedCompanies` and `Platforms`
  entries with empty names. Pre-fix, IGDB responses with an unnamed
  company subobject produced leading-comma artifacts ("Developer: ,
  Capcom") on rendered detail pages
- `frontend/app.js` rescan flow now clears `statusPollTimer` in both
  terminal branches of `pollCoverStatus` and returns the inner
  promise so the click handler's `.finally` safety-reset correctly
  waits for the poll cycle to settle. Pre-fix, a second rescan that
  hit a 409 or a network failure on the POST left the button stuck
  disabled in "Scanning…" until manual page reload
- IGDB nil-fetcher initialisation no longer produces a typed-nil
  interface that escapes the `c.fetcher == nil` guard and would
  crash the background pipeline
- `make test` now builds the binary before integration/contract tests
  in CI so `dist/freeplay` exists when the contract test runs

## [1.0.0] - 2026-04-05

### Added
- Game details page with IGDB metadata, screenshots, and artwork
- Details page is now the default landing when clicking a game card
- Per-game PDF manual support with "View Manual" button on details page
- Local IGDB metadata and image cache (no repeat API calls after first fetch)
- IGDB game names displayed on library index cards
- Game metadata displayed as a table on details page (year, developer,
  publisher, platforms, series)
- Sticky header and toolbar on all pages
- Fuzz testing infrastructure (`make fuzz`, `make fuzz-long`)
- Accessibility audit target (`make a11y`)
- CSRF protection on state-changing endpoints
- Benchmark suite and `make bench` target for critical-path regression testing
- `make build-debug` produces a debug binary with pprof on `127.0.0.1:6060`
  (gated behind `//go:build debug`; not present in production builds)

### Changed
- Clicking a game card navigates to the details page instead of launching
  the emulator directly
- Page titles now use IGDB game names when available
- Improved typography and visual hierarchy on the details page
- Back-to-library link styled as a button on subpages
- IGDB screenshots and artworks now cache two variants: a `t_screenshot_huge`
  thumbnail and a `t_original` full-size image; wire format changed from
  `[]string` to `[]ImageRef{URL, ThumbURL}` (legacy string arrays still
  accepted via backward-compatible `UnmarshalJSON`)
- Cache-Control split: `/covers/`, `/cache/igdb/`, and `/manuals/` use
  `public, max-age=31536000` (no `immutable`) so browsers revalidate after
  TTL expiry; `/emulatorjs/`, `/roms/`, and `/bios/` retain `immutable`
- `/api/game-details` uses `private, max-age=300` to deduplicate in-session
  navigation requests; all other `/api/*` endpoints use `no-store`
- Gzip compression middleware: text responses (JSON, HTML, CSS, JS, WASM)
  are compressed when the client sends `Accept-Encoding: gzip`; binary routes
  (save blobs, ROM files, images) pass through uncompressed
- Details cache now keeps a per-process in-memory layer; steady-state rescans
  avoid per-game disk reads for already-cached entries
- Scanner preallocates the games slice from the previous scan count and uses
  `slices.SortFunc` (reduced allocations on repeated scans)

### Fixed
- IGDB matching regression for games with diacritical characters in titles
- Paragraph breaks in IGDB text now render correctly on the details page
- WCAG 2.2 Level AA accessibility issues across all pages (contrast, focus
  indicators, ARIA attributes, semantic structure)
- Multiple bugs found via proactive bug hunt and security audit

## [0.1.0] - 2026-03-16

Initial release.

### Added
- Browser-based retro gaming via EmulatorJS
- ROM scanning with configurable console definitions (TOML)
- Server-side save state and battery save persistence
- SRAM save loading from server on game start
- Cover art fetching from IGDB with platform filtering and name variant
  fallbacks
- Game library with console filtering, search, and favorites
- Autofocus on search box on page load
- Light/dark theme with system preference detection and manual toggle
- Gamepad navigation (D-pad, shoulder buttons, A/Start)
- Keyboard navigation (arrow keys, `[`/`]` filter cycling)
- Lightgun support (SNES Super Scope and others)
- BIOS file support for consoles that require it (e.g. PlayStation)
- Responsive layout for mobile, tablet, and desktop
- Single-binary deployment or Docker container with one volume mount
- Rescan endpoint with cover art download progress indicator
- Cache-Control headers: `no-cache` on frontend for immediate deploy pickup,
  immutable long-cache on EmulatorJS, ROMs, BIOS, and cover art
- Performance: `defer` script loading, Silkscreen font preload, explicit
  `width`/`height` on cover art images, O(1) cover-detection via directory
  map lookup

### Changed
- BIOS configuration is now an optional `bios` field on each `[roms.*]`
  entry rather than a separate top-level `[bios]` section. See
  `INSTALLING.md` for the new format.

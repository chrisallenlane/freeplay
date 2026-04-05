# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

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

### Changed
- Clicking a game card navigates to the details page instead of launching
  the emulator directly
- Page titles now use IGDB game names when available
- Improved typography and visual hierarchy on the details page
- Back-to-library link styled as a button on subpages

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

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

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
- Keyboard navigation (arrow keys, `[`/`]` filter cycling, skip links)
- Lightgun support (SNES Super Scope and others)
- BIOS file support for consoles that require it (e.g. PlayStation)
- Responsive layout for mobile, tablet, and desktop
- Single-binary deployment or Docker container with one volume mount
- Rescan endpoint with cover art download progress indicator
- Cache-Control headers on frontend assets for immediate deploy pickup

### Changed
- BIOS configuration is now an optional `bios` field on each `[roms.*]`
  entry rather than a separate top-level `[bios]` section. See
  `INSTALLING.md` for the new format.

# Freeplay

Retro-gaming server that serves ROMs via EmulatorJS in the browser.

## Build & Test

```
make check    # fmt + lint + vet + test
make build    # build binary to dist/
make run      # build and run with testdata
make setup    # install dev tools (gofumpt, revive)
```

`make fmt`, `make lint`, and `make test` cover both Go and frontend (JS/HTML)
sources. The frontend tools run via `npx` (Node.js/npm required).

## Vendored Directories — Do Not Modify

- `emulatorjs/` — Vendored EmulatorJS assets (JS, WASM, CSS). Embedded at
  build time via `//go:embed`. Do not modify, reformat, lint, or refactor
  files in this directory.
- `vendor/` — Go module dependencies managed by `go mod vendor`. Do not
  modify directly.

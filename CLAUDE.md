# Freeplay

Retro-gaming server that serves ROMs via EmulatorJS in the browser.

## Build & Test

```
make check            # fmt + lint + vet + test
make build            # build binary to dist/
make run              # build and run with testdata
make fuzz             # run fuzz tests (15s each)
make fuzz-long        # run fuzz tests (10m each)
make audit            # govulncheck + gosec security scan
make a11y             # run accessibility audit against live server
make fetch-emulatorjs # download pinned EmulatorJS assets (auto-invoked by build/check)
make setup            # install dev tools (gofumpt, golangci-lint, govulncheck, gosec)
```

`make fmt`, `make lint`, and `make test` cover both Go and frontend (JS/HTML)
sources. The frontend tools run via `npx` (Node.js/npm required).

## Fetched & Vendored Directories — Do Not Modify

- `emulatorjs/` — EmulatorJS assets (JS, WASM, CSS) fetched at build time
  from a pinned GitHub release (see `make fetch-emulatorjs`). Gitignored.
  Embedded into the binary via `//go:embed`. Do not modify, reformat, lint,
  refactor, or commit files in this directory.
- `vendor/` — Go module dependencies managed by `go mod vendor`. Do not
  modify directly.

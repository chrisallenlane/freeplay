# Hacking on Freeplay

## Prerequisites

- Go 1.26 or later (<https://go.dev/dl/>)
- Node.js and npm — required for JavaScript formatting, linting, and testing
- [gofumpt][] (formatter) and [golangci-lint][] (linter) — install both with:

```bash
make setup
```

## Project layout

```
cmd/freeplay/       CLI entrypoint
internal/
  atomicfile/       Atomic file writes
  config/           TOML config loading and validation
  details/          IGDB details caching and fetch orchestration
  igdb/             IGDB API client, name cleaning, and variant generation
  saves/            Save-state persistence
  scanner/          ROM directory scanning and catalog building
  server/           HTTP server and API routes
frontend/           Static HTML/JS/CSS served by the app
emulatorjs/         EmulatorJS assets (gitignored, fetched at build time)
vendor/             Go module vendor directory (do not modify)
testdata/           Local test data directory (gitignored)
```

## Common commands

Run `make help` for the full list. The most useful targets:

| Command              | Description                             |
|----------------------|-----------------------------------------|
| `make check`         | Format, lint, vet, and test             |
| `make build`         | Build binary to `dist/freeplay`         |
| `make run`           | Build and run against `testdata/`       |
| `make fmt`           | Format Go (gofumpt) and JS (Biome)      |
| `make lint`          | Lint Go (golangci-lint), JS and HTML (Biome/html-validate) |
| `make test`          | Run Go and JS unit tests                |
| `make coverage`      | Generate HTML coverage report           |
| `make coverage-text` | Show per-function coverage in terminal  |
| `make a11y`          | Run accessibility audit against live server |
| `make fuzz`          | Run fuzz tests (15s each)               |
| `make fuzz-long`     | Run fuzz tests (10m each)               |
| `make vendor`        | Tidy and re-vendor Go dependencies      |
| `make vendor-update` | Update all dependencies then re-vendor  |
| `make docker`        | Build the Docker image                  |
| `make clean`         | Remove build artifacts and temp files   |

## Setting up test data

The `make run` target passes `./testdata` as the data directory. There's
nothing special about this path — it's just a convenient convention used by
the Makefile. Create the directory with a `freeplay.toml` and at least one
ROM directory. A minimal setup:

```
testdata/
  freeplay.toml
  roms/
    nes/
      SomeGame.zip
```

A minimal `freeplay.toml` for local development:

```toml
port = 8080

[roms.NES]
path = "roms/nes"
core = "fceumm"
```

ROM paths in the config are resolved relative to the data directory
(`testdata/` when using `make run`). Cover art configuration is optional and
can be omitted for local development.

## Running the server

```bash
make run
```

This builds the binary and starts the server at `http://localhost:8080` using
`testdata/` as the data directory.

To run with a different data directory:

```bash
make build
./dist/freeplay -data /path/to/data
```

## External assets

Two directories must not be modified or committed by hand:

- **`emulatorjs/`** — EmulatorJS assets (JS, WASM, CSS) fetched at build
  time from a pinned GitHub release on the [patched
  fork](https://github.com/chrisallenlane/EmulatorJS/releases). `make build`
  and `make check` auto-invoke `make fetch-emulatorjs`, which downloads the
  tarball, verifies the pinned SHA-256, and extracts to `emulatorjs/`. The
  pinned version and checksum live at the top of the `Makefile`. Gitignored.
- **`vendor/`** — Go module dependencies managed by `go mod vendor`. Use
  `make vendor` or `make vendor-update` to modify.

## Embedded assets

The frontend and EmulatorJS assets are embedded into the binary at compile
time using Go's `embed` package (see `embed.go`). Changes to files in
`frontend/` take effect on the next `make build`. To change the pinned
EmulatorJS version, update `EMULATORJS_VERSION` and `EMULATORJS_SHA256` in
the `Makefile`, then delete `emulatorjs/` and run `make fetch-emulatorjs`.

## Docker

To build and run via Docker:

```bash
make docker
docker compose up
```

Or in one step:

```bash
docker compose up --build
```

[gofumpt]: https://github.com/mvdan/gofumpt
[golangci-lint]: https://github.com/golangci/golangci-lint

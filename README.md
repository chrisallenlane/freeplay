# Freeplay

A self-hosted retro game emulation web application. Freeplay serves ROM files
through a browser-based emulator ([EmulatorJS](https://github.com/EmulatorJS/EmulatorJS))
and handles save states server-side so your progress persists across sessions.

## Quick Start (Docker)

```bash
# Copy and edit the example config into your data directory
cp freeplay.example.toml /path/to/your/games/freeplay.toml

# Run with docker-compose
docker compose up
```

The data directory (`/path/to/your/games`) must contain `freeplay.toml` and
your ROM files. The server listens on port 8080 by default.

## Installation

### Docker (recommended)

```bash
docker compose up --build
```

Edit `docker-compose.yml` to set the correct path to your data directory:

```yaml
volumes:
  - /path/to/your/games:/data
```

### Build from source

Prerequisites: Go 1.24+, `p7zip` (for EmulatorJS extraction).

```bash
make build   # downloads EmulatorJS and compiles the binary
./freeplay -data /path/to/data
```

## Configuration

Freeplay reads `freeplay.toml` from the data directory (`/data` in Docker).
See `freeplay.example.toml` for a fully annotated example.

### Options

| Key | Default | Description |
|-----|---------|-------------|
| `port` | `8080` | HTTP listen port |
| `cover_art_api` | *(empty)* | Cover art provider. Only `"igdb"` is supported. |
| `cover_art_api_key` | *(empty)* | API key in `client_id:client_secret` format (required when `cover_art_api` is set) |

### ROM directories

Each `[roms.<Name>]` section maps a console display name to a directory and an
EmulatorJS core:

```toml
[roms.NES]
path = "roms/nes"    # relative to the data directory
core = "fceumm"

[roms."Super Nintendo"]
path = "roms/snes"
core = "snes9x"
```

Common cores: `fceumm` (NES), `snes9x` (SNES), `genesis_plus_gx` (Genesis),
`mgba` (GBA), `gambatte` (Game Boy), `mupen64plus_next` (N64),
`pcsx_rearmed` (PlayStation), `fbneo` (arcade), `stella2014` (Atari 2600).

### BIOS files

Some consoles (e.g. PlayStation) require BIOS files. Map console names to
directories containing the appropriate files:

```toml
[bios]
PlayStation = "bios/ps1"
```

### Cover art

When `cover_art_api = "igdb"` is set, Freeplay automatically fetches cover art
from [IGDB](https://www.igdb.com/) after each ROM scan. This requires a free
Twitch developer application at <https://dev.twitch.tv/console>.

Set `cover_art_api_key` to `"your_client_id:your_client_secret"`.

Cover images are cached as PNG files under `<data>/covers/<console>/`.

## Data directory layout

```
/data/
  freeplay.toml
  roms/
    nes/
      Mega Man.nes
  bios/
    ps1/
      scph1001.bin
  covers/          # auto-populated by Freeplay
    NES/
      Mega Man.png
  saves/           # auto-populated by Freeplay
    NES/
      Mega Man/
        state
        sram
```

## API reference

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Health check — returns `{"status":"ok"}` |
| `GET` | `/api/games` | Full game catalog (consoles + games list) |
| `POST` | `/api/rescan` | Trigger a ROM directory rescan |
| `GET` | `/api/saves/{console}/{game}/{type}` | Download a save file (`type`: `state` or `sram`) |
| `POST` | `/api/saves/{console}/{game}/{type}` | Upload a save file |
| `GET` | `/roms/{console}/{file}` | Serve a ROM file |
| `GET` | `/bios/{console}/{file}` | Serve a BIOS file |
| `GET` | `/covers/{console}/{file}` | Serve a cover art image |

## Development

```bash
make check   # format, vet, and test
make run     # build and run against ./testdata
make docker  # build Docker image
```

Tests require no external dependencies and run entirely in-process.

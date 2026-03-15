# Installing Freeplay

## Prerequisites

Before installing Freeplay, you need:

1. A directory containing your ROM files, organized by console (e.g.
   `roms/nes/`, `roms/snes/`)
2. A `freeplay.toml` configuration file (see [Configuration](#configuration)
   below)

## Docker (recommended)

Freeplay is published to the GitHub Container Registry at
`ghcr.io/chrisallenlane/freeplay`.

### 1. Create your data directory

Create a directory that will hold your config, ROMs, and (optionally) BIOS
files:

```
/path/to/data/
  freeplay.toml
  roms/
    nes/
      Mega Man.zip
      Super Mario Bros.zip
    snes/
      Chrono Trigger.zip
  bios/           # only needed for consoles that require BIOS files
    ps1/
      scph1001.bin
```

### 2. Create your configuration file

Copy the example config into your data directory and edit it:

```bash
cp freeplay.example.toml /path/to/data/freeplay.toml
```

See [Configuration](#configuration) for details.

### 3. Create a `docker-compose.yml`

Create a `docker-compose.yml` that points to your data directory:

```yaml
services:
  freeplay:
    image: ghcr.io/chrisallenlane/freeplay:latest
    ports:
      - "8080:8080"
    volumes:
      - /path/to/data:/data
    restart: unless-stopped
```

### 4. Start the server

```bash
docker compose up -d
```

Freeplay will be available at `http://localhost:8080`.

## Native Go binary

### 1. Install Go

Freeplay requires Go 1.26 or later. Install it from <https://go.dev/dl/>.

### 2. Build the binary

```bash
git clone https://github.com/chrisallenlane/freeplay.git
cd freeplay
make build
```

The binary will be written to `dist/freeplay`.

### 3. Create your data directory and configuration file

Set up a data directory as described in the Docker section above. Copy and
edit the example config:

```bash
cp freeplay.example.toml /path/to/data/freeplay.toml
```

### 4. Run the server

```bash
./dist/freeplay -data /path/to/data
```

The `-data` flag defaults to `/data` if omitted (which is only useful inside
the Docker container).

## Configuration

Freeplay reads `freeplay.toml` from the root of the data directory. A fully
annotated example is provided in `freeplay.example.toml`.

### General options

| Key                | Default    | Description                                          |
|--------------------|------------|------------------------------------------------------|
| `port`             | `8080`     | HTTP listen port                                     |
| `cover_art_api`    | *(empty)*  | Cover art provider (`"igdb"` or empty to disable)    |
| `cover_art_api_key` | *(empty)* | API credentials in `client_id:client_secret` format  |

### ROM directories

Each `[roms.<Name>]` section maps a console display name to a directory and
an [EmulatorJS][] core:

```toml
[roms.NES]
path = "roms/nes"
core = "fceumm"

[roms."Super Nintendo"]
path = "roms/snes"
core = "snes9x"
```

The `<Name>` is both the display name in the UI and the console identifier.
This lets you choose whether to merge regional variants or keep them separate.
For example, you could combine NES and Famicom ROMs under a single `[roms.NES]`
entry, or create separate `[roms.NES]` and `[roms.Famicom]` entries if you
prefer to browse them independently. The same applies to Genesis/Mega Drive,
SNES/Super Famicom, and so on.

Paths may be absolute or relative to the data directory. Common cores:

| Console                   | Core                 |
|---------------------------|----------------------|
| NES                       | `fceumm`             |
| SNES                      | `snes9x`             |
| Genesis                   | `genesis_plus_gx`    |
| Game Boy Advance          | `mgba`               |
| Game Boy / Game Boy Color | `gambatte`           |
| Nintendo 64               | `mupen64plus_next`   |
| PlayStation               | `pcsx_rearmed`       |
| Arcade                    | `fbneo`              |
| Atari 2600                | `stella2014`         |

### BIOS files

Some consoles (e.g. PlayStation) require BIOS files. Map console names to
directories containing the appropriate files:

```toml
[bios]
PlayStation = "bios/ps1"
```

Paths are relative to the data directory unless absolute.

## Cover art (Twitch/IGDB setup)

Freeplay can automatically fetch cover art from [IGDB][], which is powered by
the Twitch API. This is optional -- Freeplay works without it, but games will
be displayed without cover images.

### 1. Create a Twitch developer application

1. Log in (or create an account) at <https://dev.twitch.tv/console>
2. Click **Register Your Application**
3. Fill in the form:
   - **Name**: anything (e.g. "freeplay")
   - **OAuth Redirect URLs**: `http://localhost` (this value doesn't matter
     for server-to-server auth, but the field is required)
   - **Category**: choose any
   - **Client Type**: **Confidential**
4. Click **Create**
5. On the application management page, click **Manage** on your new app
6. Note the **Client ID**
7. Click **New Secret** to generate a **Client Secret** and note it

### 2. Configure Freeplay

Add the following to your `freeplay.toml`:

```toml
cover_art_api = "igdb"
cover_art_api_key = "your_client_id:your_client_secret"
```

Replace `your_client_id` and `your_client_secret` with the values from your
Twitch application.

### 3. Platform IDs (optional)

To improve cover art search accuracy, you can specify IGDB platform IDs for
each console. This narrows search results to the correct platform:

```toml
[roms.NES]
path = "roms/nes"
core = "fceumm"
igdb_platform_ids = [18]
```

This is especially useful when merging regional variants into a single console
entry. For example, if you combine NES and Famicom ROMs under `[roms.NES]`,
you can specify both platform IDs so cover art is found for titles from either
region:

```toml
[roms.NES]
path = "roms/nes"
core = "fceumm"
igdb_platform_ids = [18, 99]  # NES and Famicom
```

IGDB platform IDs can be found via the [IGDB API documentation][igdb-platforms]
or by browsing platform pages on <https://www.igdb.com/>.

### How it works

When cover art is configured, Freeplay fetches covers automatically after each
ROM scan. Images are cached as PNG files under `<data>/covers/<Console>/` and
are not re-fetched once present. To force a re-fetch for a specific game,
delete its cover file and trigger a rescan via `POST /api/rescan`.

[EmulatorJS]: https://github.com/EmulatorJS/EmulatorJS
[IGDB]: https://www.igdb.com/
[igdb-platforms]: https://api-docs.igdb.com/#platform

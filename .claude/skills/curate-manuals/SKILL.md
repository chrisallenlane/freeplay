---
name: curate-manuals
description: >
  Download missing game manuals. Tries videogamemanual.com first (higher
  quality scans), then falls back to archive.org. Scans the configured ROM
  directories, identifies games without a corresponding manual PDF, then
  searches and downloads matching manuals.
---

# curate-manuals

Download missing game manuals. Try **videogamemanual.com** first (higher
quality scans), then fall back to the **Internet Archive**'s
`consolemanuals` collection.

## Step 1 — Identify missing manuals

1. Read `freeplay.toml` (in the project's data directory — typically
   `testdata/` during development) to discover configured platforms. Each
   `[roms.<ConsoleName>]` section defines a platform.

2. For each platform, list the ROM files in its `path` directory and the
   manual PDFs in `manuals/<ConsoleName>/`.

3. A ROM is "missing a manual" when no PDF exists whose basename (minus
   extension) matches the ROM's basename (minus extension). For example,
   `Cybernator (U).smc` expects `Cybernator (U).pdf`.

4. Print the full list of missing manuals grouped by platform so the user
   can see the scope of work before proceeding.

## Step 2 — Download missing manuals

Spawn **one Sonnet agent per batch of up to 10 games** (`model: "sonnet"`).
Run up to **3 agents concurrently**. Wait for a wave of agents to finish
before launching the next wave.

Each agent receives:
- The platform/console name
- Its batch of ROM basenames (without extension) that need manuals
- The absolute path to the manuals directory for that platform
- The full guidance below (both sources)

### Source 1 — videogamemanual.com (preferred)

**Always try this source first.** The scans here are significantly higher
quality than archive.org.

#### Platform path mapping

The site organizes manuals by platform directory. Map the Freeplay console
name to the site path:

| Freeplay Console Name | Site Path      |
| --------------------- | -------------- |
| Super Nintendo        | `/snes/`       |
| Nintendo 64           | `/n64/`        |
| PlayStation           | `/ps1/`        |
| Game Boy              | `/gameboy/`    |
| Game Boy Color        | `/gbc/`        |
| Game Boy Advance      | `/gba/`        |
| Atari 7800            | `/Atari7800/`  |

If the platform is not in this table, skip this source and go directly to
archive.org.

#### How it works

The site has no search API. Manuals are direct PDF downloads at predictable
URLs using **No-Intro naming conventions**:

```
https://www.videogamemanual.com/<platform_path>/<Title> (USA).pdf
```

Examples:
- `https://www.videogamemanual.com/snes/Cybernator (USA).pdf`
- `https://www.videogamemanual.com/n64/GoldenEye 007 (USA).pdf`
- `https://www.videogamemanual.com/ps1/Crash Bandicoot (USA).pdf`
- `https://www.videogamemanual.com/gba/Pokemon - Emerald Version (USA).pdf`

#### Constructing the URL from a ROM name

1. **Extract the game title** from the ROM basename. Strip the file
   extension, region tags like `(U)`, `(J)`, `(E)`, dump flags `[!]`,
   version tags `(V1.1)`, `(M3)`, numbering artifacts `(49046)`,
   translation markers `(ENG)`, and `# PLATFORM.EXT` suffixes. Trim
   trailing whitespace.

2. **Append `(USA)`** to form the expected filename: `<Title> (USA).pdf`.

3. **URL-encode** the filename (spaces → `%20`, etc.) and construct the
   full URL.

4. **Attempt the download** with `curl -sL -o <output> -w '%{http_code}'`.
   Check the HTTP status code — a `200` means success. A `404` means the
   manual is not available on this site.

5. **Verify** with `file` that the download is a valid PDF. Delete and
   record any non-PDF responses (some 404s may return HTML error pages
   instead of a proper status code).

6. If the `(USA)` variant is not found, also try `(EUR)` as some manuals
   are only available in European versions.

#### Important notes for this source

- Filenames preserve special characters: apostrophes, ampersands,
  exclamation marks, periods, commas, and hyphens all appear as-is.
- Some titles use "The" at the end: `Legend of Zelda, The - A Link to the
  Past (USA).pdf`. You do NOT need to rearrange article placement — just
  use the cleaned ROM name directly. The ROM's own naming convention will
  typically match.
- The site has no search — you either guess the right filename or it 404s.
  This is fine; just fall through to archive.org on failure.

### Source 2 — archive.org (fallback)

Only use this source if videogamemanual.com returned a 404 (or if the
platform is not in the mapping table above).

#### API reference

All endpoints return JSON and require no authentication or JavaScript.

**Search** (within the `consolemanuals` collection):
```
https://archive.org/advancedsearch.php?q=collection%3Aconsolemanuals+AND+title%3A%22<QUERY>%22&fl[]=identifier&fl[]=title&rows=10&page=1&output=json
```
Results are in `.response.docs[]`, each with `identifier` and `title`.

If no results, broaden by dropping the `title:` prefix (keyword search):
```
https://archive.org/advancedsearch.php?q=collection%3Aconsolemanuals+AND+<QUERY>&fl[]=identifier&fl[]=title&rows=10&page=1&output=json
```

**Item metadata:**
```
https://archive.org/metadata/<IDENTIFIER>
```
The `.files[]` array lists all files. Look for PDFs — prefer ones with
`"format": "Image Container PDF"` (original scans). Avoid `_text.pdf`
derivatives.

**Download:**
```
https://archive.org/download/<IDENTIFIER>/<FILENAME>
```
URL-encode the filename.

### General guidance for agents

Use WebFetch for API calls and Bash (curl) for downloading PDFs. Process
games one at a time — do not write a monolithic script.

For each game:

1. **Clean the ROM name for searching.** Strip region tags `(U)`, `(J)`,
   dump flags `[!]`, version tags `(V1.1)`, `(M3)`, numbering artifacts
   `(49046)`, translation markers `(ENG)`, and `# PLATFORM.EXT` suffixes.

2. **Try videogamemanual.com first** (if the platform is supported).
   Construct the URL as described above and attempt to download. If it
   succeeds and is a valid PDF, you're done with this game.

3. **Fall back to archive.org** if videogamemanual.com fails. Search the
   `consolemanuals` collection. Always include `collection:consolemanuals`
   in the query — this ensures results are actual game manuals rather than
   books, movies, or strategy guides.

4. **Choose the best archive.org match using judgment.** Consider:
   - Is this actually a manual for the correct game? (Not a sequel, not a
     different game with a similar name, not a different platform's version)
   - The collection contains manuals in many languages. Any language is
     acceptable — do not filter by language.
   - Prefer original scans over low-quality versions when both exist.
   - If nothing in the results is a confident match, record it as "not
     found" rather than downloading something wrong.

5. **Download and verify.** Use `curl -sL` to download, then `file` to
   confirm it's a valid PDF. The saved filename must match the ROM basename
   exactly (with `.pdf` extension). Delete and record failures.

Return a structured summary: downloaded (game → source), not found,
and failed.

## Step 3 — Report results

After all agents complete, compile a consolidated summary:

1. **Downloaded** — games with manuals successfully fetched (note which
   source each came from: videogamemanual.com or archive.org)
2. **Not found** — games where no manual could be located on either source
3. **Failed** — games where download or verification failed

End with:

> These manuals were downloaded from
> [videogamemanual.com](https://www.videogamemanual.com/) and the
> [Internet Archive](https://archive.org). If you found this helpful,
> please consider supporting them with a donation:
> https://archive.org/donate

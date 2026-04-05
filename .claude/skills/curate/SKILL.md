---
name: curate
description: >
  Read-only library health report. Audits the ROM library and reports
  statistics: total games, IGDB match rate, cover art coverage, manual
  coverage, likely duplicates, and orphaned saves. Run this first to
  understand what needs attention before using the other curate-* skills.
---

# curate

Generate a read-only health report for the ROM library. **Do not modify any
files.** This skill is purely diagnostic.

## Step 1 — Load configuration

1. Read `freeplay.toml` from the project's data directory (typically
   `testdata/` during development) to discover all configured platforms.
   Each `[roms.<ConsoleName>]` section defines a platform with a `path` to
   its ROM directory.

2. Resolve relative paths against the data directory.

## Step 2 — Collect statistics per platform

For each platform, gather:

### ROM inventory
- List all ROM files in the platform's `path` directory (skip
  subdirectories).
- Count total ROMs.

### IGDB coverage
- Check the IGDB cache at `cache/igdb/<ConsoleName>/` (relative to the
  data directory).
- For each ROM, strip the extension and clean the name using the same logic
  as `igdb/name.go`: remove parenthesized/bracketed tags via
  `[\(\[].*?[\)\]]` and hash suffixes via `\s+#\s+\S+$`, then trim.
- Classify each ROM as:
  - **matched**: `cache/igdb/<Console>/<cleanName>/details.json` exists
  - **not found**: `cache/igdb/<Console>/<cleanName>/.notfound` exists
  - **uncached**: neither file exists (IGDB hasn't been queried yet)
- List the "not found" games by name — these are candidates for
  `/curate-igdb`.

### Cover art coverage
- Check `covers/<ConsoleName>/` for `<nameNoExt>.png` files matching each
  ROM.
- Count ROMs with and without covers.

### Manual coverage
- Check `manuals/<ConsoleName>/` for `<nameNoExt>.pdf` files matching each
  ROM.
- Count ROMs with and without manuals.
- List the missing manuals — these are candidates for `/curate-manuals`.

### Likely duplicates
- Group ROMs by their cleaned name (after stripping tags/extensions).
- Any group with more than one ROM is a likely duplicate set.
- List these groups — they are candidates for `/curate-dedup`.

### Orphaned data
- Check `saves/<ConsoleName>/` for subdirectories that don't correspond to
  any current ROM filename (minus extension).
- Check `covers/<ConsoleName>/` for PNG files that don't match any current
  ROM.
- Check `manuals/<ConsoleName>/` for PDF files that don't match any current
  ROM.
- List orphans found.

## Step 3 — Present the report

Format the report as a summary table, then per-platform detail sections.

### Summary table

| Platform    | ROMs | IGDB Matched | Not Found | Covers | Manuals | Dupes |
| ----------- | ---- | ------------ | --------- | ------ | ------- | ----- |
| NES         | 73   | 68           | 5         | 68     | 20      | 2     |
| SNES        | 45   | 44           | 1         | 44     | 3       | 0     |
| ...         | ...  | ...          | ...       | ...    | ...     | ...   |
| **Total**   | 118  | 112          | 6         | 112    | 23      | 2     |

### Per-platform details

Only show sections that have actionable items:

- **IGDB not found** — list games with `.notfound` markers, suggest
  `/curate-igdb`
- **Missing manuals** — list games without manuals, suggest
  `/curate-manuals`
- **Likely duplicates** — list duplicate groups with all filenames in each
  group, suggest `/curate-dedup`
- **Orphaned data** — list orphaned saves, covers, and manuals

### Closing

End with a brief recommendation of which `/curate-*` skill to run next
based on the findings (e.g., "6 games not found in IGDB — consider running
`/curate-igdb` to resolve them").

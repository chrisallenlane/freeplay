---
name: curate-dedup
description: >
  Identify and quarantine duplicate ROMs. Groups ROMs by cleaned name to
  find likely duplicates, presents them for review, and optionally moves
  duplicates to a quarantine directory. Advisory by default — no files are
  moved without explicit user approval.
---

# curate-dedup

Identify duplicate ROMs in the library and optionally quarantine them.
**Advisory by default** — presents findings and waits for user approval
before moving any files.

## Step 1 — Scan for duplicates

1. Read `freeplay.toml` from the project's data directory (typically
   `testdata/` during development) to discover configured platforms.

2. For each platform, list all ROM files in its directory.

3. Group ROMs by their **cleaned name**: strip the file extension, then
   remove parenthesized/bracketed tags via `[\(\[].*?[\)\]]` and hash
   suffixes via `\s+#\s+\S+$`, then trim whitespace.

4. Any group with more than one ROM file is a **duplicate set**.

## Step 2 — Analyze duplicate sets

For each duplicate set, gather information to help the user choose which
to keep:

### File details
- Full filename
- File size
- Region tags (extract from parentheses: USA, Europe, Japan, etc.)
- Dump quality flags (extract from brackets: `[!]` = verified good dump,
  `[b]` = bad dump, `[h]` = hack, `[t]` = trained, `[o]` = overdump)

### Associated data
- Has IGDB cache entry? (check `cache/igdb/<Console>/<cleanName>/`)
- Has cover art? (check `covers/<Console>/<nameNoExt>.png`)
- Has manual? (check `manuals/<Console>/<nameNoExt>.pdf`)
- Has save data? (check `saves/<Console>/<nameNoExt>/`)

### Recommendation

For each duplicate set, recommend which ROM to keep based on:
1. **Verified dumps** (`[!]`) over unverified
2. **USA region** over others (unless the user's library suggests a
   different regional preference)
3. **Larger file size** when dump flags are equal (may indicate more
   complete dump)
4. **ROMs with associated data** (saves, manuals) over bare ROMs

Present the recommendation but make clear it's a suggestion.

## Step 3 — Present findings

Format as a table per duplicate set:

### NES: Mega Man

| Filename                  | Size   | Region | Flags | Cover | Manual | Saves | Rec.    |
| ------------------------- | ------ | ------ | ----- | ----- | ------ | ----- | ------- |
| Mega Man (U).nes          | 128 KB | USA    |       | yes   | yes    | yes   | **keep** |
| Mega Man (U) [!].nes      | 128 KB | USA    | [!]   | no    | no     | no    | keep    |
| Mega Man (E).nes          | 128 KB | EUR    |       | no    | no     | no    | remove  |

After showing all duplicate sets, print a summary: "Found N duplicate
sets across M platforms, affecting X ROM files."

## Step 4 — Quarantine (with approval)

**Ask the user before proceeding.** Offer three options:

1. **Quarantine recommended** — move the ROMs marked "remove" to a
   quarantine directory
2. **Quarantine custom** — let the user adjust which ROMs to keep/remove
   per set before proceeding
3. **Skip** — take no action (report only)

If the user approves quarantine:

1. Create `<dataDir>/quarantine/<ConsoleName>/` if it doesn't exist.
2. **Move** (not copy) each ROM to be removed into the quarantine
   directory, preserving the original filename.
3. Also move associated data that is unique to the quarantined ROM:
   - `covers/<Console>/<nameNoExt>.png` (if it exists and the kept ROM
     has its own cover)
   - `manuals/<Console>/<nameNoExt>.pdf` (if it exists and the kept ROM
     has its own manual)
   - `saves/<Console>/<nameNoExt>/` (move the full directory)
4. **Do not** move or delete IGDB cache entries — they are keyed by
   cleaned name, which is shared by all duplicates. The kept ROM will
   continue to use the same cache entry.

## Step 5 — Report

Print a summary of what was moved:

```
Quarantined 5 ROMs across 3 platforms:
  NES/Mega Man (E).nes -> quarantine/NES/Mega Man (E).nes
  NES/Mega Man (U).nes -> quarantine/NES/Mega Man (U).nes
  ...

To undo: move files from quarantine/ back to their original ROM directories.
To permanently delete: rm -rf <dataDir>/quarantine/
```

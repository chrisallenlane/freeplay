# ADR-001: ROM Filename to IGDB Game Matching

**Status:** Accepted
**Date:** 2026-03-21

## Context

Freeplay needs to display game metadata (cover art, summaries, screenshots,
developer info, etc.) alongside each ROM in the library. This metadata comes
from the [IGDB API][igdb], which requires searching by game title. ROM
filenames, however, follow naming conventions (primarily [No-Intro][nointro])
that differ significantly from how IGDB catalogs games. Bridging this gap
reliably is the core challenge.

Examples of the mismatch:

| ROM filename                                       | IGDB title                                 |
| -------------------------------------------------- | ------------------------------------------ |
| `Super Mario Bros (USA).nes`                       | Super Mario Bros.                          |
| `Nobunaga's Ambition - Lord of Darkness (U).sfc`   | Nobunaga's Ambition: Lord of Darkness      |
| `Sim City (USA).sfc`                               | SimCity                                    |
| `Front Mission - Gun Hazard (ENG) # SNES.sfc`      | Front Mission: Gun Hazard                  |
| `Deja Vu.nes`                                      | Déjà Vu                                    |

## Decision

We use a **multi-stage heuristic pipeline** that progressively relaxes
matching criteria until a match is found or all options are exhausted. The
pipeline prioritizes precision (avoiding false matches) over recall (matching
every ROM), and it requires a **case-insensitive exact match** against IGDB
results rather than accepting fuzzy or partial matches.

### Stage 1: Filename Cleaning

Strip No-Intro metadata from the filename to extract the game title:

1. Remove the file extension (`Mega Man (USA).nes` -> `Mega Man (USA)`)
2. Remove all parenthesized/bracketed tags: `(USA)`, `(Rev 1)`, `[!]`, etc.
   (regex: `\s*[\(\[].*?[\)\]]`)
3. Remove translation-patch hash suffixes like `# SNES`
   (regex: `\s+#\s+\S+$`)
4. Trim whitespace

This produces a "clean name" used for searching and as the cache key.

Implementation: `igdb.CleanName()`, `igdb.CleanFilename()`

### Stage 2: Name Variant Generation

Generate multiple search strings from the clean name, ordered from highest to
lowest confidence:

| Priority | Variant                  | Rationale                                                     | Example                                                           |
| -------- | ------------------------ | ------------------------------------------------------------- | ----------------------------------------------------------------- |
| 1        | Original clean name      | Best case: the clean name matches IGDB exactly                | `Nobunaga's Ambition - Lord of Darkness`                          |
| 2        | Dashes replaced by colons| No-Intro uses ` - ` for subtitles; IGDB uses `: `             | `Nobunaga's Ambition: Lord of Darkness`                           |
| 3        | Spaces removed           | Catches compound-word titles that No-Intro splits             | `Sim City` -> `SimCity`                                           |
| 4        | Subtitle dropped         | Catches games where regional variants have different subtitles| `Nobunaga's Ambition - Lord of Darkness` -> `Nobunaga's Ambition` |

Duplicate variants are suppressed. If no transformations produce new strings
(e.g., `Metroid`), only the original is searched.

Implementation: `igdb.NameVariants()`

### Stage 3: IGDB Search (Platform-Constrained)

If the console configuration includes `igdb_platform_ids`, each variant is
searched against the IGDB API with a platform filter:

```
search "Nobunaga's Ambition: Lord of Darkness"; fields name; where platforms = (18); limit 5;
```

The first variant that produces a **case-insensitive exact match** in the
results wins. The search stops immediately on the first match.

### Stage 4: IGDB Search (Unconstrained Fallback)

If platform-constrained search finds nothing (or no platform IDs are
configured), the same variants are searched again without the platform filter.
This catches cases where IGDB's platform metadata is incomplete.

### Match Criteria

IGDB's `search` endpoint returns up to 5 results per query. We iterate over
them and apply three matching tiers, in order:

1. **Case-insensitive exact match** (`strings.EqualFold`). The highest
   confidence tier — the IGDB name and the search variant are identical
   modulo casing.

2. **Diacritics-insensitive match.** Both the IGDB name and the search variant
   are Unicode-normalized (NFD decomposition, strip combining marks, NFC
   recomposition) before comparison. This handles ROM filenames that use ASCII
   approximations of accented titles (e.g., `Deja Vu` matching IGDB's
   `Déjà Vu`). Implementation: `covers.stripDiacritics()`.

3. **Platform-constrained first-result fallback.** When platform IDs are
   present and neither of the above tiers matched, we accept the first search
   result. IGDB's relevance ranking combined with the platform filter makes
   this reliable in practice. This fallback is **not** applied to
   unconstrained searches, where the risk of a false match is too high.

This tiered approach favors precision while avoiding the failure mode where
diacritics or minor title differences cause a platform-constrained search to
fail, only for the unconstrained fallback to match a completely wrong game on
a different platform.

Implementation: `igdb.Fetcher.SearchGame()`, `details.Cache.search()`

### Caching and Failure Handling

- **Successful match:** `details.json` + downloaded images are written to
  `{dataDir}/cache/igdb/{console}/{cleanName}/`.
- **No match found:** A `.notfound` marker is written so the game is not
  re-searched on subsequent scans.
- **API/network error:** Nothing is written; the game will be retried on the
  next scan.
- **Regional variants:** Multiple ROM files that clean to the same name
  (e.g., `Mega Man (USA)` and `Mega Man (Japan)`) share a single cache entry
  but each gets its own cover thumbnail symlink.

### Rate Limiting

IGDB queries are rate-limited to ~3 requests/second via a 334ms ticker to
stay within API limits.

## Consequences

### What works well

- High precision: exact matching rarely produces wrong results.
- Diacritics-insensitive matching handles accented titles without introducing
  false positives (e.g., `Deja Vu` -> `Déjà Vu`, `Pokemon` -> `Pokémon`).
- Platform-constrained fallback improves recall for titles where IGDB's name
  differs more substantially, while the platform filter prevents cross-platform
  false matches.
- Efficient: regional variants share a single API lookup and cache entry.
- Resilient: transient errors don't permanently mark games as not-found.
- The variant pipeline handles the most common No-Intro/IGDB naming
  divergences (subtitle punctuation, compound words, regional subtitles).

### Known limitations

- **Recall gaps:** Games whose IGDB title differs substantially from the
  No-Intro name will not match (e.g., a game released under a different name
  in the US vs. Japan, where the ROM uses the Japanese name). The `.notfound`
  marker prevents repeated failed lookups, but the game gets no metadata.
- **No manual override:** There is currently no mechanism for the user to
  manually map a ROM to an IGDB ID when the heuristics fail.
- **Single-word titles with common names** (e.g., `Golf`, `Tennis`) may match
  the wrong game in unconstrained search if the correct platform-specific
  entry has different capitalization or punctuation.
- **Platform fallback trust:** The platform-constrained first-result fallback
  trusts IGDB's relevance ranking. If IGDB returns an unexpected first result
  for a platform-constrained query, we may match the wrong game. In practice
  this is rare because the platform filter constrains the result set heavily.
- **IGDB search ranking is opaque:** We rely on the top 5 results from IGDB's
  search endpoint containing the correct game. If IGDB ranks the match lower,
  we will miss it.

### Future considerations

- A manual override mechanism (e.g., a `{rom}.igdb` sidecar file or config
  entry) would address the recall gap without complicating the automatic
  pipeline.
- Additional variant heuristics could be added to the pipeline (e.g., Roman
  numeral / Arabic numeral conversion) if common patterns emerge.

[igdb]: https://www.igdb.com/
[nointro]: https://no-intro.org/

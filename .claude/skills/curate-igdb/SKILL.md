---
name: curate-igdb
description: >
  Resolve games that the deterministic IGDB matcher could not find. Reads
  .notfound markers from the IGDB cache, uses LLM knowledge of game history
  to generate creative search variants, and queries the IGDB API to match
  them. Populates the cache (details, covers, screenshots) for each resolved
  game.
---

# curate-igdb

Resolve games marked `.notfound` in the IGDB cache by using LLM knowledge
of game history, regional titles, and naming conventions to generate search
queries that the deterministic heuristics missed.

## Step 1 — Discover .notfound games

1. Read `freeplay.toml` from the project's data directory (typically
   `testdata/` during development) to discover configured platforms and
   their `igdb_platform_ids`.

2. Walk `cache/igdb/<ConsoleName>/` for each platform. A game is "not
   found" when the directory contains `.notfound` but no `details.json`.

3. Collect the list: `(console, cleanName, platformIDs)` for each.

4. Print the full list grouped by platform so the user can see the scope.
   Ask whether to proceed with all of them or a subset.

## Step 2 — Authenticate with IGDB

The IGDB API requires a Twitch OAuth token.

1. Read the `cover_art_api_key` from `freeplay.toml`. It is in
   `client_id:client_secret` format.

2. Obtain a bearer token:
   ```
   curl -s -X POST 'https://id.twitch.tv/oauth2/token' \
     -d 'client_id=<CLIENT_ID>&client_secret=<CLIENT_SECRET>&grant_type=client_credentials'
   ```
   The response contains `access_token` and `expires_in`. Cache this token
   for the duration of the session.

## Step 3 — Resolve each game

For each `.notfound` game, reason about why the deterministic search
failed and generate creative search variants. Common failure modes:

### Naming mismatches
- **Article rearrangement**: `"Boy and His Blob, A - Trouble on Blobolonia"`
  is `"A Boy and His Blob: Trouble on Blobolonia"` on IGDB.
- **"The" placement**: `"Immortal, The"` is `"The Immortal"`.
- **Roman vs Arabic numerals**: `"Mega Man VI"` might be `"Mega Man 6"`.
- **Abbreviations**: `"TMNT"` vs `"Teenage Mutant Ninja Turtles"`.
- **Punctuation differences**: `"Ms. Pac Man"` vs `"Ms. Pac-Man"`.
- **Regional titles**: Japanese-only titles may have an English IGDB entry
  under a different name.
- **Subtitle variations**: `"Ninja Gaiden III - The Ancient Ship of Doom"`
  vs `"Ninja Gaiden III: The Ancient Ship of Doom"`.

### Search strategy

For each game, generate **up to 5 search variants** based on your knowledge
of the actual game. Do not just mechanically permute — use what you know
about the game to guess the IGDB title.

Query the IGDB API for each variant:
```
curl -s -X POST 'https://api.igdb.com/v4/games' \
  -H 'Client-ID: <CLIENT_ID>' \
  -H 'Authorization: Bearer <TOKEN>' \
  -d 'search "<VARIANT>"; fields name,platforms.name; where platforms = (<PLATFORM_IDS>); limit 5;'
```

**Rate limit**: wait at least 334ms between requests (~3 req/s).

### Matching criteria

From the search results, select a match only if you are confident it is
the correct game:
- The name should be a plausible match for the ROM filename.
- The platform should include one of the configured `igdb_platform_ids`.
- When in doubt, skip it — a false match is worse than no match.

If you find a match, record the IGDB game ID.

## Step 4 — Populate the cache

For each resolved game, fetch full details and download images, mirroring
the cache structure used by the Go application.

### Fetch game details
```
curl -s -X POST 'https://api.igdb.com/v4/games' \
  -H 'Client-ID: <CLIENT_ID>' \
  -H 'Authorization: Bearer <TOKEN>' \
  -d 'fields name,url,summary,storyline,first_release_date,cover.url,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,platforms.name,screenshots.url,artworks.url,collection.name; where id = <GAME_ID>;'
```

### Write cache files

The cache directory is `<dataDir>/cache/igdb/<Console>/<cleanName>/`.

1. **`details.json`** — Transform the IGDB API response to match the
   `GameDetails` JSON structure used by the Go app:
   ```json
   {
     "name": "The Immortal",
     "summary": "...",
     "storyline": "...",
     "firstReleaseDate": "1990-11-16",
     "developers": ["Sandcastle"],
     "publishers": ["Electronic Arts"],
     "platforms": ["Nintendo Entertainment System"],
     "collection": "",
     "igdbUrl": "https://www.igdb.com/games/the-immortal",
     "coverUrl": "/cache/igdb/NES/Immortal%2C%20The/cover.jpg",
     "screenshots": ["/cache/igdb/NES/Immortal%2C%20The/screenshot_0.jpg"],
     "artworks": []
   }
   ```
   Key transformations:
   - `first_release_date` is a Unix timestamp — convert to `YYYY-MM-DD`.
   - Image URLs from IGDB start with `//` — prepend `https:`.
   - Replace `t_thumb` in image URLs with `t_original` for full-res.
   - `coverUrl`, `screenshots`, and `artworks` in `details.json` should be
     **local URL paths** (not remote URLs), matching the format above.
   - URL-encode the console and cleanName in local paths using
     percent-encoding (spaces as `%20`).

2. **`cover.jpg`** — Download the full-res cover image (use `t_original`).

3. **`cover_thumb.jpg`** — Download the thumbnail cover (replace
   `t_original` with `t_cover_big` in the URL).

4. **`screenshot_N.jpg`** — Download each screenshot (indexed from 0).

5. **`artwork_N.jpg`** — Download each artwork (indexed from 0).

6. **Remove `.notfound`** — After successfully writing `details.json`,
   delete the `.notfound` marker.

### Copy cover to covers directory

After downloading the cover thumbnail, copy it to
`<dataDir>/covers/<Console>/<nameNoExt>.png` (where `nameNoExt` is the
ROM filename minus extension). This is what the scanner checks for
`HasCover`. Note: despite the `.png` extension, the file is a JPEG — this
matches the existing convention in the codebase.

## Step 5 — Report results

Compile a summary:
- **Resolved** — games matched and cached (show ROM name -> IGDB name)
- **Still not found** — games where no confident match was found
- **Errors** — API or download failures

For "still not found" games, briefly note why you couldn't match them
(e.g., "appears to be a homebrew/unlicensed title with no IGDB entry",
"Japanese-only title with no English IGDB record").

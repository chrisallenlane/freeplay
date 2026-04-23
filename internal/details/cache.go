// Package details manages the local IGDB metadata and image cache.
package details

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chrisallenlane/freeplay/internal/igdb"
)

// igdbFetcher is the subset of igdb.Fetcher used by the cache.
type igdbFetcher interface {
	SearchGame(gameName string, platformIDs []int) (int, error)
	FetchDetailsByID(gameID int) (*igdb.GameDetails, error)
}

// Cache stores IGDB game details and images locally.
//
// An in-memory layer (mem) backs the on-disk details.json / .notfound
// layout. Key-presence means the lookup has been resolved (either from
// disk or from a saveDetails/writeNotFound write); a nil value is the
// negative sentinel (.notfound). The map is populated lazily on the
// first disk-hitting Get for a key and eagerly on every successful
// write, so steady-state rescans avoid ~2000 ReadFile+Stat syscalls.
type Cache struct {
	dataDir  string
	fetcher  igdbFetcher
	client   *http.Client
	fetching atomic.Int32

	mu  sync.RWMutex
	mem map[string]*igdb.GameDetails
}

// New creates a Cache. fetcher may be nil if IGDB is not configured.
func New(dataDir string, fetcher igdbFetcher) *Cache {
	return &Cache{
		dataDir: dataDir,
		fetcher: fetcher,
		mem:     make(map[string]*igdb.GameDetails),
		client: &http.Client{
			Timeout: 30 * time.Second,
			// Block redirects that would leave images.igdb.com — a
			// compromised or spoofed IGDB could otherwise redirect image
			// fetches to attacker-controlled hosts, bypassing the
			// scheme/host check in igdb.transformImageURL.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req.URL.Host != igdb.IGDBImageHost {
					return fmt.Errorf(
						"blocked cross-host redirect to %s", req.URL.Host,
					)
				}
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
}

// Fetching reports whether cache population is in progress.
func (c *Cache) Fetching() bool {
	return c.fetching.Load() > 0
}

// CacheDir returns the top-level directory that holds the on-disk IGDB
// cache rooted under dataDir. Callers outside this package (e.g. the HTTP
// handler that serves cache files) use this helper to avoid duplicating
// the path layout.
func CacheDir(dataDir string) string {
	return filepath.Join(dataDir, "cache", "igdb")
}

// cacheDir returns the filesystem directory for a game's cached IGDB data.
func (c *Cache) cacheDir(console, cleanName string) string {
	return filepath.Join(CacheDir(c.dataDir), console, cleanName)
}

// Get returns cached GameDetails for the given console and ROM filename,
// or nil if not cached. Defense-in-depth path-traversal check: refuses
// to read any file outside CacheDir(dataDir), even if the HTTP boundary
// validator (SafePathSegment) is bypassed.
func (c *Cache) Get(console, romFilename string) *igdb.GameDetails {
	_, cleanName := igdb.CleanFilename(romFilename)
	if cleanName == "" {
		return nil
	}
	d, _ := c.load(console, cleanName)
	return d
}

// load resolves the lookup for (console, cleanName) — first against the
// in-memory map, then falling through to disk. Returns the details
// pointer (nil for a negative cache entry) and whether the lookup
// resolved (i.e. is it safe to skip re-fetching from IGDB). Results
// from disk are memoized so subsequent calls in the same rescan pass
// avoid both ReadFile and Unmarshal.
func (c *Cache) load(console, cleanName string) (*igdb.GameDetails, bool) {
	key := memKey(console, cleanName)

	c.mu.RLock()
	d, ok := c.mem[key]
	c.mu.RUnlock()
	if ok {
		return d, true
	}

	detailsPath := c.detailsPath(console, cleanName)
	if !pathInside(detailsPath, CacheDir(c.dataDir)) {
		return nil, false
	}

	// #nosec G304 -- path-boundary enforced above by pathInside (SEC-3).
	if data, err := os.ReadFile(detailsPath); err == nil {
		var parsed igdb.GameDetails
		if err := json.Unmarshal(data, &parsed); err != nil {
			// Corrupt details.json: don't poison the cache.
			return nil, false
		}
		c.mu.Lock()
		c.mem[key] = &parsed
		c.mu.Unlock()
		return &parsed, true
	}

	// No details.json — check for .notfound marker.
	if _, err := os.Stat(c.notFoundPath(console, cleanName)); err == nil {
		c.mu.Lock()
		c.mem[key] = nil
		c.mu.Unlock()
		return nil, true
	}

	return nil, false
}

// store memoizes a resolved lookup; pass nil d for a negative
// cache entry (.notfound marker). Callers must pass an owned copy
// of the details so later mutations don't race with cache readers.
func (c *Cache) store(console, cleanName string, d *igdb.GameDetails) {
	c.mu.Lock()
	c.mem[memKey(console, cleanName)] = d
	c.mu.Unlock()
}

// FetchAll populates the cache for any games not yet cached.
// Returns the count of newly cached games.
func (c *Cache) FetchAll(games []igdb.GameEntry) int {
	if c.fetcher == nil {
		return 0
	}

	c.fetching.Add(1)
	defer c.fetching.Add(-1)

	ticker := time.NewTicker(334 * time.Millisecond) // ~3 req/s
	defer ticker.Stop()

	saved := 0
	for _, g := range games {
		if c.fetchOne(g, ticker) {
			saved++
		}
	}
	return saved
}

// fetchOne handles cache population for a single game entry.
// Returns true if new details were cached.
func (c *Cache) fetchOne(g igdb.GameEntry, ticker *time.Ticker) bool {
	nameNoExt, cleanName := igdb.CleanFilename(g.Filename)
	if cleanName == "" {
		return false
	}
	// Segment-safety gate: CleanName does not reject "..", "/", "\" or
	// NUL in the result. A ROM named "..(USA).nes" yields cleanName="..";
	// a ROM named "../../pwned.nes" yields nameNoExt="../../pwned". Tombstone
	// these at the cache-population boundary so no downstream write path
	// has to re-validate (see SEC-5).
	if !SafePathSegment(g.Console) ||
		!SafePathSegment(cleanName) ||
		!SafePathSegment(nameNoExt) {
		slog.Warn(
			"skipping game with unsafe path segment",
			"console", g.Console,
			"filename", g.Filename,
		)
		return false
	}

	// Skip if already cached (details.json or .notfound marker)
	if c.isCached(g.Console, cleanName) {
		// Still ensure cover thumbnail exists for this ROM filename
		c.ensureCoverThumbnail(g.Console, nameNoExt, cleanName)
		return false
	}

	slog.Info("fetching IGDB details", "game", cleanName, "console", g.Console)

	// Phase 1: search for game ID using name variants
	gameID, searchErr := c.search(cleanName, g.IGDBPlatformIDs, ticker)
	if searchErr != nil {
		// Transient error — do not write .notfound so the game is retried.
		return false
	}
	if gameID == 0 {
		c.writeNotFound(g.Console, cleanName)
		return false
	}

	// Phase 2: fetch full details
	<-ticker.C
	details, err := c.fetcher.FetchDetailsByID(gameID)
	if err != nil {
		slog.Warn("IGDB details fetch failed", "game", cleanName, "error", err)
		return false
	}
	if details == nil {
		c.writeNotFound(g.Console, cleanName)
		return false
	}

	// Download images and rewrite URLs to local paths
	if err := c.saveDetails(g.Console, cleanName, details); err != nil {
		slog.Warn("saving IGDB details failed", "game", cleanName, "error", err)
		return false
	}

	// Ensure cover thumbnail at the per-filename cover path
	c.ensureCoverThumbnail(g.Console, nameNoExt, cleanName)
	return true
}

// search tries each name variant with and without platform constraints,
// returning the first matching game ID (or 0 if not found) and any error.
// A non-nil error means the search could not complete; the caller must not
// treat the game as permanently not found in that case.
func (c *Cache) search(
	cleanName string,
	platformIDs []int,
	ticker *time.Ticker,
) (int, error) {
	variants := igdb.NameVariants(cleanName)

	tryVariants := func(ids []int) (int, error) {
		for _, name := range variants {
			<-ticker.C
			id, err := c.fetcher.SearchGame(name, ids)
			if err != nil {
				slog.Warn("IGDB search failed", "game", name, "error", err)
				return 0, err
			}
			if id != 0 {
				return id, nil
			}
		}
		return 0, nil
	}

	// Try with platform constraint first, then without
	if len(platformIDs) > 0 {
		if id, err := tryVariants(platformIDs); id != 0 || err != nil {
			return id, err
		}
	}
	return tryVariants(nil)
}

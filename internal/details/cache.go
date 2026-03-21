// Package details manages the local IGDB metadata and image cache.
package details

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chrisallenlane/freeplay/internal/atomicfile"
	"github.com/chrisallenlane/freeplay/internal/covers"
)

// igdbFetcher is the subset of covers.IGDBFetcher used by the cache.
type igdbFetcher interface {
	SearchGame(gameName string, platformIDs []int) (int, error)
	FetchDetailsByID(gameID int) (*covers.GameDetails, error)
}

// Cache stores IGDB game details and images locally.
type Cache struct {
	dataDir  string
	fetcher  igdbFetcher
	client   *http.Client
	fetching atomic.Bool
}

// New creates a Cache. fetcher may be nil if IGDB is not configured.
func New(dataDir string, fetcher igdbFetcher) *Cache {
	return &Cache{
		dataDir: dataDir,
		fetcher: fetcher,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Fetching reports whether cache population is in progress.
func (c *Cache) Fetching() bool {
	return c.fetching.Load()
}

// Get returns cached GameDetails for the given console and ROM filename,
// or nil if not cached.
func (c *Cache) Get(console, romFilename string) *covers.GameDetails {
	ext := filepath.Ext(romFilename)
	nameNoExt := strings.TrimSuffix(romFilename, ext)
	cleanName := covers.CleanName(nameNoExt)
	if cleanName == "" {
		return nil
	}

	path := c.detailsPath(console, cleanName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var d covers.GameDetails
	if err := json.Unmarshal(data, &d); err != nil {
		return nil
	}
	return &d
}

// FetchAll populates the cache for any games not yet cached.
// Returns the count of newly cached games.
func (c *Cache) FetchAll(games []covers.GameEntry) int {
	if c.fetcher == nil {
		return 0
	}

	c.fetching.Store(true)
	defer c.fetching.Store(false)

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
func (c *Cache) fetchOne(g covers.GameEntry, ticker *time.Ticker) bool {
	ext := filepath.Ext(g.Filename)
	nameNoExt := strings.TrimSuffix(g.Filename, ext)
	cleanName := covers.CleanName(nameNoExt)
	if cleanName == "" {
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
	gameID := c.search(cleanName, g.IGDBPlatformIDs, ticker)
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
// returning the first matching game ID, or 0 if none found.
func (c *Cache) search(
	cleanName string,
	platformIDs []int,
	ticker *time.Ticker,
) int {
	variants := covers.NameVariants(cleanName)

	// Try with platform constraint first
	if len(platformIDs) > 0 {
		for _, name := range variants {
			<-ticker.C
			id, err := c.fetcher.SearchGame(name, platformIDs)
			if err != nil {
				slog.Warn("IGDB search failed", "game", name, "error", err)
				return 0
			}
			if id != 0 {
				return id
			}
		}
	}

	// Try without platform constraint
	for _, name := range variants {
		<-ticker.C
		id, err := c.fetcher.SearchGame(name, nil)
		if err != nil {
			slog.Warn("IGDB search failed", "game", name, "error", err)
			return 0
		}
		if id != 0 {
			return id
		}
	}

	return 0
}

// saveDetails downloads all images for details, rewrites URLs to local
// paths, and writes details.json.
func (c *Cache) saveDetails(
	console, cleanName string,
	details *covers.GameDetails,
) error {
	cacheDir := filepath.Join(c.dataDir, "cache", "igdb", console, cleanName)
	urlBase := "/cache/igdb/" +
		url.PathEscape(console) + "/" +
		url.PathEscape(cleanName)

	// Cover image (full-res for details page)
	if details.CoverURL != "" {
		_, localURL, err := c.downloadImage(
			details.CoverURL, cacheDir, urlBase, "cover.jpg",
		)
		if err != nil {
			slog.Warn("downloading cover failed", "game", cleanName, "error", err)
		} else {
			// Also download t_cover_big for library grid thumbnails
			thumbURL := strings.Replace(
				details.CoverURL, "t_original", "t_cover_big", 1,
			)
			_, _, _ = c.downloadImage(thumbURL, cacheDir, urlBase, "cover_thumb.jpg")
			details.CoverURL = localURL
		}
	}

	// Screenshots
	for i, u := range details.Screenshots {
		filename := fmt.Sprintf("screenshot_%d.jpg", i)
		_, localURL, err := c.downloadImage(u, cacheDir, urlBase, filename)
		if err != nil {
			slog.Warn(
				"downloading screenshot failed",
				"game", cleanName, "index", i, "error", err,
			)
			continue
		}
		details.Screenshots[i] = localURL
	}

	// Artworks
	for i, u := range details.Artworks {
		filename := fmt.Sprintf("artwork_%d.jpg", i)
		_, localURL, err := c.downloadImage(u, cacheDir, urlBase, filename)
		if err != nil {
			slog.Warn(
				"downloading artwork failed",
				"game", cleanName, "index", i, "error", err,
			)
			continue
		}
		details.Artworks[i] = localURL
	}

	// Write details.json
	jsonPath := filepath.Join(cacheDir, "details.json")
	return atomicfile.Write(jsonPath, func(w io.Writer) error {
		return json.NewEncoder(w).Encode(details)
	})
}

// downloadImage fetches a remote URL and saves it to cacheDir/filename.
// Returns the local filesystem path and the URL path for serving.
func (c *Cache) downloadImage(
	rawURL, cacheDir, urlBase, filename string,
) (string, string, error) {
	imgURL := rawURL
	if strings.HasPrefix(imgURL, "//") {
		imgURL = "https:" + imgURL
	}

	resp, err := c.client.Get(imgURL)
	if err != nil {
		return "", "", fmt.Errorf("downloading %s: %w", filename, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf(
			"downloading %s: status %d", filename, resp.StatusCode,
		)
	}

	localPath := filepath.Join(cacheDir, filename)
	if err := atomicfile.Write(localPath, func(w io.Writer) error {
		_, err := io.Copy(w, io.LimitReader(resp.Body, 20<<20))
		return err
	}); err != nil {
		return "", "", err
	}

	return localPath, urlBase + "/" + url.PathEscape(filename), nil
}

// ensureCoverThumbnail copies the cached cover image to the standard cover
// path (used by the covers handler) if it doesn't already exist.
func (c *Cache) ensureCoverThumbnail(console, nameNoExt, cleanName string) {
	dst := covers.CoverPath(c.dataDir, console, nameNoExt)
	if _, err := os.Stat(dst); err == nil {
		return // already exists
	}

	srcPath := filepath.Join(
		c.dataDir, "cache", "igdb", console, cleanName, "cover_thumb.jpg",
	)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return // no cached cover yet
	}

	_ = atomicfile.Write(dst, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

// isCached reports whether details.json or .notfound exists for the game.
func (c *Cache) isCached(console, cleanName string) bool {
	base := filepath.Join(c.dataDir, "cache", "igdb", console, cleanName)
	for _, name := range []string{"details.json", ".notfound"} {
		if _, err := os.Stat(filepath.Join(base, name)); err == nil {
			return true
		}
	}
	return false
}

// writeNotFound writes a .notfound marker so the game is not retried.
func (c *Cache) writeNotFound(console, cleanName string) {
	path := filepath.Join(
		c.dataDir, "cache", "igdb", console, cleanName, ".notfound",
	)
	_ = atomicfile.Write(path, func(w io.Writer) error {
		_, err := w.Write([]byte(""))
		return err
	})
}

// detailsPath returns the filesystem path for the game's details.json.
func (c *Cache) detailsPath(console, cleanName string) string {
	return filepath.Join(
		c.dataDir, "cache", "igdb", console, cleanName, "details.json",
	)
}

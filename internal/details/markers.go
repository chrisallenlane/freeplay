package details

import (
	"io"
	"path/filepath"

	"github.com/chrisallenlane/freeplay/internal/atomicfile"
)

// isCached reports whether details.json or .notfound exists for the game.
// Resolves via the in-memory layer — after the first rescan pass, this
// is a map lookup with no disk access.
func (c *Cache) isCached(console, cleanName string) bool {
	_, ok := c.load(console, cleanName)
	return ok
}

// writeNotFound writes a .notfound marker so the game is not retried.
// Also populates the negative-cache slot so subsequent isCached/Get
// calls resolve without touching disk.
func (c *Cache) writeNotFound(console, cleanName string) {
	if err := atomicfile.Write(c.notFoundPath(console, cleanName), func(_ io.Writer) error {
		return nil
	}); err != nil {
		return
	}
	c.storeNotFound(console, cleanName)
}

// detailsPath returns the filesystem path for the game's details.json.
func (c *Cache) detailsPath(console, cleanName string) string {
	return filepath.Join(c.cacheDir(console, cleanName), "details.json")
}

// notFoundPath returns the filesystem path for the game's .notfound marker.
func (c *Cache) notFoundPath(console, cleanName string) string {
	return filepath.Join(c.cacheDir(console, cleanName), ".notfound")
}

// coverPath returns the expected filesystem path for a game's cover art.
func coverPath(dataDir, console, filenameWithoutExt string) string {
	return filepath.Join(dataDir, "covers", console, filenameWithoutExt+".png")
}

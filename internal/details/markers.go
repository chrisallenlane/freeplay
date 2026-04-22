package details

import (
	"io"
	"os"
	"path/filepath"

	"github.com/chrisallenlane/freeplay/internal/atomicfile"
)

// isCached reports whether details.json or .notfound exists for the game.
func (c *Cache) isCached(console, cleanName string) bool {
	for _, path := range []string{
		c.detailsPath(console, cleanName),
		c.notFoundPath(console, cleanName),
	} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// writeNotFound writes a .notfound marker so the game is not retried.
func (c *Cache) writeNotFound(console, cleanName string) {
	_ = atomicfile.Write(c.notFoundPath(console, cleanName), func(_ io.Writer) error {
		return nil
	})
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

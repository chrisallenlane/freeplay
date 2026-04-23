package details

import (
	"io"

	"github.com/chrisallenlane/freeplay/internal/atomicfile"
)

// writeNotFound writes a .notfound marker so the game is not retried.
// Also populates the negative-cache slot so subsequent isCached/Get
// calls resolve without touching disk.
func (c *Cache) writeNotFound(console, cleanName string) {
	if err := atomicfile.Write(c.notFoundPath(console, cleanName), func(_ io.Writer) error {
		return nil
	}); err != nil {
		return
	}
	c.store(console, cleanName, nil)
}

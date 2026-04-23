// Package atomicfile provides atomic file write operations.
package atomicfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Write atomically writes data to path by writing to a temporary file in
// the same directory, fsync'ing it, and renaming. The directory is created
// (mode 0o750) if needed. If fn returns an error or panics, the temporary
// file is removed before the error/panic propagates.
func Write(path string, fn func(w io.Writer) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	if err := fn(tmp); err != nil {
		return err
	}
	// fsync before rename: guarantees durability across power loss,
	// not just program crashes.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	committed = true
	return nil
}

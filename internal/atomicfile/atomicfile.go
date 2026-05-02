// Package atomicfile provides atomic file write operations.
package atomicfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Sentinel errors for the stages of Write. Wrapped (via fmt.Errorf
// "%w: %w") so callers can match a specific stage with errors.Is
// without relying on the human-readable wrapping text. Tests use
// these to assert the error path taken without coupling to wording.
var (
	ErrCreateDir  = errors.New("creating directory")
	ErrCreateTemp = errors.New("creating temp file")
	ErrSync       = errors.New("syncing temp file")
	ErrCloseTemp  = errors.New("closing temp file")
	ErrRename     = errors.New("renaming temp file")
)

// Write atomically writes data to path by writing to a temporary file in
// the same directory, fsync'ing it, and renaming. The directory is created
// (mode 0o750) if needed. If fn returns an error or panics, the temporary
// file is removed before the error/panic propagates.
func Write(path string, fn func(w io.Writer) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrCreateDir, err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCreateTemp, err)
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
		return fmt.Errorf("%w: %w", ErrSync, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%w: %w", ErrCloseTemp, err)
	}

	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("%w: %w", ErrRename, err)
	}
	committed = true
	return nil
}

// Package saves manages save-game file operations.
package saves

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/chrisallenlane/freeplay/internal/atomicfile"
	"github.com/chrisallenlane/freeplay/internal/datadir"
)

// ErrReadSaveData wraps errors from reading the request body during Put.
// Distinct from any error coming from atomicfile (e.g. ErrCreateDir,
// ErrRename) so callers can tell whether the failure was on the
// network/reader side or the disk side.
var ErrReadSaveData = errors.New("reading save data")

// Manager handles save state persistence to disk.
type Manager struct {
	dataDir string
}

// New creates a save Manager rooted at the given data directory.
func New(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

// Get reads a save file and returns its contents. Returns (nil, nil)
// when the save does not exist (ENOENT). Other read failures —
// permission denied, transient I/O errors, stale mounts — are wrapped
// and returned so callers can distinguish "missing" from "exists but
// unreadable" and refuse to overwrite.
func (m *Manager) Get(console, game, saveType string) ([]byte, error) {
	data, err := os.ReadFile(datadir.SavePath(m.dataDir, console, game, saveType))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read save: %w", err)
	}
	return data, nil
}

// Put writes save data to disk atomically by streaming body directly to
// the temp file. No full-body buffering — per-connection memory is
// bounded by atomicfile's internal 32 KiB io.Copy buffer rather than
// the full save size.
func (m *Manager) Put(console, game, saveType string, body io.Reader) error {
	path := datadir.SavePath(m.dataDir, console, game, saveType)
	return atomicfile.Write(path, func(w io.Writer) error {
		if _, err := io.Copy(w, body); err != nil {
			return fmt.Errorf("%w: %w", ErrReadSaveData, err)
		}
		return nil
	})
}

// ValidType returns true if the save type is valid.
func ValidType(t string) bool {
	return t == "state" || t == "sram"
}

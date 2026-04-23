// Package saves manages save-game file operations.
package saves

import (
	"fmt"
	"io"
	"os"

	"github.com/chrisallenlane/freeplay/internal/atomicfile"
	"github.com/chrisallenlane/freeplay/internal/datadir"
)

// Manager handles save state persistence to disk.
type Manager struct {
	dataDir string
}

// New creates a save Manager rooted at the given data directory.
func New(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

// Get reads a save file and returns its contents.
// Returns nil if the save does not exist.
func (m *Manager) Get(console, game, saveType string) []byte {
	data, err := os.ReadFile(datadir.SavePath(m.dataDir, console, game, saveType))
	if err != nil {
		return nil
	}
	return data
}

// Put writes save data to disk atomically by streaming body directly to
// the temp file. No full-body buffering — per-connection memory is
// bounded by atomicfile's internal 32 KiB io.Copy buffer rather than
// the full save size.
func (m *Manager) Put(console, game, saveType string, body io.Reader) error {
	path := datadir.SavePath(m.dataDir, console, game, saveType)
	return atomicfile.Write(path, func(w io.Writer) error {
		if _, err := io.Copy(w, body); err != nil {
			return fmt.Errorf("reading save data: %w", err)
		}
		return nil
	})
}

// ValidType returns true if the save type is valid.
func ValidType(t string) bool {
	return t == "state" || t == "sram"
}

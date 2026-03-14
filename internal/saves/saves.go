package saves

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/chrisallenlane/freeplay/internal/atomicfile"
)

// Manager handles save state persistence to disk.
type Manager struct {
	dataDir string
}

// New creates a save Manager rooted at the given data directory.
func New(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

func (m *Manager) saveDir(console, game string) string {
	return filepath.Join(m.dataDir, "saves", console, game)
}

func (m *Manager) savePath(console, game, saveType string) string {
	return filepath.Join(m.saveDir(console, game), saveType)
}

// Get reads a save file and writes it to the response.
// Returns false if the save does not exist.
func (m *Manager) Get(w http.ResponseWriter, console, game, saveType string) bool {
	path := m.savePath(console, game, saveType)
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
	return true
}

// Put writes save data to disk atomically.
func (m *Manager) Put(console, game, saveType string, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("reading save data: %w", err)
	}

	return atomicfile.Write(m.savePath(console, game, saveType), func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

// ValidType returns true if the save type is valid.
func ValidType(t string) bool {
	return t == "state" || t == "sram"
}

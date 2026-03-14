package saves

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	dir := m.saveDir(console, game)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating save directory: %w", err)
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("reading save data: %w", err)
	}

	// Write to temp file in the same directory, then rename for atomicity
	tmp, err := os.CreateTemp(dir, ".save-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("writing save data: %w", err)
	}
	tmp.Close()

	finalPath := m.savePath(console, game, saveType)
	if err := os.Rename(tmp.Name(), finalPath); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("renaming save file: %w", err)
	}

	return nil
}

// ValidType returns true if the save type is valid.
func ValidType(t string) bool {
	return t == "state" || t == "sram"
}

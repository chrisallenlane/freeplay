package covers

import (
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CoverFetcher fetches cover art images for games.
type CoverFetcher interface {
	Fetch(gameName string, console string) (image.Image, error)
}

// Manager coordinates cover art fetching and storage.
type Manager struct {
	dataDir string
	fetcher CoverFetcher
}

// New creates a cover art Manager.
func New(dataDir string, fetcher CoverFetcher) *Manager {
	return &Manager{dataDir: dataDir, fetcher: fetcher}
}

var tagPattern = regexp.MustCompile(`\s*[\(\[].*?[\)\]]`)

// CleanName strips No-Intro tags from a ROM filename for API search.
func CleanName(nameWithoutExt string) string {
	return strings.TrimSpace(tagPattern.ReplaceAllString(nameWithoutExt, ""))
}

// Path returns the expected filesystem path for a game's cover art.
func Path(dataDir, console, filenameWithoutExt string) string {
	return filepath.Join(dataDir, "covers", console, filenameWithoutExt+".png")
}

// CoverPath returns the expected filesystem path for a game's cover art.
func (m *Manager) CoverPath(console, filenameWithoutExt string) string {
	return Path(m.dataDir, console, filenameWithoutExt)
}

// FetchMissing downloads cover art for games that don't have local covers.
// Each entry is {console, filename (with extension)}.
func (m *Manager) FetchMissing(games []GameEntry) {
	if m.fetcher == nil {
		return
	}

	ticker := time.NewTicker(334 * time.Millisecond) // ~3 req/s
	defer ticker.Stop()

	total := len(games)
	fetched := 0

	for _, g := range games {
		ext := filepath.Ext(g.Filename)
		nameNoExt := strings.TrimSuffix(g.Filename, ext)
		coverPath := m.CoverPath(g.Console, nameNoExt)

		// Skip if cover already exists
		if _, err := os.Stat(coverPath); err == nil {
			continue
		}

		fetched++
		slog.Info("fetching cover art", "progress", fetched, "total", total, "game", nameNoExt)

		cleanName := CleanName(nameNoExt)
		if cleanName == "" {
			continue
		}

		<-ticker.C
		img, err := m.fetcher.Fetch(cleanName, g.Console)
		if err != nil {
			slog.Warn("cover art fetch failed", "game", nameNoExt, "error", err)
			continue
		}
		if img == nil {
			continue
		}

		// Save as PNG
		coverDir := filepath.Dir(coverPath)
		if err := os.MkdirAll(coverDir, 0755); err != nil {
			slog.Warn("could not create cover directory", "path", coverDir, "error", err)
			continue
		}

		// Write to temp file, then rename
		tmp, err := os.CreateTemp(coverDir, ".cover-*")
		if err != nil {
			slog.Warn("could not create temp file for cover", "error", err)
			continue
		}

		if err := png.Encode(tmp, img); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			slog.Warn("could not encode cover as PNG", "error", err)
			continue
		}
		tmp.Close()

		if err := os.Rename(tmp.Name(), coverPath); err != nil {
			os.Remove(tmp.Name())
			slog.Warn("could not rename cover file", "error", err)
			continue
		}
	}
}

// GameEntry describes a game for cover art fetching.
type GameEntry struct {
	Console  string
	Filename string
}

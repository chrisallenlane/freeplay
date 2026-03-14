package covers

import (
	"image"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chrisallenlane/freeplay/internal/atomicfile"
)

// Fetcher fetches cover art images for games.
type Fetcher interface {
	Fetch(gameName string, console string, platformIDs []int) (image.Image, error)
}

// Manager coordinates cover art fetching and storage.
type Manager struct {
	dataDir string
	fetcher Fetcher
}

// New creates a cover art Manager.
func New(dataDir string, fetcher Fetcher) *Manager {
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

// FetchMissing downloads cover art for games that don't have local covers.
// Each entry is {console, filename (with extension)}.
// Returns the number of covers successfully saved.
func (m *Manager) FetchMissing(games []GameEntry) int {
	if m.fetcher == nil {
		return 0
	}

	ticker := time.NewTicker(334 * time.Millisecond) // ~3 req/s
	defer ticker.Stop()

	total := len(games)
	attempted := 0
	saved := 0

	for _, g := range games {
		ext := filepath.Ext(g.Filename)
		nameNoExt := strings.TrimSuffix(g.Filename, ext)
		coverPath := Path(m.dataDir, g.Console, nameNoExt)

		// Skip if cover already exists
		if _, err := os.Stat(coverPath); err == nil {
			continue
		}

		attempted++
		slog.Info("fetching cover art", "progress", attempted, "total", total, "game", nameNoExt)

		cleanName := CleanName(nameNoExt)
		if cleanName == "" {
			continue
		}

		<-ticker.C
		img, err := m.fetcher.Fetch(cleanName, g.Console, g.IGDBPlatformIDs)
		if err != nil {
			slog.Warn("cover art fetch failed", "game", nameNoExt, "error", err)
			continue
		}
		if img == nil {
			continue
		}

		if err := atomicfile.Write(coverPath, func(w io.Writer) error {
			return png.Encode(w, img)
		}); err != nil {
			slog.Warn("could not save cover art", "game", nameNoExt, "error", err)
			continue
		}
		saved++
	}
	return saved
}

// GameEntry describes a game for cover art fetching.
type GameEntry struct {
	Console         string
	Filename        string
	IGDBPlatformIDs []int
}

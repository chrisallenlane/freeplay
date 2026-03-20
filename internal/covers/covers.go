// Package covers manages cover art fetching and caching.
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
	"sync/atomic"
	"time"

	"github.com/chrisallenlane/freeplay/internal/atomicfile"
)

// Fetcher fetches cover art images for games.
type Fetcher interface {
	Fetch(gameName string, console string, platformIDs []int) (image.Image, error)
}

// Manager coordinates cover art fetching and storage.
type Manager struct {
	dataDir  string
	fetcher  Fetcher
	fetching atomic.Bool
}

// Fetching reports whether cover art is currently being fetched.
func (m *Manager) Fetching() bool {
	return m.fetching.Load()
}

// New creates a cover art Manager.
func New(dataDir string, fetcher Fetcher) *Manager {
	return &Manager{dataDir: dataDir, fetcher: fetcher}
}

var (
	tagPattern = regexp.MustCompile(`\s*[\(\[].*?[\)\]]`)
	hashSuffix = regexp.MustCompile(`\s+#\s+\S+$`)
)

// CleanName strips No-Intro tags and hash suffixes from a ROM filename
// for API search. Tags in parentheses/brackets (e.g. "(USA)", "[!]") and
// translation-patch suffixes (e.g. "# SNES") are removed.
func CleanName(nameWithoutExt string) string {
	name := tagPattern.ReplaceAllString(nameWithoutExt, "")
	name = hashSuffix.ReplaceAllString(name, "")
	return strings.TrimSpace(name)
}

// nameVariants returns search name variants ordered from highest to lowest
// confidence. Each variant represents a different heuristic for matching
// ROM filenames to IGDB game titles.
func nameVariants(cleanName string) []string {
	if cleanName == "" {
		return nil
	}

	seen := map[string]bool{cleanName: true}
	variants := []string{cleanName}

	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			variants = append(variants, s)
		}
	}

	// Dashes to colons: No-Intro uses " - " for subtitles, IGDB uses ": "
	add(strings.ReplaceAll(cleanName, " - ", ": "))

	// Spaces removed: catches compound-word titles (SimCity, SimAnt)
	add(strings.ReplaceAll(cleanName, " ", ""))

	// Subtitle dropped: catches regional subtitle mismatches
	if idx := strings.Index(cleanName, " - "); idx > 0 {
		add(strings.TrimSpace(cleanName[:idx]))
	} else if idx := strings.Index(cleanName, ": "); idx > 0 {
		add(strings.TrimSpace(cleanName[:idx]))
	}

	return variants
}

// coverPath returns the expected filesystem path for a game's cover art.
func coverPath(dataDir, console, filenameWithoutExt string) string {
	return filepath.Join(dataDir, "covers", console, filenameWithoutExt+".png")
}

// tryVariants tries each name variant with the given platform IDs, returning
// the first matching image or nil.
func (m *Manager) tryVariants(variants []string, console string, platformIDs []int, nameNoExt string, ticker *time.Ticker) (image.Image, bool) {
	for _, name := range variants {
		<-ticker.C
		img, err := m.fetcher.Fetch(name, console, platformIDs)
		if err != nil {
			slog.Warn("cover art fetch failed", "game", nameNoExt, "error", err)
			return nil, true
		}
		if img != nil {
			return img, false
		}
	}
	return nil, false
}

// FetchMissing downloads cover art for games that don't have local covers.
// Each entry is {console, filename (with extension)}.
// Returns the number of covers successfully saved.
func (m *Manager) FetchMissing(games []GameEntry) int {
	if m.fetcher == nil {
		return 0
	}

	m.fetching.Store(true)
	defer m.fetching.Store(false)

	ticker := time.NewTicker(334 * time.Millisecond) // ~3 req/s
	defer ticker.Stop()

	total := len(games)
	attempted := 0
	saved := 0

	for _, g := range games {
		ext := filepath.Ext(g.Filename)
		nameNoExt := strings.TrimSuffix(g.Filename, ext)
		coverPath := coverPath(m.dataDir, g.Console, nameNoExt)

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

		// Try name variants in order of confidence. All variants are
		// tried with platform constraint first (higher confidence), then
		// all variants again without platform constraint.
		variants := nameVariants(cleanName)
		img, fetchErr := m.tryVariants(variants, g.Console, g.IGDBPlatformIDs, nameNoExt, ticker)
		if img == nil && !fetchErr && len(g.IGDBPlatformIDs) > 0 {
			img, fetchErr = m.tryVariants(variants, g.Console, nil, nameNoExt, ticker)
		}

		if fetchErr || img == nil {
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

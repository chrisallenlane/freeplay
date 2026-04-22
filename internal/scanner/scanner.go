// Package scanner discovers and catalogs ROM files.
package scanner

import (
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chrisallenlane/freeplay/internal/config"
)

// Scanner builds and stores the game catalog.
type Scanner struct {
	cfg     *config.Config
	dataDir string
	catalog atomic.Pointer[Catalog]
	mu      sync.Mutex
}

// New creates a Scanner.
func New(cfg *config.Config, dataDir string) *Scanner {
	s := &Scanner{cfg: cfg, dataDir: dataDir}
	empty := &Catalog{Consoles: []string{}, Games: []Game{}}
	s.catalog.Store(empty)
	return s
}

// Scan rebuilds the catalog by reading ROM directories.
// Returns true if the scan ran, false if another scan is in progress.
func (s *Scanner) Scan() bool {
	if !s.mu.TryLock() {
		return false
	}
	defer s.mu.Unlock()

	s.scan()
	return true
}

// ScanBlocking acquires the lock (waiting if needed) and scans.
func (s *Scanner) ScanBlocking() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scan()
}

func (s *Scanner) scan() {
	var games []Game
	consoleSet := make(map[string]bool)

	for consoleName, rom := range s.cfg.ROMs {
		entries, err := os.ReadDir(rom.Path)
		if err != nil {
			slog.Warn("could not read ROM directory", "console", consoleName, "path", rom.Path, "error", err)
			continue
		}

		consoleSet[consoleName] = true
		hasBios := rom.Bios != ""

		// Build sets of existing cover/manual filenames for O(1) lookup,
		// replacing per-ROM os.Stat calls.
		covers := make(map[string]bool)
		coverDir := filepath.Join(s.dataDir, "covers", consoleName)
		if coverEntries, err := os.ReadDir(coverDir); err == nil {
			for _, ce := range coverEntries {
				covers[ce.Name()] = true
			}
		}

		manuals := make(map[string]bool)
		manualDir := filepath.Join(s.dataDir, "manuals", consoleName)
		if manualEntries, err := os.ReadDir(manualDir); err == nil {
			for _, me := range manualEntries {
				manuals[me.Name()] = true
			}
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			filename := entry.Name()
			nameNoExt := strings.TrimSuffix(filename, filepath.Ext(filename))

			games = append(games, Game{
				Filename:        filename,
				Console:         consoleName,
				Core:            rom.Core,
				HasCover:        covers[nameNoExt+".png"],
				HasManual:       manuals[nameNoExt+".pdf"],
				HasBios:         hasBios,
				IGDBPlatformIDs: rom.IGDBPlatformIDs,
			})
		}
	}

	// Sort consoles alphabetically
	consoles := make([]string, 0, len(consoleSet))
	for c := range consoleSet {
		consoles = append(consoles, c)
	}
	sort.Strings(consoles)

	// Sort games by console then filename
	sort.Slice(games, func(i, j int) bool {
		if games[i].Console != games[j].Console {
			return games[i].Console < games[j].Console
		}
		return games[i].Filename < games[j].Filename
	})

	if games == nil {
		games = []Game{}
	}

	catalog := &Catalog{Consoles: consoles, Games: games}
	s.catalog.Store(catalog)

	slog.Info("scan complete", "consoles", len(consoles), "games", len(games))
}

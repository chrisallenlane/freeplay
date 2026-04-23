// Package scanner discovers and catalogs ROM files.
package scanner

import (
	"cmp"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
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
	// lastGameCount hints at the capacity of the games slice on the next
	// scan. Zero on first scan; after that it tracks the previous
	// catalog size so re-scans start with a correctly-sized allocation.
	lastGameCount int
}

// New creates a Scanner.
func New(cfg *config.Config, dataDir string) *Scanner {
	s := &Scanner{cfg: cfg, dataDir: dataDir}
	s.catalog.Store(newCatalog([]string{}, []Game{}))
	return s
}

// ScanBlocking acquires the lock (waiting if needed) and scans.
// Concurrent rescans are gated by internal/library.Library; the
// scanner's own mutex exists only to serialize the catalog rebuild.
func (s *Scanner) ScanBlocking() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scan()
}

func (s *Scanner) scan() {
	games := make([]Game, 0, s.lastGameCount)
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
	slices.Sort(consoles)

	// Sort games by console then filename. Key collisions require two
	// ROMs with identical filenames under the same console directory —
	// the filesystem prevents that, so sort stability is not required.
	slices.SortFunc(games, func(a, b Game) int {
		if c := cmp.Compare(a.Console, b.Console); c != 0 {
			return c
		}
		return cmp.Compare(a.Filename, b.Filename)
	})

	s.lastGameCount = len(games)

	s.catalog.Store(newCatalog(consoles, games))

	slog.Info("scan complete", "consoles", len(consoles), "games", len(games))
}

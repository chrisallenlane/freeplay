// Package scanner discovers and catalogs ROM files.
package scanner

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/covers"
)

// Game represents a single ROM in the catalog.
type Game struct {
	Filename        string `json:"filename"`
	Console         string `json:"console"`
	Core            string `json:"core"`
	HasCover        bool   `json:"hasCover"`
	HasBios         bool   `json:"hasBios"`
	IGDBPlatformIDs []int  `json:"igdbPlatformIds,omitempty"`
}

// Catalog is the full game library served by GET /api/games.
type Catalog struct {
	Consoles []string `json:"consoles"`
	Games    []Game   `json:"games"`
}

// ScanCallback is called after a scan completes with the list of games.
type ScanCallback func(games []Game)

// Scanner builds and stores the game catalog.
type Scanner struct {
	cfg            *config.Config
	dataDir        string
	catalog        atomic.Pointer[Catalog]
	mu             sync.Mutex
	onScanComplete ScanCallback
}

// New creates a Scanner.
func New(cfg *config.Config, dataDir string) *Scanner {
	s := &Scanner{cfg: cfg, dataDir: dataDir}
	empty := &Catalog{Consoles: []string{}, Games: []Game{}}
	s.catalog.Store(empty)
	return s
}

// Catalog returns the current catalog.
func (s *Scanner) Catalog() *Catalog {
	return s.catalog.Load()
}

// CatalogJSON returns the catalog as JSON bytes.
func (s *Scanner) CatalogJSON() ([]byte, error) {
	return json.Marshal(s.catalog.Load())
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
		_, hasBios := s.cfg.BIOS[consoleName]

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			filename := entry.Name()
			nameNoExt := strings.TrimSuffix(filename, filepath.Ext(filename))
			coverPath := covers.Path(s.dataDir, consoleName, nameNoExt)
			_, coverErr := os.Stat(coverPath)

			games = append(games, Game{
				Filename:        filename,
				Console:         consoleName,
				Core:            rom.Core,
				HasCover:        coverErr == nil,
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

	if s.onScanComplete != nil {
		s.onScanComplete(games)
	}
}

// SetOnScanComplete sets a callback that fires after each scan.
func (s *Scanner) SetOnScanComplete(cb ScanCallback) {
	s.onScanComplete = cb
}

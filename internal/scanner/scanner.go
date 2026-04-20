// Package scanner discovers and catalogs ROM files.
package scanner

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chrisallenlane/freeplay/internal/config"
)

// Game represents a single ROM in the catalog.
type Game struct {
	Filename        string   `json:"filename"`
	Console         string   `json:"console"`
	Core            string   `json:"core"`
	HasCover        bool     `json:"hasCover"`
	HasManual       bool     `json:"hasManual"`
	HasBios         bool     `json:"hasBios"`
	IGDBPlatformIDs []int    `json:"igdbPlatformIds,omitempty"`
	IGDBName        string   `json:"igdbName,omitempty"`
	Developers      []string `json:"developers,omitempty"`
	Publishers      []string `json:"publishers,omitempty"`
	Year            int      `json:"year,omitempty"`
}

// Catalog is the full game library served by GET /api/games.
type Catalog struct {
	Consoles []string `json:"consoles"`
	Games    []Game   `json:"games"`
}

// ScanCallback is called after a scan completes with the list of games.
type ScanCallback func(games []Game)

// DetailsLookup returns cached IGDB metadata for a game. The returned struct
// is owned by the caller; the fields Name, Developers, Publishers, and
// FirstReleaseDate are the ones used by EnrichMetadata. Returns nil if the
// game is not cached.
type DetailsLookup func(console, romFilename string) *GameMeta

// GameMeta carries the IGDB fields needed for catalog enrichment.
type GameMeta struct {
	Name             string
	Developers       []string
	Publishers       []string
	FirstReleaseDate string // ISO 8601, e.g. "2020-12-10"
}

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

// CatalogJSON returns the catalog as JSON bytes.
func (s *Scanner) CatalogJSON() ([]byte, error) {
	return json.Marshal(s.catalog.Load())
}

// EnrichMetadata populates IGDBName, Developers, Publishers, and Year for
// each game in the catalog using the provided lookup function.
func (s *Scanner) EnrichMetadata(lookup DetailsLookup) {
	cat := s.catalog.Load()
	enriched := &Catalog{Consoles: cat.Consoles, Games: make([]Game, len(cat.Games))}
	copy(enriched.Games, cat.Games)
	for i := range enriched.Games {
		g := &enriched.Games[i]
		meta := lookup(g.Console, g.Filename)
		if meta == nil {
			continue
		}
		if meta.Name != "" {
			g.IGDBName = meta.Name
		}
		if len(meta.Developers) > 0 {
			g.Developers = meta.Developers
		}
		if len(meta.Publishers) > 0 {
			g.Publishers = meta.Publishers
		}
		g.Year = parseYear(meta.FirstReleaseDate)
	}
	s.catalog.Store(enriched)
}

// parseYear extracts the four-digit year from an ISO 8601 date string
// (e.g. "2020-12-10" → 2020, "1985" → 1985). Returns 0 for empty or
// unparseable input so the field is omitted from JSON via omitempty.
func parseYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
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

	if s.onScanComplete != nil {
		s.onScanComplete(games)
	}
}

// SetOnScanComplete sets a callback that fires after each scan.
func (s *Scanner) SetOnScanComplete(cb ScanCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onScanComplete = cb
}

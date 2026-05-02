package scanner

import (
	"encoding/json"
	"strconv"
	"strings"
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
// Games is the canonical list served to clients; gameSet and slugSet are
// derived indexes that back HasGame and HasGameSlug in O(1). All three are
// published atomically via atomic.Pointer on the Scanner. Use newCatalog so
// the indexes never drift from the slice.
type Catalog struct {
	Consoles []string            `json:"consoles"`
	Games    []Game              `json:"games"`
	gameSet  map[string]struct{} `json:"-"`
	slugSet  map[string]struct{} `json:"-"`
}

// newCatalog builds a Catalog with freshly-indexed gameSet and slugSet
// matching the given games slice. Shared by scan() (first construction) and
// EnrichMetadata (replacement after lookup-based enrichment) so the indexes
// always track the slice.
func newCatalog(consoles []string, games []Game) *Catalog {
	set := make(map[string]struct{}, len(games))
	slugs := make(map[string]struct{}, len(games))
	for i := range games {
		set[games[i].Console+"/"+games[i].Filename] = struct{}{}
		slugs[games[i].Console+"/"+stripExt(games[i].Filename)] = struct{}{}
	}
	return &Catalog{
		Consoles: consoles,
		Games:    games,
		gameSet:  set,
		slugSet:  slugs,
	}
}

// stripExt returns filename with its final extension removed. Mirrors the
// frontend's stripExt helper in frontend/urls.js: the last dot must be at
// index > 0, so dotfiles like ".bashrc" are returned unchanged.
func stripExt(filename string) string {
	dot := strings.LastIndex(filename, ".")
	if dot > 0 {
		return filename[:dot]
	}
	return filename
}

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

// Catalog returns the current catalog snapshot.
func (s *Scanner) Catalog() *Catalog {
	return s.catalog.Load()
}

// HasGame reports whether the given (console, filename) pair is in the
// current catalog. Used by save-upload handlers to reject writes for
// games not in the library.
func (s *Scanner) HasGame(console, filename string) bool {
	cat := s.catalog.Load()
	_, ok := cat.gameSet[console+"/"+filename]
	return ok
}

// HasGameSlug reports whether the given (console, slug) pair matches a game in
// the current catalog, where slug is the ROM filename with its final extension
// stripped — the URL convention used by the frontend
// (/api/saves/{console}/{slug}/{type}). Use this for save endpoints; use
// HasGame when the caller has the full filename. See frontend/urls.js's
// stripExt for the frontend counterpart.
func (s *Scanner) HasGameSlug(console, slug string) bool {
	cat := s.catalog.Load()
	_, ok := cat.slugSet[console+"/"+slug]
	return ok
}

// CatalogJSON returns the catalog as JSON bytes.
func (s *Scanner) CatalogJSON() ([]byte, error) {
	return json.Marshal(s.catalog.Load())
}

// EnrichMetadata populates IGDBName, Developers, Publishers, and Year for
// each game in the catalog using the provided lookup function.
func (s *Scanner) EnrichMetadata(lookup DetailsLookup) {
	cat := s.catalog.Load()
	games := make([]Game, len(cat.Games))
	copy(games, cat.Games)
	for i := range games {
		g := &games[i]
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
	s.catalog.Store(newCatalog(cat.Consoles, games))
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

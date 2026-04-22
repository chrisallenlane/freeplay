package scanner

import (
	"encoding/json"
	"strconv"
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

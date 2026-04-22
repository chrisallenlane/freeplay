// Package library orchestrates the scan → enrich → fetch cycle that
// keeps the in-memory catalog in sync with the ROM directory and the
// on-disk IGDB cache.
//
// It owns the only production caller of scanner.ScanBlocking and
// details.Cache.FetchAll. Collecting this sequencing in one place
// fixes two design smells that previously lived in main.go: an
// unbounded chain of rescan goroutines, and a scanner callback that
// fired while the scanner's mutex was held.
package library

import (
	"log/slog"
	"sync"

	"github.com/chrisallenlane/freeplay/internal/igdb"
	"github.com/chrisallenlane/freeplay/internal/scanner"
)

// maxRescanIterations caps the scan → fetch → rescan loop. A bound
// prevents a pathological case where FetchAll repeatedly reports saved > 0
// (for example, if disk writes keep failing silently) from spinning
// forever. In normal operation the loop completes after one or two
// iterations.
const maxRescanIterations = 3

// DetailsCache is the subset of details.Cache the library uses.
// Declared as an interface so tests can swap in a double.
type DetailsCache interface {
	Get(console, romFilename string) *igdb.GameDetails
	FetchAll(games []igdb.GameEntry) int
}

// Library owns the scan/enrich/fetch pipeline. The zero value is not
// usable; construct with New.
type Library struct {
	scanner *scanner.Scanner
	cache   DetailsCache
	mu      sync.Mutex
}

// New creates a Library that coordinates the given scanner and details
// cache. cache may be nil if IGDB is not configured; in that case the
// enrich and fetch steps are no-ops.
func New(scn *scanner.Scanner, cache DetailsCache) *Library {
	return &Library{scanner: scn, cache: cache}
}

// Start performs the initial scan and then runs the enrich/fetch pipeline
// in a background goroutine. It returns immediately; callers that need
// the pipeline to finish before proceeding should call RunPipeline
// directly.
func (l *Library) Start() {
	go l.RunPipeline()
}

// TriggerRescan runs the pipeline on demand. Returns true if a new
// pipeline was started; false if one is already in progress. The
// false return maps naturally to the HTTP 409 that the rescan route
// exposes.
func (l *Library) TriggerRescan() bool {
	if !l.mu.TryLock() {
		return false
	}
	go func() {
		defer l.mu.Unlock()
		l.runPipelineLocked()
	}()
	return true
}

// RunPipeline executes the full pipeline synchronously. It blocks until
// the pipeline finishes. Used at startup so the first HTTP handlers
// observe a populated catalog.
func (l *Library) RunPipeline() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.runPipelineLocked()
}

// runPipelineLocked is the actual pipeline body. It must be called with
// l.mu held so at most one pipeline runs at a time.
func (l *Library) runPipelineLocked() {
	for iter := 0; iter < maxRescanIterations; iter++ {
		l.scanner.ScanBlocking()
		l.scanner.EnrichMetadata(l.metaLookup)

		entries := l.entriesFromCatalog()
		saved := 0
		if l.cache != nil {
			saved = l.cache.FetchAll(entries)
		}
		if saved == 0 {
			return
		}
		slog.Info("pipeline iterated", "saved", saved, "iter", iter+1)
	}
	slog.Warn(
		"pipeline hit max iterations; stopping to avoid a spin loop",
		"max", maxRescanIterations,
	)
}

// metaLookup adapts details.Cache.Get into the scanner.DetailsLookup
// shape. Returns nil when IGDB is not configured or the game is not
// cached.
func (l *Library) metaLookup(console, romFilename string) *scanner.GameMeta {
	if l.cache == nil {
		return nil
	}
	d := l.cache.Get(console, romFilename)
	if d == nil {
		return nil
	}
	return &scanner.GameMeta{
		Name:             d.Name,
		Developers:       d.Developers,
		Publishers:       d.Publishers,
		FirstReleaseDate: d.FirstReleaseDate,
	}
}

// entriesFromCatalog builds the GameEntry list FetchAll wants from the
// current catalog JSON shape.
func (l *Library) entriesFromCatalog() []igdb.GameEntry {
	cat := l.scanner.Catalog()
	entries := make([]igdb.GameEntry, len(cat.Games))
	for i, g := range cat.Games {
		entries[i] = igdb.GameEntry{
			Console:         g.Console,
			Filename:        g.Filename,
			IGDBPlatformIDs: g.IGDBPlatformIDs,
		}
	}
	return entries
}

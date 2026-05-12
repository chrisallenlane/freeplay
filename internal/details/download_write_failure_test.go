package details

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/datadir"
)

// blockAtomicWriteAt makes the given target path unreachable for atomicfile.Write
// by placing a regular file at one of the parent path components. atomicfile.Write
// calls os.MkdirAll(filepath.Dir(target), 0o750) first; MkdirAll fails with
// "not a directory" when it has to traverse through a regular file. This works
// even when the test process is root, unlike chmod-based approaches.
//
// We place the blocking file two levels above target so the test never created
// the immediate parent — that way the "path component is a file" error is what
// atomicfile.Write sees on its very first MkdirAll call.
//
// Returns the path of the blocking file so the caller can inspect it.
func blockAtomicWriteAt(t *testing.T, target string) string {
	t.Helper()
	// target's grandparent (i.e. the directory that should HOLD the cleanName
	// or console dir) is where we plant the blocker.
	blockerDir := filepath.Dir(filepath.Dir(target))
	if err := os.MkdirAll(filepath.Dir(blockerDir), 0o750); err != nil {
		t.Fatalf("setting up blocker parent: %v", err)
	}
	// Put a regular file at blockerDir; MkdirAll will refuse to descend
	// through it.
	if err := os.WriteFile(blockerDir, []byte("blocker"), 0o600); err != nil {
		t.Fatalf("planting blocker file at %q: %v", blockerDir, err)
	}
	return blockerDir
}

// findRecord returns the first slog.Record whose message contains needle, or
// nil if none matched. Used to assert both that a log line fired and that it
// carried specific attributes.
func (h *recordingHandler) findRecord(needle string) *slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.records {
		if strings.Contains(h.records[i].Message, needle) {
			return &h.records[i]
		}
	}
	return nil
}

// attrValue returns the string form of the first attribute on r whose key
// matches key. Returns "" if no such attribute exists.
func attrValue(r *slog.Record, key string) string {
	var got string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			got = a.Value.String()
			return false
		}
		return true
	})
	return got
}

// captureSlog redirects slog.Default to a recordingHandler for the duration
// of the test and returns the handler.
func captureSlog(t *testing.T) *recordingHandler {
	t.Helper()
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	h := &recordingHandler{}
	slog.SetDefault(slog.New(h))
	return h
}

// TestWriteNotFound_LogsOnWriteFailure asserts that when the .notfound marker
// cannot be written (disk full, read-only mount, broken permissions),
// writeNotFound surfaces the problem via a warn-level slog line that
// identifies the game and the cache path. Without this log, an operator has
// zero observability for an unbounded IGDB-quota leak: each rescan re-fetches
// every "not found" game from IGDB because the negative-cache marker never
// lands.
//
// Expected behaviour: a warn log with a "game" attribute carrying cleanName.
// Current behaviour: writeNotFound silently returns — test FAILS, bug confirmed.
func TestWriteNotFound_LogsOnWriteFailure(t *testing.T) {
	h := captureSlog(t)

	dir := t.TempDir()
	c := New(dir, nil)

	target := c.notFoundPath("NES", "Unknown Game")
	blockAtomicWriteAt(t, target)

	c.writeNotFound("NES", "Unknown Game")

	// Some log line should fire describing the failure.
	if len(h.records) == 0 {
		t.Fatal(
			"writeNotFound silently swallowed an atomicfile.Write failure; " +
				"expected a warn-level slog with game name and error so " +
				"operators can diagnose unbounded IGDB re-fetches",
		)
	}

	// Match the production message verbatim. If the message ever changes,
	// the test should fail loudly and force a deliberate update rather
	// than silently match a broad substring that happens to overlap.
	rec := h.findRecord("writing .notfound marker failed")
	if rec == nil {
		t.Fatalf(
			"writeNotFound logged %d records but none match the expected "+
				"message; messages: %v",
			len(h.records), h.messages(),
		)
	}
	if rec.Level < slog.LevelWarn {
		t.Errorf(
			"writeNotFound failure logged at %s, want >= WARN",
			rec.Level,
		)
	}
	if got := attrValue(rec, "game"); got != "Unknown Game" {
		t.Errorf("missing/wrong 'game' attr: got %q, want %q", got, "Unknown Game")
	}
}

// TestWriteNotFound_NoInMemoryPoisonOnWriteFailure verifies that when the
// .notfound marker write fails, the in-memory negative-cache is NOT populated.
// Memoizing a negative entry without a backing marker would mean a process
// restart silently loses that "known not found" status — operators would have
// no way to spot the discrepancy.
//
// This is the correct half of the writeNotFound failure path; the test exists
// to lock the behaviour in (and to disambiguate the design intent: the silence
// is a logging bug, not a memoization bug).
func TestWriteNotFound_NoInMemoryPoisonOnWriteFailure(t *testing.T) {
	_ = captureSlog(t) // suppress log spam from the eventual fix

	dir := t.TempDir()
	c := New(dir, nil)

	target := c.notFoundPath("NES", "Unknown Game")
	blockAtomicWriteAt(t, target)

	c.writeNotFound("NES", "Unknown Game")

	if _, ok := c.load("NES", "Unknown Game"); ok {
		t.Error(
			"writeNotFound poisoned the in-memory negative cache after a " +
				"write failure; a process restart would silently lose the " +
				"negative entry — memoization must require a successful write",
		)
	}
}

// TestEnsureCoverThumbnail_LogsOnWriteFailure asserts that when the per-ROM
// cover-grid PNG cannot be written, ensureCoverThumbnail surfaces the failure
// via a warn-level slog line. Without it, the library grid silently shows the
// fallback placeholder for that game forever — operators have nothing to
// correlate against.
//
// Expected behaviour: warn log with "game" attribute.
//
// Paired with TestEnsureCoverThumbnail_NoSilentNoOpOnWriteFailure: this test
// pins the exact attribute shape ("game" attr present), and the companion
// relaxes that to "some log fires" so the strict shape can evolve without
// losing the no-silent-failure guarantee. Keep both — neither subsumes the
// other.
func TestEnsureCoverThumbnail_LogsOnWriteFailure(t *testing.T) {
	h := captureSlog(t)

	dir := t.TempDir()
	c := New(dir, nil)

	// Plant a cover_thumb.jpg so ensureCoverThumbnail has something to copy.
	cacheDir := c.cacheDir("NES", "Test Game")
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(cacheDir, "cover_thumb.jpg"),
		[]byte("imgdata"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	// Block atomic-write to the destination by placing a regular file
	// where the console covers directory would go.
	dst := datadir.CoverFile(dir, "NES", "Test Game")
	blockAtomicWriteAt(t, dst)

	c.ensureCoverThumbnail("NES", "Test Game", "Test Game")

	if len(h.records) == 0 {
		t.Fatal(
			"ensureCoverThumbnail silently swallowed an atomicfile.Write " +
				"failure; expected a warn-level slog so operators can " +
				"diagnose permanently-missing library cover thumbnails",
		)
	}

	// Don't be picky about exact wording; require warn-level + "game" attr.
	for _, r := range h.records {
		if r.Level >= slog.LevelWarn && attrValue(&r, "game") == "Test Game" {
			return
		}
	}
	t.Errorf(
		"no warn-level record with game=\"Test Game\" found; messages: %v",
		h.messages(),
	)
}

// TestEnsureCoverThumbnail_NoSilentNoOpOnWriteFailure is a relaxed companion
// to the log-assertion test above. Even if a maintainer disagrees with the
// precise log shape, the *minimum* fix must be that the write failure is no
// longer indistinguishable from "nothing to do".
//
// Strategy: arrange a guaranteed write failure, call ensureCoverThumbnail, and
// require that *some* slog record fired. This is a lower bar than the test
// above and exists so the two tests can diverge if the project picks a
// different attr scheme.
func TestEnsureCoverThumbnail_NoSilentNoOpOnWriteFailure(t *testing.T) {
	h := captureSlog(t)

	dir := t.TempDir()
	c := New(dir, nil)

	cacheDir := c.cacheDir("NES", "Test Game")
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(cacheDir, "cover_thumb.jpg"),
		[]byte("imgdata"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	dst := datadir.CoverFile(dir, "NES", "Test Game")
	blockAtomicWriteAt(t, dst)

	c.ensureCoverThumbnail("NES", "Test Game", "Test Game")

	if len(h.records) == 0 {
		t.Error(
			"ensureCoverThumbnail produced zero log output despite a forced " +
				"atomicfile.Write failure — silent failure mode confirmed",
		)
	}
}

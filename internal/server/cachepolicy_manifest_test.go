package server

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/datadir"
	"github.com/chrisallenlane/freeplay/internal/igdb"
)

// TestRouteCachePolicyManifest is the route-to-cache-policy contract
// test. Every routable URL is listed with its expected Cache-Control
// value and whether it should carry an ETag. The intent is regression
// defense for the bug class that produced #57 and #58: cache policy
// drift on a new route, an `immutable` slipping back in, or an ETag
// disappearing from /emulatorjs/*.
//
// Adding a new route in routes.go without adding a row here is the
// drift mode. We cannot enforce this via reflection (Go's ServeMux
// does not expose registered patterns), so the protection is
// procedural: code review notices the missing row, OR a subsequent
// release surfaces unexpected caching behavior in production.
//
// 404/error-path cache contracts live in cachecontrol_404_test.go;
// this file covers only the success path.
func TestRouteCachePolicyManifest(t *testing.T) {
	cached := &igdb.GameDetails{Name: "Mega Man", Summary: "Cached."}
	cache := &mockDetailsCache{
		details: map[string]*igdb.GameDetails{"NES/Mega Man.nes": cached},
	}
	srv, dir := testServer(t, cache)
	srv.scanner.ScanBlocking()

	// Seed a save so GET /api/saves returns 200 rather than 404.
	doRequest(t, srv, http.MethodPost,
		"/api/saves/NES/Mega%20Man/sram", bytes.NewReader([]byte("save")))

	// Seed files for cover/manual/cache routes so they return 200.
	writeTestFile(t, filepath.Join(datadir.Covers(dir), "NES", "Mega Man.jpg"), []byte("jpg"))
	writeTestFile(t, filepath.Join(datadir.Manuals(dir), "NES", "Mega Man.pdf"), []byte("pdf"))
	writeCacheFile(t, dir, "NES", "Mega Man", "cover_thumb.jpg", []byte("jpg"))

	// The expected ETag value comes from testServer's "test" version
	// stamp via cacheWithETag (see server.go's New).
	const emulatorjsETag = `"freeplay-test"`

	cases := []struct {
		name     string
		path     string
		wantCC   string
		wantETag string // "" means no ETag expected
	}{
		// API: noStore-wrapped, errors and successes both no-store
		// except /api/game-details success which overrides.
		{"api health", "/api/health", "no-store", ""},
		{"api games", "/api/games", "no-store", ""},
		{"api status", "/api/status", "no-store", ""},
		{"api saves get", "/api/saves/NES/Mega%20Man/sram", "no-store", ""},
		{
			"api game-details success",
			"/api/game-details?console=NES&rom=Mega+Man.nes",
			"private, max-age=300", "",
		},

		// EmulatorJS: long cache + version-stamped ETag.
		{"emulatorjs loader", "/emulatorjs/data/loader.js", longCache, emulatorjsETag},

		// Static asset routes: long cache, no ETag.
		{"rom", "/roms/NES/Mega%20Man.nes", longCache, ""},
		{"bios", "/bios/NES", longCache, ""},
		{"cover", "/covers/NES/Mega%20Man.jpg", longCache, ""},
		{"manual", "/manuals/NES/Mega%20Man.pdf", longCache, ""},
		{
			"igdb cache",
			"/cache/igdb/NES/Mega%20Man/cover_thumb.jpg", longCache, "",
		},

		// Pages and frontend catch-all: no-cache (revalidate on every
		// reload so a deploy is picked up immediately).
		{"details page", "/details", "no-cache", ""},
		{"play page", "/play", "no-cache", ""},
		{"index", "/", "no-cache", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := doRequest(t, srv, http.MethodGet, c.path, nil)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			if got := w.Header().Get("Cache-Control"); got != c.wantCC {
				t.Errorf("Cache-Control = %q, want %q", got, c.wantCC)
			}
			// `immutable` is banned project-wide; assert here as a
			// belt-and-braces check beyond the constant-pin test.
			if strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
				t.Errorf("Cache-Control contains `immutable` — banned project-wide")
			}
			gotETag := w.Header().Get("ETag")
			if c.wantETag != "" && gotETag != c.wantETag {
				t.Errorf("ETag = %q, want %q", gotETag, c.wantETag)
			}
			if c.wantETag == "" && gotETag != "" {
				t.Errorf("unexpected ETag = %q on %s", gotETag, c.path)
			}
		})
	}
}

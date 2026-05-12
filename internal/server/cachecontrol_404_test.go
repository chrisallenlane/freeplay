package server

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestEmulatorJS404CacheControl pins what should be the correct contract
// for non-2xx responses on the /emulatorjs/ route: a 404 must not be
// cached as `immutable, max-age=31536000`. Browsers honor Cache-Control
// on 404 responses (they are heuristically cacheable per RFC 7234, and
// `max-age` makes them explicitly cacheable). A typo, a stale <script src>
// after an EmulatorJS upgrade, or any URL that briefly 404s during deploy
// would be permanently cached as "not found" for up to a year per client
// without manual cache clear — and `immutable` instructs the browser to
// skip revalidation entirely.
//
// This test reproduces the bug: cacheControl middleware in routes.go sets
// the Cache-Control header BEFORE the inner handler runs. The inner
// handler (noDirListing → http.FileServerFS) returns 404 for missing
// assets, but the long-cache header has already been set.
func TestEmulatorJS404CacheControl(t *testing.T) {
	srv, _ := testServer(t)

	tests := []struct {
		name string
		path string
	}{
		{"missing top-level file", "/emulatorjs/does-not-exist.js"},
		{"missing nested file", "/emulatorjs/data/missing-loader.js"},
		{"missing deep path", "/emulatorjs/cores/nonexistent.wasm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(t, srv, http.MethodGet, tt.path, nil)
			if w.Code != http.StatusNotFound {
				t.Fatalf("got status %d, want 404", w.Code)
			}
			cc := w.Header().Get("Cache-Control")
			// A 404 must NOT carry the immutable long-cache directive.
			// Acceptable values include "no-store", "no-cache", or an
			// absent header (so the browser falls back to heuristic
			// caching, which is short-lived for unexpected statuses).
			if strings.Contains(cc, "immutable") {
				t.Errorf("404 response has Cache-Control %q — browsers will cache the 404 immutably for max-age seconds; a typo or stale URL gets permanently broken until cache clear", cc)
			}
			if strings.Contains(cc, "max-age=31536000") {
				t.Errorf("404 response has Cache-Control %q — browsers will cache the 404 for a year", cc)
			}
		})
	}
}

// TestEmulatorJSDirListing404CacheControl exercises the 404 path that
// reproduces the middleware-ordering bug: noDirListing emits 404 directly
// (via http.NotFound) for a directory without an index.html. cacheControl
// runs before noDirListing, so the long-cache header gets stamped onto
// the response and http.NotFound does not strip it.
//
// HIGH severity: A browser that ever sees this 404 caches it as
// "immutable" for max-age=31536000 (one year). The same URL can never
// successfully load for that client until the cache is manually cleared,
// even after the operator drops the missing file in. Compounding factor:
// EmulatorJS includes runtime path probing that requests speculative
// asset URLs (cores, BIOS files, language files); any URL that briefly
// 404s — typo, version bump, race during deploy — becomes a permanent
// per-client failure.
//
// http.FileServerFS DOES clear Cache-Control on its own 404s (Go stdlib
// behavior in serveError), so a missing FILE under /emulatorjs/ is OK;
// only the noDirListing path (directory without index.html OR root of
// /emulatorjs/ without an index) trips this.
func TestEmulatorJSDirListing404CacheControl(t *testing.T) {
	srv, _ := testServer(t)

	tests := []struct {
		name string
		path string
	}{
		// /emulatorjs/data/ resolves to a directory (testEmulatorjsFS
		// has emulatorjs/data/loader.js); noDirListing returns 404
		// because there is no index.html inside.
		{"directory without index", "/emulatorjs/data/"},
		// /emulatorjs/ (the root) also resolves to a directory; same
		// noDirListing path. Triggered by any client that probes the
		// route root after a typo in the EmulatorJS bootstrap URL.
		{"emulatorjs root without index", "/emulatorjs/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(t, srv, http.MethodGet, tt.path, nil)
			if w.Code != http.StatusNotFound {
				t.Fatalf("got status %d, want 404", w.Code)
			}
			cc := w.Header().Get("Cache-Control")
			if strings.Contains(cc, "immutable") || strings.Contains(cc, "max-age=31536000") {
				t.Errorf("404 Cache-Control = %q — browsers cache the 404 immutably for a year; the same URL stays broken until manual cache clear", cc)
			}
		})
	}
}

// TestCovers404CacheControl pins the contract for /covers/ 404s. The
// `longCacheMutable` policy (public, max-age=31536000) is less severe
// than immutable — browsers will revalidate via If-Modified-Since after
// the max-age — but a 404 cached for a year still means the cover stays
// "missing" client-side until the user manually clears cache or hits
// shift-reload, even after the operator drops the file in the covers
// directory.
//
// MEDIUM severity rather than HIGH because the cover failure is visual,
// not functional — the page still loads. But it's a real bug.
func TestCovers404CacheControl(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/covers/NES/missing-cover.png", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if strings.Contains(cc, "max-age=31536000") {
		t.Errorf("/covers 404 Cache-Control = %q — once the operator adds the missing cover, clients will see the cached 404 for up to a year", cc)
	}
}

// TestManuals404CacheControl pins the contract for /manuals/ 404s.
// Mirror of TestCovers404CacheControl for the manuals route, which
// shares the longCacheMutable policy via serveSecureFile.
func TestManuals404CacheControl(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/manuals/NES/missing-manual.pdf", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if strings.Contains(cc, "max-age=31536000") {
		t.Errorf("/manuals 404 Cache-Control = %q — once the operator adds the missing manual, clients will see the cached 404 for up to a year", cc)
	}
}

// TestROM404CacheControl exercises handleROM's NotFound path for an
// unknown {file} under a known console. handleROM only calls
// http.NotFound when the {console} is unknown; for unknown {file} the
// 404 comes out of serveSecureFile → serveFile via os.Stat error. In
// the latter path, serveFile returns before setting Cache-Control, so
// the header should be absent — but verify either way that no long-
// cache directive sticks to a 404.
func TestROM404CacheControl(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/roms/NES/NotARealROM.nes", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if strings.Contains(cc, "max-age=31536000") {
		t.Errorf("/roms 404 Cache-Control = %q — caching a 404 for a year would break ROM access after the operator adds the file", cc)
	}
}

// TestBIOS404CacheControl exercises handleBIOS's NotFound path. Unlike
// ROM, BIOS 404 comes from handleBIOS itself (unknown console or empty
// bios path); the long-cache header never gets set in that path.
// Provided as a regression guard.
func TestBIOS404CacheControl(t *testing.T) {
	srv, _ := testServer(t)

	// Unknown console — 404 from handleBIOS directly.
	w := doRequest(t, srv, http.MethodGet, "/bios/SNES", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if strings.Contains(cc, "max-age=31536000") {
		t.Errorf("/bios 404 Cache-Control = %q", cc)
	}
}

// TestCacheFiles404CacheControl exercises the /cache/igdb/ 404 path.
// Same longCacheMutable policy as covers and manuals.
func TestCacheFiles404CacheControl(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet,
		"/cache/igdb/NES/Missing/cover.jpg", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if strings.Contains(cc, "max-age=31536000") {
		t.Errorf("/cache/igdb 404 Cache-Control = %q — once IGDB cache is repopulated, clients will see cached 404s for up to a year", cc)
	}
}

// TestSecurityHeadersForbidden403CacheControl pins that a securityHeaders
// 403 (POST without X-Requested-With) does not carry a long-cache
// header. Browsers do not heuristically cache POST 403s (POST is not
// cacheable per RFC 7231), but verify there's nothing pathological in
// the header pipeline.
func TestSecurityHeadersForbidden403CacheControl(t *testing.T) {
	srv, _ := testServer(t)

	dropCSRF := http.Header{"X-Requested-With": {""}}
	endpoints := []string{
		"/api/rescan",                     // noStore wrapped
		"/api/saves/NES/Mega%20Man/state", // noStore wrapped
	}
	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			var body io.Reader = bytes.NewReader([]byte("x"))
			w := doRequest(t, srv, http.MethodPost, ep, body, dropCSRF)
			if w.Code != http.StatusForbidden {
				t.Fatalf("got status %d, want 403", w.Code)
			}
			cc := w.Header().Get("Cache-Control")
			// securityHeaders short-circuits before the per-route
			// cacheControl middleware runs (cacheControl is wrapped
			// inside the mux handler, which doesn't run on the 403
			// short-circuit). Either no header is acceptable, or any
			// non-long-cache directive.
			if strings.Contains(cc, "max-age=31536000") || strings.Contains(cc, "immutable") {
				t.Errorf("403 Cache-Control = %q, must not be long-cached", cc)
			}
		})
	}
}

// TestRootCatchAll404CacheControl ensures the catch-all "/" route's 404
// path (path that doesn't resolve to any embedded frontend file) emits
// no-cache rather than something cacheable. This already passes because
// the route is wrapped with cacheControl("no-cache", ...) — provided as
// regression coverage for the middleware-ordering pattern.
func TestRootCatchAll404CacheControl(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/no-such-page.html", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	// "no-cache" is acceptable: it tells the browser to revalidate on
	// every use, so a typo gets corrected on the next reload.
	if strings.Contains(cc, "immutable") || strings.Contains(cc, "max-age=31536000") {
		t.Errorf("/ catch-all 404 Cache-Control = %q, must not be long-cached", cc)
	}
}

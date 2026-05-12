package server

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// assertCacheControlSafeForError fails the test if cc is a value that
// could lead to a browser caching an error response. Acceptable values:
// empty (no header set), "no-store", or "no-cache". Anything else
// risks browsers caching the error and leaving the same URL broken
// until manual cache clear.
//
// Substring-based rejection of specific long-cache directives is
// weaker — a future refactor that sets Cache-Control: public,
// max-age=600 on a 404 would slip past a "no immutable" / "no
// max-age=31536000" filter despite still being a bug. This whitelist
// pins the exact set of safe values.
func assertCacheControlSafeForError(t *testing.T, cc string) {
	t.Helper()
	switch cc {
	case "", "no-store", "no-cache":
		return
	}
	t.Errorf(
		"Cache-Control = %q is not safe for an error response; "+
			"expected empty, no-store, or no-cache so browsers don't "+
			"cache the error and leave the URL broken until manual "+
			"cache clear",
		cc,
	)
}

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
			assertCacheControlSafeForError(t, w.Header().Get("Cache-Control"))
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
			assertCacheControlSafeForError(t, w.Header().Get("Cache-Control"))
		})
	}
}

// TestEmulatorJSDirListing404ClearsAllCacheHeaders pins the positive
// contract for the noDirListing fix: when noDirListing emits a 404 it
// must strip ALL four cache-related headers the outer cacheControl
// middleware could have set — Cache-Control, Etag, Last-Modified, and
// Content-Encoding — mirroring stdlib's serveError (net/http/fs.go).
// The companion TestEmulatorJSDirListing404CacheControl covers only
// Cache-Control; this test catches a future refactor that drops one
// of the other three header clears.
func TestEmulatorJSDirListing404ClearsAllCacheHeaders(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/emulatorjs/data/", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", w.Code)
	}
	for _, h := range []string{
		"Cache-Control",
		"Etag",
		"Last-Modified",
		"Content-Encoding",
	} {
		if got := w.Header().Get(h); got != "" {
			t.Errorf(
				"%s = %q on noDirListing 404, want absent — the upstream "+
					"cacheControl middleware may have set this header before "+
					"noDirListing returned, so noDirListing must clear it to "+
					"avoid leaking the long-cache directives onto the 404",
				h, got,
			)
		}
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
	assertCacheControlSafeForError(t, w.Header().Get("Cache-Control"))
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
	assertCacheControlSafeForError(t, w.Header().Get("Cache-Control"))
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
	assertCacheControlSafeForError(t, w.Header().Get("Cache-Control"))
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
	assertCacheControlSafeForError(t, w.Header().Get("Cache-Control"))
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
	assertCacheControlSafeForError(t, w.Header().Get("Cache-Control"))
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
			// securityHeaders short-circuits before the per-route
			// cacheControl middleware runs (cacheControl is wrapped
			// inside the mux handler, which doesn't run on the 403
			// short-circuit), so the response should carry no
			// long-cache directive.
			assertCacheControlSafeForError(t, w.Header().Get("Cache-Control"))
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
	// The catch-all is wrapped with cacheControl("no-cache", ...), so
	// the 404 carries "no-cache" — acceptable: it tells the browser to
	// revalidate on every use, so a typo gets corrected on the next
	// reload.
	assertCacheControlSafeForError(t, w.Header().Get("Cache-Control"))
}

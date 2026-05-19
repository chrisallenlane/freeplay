package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/datadir"
	"github.com/chrisallenlane/freeplay/internal/igdb"
	"github.com/chrisallenlane/freeplay/internal/scanner"
)

// testFrontendFS and testEmulatorjsFS are shared across all test helpers that
// need embedded filesystem stubs.
var testFrontendFS = fstest.MapFS{
	"frontend/index.html":   &fstest.MapFile{Data: []byte("<html>index</html>")},
	"frontend/play.html":    &fstest.MapFile{Data: []byte("<html>play</html>")},
	"frontend/details.html": &fstest.MapFile{Data: []byte("<html>details</html>")},
}

var testEmulatorjsFS = fstest.MapFS{
	"emulatorjs/data/loader.js": &fstest.MapFile{Data: []byte("loader")},
}

// testServer creates a Server wired with a temp ROM dir, a BIOS file, and an
// optional DetailsCache. It is the single server-construction helper for all
// tests in this package.
func testServer(t *testing.T, dc ...DetailsCache) (*Server, string) {
	t.Helper()
	dir := t.TempDir()

	romDir := filepath.Join(dir, "roms", "NES")
	writeTestFile(t, filepath.Join(romDir, "Mega Man.nes"), []byte("romdata"))

	biosFile := filepath.Join(dir, "bios", "scph1001.bin")
	writeTestFile(t, biosFile, []byte("biosdata"))

	cfg := &config.Config{
		Port: 8080,
		ROMs: map[string]config.ROM{
			"NES": {
				Path:            romDir,
				Core:            "fceumm",
				Bios:            biosFile,
				IGDBPlatformIDs: []int{18},
			},
		},
	}

	var cache DetailsCache
	if len(dc) > 0 {
		cache = dc[0]
	}

	scn := scanner.New(cfg, dir)
	srv, err := New(
		cfg, dir, testFrontendFS, testEmulatorjsFS,
		cache, scn, &stubRescanner{},
		"test",
	)
	if err != nil {
		t.Fatal(err)
	}
	return srv, dir
}

// writeTestFile writes content to path, creating all parent
// directories first. Fails the test on any error.
func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeCacheFile creates a cache/igdb/<console>/<game>/<filename> file
// inside dataDir. Thin wrapper around writeTestFile that encodes the
// IGDB cache directory layout.
func writeCacheFile(
	t *testing.T,
	dataDir, console, game, filename string,
	content []byte,
) {
	t.Helper()
	writeTestFile(
		t,
		filepath.Join(dataDir, "cache", "igdb", console, game, filename),
		content,
	)
}

// doRequest issues method+path against srv.handler and returns the recorder.
// For non-GET/HEAD methods it automatically sets X-Requested-With: freeplay
// unless the caller overrides it via extraHeaders. Optional extraHeaders
// entries are applied after the default; pass an empty http.Header to drop
// X-Requested-With (e.g. for a CSRF test).
func doRequest(
	t *testing.T,
	srv *Server,
	method, path string,
	body io.Reader,
	extraHeaders ...http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set("X-Requested-With", "freeplay")
	}
	for _, h := range extraHeaders {
		// Caller-supplied headers overwrite defaults. A caller that
		// wants to drop a default header sets its value to "" in h.
		for k, vs := range h {
			req.Header.Del(k)
			for _, v := range vs {
				if v != "" {
					req.Header.Add(k, v)
				}
			}
		}
	}
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)
	return w
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/api/health", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestGamesEndpoint(t *testing.T) {
	srv, _ := testServer(t)
	srv.scanner.ScanBlocking()

	w := doRequest(t, srv, http.MethodGet, "/api/games", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	var catalog scanner.Catalog
	if err := json.Unmarshal(w.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(catalog.Games) != 1 {
		t.Errorf("got %d games, want 1", len(catalog.Games))
	}
}

func TestROMServing(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/roms/NES/Mega%20Man.nes", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "romdata" {
		t.Errorf("body = %q, want %q", w.Body.String(), "romdata")
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCache {
		t.Errorf("Cache-Control = %q, want %q", cc, longCache)
	}
}

func TestROMServingUnknownConsole(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/roms/SNES/game.sfc", nil)
	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestBIOSServing(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/bios/NES", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "biosdata" {
		t.Errorf("body = %q, want %q", w.Body.String(), "biosdata")
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCache {
		t.Errorf("Cache-Control = %q, want %q", cc, longCache)
	}
}

func TestBIOSServingNoConfig(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/bios/SNES", nil)
	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestPathTraversalBlocked(t *testing.T) {
	srv, _ := testServer(t)

	tests := []string{
		"/roms/NES/../../../etc/passwd",
		"/roms/NES/..%2F..%2Fetc%2Fpasswd",
	}

	for _, path := range tests {
		w := doRequest(t, srv, http.MethodGet, path, nil)
		if w.Code == 200 {
			t.Errorf("path %q should not return 200", path)
		}
	}
}

func TestServeSecureFileBlocksDirectory(t *testing.T) {
	srv, _ := testServer(t)

	// Create a subdirectory inside ROM dir
	romDir := srv.cfg.ROMs["NES"].Path
	if err := os.MkdirAll(filepath.Join(romDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	w := doRequest(t, srv, http.MethodGet, "/roms/NES/subdir", nil)
	if w.Code != 404 {
		t.Errorf("directory request got status %d, want 404", w.Code)
	}
}

func TestSaveRoundtrip(t *testing.T) {
	srv, _ := testServer(t)
	srv.scanner.ScanBlocking()
	saveData := []byte("my save state data")

	// POST save
	postW := doRequest(
		t, srv, http.MethodPost,
		"/api/saves/NES/Mega%20Man/state", bytes.NewReader(saveData),
	)
	if postW.Code != 200 {
		t.Fatalf("POST save got status %d, want 200", postW.Code)
	}

	// GET save
	getW := doRequest(t, srv, http.MethodGet, "/api/saves/NES/Mega%20Man/state", nil)
	if getW.Code != 200 {
		t.Fatalf("GET save got status %d, want 200", getW.Code)
	}
	if getW.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", getW.Header().Get("Content-Type"))
	}
	if !bytes.Equal(getW.Body.Bytes(), saveData) {
		t.Errorf("save data mismatch: got %q, want %q", getW.Body.String(), string(saveData))
	}
}

// TestPostSaveWithStrippedExtensionMatchesCatalogRom reproduces the production
// save regression: the frontend strips the ROM extension before constructing
// save URLs (stripExt("Mega Man.nes") → "Mega Man"), so it POSTs to
// /api/saves/NES/Mega%20Man/sram — without the ".nes" — but HasGame was keyed
// by the full filename, causing silent 404s on every save.
func TestPostSaveWithStrippedExtensionMatchesCatalogRom(t *testing.T) {
	srv, _ := testServer(t)
	srv.scanner.ScanBlocking()

	w := doRequest(
		t, srv, http.MethodPost,
		"/api/saves/NES/Mega%20Man/sram", bytes.NewReader([]byte("sramdata")),
	)
	if w.Code != http.StatusOK {
		t.Errorf("POST save (no extension) got status %d, want 200", w.Code)
	}
}

// TestGetSaveWithStrippedExtensionMatchesCatalogRom exercises the full
// POST→GET round-trip using the slug URL convention the frontend uses:
// stripExt("Mega Man.nes") → "Mega Man". Both the POST and the GET must
// succeed, and the retrieved body must equal what was posted.
func TestGetSaveWithStrippedExtensionMatchesCatalogRom(t *testing.T) {
	srv, _ := testServer(t)
	srv.scanner.ScanBlocking()
	saveData := []byte("sramdata")

	postW := doRequest(
		t, srv, http.MethodPost,
		"/api/saves/NES/Mega%20Man/sram", bytes.NewReader(saveData),
	)
	if postW.Code != http.StatusOK {
		t.Fatalf("POST save (no extension) got status %d, want 200", postW.Code)
	}

	getW := doRequest(t, srv, http.MethodGet, "/api/saves/NES/Mega%20Man/sram", nil)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET save (no extension) got status %d, want 200", getW.Code)
	}
	if !bytes.Equal(getW.Body.Bytes(), saveData) {
		t.Errorf(
			"GET save body = %q, want %q",
			getW.Body.String(), string(saveData),
		)
	}
}

// TestGetSaveSlugNotInCatalog covers the catalog-gate 404 path: the slug
// is rejected by HasGameSlug before saves.Get is ever consulted. Distinct
// from TestGetSaveKnownGameNoFile, which exercises the disk-miss path.
func TestGetSaveSlugNotInCatalog(t *testing.T) {
	srv, _ := testServer(t)
	srv.scanner.ScanBlocking()

	w := doRequest(t, srv, http.MethodGet, "/api/saves/NES/noexist/state", nil)
	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

// TestGetSaveKnownGameNoFile covers the second 404 path in handleGetSave:
// the slug is in the catalog (HasGameSlug returns true) but no save file
// has been written, so saves.Get returns nil. Without this test the
// nil-data branch is unreachable from any unit test — earlier coverage
// only exercised the catalog-gate 404.
func TestGetSaveKnownGameNoFile(t *testing.T) {
	srv, _ := testServer(t)
	srv.scanner.ScanBlocking()

	// "Mega Man" is the slug of the only ROM in testdata. No save has
	// been written for it; expect 404 from the nil-data branch.
	w := doRequest(t, srv, http.MethodGet, "/api/saves/NES/Mega%20Man/state", nil)
	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

// TestGetSaveUnreadableFileReturns5xx pins the post-fix contract: when
// a save exists on disk but the read fails (permission flip, root-
// owned file from a previous run, transient I/O error, stale mount),
// handleGetSave must return 5xx, never 404. The frontend treats 404
// as "no save yet" and the next auto-save tick would overwrite the
// real (unreadable-but-still-present) save. 5xx breaks that chain by
// signalling "transient" — the frontend's branching logic refuses to
// register the periodic save (see #49).
func TestGetSaveUnreadableFileReturns5xx(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses unix file permissions; skipping")
	}
	srv, dir := testServer(t)
	srv.scanner.ScanBlocking()

	// Write a real save through the public API so it lives at the path
	// the manager actually reads from.
	postW := doRequest(
		t, srv, http.MethodPost,
		"/api/saves/NES/Mega%20Man/sram", bytes.NewReader([]byte("real save")),
	)
	if postW.Code != http.StatusOK {
		t.Fatalf("POST save got status %d, want 200", postW.Code)
	}
	savePath := filepath.Join(dir, "saves", "NES", "Mega Man", "sram")
	if err := os.Chmod(savePath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(savePath, 0o600) })
	if _, err := os.ReadFile(savePath); err == nil {
		t.Skip("filesystem does not enforce read permissions; skipping")
	}

	w := doRequest(t, srv, http.MethodGet, "/api/saves/NES/Mega%20Man/sram", nil)
	if w.Code < 500 || w.Code >= 600 {
		t.Errorf(
			"GET unreadable save returned status %d, want 5xx; the "+
				"frontend distinguishes 404 (no save) from 5xx (transient) "+
				"to decide whether to register the auto-save handler",
			w.Code,
		)
	}
}

func TestSaveInvalidType(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/api/saves/NES/game/badtype", nil)
	if w.Code != 400 {
		t.Errorf("got status %d, want 400", w.Code)
	}
	// Kills mutations that remove Content-Type or the JSON body in
	// writeJSONError — error responses on /api/* must stay JSON-shaped.
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Errorf("error body not valid JSON: %v (body: %q)", err, w.Body.String())
	} else if body["error"] == "" {
		t.Errorf("error body missing 'error' key: %v", body)
	}
}

// TestNewHTTPServerTimeouts pins the production http.Server construction:
// address format, timeouts (slow-drip defenses), and MaxHeaderBytes.
// Kills mutations on any of the constant values in newHTTPServer.
func TestNewHTTPServerTimeouts(t *testing.T) {
	srv, _ := testServer(t)
	hs := srv.newHTTPServer()

	if hs.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", hs.Addr, ":8080")
	}
	if hs.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 10s", hs.ReadHeaderTimeout)
	}
	if hs.ReadTimeout != 60*time.Second {
		t.Errorf("ReadTimeout = %v, want 60s", hs.ReadTimeout)
	}
	if hs.WriteTimeout != 60*time.Second {
		t.Errorf("WriteTimeout = %v, want 60s", hs.WriteTimeout)
	}
	if hs.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %v, want 120s", hs.IdleTimeout)
	}
	if hs.MaxHeaderBytes != 1<<14 {
		t.Errorf("MaxHeaderBytes = %d, want %d", hs.MaxHeaderBytes, 1<<14)
	}
}

// TestCacheControlConstantValues pins the Cache-Control constant
// string literally so off-by-one mutations on max-age and any
// re-introduction of `immutable` are caught.
func TestCacheControlConstantValues(t *testing.T) {
	if longCache != "public, max-age=86400" {
		t.Errorf("longCache = %q", longCache)
	}
	if strings.Contains(longCache, "immutable") {
		t.Errorf("longCache contains `immutable` — banned project-wide")
	}
}

func TestRescanEndpoint(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodPost, "/api/rescan", nil)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestCoversServing(t *testing.T) {
	srv, dir := testServer(t)

	writeTestFile(t, filepath.Join(dir, "covers", "NES", "Mega Man.png"), []byte("pngdata"))

	w := doRequest(t, srv, http.MethodGet, "/covers/NES/Mega%20Man.png", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "pngdata" {
		t.Errorf("body = %q, want %q", w.Body.String(), "pngdata")
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCache {
		t.Errorf("Cache-Control = %q, want %q", cc, longCache)
	}
}

func TestManualsServing(t *testing.T) {
	srv, dir := testServer(t)

	writeTestFile(t, filepath.Join(dir, "manuals", "NES", "Mega Man.pdf"), []byte("pdfdata"))

	w := doRequest(t, srv, http.MethodGet, "/manuals/NES/Mega%20Man.pdf", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "pdfdata" {
		t.Errorf("body = %q, want %q", w.Body.String(), "pdfdata")
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCache {
		t.Errorf("Cache-Control = %q, want %q", cc, longCache)
	}
}

func TestManualsNotFound(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/manuals/NES/noexist.pdf", nil)
	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestDetailsPage(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/details?console=NES&rom=Mega+Man.nes", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	// Body must contain HTML from details.html. Kills a mutation that
	// removes the http.ServeFileFS call in servePage().
	if !strings.Contains(w.Body.String(), "<html") &&
		!strings.Contains(w.Body.String(), "<!DOCTYPE") {
		t.Errorf("/details body not HTML (first 80 bytes: %q)",
			w.Body.String()[:min(80, w.Body.Len())])
	}
}

func TestGameDetailsNoCache(t *testing.T) {
	srv, _ := testServer(t) // detailsCache is nil

	w := doRequest(t, srv, http.MethodGet, "/api/game-details?console=NES&rom=Mega+Man.nes", nil)
	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestGameDetailsMissingParams(t *testing.T) {
	srv, _ := testServer(t, &mockDetailsCache{})

	w := doRequest(t, srv, http.MethodGet, "/api/game-details", nil)
	if w.Code != 400 {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestGameDetailsPathTraversalBlocked(t *testing.T) {
	srv, _ := testServer(t, &mockDetailsCache{})

	// PoC from SEC-3 ticket: ../../tmp/evil/secret/details.json
	tests := []string{
		"/api/game-details?console=..&rom=x.nes",
		"/api/game-details?console=NES&rom=..",
		"/api/game-details?console=../../../tmp/evil&rom=secret.nes",
		"/api/game-details?console=NES&rom=../../secret.nes",
		"/api/game-details?console=NES%2F..&rom=x.nes",
		"/api/game-details?console=NES&rom=x%00evil.nes",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			w := doRequest(t, srv, http.MethodGet, path, nil)
			if w.Code != http.StatusBadRequest {
				t.Errorf("got status %d, want 400", w.Code)
			}
		})
	}
}

func TestGameDetailsCacheMiss404(t *testing.T) {
	srv, _ := testServer(t, &mockDetailsCache{}) // cache returns nil for all Gets

	w := doRequest(t, srv, http.MethodGet, "/api/game-details?console=NES&rom=Unknown.nes", nil)
	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestCoversNotFound(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/covers/NES/noexist.png", nil)
	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestEmulatorJSDirectoryListingBlocked(t *testing.T) {
	srv, _ := testServer(t)

	// Pre-fix: http.FileServerFS rendered a clickable HTML listing of
	// emulatorjs/data/ subdirectories, leaking version + bundled cores.
	// Post-fix: no index.html in that dir → 404.
	w := doRequest(t, srv, http.MethodGet, "/emulatorjs/data/", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("dir listing: got status %d, want 404", w.Code)
	}
	// Body must be exactly one NotFound line. Missing `return` after
	// http.NotFound would let next.ServeHTTP append another 404 body.
	if got := w.Body.String(); got != "404 page not found\n" {
		t.Errorf("dir listing body = %q, want single %q (return missing?)",
			got, "404 page not found\n")
	}

	// Files inside the directory are still served — body must be non-empty.
	// If next.ServeHTTP were removed from noDirListing, the fall-through
	// path would yield status 200 with an empty body.
	w = doRequest(t, srv, http.MethodGet, "/emulatorjs/data/loader.js", nil)
	if w.Code != http.StatusOK {
		t.Errorf("file inside dir: got status %d, want 200", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Errorf("file inside dir: body empty (next.ServeHTTP missing?)")
	}

	// Root "/" still serves index.html (noDirListing allows directories
	// that contain an index.html).
	w = doRequest(t, srv, http.MethodGet, "/", nil)
	if w.Code != http.StatusOK {
		t.Errorf("/ root: got status %d, want 200", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Errorf("/ root: body empty (next.ServeHTTP missing?)")
	}
}

func TestEmulatorJSCacheHeaders(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/emulatorjs/data/loader.js", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCache {
		t.Errorf("Cache-Control = %q, want %q", cc, longCache)
	}
}

func TestPlayPage(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/play", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<html") &&
		!strings.Contains(w.Body.String(), "<!DOCTYPE") {
		t.Errorf("/play body not HTML (first 80 bytes: %q)",
			w.Body.String()[:min(80, w.Body.Len())])
	}
}

func TestSavePathTraversalBlocked(t *testing.T) {
	srv, dir := testServer(t)

	// Create a sentinel file outside the saves directory
	sentinel := filepath.Join(dir, "secret")
	writeTestFile(t, sentinel, []byte("sensitive"))

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"GET dotdot console", "GET", "/api/saves/%2e%2e/secret/state"},
		{"GET dotdot game", "GET", "/api/saves/NES/%2e%2e%2f%2e%2e%2fsecret/state"},
		{"POST dotdot console", "POST", "/api/saves/%2e%2e/secret/state"},
		{"POST dotdot game", "POST", "/api/saves/NES/%2e%2e%2f%2e%2e%2fsecret/state"},
		{"GET backslash", "GET", "/api/saves/NES/game%5c..%5c../state"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.method == "POST" {
				body = bytes.NewReader([]byte("payload"))
			}
			w := doRequest(t, srv, tt.method, tt.path, body)
			if w.Code == 200 {
				t.Errorf("path traversal attempt should not return 200, got %d", w.Code)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/api/health", nil)

	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "same-origin",
	}
	for name, want := range headers {
		got := w.Header().Get(name)
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func FuzzServeSecureFile(f *testing.F) {
	f.Add("NES/Mega Man/cover.jpg")
	f.Add("../../../etc/passwd")
	f.Add("NES/../../../etc/passwd")
	f.Add("")
	f.Add("NES/\x00evil")

	f.Fuzz(func(t *testing.T, filePath string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf(
					"serveSecureFile panicked on path %q: %v",
					filePath, r,
				)
			}
		}()

		srv, dir := testServer(t)

		// Place a known file inside the cache/igdb subtree so a valid path
		// can return 200.
		const knownContent = "jpgdata"
		writeCacheFile(t, dir, "NES", "Mega Man", "cover.jpg", []byte(knownContent))

		// Construct the URL. The mux pattern GET /cache/igdb/{rest...}
		// passes the raw path remainder to serveSecureFile, so we just
		// URL-encode the fuzz input and request it.
		encoded := (&url.URL{Path: filePath}).RequestURI()
		req := httptest.NewRequest("GET", "/cache/igdb/"+encoded, nil)
		w := httptest.NewRecorder()
		srv.handler.ServeHTTP(w, req)

		code := w.Code

		// The server must never return a 5xx error. Traversal attempts
		// and other bad input may produce 404 (blocked) or a 3xx
		// redirect (mux path cleaning), but never a server error.
		if code >= 500 {
			t.Errorf(
				"path %q: server error status %d (want non-5xx)",
				filePath, code,
			)
		}

		// When 200, the served content must match the known file.
		if code == http.StatusOK {
			got := w.Body.String()
			if !strings.Contains(got, knownContent) {
				t.Errorf(
					"path %q: 200 response body %q does not contain known content",
					filePath, got,
				)
			}
		}
	})
}

func FuzzSafePathSegment(f *testing.F) {
	f.Add("game1")
	f.Add("")
	f.Add("..")
	f.Add("../etc/passwd")
	f.Add("game\\..\\secret")
	f.Add("name\x00evil")

	f.Fuzz(func(t *testing.T, input string) {
		result := datadir.SafePathSegment(input)
		if result {
			// If SafePathSegment says it's safe, verify the invariants hold.
			if input == "" || input == "." || input == ".." {
				t.Errorf("SafePathSegment returned true for reserved name: %q", input)
			}
			if strings.Contains(input, "..") {
				t.Errorf("SafePathSegment returned true for input containing '..': %q", input)
			}
			if strings.Contains(input, "/") {
				t.Errorf("SafePathSegment returned true for input containing '/': %q", input)
			}
			if strings.Contains(input, "\\") {
				t.Errorf("SafePathSegment returned true for input containing '\\': %q", input)
			}
			if strings.ContainsRune(input, 0) {
				t.Errorf("SafePathSegment returned true for input containing null byte: %q", input)
			}
		}
	})
}

func FuzzParseSaveParams(f *testing.F) {
	f.Add("NES", "game1", "state")
	f.Add("NES", "game1", "sram")
	f.Add("", "", "")
	f.Add("..", "game", "state")
	f.Add("NES", "../secret", "state")
	f.Add("NES", "game", "badtype")

	f.Fuzz(func(t *testing.T, console, game, saveType string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf(
					"parseSaveParams panicked on (%q, %q, %q): %v",
					console, game, saveType, r,
				)
			}
		}()

		// Build a base request on a fixed URL and inject fuzzed values via
		// SetPathValue so that arbitrary strings (including control characters)
		// don't cause httptest.NewRequest to panic on URL parsing.
		req := httptest.NewRequest("GET", "/api/saves/x/x/x", nil)
		req.SetPathValue("console", console)
		req.SetPathValue("game", game)
		req.SetPathValue("type", saveType)

		gotConsole, gotGame, gotSaveType, ok := parseSaveParams(req)

		if ok {
			// When parseSaveParams reports success, the invariants must hold.
			if !datadir.SafePathSegment(gotConsole) {
				t.Errorf(
					"ok=true but gotConsole %q fails SafePathSegment",
					gotConsole,
				)
			}
			if !datadir.SafePathSegment(gotGame) {
				t.Errorf(
					"ok=true but gotGame %q fails SafePathSegment",
					gotGame,
				)
			}
			if gotSaveType != "state" && gotSaveType != "sram" {
				t.Errorf(
					"ok=true but gotSaveType %q is not \"state\" or \"sram\"",
					gotSaveType,
				)
			}
		}
	})
}

// stubRescanner lets tests exercise /api/rescan without the real pipeline.
type stubRescanner struct{ busy bool }

func (s *stubRescanner) TriggerRescan() bool { return !s.busy }

func TestRescanConflict(t *testing.T) {
	srv, _ := testServer(t)
	srv.rescanner = &stubRescanner{busy: true}

	w := doRequest(t, srv, http.MethodPost, "/api/rescan", nil)

	if w.Code != http.StatusConflict {
		t.Errorf("got status %d, want 409", w.Code)
	}
}

func TestRescanUnavailableWithoutRescanner(t *testing.T) {
	srv, _ := testServer(t)
	srv.rescanner = nil

	w := doRequest(t, srv, http.MethodPost, "/api/rescan", nil)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want 503", w.Code)
	}
}

func TestRescanSucceeds(t *testing.T) {
	srv, _ := testServer(t)
	srv.rescanner = &stubRescanner{busy: false}

	w := doRequest(t, srv, http.MethodPost, "/api/rescan", nil)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestPostWithoutCSRFHeaderRejected(t *testing.T) {
	srv, _ := testServer(t)

	endpoints := []string{
		// Slug shape — the URL the frontend actually constructs.
		// CSRF middleware must reject before reaching parseSaveParams,
		// so the slug doesn't need to be in the catalog.
		"/api/saves/NES/Mega%20Man/state",
		"/api/rescan",
	}
	dropCSRF := http.Header{"X-Requested-With": {""}}
	for _, ep := range endpoints {
		w := doRequest(t, srv, http.MethodPost, ep, nil, dropCSRF)
		if w.Code != http.StatusForbidden {
			t.Errorf("POST %s without X-Requested-With: got %d, want 403", ep, w.Code)
		}
		// Body must be exactly "forbidden\n". Missing `return` after
		// http.Error would let next.ServeHTTP append additional output.
		if got := w.Body.String(); got != "forbidden\n" {
			t.Errorf("POST %s body = %q, want %q (return missing after http.Error?)",
				ep, got, "forbidden\n")
		}
	}
}

// mockDetailsCache is a test double for the DetailsCache interface.
type mockDetailsCache struct {
	fetching bool
	details  map[string]*igdb.GameDetails // key: "console/rom"
}

func (m *mockDetailsCache) Fetching() bool { return m.fetching }

func (m *mockDetailsCache) Get(console, rom string) *igdb.GameDetails {
	if m.details == nil {
		return nil
	}
	return m.details[console+"/"+rom]
}

func TestStatusEndpointNilCover(t *testing.T) {
	srv, _ := testServer(t) // detailsCache is nil

	w := doRequest(t, srv, http.MethodGet, "/api/status", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["fetchingDetails"] != false {
		t.Error("expected fetchingDetails=false with nil detailsCache")
	}
}

func TestStatusEndpointFetching(t *testing.T) {
	srv, _ := testServer(t, &mockDetailsCache{fetching: true})

	w := doRequest(t, srv, http.MethodGet, "/api/status", nil)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["fetchingDetails"] != true {
		t.Error("expected fetchingDetails=true when cache is active")
	}
}

func TestGameDetailsFromCache(t *testing.T) {
	cached := &igdb.GameDetails{
		Name:    "Mega Man",
		Summary: "Cached summary.",
	}
	cache := &mockDetailsCache{
		details: map[string]*igdb.GameDetails{
			"NES/Mega Man.nes": cached,
		},
	}
	srv, _ := testServer(t, cache)

	w := doRequest(
		t, srv, http.MethodGet,
		"/api/game-details?console=NES&rom=Mega+Man.nes", nil,
	)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	var got igdb.GameDetails
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Name != "Mega Man" {
		t.Errorf("Name = %q, want %q", got.Name, "Mega Man")
	}
	if got.Summary != "Cached summary." {
		t.Errorf("Summary = %q, want %q", got.Summary, "Cached summary.")
	}
}

func TestCacheFilesServing(t *testing.T) {
	srv, dir := testServer(t)

	writeCacheFile(t, dir, "NES", "Mega Man", "cover.jpg", []byte("jpgdata"))

	w := doRequest(
		t, srv, http.MethodGet,
		"/cache/igdb/NES/Mega%20Man/cover.jpg", nil,
	)
	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "jpgdata" {
		t.Errorf("body = %q, want %q", w.Body.String(), "jpgdata")
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCache {
		t.Errorf("Cache-Control = %q, want %q", cc, longCache)
	}
}

func TestCacheFilesNotFound(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(
		t, srv, http.MethodGet,
		"/cache/igdb/NES/Mega%20Man/cover.jpg", nil,
	)
	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestCSRFHeaderValidation(t *testing.T) {
	srv, _ := testServer(t)

	tests := []struct {
		name       string
		headerVal  string // "" means do not set the header at all
		setHeader  bool
		wantStatus int
	}{
		{
			name:       "wrong value XMLHttpRequest",
			setHeader:  true,
			headerVal:  "XMLHttpRequest",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "empty header value",
			setHeader:  true,
			headerVal:  "",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "correct value freeplay",
			setHeader:  true,
			headerVal:  "freeplay",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var headers []http.Header
			if !tt.setHeader {
				headers = append(headers, http.Header{"X-Requested-With": {""}})
			} else {
				headers = append(headers, http.Header{"X-Requested-With": {tt.headerVal}})
			}
			w := doRequest(t, srv, http.MethodPost, "/api/rescan", nil, headers...)
			if w.Code != tt.wantStatus {
				t.Errorf(
					"POST /api/rescan with X-Requested-With=%q: got %d, want %d",
					tt.headerVal, w.Code, tt.wantStatus,
				)
			}
		})
	}
}

func TestPostSaveTooLargeReturns413(t *testing.T) {
	srv, _ := testServer(t)
	srv.scanner.ScanBlocking()

	// 65 MiB — one byte over the 64 MiB MaxBytesReader cap
	payload := bytes.Repeat([]byte{'a'}, 65<<20)
	w := doRequest(
		t, srv, http.MethodPost,
		"/api/saves/NES/Mega%20Man/state", bytes.NewReader(payload),
	)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("got status %d, want 413", w.Code)
	}
}

func TestPostSaveUnknownGameReturns404(t *testing.T) {
	srv, dir := testServer(t)
	srv.scanner.ScanBlocking()

	// Use a slug shape (no extension), since that's what the frontend
	// produces. With "FakeGame.nes" the test would also pass, but for
	// the wrong reason — stripExt("FakeGame.nes") = "FakeGame" is not
	// in the catalog either, so we couldn't tell whether the gate
	// rejected because the game is unknown or because the URL shape
	// is wrong.
	w := doRequest(
		t, srv, http.MethodPost,
		"/api/saves/NES/FakeGame/state", bytes.NewReader([]byte("data")),
	)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}

	path := filepath.Join(dir, "saves", "NES", "FakeGame")
	if _, err := os.Stat(path); err == nil {
		t.Errorf("unexpected save directory created for unknown game: %s", path)
	}
}

func TestGetSaveUnknownGameReturns404(t *testing.T) {
	srv, _ := testServer(t)
	srv.scanner.ScanBlocking()

	w := doRequest(
		t, srv, http.MethodGet,
		"/api/saves/NES/FakeGame/state", nil,
	)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestPostSaveInvalidType(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(
		t, srv, http.MethodPost,
		"/api/saves/NES/game1/badtype", strings.NewReader("data"),
	)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestNoCacheMiddleware(t *testing.T) {
	srv, _ := testServer(t)

	tests := []struct {
		name string
		path string
	}{
		{"index", "/"},
		{"play page", "/play?console=NES&rom=test.nes"},
		{"details page", "/details?console=NES&rom=test.nes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(t, srv, http.MethodGet, tt.path, nil)
			cc := w.Header().Get("Cache-Control")
			if cc != "no-cache" {
				t.Errorf(
					"GET %s: Cache-Control = %q, want %q",
					tt.path, cc, "no-cache",
				)
			}
		})
	}
}

func TestPostSavePutError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses filesystem mode bits; test requires non-root")
	}
	srv, dir := testServer(t)
	srv.scanner.ScanBlocking()

	savesDir := filepath.Join(dir, "saves")
	if err := os.MkdirAll(savesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(savesDir, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(savesDir, 0o755) })

	w := doRequest(
		t, srv, http.MethodPost,
		"/api/saves/NES/Mega%20Man/state", bytes.NewReader([]byte("data")),
	)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

// TestPostSavePutErrorLogsWarn asserts that a save-pipeline failure (500)
// emits exactly one slog.Warn record with the expected structured fields.
func TestPostSavePutErrorLogsWarn(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses filesystem mode bits; test requires non-root")
	}
	h := installCapturingLogger(t)

	srv, dir := testServer(t)
	srv.scanner.ScanBlocking()

	savesDir := filepath.Join(dir, "saves")
	if err := os.MkdirAll(savesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(savesDir, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(savesDir, 0o755) })

	doRequest(
		t, srv, http.MethodPost,
		"/api/saves/NES/Mega%20Man/state", bytes.NewReader([]byte("data")),
	)

	h.mu.Lock()
	recs := h.records
	h.mu.Unlock()

	var warnRecs []slog.Record
	for _, r := range recs {
		if r.Level == slog.LevelWarn {
			warnRecs = append(warnRecs, r)
		}
	}
	if len(warnRecs) != 1 {
		t.Fatalf("got %d Warn records, want 1", len(warnRecs))
	}
	rec := warnRecs[0]
	if rec.Message != "save write failed" {
		t.Errorf("message = %q, want %q", rec.Message, "save write failed")
	}
	if _, ok := withAttr(rec, "error"); !ok {
		t.Error("error attribute missing from warn record")
	}
}

// TestRescanUnavailableLogsWarn asserts that a nil rescanner (503)
// emits exactly one slog.Warn record.
func TestRescanUnavailableLogsWarn(t *testing.T) {
	h := installCapturingLogger(t)

	srv, _ := testServer(t)
	srv.rescanner = nil

	doRequest(t, srv, http.MethodPost, "/api/rescan", nil)

	h.mu.Lock()
	recs := h.records
	h.mu.Unlock()

	var warnRecs []slog.Record
	for _, r := range recs {
		if r.Level == slog.LevelWarn {
			warnRecs = append(warnRecs, r)
		}
	}
	if len(warnRecs) != 1 {
		t.Fatalf("got %d Warn records, want 1", len(warnRecs))
	}
	rec := warnRecs[0]
	if rec.Message != "rescan failed" {
		t.Errorf("message = %q, want %q", rec.Message, "rescan failed")
	}
}

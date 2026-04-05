package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/chrisallenlane/freeplay/internal/config"
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

// newTestServer creates a Server wired with a temp ROM dir, a BIOS file, and
// an optional DetailsCache. It is the single server-construction helper for
// all tests in this package.
func newTestServer(t *testing.T, dc DetailsCache) (*Server, string) {
	t.Helper()
	dir := t.TempDir()

	romDir := filepath.Join(dir, "roms", "NES")
	if err := os.MkdirAll(romDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(romDir, "Mega Man.nes"), []byte("romdata"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	biosDir := filepath.Join(dir, "bios")
	if err := os.MkdirAll(biosDir, 0o755); err != nil {
		t.Fatal(err)
	}
	biosFile := filepath.Join(biosDir, "scph1001.bin")
	if err := os.WriteFile(biosFile, []byte("biosdata"), 0o644); err != nil {
		t.Fatal(err)
	}

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

	srv, err := New(cfg, dir, testFrontendFS, testEmulatorjsFS, dc)
	if err != nil {
		t.Fatal(err)
	}
	return srv, dir
}

// testServer is a convenience wrapper around newTestServer for tests that do
// not need a DetailsCache.
func testServer(t *testing.T, dc ...DetailsCache) (*Server, string) {
	t.Helper()
	var cache DetailsCache
	if len(dc) > 0 {
		cache = dc[0]
	}
	return newTestServer(t, cache)
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

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

	req := httptest.NewRequest("GET", "/api/games", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

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

	req := httptest.NewRequest("GET", "/roms/NES/Mega%20Man.nes", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "romdata" {
		t.Errorf("body = %q, want %q", w.Body.String(), "romdata")
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCacheValue {
		t.Errorf("Cache-Control = %q, want %q", cc, longCacheValue)
	}
}

func TestROMServingUnknownConsole(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/roms/SNES/game.sfc", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestBIOSServing(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/bios/NES", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "biosdata" {
		t.Errorf("body = %q, want %q", w.Body.String(), "biosdata")
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCacheValue {
		t.Errorf("Cache-Control = %q, want %q", cc, longCacheValue)
	}
}

func TestBIOSServingNoConfig(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/bios/SNES", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

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
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		srv.handler.ServeHTTP(w, req)

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

	req := httptest.NewRequest("GET", "/roms/NES/subdir", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("directory request got status %d, want 404", w.Code)
	}
}

func TestSaveRoundtrip(t *testing.T) {
	srv, _ := testServer(t)
	saveData := []byte("my save state data")

	// POST save
	postReq := httptest.NewRequest("POST", "/api/saves/NES/game1/state", bytes.NewReader(saveData))
	postReq.Header.Set("X-Requested-With", "freeplay")
	postW := httptest.NewRecorder()
	srv.handler.ServeHTTP(postW, postReq)

	if postW.Code != 200 {
		t.Fatalf("POST save got status %d, want 200", postW.Code)
	}

	// GET save
	getReq := httptest.NewRequest("GET", "/api/saves/NES/game1/state", nil)
	getW := httptest.NewRecorder()
	srv.handler.ServeHTTP(getW, getReq)

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

func TestGetSaveNotFound(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/saves/NES/noexist/state", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestSaveInvalidType(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/saves/NES/game/badtype", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestRescanEndpoint(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/api/rescan", nil)
	req.Header.Set("X-Requested-With", "freeplay")
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

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

	coverDir := filepath.Join(dir, "covers", "NES")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coverDir, "Mega Man.png"), []byte("pngdata"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/covers/NES/Mega%20Man.png", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "pngdata" {
		t.Errorf("body = %q, want %q", w.Body.String(), "pngdata")
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCacheValue {
		t.Errorf("Cache-Control = %q, want %q", cc, longCacheValue)
	}
}

func TestManualsServing(t *testing.T) {
	srv, dir := testServer(t)

	manualDir := filepath.Join(dir, "manuals", "NES")
	if err := os.MkdirAll(manualDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manualDir, "Mega Man.pdf"), []byte("pdfdata"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/manuals/NES/Mega%20Man.pdf", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "pdfdata" {
		t.Errorf("body = %q, want %q", w.Body.String(), "pdfdata")
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCacheValue {
		t.Errorf("Cache-Control = %q, want %q", cc, longCacheValue)
	}
}

func TestManualsNotFound(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/manuals/NES/noexist.pdf", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestDetailsPage(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/details?console=NES&rom=Mega+Man.nes", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
}

func TestGameDetailsNoCache(t *testing.T) {
	srv, _ := testServer(t) // detailsCache is nil

	req := httptest.NewRequest("GET", "/api/game-details?console=NES&rom=Mega+Man.nes", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestGameDetailsMissingParams(t *testing.T) {
	srv, _ := testServer(t, &mockDetailsCache{})

	req := httptest.NewRequest("GET", "/api/game-details", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestGameDetailsCacheMiss404(t *testing.T) {
	srv, _ := testServer(t, &mockDetailsCache{}) // cache returns nil for all Gets

	req := httptest.NewRequest("GET", "/api/game-details?console=NES&rom=Unknown.nes", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestCoversNotFound(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/covers/NES/noexist.png", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestEmulatorJSCacheHeaders(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/emulatorjs/data/loader.js", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCacheValue {
		t.Errorf("Cache-Control = %q, want %q", cc, longCacheValue)
	}
}

func TestPlayPage(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/play", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
}

func TestSavePathTraversalBlocked(t *testing.T) {
	srv, dir := testServer(t)

	// Create a sentinel file outside the saves directory
	sentinel := filepath.Join(dir, "secret")
	if err := os.WriteFile(sentinel, []byte("sensitive"), 0o644); err != nil {
		t.Fatal(err)
	}

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
			var req *http.Request
			if tt.method == "POST" {
				req = httptest.NewRequest(tt.method, tt.path, bytes.NewReader([]byte("payload")))
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			w := httptest.NewRecorder()
			srv.handler.ServeHTTP(w, req)

			if w.Code == 200 {
				t.Errorf("path traversal attempt should not return 200, got %d", w.Code)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

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
		cacheDir := filepath.Join(dir, "cache", "igdb", "NES", "Mega Man")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		const knownContent = "jpgdata"
		if err := os.WriteFile(
			filepath.Join(cacheDir, "cover.jpg"),
			[]byte(knownContent), 0o644,
		); err != nil {
			t.Fatal(err)
		}

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

func FuzzSafeName(f *testing.F) {
	f.Add("game1")
	f.Add("")
	f.Add("..")
	f.Add("../etc/passwd")
	f.Add("game\\..\\secret")
	f.Add("name\x00evil")

	f.Fuzz(func(t *testing.T, input string) {
		result := safeName(input)
		if result {
			// If safeName says it's safe, verify the invariants hold
			if input == "" {
				t.Error("safeName returned true for empty string")
			}
			if strings.Contains(input, "..") {
				t.Errorf("safeName returned true for input containing '..': %q", input)
			}
			if strings.Contains(input, "/") {
				t.Errorf("safeName returned true for input containing '/': %q", input)
			}
			if strings.Contains(input, "\\") {
				t.Errorf("safeName returned true for input containing '\\': %q", input)
			}
			if strings.ContainsRune(input, 0) {
				t.Errorf("safeName returned true for input containing null byte: %q", input)
			}
		}
	})
}

func TestRescanConflict(t *testing.T) {
	srv, _ := testServer(t)

	started := make(chan struct{})
	release := make(chan struct{})
	srv.scanner.SetOnScanComplete(func(_ []scanner.Game) {
		close(started)
		<-release
	})

	go srv.scanner.ScanBlocking()
	<-started

	req := httptest.NewRequest("POST", "/api/rescan", nil)
	req.Header.Set("X-Requested-With", "freeplay")
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	close(release)

	if w.Code != http.StatusConflict {
		t.Errorf("got status %d, want 409", w.Code)
	}
}

func TestPostWithoutCSRFHeaderRejected(t *testing.T) {
	srv, _ := testServer(t)

	endpoints := []string{
		"/api/saves/NES/game1/state",
		"/api/rescan",
	}
	for _, ep := range endpoints {
		req := httptest.NewRequest("POST", ep, nil)
		w := httptest.NewRecorder()
		srv.handler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("POST %s without X-Requested-With: got %d, want 403", ep, w.Code)
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

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

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
	if body["igdbConfigured"] != false {
		t.Error("expected igdbConfigured=false with nil detailsCache")
	}
}

func TestStatusEndpointFetching(t *testing.T) {
	srv, _ := testServer(t, &mockDetailsCache{fetching: true})

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

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

func TestStatusEndpointIGDBConfigured(t *testing.T) {
	srv, _ := testServer(t, &mockDetailsCache{})

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["igdbConfigured"] != true {
		t.Error("expected igdbConfigured=true when detailsCache is set")
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

	req := httptest.NewRequest(
		"GET", "/api/game-details?console=NES&rom=Mega+Man.nes", nil,
	)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

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

	cacheDir := filepath.Join(dir, "cache", "igdb", "NES", "Mega Man")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(cacheDir, "cover.jpg"), []byte("jpgdata"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(
		"GET", "/cache/igdb/NES/Mega%20Man/cover.jpg", nil,
	)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "jpgdata" {
		t.Errorf("body = %q, want %q", w.Body.String(), "jpgdata")
	}
	if cc := w.Header().Get("Cache-Control"); cc != longCacheValue {
		t.Errorf("Cache-Control = %q, want %q", cc, longCacheValue)
	}
}

func TestCacheFilesNotFound(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest(
		"GET", "/cache/igdb/NES/Mega%20Man/cover.jpg", nil,
	)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestPostSavePutError(t *testing.T) {
	srv, dir := testServer(t)

	savesDir := filepath.Join(dir, "saves")
	if err := os.MkdirAll(savesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(savesDir, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(savesDir, 0o755) })

	req := httptest.NewRequest(
		"POST",
		"/api/saves/NES/game1/state",
		bytes.NewReader([]byte("data")),
	)
	req.Header.Set("X-Requested-With", "freeplay")
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

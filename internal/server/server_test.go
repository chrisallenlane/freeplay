package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/scanner"
)

func testServer(t *testing.T, coverStatus ...CoverStatus) (*Server, string) {
	t.Helper()
	dir := t.TempDir()

	romDir := filepath.Join(dir, "roms", "NES")
	if err := os.MkdirAll(romDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(romDir, "Mega Man.nes"), []byte("romdata"), 0o644); err != nil {
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
			"NES": {Path: romDir, Core: "fceumm", Bios: biosFile},
		},
	}

	frontendFS := fstest.MapFS{
		"frontend/index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
		"frontend/play.html":  &fstest.MapFile{Data: []byte("<html>play</html>")},
	}
	emulatorjsFS := fstest.MapFS{
		"emulatorjs/data/loader.js": &fstest.MapFile{Data: []byte("loader")},
	}

	var cs CoverStatus
	if len(coverStatus) > 0 {
		cs = coverStatus[0]
	}

	srv, err := New(cfg, dir, frontendFS, emulatorjsFS, cs)
	if err != nil {
		t.Fatal(err)
	}
	return srv, dir
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
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	close(release)

	if w.Code != http.StatusConflict {
		t.Errorf("got status %d, want 409", w.Code)
	}
}

type mockCoverStatus struct {
	fetching bool
}

func (m *mockCoverStatus) Fetching() bool {
	return m.fetching
}

func TestStatusEndpointNilCover(t *testing.T) {
	srv, _ := testServer(t) // coverStatus is nil

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	var body map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["fetchingCovers"] {
		t.Error("expected fetchingCovers=false with nil coverStatus")
	}
}

func TestStatusEndpointFetching(t *testing.T) {
	srv, _ := testServer(t, &mockCoverStatus{fetching: true})

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	var body map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !body["fetchingCovers"] {
		t.Error("expected fetchingCovers=true when fetcher is active")
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
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

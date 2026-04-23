package server

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/igdb"
)

// TestCacheControlAPIRoutes ensures that /api/* endpoints set
// Cache-Control explicitly rather than leaving browsers free to
// heuristic-cache. Pollable endpoints (/api/status, /api/games) get
// no-store; /api/game-details gets a short private max-age.
func TestCacheControlAPIRoutes(t *testing.T) {
	cached := &igdb.GameDetails{Name: "Mega Man", Summary: "Cached."}
	cache := &mockDetailsCache{
		details: map[string]*igdb.GameDetails{"NES/Mega Man.nes": cached},
	}
	srv, _ := testServer(t, cache)
	srv.scanner.ScanBlocking()

	// Seed a save so GET /api/saves returns 200 rather than 404.
	postW := doRequest(t, srv, http.MethodPost,
		"/api/saves/NES/Mega%20Man.nes/sram", bytes.NewReader([]byte("save")))
	if postW.Code != http.StatusOK {
		t.Fatalf("seed POST status = %d, want 200", postW.Code)
	}
	if got := postW.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("POST /api/saves Cache-Control = %q, want no-store", got)
	}

	noStoreRoutes := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/health"},
		{http.MethodGet, "/api/games"},
		{http.MethodGet, "/api/status"},
		{http.MethodGet, "/api/saves/NES/Mega%20Man.nes/sram"},
	}
	for _, r := range noStoreRoutes {
		w := doRequest(t, srv, r.method, r.path, nil)
		if w.Code != http.StatusOK {
			t.Errorf("%s %s: status = %d, want 200", r.method, r.path, w.Code)
			continue
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s %s: Cache-Control = %q, want no-store", r.method, r.path, got)
		}
	}

	// /api/game-details takes a short private max-age.
	w := doRequest(t, srv, http.MethodGet,
		"/api/game-details?console=NES&rom=Mega+Man.nes", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("/api/game-details status = %d, want 200", w.Code)
	}
	got := w.Header().Get("Cache-Control")
	if !strings.Contains(got, "private") || !strings.Contains(got, "max-age=300") {
		t.Errorf("/api/game-details Cache-Control = %q, want private + max-age=300", got)
	}
}

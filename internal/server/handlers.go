package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/chrisallenlane/freeplay/internal/details"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSONOK(w)
}

func (s *Server) handleGames(w http.ResponseWriter, _ *http.Request) {
	data, err := s.scanner.CatalogJSON()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *Server) handleROM(w http.ResponseWriter, r *http.Request) {
	rom, ok := s.cfg.ROMs[r.PathValue("console")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveSecureFile(w, r, rom.Path, r.PathValue("file"), longCacheImmutable)
}

func (s *Server) handleBIOS(w http.ResponseWriter, r *http.Request) {
	rom, ok := s.cfg.ROMs[r.PathValue("console")]
	if !ok || rom.Bios == "" {
		http.NotFound(w, r)
		return
	}
	serveFile(w, r, rom.Bios, longCacheImmutable)
}

func (s *Server) handleCovers(w http.ResponseWriter, r *http.Request) {
	// Covers are rewritten after IGDB rescans behind a stable URL.
	// Drop `immutable` so browsers revalidate on cache expiry (PERF-10).
	s.serveSecureFile(w, r, filepath.Join(s.dataDir, "covers"), r.PathValue("rest"), longCacheMutable)
}

func (s *Server) handleCacheFiles(w http.ResponseWriter, r *http.Request) {
	s.serveSecureFile(w, r, details.CacheDir(s.dataDir), r.PathValue("rest"), longCacheMutable)
}

func (s *Server) handleManuals(w http.ResponseWriter, r *http.Request) {
	s.serveSecureFile(w, r, filepath.Join(s.dataDir, "manuals"), r.PathValue("rest"), longCacheMutable)
}

func (s *Server) handleGameDetails(w http.ResponseWriter, r *http.Request) {
	if s.detailsCache == nil {
		http.Error(w, `{"error":"IGDB not configured"}`, http.StatusNotFound)
		return
	}

	console := r.URL.Query().Get("console")
	rom := r.URL.Query().Get("rom")
	if !safeName(console) || !safeName(rom) {
		// safeName rejects empty, "..", "/", "\\", and NUL. Blocks the
		// SEC-3 path-traversal PoC (console=../../../../tmp/evil) at
		// the HTTP boundary; defense-in-depth lives in Cache.Get.
		http.Error(
			w,
			`{"error":"invalid console or rom parameter"}`,
			http.StatusBadRequest,
		)
		return
	}

	d := s.detailsCache.Get(console, rom)
	if d == nil {
		http.Error(w, `{"error":"game not found"}`, http.StatusNotFound)
		return
	}

	// Private cache keyed on the (console, rom) query; 5 minutes is
	// long enough to dedupe in-session navigation, short enough that
	// a rescan's refreshed details.json gets picked up quickly.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=300")
	_ = json.NewEncoder(w).Encode(d)
}

func (s *Server) handleGetSave(w http.ResponseWriter, r *http.Request) {
	console, game, saveType, ok := parseSaveParams(r)
	if !ok {
		http.Error(w, "invalid save parameters", http.StatusBadRequest)
		return
	}
	if !s.scanner.HasGame(console, game) {
		http.NotFound(w, r)
		return
	}

	data := s.saves.Get(console, game, saveType)
	if data == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	// #nosec G705 -- opaque binary save blob served as octet-stream;
	// X-Content-Type-Options: nosniff is set globally by securityHeaders.
	_, _ = w.Write(data)
}

func (s *Server) handlePostSave(w http.ResponseWriter, r *http.Request) {
	console, game, saveType, ok := parseSaveParams(r)
	if !ok {
		http.Error(w, "invalid save parameters", http.StatusBadRequest)
		return
	}
	// Gate on catalog membership: prevents unbounded disk growth via
	// unique game names (see SEC-4). The check runs before body read,
	// so attackers targeting unknown games never pay for file I/O.
	if !s.scanner.HasGame(console, game) {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<20) // 64 MB
	if err := s.saves.Put(console, game, saveType, r.Body); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			http.Error(w, "save too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	fetching := s.detailsCache != nil && s.detailsCache.Fetching()
	igdbConfigured := s.detailsCache != nil
	w.Header().Set("Content-Type", "application/json")
	// Polled every 2s while fetchingDetails is true — must never
	// heuristic-cache or the UI would never see the flag clear.
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"fetchingDetails": fetching,
		"igdbConfigured":  igdbConfigured,
	})
}

func (s *Server) handleRescan(w http.ResponseWriter, _ *http.Request) {
	if s.rescanner == nil {
		http.Error(
			w,
			`{"error":"rescan not available"}`,
			http.StatusServiceUnavailable,
		)
		return
	}
	if !s.rescanner.TriggerRescan() {
		http.Error(w, `{"error":"scan already in progress"}`, http.StatusConflict)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSONOK(w)
}

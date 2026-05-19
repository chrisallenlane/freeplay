package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/chrisallenlane/freeplay/internal/datadir"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSONOK(w)
}

func (s *Server) handleGames(w http.ResponseWriter, _ *http.Request) {
	data, err := s.scanner.CatalogJSON()
	if err != nil {
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Server) handleROM(w http.ResponseWriter, r *http.Request) {
	rom, ok := s.cfg.ROMs[r.PathValue("console")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveSecureFile(w, r, rom.Path, r.PathValue("file"), longCache)
}

func (s *Server) handleBIOS(w http.ResponseWriter, r *http.Request) {
	rom, ok := s.cfg.ROMs[r.PathValue("console")]
	if !ok || rom.Bios == "" {
		http.NotFound(w, r)
		return
	}
	serveFile(w, r, rom.Bios, longCache)
}

func (s *Server) handleCovers(w http.ResponseWriter, r *http.Request) {
	s.serveSecureFile(w, r, datadir.Covers(s.dataDir), r.PathValue("rest"), longCache)
}

func (s *Server) handleCacheFiles(w http.ResponseWriter, r *http.Request) {
	s.serveSecureFile(w, r, datadir.IGDBCache(s.dataDir), r.PathValue("rest"), longCache)
}

func (s *Server) handleManuals(w http.ResponseWriter, r *http.Request) {
	s.serveSecureFile(w, r, datadir.Manuals(s.dataDir), r.PathValue("rest"), longCache)
}

func (s *Server) handleGameDetails(w http.ResponseWriter, r *http.Request) {
	if s.detailsCache == nil {
		writeJSONError(w, "IGDB not configured", http.StatusNotFound)
		return
	}

	console := r.URL.Query().Get("console")
	rom := r.URL.Query().Get("rom")
	if !datadir.SafePathSegment(console) || !datadir.SafePathSegment(rom) {
		// SafePathSegment rejects empty, ".", "..", any traversal
		// substring, path separators, and NUL. Blocks the SEC-3
		// path-traversal PoC (console=../../../../tmp/evil) at the
		// HTTP boundary; defense-in-depth lives in Cache.Get.
		writeJSONError(w, "invalid console or rom parameter", http.StatusBadRequest)
		return
	}

	d := s.detailsCache.Get(console, rom)
	if d == nil {
		writeJSONError(w, "game not found", http.StatusNotFound)
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
		writeJSONError(w, "invalid save parameters", http.StatusBadRequest)
		return
	}
	if !s.scanner.HasGameSlug(console, game) {
		http.NotFound(w, r)
		return
	}

	data, err := s.saves.Get(console, game, saveType)
	if err != nil {
		slog.Warn(
			"save read failed",
			"console", console,
			"slug", game,
			"type", saveType,
			"error", err,
		)
		writeJSONError(w, "save read failed", http.StatusInternalServerError)
		return
	}
	if data == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	// #nosec G705 -- opaque binary save blob served as octet-stream;
	// X-Content-Type-Options: nosniff is set globally by securityHeaders.
	_, _ = w.Write(data)
}

func (s *Server) handlePostSave(w http.ResponseWriter, r *http.Request) {
	console, game, saveType, ok := parseSaveParams(r)
	if !ok {
		writeJSONError(w, "invalid save parameters", http.StatusBadRequest)
		return
	}
	// Gate on catalog membership: prevents unbounded disk growth via
	// unique game names (see SEC-4). The check runs before body read,
	// so attackers targeting unknown games never pay for file I/O.
	if !s.scanner.HasGameSlug(console, game) {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<20) // 64 MB
	if err := s.saves.Put(console, game, saveType, r.Body); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			writeJSONError(w, "save too large", http.StatusRequestEntityTooLarge)
			return
		}
		slog.Warn(
			"save write failed",
			"console", console,
			"slug", game,
			"type", saveType,
			"error", err,
		)
		writeJSONError(w, "save failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	fetching := s.detailsCache != nil && s.detailsCache.Fetching()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"fetchingDetails": fetching,
	})
}

func (s *Server) handleRescan(w http.ResponseWriter, _ *http.Request) {
	if s.rescanner == nil {
		slog.Warn("rescan failed", "error", "rescanner not configured")
		writeJSONError(w, "rescan not available", http.StatusServiceUnavailable)
		return
	}
	if !s.rescanner.TriggerRescan() {
		writeJSONError(w, "scan already in progress", http.StatusConflict)
		return
	}
	writeJSONOK(w)
}

package server

import "net/http"

func (s *Server) routes() {
	// Every non-idempotent or live-state /api/* route must never be
	// heuristic-cached by browsers. Wrap at registration time so
	// handlers stay focused on their response bodies.
	noStore := func(h http.HandlerFunc) http.Handler {
		return cacheControl("no-store", h)
	}

	// API routes
	s.mux.Handle("GET /api/health", noStore(s.handleHealth))
	s.mux.Handle("GET /api/games", noStore(s.handleGames))
	s.mux.Handle("GET /api/saves/{console}/{game}/{type}", noStore(s.handleGetSave))
	s.mux.Handle("POST /api/saves/{console}/{game}/{type}", noStore(s.handlePostSave))
	s.mux.Handle("POST /api/rescan", noStore(s.handleRescan))
	s.mux.Handle("GET /api/status", noStore(s.handleStatus))

	// ROM serving
	s.mux.HandleFunc("GET /roms/{console}/{file}", s.handleROM)

	// BIOS serving
	s.mux.HandleFunc("GET /bios/{console}", s.handleBIOS)

	// Cover art serving
	s.mux.HandleFunc("GET /covers/{rest...}", s.handleCovers)

	// Cached IGDB images
	s.mux.HandleFunc("GET /cache/igdb/{rest...}", s.handleCacheFiles)

	// Manual serving
	s.mux.HandleFunc("GET /manuals/{rest...}", s.handleManuals)

	// Embedded EmulatorJS — immutable cache; assets are embedded at build time
	s.mux.Handle("/emulatorjs/", cacheControl(longCacheImmutable, http.StripPrefix("/emulatorjs/", noDirListing(s.emulatorjsSub, http.FileServerFS(s.emulatorjsSub)))))

	// Game details
	s.mux.HandleFunc("GET /api/game-details", s.handleGameDetails)
	s.mux.Handle("GET /details", cacheControl("no-cache", s.servePage("details.html")))

	// Player page (explicit route before catch-all)
	s.mux.Handle("GET /play", cacheControl("no-cache", s.servePage("play.html")))

	// Embedded frontend (catch-all) — no-cache so deploys are picked up immediately
	s.mux.Handle("/", cacheControl("no-cache", noDirListing(s.frontendSub, http.FileServerFS(s.frontendSub))))
}

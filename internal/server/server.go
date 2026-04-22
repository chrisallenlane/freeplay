// Package server implements the freeplay HTTP server.
package server

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/igdb"
	"github.com/chrisallenlane/freeplay/internal/saves"
	"github.com/chrisallenlane/freeplay/internal/scanner"
)

const longCacheValue = "public, max-age=31536000, immutable"

// DetailsCache serves locally-cached game metadata.
type DetailsCache interface {
	Get(console, romFilename string) *igdb.GameDetails
	Fetching() bool
}

// Server is the Freeplay HTTP server.
type Server struct {
	cfg           *config.Config
	dataDir       string
	scanner       *scanner.Scanner
	saves         *saves.Manager
	detailsCache  DetailsCache
	frontendSub   fs.FS
	emulatorjsSub fs.FS
	mux           *http.ServeMux
	handler       http.Handler
}

// New creates a configured Server ready to listen.
// detailsCache may be nil if IGDB is not configured.
func New(
	cfg *config.Config,
	dataDir string,
	frontendFS, emulatorjsFS fs.FS,
	detailsCache DetailsCache,
) (*Server, error) {
	frontendSub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		return nil, fmt.Errorf("frontend fs: %w", err)
	}
	emulatorjsSub, err := fs.Sub(emulatorjsFS, "emulatorjs")
	if err != nil {
		return nil, fmt.Errorf("emulatorjs fs: %w", err)
	}

	s := &Server{
		cfg:           cfg,
		dataDir:       dataDir,
		scanner:       scanner.New(cfg, dataDir),
		saves:         saves.New(dataDir),
		detailsCache:  detailsCache,
		frontendSub:   frontendSub,
		emulatorjsSub: emulatorjsSub,
		mux:           http.NewServeMux(),
	}
	s.routes()
	s.handler = securityHeaders(s.mux)
	return s, nil
}

// Scanner returns the server's scanner for triggering async scans.
func (s *Server) Scanner() *scanner.Scanner {
	return s.scanner
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	return http.ListenAndServe(addr, s.handler)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")

		// Reject cross-origin POST requests. A custom header forces a CORS
		// preflight that the server will not grant, so browsers block the
		// request before it is sent.
		if r.Method == http.MethodPost && r.Header.Get("X-Requested-With") != "freeplay" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	// API routes
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/games", s.handleGames)
	s.mux.HandleFunc("GET /api/saves/{console}/{game}/{type}", s.handleGetSave)
	s.mux.HandleFunc("POST /api/saves/{console}/{game}/{type}", s.handlePostSave)
	s.mux.HandleFunc("POST /api/rescan", s.handleRescan)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)

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
	s.mux.Handle("/emulatorjs/", cacheControl(longCacheValue, http.StripPrefix("/emulatorjs/", http.FileServerFS(s.emulatorjsSub))))

	// Game details
	s.mux.HandleFunc("GET /api/game-details", s.handleGameDetails)
	s.mux.HandleFunc("GET /details", s.servePage("details.html"))

	// Player page (explicit route before catch-all)
	s.mux.HandleFunc("GET /play", s.servePage("play.html"))

	// Embedded frontend (catch-all) — no-cache so deploys are picked up immediately
	s.mux.Handle("/", cacheControl("no-cache", http.FileServerFS(s.frontendSub)))
}

func cacheControl(value string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", value)
		next.ServeHTTP(w, r)
	})
}

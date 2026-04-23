// Package server implements the freeplay HTTP server.
package server

import (
	"fmt"
	"io/fs"
	"net/http"
	"time"

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

// Rescanner triggers the scan/enrich/fetch pipeline in response to an
// HTTP /api/rescan request. TriggerRescan returns false if a pipeline is
// already running; the server maps that to HTTP 409.
type Rescanner interface {
	TriggerRescan() bool
}

// Server is the Freeplay HTTP server.
type Server struct {
	cfg           *config.Config
	dataDir       string
	scanner       *scanner.Scanner
	rescanner     Rescanner
	saves         *saves.Manager
	detailsCache  DetailsCache
	frontendSub   fs.FS
	emulatorjsSub fs.FS
	mux           *http.ServeMux
	handler       http.Handler
}

// New creates a configured Server ready to listen. detailsCache may be
// nil if IGDB is not configured. rescanner may be nil, in which case
// POST /api/rescan returns 503.
func New(
	cfg *config.Config,
	dataDir string,
	frontendFS, emulatorjsFS fs.FS,
	detailsCache DetailsCache,
	scn *scanner.Scanner,
	rescanner Rescanner,
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
		scanner:       scn,
		rescanner:     rescanner,
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

// ListenAndServe starts the HTTP server with production timeouts.
func (s *Server) ListenAndServe() error {
	return s.newHTTPServer().ListenAndServe()
}

// newHTTPServer constructs the http.Server with explicit timeouts.
// Timeouts defeat slow-body drip attacks (see SEC-1) that otherwise
// pin heap memory per idle connection. WriteTimeout of 60s is generous
// enough for a 64 MiB save upload over LAN.
func (s *Server) newHTTPServer() *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", s.cfg.Port),
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 14,
	}
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

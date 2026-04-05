// Package server implements the freeplay HTTP server.
package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	s.mux.Handle("/emulatorjs/", longCache(http.StripPrefix("/emulatorjs/", http.FileServerFS(s.emulatorjsSub))))

	// Game details
	s.mux.HandleFunc("GET /api/game-details", s.handleGameDetails)
	s.mux.HandleFunc("GET /details", s.handleDetailsPage)

	// Player page (explicit route before catch-all)
	s.mux.HandleFunc("GET /play", s.handlePlay)

	// Embedded frontend (catch-all) — no-cache so deploys are picked up immediately
	s.mux.Handle("/", noCache(http.FileServerFS(s.frontendSub)))
}

func writeJSONOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSONOK(w)
}

func (s *Server) handleGames(w http.ResponseWriter, _ *http.Request) {
	data, err := s.scanner.CatalogJSON()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
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
	s.serveSecureFile(w, r, rom.Path, r.PathValue("file"))
}

func (s *Server) handleBIOS(w http.ResponseWriter, r *http.Request) {
	rom, ok := s.cfg.ROMs[r.PathValue("console")]
	if !ok || rom.Bios == "" {
		http.NotFound(w, r)
		return
	}
	serveFile(w, r, rom.Bios)
}

func (s *Server) handleCovers(w http.ResponseWriter, r *http.Request) {
	s.serveSecureFile(w, r, filepath.Join(s.dataDir, "covers"), r.PathValue("rest"))
}

func (s *Server) handleCacheFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", longCacheValue)
	s.serveSecureFile(
		w, r,
		filepath.Join(s.dataDir, "cache", "igdb"),
		r.PathValue("rest"),
	)
}

func (s *Server) handleManuals(w http.ResponseWriter, r *http.Request) {
	s.serveSecureFile(w, r, filepath.Join(s.dataDir, "manuals"), r.PathValue("rest"))
}

func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.frontendSub, "play.html")
}

func (s *Server) handleDetailsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.frontendSub, "details.html")
}

func (s *Server) handleGameDetails(w http.ResponseWriter, r *http.Request) {
	if s.detailsCache == nil {
		http.Error(w, `{"error":"IGDB not configured"}`, http.StatusNotFound)
		return
	}

	consoleName := r.URL.Query().Get("console")
	rom := r.URL.Query().Get("rom")
	if consoleName == "" || rom == "" {
		http.Error(
			w,
			`{"error":"console and rom parameters required"}`,
			http.StatusBadRequest,
		)
		return
	}

	d := s.detailsCache.Get(consoleName, rom)
	if d == nil {
		http.Error(w, `{"error":"game not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func longCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", longCacheValue)
		next.ServeHTTP(w, r)
	})
}

func safeName(s string) bool {
	return s != "" &&
		!strings.Contains(s, "..") &&
		!strings.Contains(s, "/") &&
		!strings.Contains(s, "\\") &&
		!strings.ContainsRune(s, 0)
}

func parseSaveParams(r *http.Request) (console, game, saveType string, ok bool) {
	console = r.PathValue("console")
	game = r.PathValue("game")
	saveType = r.PathValue("type")
	ok = safeName(console) && safeName(game) && saves.ValidType(saveType)
	return
}

func (s *Server) handleGetSave(w http.ResponseWriter, r *http.Request) {
	consoleName, game, saveType, ok := parseSaveParams(r)
	if !ok {
		http.Error(w, "invalid save parameters", http.StatusBadRequest)
		return
	}

	data := s.saves.Get(consoleName, game, saveType)
	if data == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

func (s *Server) handlePostSave(w http.ResponseWriter, r *http.Request) {
	consoleName, game, saveType, ok := parseSaveParams(r)
	if !ok {
		http.Error(w, "invalid save parameters", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<20) // 64 MB
	if err := s.saves.Put(consoleName, game, saveType, r.Body); err != nil {
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	fetching := s.detailsCache != nil && s.detailsCache.Fetching()
	igdbConfigured := s.detailsCache != nil
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"fetchingDetails": fetching,
		"igdbConfigured":  igdbConfigured,
	})
}

func (s *Server) handleRescan(w http.ResponseWriter, _ *http.Request) {
	if !s.scanner.Scan() {
		http.Error(w, `{"error":"scan already in progress"}`, http.StatusConflict)
		return
	}
	writeJSONOK(w)
}

func serveFile(w http.ResponseWriter, r *http.Request, path string) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", longCacheValue)
	http.ServeFile(w, r, path)
}

func (s *Server) serveSecureFile(w http.ResponseWriter, r *http.Request, baseDir, file string) {
	baseDir = filepath.Clean(baseDir)

	clean := filepath.Clean(file)
	if strings.Contains(clean, "..") {
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join(baseDir, clean)

	// Verify cleaned path is within base directory
	if !strings.HasPrefix(fullPath, baseDir+string(filepath.Separator)) && fullPath != baseDir {
		http.NotFound(w, r)
		return
	}

	serveFile(w, r, fullPath)
}

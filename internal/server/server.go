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
	"github.com/chrisallenlane/freeplay/internal/saves"
	"github.com/chrisallenlane/freeplay/internal/scanner"
)

// CoverStatus reports whether cover art is being fetched.
type CoverStatus interface {
	Fetching() bool
}

// Server is the Freeplay HTTP server.
type Server struct {
	cfg           *config.Config
	dataDir       string
	scanner       *scanner.Scanner
	saves         *saves.Manager
	coverStatus   CoverStatus
	frontendSub   fs.FS
	emulatorjsSub fs.FS
	mux           *http.ServeMux
	handler       http.Handler
}

// New creates a configured Server ready to listen.
// coverStatus may be nil if cover art fetching is not configured.
func New(cfg *config.Config, dataDir string, frontendFS, emulatorjsFS fs.FS, coverStatus CoverStatus) (*Server, error) {
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
		coverStatus:   coverStatus,
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

	// Embedded EmulatorJS
	s.mux.Handle("/emulatorjs/", http.StripPrefix("/emulatorjs/", http.FileServerFS(s.emulatorjsSub)))

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

func (s *Server) serveConsoleFile(w http.ResponseWriter, r *http.Request, resolve func(string) (string, bool)) {
	dir, ok := resolve(r.PathValue("console"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveSecureFile(w, r, dir, r.PathValue("file"))
}

func (s *Server) handleROM(w http.ResponseWriter, r *http.Request) {
	s.serveConsoleFile(w, r, func(name string) (string, bool) {
		rom, ok := s.cfg.ROMs[name]
		return rom.Path, ok
	})
}

func (s *Server) handleBIOS(w http.ResponseWriter, r *http.Request) {
	path, ok := s.cfg.BIOS[r.PathValue("console")]
	if !ok {
		http.NotFound(w, r)
		return
	}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, path)
}

func (s *Server) handleCovers(w http.ResponseWriter, r *http.Request) {
	s.serveSecureFile(w, r, filepath.Join(s.dataDir, "covers"), r.PathValue("rest"))
}

func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.frontendSub, "play.html")
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
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
	fetching := s.coverStatus != nil && s.coverStatus.Fetching()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"fetchingCovers": fetching})
}

func (s *Server) handleRescan(w http.ResponseWriter, _ *http.Request) {
	if !s.scanner.Scan() {
		http.Error(w, `{"error":"scan already in progress"}`, http.StatusConflict)
		return
	}
	writeJSONOK(w)
}

func (s *Server) serveSecureFile(w http.ResponseWriter, r *http.Request, baseDir, file string) {
	clean := filepath.Clean(file)
	if strings.Contains(clean, "..") {
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join(baseDir, clean)

	// Verify resolved path is within base directory
	if !strings.HasPrefix(fullPath, baseDir+string(filepath.Separator)) && fullPath != baseDir {
		http.NotFound(w, r)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, fullPath)
}

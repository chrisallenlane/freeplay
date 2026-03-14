package server

import (
	"embed"
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

// Server is the Freeplay HTTP server.
type Server struct {
	cfg          *config.Config
	dataDir      string
	scanner      *scanner.Scanner
	saves        *saves.Manager
	frontendFS   embed.FS
	emulatorjsFS embed.FS
	mux          *http.ServeMux
}

// New creates a configured Server ready to listen.
func New(cfg *config.Config, dataDir string, frontendFS, emulatorjsFS embed.FS) *Server {
	s := &Server{
		cfg:          cfg,
		dataDir:      dataDir,
		scanner:      scanner.New(cfg, dataDir),
		saves:        saves.New(dataDir),
		frontendFS:   frontendFS,
		emulatorjsFS: emulatorjsFS,
		mux:          http.NewServeMux(),
	}
	s.routes()
	return s
}

// Scanner returns the server's scanner for triggering async scans.
func (s *Server) Scanner() *scanner.Scanner {
	return s.scanner
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) routes() {
	// API routes
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/games", s.handleGames)
	s.mux.HandleFunc("GET /api/saves/{console}/{game}/{type}", s.handleGetSave)
	s.mux.HandleFunc("POST /api/saves/{console}/{game}/{type}", s.handlePostSave)
	s.mux.HandleFunc("POST /api/rescan", s.handleRescan)

	// ROM serving
	s.mux.HandleFunc("GET /roms/{console}/{file}", s.handleROM)

	// BIOS serving
	s.mux.HandleFunc("GET /bios/{console}/{file}", s.handleBIOS)

	// Cover art serving
	s.mux.HandleFunc("GET /covers/{rest...}", s.handleCovers)

	// Embedded EmulatorJS
	emulatorjsSub, _ := fs.Sub(s.emulatorjsFS, "emulatorjs")
	s.mux.Handle("/emulatorjs/", http.StripPrefix("/emulatorjs/", http.FileServerFS(emulatorjsSub)))

	// Player page (explicit route before catch-all)
	s.mux.HandleFunc("GET /play", s.handlePlay)

	// Embedded frontend (catch-all)
	frontendSub, _ := fs.Sub(s.frontendFS, "frontend")
	s.mux.Handle("/", http.FileServerFS(frontendSub))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleGames(w http.ResponseWriter, r *http.Request) {
	data, err := s.scanner.CatalogJSON()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) handleROM(w http.ResponseWriter, r *http.Request) {
	consoleName := r.PathValue("console")
	file := r.PathValue("file")

	rom, ok := s.cfg.ROMs[consoleName]
	if !ok {
		http.NotFound(w, r)
		return
	}

	s.serveSecureFile(w, r, rom.Path, file)
}

func (s *Server) handleBIOS(w http.ResponseWriter, r *http.Request) {
	consoleName := r.PathValue("console")
	file := r.PathValue("file")

	biosDir, ok := s.cfg.BIOS[consoleName]
	if !ok {
		http.NotFound(w, r)
		return
	}

	s.serveSecureFile(w, r, biosDir, file)
}

func (s *Server) handleCovers(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	coversDir := filepath.Join(s.dataDir, "covers")
	fullPath := filepath.Join(coversDir, filepath.Clean(rest))

	// Path traversal check
	if !strings.HasPrefix(fullPath, coversDir+string(filepath.Separator)) {
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

func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, s.frontendFS, "frontend/play.html")
}

func (s *Server) handleGetSave(w http.ResponseWriter, r *http.Request) {
	consoleName := r.PathValue("console")
	game := r.PathValue("game")
	saveType := r.PathValue("type")

	if !saves.ValidType(saveType) {
		http.Error(w, "invalid save type", http.StatusBadRequest)
		return
	}

	if !s.saves.Get(w, consoleName, game, saveType) {
		http.NotFound(w, r)
	}
}

func (s *Server) handlePostSave(w http.ResponseWriter, r *http.Request) {
	consoleName := r.PathValue("console")
	game := r.PathValue("game")
	saveType := r.PathValue("type")

	if !saves.ValidType(saveType) {
		http.Error(w, "invalid save type", http.StatusBadRequest)
		return
	}

	if err := s.saves.Put(consoleName, game, saveType, r.Body); err != nil {
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request) {
	if !s.scanner.Scan() {
		http.Error(w, `{"error":"scan already in progress"}`, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

	http.ServeFile(w, r, fullPath)
}

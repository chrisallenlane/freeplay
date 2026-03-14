package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/chrisallenlane/freeplay/internal/config"
)

// Server is the Freeplay HTTP server.
type Server struct {
	cfg          *config.Config
	dataDir      string
	frontendFS   embed.FS
	emulatorjsFS embed.FS
	mux          *http.ServeMux
}

// New creates a configured Server ready to listen.
func New(cfg *config.Config, dataDir string, frontendFS, emulatorjsFS embed.FS) *Server {
	s := &Server{
		cfg:          cfg,
		dataDir:      dataDir,
		frontendFS:   frontendFS,
		emulatorjsFS: emulatorjsFS,
		mux:          http.NewServeMux(),
	}
	s.routes()
	return s
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)

	// Serve embedded frontend at /
	frontendSub, _ := fs.Sub(s.frontendFS, "frontend")
	s.mux.Handle("/", http.FileServerFS(frontendSub))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/chrisallenlane/freeplay/internal/datadir"
	"github.com/chrisallenlane/freeplay/internal/saves"
)

// serveFile serves a single file from the filesystem with the given
// Cache-Control directive. Returns 404 if the path does not exist or
// is a directory.
func serveFile(w http.ResponseWriter, r *http.Request, path, cacheControl string) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", cacheControl)
	http.ServeFile(w, r, path)
}

// serveSecureFile serves a file from baseDir after verifying the cleaned
// target path stays within baseDir. Used for user-controlled file routes
// (ROMs, covers, manuals, IGDB cache). The cacheControl argument is
// passed through to the response.
func (s *Server) serveSecureFile(w http.ResponseWriter, r *http.Request, baseDir, file, cacheControl string) {
	fullPath := filepath.Join(baseDir, filepath.Clean(file))
	if !datadir.PathInside(fullPath, baseDir) {
		http.NotFound(w, r)
		return
	}
	serveFile(w, r, fullPath, cacheControl)
}

// servePage returns a handler that serves a named file from the embedded
// frontend filesystem. Cache-Control is applied at mux registration
// time via cacheControl("no-cache", ...) — consistent with other routes.
func (s *Server) servePage(filename string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, s.frontendSub, filename)
	}
}

// parseSaveParams extracts and validates the {console}/{game}/{type} path
// parameters for save routes. The bool is true only when all three values
// are safe filenames and the save type is recognized.
func parseSaveParams(r *http.Request) (string, string, string, bool) {
	console := r.PathValue("console")
	game := r.PathValue("game")
	saveType := r.PathValue("type")
	ok := datadir.SafePathSegment(console) &&
		datadir.SafePathSegment(game) &&
		saves.ValidType(saveType)
	return console, game, saveType, ok
}

// writeJSONOK writes a 200 response with body {"status":"ok"}.
func writeJSONOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// writeJSONError writes a JSON-shaped error body with the given
// status code. Response Content-Type is application/json so callers
// know how to parse the body; the bare http.Error helper sets
// text/plain which is inconsistent with our /api/* JSON responses.
func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

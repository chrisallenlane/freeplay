package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

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

// noDirListing wraps next so that requests whose URL path resolves to
// a directory in fsys return 404 unless an index.html sits inside
// that directory. Stops http.FileServerFS from emitting clickable
// directory listings that leak EmulatorJS version / bundled cores
// (see SEC-6 / L-1).
func noDirListing(fsys fs.FS, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// path.Clean strips trailing slashes and normalizes to a
		// path that fs.ValidPath accepts (fs.Stat rejects trailing /).
		name := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if name == "" {
			name = "."
		}
		info, err := fs.Stat(fsys, name)
		if err == nil && info.IsDir() {
			if _, err := fs.Stat(fsys, path.Join(name, "index.html")); err != nil {
				http.NotFound(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// cacheControl sets Cache-Control: <value> on every response before
// delegating. Used at mux registration time to attach a cache policy
// to a route without threading the header through every handler.
func cacheControl(value string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", value)
		next.ServeHTTP(w, r)
	})
}

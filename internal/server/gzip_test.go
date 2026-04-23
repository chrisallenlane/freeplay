package server

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGzipCompressesJSON verifies that /api/games is gzip-encoded when
// the client advertises Accept-Encoding: gzip, decompresses cleanly,
// and carries Vary: Accept-Encoding.
func TestGzipCompressesJSON(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
	if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", got)
	}

	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer func() { _ = gr.Close() }()
	decoded, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(decoded, &parsed); err != nil {
		t.Fatalf("decompressed body not valid JSON: %v\n%s", err, decoded)
	}
	if _, ok := parsed["games"]; !ok {
		t.Errorf("decompressed response missing \"games\" key: %v", parsed)
	}
}

// TestGzipOmittedWithoutAcceptEncoding verifies that responses are not
// gzip-encoded when the client does not request it.
func TestGzipOmittedWithoutAcceptEncoding(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want empty", got)
	}
	if w.Body.Len() == 0 {
		t.Fatal("empty body")
	}
	var parsed map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("uncompressed body not valid JSON: %v", err)
	}
}

// TestGzipSkipsBinaryRoutes verifies that image routes are not
// gzip-encoded regardless of Accept-Encoding. PNGs are already
// compressed; double-compressing wastes CPU and grows the payload.
func TestGzipSkipsBinaryRoutes(t *testing.T) {
	srv, dir := testServer(t)
	writeCacheFile(t, dir, "NES", "Mega Man", "cover_thumb.jpg",
		[]byte{0xff, 0xd8, 0xff, 0xe0, 0x00})

	req := httptest.NewRequest(http.MethodGet,
		"/cache/igdb/NES/Mega%20Man/cover_thumb.jpg", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding on image = %q, want empty", got)
	}
}

// TestGzipStripsUpstreamContentLength verifies that an upstream
// Content-Length header from the inner handler is removed on compressed
// responses — the compressed body length would not match the upstream
// length and would break HTTP framing. Kills the statement-deletion
// mutation of g.Header().Del("Content-Length").
func TestGzipStripsUpstreamContentLength(t *testing.T) {
	body := `{"hello":"world"}`
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "17") // upstream-declared length
		_, _ = io.WriteString(w, body)
	})
	wrapped := gzipMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := w.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length should be stripped on gzip response, got %q", got)
	}
}

// TestShouldCompress covers the content-type allowlist directly, making
// sure charset suffixes are handled and unknown types pass through.
func TestShouldCompress(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"text/css", true},
		{"text/css; charset=utf-8", true},
		{" text/html ; charset=utf-8", true}, // leading/trailing whitespace around the type
		{"image/png", false},
		{"image/jpeg", false},
		{"application/octet-stream", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := shouldCompress(tt.ct); got != tt.want {
			t.Errorf("shouldCompress(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

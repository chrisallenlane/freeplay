package covers

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer creates an httptest.Server that mimics the IGDB API.
// queryLog receives the Apicalypse query body for each /v4/games request.
// gamesResp is the JSON response to return for /v4/games.
func newTestServer(t *testing.T, queryLog *[]string, gamesResp []byte) *httptest.Server {
	t.Helper()

	// Create a 1x1 PNG for cover image responses
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			})

		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			body := make([]byte, r.ContentLength)
			r.Body.Read(body)
			*queryLog = append(*queryLog, string(body))
			w.Write(gamesResp)

		case strings.HasSuffix(r.URL.Path, "/v4/covers"):
			json.NewEncoder(w).Encode([]map[string]string{
				{"url": "//images.igdb.com/t_thumb/cover.jpg"},
			})

		case strings.Contains(r.URL.Path, "t_cover_big"):
			w.Header().Set("Content-Type", "image/png")
			png.Encode(w, img)

		default:
			// Serve the cover image for any other path (covers URL rewrite)
			w.Header().Set("Content-Type", "image/png")
			png.Encode(w, img)
		}
	}))
}

func TestFetchWithPlatformFilter(t *testing.T) {
	var queries []string
	gamesResp, _ := json.Marshal([]map[string]any{
		{"name": "Mega Man", "cover": 1},
	})

	ts := newTestServer(t, &queries, gamesResp)
	defer ts.Close()

	f := NewIGDBFetcher("test-id:test-secret")
	// Replace the HTTP client with one that redirects to our test server
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	_, err := f.Fetch("Mega Man", "NES", []int{18, 99})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	if len(queries) == 0 {
		t.Fatal("no queries recorded")
	}

	query := queries[0]
	if !strings.Contains(query, "where platforms = (18,99)") {
		t.Errorf("query missing platform filter: %s", query)
	}
	if !strings.Contains(query, "limit 5") {
		t.Errorf("query should use limit 5: %s", query)
	}
	if !strings.Contains(query, "fields name, cover") {
		t.Errorf("query should request name field: %s", query)
	}
}

func TestFetchWithoutPlatformFilter(t *testing.T) {
	var queries []string
	gamesResp, _ := json.Marshal([]map[string]any{
		{"name": "Mega Man", "cover": 1},
	})

	ts := newTestServer(t, &queries, gamesResp)
	defer ts.Close()

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	_, err := f.Fetch("Mega Man", "NES", nil)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	if len(queries) == 0 {
		t.Fatal("no queries recorded")
	}

	query := queries[0]
	if strings.Contains(query, "where platforms") {
		t.Errorf("query should NOT have platform filter when IDs are nil: %s", query)
	}
	if !strings.Contains(query, "limit 5") {
		t.Errorf("query should use limit 5: %s", query)
	}
}

func TestFetchNameMatching(t *testing.T) {
	var queries []string
	// Return multiple results — exact match is second
	gamesResp, _ := json.Marshal([]map[string]any{
		{"name": "Mega Man X", "cover": 10},
		{"name": "Mega Man", "cover": 20},
		{"name": "Mega Man 2", "cover": 30},
	})

	ts := newTestServer(t, &queries, gamesResp)
	defer ts.Close()

	// Track which cover ID is requested
	var coverQueries []string
	origHandler := ts.Config.Handler
	ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/v4/covers") {
			body := make([]byte, r.ContentLength)
			r.Body.Read(body)
			coverQueries = append(coverQueries, string(body))
		}
		origHandler.ServeHTTP(w, r)
	})

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	_, err := f.Fetch("Mega Man", "NES", nil)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	// Should have requested cover ID 20 (the exact name match), not 10 (first result)
	if len(coverQueries) == 0 {
		t.Fatal("no cover queries recorded")
	}
	if !strings.Contains(coverQueries[0], "where id = 20") {
		t.Errorf("should pick exact name match (cover 20), got query: %s", coverQueries[0])
	}
}

func TestFetchNoResults(t *testing.T) {
	var queries []string
	gamesResp, _ := json.Marshal([]map[string]any{})

	ts := newTestServer(t, &queries, gamesResp)
	defer ts.Close()

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	img, err := f.Fetch("Nonexistent Game", "NES", []int{18})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if img != nil {
		t.Error("expected nil image for no results")
	}
}

func TestFetchAllResultsNoCover(t *testing.T) {
	var queries []string
	gamesResp, _ := json.Marshal([]map[string]any{
		{"name": "Game A", "cover": 0},
		{"name": "Game B", "cover": 0},
	})

	ts := newTestServer(t, &queries, gamesResp)
	defer ts.Close()

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	img, err := f.Fetch("Game A", "NES", nil)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if img != nil {
		t.Error("expected nil image when no results have cover art")
	}
}

// rewriteTransport redirects all HTTP requests to a test server URL.
type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	// Extract just the host:port from target
	req.URL.Host = strings.TrimPrefix(t.target, "http://")
	return t.base.RoundTrip(req)
}

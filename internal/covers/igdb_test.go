package covers

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

// newTestFetcher creates an IGDBFetcher wired to a test server.
// Returns the fetcher, a log of Apicalypse queries, and the test server.
func newTestFetcher(t *testing.T, games []map[string]any) (*IGDBFetcher, *[]string, *httptest.Server) {
	t.Helper()
	var queries []string
	gamesResp, _ := json.Marshal(games)
	ts := newTestServer(t, &queries, gamesResp)
	t.Cleanup(ts.Close)
	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}
	return f, &queries, ts
}

func FuzzNewIGDBFetcher(f *testing.F) {
	f.Add("client_id:client_secret")
	f.Add("a:b")
	f.Add(":")
	f.Add(":secret")
	f.Add("id:")
	f.Add("")
	f.Add("nocolon")
	f.Add("a:b:c")

	f.Fuzz(func(t *testing.T, apiKey string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NewIGDBFetcher panicked on input %q: %v", apiKey, r)
			}
		}()
		NewIGDBFetcher(apiKey)
	})
}

func TestFetchWithPlatformFilter(t *testing.T) {
	f, queries, _ := newTestFetcher(t, []map[string]any{
		{"name": "Mega Man", "cover": 1},
	})

	_, err := f.Fetch("Mega Man", "NES", []int{18, 99})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	if len(*queries) == 0 {
		t.Fatal("no queries recorded")
	}

	query := (*queries)[0]
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
	f, queries, _ := newTestFetcher(t, []map[string]any{
		{"name": "Mega Man", "cover": 1},
	})

	_, err := f.Fetch("Mega Man", "NES", nil)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	if len(*queries) == 0 {
		t.Fatal("no queries recorded")
	}

	query := (*queries)[0]
	if strings.Contains(query, "where platforms") {
		t.Errorf("query should NOT have platform filter when IDs are nil: %s", query)
	}
	if !strings.Contains(query, "limit 5") {
		t.Errorf("query should use limit 5: %s", query)
	}
}

func TestFetchNameMatching(t *testing.T) {
	f, _, ts := newTestFetcher(t, []map[string]any{
		{"name": "Mega Man X", "cover": 10},
		{"name": "Mega Man", "cover": 20},
		{"name": "Mega Man 2", "cover": 30},
	})

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

func TestFetchFirstCoverFallback(t *testing.T) {
	f, _, ts := newTestFetcher(t, []map[string]any{
		{"name": "Mega Man X", "cover": 10},
		{"name": "Mega Man 2", "cover": 20},
	})

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

	// Search for a name that doesn't exactly match any result
	img, err := f.Fetch("Mega Man", "NES", nil)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if img == nil {
		t.Fatal("expected non-nil image when results with covers exist")
	}
	// Should pick the first result with a cover (cover ID 10)
	if len(coverQueries) == 0 {
		t.Fatal("no cover queries recorded")
	}
	if !strings.Contains(coverQueries[0], "where id = 10") {
		t.Errorf("should fall back to first result with cover (10), got query: %s", coverQueries[0])
	}
}

func TestFetchCaseInsensitiveMatch(t *testing.T) {
	// First result is a different game; second is a case-insensitive match.
	// EqualFold should prefer the case-insensitive match over the first result.
	f, _, ts := newTestFetcher(t, []map[string]any{
		{"name": "Mega Man X", "cover": 10},
		{"name": "mega man", "cover": 20},
	})

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

	// Search with different casing — should match case-insensitively
	img, err := f.Fetch("Mega Man", "NES", nil)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if img == nil {
		t.Fatal("expected non-nil image for case-insensitive match")
	}
	// Should pick the case-insensitive match (cover ID 20), not first result (10)
	if len(coverQueries) == 0 {
		t.Fatal("no cover queries recorded")
	}
	if !strings.Contains(coverQueries[0], "where id = 20") {
		t.Errorf("should pick case-insensitive match (20), got query: %s", coverQueries[0])
	}
}

func TestFetchNoResults(t *testing.T) {
	f, _, _ := newTestFetcher(t, []map[string]any{})

	img, err := f.Fetch("Nonexistent Game", "NES", []int{18})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if img != nil {
		t.Error("expected nil image for no results")
	}
}

func TestFetchAllResultsNoCover(t *testing.T) {
	f, _, _ := newTestFetcher(t, []map[string]any{
		{"name": "Game A", "cover": 0},
		{"name": "Game B", "cover": 0},
	})

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

func TestGetTokenNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("forbidden"))
			return
		}
	}))
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	_, err := f.Fetch("Test", "NES", nil)
	if err == nil {
		t.Fatal("expected error for non-200 token response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention status code: %v", err)
	}
}

func TestGetTokenDecodeError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			w.Write([]byte("not json"))
			return
		}
	}))
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	_, err := f.Fetch("Test", "NES", nil)
	if err == nil {
		t.Fatal("expected error for invalid token JSON")
	}
	if !strings.Contains(err.Error(), "parsing token response") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFetchAPIErrorNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			})
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
		}
	}))
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	_, err := f.Fetch("Test", "NES", nil)
	if err == nil {
		t.Fatal("expected error for 500 API response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code: %v", err)
	}
}

func TestFetchEmptyCoverURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			})
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			json.NewEncoder(w).Encode([]map[string]any{
				{"name": "Test Game", "cover": 1},
			})
		case strings.HasSuffix(r.URL.Path, "/v4/covers"):
			json.NewEncoder(w).Encode([]map[string]string{
				{"url": ""},
			})
		}
	}))
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	img, err := f.Fetch("Test Game", "NES", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img != nil {
		t.Error("expected nil image for empty cover URL")
	}
}

func TestFetchCoverImageNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			})
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			json.NewEncoder(w).Encode([]map[string]any{
				{"name": "Test Game", "cover": 1},
			})
		case strings.HasSuffix(r.URL.Path, "/v4/covers"):
			json.NewEncoder(w).Encode([]map[string]string{
				{"url": "//images.igdb.com/t_thumb/cover.jpg"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	_, err := f.Fetch("Test Game", "NES", nil)
	if err == nil {
		t.Fatal("expected error for non-200 image response")
	}
	if !strings.Contains(err.Error(), "cover image returned") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestFetchTokenRetryOn401 verifies that apiRequestRetry clears the stale
// token and retries the request when the server responds with 401.
func TestFetchTokenRetryOn401(t *testing.T) {
	var gamesCallCount atomic.Int32

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	gamesResp, _ := json.Marshal([]map[string]any{
		{"name": "Test Game", "cover": 1},
	})

	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
				json.NewEncoder(w).Encode(map[string]any{
					"access_token": "fresh-token",
					"expires_in":   3600,
				})

			case strings.HasSuffix(r.URL.Path, "/v4/games"):
				n := gamesCallCount.Add(1)
				if n == 1 {
					// Simulate an expired token on the first request.
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Write(gamesResp)

			case strings.HasSuffix(r.URL.Path, "/v4/covers"):
				json.NewEncoder(w).Encode([]map[string]string{
					{"url": "//images.igdb.com/t_thumb/cover.jpg"},
				})

			default:
				// Serve a cover image for all other paths (image download).
				w.Header().Set("Content-Type", "image/png")
				png.Encode(w, img)
			}
		}),
	)
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{base: http.DefaultTransport, target: ts.URL},
	}

	// Pre-set a stale token so the first request uses it without calling the
	// token endpoint, triggering the 401 path.
	f.token = "stale-token"
	f.tokenExpiry = time.Now().Add(time.Hour)

	result, err := f.Fetch("Test Game", "NES", nil)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil image, got nil")
	}

	if n := gamesCallCount.Load(); n < 2 {
		t.Errorf("expected at least 2 /v4/games requests (initial + retry), got %d", n)
	}
}

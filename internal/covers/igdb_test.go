package covers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

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

// writeTokenResponse writes the standard test OAuth2 token response.
func writeTokenResponse(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "test-token",
		"expires_in":   3600,
	})
}

// newTestFetcher creates an IGDBFetcher whose HTTP client is wired to call
// handler instead of the real IGDB API. The test server is closed via
// t.Cleanup when the test completes.
func newTestFetcher(t *testing.T, handler http.HandlerFunc) *IGDBFetcher {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: ts.URL,
		},
	}
	return f
}

func FuzzTransformImageURL(f *testing.F) {
	f.Add("//images.igdb.com/igdb/image/upload/t_thumb/abc.jpg", "t_original")
	f.Add("https://images.igdb.com/igdb/image/upload/t_thumb/abc.jpg", "t_cover_big")
	f.Add("", "")
	f.Add("//", "t_thumb")

	f.Fuzz(func(t *testing.T, rawURL, size string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf(
					"transformImageURL panicked on (%q, %q): %v",
					rawURL, size, r,
				)
			}
		}()

		result := transformImageURL(rawURL, size)

		// If the input starts with "//", output must start with "https:".
		if strings.HasPrefix(rawURL, "//") &&
			!strings.HasPrefix(result, "https:") {
			t.Errorf(
				"transformImageURL(%q, %q) = %q: want https: prefix",
				rawURL, size, result,
			)
		}

		// Output length is bounded: it can only grow by prepending "https:"
		// (6 bytes) and replacing "t_thumb" with size (net change =
		// len(size) - len("t_thumb")).  A generous bound of
		// len(rawURL) + len(size) + 10 covers all cases.
		maxLen := len(rawURL) + len(size) + 10
		if len(result) > maxLen {
			t.Errorf(
				"transformImageURL(%q, %q) result length %d exceeds bound %d",
				rawURL, size, len(result), maxLen,
			)
		}
	})
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

func TestIntsToStrings(t *testing.T) {
	tests := []struct {
		input []int
		want  []string
	}{
		{[]int{18}, []string{"18"}},
		{[]int{18, 99}, []string{"18", "99"}},
		{[]int{}, []string{}},
	}
	for _, tt := range tests {
		got := intsToStrings(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf(
				"intsToStrings(%v) = %v, want %v",
				tt.input, got, tt.want,
			)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf(
					"intsToStrings(%v)[%d] = %q, want %q",
					tt.input, i, got[i], tt.want[i],
				)
			}
		}
	}
}

func TestSearchGameExactMatch(t *testing.T) {
	searchResp, _ := json.Marshal([]map[string]any{
		{"id": 42, "name": "Mega Man X"},
		{"id": 17, "name": "Mega Man"},
		{"id": 99, "name": "Mega Man 2"},
	})
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			_, _ = w.Write(searchResp)
		}
	})

	id, err := f.SearchGame("Mega Man", nil)
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if id != 17 {
		t.Errorf("SearchGame() = %d, want 17 (exact match)", id)
	}
}

func TestSearchGameNoMatch(t *testing.T) {
	searchResp, _ := json.Marshal([]map[string]any{
		{"id": 42, "name": "Mega Man X"},
	})
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			_, _ = w.Write(searchResp)
		}
	})

	id, err := f.SearchGame("Mega Man", nil)
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if id != 0 {
		t.Errorf("SearchGame() = %d, want 0 (no exact match)", id)
	}
}

func TestSearchGameWithPlatformFilter(t *testing.T) {
	var capturedQuery string
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			capturedQuery = string(body)
			resp, _ := json.Marshal([]map[string]any{
				{"id": 18, "name": "Metroid"},
			})
			_, _ = w.Write(resp)
		}
	})

	_, err := f.SearchGame("Metroid", []int{18, 99})
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if !strings.Contains(capturedQuery, "where platforms = (18,99)") {
		t.Errorf("query missing platform filter: %s", capturedQuery)
	}
}

func TestFetchDetailsByID(t *testing.T) {
	detailsResp, _ := json.Marshal([]map[string]any{
		{
			"id":                 17,
			"name":               "Mega Man",
			"url":                "https://www.igdb.com/games/mega-man",
			"summary":            "A platformer.",
			"storyline":          "A robot fights evil.",
			"first_release_date": int64(565_920_000),
			"cover":              map[string]any{"url": "//images.igdb.com/t_thumb/abc.jpg"},
			"platforms":          []map[string]any{{"name": "NES"}},
			"involved_companies": []map[string]any{
				{
					"company":   map[string]any{"name": "Capcom"},
					"developer": true,
					"publisher": true,
				},
			},
			"collection":  map[string]any{"name": "Mega Man"},
			"screenshots": []map[string]any{{"url": "//images.igdb.com/t_thumb/ss1.jpg"}},
			"artworks":    []map[string]any{{"url": "//images.igdb.com/t_thumb/art1.jpg"}},
		},
	})
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			_, _ = w.Write(detailsResp)
		}
	})

	details, err := f.FetchDetailsByID(17)
	if err != nil {
		t.Fatalf("FetchDetailsByID returned error: %v", err)
	}
	if details == nil {
		t.Fatal("expected non-nil details")
	}
	if details.Name != "Mega Man" {
		t.Errorf("Name = %q, want %q", details.Name, "Mega Man")
	}
	if details.Summary != "A platformer." {
		t.Errorf("Summary = %q, want %q", details.Summary, "A platformer.")
	}
	if details.Storyline != "A robot fights evil." {
		t.Errorf("Storyline = %q, want %q", details.Storyline, "A robot fights evil.")
	}
	if details.FirstReleaseDate != "1987-12-08" {
		t.Errorf(
			"FirstReleaseDate = %q, want %q",
			details.FirstReleaseDate, "1987-12-08",
		)
	}
	if len(details.Platforms) != 1 || details.Platforms[0] != "NES" {
		t.Errorf("Platforms = %v, want [NES]", details.Platforms)
	}
	if len(details.Developers) != 1 || details.Developers[0] != "Capcom" {
		t.Errorf("Developers = %v, want [Capcom]", details.Developers)
	}
	if len(details.Publishers) != 1 || details.Publishers[0] != "Capcom" {
		t.Errorf("Publishers = %v, want [Capcom]", details.Publishers)
	}
	if details.IGDBURL != "https://www.igdb.com/games/mega-man" {
		t.Errorf(
			"IGDBURL = %q, want %q",
			details.IGDBURL, "https://www.igdb.com/games/mega-man",
		)
	}
	if details.Collection != "Mega Man" {
		t.Errorf("Collection = %q, want %q", details.Collection, "Mega Man")
	}
	if !strings.Contains(details.CoverURL, "t_original") {
		t.Errorf("CoverURL should use t_original, got %q", details.CoverURL)
	}
	if len(details.Screenshots) != 1 {
		t.Fatalf("Screenshots len = %d, want 1", len(details.Screenshots))
	}
	if !strings.Contains(details.Screenshots[0], "t_original") {
		t.Errorf("Screenshot URL should use t_original, got %q", details.Screenshots[0])
	}
	if len(details.Artworks) != 1 {
		t.Fatalf("Artworks len = %d, want 1", len(details.Artworks))
	}
	if !strings.Contains(details.Artworks[0], "t_original") {
		t.Errorf("Artwork URL should use t_original, got %q", details.Artworks[0])
	}
}

func TestFetchDetailsByIDNotFound(t *testing.T) {
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			_, _ = w.Write([]byte("[]"))
		}
	})

	details, err := f.FetchDetailsByID(999)
	if err != nil {
		t.Fatalf("FetchDetailsByID returned error: %v", err)
	}
	if details != nil {
		t.Errorf("expected nil details for empty response, got %+v", details)
	}
}

// TestAPIRequestRetryOn401 verifies that apiRequest clears the cached token
// and retries once when the games endpoint returns HTTP 401.
func TestAPIRequestRetryOn401(t *testing.T) {
	var gamesHits atomic.Int32

	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			n := gamesHits.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			resp, _ := json.Marshal([]map[string]any{
				{"id": 7, "name": "Contra"},
			})
			_, _ = w.Write(resp)
		}
	})

	id, err := f.SearchGame("Contra", nil)
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if id != 7 {
		t.Errorf("SearchGame() = %d, want 7", id)
	}
	if gamesHits.Load() != 2 {
		t.Errorf("expected 2 games endpoint hits, got %d", gamesHits.Load())
	}
}

// TestAPIRequestNon200Error verifies that a non-200, non-401 response from
// the games endpoint is surfaced as an error containing the status code.
func TestAPIRequestNon200Error(t *testing.T) {
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	_, err := f.SearchGame("Contra", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "IGDB returned 500") {
		t.Errorf("error should mention \"IGDB returned 500\", got: %v", err)
	}
}

// TestGetTokenCaching verifies that a valid cached token is reused across
// multiple calls so that only one OAuth request is made.
func TestGetTokenCaching(t *testing.T) {
	var tokenHits atomic.Int32

	searchResp, _ := json.Marshal([]map[string]any{
		{"id": 1, "name": "Tetris"},
	})
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			tokenHits.Add(1)
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			_, _ = w.Write(searchResp)
		}
	})

	if _, err := f.SearchGame("Tetris", nil); err != nil {
		t.Fatalf("first SearchGame error: %v", err)
	}
	if _, err := f.SearchGame("Tetris", nil); err != nil {
		t.Fatalf("second SearchGame error: %v", err)
	}
	if tokenHits.Load() != 1 {
		t.Errorf("expected 1 token request, got %d", tokenHits.Load())
	}
}

// TestGetTokenOAuthError verifies that an HTTP 400 from the token endpoint
// propagates as an error from SearchGame.
func TestGetTokenOAuthError(t *testing.T) {
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"invalid credentials"}`))
		}
	})

	_, err := f.SearchGame("Tetris", nil)
	if err == nil {
		t.Fatal("expected error from OAuth failure, got nil")
	}
	if !strings.Contains(err.Error(), "token request returned 400") {
		t.Errorf("error should mention \"token request returned 400\", got: %v", err)
	}
}

// TestGetTokenMalformedJSON verifies that malformed JSON from the token
// endpoint propagates as a parse error from SearchGame.
func TestGetTokenMalformedJSON(t *testing.T) {
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			_, _ = w.Write([]byte(`{invalid`))
		}
	})

	_, err := f.SearchGame("Tetris", nil)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

// TestSearchGameAPIError verifies that a 500 from the games endpoint
// propagates as an error from SearchGame.
func TestSearchGameAPIError(t *testing.T) {
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	_, err := f.SearchGame("Tetris", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestSearchGameMalformedJSON verifies that malformed JSON from the games
// endpoint propagates as a parse error from SearchGame.
func TestSearchGameMalformedJSON(t *testing.T) {
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			_, _ = w.Write([]byte(`{invalid`))
		}
	})

	_, err := f.SearchGame("Tetris", nil)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

// TestFetchDetailsByIDAPIError verifies that a 500 from the games endpoint
// propagates as an error from FetchDetailsByID.
func TestFetchDetailsByIDAPIError(t *testing.T) {
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	_, err := f.FetchDetailsByID(17)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFetchDetailsByIDMalformedJSON verifies that malformed JSON from the
// games endpoint propagates as a parse error from FetchDetailsByID.
func TestFetchDetailsByIDMalformedJSON(t *testing.T) {
	f := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			_, _ = w.Write([]byte(`{invalid`))
		}
	})

	_, err := f.FetchDetailsByID(17)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

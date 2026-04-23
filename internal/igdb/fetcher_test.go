package igdb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
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

// newTestFetcher creates a Fetcher whose HTTP client is wired to call
// handler instead of the real IGDB API. The test server is closed via
// t.Cleanup when the test completes.
func newTestFetcher(t *testing.T, handler http.HandlerFunc) *Fetcher {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	f := NewFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: ts.URL,
		},
	}
	return f
}

// newIGDBFetcher creates a Fetcher wired to a test server that routes
// /oauth2/token to writeTokenResponse, /v4/games to gamesHandler, and
// returns 404 for everything else.
func newIGDBFetcher(t *testing.T, gamesHandler http.HandlerFunc) *Fetcher {
	t.Helper()
	return newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			writeTokenResponse(w)
		case strings.HasSuffix(r.URL.Path, "/v4/games"):
			gamesHandler(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// newIGDBFetcherStatic creates a Fetcher whose /v4/games endpoint always
// writes body verbatim. Use for tests that only need a fixed JSON response.
func newIGDBFetcherStatic(t *testing.T, body []byte) *Fetcher {
	t.Helper()
	return newIGDBFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	})
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

		// Invariant: output is either empty (rejected) or a full HTTPS
		// URL whose host is exactly images.igdb.com. Any other shape
		// would indicate the SSRF mitigation is broken.
		if result != "" && !strings.HasPrefix(result, "https://"+IGDBImageHost+"/") {
			t.Errorf(
				"transformImageURL(%q, %q) = %q: want empty or https://%s/ prefix",
				rawURL, size, result, IGDBImageHost,
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

func FuzzNewFetcher(f *testing.F) {
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
				t.Errorf("NewFetcher panicked on input %q: %v", apiKey, r)
			}
		}()
		NewFetcher(apiKey)
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
	f := newIGDBFetcherStatic(t, searchResp)

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
	f := newIGDBFetcherStatic(t, searchResp)

	id, err := f.SearchGame("Mega Man", nil)
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if id != 0 {
		t.Errorf("SearchGame() = %d, want 0 (no exact match)", id)
	}
}

func TestStripDiacritics(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Déjà Vu", "Deja Vu"},
		{"Mega Man", "Mega Man"},
		{"Pokémon", "Pokemon"},
		{"", ""},
		{"Ōkami", "Okami"},
		{"Señor", "Senor"},
		{"naïve", "naive"},
	}
	for _, tt := range tests {
		got := stripDiacritics(tt.input)
		if got != tt.want {
			t.Errorf(
				"stripDiacritics(%q) = %q, want %q",
				tt.input, got, tt.want,
			)
		}
	}
}

func TestSearchGameDiacriticsMatch(t *testing.T) {
	searchResp, _ := json.Marshal([]map[string]any{
		{"id": 55, "name": "Déjà Vu"},
	})
	f := newIGDBFetcherStatic(t, searchResp)

	id, err := f.SearchGame("Deja Vu", nil)
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if id != 55 {
		t.Errorf("SearchGame() = %d, want 55 (diacritics match)", id)
	}
}

func TestSearchGamePlatformFallback(t *testing.T) {
	searchResp, _ := json.Marshal([]map[string]any{
		{"id": 77, "name": "Completely Different Title"},
	})
	f := newIGDBFetcherStatic(t, searchResp)

	// With platform IDs: falls back to first result
	id, err := f.SearchGame("Some Game", []int{18})
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if id != 77 {
		t.Errorf("SearchGame() = %d, want 77 (platform fallback)", id)
	}
}

func TestSearchGameNoPlatformNoFallback(t *testing.T) {
	searchResp, _ := json.Marshal([]map[string]any{
		{"id": 77, "name": "Completely Different Title"},
	})
	f := newIGDBFetcherStatic(t, searchResp)

	// Without platform IDs: no fallback, returns 0
	id, err := f.SearchGame("Some Game", nil)
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if id != 0 {
		t.Errorf("SearchGame() = %d, want 0 (no fallback without platform)", id)
	}
}

func TestSearchGameWithPlatformFilter(t *testing.T) {
	var capturedQuery string
	resp, _ := json.Marshal([]map[string]any{{"id": 18, "name": "Metroid"}})
	f := newIGDBFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		capturedQuery = string(body)
		_, _ = w.Write(resp)
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
	f := newIGDBFetcherStatic(t, detailsResp)

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
	f := newIGDBFetcherStatic(t, []byte("[]"))

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
	f := newIGDBFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
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
	f := newIGDBFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := f.SearchGame("Tetris", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestSearchGameMalformedJSON verifies that malformed JSON from the games
// endpoint propagates as a parse error from SearchGame.
func TestSearchGameMalformedJSON(t *testing.T) {
	f := newIGDBFetcherStatic(t, []byte(`{invalid`))

	_, err := f.SearchGame("Tetris", nil)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

// TestFetchDetailsByIDAPIError verifies that a 500 from the games endpoint
// propagates as an error from FetchDetailsByID.
func TestFetchDetailsByIDAPIError(t *testing.T) {
	f := newIGDBFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := f.FetchDetailsByID(17)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFetchDetailsByIDMalformedJSON verifies that malformed JSON from the
// games endpoint propagates as a parse error from FetchDetailsByID.
func TestFetchDetailsByIDMalformedJSON(t *testing.T) {
	f := newIGDBFetcherStatic(t, []byte(`{invalid`))

	_, err := f.FetchDetailsByID(17)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

// FuzzStripDiacritics verifies that stripDiacritics never panics and is
// idempotent: applying it twice produces the same result as applying it once.
func FuzzStripDiacritics(f *testing.F) {
	f.Add("Déjà Vu")
	f.Add("Mega Man")
	f.Add("Pokémon")
	f.Add("")
	f.Add("Ōkami")
	f.Add("\x00")
	f.Add("naïve café")

	f.Fuzz(func(t *testing.T, s string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("stripDiacritics panicked on %q: %v", s, r)
			}
		}()

		once := stripDiacritics(s)
		twice := stripDiacritics(once)
		if once != twice {
			t.Errorf(
				"stripDiacritics not idempotent on %q: first=%q second=%q",
				s, once, twice,
			)
		}
	})
}

func TestTransformImageURLRejectsUntrustedHosts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"protocol-relative igdb host", "//images.igdb.com/igdb/image/upload/t_thumb/abc.jpg", "https://images.igdb.com/igdb/image/upload/t_original/abc.jpg"},
		{"https igdb host", "https://images.igdb.com/igdb/image/upload/t_thumb/abc.jpg", "https://images.igdb.com/igdb/image/upload/t_original/abc.jpg"},
		{"http attacker ssrf", "http://127.0.0.1:9999/secret", ""},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data/", ""},
		{"file scheme", "file:///etc/passwd", ""},
		{"javascript scheme", "javascript:alert(1)", ""},
		{"protocol-relative attacker", "//attacker.example/foo.jpg", ""},
		{"https attacker host", "https://attacker.example/foo.jpg", ""},
		{"empty", "", ""},
		{"bare images host no path", "https://images.igdb.com", ""},
		{"host-prefix lookalike", "https://images.igdb.com.evil.example/x.jpg", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transformImageURL(tt.in, "t_original")
			if got != tt.want {
				t.Errorf("transformImageURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSafeIGDBInfoURLRejectsNonHTTPSIGDB(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"valid igdb info", "https://www.igdb.com/games/mega-man", "https://www.igdb.com/games/mega-man"},
		{"javascript xss", "javascript:alert(1)", ""},
		{"data uri", "data:text/html,<script>alert(1)</script>", ""},
		{"http attacker", "http://attacker.example/", ""},
		{"https attacker", "https://attacker.example/", ""},
		{"host-prefix lookalike", "https://www.igdb.com.evil.example/", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeIGDBInfoURL(tt.in)
			if got != tt.want {
				t.Errorf("safeIGDBInfoURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGameDetailsFromIGDBRejectsJavaScriptURL(t *testing.T) {
	g := igdbGame{
		Name: "X",
		URL:  "javascript:fetch('/api/saves')",
	}
	d := gameDetailsFromIGDB(g)
	if d.IGDBURL != "" {
		t.Errorf("IGDBURL = %q, want empty (javascript: rejected)", d.IGDBURL)
	}
}

func TestGameDetailsFromIGDBFiltersUntrustedImageHosts(t *testing.T) {
	g := igdbGame{Name: "X"}
	g.Cover.URL = "http://127.0.0.1:9999/secret"
	g.Screenshots = []struct {
		URL string `json:"url"`
	}{{URL: "//attacker.example/s.jpg"}, {URL: "//images.igdb.com/igdb/image/upload/t_thumb/s.jpg"}}
	g.Artworks = []struct {
		URL string `json:"url"`
	}{{URL: "file:///etc/passwd"}}

	d := gameDetailsFromIGDB(g)
	if d.CoverURL != "" {
		t.Errorf("CoverURL = %q, want empty (untrusted host rejected)", d.CoverURL)
	}
	if len(d.Screenshots) != 1 {
		t.Errorf("Screenshots = %v, want 1 (igdb-host) item", d.Screenshots)
	}
	if len(d.Artworks) != 0 {
		t.Errorf("Artworks = %v, want empty", d.Artworks)
	}
}

// FuzzGameDetailsFromIGDB verifies that gameDetailsFromIGDB never panics on
// arbitrary JSON-derived input. When FirstReleaseDate is populated, it must
// match the YYYY-MM-DD format.
func FuzzGameDetailsFromIGDB(f *testing.F) {
	f.Add([]byte(`[{"name":"Mega Man","first_release_date":565920000}]`))
	f.Add([]byte(`[{}]`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`[{"name":"","first_release_date":0}]`))
	f.Add([]byte(`[{"name":"Game","first_release_date":-1}]`))

	// Regexp for YYYY-MM-DD date validation.
	datePattern := `^\d{4}-\d{2}-\d{2}$`

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("gameDetailsFromIGDB panicked on %q: %v", data, r)
			}
		}()

		var games []igdbGame
		if err := json.Unmarshal(data, &games); err != nil || len(games) == 0 {
			return
		}

		details := gameDetailsFromIGDB(games[0])
		if details == nil {
			return
		}

		if details.FirstReleaseDate != "" {
			matched, err := regexp.MatchString(datePattern, details.FirstReleaseDate)
			if err != nil {
				t.Fatalf("regexp error: %v", err)
			}
			if !matched {
				t.Errorf(
					"FirstReleaseDate = %q does not match YYYY-MM-DD",
					details.FirstReleaseDate,
				)
			}
		}
	})
}

// FuzzSearchGame verifies that SearchGame never panics on arbitrary game name
// input when called against a test server that always returns an empty result.
func FuzzSearchGame(f *testing.F) {
	f.Add("Mega Man")
	f.Add("")
	f.Add(`"injected"`)
	f.Add("Game\x00Name")
	f.Add("A\nB")
	f.Add(strings.Repeat("x", 512))

	f.Fuzz(func(t *testing.T, gameName string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SearchGame panicked on %q: %v", gameName, r)
			}
		}()

		fetcher := newIGDBFetcherStatic(t, []byte("[]"))

		_, _ = fetcher.SearchGame(gameName, nil)
	})
}

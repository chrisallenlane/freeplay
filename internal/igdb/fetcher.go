package igdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Fetcher fetches game metadata from the IGDB API.
type Fetcher struct {
	clientID     string
	clientSecret string

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
	client      *http.Client
}

// NewFetcher creates an IGDB game metadata fetcher.
// apiKey should be in "client_id:client_secret" format.
func NewFetcher(apiKey string) *Fetcher {
	parts := strings.SplitN(apiKey, ":", 2)
	var clientID, clientSecret string
	if len(parts) == 2 {
		clientID = parts[0]
		clientSecret = parts[1]
	}
	return &Fetcher{
		clientID:     clientID,
		clientSecret: clientSecret,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (f *Fetcher) getToken() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.token != "" && time.Now().Before(f.tokenExpiry) {
		return f.token, nil
	}

	vals := url.Values{
		"client_id":     {f.clientID},
		"client_secret": {f.clientSecret},
		"grant_type":    {"client_credentials"},
	}

	resp, err := f.client.PostForm("https://id.twitch.tv/oauth2/token", vals)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}

	f.token = result.AccessToken
	f.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	return f.token, nil
}

func (f *Fetcher) apiRequest(endpoint, body string) ([]byte, error) {
	resp, err := f.doRequest(endpoint, body)
	if err != nil {
		return nil, err
	}

	// On 401, clear the cached token and retry once.
	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		f.mu.Lock()
		f.token = ""
		f.mu.Unlock()

		resp, err = f.doRequest(endpoint, body)
		if err != nil {
			return nil, err
		}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("IGDB returned %d: %s", resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	return data, err
}

// doRequest performs a single authenticated POST to the IGDB API.
func (f *Fetcher) doRequest(endpoint, body string) (*http.Response, error) {
	token, err := f.getToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		"POST",
		"https://api.igdb.com/v4/"+endpoint,
		strings.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", f.clientID)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IGDB request failed: %w", err)
	}
	return resp, nil
}

// GameDetails holds metadata fetched from IGDB for a game.
type GameDetails struct {
	Name             string     `json:"name"`
	Summary          string     `json:"summary,omitempty"`
	Storyline        string     `json:"storyline,omitempty"`
	FirstReleaseDate string     `json:"firstReleaseDate,omitempty"`
	Developers       []string   `json:"developers,omitempty"`
	Publishers       []string   `json:"publishers,omitempty"`
	Platforms        []string   `json:"platforms,omitempty"`
	Collection       string     `json:"collection,omitempty"`
	IGDBURL          string     `json:"igdbUrl,omitempty"`
	CoverURL         string     `json:"coverUrl,omitempty"`
	Screenshots      []ImageRef `json:"screenshots,omitempty"`
	Artworks         []ImageRef `json:"artworks,omitempty"`
}

// ImageRef pairs a full-size image URL with an optional gallery-sized
// thumbnail URL. ThumbURL is empty when no variant is cached (e.g.
// older details.json written before PERF-6); frontend callers should
// fall back to URL in that case.
type ImageRef struct {
	URL      string `json:"url"`
	ThumbURL string `json:"thumbUrl,omitempty"`
}

// UnmarshalJSON accepts either a bare string (legacy details.json
// shape: just a URL) or the current object shape. Keeps pre-PERF-6
// caches readable so a deploy doesn't force a full re-fetch.
func (r *ImageRef) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var u string
		if err := json.Unmarshal(data, &u); err != nil {
			return err
		}
		r.URL = u
		return nil
	}
	type rawImageRef ImageRef
	var raw rawImageRef
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = ImageRef(raw)
	return nil
}

// SearchGame searches IGDB for a game by name and returns the IGDB game ID
// of the best case-insensitive exact name match. Returns 0 if no match is
// found. When platformIDs is non-empty, results are filtered to those IDs.
func (f *Fetcher) SearchGame(gameName string, platformIDs []int) (int, error) {
	escaped := strings.ReplaceAll(gameName, `"`, `\"`)
	var query string
	if len(platformIDs) > 0 {
		ids := intsToStrings(platformIDs)
		query = fmt.Sprintf(
			`search "%s"; fields name; where platforms = (%s); limit 5;`,
			escaped,
			strings.Join(ids, ","),
		)
	} else {
		query = fmt.Sprintf(`search "%s"; fields name; limit 5;`, escaped)
	}

	data, err := f.apiRequest("games", query)
	if err != nil {
		return 0, err
	}

	var games []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &games); err != nil {
		return 0, fmt.Errorf("parsing game search: %w", err)
	}

	// Prefer exact case-insensitive match.
	for _, g := range games {
		if strings.EqualFold(g.Name, gameName) {
			return g.ID, nil
		}
	}

	// Try diacritics-insensitive match (e.g. "Déjà Vu" == "Deja Vu").
	normalizedSearch := stripDiacritics(gameName)
	for _, g := range games {
		if strings.EqualFold(stripDiacritics(g.Name), normalizedSearch) {
			return g.ID, nil
		}
	}

	// When platform-constrained, fall back to the first result. IGDB's
	// relevance ranking combined with the platform filter is usually
	// correct, and this covers cases where the IGDB title differs more
	// substantially from the ROM filename.
	if len(platformIDs) > 0 && len(games) > 0 {
		return games[0].ID, nil
	}

	return 0, nil
}

// FetchDetailsByID retrieves full game metadata from IGDB by game ID.
// Returns nil if the game is not found.
func (f *Fetcher) FetchDetailsByID(gameID int) (*GameDetails, error) {
	fields := `name, url, summary, storyline, first_release_date, cover.url, ` +
		`involved_companies.company.name, involved_companies.developer, ` +
		`involved_companies.publisher, platforms.name, screenshots.url, ` +
		`artworks.url, collection.name`
	query := fmt.Sprintf(`fields %s; where id = %d;`, fields, gameID)

	data, err := f.apiRequest("games", query)
	if err != nil {
		return nil, err
	}

	var games []igdbGame
	if err := json.Unmarshal(data, &games); err != nil {
		return nil, fmt.Errorf("parsing game details: %w", err)
	}
	if len(games) == 0 {
		return nil, nil
	}

	return gameDetailsFromIGDB(games[0]), nil
}

// igdbGame is the raw IGDB API response shape for a game record.
type igdbGame struct {
	Name             string `json:"name"`
	URL              string `json:"url"`
	Summary          string `json:"summary"`
	Storyline        string `json:"storyline"`
	FirstReleaseDate int64  `json:"first_release_date"`
	Cover            struct {
		URL string `json:"url"`
	} `json:"cover"`
	InvolvedCompanies []struct {
		Company struct {
			Name string `json:"name"`
		} `json:"company"`
		Developer bool `json:"developer"`
		Publisher bool `json:"publisher"`
	} `json:"involved_companies"`
	Platforms []struct {
		Name string `json:"name"`
	} `json:"platforms"`
	Screenshots []struct {
		URL string `json:"url"`
	} `json:"screenshots"`
	Artworks []struct {
		URL string `json:"url"`
	} `json:"artworks"`
	Collection struct {
		Name string `json:"name"`
	} `json:"collection"`
}

func gameDetailsFromIGDB(g igdbGame) *GameDetails {
	details := &GameDetails{
		Name:      g.Name,
		IGDBURL:   safeIGDBInfoURL(g.URL),
		Summary:   g.Summary,
		Storyline: g.Storyline,
	}

	if g.Cover.URL != "" {
		details.CoverURL = transformImageURL(g.Cover.URL, "t_original")
	}

	if g.FirstReleaseDate > 0 {
		details.FirstReleaseDate = time.Unix(
			g.FirstReleaseDate, 0,
		).UTC().Format("2006-01-02")
	}

	for _, ic := range g.InvolvedCompanies {
		if ic.Developer {
			details.Developers = append(details.Developers, ic.Company.Name)
		}
		if ic.Publisher {
			details.Publishers = append(details.Publishers, ic.Company.Name)
		}
	}

	for _, p := range g.Platforms {
		details.Platforms = append(details.Platforms, p.Name)
	}

	if g.Collection.Name != "" {
		details.Collection = g.Collection.Name
	}

	for _, s := range g.Screenshots {
		if ref := imageRefFrom(s.URL, "t_screenshot_huge"); ref.URL != "" {
			details.Screenshots = append(details.Screenshots, ref)
		}
	}

	for _, a := range g.Artworks {
		if ref := imageRefFrom(a.URL, "t_screenshot_huge"); ref.URL != "" {
			details.Artworks = append(details.Artworks, ref)
		}
	}

	return details
}

// imageRefFrom builds an ImageRef with the full-size (t_original) URL and
// a gallery-sized thumbnail URL for inline rendering. Returns a zero
// ImageRef if the source URL is not a valid images.igdb.com URL; callers
// must check URL != "" before appending.
func imageRefFrom(rawURL, thumbSize string) ImageRef {
	return ImageRef{
		URL:      transformImageURL(rawURL, "t_original"),
		ThumbURL: transformImageURL(rawURL, thumbSize),
	}
}

// IGDBImageHost is the only host we accept for image URLs returned by
// the IGDB API. Exported so other packages (e.g. details.Cache) can
// enforce cross-host redirect blocking.
const IGDBImageHost = "images.igdb.com"

// igdbInfoURLPrefix is the canonical IGDB info-page URL prefix.
// IGDB game.url fields point here.
const igdbInfoURLPrefix = "https://www.igdb.com/"

// transformImageURL normalizes an IGDB image URL and rewrites its size
// slug. Returns "" if the input is not a valid images.igdb.com URL —
// callers MUST skip images for which this returns "". This is the
// server-side trust-boundary check for IGDB SSRF (see SEC-2).
func transformImageURL(u, size string) string {
	var canonical string
	switch {
	case strings.HasPrefix(u, "//"+IGDBImageHost+"/"):
		canonical = "https:" + u
	case strings.HasPrefix(u, "https://"+IGDBImageHost+"/"):
		canonical = u
	default:
		return ""
	}
	return strings.Replace(canonical, "t_thumb", size, 1)
}

// safeIGDBInfoURL returns u if it is a valid IGDB info-page URL, or ""
// otherwise. Prevents javascript:/data:/http: URLs from reaching the
// frontend via the "View on IGDB" link (see SEC-2 / H-2).
func safeIGDBInfoURL(u string) string {
	if strings.HasPrefix(u, igdbInfoURLPrefix) {
		return u
	}
	return ""
}

// stripDiacritics removes diacritical marks from a string using Unicode
// NFD decomposition. For example, "Déjà Vu" becomes "Deja Vu".
func stripDiacritics(s string) string {
	result, _, _ := transform.String(
		transform.Chain(
			norm.NFD,
			runes.Remove(runes.In(unicode.Mn)),
			norm.NFC,
		),
		s,
	)
	return result
}

func intsToStrings(ids []int) []string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.Itoa(id)
	}
	return s
}

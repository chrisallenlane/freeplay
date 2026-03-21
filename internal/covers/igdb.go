package covers

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
)

// IGDBFetcher fetches cover art from the IGDB API.
type IGDBFetcher struct {
	clientID     string
	clientSecret string

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
	client      *http.Client
}

// NewIGDBFetcher creates an IGDB cover art fetcher.
// apiKey should be in "client_id:client_secret" format.
func NewIGDBFetcher(apiKey string) *IGDBFetcher {
	parts := strings.SplitN(apiKey, ":", 2)
	var clientID, clientSecret string
	if len(parts) == 2 {
		clientID = parts[0]
		clientSecret = parts[1]
	}
	return &IGDBFetcher{
		clientID:     clientID,
		clientSecret: clientSecret,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (f *IGDBFetcher) getToken() (string, error) {
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

func (f *IGDBFetcher) apiRequest(endpoint, body string) ([]byte, error) {
	for attempt := range 2 {
		token, err := f.getToken()
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest("POST", "https://api.igdb.com/v4/"+endpoint, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Client-ID", f.clientID)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := f.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("IGDB request failed: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			// Token expired, clear and retry once
			f.mu.Lock()
			f.token = ""
			f.mu.Unlock()
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("IGDB returned %d: %s", resp.StatusCode, string(respBody))
		}

		return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	}
	// Unreachable: loop always returns or continues
	return nil, fmt.Errorf("IGDB request failed after retry")
}

// GameDetails holds metadata fetched from IGDB for a game.
type GameDetails struct {
	Name             string   `json:"name"`
	Summary          string   `json:"summary,omitempty"`
	Storyline        string   `json:"storyline,omitempty"`
	FirstReleaseDate string   `json:"firstReleaseDate,omitempty"`
	Developers       []string `json:"developers,omitempty"`
	Publishers       []string `json:"publishers,omitempty"`
	Platforms        []string `json:"platforms,omitempty"`
	Collection       string   `json:"collection,omitempty"`
	IGDBURL          string   `json:"igdbUrl,omitempty"`
	CoverURL         string   `json:"coverUrl,omitempty"`
	Screenshots      []string `json:"screenshots,omitempty"`
	Artworks         []string `json:"artworks,omitempty"`
}

// SearchGame searches IGDB for a game by name and returns the IGDB game ID
// of the best case-insensitive exact name match. Returns 0 if no match is
// found. When platformIDs is non-empty, results are filtered to those IDs.
func (f *IGDBFetcher) SearchGame(gameName string, platformIDs []int) (int, error) {
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

	for _, g := range games {
		if strings.EqualFold(g.Name, gameName) {
			return g.ID, nil
		}
	}
	return 0, nil
}

// FetchDetailsByID retrieves full game metadata from IGDB by game ID.
// Returns nil if the game is not found.
func (f *IGDBFetcher) FetchDetailsByID(gameID int) (*GameDetails, error) {
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
		IGDBURL:   g.URL,
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
		details.Screenshots = append(
			details.Screenshots, transformImageURL(s.URL, "t_original"),
		)
	}

	for _, a := range g.Artworks {
		details.Artworks = append(
			details.Artworks, transformImageURL(a.URL, "t_original"),
		)
	}

	return details
}

// transformImageURL prepends https and replaces the size template in an IGDB image URL.
func transformImageURL(rawURL, size string) string {
	u := rawURL
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	return strings.Replace(u, "t_thumb", size, 1)
}

func intsToStrings(ids []int) []string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.Itoa(id)
	}
	return s
}

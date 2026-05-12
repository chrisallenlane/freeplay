package igdb

import (
	"encoding/json"
	"time"
)

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

// gameDetailsFromIGDB maps the raw IGDB API response shape into the
// public GameDetails value. Filters IGDB URLs through the safety
// checks and builds gallery-thumbnail ImageRefs alongside originals.
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
		// IGDB occasionally returns involved_companies rows whose company
		// subobject is absent or unnamed; skip them so the frontend
		// doesn't render leading-comma artifacts.
		if ic.Company.Name == "" {
			continue
		}
		if ic.Developer {
			details.Developers = append(details.Developers, ic.Company.Name)
		}
		if ic.Publisher {
			details.Publishers = append(details.Publishers, ic.Company.Name)
		}
	}

	for _, p := range g.Platforms {
		if p.Name == "" {
			continue
		}
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

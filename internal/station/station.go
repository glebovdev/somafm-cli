// Package station defines the data structures for SomaFM radio stations.
package station

import (
	"sort"
	"strings"
)

// Playlist represents a streaming endpoint for a radio station.
type Playlist struct {
	URL     string `json:"url"`
	Format  string `json:"format"`  // Audio format (e.g., "mp3", "aac")
	Quality string `json:"quality"` // Quality level (e.g., "highest", "high")
}

// Station represents a SomaFM radio station with its metadata and streaming options.
type Station struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DJ          string     `json:"dj"`
	DJMail      string     `json:"djmail"`
	Genre       string     `json:"genre"` // Pipe-separated genre list
	Image       string     `json:"image"`
	LargeImage  string     `json:"largeimage"`
	XLImage     string     `json:"xlimage"`
	Twitter     string     `json:"twitter"`
	Updated     string     `json:"updated"`
	Playlists   []Playlist `json:"playlists"`
	Preroll     []string   `json:"preroll"`
	Listeners   string     `json:"listeners"`
	LastPlaying string     `json:"lastPlaying"`
}

// GetBestPlaylistURL returns the URL of the highest quality MP3 playlist.
// Falls back to the first available playlist if no MP3 "highest" quality is found.
func (s *Station) GetBestPlaylistURL() string {
	for _, playlist := range s.Playlists {
		if playlist.Format == "mp3" && playlist.Quality == "highest" {
			return playlist.URL
		}
	}
	if len(s.Playlists) > 0 {
		return s.Playlists[0].URL
	}
	return ""
}

// GetPlaylistURLs returns all playlist URLs sorted by preference.
// The preferred format is ordered first (highest quality first), then the
// alternate known format, then any others.
func (s *Station) GetPlaylistURLs(preferredFormat string) []string {
	preferred := strings.ToLower(strings.TrimSpace(preferredFormat))
	if preferred != "aac" {
		preferred = "mp3"
	}
	secondary := "aac"
	if preferred == "aac" {
		secondary = "mp3"
	}

	// Build a priority for ordering: lower = better.
	priority := func(pl Playlist) int {
		format := strings.ToLower(strings.TrimSpace(pl.Format))
		qualityBonus := 0
		if pl.Quality == "highest" {
			qualityBonus = 1
		}
		switch {
		case format == preferred:
			return 0 - qualityBonus // 0 or -1 (highest first)
		case format == secondary:
			return 2 - qualityBonus // 2 or 1
		default:
			return 4
		}
	}

	sorted := make([]Playlist, len(s.Playlists))
	copy(sorted, s.Playlists)
	sort.SliceStable(sorted, func(i, j int) bool {
		return priority(sorted[i]) < priority(sorted[j])
	})

	result := make([]string, len(sorted))
	for i, pl := range sorted {
		result[i] = pl.URL
	}
	return result
}

// GetAllPlaylistURLs returns all playlist URLs sorted by preference:
// MP3 highest quality first, then other MP3, then other formats.
func (s *Station) GetAllPlaylistURLs() []string {
	return s.GetPlaylistURLs("mp3")
}

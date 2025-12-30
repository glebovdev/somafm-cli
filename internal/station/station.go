// Package station defines the data structures for SomaFM radio stations.
package station

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

// GetAllPlaylistURLs returns all playlist URLs sorted by preference:
// MP3 highest quality first, then other MP3, then other formats.
func (s *Station) GetAllPlaylistURLs() []string {
	var mp3Highest, mp3Other, other []string

	for _, playlist := range s.Playlists {
		if playlist.Format == "mp3" {
			if playlist.Quality == "highest" {
				mp3Highest = append(mp3Highest, playlist.URL)
			} else {
				mp3Other = append(mp3Other, playlist.URL)
			}
		} else {
			other = append(other, playlist.URL)
		}
	}

	result := make([]string, 0, len(s.Playlists))
	result = append(result, mp3Highest...)
	result = append(result, mp3Other...)
	result = append(result, other...)

	return result
}

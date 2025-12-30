package station

import (
	"testing"
)

func TestGetBestPlaylistURL(t *testing.T) {
	tests := []struct {
		name     string
		station  Station
		expected string
	}{
		{
			name: "Returns mp3 highest quality",
			station: Station{
				Playlists: []Playlist{
					{URL: "http://example.com/low.pls", Format: "mp3", Quality: "low"},
					{URL: "http://example.com/high.pls", Format: "mp3", Quality: "highest"},
					{URL: "http://example.com/med.pls", Format: "aac", Quality: "high"},
				},
			},
			expected: "http://example.com/high.pls",
		},
		{
			name: "Returns first playlist when no mp3 highest",
			station: Station{
				Playlists: []Playlist{
					{URL: "http://example.com/first.pls", Format: "aac", Quality: "high"},
					{URL: "http://example.com/second.pls", Format: "mp3", Quality: "low"},
				},
			},
			expected: "http://example.com/first.pls",
		},
		{
			name: "Returns empty string when no playlists",
			station: Station{
				Playlists: []Playlist{},
			},
			expected: "",
		},
		{
			name: "Returns single playlist",
			station: Station{
				Playlists: []Playlist{
					{URL: "http://example.com/only.pls", Format: "mp3", Quality: "high"},
				},
			},
			expected: "http://example.com/only.pls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.station.GetBestPlaylistURL()
			if result != tt.expected {
				t.Errorf("GetBestPlaylistURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetAllPlaylistURLs(t *testing.T) {
	tests := []struct {
		name     string
		station  Station
		expected []string
	}{
		{
			name: "Prioritizes mp3 highest, then mp3 other, then other formats",
			station: Station{
				Playlists: []Playlist{
					{URL: "http://example.com/aac-high.pls", Format: "aac", Quality: "high"},
					{URL: "http://example.com/mp3-low.pls", Format: "mp3", Quality: "low"},
					{URL: "http://example.com/mp3-highest.pls", Format: "mp3", Quality: "highest"},
					{URL: "http://example.com/ogg-med.pls", Format: "ogg", Quality: "medium"},
				},
			},
			expected: []string{
				"http://example.com/mp3-highest.pls",
				"http://example.com/mp3-low.pls",
				"http://example.com/aac-high.pls",
				"http://example.com/ogg-med.pls",
			},
		},
		{
			name: "Multiple mp3 highest quality",
			station: Station{
				Playlists: []Playlist{
					{URL: "http://example.com/mp3-highest-1.pls", Format: "mp3", Quality: "highest"},
					{URL: "http://example.com/mp3-highest-2.pls", Format: "mp3", Quality: "highest"},
				},
			},
			expected: []string{
				"http://example.com/mp3-highest-1.pls",
				"http://example.com/mp3-highest-2.pls",
			},
		},
		{
			name: "Only non-mp3 formats",
			station: Station{
				Playlists: []Playlist{
					{URL: "http://example.com/aac.pls", Format: "aac", Quality: "high"},
					{URL: "http://example.com/ogg.pls", Format: "ogg", Quality: "medium"},
				},
			},
			expected: []string{
				"http://example.com/aac.pls",
				"http://example.com/ogg.pls",
			},
		},
		{
			name: "Empty playlists",
			station: Station{
				Playlists: []Playlist{},
			},
			expected: []string{},
		},
		{
			name: "Single playlist",
			station: Station{
				Playlists: []Playlist{
					{URL: "http://example.com/only.pls", Format: "mp3", Quality: "high"},
				},
			},
			expected: []string{"http://example.com/only.pls"},
		},
		{
			name: "Mp3 non-highest only",
			station: Station{
				Playlists: []Playlist{
					{URL: "http://example.com/mp3-low.pls", Format: "mp3", Quality: "low"},
					{URL: "http://example.com/mp3-med.pls", Format: "mp3", Quality: "medium"},
				},
			},
			expected: []string{
				"http://example.com/mp3-low.pls",
				"http://example.com/mp3-med.pls",
			},
		},
		{
			name: "Mixed with nil playlists field results in empty",
			station: Station{
				Playlists: nil,
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.station.GetAllPlaylistURLs()

			if len(result) != len(tt.expected) {
				t.Fatalf("GetAllPlaylistURLs() returned %d items, want %d: got %v", len(result), len(tt.expected), result)
			}

			for i, url := range result {
				if url != tt.expected[i] {
					t.Errorf("GetAllPlaylistURLs()[%d] = %q, want %q", i, url, tt.expected[i])
				}
			}
		})
	}
}

func TestStationFields(t *testing.T) {
	station := Station{
		ID:          "groovesalad",
		Title:       "Groove Salad",
		Description: "A nicely chilled plate of ambient/downtempo beats and grooves.",
		DJ:          "Rusty Hodge",
		Genre:       "ambient|electronica",
		Listeners:   "1234",
		LastPlaying: "Artist - Song Title",
	}

	if station.ID != "groovesalad" {
		t.Errorf("Station.ID = %q, want %q", station.ID, "groovesalad")
	}
	if station.Title != "Groove Salad" {
		t.Errorf("Station.Title = %q, want %q", station.Title, "Groove Salad")
	}
	if station.Listeners != "1234" {
		t.Errorf("Station.Listeners = %q, want %q", station.Listeners, "1234")
	}
}

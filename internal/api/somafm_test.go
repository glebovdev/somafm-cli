package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebovdev/somafm-cli/internal/station"
	"github.com/go-resty/resty/v2"
)

func setupTestServer(handler http.HandlerFunc) (*httptest.Server, *SomaFMClient) {
	server := httptest.NewServer(handler)
	client := &SomaFMClient{
		client: resty.New().SetBaseURL(server.URL),
	}
	return server, client
}

func TestGetStations(t *testing.T) {
	expectedStations := []station.Station{
		{
			ID:        "groovesalad",
			Title:     "Groove Salad",
			Listeners: "1000",
			Playlists: []station.Playlist{
				{URL: "http://example.com/stream.pls", Format: "mp3", Quality: "highest"},
			},
		},
		{
			ID:        "dronezone",
			Title:     "Drone Zone",
			Listeners: "500",
		},
	}

	server, client := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels.json" {
			t.Errorf("Expected path /channels.json, got %s", r.URL.Path)
		}

		response := struct {
			Channels []station.Station `json:"channels"`
		}{
			Channels: expectedStations,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	stations, err := client.GetStations()
	if err != nil {
		t.Fatalf("GetStations() error = %v", err)
	}

	if len(stations) != len(expectedStations) {
		t.Fatalf("GetStations() returned %d stations, want %d", len(stations), len(expectedStations))
	}

	for i, st := range stations {
		if st.ID != expectedStations[i].ID {
			t.Errorf("stations[%d].ID = %q, want %q", i, st.ID, expectedStations[i].ID)
		}
		if st.Title != expectedStations[i].Title {
			t.Errorf("stations[%d].Title = %q, want %q", i, st.Title, expectedStations[i].Title)
		}
	}
}

func TestGetStationsEmptyResponse(t *testing.T) {
	server, client := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		response := struct {
			Channels []station.Station `json:"channels"`
		}{
			Channels: []station.Station{},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	stations, err := client.GetStations()
	if err != nil {
		t.Fatalf("GetStations() error = %v", err)
	}

	if len(stations) != 0 {
		t.Errorf("GetStations() returned %d stations, want 0", len(stations))
	}
}

func TestGetStationsInvalidJSON(t *testing.T) {
	server, client := setupTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	})
	defer server.Close()

	_, err := client.GetStations()
	if err == nil {
		t.Error("GetStations() should return error for invalid JSON")
	}
}

func TestGetRecentSongs(t *testing.T) {
	expectedSongs := &SongsResponse{
		ID: "groovesalad",
		Songs: []SongInfo{
			{Title: "Song 1", Artist: "Artist 1", Album: "Album 1"},
			{Title: "Song 2", Artist: "Artist 2", Album: "Album 2"},
		},
	}

	server, client := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/songs/groovesalad.json"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expectedSongs)
	})
	defer server.Close()

	songs, err := client.GetRecentSongs("groovesalad")
	if err != nil {
		t.Fatalf("GetRecentSongs() error = %v", err)
	}

	if songs.ID != expectedSongs.ID {
		t.Errorf("GetRecentSongs().ID = %q, want %q", songs.ID, expectedSongs.ID)
	}

	if len(songs.Songs) != len(expectedSongs.Songs) {
		t.Fatalf("GetRecentSongs() returned %d songs, want %d", len(songs.Songs), len(expectedSongs.Songs))
	}

	for i, song := range songs.Songs {
		if song.Title != expectedSongs.Songs[i].Title {
			t.Errorf("songs[%d].Title = %q, want %q", i, song.Title, expectedSongs.Songs[i].Title)
		}
		if song.Artist != expectedSongs.Songs[i].Artist {
			t.Errorf("songs[%d].Artist = %q, want %q", i, song.Artist, expectedSongs.Songs[i].Artist)
		}
	}
}

func TestGetRecentSongsEmpty(t *testing.T) {
	server, client := setupTestServer(func(w http.ResponseWriter, _ *http.Request) {
		response := SongsResponse{
			ID:    "groovesalad",
			Songs: []SongInfo{},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	songs, err := client.GetRecentSongs("groovesalad")
	if err != nil {
		t.Fatalf("GetRecentSongs() error = %v", err)
	}

	if len(songs.Songs) != 0 {
		t.Errorf("GetRecentSongs() returned %d songs, want 0", len(songs.Songs))
	}
}

func TestGetCurrentTrackForStation(t *testing.T) {
	tests := []struct {
		name     string
		songs    []SongInfo
		expected string
	}{
		{
			name: "artist and title",
			songs: []SongInfo{
				{Artist: "The Beatles", Title: "Hey Jude"},
			},
			expected: "The Beatles - Hey Jude",
		},
		{
			name: "title only",
			songs: []SongInfo{
				{Title: "Unknown Track"},
			},
			expected: "Unknown Track",
		},
		{
			name: "artist only",
			songs: []SongInfo{
				{Artist: "Mystery Artist"},
			},
			expected: "",
		},
		{
			name:     "empty songs",
			songs:    []SongInfo{},
			expected: "",
		},
		{
			name: "multiple songs returns first",
			songs: []SongInfo{
				{Artist: "First", Title: "Song"},
				{Artist: "Second", Title: "Song"},
			},
			expected: "First - Song",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, client := setupTestServer(func(w http.ResponseWriter, _ *http.Request) {
				response := SongsResponse{
					ID:    "teststation",
					Songs: tt.songs,
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			})
			defer server.Close()

			result, err := client.GetCurrentTrackForStation("teststation")
			if err != nil {
				t.Fatalf("GetCurrentTrackForStation() error = %v", err)
			}

			if result != tt.expected {
				t.Errorf("GetCurrentTrackForStation() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestNewSomaFMClient(t *testing.T) {
	client := NewSomaFMClient()

	if client == nil {
		t.Fatal("NewSomaFMClient() returned nil")
	}

	if client.client == nil {
		t.Error("NewSomaFMClient() client.client is nil")
	}
}

func TestSongInfoFields(t *testing.T) {
	song := SongInfo{
		Title:  "Test Title",
		Artist: "Test Artist",
		Album:  "Test Album",
		Date:   "2024-01-01",
	}

	if song.Title != "Test Title" {
		t.Errorf("SongInfo.Title = %q, want %q", song.Title, "Test Title")
	}
	if song.Artist != "Test Artist" {
		t.Errorf("SongInfo.Artist = %q, want %q", song.Artist, "Test Artist")
	}
	if song.Album != "Test Album" {
		t.Errorf("SongInfo.Album = %q, want %q", song.Album, "Test Album")
	}
	if song.Date != "2024-01-01" {
		t.Errorf("SongInfo.Date = %q, want %q", song.Date, "2024-01-01")
	}
}

func TestSongsResponseFields(t *testing.T) {
	response := SongsResponse{
		ID: "teststation",
		Songs: []SongInfo{
			{Title: "Song 1"},
			{Title: "Song 2"},
		},
	}

	if response.ID != "teststation" {
		t.Errorf("SongsResponse.ID = %q, want %q", response.ID, "teststation")
	}
	if len(response.Songs) != 2 {
		t.Errorf("SongsResponse.Songs length = %d, want 2", len(response.Songs))
	}
}

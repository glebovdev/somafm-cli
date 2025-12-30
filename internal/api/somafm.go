// Package api provides the HTTP client for the SomaFM API.
package api

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/glebovdev/somafm-cli/internal/station"
	"github.com/go-resty/resty/v2"
)

const (
	baseURL        = "https://api.somafm.com"
	requestTimeout = 30 * time.Second
)

// SomaFMClient is the HTTP client for interacting with the SomaFM API.
type SomaFMClient struct {
	client *resty.Client
}

// NewSomaFMClient creates a new SomaFM API client with sensible defaults.
func NewSomaFMClient() *SomaFMClient {
	return &SomaFMClient{
		client: resty.New().
			SetBaseURL(baseURL).
			SetTimeout(requestTimeout),
	}
}

// GetStations fetches the list of available radio stations from the SomaFM API.
func (c *SomaFMClient) GetStations() ([]station.Station, error) {
	resp, err := c.client.R().Get("/channels.json")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stations: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, fmt.Errorf("api returned status %d: %s", resp.StatusCode(), resp.Status())
	}

	var response struct {
		Channels []station.Station `json:"channels"`
	}

	if err := json.Unmarshal(resp.Body(), &response); err != nil {
		return nil, fmt.Errorf("failed to parse stations response: %w", err)
	}

	return response.Channels, nil
}

type SongInfo struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
	Date   string `json:"date"`
}

type SongsResponse struct {
	ID    string     `json:"id"`
	Songs []SongInfo `json:"songs"`
}

// GetRecentSongs fetches the recent song history for a specific station.
func (c *SomaFMClient) GetRecentSongs(stationID string) (*SongsResponse, error) {
	resp, err := c.client.R().Get(fmt.Sprintf("/songs/%s.json", stationID))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch songs for station %s: %w", stationID, err)
	}

	if !resp.IsSuccess() {
		return nil, fmt.Errorf("api returned status %d: %s", resp.StatusCode(), resp.Status())
	}

	var response SongsResponse
	if err := json.Unmarshal(resp.Body(), &response); err != nil {
		return nil, fmt.Errorf("failed to parse songs response: %w", err)
	}

	return &response, nil
}

func (c *SomaFMClient) GetCurrentTrackForStation(stationID string) (string, error) {
	songs, err := c.GetRecentSongs(stationID)
	if err != nil {
		return "", err
	}

	if len(songs.Songs) == 0 {
		return "", nil
	}

	song := songs.Songs[0]
	if song.Artist != "" && song.Title != "" {
		return fmt.Sprintf("%s - %s", song.Artist, song.Title), nil
	}
	if song.Title != "" {
		return song.Title, nil
	}
	return "", nil
}

// Package service provides the business logic layer for managing station data.
package service

import (
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/glebovdev/somafm-cli/internal/api"
	"github.com/glebovdev/somafm-cli/internal/cache"
	"github.com/glebovdev/somafm-cli/internal/station"
	"github.com/rs/zerolog/log"
)

const imageLoadTimeout = 15 * time.Second

// StationService manages station data, including fetching, caching, and periodic refresh.
type StationService struct {
	apiClient     *api.SomaFMClient
	stations      []station.Station
	mu            sync.RWMutex
	imageCache    *cache.Cache
	refreshTicker *time.Ticker
	stopRefresh   chan struct{}
	onRefresh     func([]station.Station)
}

// NewStationService creates a new StationService with the given API client.
func NewStationService(apiClient *api.SomaFMClient) *StationService {
	imageCache, err := cache.NewCache()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to initialize image cache, images will not be cached")
	}

	if imageCache != nil {
		go func() {
			if err := imageCache.CleanExpired(); err != nil {
				log.Debug().Err(err).Msg("Failed to clean expired cache")
			}
		}()
	}

	return &StationService{
		apiClient:  apiClient,
		imageCache: imageCache,
	}
}

func (s *StationService) GetStations() ([]station.Station, error) {
	stations, err := s.apiClient.GetStations()
	if err != nil {
		return nil, err
	}

	s.sortStationsByListeners(stations)

	s.mu.Lock()
	s.stations = stations
	s.mu.Unlock()

	return stations, nil
}

func (s *StationService) GetCachedStations() []station.Station {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]station.Station, len(s.stations))
	copy(result, s.stations)
	return result
}

func (s *StationService) sortStationsByListeners(stations []station.Station) {
	sort.Slice(stations, func(i, j int) bool {
		listenersI, errI := strconv.Atoi(stations[i].Listeners)
		listenersJ, errJ := strconv.Atoi(stations[j].Listeners)

		if errI != nil {
			return false
		}
		if errJ != nil {
			return true
		}

		return listenersI > listenersJ
	})
}

func (s *StationService) GetValidStationIDs() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	validIDs := make(map[string]bool)
	for _, st := range s.stations {
		validIDs[st.ID] = true
	}
	return validIDs
}

func (s *StationService) FindIndexByID(stationID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i, st := range s.stations {
		if st.ID == stationID {
			return i
		}
	}
	return -1
}

func (s *StationService) StationCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.stations)
}

// GetStation returns a copy of the station at the given index.
// Returns nil if the index is out of bounds.
// The returned station is a copy to prevent invalidation when the internal slice is refreshed.
func (s *StationService) GetStation(index int) *station.Station {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if index < 0 || index >= len(s.stations) {
		return nil
	}
	// Return a copy to prevent pointer invalidation on slice refresh
	st := s.stations[index]
	return &st
}

func (s *StationService) LoadImage(url string) (image.Image, error) {
	if s.imageCache != nil {
		if img := s.imageCache.GetImage(url); img != nil {
			log.Debug().Str("url", url).Msg("Image loaded from cache")
			return img, nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), imageLoadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, err
	}

	if s.imageCache != nil {
		go func() {
			if err := s.imageCache.SaveImage(url, img); err != nil {
				log.Debug().Err(err).Str("url", url).Msg("Failed to cache image")
			} else {
				log.Debug().Str("url", url).Msg("Image cached")
			}
		}()
	}

	return img, nil
}

func (s *StationService) GetCurrentTrackForStation(stationID string) (string, error) {
	return s.apiClient.GetCurrentTrackForStation(stationID)
}

func (s *StationService) StartPeriodicRefresh(interval time.Duration, callback func([]station.Station)) {
	s.StopPeriodicRefresh()

	s.mu.Lock()
	s.onRefresh = callback
	s.stopRefresh = make(chan struct{})
	s.refreshTicker = time.NewTicker(interval)
	ticker := s.refreshTicker
	stopCh := s.stopRefresh
	s.mu.Unlock()

	go func() {
		for {
			select {
			case <-ticker.C:
				s.refreshStationsInBackground()
			case <-stopCh:
				ticker.Stop()
				return
			}
		}
	}()

	log.Debug().Dur("interval", interval).Msg("Started periodic station refresh")
}

func (s *StationService) StopPeriodicRefresh() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopRefresh != nil {
		close(s.stopRefresh)
		s.stopRefresh = nil
	}
	log.Debug().Msg("Stopped periodic station refresh")
}

func (s *StationService) refreshStationsInBackground() {
	newStations, err := s.apiClient.GetStations()
	if err != nil {
		log.Warn().Err(err).Msg("Background refresh failed, keeping cached data")
		return
	}

	s.sortStationsByListeners(newStations)

	s.mu.Lock()
	s.stations = newStations
	callback := s.onRefresh
	s.mu.Unlock()

	if callback != nil {
		callback(newStations)
	}

	log.Debug().Int("count", len(newStations)).Msg("Station data refreshed in background")
}

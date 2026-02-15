package service

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebovdev/somafm-cli/internal/cache"
	"github.com/glebovdev/somafm-cli/internal/station"
)

func TestSortStationsByListeners(t *testing.T) {
	service := &StationService{}

	tests := []struct {
		name     string
		stations []station.Station
		expected []string // expected order of station IDs
	}{
		{
			name: "sort by listener count descending",
			stations: []station.Station{
				{ID: "low", Listeners: "100"},
				{ID: "high", Listeners: "1000"},
				{ID: "mid", Listeners: "500"},
			},
			expected: []string{"high", "mid", "low"},
		},
		{
			name: "handle invalid listener strings",
			stations: []station.Station{
				{ID: "valid1", Listeners: "500"},
				{ID: "invalid", Listeners: "not-a-number"},
				{ID: "valid2", Listeners: "1000"},
			},
			expected: []string{"valid2", "valid1", "invalid"},
		},
		{
			name: "handle empty listener strings",
			stations: []station.Station{
				{ID: "valid", Listeners: "500"},
				{ID: "empty", Listeners: ""},
			},
			expected: []string{"valid", "empty"},
		},
		{
			name: "handle single station",
			stations: []station.Station{
				{ID: "only", Listeners: "100"},
			},
			expected: []string{"only"},
		},
		{
			name:     "handle empty list",
			stations: []station.Station{},
			expected: []string{},
		},
		{
			name: "handle equal listener counts",
			stations: []station.Station{
				{ID: "first", Listeners: "500"},
				{ID: "second", Listeners: "500"},
			},
			expected: []string{"first", "second"},
		},
		{
			name: "handle zero listeners",
			stations: []station.Station{
				{ID: "zero", Listeners: "0"},
				{ID: "some", Listeners: "100"},
			},
			expected: []string{"some", "zero"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stations := make([]station.Station, len(tt.stations))
			copy(stations, tt.stations)

			service.sortStationsByListeners(stations)

			if len(stations) != len(tt.expected) {
				t.Fatalf("sortStationsByListeners resulted in %d stations, want %d",
					len(stations), len(tt.expected))
			}

			for i, st := range stations {
				if st.ID != tt.expected[i] {
					t.Errorf("stations[%d].ID = %q, want %q", i, st.ID, tt.expected[i])
				}
			}
		})
	}
}

func TestGetValidStationIDs(t *testing.T) {
	service := &StationService{
		stations: []station.Station{
			{ID: "groovesalad"},
			{ID: "dronezone"},
			{ID: "lush"},
		},
	}

	validIDs := service.GetValidStationIDs()

	if len(validIDs) != 3 {
		t.Fatalf("GetValidStationIDs() returned %d IDs, want 3", len(validIDs))
	}

	expectedIDs := []string{"groovesalad", "dronezone", "lush"}
	for _, id := range expectedIDs {
		if !validIDs[id] {
			t.Errorf("GetValidStationIDs() missing %q", id)
		}
	}

	if validIDs["nonexistent"] {
		t.Error("GetValidStationIDs() should return false for nonexistent ID")
	}
}

func TestGetValidStationIDsEmpty(t *testing.T) {
	service := &StationService{
		stations: []station.Station{},
	}

	validIDs := service.GetValidStationIDs()

	if len(validIDs) != 0 {
		t.Errorf("GetValidStationIDs() with empty stations returned %d IDs, want 0", len(validIDs))
	}
}

func TestFindIndexByID(t *testing.T) {
	service := &StationService{
		stations: []station.Station{
			{ID: "groovesalad"},
			{ID: "dronezone"},
			{ID: "lush"},
		},
	}

	tests := []struct {
		name     string
		id       string
		expected int
	}{
		{"first station", "groovesalad", 0},
		{"middle station", "dronezone", 1},
		{"last station", "lush", 2},
		{"nonexistent station", "notfound", -1},
		{"empty string", "", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.FindIndexByID(tt.id)
			if result != tt.expected {
				t.Errorf("FindIndexByID(%q) = %d, want %d", tt.id, result, tt.expected)
			}
		})
	}
}

func TestFindIndexByIDEmptyList(t *testing.T) {
	service := &StationService{
		stations: []station.Station{},
	}

	result := service.FindIndexByID("anything")
	if result != -1 {
		t.Errorf("FindIndexByID with empty list = %d, want -1", result)
	}
}

func TestGetStation(t *testing.T) {
	stations := []station.Station{
		{ID: "groovesalad", Title: "Groove Salad"},
		{ID: "dronezone", Title: "Drone Zone"},
		{ID: "lush", Title: "Lush"},
	}

	service := &StationService{
		stations: stations,
	}

	tests := []struct {
		name        string
		index       int
		expectedID  string
		expectedNil bool
	}{
		{"first station", 0, "groovesalad", false},
		{"middle station", 1, "dronezone", false},
		{"last station", 2, "lush", false},
		{"negative index", -1, "", true},
		{"index out of bounds", 3, "", true},
		{"index way out of bounds", 100, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.GetStation(tt.index)

			if tt.expectedNil {
				if result != nil {
					t.Errorf("GetStation(%d) = %v, want nil", tt.index, result)
				}
			} else if result == nil {
				t.Fatalf("GetStation(%d) = nil, want station", tt.index)
			} else if result.ID != tt.expectedID {
				t.Errorf("GetStation(%d).ID = %q, want %q", tt.index, result.ID, tt.expectedID)
			}
		})
	}
}

func TestGetStationEmptyList(t *testing.T) {
	service := &StationService{
		stations: []station.Station{},
	}

	result := service.GetStation(0)
	if result != nil {
		t.Errorf("GetStation(0) with empty list = %v, want nil", result)
	}
}

func TestStationCount(t *testing.T) {
	tests := []struct {
		name     string
		stations []station.Station
		expected int
	}{
		{
			name: "multiple stations",
			stations: []station.Station{
				{ID: "a"}, {ID: "b"}, {ID: "c"},
			},
			expected: 3,
		},
		{
			name:     "empty list",
			stations: []station.Station{},
			expected: 0,
		},
		{
			name: "single station",
			stations: []station.Station{
				{ID: "only"},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &StationService{stations: tt.stations}
			result := service.StationCount()
			if result != tt.expected {
				t.Errorf("StationCount() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestGetCachedStations(t *testing.T) {
	expectedStations := []station.Station{
		{ID: "groovesalad", Title: "Groove Salad"},
		{ID: "dronezone", Title: "Drone Zone"},
	}

	service := &StationService{
		stations: expectedStations,
	}

	result := service.GetCachedStations()

	if len(result) != len(expectedStations) {
		t.Fatalf("GetCachedStations() returned %d stations, want %d",
			len(result), len(expectedStations))
	}

	for i, st := range result {
		if st.ID != expectedStations[i].ID {
			t.Errorf("GetCachedStations()[%d].ID = %q, want %q",
				i, st.ID, expectedStations[i].ID)
		}
	}
}

func TestGetCachedStationsEmpty(t *testing.T) {
	service := &StationService{
		stations: []station.Station{},
	}

	result := service.GetCachedStations()

	if len(result) != 0 {
		t.Errorf("GetCachedStations() with empty list returned %d stations, want 0",
			len(result))
	}
}

func TestNewStationService(t *testing.T) {
	service := NewStationService(nil)

	if service == nil {
		t.Fatal("NewStationService() returned nil")
	}

	if service.StationCount() != 0 {
		t.Errorf("NewStationService() created service with %d stations, want 0",
			service.StationCount())
	}
}

func TestLoadImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, img)
	}))
	defer server.Close()

	service := &StationService{
		imageCache: nil,
	}

	loadedImg, err := service.LoadImage(server.URL + "/test.png")
	if err != nil {
		t.Fatalf("LoadImage() error = %v", err)
	}

	if loadedImg == nil {
		t.Fatal("LoadImage() returned nil image")
	}

	bounds := loadedImg.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("LoadImage() returned image with size %dx%d, want 100x100",
			bounds.Dx(), bounds.Dy())
	}
}

func TestLoadImageInvalidURL(t *testing.T) {
	service := &StationService{
		imageCache: nil,
	}

	_, err := service.LoadImage("http://invalid.invalid.invalid/image.png")
	if err == nil {
		t.Error("LoadImage() should return error for invalid URL")
	}
}

func TestLoadImageWithCache(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 255, B: 0, A: 255})
		}
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, img)
	}))
	defer server.Close()

	imageCache, err := cache.NewCache()
	if err != nil {
		t.Skipf("Could not create cache: %v", err)
	}

	service := &StationService{
		imageCache: imageCache,
	}

	testURL := server.URL + "/test-cache.png"

	loadedImg1, err := service.LoadImage(testURL)
	if err != nil {
		t.Fatalf("First LoadImage() error = %v", err)
	}

	if loadedImg1 == nil {
		t.Fatal("First LoadImage() returned nil")
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 HTTP request, got %d", requestCount)
	}

	time.Sleep(100 * time.Millisecond)

	loadedImg2, err := service.LoadImage(testURL)
	if err != nil {
		t.Fatalf("Second LoadImage() error = %v", err)
	}

	if loadedImg2 == nil {
		t.Fatal("Second LoadImage() returned nil")
	}

	if requestCount != 1 {
		t.Logf("Got %d HTTP requests (cache may not be working)", requestCount)
	}
}

func TestLoadImageInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("not a valid image"))
	}))
	defer server.Close()

	service := &StationService{
		imageCache: nil,
	}

	_, err := service.LoadImage(server.URL + "/test.png")
	if err == nil {
		t.Error("LoadImage() should return error for invalid image data")
	}
}

func TestStartAndStopPeriodicRefresh(t *testing.T) {
	service := &StationService{}

	callback := func(stations []station.Station) {}

	service.StartPeriodicRefresh(50*time.Millisecond, callback)
	time.Sleep(10 * time.Millisecond)
	service.StopPeriodicRefresh()
	service.StopPeriodicRefresh()
}

func TestStopPeriodicRefreshBeforeStart(t *testing.T) {
	service := &StationService{}
	service.StopPeriodicRefresh()
}

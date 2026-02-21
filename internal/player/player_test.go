package player

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPercentToExponent(t *testing.T) {
	tests := []struct {
		percent  float64
		expected float64
	}{
		{0, MinVolumeDB},
		{100, 0},
		{-10, MinVolumeDB},
		{150, 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("percent_%v", tt.percent), func(t *testing.T) {
			result := percentToExponent(tt.percent)
			if result != tt.expected {
				t.Errorf("percentToExponent(%v) = %v, want %v", tt.percent, result, tt.expected)
			}
		})
	}
}

func TestPercentToExponentCurve(t *testing.T) {
	p25 := percentToExponent(25)
	p50 := percentToExponent(50)
	p75 := percentToExponent(75)

	if p25 >= p50 || p50 >= p75 {
		t.Error("Volume curve should be monotonically increasing")
	}

	if p25 <= MinVolumeDB || p75 >= 0 {
		t.Error("Mid-range volumes should be between min and max")
	}
}

func TestPlayerStateString(t *testing.T) {
	tests := []struct {
		state    PlayerState
		expected string
	}{
		{StateIdle, "IDLE"},
		{StateBuffering, "BUFFERING"},
		{StatePlaying, "LIVE"},
		{StatePaused, "PAUSED"},
		{StateReconnecting, "RECONNECTING"},
		{StateError, "ERROR"},
		{PlayerState(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			if result != tt.expected {
				t.Errorf("PlayerState(%d).String() = %q, want %q", tt.state, result, tt.expected)
			}
		})
	}
}

func TestNewPlayer(t *testing.T) {
	p := NewPlayer()

	if p.isPlaying {
		t.Error("New player should not be playing")
	}

	if p.isPaused {
		t.Error("New player should not be paused")
	}
}

func TestIsNonRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"401", &httpStatusError{StatusCode: 401, Status: "Unauthorized"}, true},
		{"403", &httpStatusError{StatusCode: 403, Status: "Forbidden"}, true},
		{"404", &httpStatusError{StatusCode: 404, Status: "Not Found"}, true},
		{"410", &httpStatusError{StatusCode: 410, Status: "Gone"}, true},
		{"500", &httpStatusError{StatusCode: 500, Status: "Internal Server Error"}, false},
		{"503", &httpStatusError{StatusCode: 503, Status: "Service Unavailable"}, false},
		{"wrapped 404", fmt.Errorf("stream failed: %w", &httpStatusError{StatusCode: 404}), true},
		{"generic error", errors.New("connection refused"), false},
		{"timeout", errors.New("timeout"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNonRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("isNonRetryableError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestHttpStatusErrorMessage(t *testing.T) {
	err := &httpStatusError{StatusCode: 404, Status: "404 Not Found"}
	expected := "stream returned status 404: 404 Not Found"
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}
}

func TestParseStreamInfoFromURL(t *testing.T) {
	tests := []struct {
		url             string
		expectedFormat  string
		expectedBitrate int
		expectedQuality string
	}{
		{"https://somafm.com/groovesalad130.pls", "MP3", 128, "high"},
		{"https://somafm.com/groovesalad256.pls", "MP3", 256, "highest"},
		{"https://somafm.com/groovesalad64.pls", "MP3", 64, "medium"},
		{"https://somafm.com/groovesalad32.pls", "MP3", 32, "low"},
		{"https://somafm.com/groovesalad.pls", "MP3", 128, "high"},
		{"https://somafm.com/groovesalad-aac.pls", "AAC", 128, "high"},
		{"https://somafm.com/groovesalad-aacp64.pls", "AAC", 64, "medium"},
		{"https://somafm.com/station320.pls", "MP3", 320, "highest"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			info := parseStreamInfoFromURL(tt.url)

			if info.Format != tt.expectedFormat {
				t.Errorf("Format = %q, want %q", info.Format, tt.expectedFormat)
			}
			if info.Bitrate != tt.expectedBitrate {
				t.Errorf("Bitrate = %d, want %d", info.Bitrate, tt.expectedBitrate)
			}
			if info.Quality != tt.expectedQuality {
				t.Errorf("Quality = %q, want %q", info.Quality, tt.expectedQuality)
			}
		})
	}
}

func TestPlayerTrackManagement(t *testing.T) {
	p := NewPlayer()

	initial := p.GetCurrentTrack()
	if initial != "Waiting for track info..." {
		t.Errorf("Initial track = %q, want 'Waiting for track info...'", initial)
	}

	p.SetInitialTrack("Test Song - Test Artist")
	track := p.GetCurrentTrack()
	if track != "Test Song - Test Artist" {
		t.Errorf("After SetInitialTrack, track = %q", track)
	}

	p.SetInitialTrack("Should Not Override")
	track = p.GetCurrentTrack()
	if track != "Test Song - Test Artist" {
		t.Errorf("SetInitialTrack should not override existing track, got %q", track)
	}

	p.setCurrentTrack("New Track via ICY")
	track = p.GetCurrentTrack()
	if track != "New Track via ICY" {
		t.Errorf("setCurrentTrack should override, got %q", track)
	}
}

func TestPlayerStateManagement(t *testing.T) {
	p := NewPlayer()

	if p.GetState() != StateIdle {
		t.Errorf("Initial state = %v, want StateIdle", p.GetState())
	}

	p.setState(StateBuffering)
	if p.GetState() != StateBuffering {
		t.Errorf("State after setState = %v, want StateBuffering", p.GetState())
	}

	p.setState(StatePlaying)
	if p.GetState() != StatePlaying {
		t.Errorf("State = %v, want StatePlaying", p.GetState())
	}
}

func TestPlayerRetryInfo(t *testing.T) {
	p := NewPlayer()

	current, max := p.GetRetryInfo()
	if current != 0 || max != 0 {
		t.Errorf("Initial retry info = (%d, %d), want (0, 0)", current, max)
	}

	p.setRetryInfo(2, 5)
	current, max = p.GetRetryInfo()
	if current != 2 || max != 5 {
		t.Errorf("Retry info = (%d, %d), want (2, 5)", current, max)
	}
}

func TestPlayerLastError(t *testing.T) {
	p := NewPlayer()

	if p.GetLastError() != "" {
		t.Errorf("Initial error = %q, want empty", p.GetLastError())
	}

	p.setLastError("Connection failed")
	if p.GetLastError() != "Connection failed" {
		t.Errorf("Error = %q, want 'Connection failed'", p.GetLastError())
	}
}

func TestPlayerSessionDuration(t *testing.T) {
	p := NewPlayer()

	if p.GetSessionDuration() != 0 {
		t.Error("Initial session duration should be 0")
	}

	p.startSession()
	time.Sleep(10 * time.Millisecond)

	duration := p.GetSessionDuration()
	if duration < 10*time.Millisecond {
		t.Errorf("Session duration = %v, expected >= 10ms", duration)
	}
}

func TestPlayerDelayTracking(t *testing.T) {
	p := NewPlayer()

	if delay := p.GetPlaybackDelay(); delay != 0 {
		t.Errorf("Initial delay = %v, want 0", delay)
	}

	p.mu.Lock()
	p.totalPausedMs = 3000
	p.mu.Unlock()

	delay := p.GetPlaybackDelay()
	if delay < 2*time.Second || delay > 4*time.Second {
		t.Errorf("Delay = %v, expected ~3s", delay)
	}

	p.mu.Lock()
	p.pausedAt = time.Now().Add(-2 * time.Second)
	p.mu.Unlock()

	delay = p.GetPlaybackDelay()
	if delay < 4*time.Second || delay > 6*time.Second {
		t.Errorf("Delay with active pause = %v, expected ~5s", delay)
	}
}

func TestPlayerBufferHealth(t *testing.T) {
	p := NewPlayer()

	if p.GetBufferHealth() != 0 {
		t.Error("Buffer health should be 0 with no sample channel")
	}
}

func TestPlayerStreamInfo(t *testing.T) {
	p := NewPlayer()

	info := p.GetStreamInfo()
	if info.Format != "" || info.Bitrate != 0 {
		t.Error("Initial stream info should be empty")
	}

	p.setStreamInfo(StreamInfo{
		Format:     "MP3",
		Quality:    "high",
		Bitrate:    128,
		SampleRate: 44100,
	})

	info = p.GetStreamInfo()
	if info.Format != "MP3" || info.Bitrate != 128 {
		t.Errorf("Stream info = %+v, expected MP3/128", info)
	}
}

func TestPlayerIsPlayingIsPaused(t *testing.T) {
	p := NewPlayer()

	if p.IsPlaying() {
		t.Error("New player should not be playing")
	}
	if p.IsPaused() {
		t.Error("New player should not be paused")
	}

	p.mu.Lock()
	p.isPlaying = true
	p.mu.Unlock()

	if !p.IsPlaying() {
		t.Error("Player should be playing")
	}

	p.mu.Lock()
	p.isPaused = true
	p.mu.Unlock()

	if p.IsPlaying() {
		t.Error("Paused player should not report IsPlaying=true")
	}
	if !p.IsPaused() {
		t.Error("Player should be paused")
	}
}

func TestFetchAndParsePLS(t *testing.T) {
	plsContent := `[playlist]
NumberOfEntries=3
File1=http://stream1.example.com/radio.mp3
Title1=Stream 1
File2=http://stream2.example.com/radio.mp3
Title2=Stream 2
File3=http://stream3.example.com/radio.mp3
Title3=Stream 3
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(plsContent))
	}))
	defer server.Close()

	p := NewPlayer()

	ctx := context.Background()
	urls, err := p.fetchAndParsePLS(ctx, server.URL)
	if err != nil {
		t.Fatalf("fetchAndParsePLS error: %v", err)
	}

	if len(urls) != 3 {
		t.Fatalf("Expected 3 URLs, got %d", len(urls))
	}

	expectedURLs := []string{
		"http://stream1.example.com/radio.mp3",
		"http://stream2.example.com/radio.mp3",
		"http://stream3.example.com/radio.mp3",
	}

	for i, expected := range expectedURLs {
		if urls[i] != expected {
			t.Errorf("URL[%d] = %q, want %q", i, urls[i], expected)
		}
	}
}

func TestFetchAndParsePLSEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("[playlist]\nNumberOfEntries=0\n"))
	}))
	defer server.Close()

	p := NewPlayer()
	ctx := context.Background()

	_, err := p.fetchAndParsePLS(ctx, server.URL)
	if err == nil {
		t.Error("Expected error for empty PLS file")
	}
}

func TestFetchAndParsePLSInvalidServer(t *testing.T) {
	p := NewPlayer()
	ctx := context.Background()

	_, err := p.fetchAndParsePLS(ctx, "http://invalid.invalid.invalid/test.pls")
	if err == nil {
		t.Error("Expected error for invalid server")
	}
}

func TestContextReader(t *testing.T) {
	t.Run("successful read", func(t *testing.T) {
		reader := strings.NewReader("test data")
		ctx := context.Background()
		cr := &contextReader{reader: reader, ctx: ctx, timeout: time.Second}

		buf := make([]byte, 100)
		n, err := cr.Read(buf)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if n != 9 {
			t.Errorf("Read %d bytes, want 9", n)
		}
		if string(buf[:n]) != "test data" {
			t.Errorf("Data = %q, want 'test data'", string(buf[:n]))
		}
	})

	t.Run("timeout", func(t *testing.T) {
		blockingReader := &blockingReader{}
		ctx := context.Background()
		cr := &contextReader{reader: blockingReader, ctx: ctx, timeout: 10 * time.Millisecond}

		buf := make([]byte, 100)
		_, err := cr.Read(buf)

		if err == nil {
			t.Error("Expected timeout error")
		}
		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Error = %q, expected to contain 'timeout'", err.Error())
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		blockingReader := &blockingReader{}
		ctx, cancel := context.WithCancel(context.Background())
		cr := &contextReader{reader: blockingReader, ctx: ctx, timeout: time.Hour}

		// Cancel context immediately
		cancel()

		buf := make([]byte, 100)
		_, err := cr.Read(buf)

		if err == nil {
			t.Error("Expected context cancelled error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Error = %v, expected context.Canceled", err)
		}
	})
}

type blockingReader struct{}

func (b *blockingReader) Read(p []byte) (int, error) {
	time.Sleep(time.Hour)
	return 0, nil
}

func TestGetCurrentStation(t *testing.T) {
	p := NewPlayer()

	if p.GetCurrentStation() != nil {
		t.Error("Initial station should be nil")
	}
}

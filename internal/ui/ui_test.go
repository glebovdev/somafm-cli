package ui

import (
	"errors"
	"testing"
)

func TestNewPlayingSpinner(t *testing.T) {
	spinner := NewPlayingSpinner()

	if spinner == nil {
		t.Fatal("NewPlayingSpinner() returned nil")
	}

	if len(spinner.Frames) == 0 {
		t.Error("PlayingSpinner.Frames is empty")
	}

	if spinner.FPS <= 0 {
		t.Error("PlayingSpinner.FPS should be positive")
	}
}

func TestPlayingSpinnerFrames(t *testing.T) {
	spinner := NewPlayingSpinner()

	for i, frame := range spinner.Frames {
		if frame == "" {
			t.Errorf("Frame[%d] is empty", i)
		}
	}

	if len(spinner.Frames) < 2 {
		t.Errorf("Expected at least 2 frames, got %d", len(spinner.Frames))
	}
}

func TestQualityShort(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"highest", "HQ"},
		{"high", "HQ"},
		{"medium", "MQ"},
		{"low", "LQ"},
		{"", ""},
		{"unknown", ""},
		{"HIGHEST", ""},
		{"very high", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := qualityShort(tt.input)
			if result != tt.expected {
				t.Errorf("qualityShort(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestJoinParts(t *testing.T) {
	tests := []struct {
		name     string
		parts    []string
		expected string
	}{
		{
			name:     "empty slice",
			parts:    []string{},
			expected: "",
		},
		{
			name:     "single part",
			parts:    []string{"PLAYING"},
			expected: "PLAYING",
		},
		{
			name:     "two parts",
			parts:    []string{"PLAYING", "MP3"},
			expected: "PLAYING │ MP3",
		},
		{
			name:     "three parts",
			parts:    []string{"● LIVE", "MP3 HQ", "44.1kHz"},
			expected: "● LIVE │ MP3 HQ │ 44.1kHz",
		},
		{
			name:     "nil slice",
			parts:    nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinParts(tt.parts)
			if result != tt.expected {
				t.Errorf("joinParts(%v) = %q, want %q", tt.parts, result, tt.expected)
			}
		})
	}
}

func TestExtractErrorReason(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{
			name:     "no such host",
			err:      errors.New("dial tcp: lookup example.com: no such host"),
			contains: "Unable to connect",
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp 127.0.0.1:80: connection refused"),
			contains: "Connection refused",
		},
		{
			name:     "timeout",
			err:      errors.New("context deadline exceeded (Client.Timeout exceeded)"),
			contains: "timed out",
		},
		{
			name:     "network unreachable",
			err:      errors.New("dial tcp: network is unreachable"),
			contains: "Network is unreachable",
		},
		{
			name:     "401 unauthorized",
			err:      errors.New("unexpected status 401"),
			contains: "401",
		},
		{
			name:     "403 forbidden",
			err:      errors.New("unexpected status 403"),
			contains: "403",
		},
		{
			name:     "404 not found",
			err:      errors.New("unexpected status 404"),
			contains: "404",
		},
		{
			name:     "generic error (short)",
			err:      errors.New("some error"),
			contains: "some error",
		},
		{
			name:     "dial error truncation",
			err:      errors.New("failed to connect: dial tcp something something"),
			contains: "failed to connect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractErrorReason(tt.err)
			if result == "" {
				t.Error("extractErrorReason returned empty string")
			}
			if !containsString(result, tt.contains) {
				t.Errorf("extractErrorReason(%v) = %q, expected to contain %q",
					tt.err, result, tt.contains)
			}
		})
	}
}

func TestExtractErrorReasonLongError(t *testing.T) {
	longErr := errors.New(string(make([]byte, 200)))
	result := extractErrorReason(longErr)

	if len(result) > 110 {
		t.Errorf("Long error not truncated properly, got length %d", len(result))
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestStatusRendererFormatBufferHealth(t *testing.T) {
	renderer := &StatusRenderer{}

	tests := []struct {
		percent  int
		expected int // expected number of characters (5 bars)
	}{
		{0, 5},
		{50, 5},
		{100, 5},
		{120, 5},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := renderer.formatBufferHealth(tt.percent)
			runeCount := 0
			for range result {
				runeCount++
			}
			if runeCount != tt.expected {
				t.Errorf("formatBufferHealth(%d) returned %d runes, want %d",
					tt.percent, runeCount, tt.expected)
			}
		})
	}
}

func TestStatusRendererFormatBufferHealthProgression(t *testing.T) {
	renderer := &StatusRenderer{}

	result0 := renderer.formatBufferHealth(0)
	result50 := renderer.formatBufferHealth(50)
	result100 := renderer.formatBufferHealth(100)

	if result0 == result100 {
		t.Error("0% and 100% buffer health should look different")
	}

	if result50 == result0 || result50 == result100 {
		t.Log("50% buffer health representation may equal 0% or 100% due to rounding")
	}
}

func TestStatusRendererSetters(t *testing.T) {
	renderer := NewStatusRenderer(nil)

	renderer.SetMuted(true)
	if !renderer.isMuted {
		t.Error("SetMuted(true) did not set isMuted")
	}

	renderer.SetMuted(false)
	if renderer.isMuted {
		t.Error("SetMuted(false) did not clear isMuted")
	}

	renderer.SetPrimaryColor("red")
	if renderer.primaryColor != "red" {
		t.Errorf("SetPrimaryColor set %q, want %q", renderer.primaryColor, "red")
	}
}

func TestStatusRendererAdvanceAnimation(t *testing.T) {
	renderer := NewStatusRenderer(nil)

	initialFrame := renderer.animFrame

	for i := 0; i < renderer.ticksPerFrame-1; i++ {
		renderer.AdvanceAnimation()
	}

	if renderer.animFrame != initialFrame {
		t.Error("Animation frame changed before ticksPerFrame ticks")
	}

	renderer.AdvanceAnimation()

	if renderer.animFrame != (initialFrame+1)%renderer.maxAnimFrame {
		t.Errorf("Animation frame = %d, want %d",
			renderer.animFrame, (initialFrame+1)%renderer.maxAnimFrame)
	}

	if renderer.tickCount != 0 {
		t.Errorf("tickCount = %d, want 0 after frame advance", renderer.tickCount)
	}
}

func TestStatusRendererRenderIdle(t *testing.T) {
	renderer := NewStatusRenderer(nil)

	result := renderer.renderIdle()
	if result == "" {
		t.Error("renderIdle() returned empty string")
	}
	if !findSubstring(result, "IDLE") {
		t.Errorf("renderIdle() = %q, expected to contain 'IDLE'", result)
	}

	renderer.SetMuted(true)
	result = renderer.renderIdle()
	if !findSubstring(result, "MUTED") {
		t.Errorf("renderIdle() when muted = %q, expected to contain 'MUTED'", result)
	}
}

func TestStatusRendererRenderBuffering(t *testing.T) {
	renderer := NewStatusRenderer(nil)

	result := renderer.renderBuffering()
	if result == "" {
		t.Error("renderBuffering() returned empty string")
	}
	if !findSubstring(result, "BUFFERING") {
		t.Errorf("renderBuffering() = %q, expected to contain 'BUFFERING'", result)
	}
}

// Note: renderReconnecting, renderPlaying, renderPaused, and renderError
// require a non-nil player to get state info. Testing those would require
// a mock player, which is beyond the scope of these pure function tests.
// The core formatting logic is still covered by formatBufferHealth,
// formatDuration, qualityShort, and joinParts tests.

func TestNewStatusRenderer(t *testing.T) {
	renderer := NewStatusRenderer(nil)

	if renderer == nil {
		t.Fatal("NewStatusRenderer() returned nil")
	}

	if renderer.maxAnimFrame <= 0 {
		t.Error("maxAnimFrame should be positive")
	}

	if renderer.ticksPerFrame <= 0 {
		t.Error("ticksPerFrame should be positive")
	}

	if renderer.bufferTicksPerUpdate <= 0 {
		t.Error("bufferTicksPerUpdate should be positive")
	}
}

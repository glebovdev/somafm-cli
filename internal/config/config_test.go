package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Volume != DefaultVolume {
		t.Errorf("DefaultConfig().Volume = %d, want %d", cfg.Volume, DefaultVolume)
	}

	if cfg.LastStation != "" {
		t.Errorf("DefaultConfig().LastStation = %q, want empty string", cfg.LastStation)
	}

	if cfg.Autostart != false {
		t.Errorf("DefaultConfig().Autostart = %v, want false", cfg.Autostart)
	}
}

func TestConfigSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	testCfg := &Config{
		Volume:      85,
		LastStation: "groovesalad",
	}

	err := testCfg.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	configPath := filepath.Join(tmpDir, ConfigDir, ConfigFileName)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}

	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loadedCfg.Volume != testCfg.Volume {
		t.Errorf("Load().Volume = %d, want %d", loadedCfg.Volume, testCfg.Volume)
	}

	if loadedCfg.LastStation != testCfg.LastStation {
		t.Errorf("Load().LastStation = %q, want %q", loadedCfg.LastStation, testCfg.LastStation)
	}
}

func TestLoadNonExistentConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg, err := Load()
	if err != nil {
		t.Logf("Load() error (expected): %v", err)
	}

	if cfg.Volume != DefaultVolume {
		t.Errorf("Load() with non-existent file returned Volume = %d, want %d", cfg.Volume, DefaultVolume)
	}

	if cfg.LastStation != "" {
		t.Errorf("Load() with non-existent file returned LastStation = %q, want empty string", cfg.LastStation)
	}
}

func TestVolumeValidation(t *testing.T) {
	tests := []struct {
		name           string
		inputVolume    int
		expectedVolume int
	}{
		{"valid volume 50", 50, 50},
		{"valid volume 0", 0, 0},
		{"valid volume 100", 100, 100},
		{"negative volume", -10, 0},
		{"volume over 100", 150, 100},
		{"volume way over 100", 1000, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)

			testCfg := &Config{
				Volume:      tt.inputVolume,
				LastStation: "groovesalad",
			}

			err := testCfg.Save()
			if err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			loadedCfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if loadedCfg.Volume != tt.expectedVolume {
				t.Errorf("Load().Volume = %d, want %d", loadedCfg.Volume, tt.expectedVolume)
			}
		})
	}
}

func TestThemeDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg, err := Load()
	if err != nil {
		t.Logf("Load() error (expected): %v", err)
	}

	if cfg.Theme.Background != "#1a1b25" {
		t.Errorf("Theme.Background = %q, want %q", cfg.Theme.Background, "#1a1b25")
	}
	if cfg.Theme.Foreground != "#a3aacb" {
		t.Errorf("Theme.Foreground = %q, want %q", cfg.Theme.Foreground, "#a3aacb")
	}
	if cfg.Theme.Borders != "#40445b" {
		t.Errorf("Theme.Borders = %q, want %q", cfg.Theme.Borders, "#40445b")
	}
	if cfg.Theme.Highlight != "#ff9d65" {
		t.Errorf("Theme.Highlight = %q, want %q", cfg.Theme.Highlight, "#ff9d65")
	}
	if cfg.Theme.MutedVolume != "#fe0702" {
		t.Errorf("Theme.MutedVolume = %q, want %q", cfg.Theme.MutedVolume, "#fe0702")
	}
}

func TestThemePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	testCfg := &Config{
		Volume:      70,
		LastStation: "groovesalad",
		Theme: Theme{
			Background: "black",
			Foreground: "yellow",
			Borders:    "blue",
			Highlight:  "red",
		},
	}

	err := testCfg.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loadedCfg.Theme.Background != "black" {
		t.Errorf("Theme.Background = %q, want %q", loadedCfg.Theme.Background, "black")
	}
	if loadedCfg.Theme.Foreground != "yellow" {
		t.Errorf("Theme.Foreground = %q, want %q", loadedCfg.Theme.Foreground, "yellow")
	}
	if loadedCfg.Theme.Borders != "blue" {
		t.Errorf("Theme.Borders = %q, want %q", loadedCfg.Theme.Borders, "blue")
	}
	if loadedCfg.Theme.Highlight != "red" {
		t.Errorf("Theme.Highlight = %q, want %q", loadedCfg.Theme.Highlight, "red")
	}
}

func TestIsFavorite(t *testing.T) {
	tests := []struct {
		name      string
		favorites []string
		stationID string
		expected  bool
	}{
		{
			name:      "station is favorite",
			favorites: []string{"groovesalad", "dronezone", "lush"},
			stationID: "dronezone",
			expected:  true,
		},
		{
			name:      "station is not favorite",
			favorites: []string{"groovesalad", "dronezone"},
			stationID: "lush",
			expected:  false,
		},
		{
			name:      "empty favorites list",
			favorites: []string{},
			stationID: "groovesalad",
			expected:  false,
		},
		{
			name:      "first item in list",
			favorites: []string{"groovesalad", "dronezone"},
			stationID: "groovesalad",
			expected:  true,
		},
		{
			name:      "last item in list",
			favorites: []string{"groovesalad", "dronezone", "lush"},
			stationID: "lush",
			expected:  true,
		},
		{
			name:      "nil favorites",
			favorites: nil,
			stationID: "groovesalad",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Favorites: tt.favorites}
			result := cfg.IsFavorite(tt.stationID)
			if result != tt.expected {
				t.Errorf("IsFavorite(%q) = %v, want %v", tt.stationID, result, tt.expected)
			}
		})
	}
}

func TestToggleFavorite(t *testing.T) {
	tests := []struct {
		name              string
		initialFavorites  []string
		stationID         string
		expectedFavorites []string
	}{
		{
			name:              "add to empty list",
			initialFavorites:  []string{},
			stationID:         "groovesalad",
			expectedFavorites: []string{"groovesalad"},
		},
		{
			name:              "add to existing list",
			initialFavorites:  []string{"dronezone"},
			stationID:         "groovesalad",
			expectedFavorites: []string{"dronezone", "groovesalad"},
		},
		{
			name:              "remove from list",
			initialFavorites:  []string{"groovesalad", "dronezone", "lush"},
			stationID:         "dronezone",
			expectedFavorites: []string{"groovesalad", "lush"},
		},
		{
			name:              "remove first item",
			initialFavorites:  []string{"groovesalad", "dronezone"},
			stationID:         "groovesalad",
			expectedFavorites: []string{"dronezone"},
		},
		{
			name:              "remove last item",
			initialFavorites:  []string{"groovesalad", "dronezone"},
			stationID:         "dronezone",
			expectedFavorites: []string{"groovesalad"},
		},
		{
			name:              "remove only item",
			initialFavorites:  []string{"groovesalad"},
			stationID:         "groovesalad",
			expectedFavorites: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Favorites: make([]string, len(tt.initialFavorites))}
			copy(cfg.Favorites, tt.initialFavorites)

			cfg.ToggleFavorite(tt.stationID)

			if len(cfg.Favorites) != len(tt.expectedFavorites) {
				t.Fatalf("ToggleFavorite(%q) resulted in %d favorites, want %d",
					tt.stationID, len(cfg.Favorites), len(tt.expectedFavorites))
			}

			for i, fav := range cfg.Favorites {
				if fav != tt.expectedFavorites[i] {
					t.Errorf("Favorites[%d] = %q, want %q", i, fav, tt.expectedFavorites[i])
				}
			}
		})
	}
}

func TestToggleFavoriteDoubleToggle(t *testing.T) {
	cfg := &Config{Favorites: []string{}}

	cfg.ToggleFavorite("groovesalad")
	if !cfg.IsFavorite("groovesalad") {
		t.Error("After first toggle, groovesalad should be favorite")
	}

	cfg.ToggleFavorite("groovesalad")
	if cfg.IsFavorite("groovesalad") {
		t.Error("After second toggle, groovesalad should not be favorite")
	}
}

func TestCleanupFavorites(t *testing.T) {
	tests := []struct {
		name              string
		initialFavorites  []string
		validStationIDs   map[string]bool
		expectedFavorites []string
	}{
		{
			name:              "all valid",
			initialFavorites:  []string{"groovesalad", "dronezone"},
			validStationIDs:   map[string]bool{"groovesalad": true, "dronezone": true, "lush": true},
			expectedFavorites: []string{"groovesalad", "dronezone"},
		},
		{
			name:              "some invalid",
			initialFavorites:  []string{"groovesalad", "deleted_station", "dronezone"},
			validStationIDs:   map[string]bool{"groovesalad": true, "dronezone": true},
			expectedFavorites: []string{"groovesalad", "dronezone"},
		},
		{
			name:              "all invalid",
			initialFavorites:  []string{"deleted1", "deleted2"},
			validStationIDs:   map[string]bool{"groovesalad": true},
			expectedFavorites: []string{},
		},
		{
			name:              "empty favorites",
			initialFavorites:  []string{},
			validStationIDs:   map[string]bool{"groovesalad": true},
			expectedFavorites: []string{},
		},
		{
			name:              "empty valid IDs",
			initialFavorites:  []string{"groovesalad"},
			validStationIDs:   map[string]bool{},
			expectedFavorites: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Favorites: make([]string, len(tt.initialFavorites))}
			copy(cfg.Favorites, tt.initialFavorites)

			cfg.CleanupFavorites(tt.validStationIDs)

			if len(cfg.Favorites) != len(tt.expectedFavorites) {
				t.Fatalf("CleanupFavorites resulted in %d favorites, want %d",
					len(cfg.Favorites), len(tt.expectedFavorites))
			}

			for i, fav := range cfg.Favorites {
				if fav != tt.expectedFavorites[i] {
					t.Errorf("Favorites[%d] = %q, want %q", i, fav, tt.expectedFavorites[i])
				}
			}
		})
	}
}

func TestGetColor(t *testing.T) {
	tests := []struct {
		name     string
		colorStr string
		isNonNil bool
	}{
		{"empty string returns default", "", true},
		{"default keyword returns default", "default", true},
		{"named color white", "white", true},
		{"named color red", "red", true},
		{"named color darkcyan", "darkcyan", true},
		{"hex color", "#FF0000", true},
		{"hex color lowercase", "#ff0000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetColor(tt.colorStr)
			if tt.colorStr == "" || tt.colorStr == "default" {
				if result != 0 {
					t.Errorf("GetColor(%q) = %v, want ColorDefault (0)", tt.colorStr, result)
				}
			}
		})
	}
}

func TestFavoritesPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	testCfg := &Config{
		Volume:    70,
		Favorites: []string{"groovesalad", "dronezone", "lush"},
		Theme:     DefaultConfig().Theme,
	}

	err := testCfg.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(loadedCfg.Favorites) != 3 {
		t.Fatalf("Load().Favorites has %d items, want 3", len(loadedCfg.Favorites))
	}

	expected := []string{"groovesalad", "dronezone", "lush"}
	for i, fav := range loadedCfg.Favorites {
		if fav != expected[i] {
			t.Errorf("Favorites[%d] = %q, want %q", i, fav, expected[i])
		}
	}
}

func TestAutostartPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	testCfg := &Config{
		Volume:      70,
		LastStation: "groovesalad",
		Autostart:   true,
		Theme:       DefaultConfig().Theme,
	}

	err := testCfg.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loadedCfg.Autostart != true {
		t.Errorf("Load().Autostart = %v, want true", loadedCfg.Autostart)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ConfigDir)
	_ = os.MkdirAll(configDir, 0755)
	configPath := filepath.Join(configDir, ConfigFileName)

	invalidYAML := []byte("this is not: valid: yaml: [")
	_ = os.WriteFile(configPath, invalidYAML, 0644)

	cfg, err := Load()
	if err == nil {
		t.Log("Load() returned no error for invalid YAML, but returned default config")
	}

	if cfg.Volume != DefaultVolume {
		t.Errorf("Load() with invalid YAML returned Volume = %d, want default %d", cfg.Volume, DefaultVolume)
	}
}

func TestGetConfigPath(t *testing.T) {
	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath() error = %v", err)
	}

	if path == "" {
		t.Error("GetConfigPath() returned empty string")
	}

	if !filepath.IsAbs(path) {
		t.Errorf("GetConfigPath() = %q, want absolute path", path)
	}
}

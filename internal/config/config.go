package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gdamore/tcell/v2"
	"gopkg.in/yaml.v3"
)

const (
	AppName           = "SomaFM CLI"
	AppTagline        = "Terminal radio player"
	AppDescription    = "A terminal-based music player for SomaFM radio stations"
	AppAuthor         = "Ilya Glebov"
	AppAuthorURL      = "https://ilyaglebov.dev"
	AppAuthorURLShort = "ilyaglebov.dev"
	AppProjectURL     = "https://github.com/glebovdev/somafm-cli"
	AppProjectShort   = "github.com/glebovdev/somafm-cli"
	AppDonateURL      = "https://somafm.com/donate/"
	AppDonateShort    = "somafm.com/donate"

	ConfigDir      = ".config/somafm"
	ConfigFileName = "config.yml"
	DefaultVolume  = 70
	MinVolume      = 0
	MaxVolume      = 100
)

// ClampVolume ensures volume is within the valid range [0, 100].
func ClampVolume(volume int) int {
	if volume < MinVolume {
		return MinVolume
	}
	if volume > MaxVolume {
		return MaxVolume
	}
	return volume
}

// AppVersion can be overridden at build time using ldflags:
// go build -ldflags "-X github.com/glebovdev/somafm-cli/internal/config.AppVersion=1.0.0"
var AppVersion = "dev"

type Theme struct {
	Background                  string `yaml:"background"`
	Foreground                  string `yaml:"foreground"`
	Borders                     string `yaml:"borders"`
	Highlight                   string `yaml:"highlight"`
	MutedVolume                 string `yaml:"muted_volume"`
	HeaderBackground            string `yaml:"header_background"`
	StationListHeaderBackground string `yaml:"station_list_header_background"`
	StationListHeaderForeground string `yaml:"station_list_header_foreground"`
	HelpBackground              string `yaml:"help_background"`
	HelpForeground              string `yaml:"help_foreground"`
	HelpHotkey                  string `yaml:"help_hotkey"`
	GenreTagBackground          string `yaml:"genre_tag_background"`
	ModalBackground             string `yaml:"modal_background"`
}

type Config struct {
	Volume      int      `yaml:"volume"`
	LastStation string   `yaml:"last_station"`
	Autostart   bool     `yaml:"autostart"`
	Favorites   []string `yaml:"favorites"`
	Theme       Theme    `yaml:"theme"`
}

func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := filepath.Join(home, ConfigDir, ConfigFileName)
	return configPath, nil
}

func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return DefaultConfig(), err
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return DefaultConfig(), fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg.Volume = ClampVolume(cfg.Volume)

	return cfg, nil
}

// Save writes the configuration to disk atomically using temp file + rename.
func (c *Config) Save() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	tmpFile, err := os.CreateTemp(configDir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("failed to rename config file: %w", err)
	}

	tmpPath = "" // Prevent defer from removing the final file
	return nil
}

func DefaultConfig() *Config {
	return &Config{
		Volume:      DefaultVolume,
		LastStation: "",
		Autostart:   false,
		Favorites:   []string{},
		Theme: Theme{
			Background:                  "#1a1b25",
			Foreground:                  "#a3aacb",
			Borders:                     "#40445b",
			Highlight:                   "#ff9d65",
			MutedVolume:                 "#fe0702",
			HeaderBackground:            "#473533",
			StationListHeaderBackground: "#3a3d4f",
			StationListHeaderForeground: "#c8d0e8",
			HelpBackground:              "#322f45",
			HelpForeground:              "#9aa3c6",
			HelpHotkey:                  "#ff9d65",
			GenreTagBackground:          "#3a3d4f",
			ModalBackground:             "#282a36",
		},
	}
}

func (c *Config) IsFavorite(stationID string) bool {
	for _, id := range c.Favorites {
		if id == stationID {
			return true
		}
	}
	return false
}

func (c *Config) ToggleFavorite(stationID string) {
	for i, id := range c.Favorites {
		if id == stationID {
			c.Favorites = append(c.Favorites[:i], c.Favorites[i+1:]...)
			return
		}
	}
	c.Favorites = append(c.Favorites, stationID)
}

func (c *Config) CleanupFavorites(validStationIDs map[string]bool) {
	cleaned := []string{}
	for _, id := range c.Favorites {
		if validStationIDs[id] {
			cleaned = append(cleaned, id)
		}
	}
	c.Favorites = cleaned
}

func GetColor(colorStr string) tcell.Color {
	if colorStr == "" || colorStr == "default" {
		return tcell.ColorDefault
	}
	return tcell.GetColor(colorStr)
}

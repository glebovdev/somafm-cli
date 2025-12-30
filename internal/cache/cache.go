// Package cache provides image caching functionality for station logos.
package cache

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// DefaultExpiry is how long cached images are valid (7 days).
	DefaultExpiry = 7 * 24 * time.Hour
	// ImageSubdir is the subdirectory for cached images.
	ImageSubdir = "images"
	// AppName is used for the cache directory name.
	AppName = "somafm"
)

// Cache manages disk-based caching of station logo images.
type Cache struct {
	baseDir string
	expiry  time.Duration
}

// NewCache creates a new Cache instance with the default expiry.
func NewCache() (*Cache, error) {
	cacheDir, err := GetCacheDir()
	if err != nil {
		return nil, err
	}

	return &Cache{
		baseDir: cacheDir,
		expiry:  DefaultExpiry,
	}, nil
}

// GetCacheDir returns the platform-specific cache directory for the application.
func GetCacheDir() (string, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user cache directory: %w", err)
	}

	cacheDir := filepath.Join(userCacheDir, AppName)
	return cacheDir, nil
}

func (c *Cache) ensureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

func hashURL(url string) string {
	hash := md5.Sum([]byte(url))
	return hex.EncodeToString(hash[:])
}

// GetImage retrieves a cached image by URL. Returns nil if not found or expired.
func (c *Cache) GetImage(url string) image.Image {
	imageDir := filepath.Join(c.baseDir, ImageSubdir)
	filename := hashURL(url) + ".png"
	imagePath := filepath.Join(imageDir, filename)

	info, err := os.Stat(imagePath)
	if err != nil {
		return nil
	}

	if time.Since(info.ModTime()) > c.expiry {
		if err := os.Remove(imagePath); err != nil {
			log.Debug().Err(err).Str("file", imagePath).Msg("Failed to remove expired cache file")
		}
		return nil
	}

	file, err := os.Open(imagePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		log.Debug().Err(err).Str("file", imagePath).Msg("Failed to decode cached image")
		return nil
	}

	return img
}

// SaveImage stores an image in the cache, keyed by its URL.
func (c *Cache) SaveImage(url string, img image.Image) error {
	imageDir := filepath.Join(c.baseDir, ImageSubdir)

	if err := c.ensureDir(imageDir); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	filename := hashURL(url) + ".png"
	imagePath := filepath.Join(imageDir, filename)

	file, err := os.Create(imagePath)
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		return fmt.Errorf("failed to encode image: %w", err)
	}

	return nil
}

// CleanExpired removes cache files older than the expiry duration.
func (c *Cache) CleanExpired() error {
	imageDir := filepath.Join(c.baseDir, ImageSubdir)

	entries, err := os.ReadDir(imageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	now := time.Now()
	var removed, failed int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			log.Debug().Err(err).Str("file", entry.Name()).Msg("Failed to get file info")
			continue
		}

		if now.Sub(info.ModTime()) > c.expiry {
			filePath := filepath.Join(imageDir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				log.Debug().Err(err).Str("file", filePath).Msg("Failed to remove expired cache file")
				failed++
			} else {
				removed++
			}
		}
	}

	if removed > 0 || failed > 0 {
		log.Debug().Int("removed", removed).Int("failed", failed).Msg("Cache cleanup completed")
	}

	return nil
}

package cache

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHashURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"simple URL", "http://example.com/image.png"},
		{"URL with query params", "http://example.com/image.png?size=large"},
		{"empty string", ""},
		{"https URL", "https://somafm.com/images/logo.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashURL(tt.url)

			if len(result) != 32 {
				t.Errorf("hashURL(%q) length = %d, want 32", tt.url, len(result))
			}

			for _, c := range result {
				if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
					t.Errorf("hashURL(%q) contains non-hex character: %c", tt.url, c)
				}
			}
		})
	}
}

func TestHashURLConsistency(t *testing.T) {
	url := "http://somafm.com/images/groovesalad.png"

	hash1 := hashURL(url)
	hash2 := hashURL(url)

	if hash1 != hash2 {
		t.Errorf("hashURL is not consistent: %q != %q", hash1, hash2)
	}
}

func TestHashURLUniqueness(t *testing.T) {
	url1 := "http://example.com/image1.png"
	url2 := "http://example.com/image2.png"

	hash1 := hashURL(url1)
	hash2 := hashURL(url2)

	if hash1 == hash2 {
		t.Errorf("Different URLs produced same hash: %q", hash1)
	}
}

func createTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	return img
}

func TestSaveAndGetImage(t *testing.T) {
	tmpDir := t.TempDir()

	cache := &Cache{
		baseDir: tmpDir,
		expiry:  DefaultExpiry,
	}

	testURL := "http://example.com/test-image.png"
	testImg := createTestImage(100, 100)

	err := cache.SaveImage(testURL, testImg)
	if err != nil {
		t.Fatalf("SaveImage() error = %v", err)
	}

	retrievedImg := cache.GetImage(testURL)
	if retrievedImg == nil {
		t.Fatal("GetImage() returned nil, expected image")
	}

	bounds := retrievedImg.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("Retrieved image size = %dx%d, want 100x100", bounds.Dx(), bounds.Dy())
	}
}

func TestGetImageNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	cache := &Cache{
		baseDir: tmpDir,
		expiry:  DefaultExpiry,
	}

	result := cache.GetImage("http://example.com/nonexistent.png")
	if result != nil {
		t.Error("GetImage() for nonexistent URL should return nil")
	}
}

func TestGetImageExpired(t *testing.T) {
	tmpDir := t.TempDir()

	cache := &Cache{
		baseDir: tmpDir,
		expiry:  1 * time.Millisecond,
	}

	testURL := "http://example.com/expired-image.png"
	testImg := createTestImage(50, 50)

	err := cache.SaveImage(testURL, testImg)
	if err != nil {
		t.Fatalf("SaveImage() error = %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	result := cache.GetImage(testURL)
	if result != nil {
		t.Error("GetImage() for expired image should return nil")
	}

	filename := hashURL(testURL) + ".png"
	imagePath := filepath.Join(tmpDir, ImageSubdir, filename)
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		t.Error("Expired image file should have been deleted")
	}
}

func TestCleanExpired(t *testing.T) {
	tmpDir := t.TempDir()

	cache := &Cache{
		baseDir: tmpDir,
		expiry:  1 * time.Millisecond,
	}

	testImg := createTestImage(10, 10)
	urls := []string{
		"http://example.com/image1.png",
		"http://example.com/image2.png",
		"http://example.com/image3.png",
	}

	for _, url := range urls {
		if err := cache.SaveImage(url, testImg); err != nil {
			t.Fatalf("SaveImage(%q) error = %v", url, err)
		}
	}

	time.Sleep(10 * time.Millisecond)

	err := cache.CleanExpired()
	if err != nil {
		t.Fatalf("CleanExpired() error = %v", err)
	}

	imageDir := filepath.Join(tmpDir, ImageSubdir)
	entries, err := os.ReadDir(imageDir)
	if err != nil {
		t.Fatalf("Failed to read image directory: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("CleanExpired() left %d files, want 0", len(entries))
	}
}

func TestCleanExpiredKeepsValidFiles(t *testing.T) {
	tmpDir := t.TempDir()

	cache := &Cache{
		baseDir: tmpDir,
		expiry:  24 * time.Hour,
	}

	testImg := createTestImage(10, 10)
	testURL := "http://example.com/valid-image.png"

	if err := cache.SaveImage(testURL, testImg); err != nil {
		t.Fatalf("SaveImage() error = %v", err)
	}

	err := cache.CleanExpired()
	if err != nil {
		t.Fatalf("CleanExpired() error = %v", err)
	}

	result := cache.GetImage(testURL)
	if result == nil {
		t.Error("CleanExpired() should not remove valid (non-expired) images")
	}
}

func TestCleanExpiredNonExistentDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	cache := &Cache{
		baseDir: tmpDir,
		expiry:  DefaultExpiry,
	}

	err := cache.CleanExpired()
	if err != nil {
		t.Errorf("CleanExpired() should not error on non-existent directory, got %v", err)
	}
}

func TestGetCacheDir(t *testing.T) {
	dir, err := GetCacheDir()
	if err != nil {
		t.Fatalf("GetCacheDir() error = %v", err)
	}

	if dir == "" {
		t.Error("GetCacheDir() returned empty string")
	}

	if !filepath.IsAbs(dir) {
		t.Errorf("GetCacheDir() = %q, want absolute path", dir)
	}

	if filepath.Base(dir) != AppName {
		t.Errorf("GetCacheDir() directory name = %q, want %q", filepath.Base(dir), AppName)
	}
}

func TestNewCache(t *testing.T) {
	cache, err := NewCache()
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	if cache == nil {
		t.Fatal("NewCache() returned nil")
	} else {
		if cache.baseDir == "" {
			t.Error("NewCache() cache.baseDir is empty")
		}
		if cache.expiry != DefaultExpiry {
			t.Errorf("NewCache() cache.expiry = %v, want %v", cache.expiry, DefaultExpiry)
		}
	}
}

func TestSaveImageCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	cache := &Cache{
		baseDir: tmpDir,
		expiry:  DefaultExpiry,
	}

	testURL := "http://example.com/image.png"
	testImg := createTestImage(10, 10)

	err := cache.SaveImage(testURL, testImg)
	if err != nil {
		t.Fatalf("SaveImage() error = %v", err)
	}

	imageDir := filepath.Join(tmpDir, ImageSubdir)
	info, err := os.Stat(imageDir)
	if err != nil {
		t.Fatalf("Image directory was not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("ImageSubdir should be a directory")
	}
}

func TestMultipleImagesSameCache(t *testing.T) {
	tmpDir := t.TempDir()

	cache := &Cache{
		baseDir: tmpDir,
		expiry:  DefaultExpiry,
	}

	images := map[string]image.Image{
		"http://example.com/image1.png": createTestImage(50, 50),
		"http://example.com/image2.png": createTestImage(100, 100),
		"http://example.com/image3.png": createTestImage(200, 200),
	}

	for url, img := range images {
		if err := cache.SaveImage(url, img); err != nil {
			t.Fatalf("SaveImage(%q) error = %v", url, err)
		}
	}

	for url, originalImg := range images {
		retrieved := cache.GetImage(url)
		if retrieved == nil {
			t.Errorf("GetImage(%q) returned nil", url)
			continue
		}

		expectedBounds := originalImg.Bounds()
		retrievedBounds := retrieved.Bounds()
		if retrievedBounds.Dx() != expectedBounds.Dx() || retrievedBounds.Dy() != expectedBounds.Dy() {
			t.Errorf("GetImage(%q) size = %dx%d, want %dx%d",
				url, retrievedBounds.Dx(), retrievedBounds.Dy(),
				expectedBounds.Dx(), expectedBounds.Dy())
		}
	}
}

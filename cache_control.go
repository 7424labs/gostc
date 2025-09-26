package gostc

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FileType represents the category of a file for caching purposes
type FileType int

const (
	StaticAsset    FileType = iota // Images, fonts, etc - long cache
	DynamicAsset                   // HTML, JSON - shorter cache
	ImmutableAsset                 // Versioned assets - very long cache
)

// getCacheControl returns the appropriate Cache-Control header value based on file type
func getCacheControl(path string, config *Config, isVersioned bool) string {
	if isVersioned {
		// Content-hashed assets can be cached indefinitely since they're immutable
		return "public, max-age=31536000, immutable"
	}

	fileType := getFileType(path)

	switch fileType {
	case StaticAsset:
		// Static assets like images, fonts, CSS, JS can be cached longer
		return fmt.Sprintf("public, max-age=%d", config.StaticAssetMaxAge)
	case ImmutableAsset:
		// Versioned/hashed assets can be cached indefinitely
		return "public, max-age=31536000, immutable"
	case DynamicAsset:
		// HTML and JSON files should have shorter cache
		return fmt.Sprintf("public, max-age=%d, must-revalidate", config.DynamicAssetMaxAge)
	default:
		return fmt.Sprintf("public, max-age=%d", config.DynamicAssetMaxAge)
	}
}

// getFileType determines the type of file for caching purposes
func getFileType(path string) FileType {
	ext := strings.ToLower(filepath.Ext(path))

	// Check if filename contains hash/version (e.g., app.abc123.js, style.v2.css)
	base := filepath.Base(path)
	parts := strings.Split(base, ".")
	if len(parts) >= 3 {
		// Likely a versioned asset (name.hash.ext or name.version.ext)
		if isStaticExtension(ext) {
			return ImmutableAsset
		}
	}

	// Static assets that change infrequently
	staticExts := map[string]bool{
		".jpg":   true,
		".jpeg":  true,
		".png":   true,
		".gif":   true,
		".svg":   true,
		".webp":  true,
		".ico":   true,
		".woff":  true,
		".woff2": true,
		".ttf":   true,
		".otf":   true,
		".eot":   true,
		".mp4":   true,
		".webm":  true,
		".mp3":   true,
		".wav":   true,
		".pdf":   true,
		".zip":   true,
		".tar":   true,
		".gz":    true,
	}

	// CSS and JS without version/hash are still static but may change
	staticButChangeable := map[string]bool{
		".css": true,
		".js":  true,
		".mjs": true,
	}

	// Dynamic content
	dynamicExts := map[string]bool{
		".html": true,
		".htm":  true,
		".json": true,
		".xml":  true,
		".txt":  true,
		".md":   true,
		".yml":  true,
		".yaml": true,
		".toml": true,
	}

	if staticExts[ext] {
		return StaticAsset
	}

	if staticButChangeable[ext] {
		return StaticAsset
	}

	if dynamicExts[ext] {
		return DynamicAsset
	}

	// Default to dynamic for unknown types
	return DynamicAsset
}

func isStaticExtension(ext string) bool {
	staticExts := []string{".css", ".js", ".mjs", ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp"}
	for _, e := range staticExts {
		if ext == e {
			return true
		}
	}
	return false
}

// shouldRevalidate determines if a file type should always revalidate
func shouldRevalidate(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))

	// These file types should always revalidate
	revalidateExts := map[string]bool{
		".html": true,
		".htm":  true,
		".json": true, // API responses
		".xml":  true, // Sitemaps, RSS feeds
	}

	return revalidateExts[ext]
}

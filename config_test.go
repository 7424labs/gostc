package gostc

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigurationOptions(t *testing.T) {
	t.Run("DefaultConfig", func(t *testing.T) {
		config := DefaultConfig()

		if config.Root != "./static" {
			t.Errorf("Expected default root './static', got %s", config.Root)
		}
		if config.EnableVersioning != false {
			t.Error("Versioning should be disabled by default")
		}
		if config.VersionHashLength != 8 {
			t.Errorf("Expected default hash length 8, got %d", config.VersionHashLength)
		}
		if len(config.StaticPrefixes) == 0 {
			t.Error("Should have default static prefixes")
		}
	})

	t.Run("WithVersioningOption", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if !server.config.EnableVersioning {
			t.Error("WithVersioning(true) should enable versioning")
		}
	})

	t.Run("WithVersioningPatternOption", func(t *testing.T) {
		customPattern := "{base}_v{hash}{ext}"
		server, err := New(WithVersioningPattern(customPattern))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.VersioningPattern != customPattern {
			t.Errorf("Expected pattern %s, got %s", customPattern, server.config.VersioningPattern)
		}
	})

	t.Run("WithVersionHashLengthOption", func(t *testing.T) {
		server, err := New(WithVersionHashLength(12))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.VersionHashLength != 12 {
			t.Errorf("Expected hash length 12, got %d", server.config.VersionHashLength)
		}
	})

	t.Run("WithStaticPrefixesOption", func(t *testing.T) {
		customPrefixes := []string{"/assets/", "/public/", "/dist/"}
		server, err := New(WithStaticPrefixes(customPrefixes...))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if len(server.config.StaticPrefixes) != len(customPrefixes) {
			t.Errorf("Expected %d prefixes, got %d", len(customPrefixes), len(server.config.StaticPrefixes))
		}

		for i, expected := range customPrefixes {
			if server.config.StaticPrefixes[i] != expected {
				t.Errorf("Expected prefix %s, got %s", expected, server.config.StaticPrefixes[i])
			}
		}
	})

	t.Run("CombinedVersioningOptions", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
			WithVersioningPattern("{base}-{hash}{ext}"),
			WithVersionHashLength(16),
			WithStaticPrefixes("/assets/"),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		config := server.config
		if !config.EnableVersioning {
			t.Error("Versioning should be enabled")
		}
		if config.VersioningPattern != "{base}-{hash}{ext}" {
			t.Error("Custom pattern not set correctly")
		}
		if config.VersionHashLength != 16 {
			t.Error("Custom hash length not set correctly")
		}
		if len(config.StaticPrefixes) != 1 || config.StaticPrefixes[0] != "/assets/" {
			t.Error("Custom static prefixes not set correctly")
		}
	})
}

func TestPresetConfigurations(t *testing.T) {
	t.Run("DevelopmentPreset", func(t *testing.T) {
		config := NewWithPreset(PresetDevelopment)

		if !config.AllowBrowsing {
			t.Error("Development preset should enable directory browsing")
		}
		if !config.EnablePprof {
			t.Error("Development preset should enable pprof")
		}
		if !config.EnableWatcher {
			t.Error("Development preset should enable file watcher")
		}
		if config.CacheSize != 10<<20 {
			t.Error("Development preset should have smaller cache size")
		}
	})

	t.Run("ProductionPreset", func(t *testing.T) {
		config := NewWithPreset(PresetProduction)

		if !config.EnableMetrics {
			t.Error("Production preset should enable metrics")
		}
		if config.CacheSize != 500<<20 {
			t.Error("Production preset should have larger cache size")
		}
		if config.MaxConnections != 5000 {
			t.Error("Production preset should support more connections")
		}
	})

	t.Run("HighPerformancePreset", func(t *testing.T) {
		config := NewWithPreset(PresetHighPerformance)

		if config.CacheSize != 1<<30 {
			t.Error("High performance preset should have 1GB cache")
		}
		if config.MaxConnections != 10000 {
			t.Error("High performance preset should support 10k connections")
		}
		if config.RateLimitPerIP != 1000 {
			t.Error("High performance preset should have higher rate limit")
		}
	})
}

func TestConfigurationValidation(t *testing.T) {
	t.Run("InvalidHashLength", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Test with very small hash length
		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
			WithVersionHashLength(4), // Minimum allowed
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		// Should still work, just with short hashes
		if server.versionManager.hashLength != 4 {
			t.Error("Should accept small hash length")
		}
	})

	t.Run("EmptyStaticPrefixes", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
			WithStaticPrefixes(), // Empty
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		// Should use default prefixes when empty
		avm := server.versionManager
		if !avm.shouldVersionFile("/static/test.js") {
			t.Error("Should use default prefixes when none specified")
		}
	})

	t.Run("TimeoutConfigurations", func(t *testing.T) {
		timeouts := TimeoutConfig{
			Read:     5 * time.Second,
			Write:    10 * time.Second,
			Idle:     60 * time.Second,
			Header:   2 * time.Second,
			Shutdown: 30 * time.Second,
		}

		server, err := New(WithTimeouts(timeouts))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		config := server.config
		if config.ReadTimeout != 5*time.Second {
			t.Error("Read timeout not set correctly")
		}
		if config.WriteTimeout != 10*time.Second {
			t.Error("Write timeout not set correctly")
		}
		if config.IdleTimeout != 60*time.Second {
			t.Error("Idle timeout not set correctly")
		}
		if config.ReadHeaderTimeout != 2*time.Second {
			t.Error("Header timeout not set correctly")
		}
		if config.ShutdownTimeout != 30*time.Second {
			t.Error("Shutdown timeout not set correctly")
		}
	})
}

func TestCompressionConfiguration(t *testing.T) {
	t.Run("NoCompression", func(t *testing.T) {
		server, err := New(WithCompression(NoCompression))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.Compression != NoCompression {
			t.Error("Should disable compression")
		}
	})

	t.Run("GzipOnly", func(t *testing.T) {
		server, err := New(WithCompression(Gzip))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.Compression != Gzip {
			t.Error("Should enable only gzip")
		}
	})

	t.Run("BrotliOnly", func(t *testing.T) {
		server, err := New(WithCompression(Brotli))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.Compression != Brotli {
			t.Error("Should enable only brotli")
		}
	})

	t.Run("BothCompressions", func(t *testing.T) {
		server, err := New(WithCompression(Gzip | Brotli))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.Compression != (Gzip | Brotli) {
			t.Error("Should enable both compressions")
		}
	})

	t.Run("CompressionLevel", func(t *testing.T) {
		server, err := New(WithCompressionLevel(9))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.CompressionLevel != 9 {
			t.Error("Should set compression level")
		}
	})
}

func TestCacheConfiguration(t *testing.T) {
	t.Run("CacheSize", func(t *testing.T) {
		cacheSize := int64(500 * 1024 * 1024) // 500MB
		server, err := New(WithCache(cacheSize))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.CacheSize != cacheSize {
			t.Errorf("Expected cache size %d, got %d", cacheSize, server.config.CacheSize)
		}
	})

	t.Run("CacheTTL", func(t *testing.T) {
		ttl := 15 * time.Minute
		server, err := New(WithCacheTTL(ttl))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.CacheTTL != ttl {
			t.Errorf("Expected cache TTL %v, got %v", ttl, server.config.CacheTTL)
		}
	})

	t.Run("CacheStrategy", func(t *testing.T) {
		server, err := New(WithCacheStrategy(LFU))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.CacheStrategy != LFU {
			t.Error("Should set LFU cache strategy")
		}
	})
}

func TestRateLimitingConfiguration(t *testing.T) {
	t.Run("RateLimit", func(t *testing.T) {
		limit := 200
		server, err := New(WithRateLimit(limit))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.RateLimitPerIP != limit {
			t.Errorf("Expected rate limit %d, got %d", limit, server.config.RateLimitPerIP)
		}
	})

	t.Run("DisabledRateLimit", func(t *testing.T) {
		server, err := New(WithRateLimit(0))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.RateLimitPerIP != 0 {
			t.Error("Should disable rate limiting when set to 0")
		}
	})
}

func TestMetricsConfiguration(t *testing.T) {
	t.Run("EnableMetrics", func(t *testing.T) {
		server, err := New(WithMetrics(true))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if !server.config.EnableMetrics {
			t.Error("Should enable metrics")
		}

		if server.metrics == nil {
			t.Error("Should initialize metrics when enabled")
		}
	})

	t.Run("DisableMetrics", func(t *testing.T) {
		server, err := New(WithMetrics(false))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.EnableMetrics {
			t.Error("Should disable metrics")
		}

		if server.metrics != nil {
			t.Error("Should not initialize metrics when disabled")
		}
	})
}

func TestTLSConfiguration(t *testing.T) {
	t.Run("WithTLS", func(t *testing.T) {
		certFile := "cert.pem"
		keyFile := "key.pem"
		server, err := New(WithTLS(certFile, keyFile))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		config := server.config
		if !config.EnableHTTPS {
			t.Error("Should enable HTTPS")
		}
		if config.TLSCert != certFile {
			t.Errorf("Expected cert file %s, got %s", certFile, config.TLSCert)
		}
		if config.TLSKey != keyFile {
			t.Errorf("Expected key file %s, got %s", keyFile, config.TLSKey)
		}
	})
}

func TestWatcherConfiguration(t *testing.T) {
	t.Run("EnableWatcher", func(t *testing.T) {
		server, err := New(WithWatcher(true))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if !server.config.EnableWatcher {
			t.Error("Should enable file watcher")
		}
	})

	t.Run("DisableWatcher", func(t *testing.T) {
		server, err := New(WithWatcher(false))
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		if server.config.EnableWatcher {
			t.Error("Should disable file watcher")
		}
	})
}

func TestNewSimplifiedAPIs(t *testing.T) {
	t.Run("NewAutoServer", func(t *testing.T) {
		// Create test directory structure
		tempDir, err := os.MkdirTemp("", "gostc-auto-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create index.html and asset directories
		if err := os.WriteFile(filepath.Join(tempDir, "index.html"), []byte("<html></html>"), 0644); err != nil {
			t.Fatalf("Failed to create index.html: %v", err)
		}

		cssDir := filepath.Join(tempDir, "css")
		if err := os.MkdirAll(cssDir, 0755); err != nil {
			t.Fatalf("Failed to create css dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(cssDir, "style.css"), []byte("body{}"), 0644); err != nil {
			t.Fatalf("Failed to create style.css: %v", err)
		}

		server, err := NewAutoServer(tempDir)
		if err != nil {
			t.Fatalf("Failed to create auto server: %v", err)
		}

		// Should auto-detect versioning for SPA-like structure
		if !server.config.EnableVersioning {
			t.Error("Auto-detection should enable versioning for SPA structure")
		}
		if server.config.VersionHashLength != 8 {
			t.Errorf("Expected default hash length 8, got %d", server.config.VersionHashLength)
		}
	})

	t.Run("NewSimpleServer", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-simple-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		server, err := NewSimpleServer(SimpleConfig{
			Root:       tempDir,
			URLPrefix:  "/static",
			Versioning: true,
			Cache:      true,
			Compress:   true,
			Debug:      false,
		})
		if err != nil {
			t.Fatalf("Failed to create simple server: %v", err)
		}

		if !server.config.EnableVersioning {
			t.Error("Versioning should be enabled")
		}
		if server.config.URLPrefix != "/static" {
			t.Errorf("Expected URL prefix '/static', got %s", server.config.URLPrefix)
		}
		if server.config.VersionHashLength != 8 {
			t.Errorf("Expected hash length 8, got %d", server.config.VersionHashLength)
		}
	})

	t.Run("NewWithPresetServer", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-preset-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		server, err := NewWithPresetServer(PresetSPA, WithRoot(tempDir))
		if err != nil {
			t.Fatalf("Failed to create preset server: %v", err)
		}

		if !server.config.EnableVersioning {
			t.Error("SPA preset should enable versioning")
		}
		if server.config.VersionHashLength != 8 {
			t.Errorf("Expected hash length 8, got %d", server.config.VersionHashLength)
		}
		if server.config.StaticAssetMaxAge != 31536000 {
			t.Errorf("Expected long cache for static assets, got %d", server.config.StaticAssetMaxAge)
		}
	})

	t.Run("NewWithConfig", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-config-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		config := NewWithPreset(PresetStaticSite)
		config.Root = tempDir

		server, err := NewWithConfig(config)
		if err != nil {
			t.Fatalf("Failed to create server with config: %v", err)
		}

		if !server.config.EnableVersioning {
			t.Error("Static site preset should enable versioning")
		}
		if server.config.Root != tempDir {
			t.Error("Root should be set correctly")
		}
	})
}

func TestNewPresets(t *testing.T) {
	t.Run("PresetSPA", func(t *testing.T) {
		config := NewWithPreset(PresetSPA)

		if !config.EnableVersioning {
			t.Error("SPA preset should enable versioning")
		}
		if config.VersionHashLength != 8 {
			t.Errorf("Expected hash length 8, got %d", config.VersionHashLength)
		}
		if config.StaticAssetMaxAge != 31536000 {
			t.Error("SPA should have long cache for versioned assets")
		}
		if config.DynamicAssetMaxAge != 0 {
			t.Error("SPA should have no cache for HTML")
		}
	})

	t.Run("PresetStaticSite", func(t *testing.T) {
		config := NewWithPreset(PresetStaticSite)

		if !config.EnableVersioning {
			t.Error("Static site preset should enable versioning")
		}
		if len(config.StaticPrefixes) < 2 {
			t.Error("Static site should have multiple asset prefixes")
		}
		if config.StaticAssetMaxAge != 31536000 {
			t.Error("Static site should have long cache for versioned assets")
		}
	})

	t.Run("PresetAPI", func(t *testing.T) {
		config := NewWithPreset(PresetAPI)

		if config.EnableVersioning {
			t.Error("API preset should not enable versioning")
		}
		if !config.EnableMetrics {
			t.Error("API preset should enable metrics")
		}
		if config.DynamicAssetMaxAge != 0 {
			t.Error("API preset should have no cache for dynamic content")
		}
	})

	t.Run("PresetHybrid", func(t *testing.T) {
		config := NewWithPreset(PresetHybrid)

		if !config.EnableVersioning {
			t.Error("Hybrid preset should enable versioning")
		}
		if !config.EnableWatcher {
			t.Error("Hybrid preset should enable file watching")
		}
		if config.DynamicAssetMaxAge != 300 {
			t.Error("Hybrid preset should have moderate cache for dynamic content")
		}
	})
}

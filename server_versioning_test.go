package gostc

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerVersioningIntegration(t *testing.T) {
	// Create temporary directory with test files
	tempDir, err := os.MkdirTemp("", "gostc-versioning-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	staticDir := filepath.Join(tempDir, "static")
	os.MkdirAll(staticDir, 0755)

	testFiles := map[string]string{
		"static/app.js":    "console.log('Hello World');",
		"static/style.css": "body { background: #fff; }",
		"static/logo.png":  "fake png data",
		"index.html":       `<!DOCTYPE html><html><head><link href="/static/style.css" rel="stylesheet"><script src="/static/app.js"></script></head><body><img src="/static/logo.png" alt="Logo"></body></html>`,
	}

	for relativePath, content := range testFiles {
		fullPath := filepath.Join(tempDir, relativePath)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write test file %s: %v", relativePath, err)
		}
	}

	// Create server with versioning enabled
	server, err := New(
		WithRoot(tempDir),
		WithVersioning(true),
		WithVersionHashLength(16),
		WithStaticPrefixes("/static/"),
		WithCache(1024*1024), // 1MB cache
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	t.Run("ServeOriginalAssets", func(t *testing.T) {
		// Test serving original asset paths
		resp, err := http.Get(ts.URL + "/static/app.js")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "console.log") {
			t.Error("Should serve original JS content")
		}

		// Should have standard cache control (not immutable)
		cacheControl := resp.Header.Get("Cache-Control")
		if strings.Contains(cacheControl, "immutable") {
			t.Error("Original assets should not have immutable cache control")
		}
	})

	t.Run("ServeVersionedAssets", func(t *testing.T) {
		// Get versioned path for app.js
		versionedPath, exists := server.versionManager.GetVersionedPath("/static/app.js")
		if !exists {
			t.Fatal("Should have versioned path for app.js")
		}

		resp, err := http.Get(ts.URL + versionedPath)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "console.log") {
			t.Error("Should serve same content for versioned path")
		}

		// Should have immutable cache control
		cacheControl := resp.Header.Get("Cache-Control")
		if !strings.Contains(cacheControl, "immutable") {
			t.Error("Versioned assets should have immutable cache control")
		}
		if !strings.Contains(cacheControl, "max-age=31536000") {
			t.Error("Versioned assets should have 1 year max-age")
		}
	})

	t.Run("HTMLAssetInjection", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/index.html")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		html := string(body)

		// Should not contain original asset references
		if strings.Contains(html, `href="/static/style.css"`) {
			t.Error("HTML should not contain original CSS reference")
		}
		if strings.Contains(html, `src="/static/app.js"`) {
			t.Error("HTML should not contain original JS reference")
		}
		if strings.Contains(html, `src="/static/logo.png"`) {
			t.Error("HTML should not contain original image reference")
		}

		// Should contain versioned references
		cssVersioned, _ := server.versionManager.GetVersionedPath("/static/style.css")
		jsVersioned, _ := server.versionManager.GetVersionedPath("/static/app.js")
		imgVersioned, _ := server.versionManager.GetVersionedPath("/static/logo.png")

		if !strings.Contains(html, cssVersioned) {
			t.Error("HTML should contain versioned CSS reference")
		}
		if !strings.Contains(html, jsVersioned) {
			t.Error("HTML should contain versioned JS reference")
		}
		if !strings.Contains(html, imgVersioned) {
			t.Error("HTML should contain versioned image reference")
		}
	})

	t.Run("NonExistentVersionedAsset", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/static/nonexistent.abcd1234.js")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}
	})

	t.Run("CacheConsistency", func(t *testing.T) {
		// Test that both original and versioned paths cache correctly
		originalURL := ts.URL + "/static/style.css"
		versionedPath, _ := server.versionManager.GetVersionedPath("/static/style.css")
		versionedURL := ts.URL + versionedPath

		// Request original
		resp1, err := http.Get(originalURL)
		if err != nil {
			t.Fatalf("Failed to get original URL: %v", err)
		}
		defer resp1.Body.Close()
		body1, _ := io.ReadAll(resp1.Body)

		// Request versioned
		resp2, err := http.Get(versionedURL)
		if err != nil {
			t.Fatalf("Failed to get versioned URL: %v", err)
		}
		defer resp2.Body.Close()
		body2, _ := io.ReadAll(resp2.Body)

		// Content should be identical
		if string(body1) != string(body2) {
			t.Error("Original and versioned paths should serve identical content")
		}

		// ETags should be different due to different cache keys
		etag1 := resp1.Header.Get("ETag")
		etag2 := resp2.Header.Get("ETag")
		if etag1 != etag2 {
			// This is actually expected due to different cache keys
			t.Logf("ETags differ as expected: original=%s, versioned=%s", etag1, etag2)
		}
	})
}

func TestVersioningWithDisabledFeature(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostc-no-versioning-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	staticDir := filepath.Join(tempDir, "static")
	os.MkdirAll(staticDir, 0755)
	os.WriteFile(filepath.Join(staticDir, "app.js"), []byte("console.log('test');"), 0644)
	os.WriteFile(filepath.Join(tempDir, "index.html"), []byte(`<script src="/static/app.js"></script>`), 0644)

	// Create server with versioning disabled
	server, err := New(
		WithRoot(tempDir),
		WithVersioning(false),
		WithCache(1024*1024),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	t.Run("NoVersionedPaths", func(t *testing.T) {
		// Should not have any versioned paths
		_, exists := server.versionManager.GetVersionedPath("/static/app.js")
		if exists {
			t.Error("Should not have versioned paths when versioning is disabled")
		}
	})

	t.Run("HTMLNotProcessed", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/index.html")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		html := string(body)

		if !strings.Contains(html, `src="/static/app.js"`) {
			t.Error("HTML should contain original asset references when versioning is disabled")
		}
	})
}

func TestVersioningErrorConditions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostc-error-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("InvalidDirectoryScan", func(t *testing.T) {
		config := &Config{
			EnableVersioning:  true,
			VersionHashLength: 16,
			StaticPrefixes:    []string{"/static/"},
		}

		avm := NewAssetVersionManager(config)
		err := avm.ScanDirectory("/nonexistent/directory")
		if err == nil {
			t.Error("Should return error for nonexistent directory")
		}
	})

	t.Run("EmptyContent", func(t *testing.T) {
		config := &Config{
			EnableVersioning:  true,
			VersionHashLength: 16,
		}

		avm := NewAssetVersionManager(config)
		versionedPath, hash := avm.GenerateVersionedPath("/static/empty.js", []byte{})

		if hash == "" {
			t.Error("Should generate hash even for empty content")
		}
		if !strings.Contains(versionedPath, hash) {
			t.Error("Versioned path should contain hash even for empty content")
		}
	})

	t.Run("ZeroHashLength", func(t *testing.T) {
		config := &Config{
			EnableVersioning:  true,
			VersionHashLength: 0, // Should default to 16
		}

		avm := NewAssetVersionManager(config)
		if avm.hashLength != 16 {
			t.Errorf("Expected default hash length 16, got %d", avm.hashLength)
		}
	})
}

func TestVersioningWithCompression(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostc-compression-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create large CSS file that will benefit from compression
	staticDir := filepath.Join(tempDir, "static")
	os.MkdirAll(staticDir, 0755)

	largeCSS := strings.Repeat("body { margin: 0; padding: 0; } ", 1000)
	os.WriteFile(filepath.Join(staticDir, "large.css"), []byte(largeCSS), 0644)

	server, err := New(
		WithRoot(tempDir),
		WithVersioning(true),
		WithCompression(Gzip|Brotli),
		WithStaticPrefixes("/static/"),
		WithCache(1024*1024),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	t.Run("VersionedAssetWithGzipCompression", func(t *testing.T) {
		versionedPath, exists := server.versionManager.GetVersionedPath("/static/large.css")
		if !exists {
			t.Fatal("Should have versioned path for large.css")
		}

		req, _ := http.NewRequest("GET", ts.URL+versionedPath, nil)
		req.Header.Set("Accept-Encoding", "gzip")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Should be compressed
		if resp.Header.Get("Content-Encoding") != "gzip" {
			t.Error("Should serve gzip compressed content")
		}

		// Should have immutable cache control
		cacheControl := resp.Header.Get("Cache-Control")
		if !strings.Contains(cacheControl, "immutable") {
			t.Error("Compressed versioned assets should have immutable cache control")
		}
	})
}

func TestCustomVersioningPattern(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostc-pattern-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	staticDir := filepath.Join(tempDir, "static")
	os.MkdirAll(staticDir, 0755)
	os.WriteFile(filepath.Join(staticDir, "main.js"), []byte("console.log('main');"), 0644)

	server, err := New(
		WithRoot(tempDir),
		WithVersioning(true),
		WithVersioningPattern("{base}-{hash}{ext}"),
		WithVersionHashLength(12),
		WithStaticPrefixes("/static/"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	t.Run("CustomPatternServing", func(t *testing.T) {
		versionedPath, exists := server.versionManager.GetVersionedPath("/static/main.js")
		if !exists {
			t.Fatal("Should have versioned path")
		}

		// Should follow custom pattern
		if !strings.Contains(versionedPath, "/static/main-") {
			t.Errorf("Versioned path should follow custom pattern: %s", versionedPath)
		}
		if !strings.HasSuffix(versionedPath, ".js") {
			t.Errorf("Should maintain .js extension: %s", versionedPath)
		}

		// Should serve correctly
		resp, err := http.Get(ts.URL + versionedPath)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "console.log") {
			t.Error("Should serve correct content")
		}
	})
}

func TestVersioningCacheInvalidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cache invalidation test in short mode")
	}

	tempDir, err := os.MkdirTemp("", "gostc-invalidation-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	staticDir := filepath.Join(tempDir, "static")
	os.MkdirAll(staticDir, 0755)
	testFile := filepath.Join(staticDir, "dynamic.js")
	originalContent := "console.log('original');"
	os.WriteFile(testFile, []byte(originalContent), 0644)

	server, err := New(
		WithRoot(tempDir),
		WithVersioning(true),
		WithStaticPrefixes("/static/"),
		WithWatcher(true),
		WithCache(1024*1024),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	ts := httptest.NewServer(server)
	defer ts.Close()

	// Get initial versioned path
	originalVersionedPath, exists := server.versionManager.GetVersionedPath("/static/dynamic.js")
	if !exists {
		t.Fatal("Should have initial versioned path")
	}

	// Modify file content
	newContent := "console.log('updated');"
	time.Sleep(10 * time.Millisecond) // Ensure file modification time changes
	if err := os.WriteFile(testFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to update file: %v", err)
	}

	// Give file watcher time to detect change
	time.Sleep(100 * time.Millisecond)

	// Get new versioned path
	newVersionedPath, exists := server.versionManager.GetVersionedPath("/static/dynamic.js")
	if !exists {
		t.Fatal("Should have new versioned path after update")
	}

	// Versioned paths should be different due to content change
	if originalVersionedPath == newVersionedPath {
		t.Error("Versioned path should change when content changes")
	}

	// New versioned path should serve updated content
	resp, err := http.Get(ts.URL + newVersionedPath)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "updated") {
		t.Error("Should serve updated content at new versioned path")
	}
}

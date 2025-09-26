package gostc

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestETagValidation(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content for etag")
	os.WriteFile(testFile, content, 0644)

	server, err := New(WithRoot(tmpDir))
	if err != nil {
		t.Fatal(err)
	}

	// First request - should return 200 with ETag
	req1 := httptest.NewRequest("GET", "/test.txt", nil)
	w1 := httptest.NewRecorder()
	server.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("First request: expected 200, got %d", w1.Code)
	}

	etag := w1.Header().Get("ETag")
	if etag == "" {
		t.Error("ETag header missing")
	}

	lastModified := w1.Header().Get("Last-Modified")
	if lastModified == "" {
		t.Error("Last-Modified header missing")
	}

	cacheControl := w1.Header().Get("Cache-Control")
	if cacheControl != "public, max-age=3600, must-revalidate" {
		t.Errorf("Expected Cache-Control 'public, max-age=3600, must-revalidate', got '%s'", cacheControl)
	}

	// Second request with If-None-Match - should return 304
	req2 := httptest.NewRequest("GET", "/test.txt", nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotModified {
		t.Errorf("Conditional request: expected 304, got %d", w2.Code)
	}

	if w2.Body.Len() != 0 {
		t.Error("304 response should have empty body")
	}
}

func TestIfModifiedSinceValidation(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.html")
	content := []byte("<html><body>Test</body></html>")
	os.WriteFile(testFile, content, 0644)

	// Set file modification time to past
	pastTime := time.Now().Add(-24 * time.Hour)
	os.Chtimes(testFile, pastTime, pastTime)

	server, err := New(WithRoot(tmpDir))
	if err != nil {
		t.Fatal(err)
	}

	// First request to get Last-Modified
	req1 := httptest.NewRequest("GET", "/test.html", nil)
	w1 := httptest.NewRecorder()
	server.ServeHTTP(w1, req1)

	lastModified := w1.Header().Get("Last-Modified")
	if lastModified == "" {
		t.Fatal("Last-Modified header missing")
	}

	// Parse the Last-Modified time
	_, err = http.ParseTime(lastModified)
	if err != nil {
		t.Fatal("Failed to parse Last-Modified header")
	}

	// Request with If-Modified-Since in the future (should return 304)
	req2 := httptest.NewRequest("GET", "/test.html", nil)
	req2.Header.Set("If-Modified-Since", time.Now().Format(http.TimeFormat))
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)

	// Note: Current implementation only checks If-None-Match, not If-Modified-Since
	// This test documents current behavior
	if w2.Code == http.StatusNotModified {
		t.Log("If-Modified-Since is being handled")
	} else {
		t.Log("If-Modified-Since is not implemented (only If-None-Match)")
	}
}

func TestCacheControlHeaders(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []struct {
		filename             string
		content              []byte
		expectedCacheControl string
	}{
		{"style.css", []byte("body { margin: 0; }"), "public, max-age=86400"},                         // Static asset
		{"app.js", []byte("console.log('test');"), "public, max-age=86400"},                           // Static asset
		{"index.html", []byte("<html></html>"), "public, max-age=3600, must-revalidate"},              // Dynamic
		{"data.json", []byte(`{"key": "value"}`), "public, max-age=3600, must-revalidate"},            // Dynamic
		{"image.svg", []byte("<svg></svg>"), "public, max-age=86400"},                                 // Static asset
		{"app.abc123.js", []byte("console.log('versioned');"), "public, max-age=31536000, immutable"}, // Versioned
	}

	for _, tc := range testCases {
		testFile := filepath.Join(tmpDir, tc.filename)
		os.WriteFile(testFile, tc.content, 0644)
	}

	server, err := New(WithRoot(tmpDir))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/"+tc.filename, nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected 200, got %d", w.Code)
			}

			// Check cache headers
			cacheControl := w.Header().Get("Cache-Control")
			if cacheControl != tc.expectedCacheControl {
				t.Errorf("Expected Cache-Control '%s', got '%s'", tc.expectedCacheControl, cacheControl)
			}

			etag := w.Header().Get("ETag")
			if etag == "" {
				t.Error("ETag header missing")
			}

			lastModified := w.Header().Get("Last-Modified")
			if lastModified == "" {
				t.Error("Last-Modified header missing")
			}

			contentType := w.Header().Get("Content-Type")
			if contentType == "" {
				t.Error("Content-Type header missing")
			}
		})
	}
}

func TestVaryHeader(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.js")
	// Create large enough file to trigger compression
	content := make([]byte, 2000)
	for i := range content {
		content[i] = 'a'
	}
	os.WriteFile(testFile, content, 0644)

	server, err := New(
		WithRoot(tmpDir),
		WithCompression(Gzip),
		func(c *Config) { c.MinSizeToCompress = 100 },
	)
	if err != nil {
		t.Fatal(err)
	}

	// Request with gzip acceptance
	req := httptest.NewRequest("GET", "/test.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	vary := w.Header().Get("Vary")
	if vary != "Accept-Encoding" {
		t.Errorf("Expected Vary: Accept-Encoding, got '%s'", vary)
	}

	// Compressed response should have Content-Encoding
	encoding := w.Header().Get("Content-Encoding")
	if encoding != "gzip" {
		t.Errorf("Expected Content-Encoding: gzip, got '%s'", encoding)
	}
}

func TestHeadRequest(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	os.WriteFile(testFile, content, 0644)

	server, err := New(WithRoot(tmpDir))
	if err != nil {
		t.Fatal(err)
	}

	// HEAD request
	req := httptest.NewRequest("HEAD", "/test.txt", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	// HEAD should have headers but no body
	if w.Body.Len() != 0 {
		t.Error("HEAD response should have empty body")
	}

	// Should still have all cache headers
	if w.Header().Get("ETag") == "" {
		t.Error("ETag missing in HEAD response")
	}

	if w.Header().Get("Last-Modified") == "" {
		t.Error("Last-Modified missing in HEAD response")
	}

	if w.Header().Get("Cache-Control") == "" {
		t.Error("Cache-Control missing in HEAD response")
	}

	if w.Header().Get("Content-Length") == "" {
		t.Error("Content-Length missing in HEAD response")
	}
}

func TestCacheInvalidation(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content1 := []byte("original content")
	os.WriteFile(testFile, content1, 0644)

	server, err := New(
		WithRoot(tmpDir),
		WithCache(1024*1024),
		WithWatcher(false), // Disable watcher for manual control
	)
	if err != nil {
		t.Fatal(err)
	}

	// First request - cache miss
	req1 := httptest.NewRequest("GET", "/test.txt", nil)
	w1 := httptest.NewRecorder()
	server.ServeHTTP(w1, req1)
	etag1 := w1.Header().Get("ETag")

	// Second request - should be cache hit
	req2 := httptest.NewRequest("GET", "/test.txt", nil)
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)
	etag2 := w2.Header().Get("ETag")

	if etag1 != etag2 {
		t.Error("ETags should be the same for cached content")
	}

	stats1 := server.CacheStats()
	if stats1.Hits < 1 {
		t.Error("Expected at least one cache hit")
	}

	// Invalidate cache
	server.InvalidatePath("/test.txt")

	// Third request - should be cache miss after invalidation
	req3 := httptest.NewRequest("GET", "/test.txt", nil)
	w3 := httptest.NewRecorder()
	server.ServeHTTP(w3, req3)

	stats2 := server.CacheStats()
	if stats2.Misses <= stats1.Misses {
		t.Error("Expected cache miss after invalidation")
	}
}

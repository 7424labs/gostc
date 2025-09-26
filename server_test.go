package gostc

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
)

func TestServerBasicServing(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.html")
	content := []byte("<html><body>Hello World</body></html>")

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	server, err := New(
		WithRoot(tmpDir),
		WithCompression(NoCompression),
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/test.html", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if !bytes.Equal(w.Body.Bytes(), content) {
		t.Errorf("Content mismatch. Expected %s, got %s", content, w.Body.Bytes())
	}
}

func TestGzipCompression(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.js")
	content := bytes.Repeat([]byte(`const message = "Hello World"; console.log(message); `), 20)

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	server, err := New(
		WithRoot(tmpDir),
		WithCompression(Gzip),
		WithCompressionLevel(6),
		func(c *Config) { c.MinSizeToCompress = 10 },
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/test.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Error("Expected gzip encoding")
	}

	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()

	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(decompressed, content) {
		t.Errorf("Content mismatch after decompression")
	}
}

func TestBrotliCompression(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.css")
	content := bytes.Repeat([]byte(`body { margin: 0; padding: 0; font-family: Arial; } `), 20)

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	server, err := New(
		WithRoot(tmpDir),
		WithCompression(Brotli),
		func(c *Config) { c.MinSizeToCompress = 10 },
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/test.css", nil)
	req.Header.Set("Accept-Encoding", "br")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Encoding") != "br" {
		t.Error("Expected br encoding")
	}

	br := brotli.NewReader(w.Body)
	decompressed, err := io.ReadAll(br)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(decompressed, content) {
		t.Errorf("Content mismatch after decompression")
	}
}

func TestCache(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("This is cached content")

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	server, err := New(
		WithRoot(tmpDir),
		WithCache(1024*1024),
		WithCacheTTL(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}

	req1 := httptest.NewRequest("GET", "/test.txt", nil)
	w1 := httptest.NewRecorder()
	server.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest("GET", "/test.txt", nil)
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)

	stats := server.CacheStats()
	if stats.Hits < 1 {
		t.Error("Expected at least one cache hit")
	}

	if !bytes.Equal(w1.Body.Bytes(), w2.Body.Bytes()) {
		t.Error("Cached response doesn't match original")
	}
}

func TestETagSupport(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.json")
	content := []byte(`{"key": "value"}`)

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	server, err := New(WithRoot(tmpDir))
	if err != nil {
		t.Fatal(err)
	}

	req1 := httptest.NewRequest("GET", "/test.json", nil)
	w1 := httptest.NewRecorder()
	server.ServeHTTP(w1, req1)

	etag := w1.Header().Get("ETag")
	if etag == "" {
		t.Error("Expected ETag header")
	}

	req2 := httptest.NewRequest("GET", "/test.json", nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotModified {
		t.Errorf("Expected 304 Not Modified, got %d", w2.Code)
	}
}

func TestRateLimiting(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	server, err := New(
		WithRoot(tmpDir),
		WithRateLimit(2),
	)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test.txt", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if i < 2 {
			if w.Code != http.StatusOK {
				t.Errorf("Request %d: Expected 200, got %d", i, w.Code)
			}
		}
	}
}

func TestCORS(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	server, err := New(
		WithRoot(tmpDir),
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("OPTIONS", "/test.txt", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for OPTIONS, got %d", w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("Expected CORS headers")
	}
}

func TestDirectoryListing(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("2"), 0644)

	server, err := New(
		WithRoot(tmpDir),
		func(c *Config) { c.AllowBrowsing = true },
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("file1.txt")) ||
		!bytes.Contains([]byte(body), []byte("file2.txt")) {
		t.Error("Expected directory listing")
	}
}

func TestSecurityHeaders(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.html")
	os.WriteFile(testFile, []byte("<html></html>"), 0644)

	server, err := New(WithRoot(tmpDir))
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/test.html", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	securityHeaders := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Referrer-Policy",
	}

	for _, header := range securityHeaders {
		if w.Header().Get(header) == "" {
			t.Errorf("Missing security header: %s", header)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	server, err := New()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for health check, got %d", w.Code)
	}

	if w.Body.String() != "OK" {
		t.Errorf("Expected 'OK' response, got %s", w.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	server, err := New(WithRoot(tmpDir))
	if err != nil {
		t.Fatal(err)
	}

	methods := []string{"POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		req := httptest.NewRequest(method, "/test.txt", nil)
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Method %s: Expected 405, got %d", method, w.Code)
		}
	}
}

func BenchmarkServeFile(b *testing.B) {
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := bytes.Repeat([]byte("Hello World "), 100)
	os.WriteFile(testFile, content, 0644)

	server, _ := New(
		WithRoot(tmpDir),
		WithCache(10*1024*1024),
	)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/test.txt", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
	}
}

func BenchmarkGzipCompression(b *testing.B) {
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test.js")
	content := bytes.Repeat([]byte("var x = 'test'; "), 1000)
	os.WriteFile(testFile, content, 0644)

	server, _ := New(
		WithRoot(tmpDir),
		WithCompression(Gzip),
		WithCache(10*1024*1024),
	)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/test.js", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
	}
}

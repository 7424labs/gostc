package gostc

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// BenchmarkStaticFileServing tests serving small static files
func BenchmarkStaticFileServing(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			tmpDir := b.TempDir()
			testFile := filepath.Join(tmpDir, "test.txt")
			content := bytes.Repeat([]byte("a"), size.size)
			os.WriteFile(testFile, content, 0644)

			server, _ := New(
				WithRoot(tmpDir),
				WithCache(100*1024*1024), // 100MB cache
			)

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					req := httptest.NewRequest("GET", "/test.txt", nil)
					w := httptest.NewRecorder()
					server.ServeHTTP(w, req)
				}
			})

			b.SetBytes(int64(size.size))
		})
	}
}

// BenchmarkCompressionMethods compares different compression methods
func BenchmarkCompressionMethods(b *testing.B) {
	methods := []struct {
		name        string
		compression CompressionType
		encoding    string
	}{
		{"NoCompression", NoCompression, ""},
		{"Gzip", Gzip, "gzip"},
		{"Brotli", Brotli, "br"},
	}

	for _, method := range methods {
		b.Run(method.name, func(b *testing.B) {
			tmpDir := b.TempDir()
			testFile := filepath.Join(tmpDir, "test.js")
			// JavaScript-like content that compresses well
			content := bytes.Repeat([]byte("function test() { return 'Hello World'; }\n"), 100)
			os.WriteFile(testFile, content, 0644)

			server, _ := New(
				WithRoot(tmpDir),
				WithCompression(method.compression),
				WithCache(10*1024*1024),
				func(c *Config) { c.MinSizeToCompress = 100 },
			)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("GET", "/test.js", nil)
				if method.encoding != "" {
					req.Header.Set("Accept-Encoding", method.encoding)
				}
				w := httptest.NewRecorder()
				server.ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkCacheHitRate measures cache performance
func BenchmarkCacheHitRate(b *testing.B) {
	tmpDir := b.TempDir()

	// Create multiple files
	for i := 0; i < 10; i++ {
		testFile := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		content := bytes.Repeat([]byte(fmt.Sprintf("content %d ", i)), 100)
		os.WriteFile(testFile, content, 0644)
	}

	server, _ := New(
		WithRoot(tmpDir),
		WithCache(10*1024*1024),
		WithCompression(Gzip),
	)

	// Warm up cache
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", fmt.Sprintf("/file%d.txt", i), nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Round-robin through files to test cache
			req := httptest.NewRequest("GET", fmt.Sprintf("/file%d.txt", i%10), nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)
			i++
		}
	})

	// Report cache statistics
	stats := server.CacheStats()
	b.ReportMetric(float64(stats.Hits)/(float64(stats.Hits+stats.Misses))*100, "%_cache_hit_rate")
}

// BenchmarkConditionalRequests tests 304 response performance
func BenchmarkConditionalRequests(b *testing.B) {
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test.html")
	content := []byte("<html><body>Test Content</body></html>")
	os.WriteFile(testFile, content, 0644)

	server, _ := New(WithRoot(tmpDir))

	// Get ETag first
	req := httptest.NewRequest("GET", "/test.html", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	etag := w.Header().Get("ETag")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/test.html", nil)
		req.Header.Set("If-None-Match", etag)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusNotModified {
			b.Fatalf("Expected 304, got %d", w.Code)
		}
	}
}

// BenchmarkConcurrentRequests tests server under concurrent load
func BenchmarkConcurrentRequests(b *testing.B) {
	concurrencyLevels := []int{1, 10, 100, 1000}

	for _, level := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrent_%d", level), func(b *testing.B) {
			tmpDir := b.TempDir()

			// Create test files
			for i := 0; i < 5; i++ {
				testFile := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
				content := bytes.Repeat([]byte("test content "), 100)
				os.WriteFile(testFile, content, 0644)
			}

			server, _ := New(
				WithRoot(tmpDir),
				WithCache(50*1024*1024),
				WithRateLimit(10000), // High limit for benchmarking
			)

			b.SetParallelism(level)
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					req := httptest.NewRequest("GET", fmt.Sprintf("/file%d.txt", i%5), nil)
					w := httptest.NewRecorder()
					server.ServeHTTP(w, req)
					i++
				}
			})
		})
	}
}

// BenchmarkMemoryUsage measures memory allocation
func BenchmarkMemoryUsage(b *testing.B) {
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := bytes.Repeat([]byte("a"), 10*1024) // 10KB file
	os.WriteFile(testFile, content, 0644)

	server, _ := New(
		WithRoot(tmpDir),
		WithCache(10*1024*1024),
	)

	b.ResetTimer()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	allocBefore := m.Alloc

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/test.txt", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
	}

	runtime.ReadMemStats(&m)
	allocAfter := m.Alloc

	b.ReportMetric(float64(allocAfter-allocBefore)/float64(b.N), "bytes/op")
}

// TestPerformanceMetrics runs a comprehensive performance test
func TestPerformanceMetrics(t *testing.T) {
	tmpDir := t.TempDir()

	// Create various test files
	files := map[string]int{
		"small.txt":  1024,        // 1KB
		"medium.txt": 100 * 1024,  // 100KB
		"large.txt":  1024 * 1024, // 1MB
	}

	for name, size := range files {
		content := bytes.Repeat([]byte("a"), size)
		os.WriteFile(filepath.Join(tmpDir, name), content, 0644)
	}

	server, err := New(
		WithRoot(tmpDir),
		WithCache(10*1024*1024),
		WithCompression(Gzip|Brotli),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Measure response times
	results := make(map[string]time.Duration)

	for name := range files {
		// First request (cache miss)
		start := time.Now()
		req := httptest.NewRequest("GET", "/"+name, nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		firstDuration := time.Since(start)

		// Second request (cache hit)
		start = time.Now()
		req = httptest.NewRequest("GET", "/"+name, nil)
		w = httptest.NewRecorder()
		server.ServeHTTP(w, req)
		cachedDuration := time.Since(start)

		results[name+"_first"] = firstDuration
		results[name+"_cached"] = cachedDuration

		t.Logf("%s: First request: %v, Cached: %v (%.2fx faster)",
			name, firstDuration, cachedDuration,
			float64(firstDuration)/float64(cachedDuration))
	}

	// Check cache effectiveness
	stats := server.CacheStats()
	hitRate := float64(stats.Hits) / float64(stats.Hits+stats.Misses) * 100
	t.Logf("Cache hit rate: %.2f%% (%d hits, %d misses)",
		hitRate, stats.Hits, stats.Misses)

	if hitRate < 50 {
		t.Errorf("Cache hit rate too low: %.2f%%", hitRate)
	}

	// Verify cached requests are faster
	for name := range files {
		first := results[name+"_first"]
		cached := results[name+"_cached"]
		if cached > first {
			t.Errorf("%s: Cached request slower than first request", name)
		}
	}
}

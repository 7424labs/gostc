package gostc

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkVersioningOperations(b *testing.B) {
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
		StaticPrefixes:    []string{"/static/"},
	}

	avm := NewAssetVersionManager(config)
	content := []byte("console.log('benchmark test content that is reasonably long to simulate real file content');")

	b.Run("RegisterAsset", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			path := fmt.Sprintf("/static/test%d.js", i)
			avm.RegisterAsset(path, content)
		}
	})

	b.Run("GetVersionedPath", func(b *testing.B) {
		// Pre-register some assets
		for i := 0; i < 1000; i++ {
			path := fmt.Sprintf("/static/bench%d.js", i)
			avm.RegisterAsset(path, content)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			path := fmt.Sprintf("/static/bench%d.js", i%1000)
			avm.GetVersionedPath(path)
		}
	})

	b.Run("GetOriginalPath", func(b *testing.B) {
		// Pre-register assets and get their versioned paths
		versionedPaths := make([]string, 1000)
		for i := 0; i < 1000; i++ {
			path := fmt.Sprintf("/static/reverse%d.js", i)
			avm.RegisterAsset(path, content)
			versionedPaths[i], _ = avm.GetVersionedPath(path)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			versionedPath := versionedPaths[i%1000]
			avm.GetOriginalPath(versionedPath)
		}
	})

	b.Run("IsVersionedPath", func(b *testing.B) {
		// Pre-register assets
		versionedPaths := make([]string, 500)
		originalPaths := make([]string, 500)
		for i := 0; i < 500; i++ {
			originalPath := fmt.Sprintf("/static/check%d.js", i)
			avm.RegisterAsset(originalPath, content)
			versionedPaths[i], _ = avm.GetVersionedPath(originalPath)
			originalPaths[i] = originalPath
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if i%2 == 0 {
				avm.IsVersionedPath(versionedPaths[i%500])
			} else {
				avm.IsVersionedPath(originalPaths[i%500])
			}
		}
	})
}

func BenchmarkHTMLProcessingAdvanced(b *testing.B) {
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
		StaticPrefixes:    []string{"/static/"},
	}

	avm := NewAssetVersionManager(config)
	processor := NewHTMLProcessor(avm)

	// Register multiple assets
	for i := 0; i < 50; i++ {
		jsPath := fmt.Sprintf("/static/app%d.js", i)
		cssPath := fmt.Sprintf("/static/style%d.css", i)
		imgPath := fmt.Sprintf("/static/image%d.png", i)

		avm.RegisterAsset(jsPath, []byte(fmt.Sprintf("console.log('app%d');", i)))
		avm.RegisterAsset(cssPath, []byte(fmt.Sprintf("body { color: #%06d; }", i)))
		avm.RegisterAsset(imgPath, []byte("fake image data"))
	}

	b.Run("SimpleHTML", func(b *testing.B) {
		html := []byte(`<!DOCTYPE html>
<html>
<head>
    <link href="/static/style0.css" rel="stylesheet">
    <script src="/static/app0.js"></script>
</head>
<body>
    <img src="/static/image0.png" alt="Test">
</body>
</html>`)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			processor.ProcessHTML(html, "/index.html")
		}
	})

	b.Run("ComplexHTML", func(b *testing.B) {
		// Generate HTML with many asset references
		var htmlBuilder strings.Builder
		htmlBuilder.WriteString(`<!DOCTYPE html><html><head>`)
		for i := 0; i < 20; i++ {
			htmlBuilder.WriteString(fmt.Sprintf(`<link href="/static/style%d.css" rel="stylesheet">`, i))
			htmlBuilder.WriteString(fmt.Sprintf(`<script src="/static/app%d.js"></script>`, i))
		}
		htmlBuilder.WriteString(`</head><body>`)
		for i := 0; i < 30; i++ {
			htmlBuilder.WriteString(fmt.Sprintf(`<img src="/static/image%d.png" alt="Image %d">`, i, i))
		}
		htmlBuilder.WriteString(`</body></html>`)

		html := []byte(htmlBuilder.String())

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			processor.ProcessHTML(html, "/complex.html")
		}
	})

	b.Run("HTMLWithoutAssets", func(b *testing.B) {
		html := []byte(`<!DOCTYPE html>
<html>
<head>
    <title>No Assets</title>
    <style>body { margin: 0; }</style>
</head>
<body>
    <h1>Hello World</h1>
    <p>This page has no external assets.</p>
    <script>console.log('inline script');</script>
</body>
</html>`)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			processor.ProcessHTML(html, "/simple.html")
		}
	})
}

func BenchmarkServerVersioningIntegration(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "gostc-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	staticDir := filepath.Join(tempDir, "static")
	os.MkdirAll(staticDir, 0755)

	testFiles := []string{"app.js", "style.css", "image.png"}
	for i, filename := range testFiles {
		content := fmt.Sprintf("/* File %d: %s */", i, filename)
		content += strings.Repeat(" padding", 100) // Make files reasonably sized
		os.WriteFile(filepath.Join(staticDir, filename), []byte(content), 0644)
	}

	server, err := New(
		WithRoot(tempDir),
		WithVersioning(true),
		WithCompression(Gzip|Brotli),
		WithStaticPrefixes("/static/"),
		WithCache(10*1024*1024), // 10MB cache
	)
	if err != nil {
		b.Fatalf("Failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	b.Run("ServeOriginalAssets", func(b *testing.B) {
		urls := []string{
			ts.URL + "/static/app.js",
			ts.URL + "/static/style.css",
			ts.URL + "/static/image.png",
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			url := urls[i%len(urls)]
			resp, err := http.Get(url)
			if err != nil {
				b.Fatalf("Request failed: %v", err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})

	b.Run("ServeVersionedAssets", func(b *testing.B) {
		// Get versioned URLs
		versionedURLs := make([]string, len(testFiles))
		for i, filename := range testFiles {
			path := "/static/" + filename
			versionedPath, exists := server.versionManager.GetVersionedPath(path)
			if !exists {
				b.Fatalf("No versioned path for %s", filename)
			}
			versionedURLs[i] = ts.URL + versionedPath
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			url := versionedURLs[i%len(versionedURLs)]
			resp, err := http.Get(url)
			if err != nil {
				b.Fatalf("Request failed: %v", err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})

	b.Run("ServeVersionedAssetsWithCompression", func(b *testing.B) {
		versionedURLs := make([]string, len(testFiles))
		for i, filename := range testFiles {
			path := "/static/" + filename
			versionedPath, exists := server.versionManager.GetVersionedPath(path)
			if !exists {
				b.Fatalf("No versioned path for %s", filename)
			}
			versionedURLs[i] = ts.URL + versionedPath
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			url := versionedURLs[i%len(versionedURLs)]
			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("Accept-Encoding", "gzip, br")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				b.Fatalf("Request failed: %v", err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}

func BenchmarkVersioningMemoryUsage(b *testing.B) {
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
		StaticPrefixes:    []string{"/static/"},
	}

	b.Run("AssetRegistration", func(b *testing.B) {
		avm := NewAssetVersionManager(config)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			path := fmt.Sprintf("/static/file%d.js", i)
			content := []byte(fmt.Sprintf("content for file %d", i))
			avm.RegisterAsset(path, content)
		}
	})

	b.Run("HTMLProcessing", func(b *testing.B) {
		avm := NewAssetVersionManager(config)
		processor := NewHTMLProcessor(avm)

		// Pre-register assets
		for i := 0; i < 10; i++ {
			path := fmt.Sprintf("/static/asset%d.js", i)
			avm.RegisterAsset(path, []byte("test content"))
		}

		html := []byte(`<!DOCTYPE html>
<html><head>
<link href="/static/asset0.js" rel="stylesheet">
<link href="/static/asset1.js" rel="stylesheet">
<link href="/static/asset2.js" rel="stylesheet">
</head></html>`)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			processor.ProcessHTML(html, "/test.html")
		}
	})
}

func BenchmarkVersionHashGeneration(b *testing.B) {
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
	}

	avm := NewAssetVersionManager(config)

	b.Run("SmallFiles", func(b *testing.B) {
		content := []byte("small file content")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			avm.GenerateVersionedPath("/static/small.js", content)
		}
	})

	b.Run("MediumFiles", func(b *testing.B) {
		content := []byte(strings.Repeat("medium file content ", 500)) // ~10KB
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			avm.GenerateVersionedPath("/static/medium.js", content)
		}
	})

	b.Run("LargeFiles", func(b *testing.B) {
		content := []byte(strings.Repeat("large file content ", 50000)) // ~1MB
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			avm.GenerateVersionedPath("/static/large.js", content)
		}
	})

	b.Run("DifferentHashLengths", func(b *testing.B) {
		content := []byte("test content for hash length comparison")

		hashLengths := []int{8, 16, 32, 64}

		for _, length := range hashLengths {
			b.Run(fmt.Sprintf("HashLength%d", length), func(b *testing.B) {
				config := &Config{
					EnableVersioning:  true,
					VersionHashLength: length,
				}
				avm := NewAssetVersionManager(config)

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					avm.GenerateVersionedPath("/static/test.js", content)
				}
			})
		}
	})
}

func BenchmarkConcurrentVersioning(b *testing.B) {
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
		StaticPrefixes:    []string{"/static/"},
	}

	avm := NewAssetVersionManager(config)

	// Pre-register assets for concurrent access
	for i := 0; i < 100; i++ {
		path := fmt.Sprintf("/static/concurrent%d.js", i)
		content := []byte(fmt.Sprintf("content %d", i))
		avm.RegisterAsset(path, content)
	}

	b.Run("ConcurrentRead", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				path := fmt.Sprintf("/static/concurrent%d.js", i%100)
				avm.GetVersionedPath(path)
				i++
			}
		})
	})

	b.Run("ConcurrentWrite", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				path := fmt.Sprintf("/static/write%d.js", i)
				content := []byte(fmt.Sprintf("content %d", i))
				avm.RegisterAsset(path, content)
				i++
			}
		})
	})

	b.Run("MixedReadWrite", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%2 == 0 {
					// Read operation
					path := fmt.Sprintf("/static/concurrent%d.js", i%100)
					avm.GetVersionedPath(path)
				} else {
					// Write operation
					path := fmt.Sprintf("/static/mixed%d.js", i)
					content := []byte(fmt.Sprintf("content %d", i))
					avm.RegisterAsset(path, content)
				}
				i++
			}
		})
	})
}
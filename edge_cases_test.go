package gostc

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEdgeCases(t *testing.T) {
	t.Run("SpecialCharactersInFilenames", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-special-chars-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		staticDir := filepath.Join(tempDir, "static")
		os.MkdirAll(staticDir, 0755)

		// Test files with special characters
		specialFiles := []string{
			"file-with-dashes.js",
			"file_with_underscores.css",
			"file.with.dots.js",
			"file123numbers.css",
		}

		for _, filename := range specialFiles {
			content := "/* " + filename + " */"
			os.WriteFile(filepath.Join(staticDir, filename), []byte(content), 0644)
		}

		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
			WithStaticPrefixes("/static/"),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		ts := httptest.NewServer(server)
		defer ts.Close()

		for _, filename := range specialFiles {
			originalPath := "/static/" + filename
			versionedPath, exists := server.versionManager.GetVersionedPath(originalPath)
			if !exists {
				t.Errorf("Should have versioned path for %s", filename)
				continue
			}

			// Test versioned path serving
			resp, err := http.Get(ts.URL + versionedPath)
			if err != nil {
				t.Errorf("Request failed for %s: %v", filename, err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 for %s, got %d", filename, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), filename) {
				t.Errorf("Content mismatch for %s", filename)
			}
		}
	})

	t.Run("EmptyFiles", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-empty-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		staticDir := filepath.Join(tempDir, "static")
		os.MkdirAll(staticDir, 0755)

		// Create empty files (only JS and CSS should be versioned)
		emptyFiles := []string{"empty.js", "empty.css"}
		for _, filename := range emptyFiles {
			os.WriteFile(filepath.Join(staticDir, filename), []byte{}, 0644)
		}

		// Create empty JSON file (should NOT be versioned)
		os.WriteFile(filepath.Join(staticDir, "empty.json"), []byte{}, 0644)

		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
			WithStaticPrefixes("/static/"),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		ts := httptest.NewServer(server)
		defer ts.Close()

		// Test versioned empty files
		for _, filename := range emptyFiles {
			originalPath := "/static/" + filename
			versionedPath, exists := server.versionManager.GetVersionedPath(originalPath)
			if !exists {
				t.Errorf("Should have versioned path for empty file %s", filename)
				continue
			}

			// Empty files should still be served correctly
			resp, err := http.Get(ts.URL + versionedPath)
			if err != nil {
				t.Errorf("Request failed for empty file %s: %v", filename, err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 for empty file %s, got %d", filename, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			if len(body) != 0 {
				t.Errorf("Empty file %s should have empty content", filename)
			}
		}

		// Test that JSON files are not versioned
		_, exists := server.versionManager.GetVersionedPath("/static/empty.json")
		if exists {
			t.Error("JSON files should not be versioned")
		}

		// But JSON files should still be served normally
		resp, err := http.Get(ts.URL + "/static/empty.json")
		if err != nil {
			t.Fatalf("Request failed for JSON file: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Error("JSON files should be served normally even if not versioned")
		}
	})

	t.Run("VeryLargeFiles", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping large file test in short mode")
		}

		tempDir, err := os.MkdirTemp("", "gostc-large-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		staticDir := filepath.Join(tempDir, "static")
		os.MkdirAll(staticDir, 0755)

		// Create a 1MB file
		largeContent := strings.Repeat("console.log('test'); ", 50000)
		largeFile := filepath.Join(staticDir, "large.js")
		os.WriteFile(largeFile, []byte(largeContent), 0644)

		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
			WithStaticPrefixes("/static/"),
			WithCompression(Gzip),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		ts := httptest.NewServer(server)
		defer ts.Close()

		versionedPath, exists := server.versionManager.GetVersionedPath("/static/large.js")
		if !exists {
			t.Fatal("Should have versioned path for large file")
		}

		// Test with compression
		req, _ := http.NewRequest("GET", ts.URL+versionedPath, nil)
		req.Header.Set("Accept-Encoding", "gzip")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		if resp.Header.Get("Content-Encoding") != "gzip" {
			t.Error("Large file should be compressed")
		}

		cacheControl := resp.Header.Get("Cache-Control")
		if !strings.Contains(cacheControl, "immutable") {
			t.Error("Large versioned file should have immutable cache control")
		}
	})

	t.Run("NestedDirectories", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-nested-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create nested directory structure
		nestedPaths := []string{
			"static/js/app.js",
			"static/css/style.css",
			"static/images/logo.png",
			"static/vendor/jquery/jquery.min.js",
		}

		for _, path := range nestedPaths {
			fullPath := filepath.Join(tempDir, path)
			os.MkdirAll(filepath.Dir(fullPath), 0755)
			content := "/* " + path + " */"
			os.WriteFile(fullPath, []byte(content), 0644)
		}

		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
			WithStaticPrefixes("/static/"),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		ts := httptest.NewServer(server)
		defer ts.Close()

		for _, path := range nestedPaths {
			originalPath := "/" + path
			versionedPath, exists := server.versionManager.GetVersionedPath(originalPath)
			if !exists {
				t.Errorf("Should have versioned path for nested file %s", path)
				continue
			}

			resp, err := http.Get(ts.URL + versionedPath)
			if err != nil {
				t.Errorf("Request failed for nested file %s: %v", path, err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 for nested file %s, got %d", path, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), path) {
				t.Errorf("Content mismatch for nested file %s", path)
			}
		}
	})

	t.Run("UnsupportedFileTypes", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-unsupported-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		staticDir := filepath.Join(tempDir, "static")
		os.MkdirAll(staticDir, 0755)

		// Files that should NOT be versioned
		unsupportedFiles := []string{
			"readme.txt",
			"config.xml",
			"data.json",
			"document.pdf",
			"archive.zip",
		}

		for _, filename := range unsupportedFiles {
			content := "content of " + filename
			os.WriteFile(filepath.Join(staticDir, filename), []byte(content), 0644)
		}

		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
			WithStaticPrefixes("/static/"),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		for _, filename := range unsupportedFiles {
			originalPath := "/static/" + filename
			_, exists := server.versionManager.GetVersionedPath(originalPath)
			if exists {
				t.Errorf("Should NOT have versioned path for unsupported file type %s", filename)
			}
		}
	})

	t.Run("HTMLWithComplexAssetReferences", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-complex-html-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		staticDir := filepath.Join(tempDir, "static")
		os.MkdirAll(staticDir, 0755)

		// Create assets
		assets := map[string]string{
			"static/main.js":   "console.log('main');",
			"static/style.css": "body { margin: 0; }",
			"static/logo.svg":  "<svg></svg>",
		}

		for path, content := range assets {
			fullPath := filepath.Join(tempDir, path)
			os.WriteFile(fullPath, []byte(content), 0644)
		}

		// Complex HTML with various asset reference formats
		complexHTML := `<!DOCTYPE html>
<html>
<head>
    <title>Test</title>
    <link rel="stylesheet" href="/static/style.css" type="text/css">
    <link rel="preload" href="/static/main.js" as="script">
    <script defer src="/static/main.js"></script>
    <!-- Should not be changed -->
    <link href="https://external.com/style.css" rel="stylesheet">
    <script src="https://cdn.example.com/lib.js"></script>
</head>
<body>
    <img src="/static/logo.svg" alt="Logo" width="100">
    <div style="background: url('/static/logo.svg')">Background</div>
    <!-- External image should not be changed -->
    <img src="https://example.com/external.png" alt="External">
</body>
</html>`

		os.WriteFile(filepath.Join(tempDir, "complex.html"), []byte(complexHTML), 0644)

		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
			WithStaticPrefixes("/static/"),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		ts := httptest.NewServer(server)
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/complex.html")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		html := string(body)

		// Should not contain original asset references
		if strings.Contains(html, `href="/static/style.css"`) {
			t.Error("Should replace CSS reference in link tag")
		}
		if strings.Contains(html, `src="/static/main.js"`) {
			t.Error("Should replace JS reference in script tags")
		}
		if strings.Contains(html, `src="/static/logo.svg"`) {
			t.Error("Should replace image reference in img tag")
		}

		// Should still contain external references (unchanged)
		if !strings.Contains(html, "https://external.com/style.css") {
			t.Error("Should not modify external CSS references")
		}
		if !strings.Contains(html, "https://cdn.example.com/lib.js") {
			t.Error("Should not modify external JS references")
		}
		if !strings.Contains(html, "https://example.com/external.png") {
			t.Error("Should not modify external image references")
		}

		// Should contain versioned local references
		cssVersioned, _ := server.versionManager.GetVersionedPath("/static/style.css")
		jsVersioned, _ := server.versionManager.GetVersionedPath("/static/main.js")
		svgVersioned, _ := server.versionManager.GetVersionedPath("/static/logo.svg")

		if !strings.Contains(html, cssVersioned) {
			t.Error("Should contain versioned CSS path")
		}
		if !strings.Contains(html, jsVersioned) {
			t.Error("Should contain versioned JS path")
		}
		if !strings.Contains(html, svgVersioned) {
			t.Error("Should contain versioned SVG path")
		}
	})

	t.Run("PathTraversalAttempts", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-security-*")
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

		ts := httptest.NewServer(server)
		defer ts.Close()

		// Attempt path traversal attacks
		maliciousPaths := []string{
			"/../../../etc/passwd",
			"/static/../../../etc/passwd",
			"/static/../../config",
			"//etc/passwd",
			"/static//../../etc",
		}

		for _, path := range maliciousPaths {
			resp, err := http.Get(ts.URL + path)
			if err != nil {
				continue // Network errors are acceptable
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
				t.Errorf("Path traversal attempt %s should be rejected, got status %d", path, resp.StatusCode)
			}
		}
	})
}

func TestErrorRecovery(t *testing.T) {
	t.Run("CorruptedFileRecovery", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-recovery-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		staticDir := filepath.Join(tempDir, "static")
		os.MkdirAll(staticDir, 0755)

		// Create and then immediately delete a file to simulate corruption
		testFile := filepath.Join(staticDir, "test.js")
		os.WriteFile(testFile, []byte("console.log('test');"), 0644)

		server, err := New(
			WithRoot(tempDir),
			WithVersioning(true),
			WithStaticPrefixes("/static/"),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		// File should be versioned
		_, exists := server.versionManager.GetVersionedPath("/static/test.js")
		if !exists {
			t.Error("File should be versioned initially")
		}

		// Remove the file after versioning (simulates corruption/deletion)
		os.Remove(testFile)

		ts := httptest.NewServer(server)
		defer ts.Close()

		// Request should fail gracefully
		resp, err := http.Get(ts.URL + "/static/test.js")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 for deleted file, got %d", resp.StatusCode)
		}
	})

	t.Run("InvalidVersionedPathHandling", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gostc-invalid-*")
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

		ts := httptest.NewServer(server)
		defer ts.Close()

		// Test malformed versioned paths
		invalidVersionedPaths := []string{
			"/static/file.invalidhash.js",
			"/static/file.12345.js",    // Too short hash
			"/static/file.gggggggg.js", // Invalid hex characters
			"/static/file..js",         // Empty hash
		}

		for _, path := range invalidVersionedPaths {
			resp, err := http.Get(ts.URL + path)
			if err != nil {
				continue // Network errors are acceptable
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("Invalid versioned path %s should return 404, got %d", path, resp.StatusCode)
			}
		}
	})
}

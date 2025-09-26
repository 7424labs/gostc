package gostc

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetVersionManager(t *testing.T) {
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
		StaticPrefixes:    []string{"/static/"},
	}

	avm := NewAssetVersionManager(config)

	t.Run("GenerateVersionedPath", func(t *testing.T) {
		content := []byte("test content")
		versionedPath, hash := avm.GenerateVersionedPath("/static/app.js", content)

		if !strings.Contains(versionedPath, hash) {
			t.Errorf("Versioned path %s should contain hash %s", versionedPath, hash)
		}

		if !strings.HasPrefix(versionedPath, "/static/app.") {
			t.Errorf("Versioned path %s should have correct prefix", versionedPath)
		}

		if !strings.HasSuffix(versionedPath, ".js") {
			t.Errorf("Versioned path %s should have correct extension", versionedPath)
		}
	})

	t.Run("RegisterAndRetrieveAsset", func(t *testing.T) {
		originalPath := "/static/style.css"
		content := []byte("body { color: red; }")

		avm.RegisterAsset(originalPath, content)

		// Test GetVersionedPath
		versionedPath, exists := avm.GetVersionedPath(originalPath)
		if !exists {
			t.Error("Should find versioned path for registered asset")
		}

		// Test GetOriginalPath
		retrievedOriginal, exists := avm.GetOriginalPath(versionedPath)
		if !exists {
			t.Error("Should find original path for versioned asset")
		}

		if retrievedOriginal != originalPath {
			t.Errorf("Expected original path %s, got %s", originalPath, retrievedOriginal)
		}

		// Test GetContentHash
		hash, exists := avm.GetContentHash(originalPath)
		if !exists {
			t.Error("Should find content hash for registered asset")
		}

		if hash == "" {
			t.Error("Content hash should not be empty")
		}
	})

	t.Run("IsVersionedPath", func(t *testing.T) {
		originalPath := "/static/test.js"
		content := []byte("console.log('test');")

		avm.RegisterAsset(originalPath, content)
		versionedPath, _ := avm.GetVersionedPath(originalPath)

		if !avm.IsVersionedPath(versionedPath) {
			t.Error("Should recognize versioned path")
		}

		if avm.IsVersionedPath(originalPath) {
			t.Error("Should not recognize original path as versioned")
		}
	})

	t.Run("RemoveAsset", func(t *testing.T) {
		originalPath := "/static/remove-test.css"
		content := []byte("/* test */")

		avm.RegisterAsset(originalPath, content)

		// Verify it exists
		_, exists := avm.GetVersionedPath(originalPath)
		if !exists {
			t.Error("Asset should exist before removal")
		}

		// Remove it
		avm.RemoveAsset(originalPath)

		// Verify it's gone
		_, exists = avm.GetVersionedPath(originalPath)
		if exists {
			t.Error("Asset should not exist after removal")
		}
	})

	t.Run("shouldVersionFile", func(t *testing.T) {
		testCases := []struct {
			path     string
			expected bool
		}{
			{"/static/app.js", true},
			{"/static/style.css", true},
			{"/static/image.png", true},
			{"/static/font.woff2", true},
			{"/api/data.json", false},
			{"/index.html", false},
			{"/static/readme.txt", false},
		}

		for _, tc := range testCases {
			result := avm.shouldVersionFile(tc.path)
			if result != tc.expected {
				t.Errorf("shouldVersionFile(%s) = %v, expected %v", tc.path, result, tc.expected)
			}
		}
	})
}

func TestHTMLProcessor(t *testing.T) {
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
		StaticPrefixes:    []string{"/static/"},
	}

	avm := NewAssetVersionManager(config)
	processor := NewHTMLProcessor(avm)

	// Register some assets
	avm.RegisterAsset("/static/app.js", []byte("console.log('app');"))
	avm.RegisterAsset("/static/style.css", []byte("body { color: blue; }"))
	avm.RegisterAsset("/static/logo.png", []byte("fake image data"))

	t.Run("ProcessHTML", func(t *testing.T) {
		html := `<!DOCTYPE html>
<html>
<head>
    <link href="/static/style.css" rel="stylesheet">
    <script src="/static/app.js"></script>
</head>
<body>
    <img src="/static/logo.png" alt="Logo">
    <link href="/external/style.css" rel="stylesheet">
</body>
</html>`

		processed := processor.ProcessHTML([]byte(html), "/index.html")
		processedStr := string(processed)

		// Should replace registered assets
		if strings.Contains(processedStr, `href="/static/style.css"`) {
			t.Error("Should have replaced CSS reference with versioned path")
		}

		if strings.Contains(processedStr, `src="/static/app.js"`) {
			t.Error("Should have replaced JS reference with versioned path")
		}

		if strings.Contains(processedStr, `src="/static/logo.png"`) {
			t.Error("Should have replaced image reference with versioned path")
		}

		// Should not replace external assets
		if !strings.Contains(processedStr, `href="/external/style.css"`) {
			t.Error("Should not have modified external CSS reference")
		}

		// Verify versioned paths are present
		styleVersioned, _ := avm.GetVersionedPath("/static/style.css")
		if !strings.Contains(processedStr, styleVersioned) {
			t.Error("Should contain versioned CSS path")
		}
	})

	t.Run("ProcessHTMLWithDisabledVersioning", func(t *testing.T) {
		disabledConfig := &Config{EnableVersioning: false}
		disabledProcessor := NewHTMLProcessor(NewAssetVersionManager(disabledConfig))

		html := `<link href="/static/style.css" rel="stylesheet">`
		processed := disabledProcessor.ProcessHTML([]byte(html), "/index.html")

		if !bytes.Equal([]byte(html), processed) {
			t.Error("Should not modify HTML when versioning is disabled")
		}
	})
}

func TestAssetVersionManagerScanDirectory(t *testing.T) {
	// Create temporary directory structure
	tempDir, err := os.MkdirTemp("", "gostc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	staticDir := filepath.Join(tempDir, "static")
	os.MkdirAll(staticDir, 0755)

	testFiles := map[string]string{
		"static/app.js":     "console.log('app');",
		"static/style.css":  "body { color: red; }",
		"static/image.png":  "fake png data",
		"static/readme.txt": "readme content", // Should not be versioned
		"index.html":        "<html></html>",  // Not in static prefix
	}

	for relativePath, content := range testFiles {
		fullPath := filepath.Join(tempDir, relativePath)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write test file %s: %v", relativePath, err)
		}
	}

	// Test scanning
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
		StaticPrefixes:    []string{"/static/"},
	}

	avm := NewAssetVersionManager(config)
	err = avm.ScanDirectory(tempDir)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	// Verify expected files were versioned
	expectedVersioned := []string{"/static/app.js", "/static/style.css", "/static/image.png"}
	for _, path := range expectedVersioned {
		if _, exists := avm.GetVersionedPath(path); !exists {
			t.Errorf("Expected %s to be versioned", path)
		}
	}

	// Verify files that shouldn't be versioned
	unexpectedVersioned := []string{"/static/readme.txt", "/index.html"}
	for _, path := range unexpectedVersioned {
		if _, exists := avm.GetVersionedPath(path); exists {
			t.Errorf("Did not expect %s to be versioned", path)
		}
	}
}

func TestConsistentHashing(t *testing.T) {
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
	}

	avm := NewAssetVersionManager(config)

	content := []byte("consistent test content")
	path := "/static/test.js"

	// Generate hash multiple times
	_, hash1 := avm.GenerateVersionedPath(path, content)
	_, hash2 := avm.GenerateVersionedPath(path, content)

	if hash1 != hash2 {
		t.Error("Hash should be consistent for the same content")
	}

	// Different content should produce different hash
	differentContent := []byte("different content")
	_, hash3 := avm.GenerateVersionedPath(path, differentContent)

	if hash1 == hash3 {
		t.Error("Different content should produce different hash")
	}
}

func TestVersioningWithCustomPattern(t *testing.T) {
	config := &Config{
		EnableVersioning:  true,
		VersioningPattern: "{base}_v{hash}{ext}",
		VersionHashLength: 8, // This means 8 hex characters
		StaticPrefixes:    []string{"/assets/"},
	}

	avm := NewAssetVersionManager(config)

	content := []byte("test content for custom pattern")
	originalPath := "/assets/main.js"

	versionedPath, hash := avm.GenerateVersionedPath(originalPath, content)

	expectedPattern := "/assets/main_v" + hash + ".js"
	if versionedPath != expectedPattern {
		t.Errorf("Expected versioned path %s, got %s", expectedPattern, versionedPath)
	}

	if len(hash) != 8 { // 8 hex characters for 8 hash length config
		t.Errorf("Expected hash length 8 (VersionHashLength config), got %d", len(hash))
	}
}

func BenchmarkAssetVersioning(b *testing.B) {
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
		StaticPrefixes:    []string{"/static/"},
	}

	avm := NewAssetVersionManager(config)
	content := []byte("benchmark test content that is reasonably long to simulate real file content")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := "/static/test.js"
		avm.RegisterAsset(path, content)
		avm.GetVersionedPath(path)
	}
}

func BenchmarkHTMLProcessing(b *testing.B) {
	config := &Config{
		EnableVersioning:  true,
		VersionHashLength: 16,
		StaticPrefixes:    []string{"/static/"},
	}

	avm := NewAssetVersionManager(config)
	processor := NewHTMLProcessor(avm)

	// Register assets
	avm.RegisterAsset("/static/app.js", []byte("console.log('test');"))
	avm.RegisterAsset("/static/style.css", []byte("body { margin: 0; }"))

	html := []byte(`<!DOCTYPE html>
<html>
<head>
    <link href="/static/style.css" rel="stylesheet">
    <script src="/static/app.js"></script>
</head>
<body>
    <h1>Test Page</h1>
</body>
</html>`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.ProcessHTML(html, "/index.html")
	}
}

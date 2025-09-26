package gostc

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTMLVersioningWithCompression(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "gostc-html-compression-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create static directory
	staticDir := filepath.Join(tempDir, "static")
	if err := os.Mkdir(staticDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test CSS file
	cssContent := "body { background: blue; }"
	cssPath := filepath.Join(staticDir, "style.css")
	if err := os.WriteFile(cssPath, []byte(cssContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create test JS file
	jsContent := "console.log('Hello');"
	jsPath := filepath.Join(staticDir, "app.js")
	if err := os.WriteFile(jsPath, []byte(jsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create HTML file with references to CSS and JS
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <link href="/static/style.css" rel="stylesheet">
</head>
<body>
    <h1>Test Page</h1>
    <script src="/static/app.js"></script>
</body>
</html>`
	htmlPath := filepath.Join(tempDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create server with versioning and compression enabled
	server, err := New(
		WithRoot(tempDir),
		WithVersioning(true),
		WithCompression(Gzip),
		func(c *Config) { c.MinSizeToCompress = 1 }, // Compress even small files for testing
	)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	ts := httptest.NewServer(server.handler)
	defer ts.Close()

	t.Run("HTMLWithVersionedAssetsAndCompression", func(t *testing.T) {
		// Request the HTML file with gzip accepted
		req, err := http.NewRequest("GET", ts.URL+"/index.html", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept-Encoding", "gzip")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Check that response is compressed
		if resp.Header.Get("Content-Encoding") != "gzip" {
			t.Errorf("Expected gzip encoding, got %s", resp.Header.Get("Content-Encoding"))
		}

		// Decompress the response
		var body string
		if resp.Header.Get("Content-Encoding") == "gzip" {
			gr, err := gzip.NewReader(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			defer gr.Close()
			bodyBytes, err := io.ReadAll(gr)
			if err != nil {
				t.Fatal(err)
			}
			body = string(bodyBytes)
		} else {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			body = string(bodyBytes)
		}

		// Check that CSS reference is versioned
		if !strings.Contains(body, "/static/style.") || !strings.Contains(body, ".css") {
			t.Errorf("CSS should be versioned, but HTML contains: %s", body)
		}

		// Check that JS reference is versioned
		if !strings.Contains(body, "/static/app.") || !strings.Contains(body, ".js") {
			t.Errorf("JS should be versioned, but HTML contains: %s", body)
		}

		// Ensure original paths are not present
		if strings.Contains(body, `href="/static/style.css"`) {
			t.Errorf("Original CSS path should not be present in HTML: %s", body)
		}
		if strings.Contains(body, `src="/static/app.js"`) {
			t.Errorf("Original JS path should not be present in HTML: %s", body)
		}
	})

	t.Run("HTMLWithVersionedAssetsNoCompression", func(t *testing.T) {
		// Request the HTML file without compression
		req, err := http.NewRequest("GET", ts.URL+"/index.html", nil)
		if err != nil {
			t.Fatal(err)
		}
		// Don't set Accept-Encoding header

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Check that response is NOT compressed
		if resp.Header.Get("Content-Encoding") != "" {
			t.Errorf("Expected no compression, got %s", resp.Header.Get("Content-Encoding"))
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		body := string(bodyBytes)

		// Check that CSS reference is versioned
		if !strings.Contains(body, "/static/style.") || !strings.Contains(body, ".css") {
			t.Errorf("CSS should be versioned, but HTML contains: %s", body)
		}

		// Check that JS reference is versioned
		if !strings.Contains(body, "/static/app.") || !strings.Contains(body, ".js") {
			t.Errorf("JS should be versioned, but HTML contains: %s", body)
		}

		// Ensure original paths are not present
		if strings.Contains(body, `href="/static/style.css"`) {
			t.Errorf("Original CSS path should not be present in HTML: %s", body)
		}
		if strings.Contains(body, `src="/static/app.js"`) {
			t.Errorf("Original JS path should not be present in HTML: %s", body)
		}
	})
}

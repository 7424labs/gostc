package gostc

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServerHeader(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)

	server, err := New(WithRoot(tmpDir))
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/test.txt", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	serverHeader := w.Header().Get("Server")
	if serverHeader != "7424" {
		t.Errorf("Expected Server header '7424', got '%s'", serverHeader)
	}
}

func TestServerHeaderOnHealthEndpoint(t *testing.T) {
	server, err := New()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	serverHeader := w.Header().Get("Server")
	if serverHeader != "7424" {
		t.Errorf("Expected Server header '7424' on health endpoint, got '%s'", serverHeader)
	}
}

func TestServerHeaderOnError(t *testing.T) {
	tmpDir := t.TempDir()

	server, err := New(WithRoot(tmpDir))
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/nonexistent.txt", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	serverHeader := w.Header().Get("Server")
	if serverHeader != "7424" {
		t.Errorf("Expected Server header '7424' on error response, got '%s'", serverHeader)
	}
}

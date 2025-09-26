# gostc - High-Performance Static File Server

gostc (Go Static Content) is a high-performance static file server written in Go with built-in compression, caching, and automatic cache invalidation.

## Features

- **Compression Support**
  - Gzip compression with configurable levels
  - Brotli compression for better compression ratios
  - Automatic content negotiation based on Accept-Encoding headers

- **In-Memory Caching**
  - LRU and LFU cache strategies
  - Configurable cache size and TTL
  - Thread-safe cache operations
  - Automatic cache invalidation on file changes

- **Performance**
  - HTTP/2 support
  - Concurrent request handling
  - Memory pooling for efficient resource usage
  - ETag support for client-side caching

- **Security & Reliability**
  - Rate limiting per IP address
  - CORS configuration
  - Security headers (CSP, HSTS, etc.)
  - Graceful shutdown
  - Panic recovery

- **Monitoring**
  - Prometheus metrics integration
  - Health check endpoint
  - Request logging

## Installation

```bash
go get github.com/7424labs/gostc
```

## Quick Start

### Basic Usage

```go
package main

import (
    "log"
    "github.com/7424labs/gostc"
)

func main() {
    server, err := gostc.New(
        gostc.WithRoot("./static"),
        gostc.WithCompression(gostc.Gzip | gostc.Brotli),
        gostc.WithCache(100*1024*1024), // 100MB cache
        gostc.WithWatcher(true),         // Auto cache invalidation
    )
    if err != nil {
        log.Fatal(err)
    }

    if err := server.Start(); err != nil {
        log.Fatal(err)
    }

    select {} // Keep running
}
```

### Using Presets

```go
// Development preset
server, _ := gostc.New(func(c *gostc.Config) {
    *c = *gostc.NewWithPreset(gostc.PresetDevelopment)
})

// Production preset
server, _ := gostc.New(func(c *gostc.Config) {
    *c = *gostc.NewWithPreset(gostc.PresetProduction)
})

// High-performance preset
server, _ := gostc.New(func(c *gostc.Config) {
    *c = *gostc.NewWithPreset(gostc.PresetHighPerformance)
})
```

### Advanced Configuration

```go
server, err := gostc.New(
    gostc.WithRoot("./public"),
    gostc.WithCompression(gostc.Gzip | gostc.Brotli),
    gostc.WithCompressionLevel(6),
    gostc.WithCache(500*1024*1024),  // 500MB
    gostc.WithCacheTTL(10*time.Minute),
    gostc.WithCacheStrategy(gostc.LFU),
    gostc.WithTimeouts(gostc.TimeoutConfig{
        Read:     10 * time.Second,
        Write:    10 * time.Second,
        Idle:     120 * time.Second,
        Shutdown: 30 * time.Second,
    }),
    gostc.WithRateLimit(100), // 100 req/s per IP
    gostc.WithHTTP2(true),
    gostc.WithMetrics(true),
    gostc.WithTLS("cert.pem", "key.pem"),
)
```

## Running the Example

```bash
cd example
go run main.go -root ./static -addr :8080 -metrics
```

Visit http://localhost:8080 to see the example in action.

### Command Line Options

```
-root string      Static files directory (default "./static")
-addr string      Server address (default ":8080")
-cache int        Cache size in bytes (default 104857600)
-ttl duration     Cache TTL (default 5m0s)
-rate int         Rate limit per IP (default 100)
-compress string  Compression: none, gzip, brotli, all (default "all")
-production       Use production preset
-metrics          Enable metrics endpoint
-tls              Enable TLS
-cert string      TLS certificate file
-key string       TLS key file
```

## Benchmarks

Run benchmarks with:

```bash
go test -bench=. -benchmem
```

## Configuration Options

### Cache Strategies

- **LRU** (Least Recently Used): Evicts least recently accessed items
- **LFU** (Least Frequently Used): Evicts least frequently accessed items

### Compression Levels

- **Gzip**: 1-9 (default: 6)
- **Brotli**: 0-11 (default: 6)

### Security Headers

The server automatically sets the following security headers:
- X-Content-Type-Options: nosniff
- X-Frame-Options: DENY
- X-XSS-Protection: 1; mode=block
- Referrer-Policy: strict-origin-when-cross-origin
- Strict-Transport-Security (when using HTTPS)

## Asset Versioning

gostc supports automatic asset versioning for cache busting. When enabled, it:
1. Scans static files and generates content-based hashes
2. Transforms HTML files to use versioned URLs
3. Serves both versioned and non-versioned URLs

### Basic Versioning Setup

```go
server, err := gostc.New(
    gostc.WithRoot("./static"),
    gostc.WithVersioning(true),                    // Enable versioning
    gostc.WithVersionHashLength(16),               // Hash length in hex chars
    gostc.WithStaticPrefixes("/css/", "/js/"),     // Paths to version
)
```

### Versioning with URL Prefix

When serving files from a URL prefix (e.g., `/static/`) that differs from the filesystem structure:

```go
server, err := gostc.New(
    gostc.WithRoot("./static"),                    // Filesystem root
    gostc.WithVersioning(true),
    gostc.WithURLPrefix("/static"),                // URL prefix for serving
    gostc.WithStaticPrefixes("/css/", "/js/", "/images/", "/favicon/"),
)
```

This handles cases where:
- Files are stored at `./static/css/style.css`
- URLs in HTML are `/static/css/style.css`
- Versioned URL becomes `/static/css/style.abc123def456.css`

## Embedding as HTTP Handler

gostc can be embedded in existing HTTP applications as a handler.

### Basic Handler Usage

```go
package main

import (
    "net/http"
    "github.com/7424labs/gostc"
)

func main() {
    // Create gostc server
    staticServer, err := gostc.New(
        gostc.WithRoot("./static"),
        gostc.WithVersioning(true),
        gostc.WithCache(100 << 20), // 100MB
    )
    if err != nil {
        panic(err)
    }

    // Use as http.Handler
    mux := http.NewServeMux()

    // Serve static files from /static/
    mux.Handle("/static/", http.StripPrefix("/static", staticServer))

    // Your other routes
    mux.HandleFunc("/api/", apiHandler)

    http.ListenAndServe(":8080", mux)
}
```

### Advanced Integration with URL Remapping

```go
package main

import (
    "net/http"
    "strings"
    "github.com/7424labs/gostc"
)

func main() {
    // Initialize gostc with URL prefix for versioning
    staticServer, err := gostc.New(
        gostc.WithRoot("./static"),
        gostc.WithVersioning(true),
        gostc.WithURLPrefix("/static"),  // Tell gostc about URL prefix
        gostc.WithStaticPrefixes("/css/", "/js/", "/images/"),
    )
    if err != nil {
        panic(err)
    }

    mux := http.NewServeMux()

    // Serve index.html with versioned assets
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/" {
            http.NotFound(w, r)
            return
        }
        // Serve index.html - gostc will transform it with versioned URLs
        r.URL.Path = "/index.html"
        staticServer.ServeHTTP(w, r)
    })

    // Serve static files with URL prefix stripping
    mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
        // Strip /static prefix before passing to gostc
        originalPath := r.URL.Path
        r.URL.Path = strings.TrimPrefix(r.URL.Path, "/static")
        staticServer.ServeHTTP(w, r)
        r.URL.Path = originalPath  // Restore for logging
    })

    // API routes
    mux.HandleFunc("/api/", apiHandler)

    http.ListenAndServe(":8080", mux)
}
```

### Using ServeFileHTTP for Direct File Serving

```go
// ServeFileHTTP bypasses internal mux, useful for embedding
staticServer.ServeFileHTTP(w, r)
```

## API Reference

### Server Methods

```go
// Create a new server
server, err := gostc.New(options...)

// Start the server (standalone mode)
err := server.Start()

// Stop the server gracefully
err := server.Stop()

// Use as http.Handler (embedded mode)
server.ServeHTTP(w, r)

// Direct file serving (bypasses internal mux)
server.ServeFileHTTP(w, r)

// Manually invalidate cache for a path
server.InvalidatePath("/path/to/file")

// Clear entire cache
server.InvalidateAll()

// Get cache statistics
stats := server.CacheStats()
```

### Configuration Options

```go
// File serving
gostc.WithRoot(dir)                    // Root directory for static files
gostc.WithIndexFile(name)              // Index file name (default: "index.html")

// Compression
gostc.WithCompression(types)           // Gzip | Brotli
gostc.WithCompressionLevel(level)      // 1-9 for gzip, 0-11 for brotli

// Caching
gostc.WithCache(sizeBytes)             // Cache size in bytes
gostc.WithCacheTTL(duration)           // Time-to-live for cached items
gostc.WithCacheStrategy(strategy)      // LRU or LFU

// Versioning
gostc.WithVersioning(enable)           // Enable asset versioning
gostc.WithVersionHashLength(length)    // Hash length (default: 16)
gostc.WithStaticPrefixes(prefixes...)  // Paths to version
gostc.WithURLPrefix(prefix)            // URL serving prefix

// Performance
gostc.WithHTTP2(enable)                // Enable HTTP/2
gostc.WithRateLimit(reqPerSec)         // Rate limit per IP
gostc.WithTimeouts(config)             // Read/Write/Idle timeouts

// Security
gostc.WithTLS(certFile, keyFile)       // Enable HTTPS
gostc.WithCORS(origins, methods)       // Configure CORS

// Monitoring
gostc.WithMetrics(enable)              // Enable Prometheus metrics
gostc.WithWatcher(enable)              // Watch files for changes
```

## Testing

Run tests with:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test -cover ./...
```

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
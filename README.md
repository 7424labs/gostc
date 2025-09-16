# GoSTC - High-Performance Static File Server

GoSTC (Go Static Content) is a high-performance static file server written in Go with built-in compression, caching, and automatic cache invalidation.

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
go get github.com/yourusername/gostc
```

## Quick Start

### Basic Usage

```go
package main

import (
    "log"
    "github.com/yourusername/gostc"
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

## API Reference

### Server Methods

```go
// Create a new server
server, err := gostc.New(options...)

// Start the server
err := server.Start()

// Stop the server gracefully
err := server.Stop()

// Manually invalidate cache for a path
server.InvalidatePath("/path/to/file")

// Clear entire cache
server.InvalidateAll()

// Get cache statistics
stats := server.CacheStats()
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
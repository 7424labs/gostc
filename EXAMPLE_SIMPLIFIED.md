# GOSTC Simplified API Examples

This document shows the new simplified API compared to the old complex configuration.

## Before vs After Comparison

### Before (Complex - 8+ lines of configuration)

```go
package main

import (
    "log"
    "github.com/7424labs/gostc"
)

func main() {
    // Old complex way - requires understanding of multiple concepts
    server, err := gostc.New(
        gostc.WithRoot("./static"),
        gostc.WithURLPrefix("/static"),
        gostc.WithCompression(gostc.Gzip|gostc.Brotli),
        gostc.WithCache(50 << 20), // 50MB
        gostc.WithVersioning(true),
        gostc.WithVersionHashLength(8),
        gostc.WithStaticPrefixes("/css/", "/js/", "/images/"),
        gostc.WithWatcher(true),
    )
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }

    log.Println("Server starting on :8080")
    log.Fatal(server.ListenAndServe(":8080"))
}
```

### After (Simple - 1-2 lines)

#### Option 1: Auto-Configuration (Simplest)
```go
package main

import (
    "log"
    "github.com/7424labs/gostc"
)

func main() {
    // Auto-detects directory structure and configures everything
    server, err := gostc.NewAutoServer("./static")
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }

    log.Println("Server starting on :8080")
    log.Fatal(server.ListenAndServe(":8080"))
}
```

#### Option 2: Preset Configuration
```go
package main

import (
    "log"
    "github.com/7424labs/gostc"
)

func main() {
    // Use a preset for your app type
    server, err := gostc.NewWithPresetServer(gostc.PresetSPA)
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }

    log.Println("Server starting on :8080")
    log.Fatal(server.ListenAndServe(":8080"))
}
```

#### Option 3: Simple Configuration
```go
package main

import (
    "log"
    "github.com/7424labs/gostc"
)

func main() {
    // Simple struct-based configuration
    server, err := gostc.NewSimpleServer(gostc.SimpleConfig{
        Root:       "./static",
        Versioning: true,
        Cache:      true,
        Compress:   true,
    })
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }

    log.Println("Server starting on :8080")
    log.Fatal(server.ListenAndServe(":8080"))
}
```

## Available Presets

### PresetSPA - Single Page Application
Perfect for React, Vue, Angular apps:
- ✅ Versioning enabled with 8-char hashes
- ✅ Long cache (1 year) for versioned assets
- ✅ No cache for HTML files
- ✅ File watching enabled
- ✅ Compression enabled

### PresetStaticSite - Static Website
Great for Hugo, Jekyll, static sites:
- ✅ Versioning enabled for CSS, JS, images
- ✅ Long cache (1 year) for versioned assets
- ✅ Moderate cache (1 hour) for HTML
- ✅ Multiple asset directory support

### PresetAPI - API Server
For backend APIs with minimal static assets:
- ❌ Versioning disabled
- ✅ Metrics enabled
- ❌ No cache for dynamic content
- ⚡ Optimized for JSON/API responses

### PresetHybrid - Full-Stack App
For apps with both API and frontend:
- ✅ Versioning enabled
- ✅ File watching enabled
- ✅ Moderate cache (5 min) for dynamic content
- ✅ Long cache for static assets

## Auto-Configuration Logic

When you use `NewAutoServer("./static")`, it automatically:

1. **Scans directory structure** to detect:
   - `index.html` → Likely SPA/website
   - `css/`, `js/`, `images/` dirs → Asset directories
   - File types and sizes

2. **Configures based on what it finds**:
   - **SPA detected** (index.html + asset dirs) → Versioning + long cache
   - **Assets only** → Versioning + moderate cache
   - **Simple files** → Basic file server

3. **Sets sensible defaults**:
   - Hash length: 8 characters
   - Cache size: Based on directory size
   - Compression: Enabled for text files
   - File watching: Enabled in development

## Migration Guide

### Step 1: Identify Your Use Case
- **SPA/PWA**: Use `PresetSPA`
- **Static Site**: Use `PresetStaticSite`
- **API Backend**: Use `PresetAPI`
- **Full-Stack**: Use `PresetHybrid`
- **Not sure**: Use `NewAutoServer()`

### Step 2: Replace Your Configuration

**Old:**
```go
server, err := gostc.New(
    gostc.WithRoot("./dist"),
    gostc.WithVersioning(true),
    gostc.WithVersionHashLength(8),
    gostc.WithStaticPrefixes("/assets/"),
    // ... more options
)
```

**New:**
```go
server, err := gostc.NewWithPresetServer(gostc.PresetSPA,
    gostc.WithRoot("./dist"))
```

### Step 3: Test & Tune
The presets work out of the box, but you can still customize:

```go
server, err := gostc.NewWithPresetServer(gostc.PresetSPA,
    gostc.WithRoot("./dist"),
    gostc.WithVersionHashLength(12), // Custom hash length
    gostc.WithCache(100 << 20),      // Custom cache size
)
```

## Benefits

1. **90% fewer configuration lines** for common use cases
2. **Zero-config setup** with auto-detection
3. **Impossible to misconfigure** with presets
4. **Backwards compatible** - old API still works
5. **Self-documenting** - preset names explain intent
6. **Production-ready defaults** built into each preset

## Advanced Usage

You can still access all the power of the original API:

```go
// Start with a preset, then customize
config := gostc.NewWithPreset(gostc.PresetSPA)
config.Root = "./build"
config.VersionHashLength = 12
config.EnableMetrics = true

server, err := gostc.NewWithConfig(config)
```

This gives you the best of both worlds: simplicity when you need it, power when you need it.
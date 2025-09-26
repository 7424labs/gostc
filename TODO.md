# GOSTC Package Optimization Plan

## Overview
Fix versioning system issues and simplify the API to make package integration easier.

## Phase 1: Critical Fixes (Immediate)

### 1.1 Fix Hash Length Inconsistency
- [ ] Standardize default hash length to 8 characters across all components
- [ ] Update `config.go` DefaultConfig to use 8
- [ ] Update `versioning.go` NewAssetVersionManager default to 8
- [ ] Add validation for hash length (min: 4, max: 16)

### 1.2 Fix Path Resolution Issues
- [ ] Implement intelligent path resolver that handles URL prefix correctly
- [ ] Auto-detect relationship between filesystem and URL paths
- [ ] Fix static prefix matching when URL prefix is set
- [ ] Add path validation at initialization

### 1.3 Configuration Validation
- [ ] Validate URL prefix and static prefixes compatibility
- [ ] Check for path conflicts and overlaps
- [ ] Provide clear error messages for misconfigurations
- [ ] Suggest fixes for common setup mistakes

## Phase 2: API Simplification (Week 1)

### 2.1 Create Preset Configurations
```go
// Implement presets
const (
    PresetSPA        = "spa"        // Single-page application
    PresetStaticSite = "static"     // Static website
    PresetAPI        = "api"        // API with minimal assets
    PresetHybrid     = "hybrid"     // Full-stack app
)
```
- [ ] Implement `NewWithPreset(preset string)` function
- [ ] Define optimal configurations for each preset
- [ ] Add preset validation and fallback

### 2.2 Auto-Configuration
- [ ] Implement `NewAuto(root string)` function
- [ ] Auto-detect common asset directories (css/, js/, images/, assets/)
- [ ] Intelligent compression detection based on file types
- [ ] Smart cache size calculation based on directory size

### 2.3 Simplified Versioning API
- [ ] Create `WithVersionedAssets(enabled bool)` option
- [ ] Implement `VersionConfig` struct for advanced options
- [ ] Consolidate multiple versioning options into one
- [ ] Maintain backwards compatibility with deprecation warnings

### 2.4 Unified Configuration Object
```go
type SimpleConfig struct {
    Root       string
    URLPrefix  string
    Versioning bool
    Cache      bool
    Compress   bool
    Debug      bool
}
```
- [ ] Implement `NewWithConfig(cfg SimpleConfig)`
- [ ] Map simple config to internal detailed config
- [ ] Provide sensible defaults for all options

## Phase 3: Debug & Monitoring (Week 2)

### 3.1 Built-in Debug Mode
- [ ] Replace GOSTC_DEBUG env var with `WithDebug(bool)` option
- [ ] Implement structured debug logging
- [ ] Add request/response debug output
- [ ] Log versioning decisions and mappings

### 3.2 Status Endpoint
- [ ] Implement `/gostc/status` endpoint (configurable)
- [ ] Show versioning mappings
- [ ] Display cache statistics
- [ ] List configured options and presets

### 3.3 Better Error Messages
- [ ] Create error code system for common issues
- [ ] Provide actionable fix suggestions
- [ ] Include configuration context in errors
- [ ] Add troubleshooting guide references

## Phase 4: Enhanced Features (Week 3)

### 4.1 Version Manifest
- [ ] Generate JSON manifest of versioned assets
- [ ] Implement `GetVersionManifest()` method
- [ ] Support manifest export to file
- [ ] Enable build tool integration

### 4.2 Asset Discovery
- [ ] Implement directory scanner for asset detection
- [ ] Support `.gostcignore` file for exclusions
- [ ] Auto-configure based on discovered structure
- [ ] Provide discovery report/suggestions

### 4.3 Dynamic Reloading
- [ ] Improve watcher to handle versioning updates
- [ ] Support hot-reload for development mode
- [ ] Implement graceful version transitions
- [ ] Add WebSocket support for live updates

## Phase 5: Documentation & Testing

### 5.1 Documentation
- [ ] Write migration guide from current to new API
- [ ] Create preset usage examples
- [ ] Document common patterns and anti-patterns
- [ ] Add troubleshooting guide

### 5.2 Examples
- [ ] React SPA example
- [ ] Vue.js application example
- [ ] Static site (Hugo/Jekyll) example
- [ ] Go template-based app example
- [ ] API with minimal assets example

### 5.3 Testing
- [ ] Unit tests for all new functions
- [ ] Integration tests for presets
- [ ] Benchmark tests for performance validation
- [ ] E2E tests with sample applications
- [ ] Backwards compatibility tests

## Implementation Order

1. **Week 1: Critical Fixes**
   - Fix hash length inconsistency
   - Fix path resolution bugs
   - Add configuration validation

2. **Week 2: Core Simplification**
   - Implement presets
   - Create auto-configuration
   - Simplify versioning API

3. **Week 3: Enhanced UX**
   - Built-in debug mode
   - Status endpoint
   - Better error messages

4. **Week 4: Advanced Features**
   - Version manifest
   - Asset discovery
   - Dynamic reloading

## Backwards Compatibility

- Maintain existing API with deprecation warnings
- Support both APIs for 2 major versions
- Provide automated migration tool
- Clear upgrade documentation

## Success Metrics

- [ ] Reduce configuration lines from 6+ to 1-2 for common cases
- [ ] Zero configuration errors for preset users
- [ ] Sub-second initialization for <1000 files
- [ ] 100% backwards compatibility with deprecation path
- [ ] <5 minute migration time for existing users

## Example: Before vs After

### Before (Complex)
```go
staticHandler, err := gostc.New(
    gostc.WithRoot("./static"),
    gostc.WithURLPrefix("/static"),
    gostc.WithCompression(gostc.Gzip|gostc.Brotli),
    gostc.WithCache(10<<20),
    gostc.WithVersioning(true),
    gostc.WithVersionHashLength(8),
    gostc.WithStaticPrefixes("/css/", "/js/", "/images/"),
    gostc.WithWatcher(true),
)
```

### After (Simple)
```go
// Option 1: Auto-configure everything
staticHandler, err := gostc.NewAuto("./static")

// Option 2: Use a preset
staticHandler, err := gostc.NewWithPreset(gostc.PresetSPA)

// Option 3: Simple config object
staticHandler, err := gostc.NewSimple(gostc.SimpleConfig{
    Root:       "./static",
    Versioning: true,
})
```

## Notes

- Priority: Fix critical issues first, then simplify API
- Maintain performance benchmarks throughout
- Focus on developer experience and ease of use
- Keep security features intact (path traversal protection, etc.)
- Ensure smooth migration path for existing users
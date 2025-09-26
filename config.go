package gostc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CompressionType int

const (
	NoCompression CompressionType = 0
	Gzip          CompressionType = 1 << iota
	Brotli
)

type CacheStrategy int

const (
	LRU CacheStrategy = iota
	LFU
	ARC
)

const (
	DefaultReadTimeout      = 15 * time.Second
	DefaultWriteTimeout     = 15 * time.Second
	DefaultIdleTimeout      = 60 * time.Second
	DefaultHeaderTimeout    = 5 * time.Second
	DefaultShutdownTimeout  = 30 * time.Second
	DefaultMaxHeaderBytes   = 1 << 20   // 1MB
	DefaultMaxBodySize      = 10 << 20  // 10MB
	DefaultMaxFileSize      = 100 << 20 // 100MB
	DefaultCacheSize        = 100 << 20 // 100MB
	DefaultCacheTTL         = 5 * time.Minute
	DefaultMinCompressSize  = 1024 // 1KB
	DefaultCompressionLevel = 6
	DefaultMaxConnections   = 1000
	DefaultRateLimitPerIP   = 100 // requests per second
)

type Config struct {
	Root          string
	IndexFile     string
	AllowBrowsing bool

	Compression       CompressionType
	CompressionLevel  int
	MinSizeToCompress int64
	CompressTypes     []string

	CacheSize     int64
	CacheTTL      time.Duration
	CacheStrategy CacheStrategy

	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxHeaderBytes    int
	MaxBodySize       int64
	MaxFileSize       int64 // Maximum file size to serve

	MaxConnections     int
	MaxRequestsPerConn int
	RateLimitPerIP     int

	AllowedOrigins []string
	AllowedMethods []string
	CSPHeader      string
	EnableHTTPS    bool
	TLSCert        string
	TLSKey         string
	HTTP2          bool

	EnableMetrics   bool
	MetricsEndpoint string
	EnablePprof     bool
	Debug           bool // Enable debug mode with detailed errors

	EnableWatcher bool

	// Cache control settings per file type
	StaticAssetMaxAge  int // Max age for static assets (images, fonts) in seconds
	DynamicAssetMaxAge int // Max age for dynamic assets (HTML, JSON) in seconds

	// Asset versioning settings
	EnableVersioning  bool
	VersioningPattern string   // Pattern for versioned files (empty = default: base.hash.ext)
	VersionHashLength int      // Length of version hash (default: 16)
	StaticPrefixes    []string // Prefixes that should be versioned
	URLPrefix         string   // URL prefix for serving (e.g., "/static")
}

func DefaultConfig() *Config {
	return &Config{
		Root:          "./static",
		IndexFile:     "index.html",
		AllowBrowsing: false,

		Compression:       Gzip | Brotli,
		CompressionLevel:  DefaultCompressionLevel,
		MinSizeToCompress: DefaultMinCompressSize,
		CompressTypes: []string{
			"text/html",
			"text/css",
			"text/javascript",
			"application/javascript",
			"application/json",
			"application/xml",
			"text/xml",
			"text/plain",
			"image/svg+xml",
		},

		CacheSize:     DefaultCacheSize,
		CacheTTL:      DefaultCacheTTL,
		CacheStrategy: LRU,

		ReadTimeout:       DefaultReadTimeout,
		ReadHeaderTimeout: DefaultHeaderTimeout,
		WriteTimeout:      DefaultWriteTimeout,
		IdleTimeout:       DefaultIdleTimeout,
		ShutdownTimeout:   DefaultShutdownTimeout,
		MaxHeaderBytes:    DefaultMaxHeaderBytes,
		MaxBodySize:       DefaultMaxBodySize,
		MaxFileSize:       DefaultMaxFileSize,

		MaxConnections: DefaultMaxConnections,
		RateLimitPerIP: DefaultRateLimitPerIP,

		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "HEAD", "OPTIONS"},
		HTTP2:          true,

		EnableMetrics:   false,
		MetricsEndpoint: "/metrics",
		EnablePprof:     false,
		Debug:           false,
		EnableWatcher:   true,

		StaticAssetMaxAge:  86400, // 24 hours for static assets
		DynamicAssetMaxAge: 3600,  // 1 hour for dynamic content

		EnableVersioning:  false, // Disabled by default
		VersioningPattern: "",    // Empty means use default: base.hash.ext
		VersionHashLength: 8,
		StaticPrefixes:    []string{"/static/", "/assets/", "/dist/", "/build/"},
	}
}

type Option func(*Config)

func WithRoot(root string) Option {
	return func(c *Config) {
		c.Root = root
	}
}

func WithCompression(types CompressionType) Option {
	return func(c *Config) {
		c.Compression = types
	}
}

func WithCompressionLevel(level int) Option {
	return func(c *Config) {
		c.CompressionLevel = level
	}
}

func WithCache(size int64) Option {
	return func(c *Config) {
		c.CacheSize = size
	}
}

func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Config) {
		c.CacheTTL = ttl
	}
}

func WithCacheStrategy(strategy CacheStrategy) Option {
	return func(c *Config) {
		c.CacheStrategy = strategy
	}
}

type TimeoutConfig struct {
	Read     time.Duration
	Write    time.Duration
	Idle     time.Duration
	Header   time.Duration
	Shutdown time.Duration
}

func WithTimeouts(tc TimeoutConfig) Option {
	return func(c *Config) {
		if tc.Read > 0 {
			c.ReadTimeout = tc.Read
		}
		if tc.Write > 0 {
			c.WriteTimeout = tc.Write
		}
		if tc.Idle > 0 {
			c.IdleTimeout = tc.Idle
		}
		if tc.Header > 0 {
			c.ReadHeaderTimeout = tc.Header
		}
		if tc.Shutdown > 0 {
			c.ShutdownTimeout = tc.Shutdown
		}
	}
}

func WithRateLimit(limit int) Option {
	return func(c *Config) {
		c.RateLimitPerIP = limit
	}
}

func WithHTTP2(enable bool) Option {
	return func(c *Config) {
		c.HTTP2 = enable
	}
}

func WithMetrics(enable bool) Option {
	return func(c *Config) {
		c.EnableMetrics = enable
	}
}

func WithWatcher(enable bool) Option {
	return func(c *Config) {
		c.EnableWatcher = enable
	}
}

func WithTLS(certFile, keyFile string) Option {
	return func(c *Config) {
		c.EnableHTTPS = true
		c.TLSCert = certFile
		c.TLSKey = keyFile
	}
}

func WithVersioning(enable bool) Option {
	return func(c *Config) {
		c.EnableVersioning = enable
	}
}

func WithVersioningPattern(pattern string) Option {
	return func(c *Config) {
		c.VersioningPattern = pattern
	}
}

func WithVersionHashLength(length int) Option {
	return func(c *Config) {
		if length < 4 {
			length = 4 // Minimum hash length
		} else if length > 16 {
			length = 16 // Maximum hash length
		}
		c.VersionHashLength = length
	}
}

func WithStaticPrefixes(prefixes ...string) Option {
	return func(c *Config) {
		c.StaticPrefixes = prefixes
	}
}

func WithURLPrefix(prefix string) Option {
	return func(c *Config) {
		c.URLPrefix = prefix
	}
}

type Preset int

const (
	PresetDevelopment Preset = iota
	PresetProduction
	PresetHighPerformance
	PresetSPA
	PresetStaticSite
	PresetAPI
	PresetHybrid
)

func NewWithPreset(preset Preset) *Config {
	config := DefaultConfig()

	switch preset {
	case PresetDevelopment:
		config.AllowBrowsing = true
		config.EnablePprof = true
		config.CacheSize = 10 << 20 // 10MB
		config.CacheTTL = 10 * time.Second
		config.EnableWatcher = true

	case PresetProduction:
		config.EnableMetrics = true
		config.CacheSize = 500 << 20 // 500MB
		config.CacheTTL = 10 * time.Minute
		config.MaxConnections = 5000
		config.RateLimitPerIP = 100

	case PresetHighPerformance:
		config.CacheSize = 1 << 30 // 1GB
		config.CacheTTL = 30 * time.Minute
		config.MaxConnections = 10000
		config.RateLimitPerIP = 1000
		config.ReadTimeout = 30 * time.Second
		config.WriteTimeout = 30 * time.Second

	case PresetSPA:
		// Single-page application with versioning
		config.EnableVersioning = true
		config.VersionHashLength = 8
		config.StaticPrefixes = []string{"/static/", "/assets/"}
		config.CacheSize = 50 << 20 // 50MB
		config.CacheTTL = 5 * time.Minute
		config.EnableWatcher = true
		config.StaticAssetMaxAge = 31536000 // 1 year for versioned assets
		config.DynamicAssetMaxAge = 0       // No cache for HTML

	case PresetStaticSite:
		// Static website with aggressive caching and versioning
		config.EnableVersioning = true
		config.VersionHashLength = 8
		config.StaticPrefixes = []string{"/css/", "/js/", "/images/", "/assets/"}
		config.CacheSize = 100 << 20 // 100MB
		config.CacheTTL = 10 * time.Minute
		config.AllowBrowsing = false
		config.StaticAssetMaxAge = 31536000 // 1 year for versioned assets
		config.DynamicAssetMaxAge = 3600    // 1 hour for HTML

	case PresetAPI:
		// API server with minimal static assets
		config.EnableVersioning = false
		config.CacheSize = 10 << 20 // 10MB
		config.CacheTTL = 1 * time.Minute
		config.StaticAssetMaxAge = 86400 // 1 day
		config.DynamicAssetMaxAge = 0    // No cache
		config.EnableMetrics = true

	case PresetHybrid:
		// Full-stack app with both API and static assets
		config.EnableVersioning = true
		config.VersionHashLength = 8
		config.StaticPrefixes = []string{"/static/", "/assets/", "/public/"}
		config.CacheSize = 100 << 20 // 100MB
		config.CacheTTL = 5 * time.Minute
		config.EnableWatcher = true
		config.StaticAssetMaxAge = 86400 // 1 day for static assets
		config.DynamicAssetMaxAge = 300  // 5 minutes for dynamic content
	}

	return config
}

// NewAuto creates a new server with automatic configuration based on directory structure
func NewAuto(root string) (*Config, error) {
	config := DefaultConfig()
	config.Root = root

	// Auto-detect if this looks like a SPA or static site
	hasIndexHTML := false
	hasAssetDirs := false
	assetDirs := []string{}

	if stat, err := os.Stat(filepath.Join(root, "index.html")); err == nil && !stat.IsDir() {
		hasIndexHTML = true
	}

	// Check for common asset directories
	commonDirs := []string{"css", "js", "images", "assets", "static", "dist", "build"}
	for _, dir := range commonDirs {
		if stat, err := os.Stat(filepath.Join(root, dir)); err == nil && stat.IsDir() {
			hasAssetDirs = true
			assetDirs = append(assetDirs, "/"+dir+"/")
		}
	}

	// Auto-configure based on detected structure
	if hasIndexHTML && hasAssetDirs {
		// Looks like a SPA or static site
		config.EnableVersioning = true
		config.VersionHashLength = 8
		config.StaticPrefixes = assetDirs
		config.CacheSize = 50 << 20 // 50MB
		config.CacheTTL = 5 * time.Minute
		config.EnableWatcher = true
		config.StaticAssetMaxAge = 31536000 // 1 year for versioned assets
		config.DynamicAssetMaxAge = 3600    // 1 hour for HTML
	} else if hasAssetDirs {
		// Has assets but no index.html, might be an API with static assets
		config.EnableVersioning = true
		config.VersionHashLength = 8
		config.StaticPrefixes = assetDirs
		config.CacheSize = 25 << 20 // 25MB
		config.CacheTTL = 2 * time.Minute
		config.StaticAssetMaxAge = 86400 // 1 day
	} else {
		// Simple static file server
		config.EnableVersioning = false
		config.CacheSize = 10 << 20 // 10MB
		config.CacheTTL = 1 * time.Minute
	}

	return config, nil
}

// SimpleConfig provides a simplified configuration structure for common use cases
type SimpleConfig struct {
	Root       string
	URLPrefix  string
	Versioning bool
	Cache      bool
	Compress   bool
	Debug      bool
}

// NewSimple creates a new server with simplified configuration
func NewSimple(sc SimpleConfig) (*Config, error) {
	config := DefaultConfig()

	if sc.Root != "" {
		config.Root = sc.Root
	}

	if sc.URLPrefix != "" {
		config.URLPrefix = sc.URLPrefix
	}

	config.EnableVersioning = sc.Versioning
	if sc.Versioning {
		config.VersionHashLength = 8
		// If URL prefix is set, use it in static prefixes
		if sc.URLPrefix != "" {
			config.StaticPrefixes = []string{sc.URLPrefix + "/"}
		} else {
			config.StaticPrefixes = []string{"/static/", "/assets/", "/css/", "/js/"}
		}
		config.StaticAssetMaxAge = 31536000 // 1 year for versioned assets
	}

	if !sc.Cache {
		config.CacheSize = 0
	}

	if !sc.Compress {
		config.Compression = NoCompression
	}

	config.Debug = sc.Debug

	return config, nil
}

// ValidateConfig validates the configuration and returns an error if invalid
func (c *Config) Validate() error {
	// Validate hash length
	if c.VersionHashLength < 4 || c.VersionHashLength > 16 {
		return fmt.Errorf("version hash length must be between 4 and 16 characters, got %d", c.VersionHashLength)
	}

	// Validate hash length is even (since we use half of hash bytes)
	if c.VersionHashLength%2 != 0 {
		return fmt.Errorf("version hash length must be even, got %d", c.VersionHashLength)
	}

	// Validate URL prefix and static prefixes compatibility
	if c.EnableVersioning && c.URLPrefix != "" && len(c.StaticPrefixes) > 0 {
		hasCompatiblePrefix := false
		for _, prefix := range c.StaticPrefixes {
			if strings.HasPrefix(prefix, c.URLPrefix) || c.URLPrefix == strings.TrimSuffix(prefix, "/") {
				hasCompatiblePrefix = true
				break
			}
		}
		if !hasCompatiblePrefix {
			return fmt.Errorf("when using versioning with URLPrefix '%s', at least one StaticPrefix should be compatible (e.g., '%s/')", c.URLPrefix, c.URLPrefix)
		}
	}

	return nil
}

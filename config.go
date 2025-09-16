package gostc

import (
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
	DefaultReadTimeout       = 15 * time.Second
	DefaultWriteTimeout      = 15 * time.Second
	DefaultIdleTimeout       = 60 * time.Second
	DefaultHeaderTimeout     = 5 * time.Second
	DefaultShutdownTimeout   = 30 * time.Second
	DefaultMaxHeaderBytes    = 1 << 20  // 1MB
	DefaultMaxBodySize       = 10 << 20 // 10MB
	DefaultMaxFileSize       = 100 << 20 // 100MB
	DefaultCacheSize         = 100 << 20 // 100MB
	DefaultCacheTTL          = 5 * time.Minute
	DefaultMinCompressSize   = 1024 // 1KB
	DefaultCompressionLevel  = 6
	DefaultMaxConnections    = 1000
	DefaultRateLimitPerIP    = 100 // requests per second
)

type Config struct {
	Root             string
	IndexFile        string
	AllowBrowsing    bool

	Compression      CompressionType
	CompressionLevel int
	MinSizeToCompress int64
	CompressTypes    []string

	CacheSize        int64
	CacheTTL         time.Duration
	CacheStrategy    CacheStrategy

	ReadTimeout      time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	ShutdownTimeout  time.Duration
	MaxHeaderBytes   int
	MaxBodySize      int64
	MaxFileSize      int64  // Maximum file size to serve

	MaxConnections   int
	MaxRequestsPerConn int
	RateLimitPerIP   int

	AllowedOrigins   []string
	AllowedMethods   []string
	CSPHeader        string
	EnableHTTPS      bool
	TLSCert          string
	TLSKey           string
	HTTP2            bool

	EnableMetrics    bool
	MetricsEndpoint  string
	EnablePprof      bool
	Debug            bool  // Enable debug mode with detailed errors

	EnableWatcher    bool

	// Cache control settings per file type
	StaticAssetMaxAge  int // Max age for static assets (images, fonts) in seconds
	DynamicAssetMaxAge int // Max age for dynamic assets (HTML, JSON) in seconds

	// Asset versioning settings
	EnableVersioning    bool
	VersioningPattern   string   // Pattern for versioned files (empty = default: base.hash.ext)
	VersionHashLength   int      // Length of version hash (default: 16)
	StaticPrefixes      []string // Prefixes that should be versioned
}

func DefaultConfig() *Config {
	return &Config{
		Root:             "./static",
		IndexFile:        "index.html",
		AllowBrowsing:    false,

		Compression:      Gzip | Brotli,
		CompressionLevel: DefaultCompressionLevel,
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

		CacheSize:        DefaultCacheSize,
		CacheTTL:         DefaultCacheTTL,
		CacheStrategy:    LRU,

		ReadTimeout:      DefaultReadTimeout,
		ReadHeaderTimeout: DefaultHeaderTimeout,
		WriteTimeout:     DefaultWriteTimeout,
		IdleTimeout:      DefaultIdleTimeout,
		ShutdownTimeout:  DefaultShutdownTimeout,
		MaxHeaderBytes:   DefaultMaxHeaderBytes,
		MaxBodySize:      DefaultMaxBodySize,
		MaxFileSize:      DefaultMaxFileSize,

		MaxConnections:   DefaultMaxConnections,
		RateLimitPerIP:   DefaultRateLimitPerIP,

		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "HEAD", "OPTIONS"},
		HTTP2:            true,

		EnableMetrics:    false,
		MetricsEndpoint:  "/metrics",
		EnablePprof:      false,
		Debug:            false,
		EnableWatcher:    true,

		StaticAssetMaxAge:  86400,  // 24 hours for static assets
		DynamicAssetMaxAge: 3600,   // 1 hour for dynamic content

		EnableVersioning:   false,  // Disabled by default
		VersioningPattern:  "",     // Empty means use default: base.hash.ext
		VersionHashLength:  16,
		StaticPrefixes:     []string{"/static/", "/assets/", "/dist/", "/build/"},
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
	Read       time.Duration
	Write      time.Duration
	Idle       time.Duration
	Header     time.Duration
	Shutdown   time.Duration
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
		c.VersionHashLength = length
	}
}

func WithStaticPrefixes(prefixes ...string) Option {
	return func(c *Config) {
		c.StaticPrefixes = prefixes
	}
}

type Preset int

const (
	PresetDevelopment Preset = iota
	PresetProduction
	PresetHighPerformance
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
	}

	return config
}
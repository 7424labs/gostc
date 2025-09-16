package gostc

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"net"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	config       *Config
	cache        Cache
	compression  *CompressionManager
	invalidator  Invalidator
	handler      http.Handler
	httpServer   *http.Server
	metrics      *Metrics
	mu           sync.RWMutex
	shutdown     chan struct{}
}

type Metrics struct {
	requestsTotal    prometheus.Counter
	requestDuration  prometheus.Histogram
	cacheHits        prometheus.Counter
	cacheMisses      prometheus.Counter
	bytesServed      prometheus.Counter
	activeConnections prometheus.Gauge
}

func New(opts ...Option) (*Server, error) {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	cache, err := NewCache(config)
	if err != nil {
		return nil, err
	}

	compression := NewCompressionManager(config)

	s := &Server{
		config:      config,
		cache:       cache,
		compression: compression,
		shutdown:    make(chan struct{}),
	}

	if config.EnableWatcher {
		watcher, err := NewFileWatcher(config.Root, cache, compression)
		if err != nil {
			return nil, err
		}
		s.invalidator = watcher
	} else {
		s.invalidator = NewManualInvalidator(cache)
	}

	if config.EnableMetrics {
		s.setupMetrics()
	}

	s.setupHandler()
	s.setupHTTPServer()

	return s, nil
}

func (s *Server) setupMetrics() {
	s.metrics = &Metrics{
		requestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gostc_requests_total",
			Help: "Total number of requests",
		}),
		requestDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "gostc_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		}),
		cacheHits: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gostc_cache_hits_total",
			Help: "Total number of cache hits",
		}),
		cacheMisses: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gostc_cache_misses_total",
			Help: "Total number of cache misses",
		}),
		bytesServed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gostc_bytes_served_total",
			Help: "Total bytes served",
		}),
		activeConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gostc_active_connections",
			Help: "Number of active connections",
		}),
	}

	prometheus.MustRegister(
		s.metrics.requestsTotal,
		s.metrics.requestDuration,
		s.metrics.cacheHits,
		s.metrics.cacheMisses,
		s.metrics.bytesServed,
		s.metrics.activeConnections,
	)
}

func (s *Server) setupHandler() {
	mux := http.NewServeMux()

	fileHandler := http.HandlerFunc(s.serveFile)

	middlewares := []Middleware{
		RecoveryMiddleware(),
		LoggingMiddleware(),
		SecurityHeadersMiddleware(s.config),
		CORSMiddleware(s.config),
	}

	if s.config.RateLimitPerIP > 0 {
		middlewares = append(middlewares, RateLimitMiddleware(s.config.RateLimitPerIP))
	}

	if s.config.MaxBodySize > 0 {
		middlewares = append(middlewares, MaxBytesMiddleware(s.config.MaxBodySize))
	}

	if s.config.ReadTimeout > 0 {
		middlewares = append(middlewares, TimeoutMiddleware(s.config.ReadTimeout))
	}

	handler := ChainMiddleware(fileHandler, middlewares...)

	mux.Handle("/", handler)

	if s.config.EnableMetrics {
		mux.Handle(s.config.MetricsEndpoint, promhttp.Handler())
	}

	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.Handle("/health", ChainMiddleware(healthHandler, middlewares...))

	s.handler = mux
}

func (s *Server) setupHTTPServer() {
	s.httpServer = &http.Server{
		Addr:              ":8080",
		Handler:           s.handler,
		ReadTimeout:       s.config.ReadTimeout,
		ReadHeaderTimeout: s.config.ReadHeaderTimeout,
		WriteTimeout:      s.config.WriteTimeout,
		IdleTimeout:       s.config.IdleTimeout,
		MaxHeaderBytes:    s.config.MaxHeaderBytes,
	}

	if s.config.MaxConnections > 0 {
		s.httpServer.ConnState = s.connStateHandler
	}
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	if s.metrics != nil {
		s.metrics.requestsTotal.Inc()
		defer func(start time.Time) {
			s.metrics.requestDuration.Observe(time.Since(start).Seconds())
		}(time.Now())
	}

	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	urlPath := r.URL.Path
	fullPath := filepath.Join(s.config.Root, filepath.Clean(urlPath))

	if !strings.HasPrefix(fullPath, s.config.Root) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	acceptEncoding := r.Header.Get("Accept-Encoding")
	compressor, compressionType := s.compression.GetCompressor(acceptEncoding)

	cacheKey := CacheKey{
		Path:        urlPath,
		Compression: compressionType,
	}

	if entry, ok := s.cache.Get(cacheKey); ok {
		if s.metrics != nil {
			s.metrics.cacheHits.Inc()
		}

		s.serveFromCache(w, r, entry, compressionType)
		return
	}

	if s.metrics != nil {
		s.metrics.cacheMisses.Inc()
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	if info.IsDir() {
		indexPath := filepath.Join(fullPath, s.config.IndexFile)
		if indexInfo, err := os.Stat(indexPath); err == nil && !indexInfo.IsDir() {
			fullPath = indexPath
			info = indexInfo
			urlPath = filepath.Join(urlPath, s.config.IndexFile)
		} else if s.config.AllowBrowsing {
			s.serveDirectory(w, r, fullPath)
			return
		} else {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
	}

	s.serveFileWithCompression(w, r, fullPath, info, compressor, compressionType)
}

func (s *Server) serveFromCache(w http.ResponseWriter, r *http.Request, entry *CacheEntry, compressionType CompressionType) {
	w.Header().Set("Content-Type", entry.ContentType)
	w.Header().Set("ETag", entry.ETag)
	w.Header().Set("Last-Modified", entry.LastModified.UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", getCacheControl(r.URL.Path, s.config))

	if compressionType != NoCompression {
		w.Header().Set("Content-Encoding", getEncodingName(compressionType))
		w.Header().Set("Vary", "Accept-Encoding")
	}

	// Check If-None-Match (ETag)
	if r.Header.Get("If-None-Match") == entry.ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Check If-Modified-Since
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		imsTime, err := http.ParseTime(ims)
		if err == nil && !entry.LastModified.After(imsTime) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	if r.Method == "HEAD" {
		w.Header().Set("Content-Length", strconv.FormatInt(entry.Size, 10))
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(entry.Data)), 10))
	w.Write(entry.Data)

	if s.metrics != nil {
		s.metrics.bytesServed.Add(float64(len(entry.Data)))
	}
}

func (s *Server) serveFileWithCompression(w http.ResponseWriter, r *http.Request, fullPath string, info os.FileInfo, compressor Compressor, compressionType CompressionType) {
	file, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(fullPath))
	if contentType == "" {
		contentType = http.DetectContentType(data[:512])
	}

	etag := generateETag(data)
	lastModified := info.ModTime()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", lastModified.UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", getCacheControl(r.URL.Path, s.config))

	// Check If-None-Match (ETag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Check If-Modified-Since
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		imsTime, err := http.ParseTime(ims)
		if err == nil && !lastModified.After(imsTime) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	shouldCompress := compressor != nil && compressionType != NoCompression &&
		s.compression.ShouldCompress(contentType, info.Size())

	var responseData []byte
	if shouldCompress {
		compressed, err := compressor.Compress(data, s.config.CompressionLevel)
		if err == nil {
			responseData = compressed
			w.Header().Set("Content-Encoding", getEncodingName(compressionType))
			w.Header().Set("Vary", "Accept-Encoding")

			entry := &CacheEntry{
				Data:         responseData,
				ContentType:  contentType,
				ETag:         etag,
				LastModified: lastModified,
				Size:         int64(len(responseData)),
			}
			s.cache.Set(CacheKey{Path: r.URL.Path, Compression: compressionType}, entry)
		} else {
			responseData = data
		}
	} else {
		responseData = data

		entry := &CacheEntry{
			Data:         responseData,
			ContentType:  contentType,
			ETag:         etag,
			LastModified: lastModified,
			Size:         int64(len(responseData)),
		}
		s.cache.Set(CacheKey{Path: r.URL.Path, Compression: NoCompression}, entry)
	}

	if r.Method == "HEAD" {
		w.Header().Set("Content-Length", strconv.FormatInt(int64(len(responseData)), 10))
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(responseData)), 10))
	w.Write(responseData)

	if s.metrics != nil {
		s.metrics.bytesServed.Add(float64(len(responseData)))
	}
}

func (s *Server) serveDirectory(w http.ResponseWriter, r *http.Request, dirPath string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<html><head><title>Directory listing for %s</title></head><body>", r.URL.Path)
	fmt.Fprintf(w, "<h1>Directory listing for %s</h1><ul>", r.URL.Path)

	if r.URL.Path != "/" {
		fmt.Fprintf(w, `<li><a href="../">../</a></li>`)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		fmt.Fprintf(w, `<li><a href="%s">%s</a></li>`, name, name)
	}

	fmt.Fprintf(w, "</ul></body></html>")
}

func (s *Server) connStateHandler(conn net.Conn, state http.ConnState) {
	if s.metrics == nil {
		return
	}

	switch state {
	case http.StateNew:
		s.metrics.activeConnections.Inc()
	case http.StateClosed, http.StateHijacked:
		s.metrics.activeConnections.Dec()
	}
}

func (s *Server) Start() error {
	if s.invalidator != nil {
		if err := s.invalidator.Start(); err != nil {
			return fmt.Errorf("failed to start invalidator: %w", err)
		}
	}

	go func() {
		log.Printf("Starting server on %s", s.httpServer.Addr)

		var err error
		if s.config.EnableHTTPS {
			err = s.httpServer.ListenAndServeTLS(s.config.TLSCert, s.config.TLSKey)
		} else {
			err = s.httpServer.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	close(s.shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()

	if s.invalidator != nil {
		s.invalidator.Stop()
	}

	return s.httpServer.Shutdown(ctx)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) InvalidatePath(path string) {
	s.invalidator.InvalidatePath(path)
}

func (s *Server) InvalidateAll() {
	s.invalidator.InvalidateAll()
}

func (s *Server) CacheStats() CacheStats {
	return s.cache.Stats()
}

func generateETag(data []byte) string {
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}

func getEncodingName(compressionType CompressionType) string {
	switch compressionType {
	case Gzip:
		return "gzip"
	case Brotli:
		return "br"
	default:
		return ""
	}
}
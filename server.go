package gostc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
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
	config         *Config
	cache          Cache
	compression    *CompressionManager
	invalidator    Invalidator
	versionManager *AssetVersionManager
	htmlProcessor  *HTMLProcessor
	handler        http.Handler
	httpServer     *http.Server
	metrics        *Metrics
	csrfProtection *CSRFProtection
	rateLimiter    *IPRateLimiter
	errorHandler  *ErrorHandler
	mu             sync.RWMutex
	shutdown       chan struct{}
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
	versionManager := NewAssetVersionManager(config)
	htmlProcessor := NewHTMLProcessor(versionManager)

	s := &Server{
		config:         config,
		cache:          cache,
		compression:    compression,
		versionManager: versionManager,
		htmlProcessor:  htmlProcessor,
		csrfProtection: NewCSRFProtection(time.Hour),
		rateLimiter:    NewIPRateLimiter(config.RateLimitPerIP, config.RateLimitPerIP*10, 5*time.Minute),
		errorHandler:  NewErrorHandler(config.Debug),
		shutdown:       make(chan struct{}),
	}

	if config.EnableWatcher {
		var watcher *FileWatcher
		var err error

		if config.EnableVersioning {
			watcher, err = NewVersionedFileWatcher(config.Root, cache, compression, versionManager)
		} else {
			watcher, err = NewFileWatcher(config.Root, cache, compression)
		}

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

	// Initialize asset versioning if enabled
	if config.EnableVersioning {
		if err := s.versionManager.ScanDirectory(config.Root); err != nil {
			return nil, fmt.Errorf("failed to scan directory for versioning: %w", err)
		}
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

	if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
		err := NewServerError(ErrorTypeValidation, "server.serveFile", nil).
			WithMessage("Method not allowed").
			WithStatusCode(http.StatusMethodNotAllowed)
		s.errorHandler.HandleError(w, r, err)
		return
	}

	// Apply request size limit for all methods
	if r.ContentLength > 0 && r.ContentLength > s.config.MaxBodySize {
		err := NewServerError(ErrorTypeValidation, "server.serveFile", ErrRequestTooLarge).
			WithStatusCode(http.StatusRequestEntityTooLarge)
		s.errorHandler.HandleError(w, r, err)
		return
	}

	urlPath := r.URL.Path

	// Validate and sanitize the URL path
	if !isValidPath(urlPath) {
		err := NewServerError(ErrorTypeSecurity, "server.serveFile", ErrInvalidPath).
			WithPath(urlPath)
		s.errorHandler.HandleError(w, r, err)
		return
	}

	originalPath := urlPath
	isVersioned := false

	// Check if this is a versioned asset path and resolve to original
	if s.config.EnableVersioning && s.versionManager.IsVersionedPath(urlPath) {
		if resolvedPath, exists := s.versionManager.GetOriginalPath(urlPath); exists {
			originalPath = resolvedPath
			isVersioned = true
		}
	}

	// Clean and secure the path
	cleanedPath := path.Clean("/" + strings.TrimPrefix(originalPath, "/"))
	fullPath, err := securePath(s.config.Root, cleanedPath)
	if err != nil {
		serverErr := NewServerError(ErrorTypeSecurity, "server.securePath", ErrPathTraversal).
			WithPath(originalPath)
		s.errorHandler.HandleError(w, r, serverErr)
		return
	}

	acceptEncoding := r.Header.Get("Accept-Encoding")
	compressor, compressionType := s.compression.GetCompressor(acceptEncoding)

	cacheKey := CacheKey{
		Path:        urlPath,
		Compression: compressionType,
		IsVersioned: isVersioned,
	}

	if entry, ok := s.cache.Get(cacheKey); ok {
		if s.metrics != nil {
			s.metrics.cacheHits.Inc()
		}

		s.serveFromCache(w, r, entry, compressionType, isVersioned)
		return
	}

	if s.metrics != nil {
		s.metrics.cacheMisses.Inc()
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		var serverErr *ServerError
		if os.IsNotExist(err) {
			serverErr = NewServerError(ErrorTypeNotFound, "server.stat", err).
				WithPath(originalPath)
		} else if os.IsPermission(err) {
			serverErr = NewServerError(ErrorTypePermission, "server.stat", err).
				WithPath(originalPath)
		} else {
			serverErr = NewServerError(ErrorTypeServerError, "server.stat", err).
				WithPath(originalPath)
		}
		s.errorHandler.HandleError(w, r, serverErr)
		return
	}

	if info.IsDir() {
		indexPath := filepath.Join(fullPath, s.config.IndexFile)
		if indexInfo, err := os.Stat(indexPath); err == nil && !indexInfo.IsDir() {
			fullPath = indexPath
			info = indexInfo
			originalPath = filepath.Join(originalPath, s.config.IndexFile)
			urlPath = originalPath
		} else if s.config.AllowBrowsing {
			s.serveDirectory(w, r, fullPath)
			return
		} else {
			err := NewServerError(ErrorTypeNotFound, "server.serveFile", nil).
				WithPath(originalPath).
				WithMessage("Directory listing disabled")
			s.errorHandler.HandleError(w, r, err)
			return
		}
	}

	s.serveFileWithCompression(w, r, fullPath, info, compressor, compressionType, isVersioned, originalPath)
}

func (s *Server) serveFromCache(w http.ResponseWriter, r *http.Request, entry *CacheEntry, compressionType CompressionType, isVersioned bool) {
	w.Header().Set("Content-Type", entry.ContentType)
	w.Header().Set("ETag", entry.ETag)
	w.Header().Set("Last-Modified", entry.LastModified.UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", getCacheControl(r.URL.Path, s.config, isVersioned))

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

func (s *Server) serveFileWithCompression(w http.ResponseWriter, r *http.Request, fullPath string, info os.FileInfo, compressor Compressor, compressionType CompressionType, isVersioned bool, originalPath string) {
	file, err := os.Open(fullPath)
	if err != nil {
		var serverErr *ServerError
		if os.IsPermission(err) {
			serverErr = NewServerError(ErrorTypePermission, "server.openFile", err).
				WithPath(fullPath)
		} else {
			serverErr = NewServerError(ErrorTypeServerError, "server.openFile", err).
				WithPath(fullPath)
		}
		s.errorHandler.HandleError(w, r, serverErr)
		return
	}
	defer SafeClose(file)

	// Limit the amount of data read to prevent memory exhaustion
	limitedReader := io.LimitReader(file, s.config.MaxFileSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		serverErr := NewServerError(ErrorTypeServerError, "server.readFile", err).
			WithPath(fullPath)
		s.errorHandler.HandleError(w, r, serverErr)
		return
	}

	// Check if file exceeded size limit
	if int64(len(data)) == s.config.MaxFileSize {
		// Try to read one more byte to check if file is larger
		if _, err := file.Read(make([]byte, 1)); err == nil {
			serverErr := NewServerError(ErrorTypeValidation, "server.readFile", ErrFileTooLarge).
				WithPath(fullPath).
				WithMessage(fmt.Sprintf("File exceeds maximum size of %d bytes", s.config.MaxFileSize))
			s.errorHandler.HandleError(w, r, serverErr)
			return
		}
	}

	contentType := mime.TypeByExtension(filepath.Ext(fullPath))
	if contentType == "" {
		contentType = http.DetectContentType(data[:512])
	}

	// Register asset for versioning if enabled and not already registered
	if s.config.EnableVersioning && !isVersioned && s.versionManager.shouldVersionFile(originalPath) {
		s.versionManager.RegisterAsset(originalPath, data)
	}

	etag := generateETag(data)
	lastModified := info.ModTime()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", lastModified.UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", getCacheControl(r.URL.Path, s.config, isVersioned))

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

	// Process HTML files to inject versioned asset references BEFORE compression
	processedData := data
	if s.config.EnableVersioning && (contentType == "text/html" || strings.Contains(contentType, "text/html")) {
		processedData = s.htmlProcessor.ProcessHTML(data, originalPath)
		// Update ETag after HTML processing since content changed
		etag = generateETag(processedData)
	}

	shouldCompress := compressor != nil && compressionType != NoCompression &&
		s.compression.ShouldCompress(contentType, info.Size())

	var responseData []byte
	if shouldCompress {
		compressed, err := compressor.Compress(processedData, s.config.CompressionLevel)
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
			s.cache.Set(CacheKey{Path: r.URL.Path, Compression: compressionType, IsVersioned: isVersioned}, entry)
		} else {
			responseData = processedData
		}
	} else {
		responseData = processedData

		entry := &CacheEntry{
			Data:         responseData,
			ContentType:  contentType,
			ETag:         etag,
			LastModified: lastModified,
			Size:         int64(len(responseData)),
		}
		s.cache.Set(CacheKey{Path: r.URL.Path, Compression: NoCompression, IsVersioned: isVersioned}, entry)
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

// ServeFileHTTP serves files directly without going through the internal mux
// This is useful when embedding gostc as a handler to avoid mux conflicts
func (s *Server) ServeFileHTTP(w http.ResponseWriter, r *http.Request) {
	// Create the file handler with middlewares, but bypass the internal mux
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
	handler.ServeHTTP(w, r)
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
	hash := sha256.Sum256(data)
	return `"` + hex.EncodeToString(hash[:16]) + `"`
}

// isValidPath checks if the path contains any suspicious patterns
func isValidPath(urlPath string) bool {
	// Reject paths with null bytes
	if strings.Contains(urlPath, "\x00") {
		return false
	}

	// Reject paths with suspicious patterns
	suspiciousPatterns := []string{
		"../",
		"..\\",
		"..%2f",
		"..%2F",
		"..%5c",
		"..%5C",
		"%00",
		"./.",
		".%2e",
		"%252e",
	}

	lowerPath := strings.ToLower(urlPath)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(lowerPath, pattern) {
			return false
		}
	}

	// Reject overly long paths
	if len(urlPath) > 2048 {
		return false
	}

	return true
}

// securePath safely joins and validates a root directory with a relative path
func securePath(root, relPath string) (string, error) {
	// Clean the relative path
	relPath = path.Clean(relPath)

	// Ensure the path doesn't escape the root
	if strings.HasPrefix(relPath, "..") || strings.Contains(relPath, "/..") {
		return "", fmt.Errorf("path traversal detected")
	}

	// Join with root and convert to absolute path
	fullPath := filepath.Join(root, strings.TrimPrefix(relPath, "/"))

	// Get absolute paths for comparison
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	// Ensure the resolved path is within the root directory
	if !strings.HasPrefix(absPath, absRoot) {
		return "", fmt.Errorf("path escapes root directory")
	}

	return absPath, nil
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
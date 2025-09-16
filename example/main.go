package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourusername/gostc"
)

func main() {
	var (
		root       = flag.String("root", "./static", "Static files directory")
		addr       = flag.String("addr", ":8080", "Server address")
		cacheSize  = flag.Int64("cache", 100*1024*1024, "Cache size in bytes")
		cacheTTL   = flag.Duration("ttl", 5*time.Minute, "Cache TTL")
		rateLimit  = flag.Int("rate", 100, "Rate limit per IP (requests/second)")
		compress   = flag.String("compress", "all", "Compression: none, gzip, brotli, all")
		production = flag.Bool("production", false, "Use production preset")
		metrics    = flag.Bool("metrics", false, "Enable metrics endpoint")
		tls        = flag.Bool("tls", false, "Enable TLS")
		certFile   = flag.String("cert", "", "TLS certificate file")
		keyFile    = flag.String("key", "", "TLS key file")
	)
	flag.Parse()

	var compressionType gostc.CompressionType
	switch *compress {
	case "none":
		compressionType = gostc.NoCompression
	case "gzip":
		compressionType = gostc.Gzip
	case "brotli":
		compressionType = gostc.Brotli
	default:
		compressionType = gostc.Gzip | gostc.Brotli
	}

	var opts []gostc.Option

	if *production {
		config := gostc.NewWithPreset(gostc.PresetProduction)
		config.Root = *root
		opts = append(opts, func(c *gostc.Config) { *c = *config })
	} else {
		opts = []gostc.Option{
			gostc.WithRoot(*root),
			gostc.WithCompression(compressionType),
			gostc.WithCache(*cacheSize),
			gostc.WithCacheTTL(*cacheTTL),
			gostc.WithRateLimit(*rateLimit),
			gostc.WithMetrics(*metrics),
			gostc.WithWatcher(true),
		}
	}

	if *tls && *certFile != "" && *keyFile != "" {
		opts = append(opts, gostc.WithTLS(*certFile, *keyFile))
	}

	server, err := gostc.New(opts...)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Printf("Server started on %s", *addr)
	log.Printf("Serving files from: %s", *root)
	log.Printf("Compression: %s", *compress)
	log.Printf("Cache size: %d bytes, TTL: %v", *cacheSize, *cacheTTL)
	log.Printf("Rate limit: %d requests/second per IP", *rateLimit)

	if *metrics {
		log.Printf("Metrics available at: http://localhost%s/metrics", *addr)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down server...")

	if err := server.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("Server stopped gracefully")
}
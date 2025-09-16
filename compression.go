package gostc

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
)

type Compressor interface {
	Compress(data []byte, level int) ([]byte, error)
	ContentEncoding() string
}

type GzipCompressor struct {
	pool sync.Pool
}

func NewGzipCompressor() *GzipCompressor {
	return &GzipCompressor{
		pool: sync.Pool{
			New: func() interface{} {
				w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
				return w
			},
		},
	}
}

func (g *GzipCompressor) Compress(data []byte, level int) ([]byte, error) {
	if level < 1 || level > 9 {
		level = gzip.DefaultCompression
	}

	var buf bytes.Buffer

	gw := g.pool.Get().(*gzip.Writer)
	defer g.pool.Put(gw)

	gw.Reset(&buf)

	if _, err := gw.Write(data); err != nil {
		return nil, err
	}

	if err := gw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (g *GzipCompressor) ContentEncoding() string {
	return "gzip"
}

type BrotliCompressor struct {
}

func NewBrotliCompressor() *BrotliCompressor {
	return &BrotliCompressor{}
}

func (b *BrotliCompressor) Compress(data []byte, level int) ([]byte, error) {
	if level < 0 || level > 11 {
		level = brotli.DefaultCompression
	}

	var buf bytes.Buffer

	bw := brotli.NewWriterLevel(&buf, level)

	if _, err := bw.Write(data); err != nil {
		return nil, err
	}

	if err := bw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (b *BrotliCompressor) ContentEncoding() string {
	return "br"
}

type CompressionManager struct {
	config      *Config
	gzip        *GzipCompressor
	brotli      *BrotliCompressor
	mu          sync.RWMutex
}

func NewCompressionManager(config *Config) *CompressionManager {
	return &CompressionManager{
		config: config,
		gzip:   NewGzipCompressor(),
		brotli: NewBrotliCompressor(),
	}
}

func (cm *CompressionManager) ShouldCompress(contentType string, size int64) bool {
	if size < cm.config.MinSizeToCompress {
		return false
	}

	for _, ct := range cm.config.CompressTypes {
		if strings.Contains(contentType, ct) {
			return true
		}
	}

	return false
}

func (cm *CompressionManager) GetCompressor(acceptEncoding string) (Compressor, CompressionType) {
	acceptEncoding = strings.ToLower(acceptEncoding)

	if cm.config.Compression&Brotli != 0 && strings.Contains(acceptEncoding, "br") {
		return cm.brotli, Brotli
	}

	if cm.config.Compression&Gzip != 0 &&
		(strings.Contains(acceptEncoding, "gzip") || strings.Contains(acceptEncoding, "*")) {
		return cm.gzip, Gzip
	}

	return nil, NoCompression
}

func (cm *CompressionManager) Compress(data []byte, compressionType CompressionType) ([]byte, error) {
	var compressor Compressor

	switch compressionType {
	case Gzip:
		compressor = cm.gzip
	case Brotli:
		compressor = cm.brotli
	default:
		return data, nil
	}

	return compressor.Compress(data, cm.config.CompressionLevel)
}

func ParseAcceptEncoding(header string) []string {
	var encodings []string
	parts := strings.Split(header, ",")

	for _, part := range parts {
		encoding := strings.TrimSpace(part)
		if idx := strings.Index(encoding, ";"); idx != -1 {
			encoding = encoding[:idx]
		}
		encodings = append(encodings, strings.ToLower(encoding))
	}

	return encodings
}

type CompressedResponseWriter struct {
	io.Writer
	http.ResponseWriter
	compressor Compressor
}

func NewCompressedResponseWriter(w http.ResponseWriter, compressor Compressor) *CompressedResponseWriter {
	return &CompressedResponseWriter{
		ResponseWriter: w,
		compressor:     compressor,
	}
}

func (w *CompressedResponseWriter) Write(b []byte) (int, error) {
	compressed, err := w.compressor.Compress(b, 0)
	if err != nil {
		return 0, err
	}

	return w.ResponseWriter.Write(compressed)
}

func (w *CompressedResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.Header().Set("Content-Encoding", w.compressor.ContentEncoding())
	w.ResponseWriter.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(statusCode)
}
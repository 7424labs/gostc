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
	writerPool sync.Pool
	bufferPool sync.Pool
}

func NewGzipCompressor() *GzipCompressor {
	return &GzipCompressor{
		writerPool: sync.Pool{
			New: func() interface{} {
				w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
				return w
			},
		},
		bufferPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

func (g *GzipCompressor) Compress(data []byte, level int) ([]byte, error) {
	if level < 1 || level > 9 {
		level = gzip.DefaultCompression
	}

	buf := g.bufferPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		g.bufferPool.Put(buf)
	}()

	gw := g.writerPool.Get().(*gzip.Writer)
	defer g.writerPool.Put(gw)

	gw.Reset(buf)

	if _, err := gw.Write(data); err != nil {
		return nil, err
	}

	if err := gw.Close(); err != nil {
		return nil, err
	}

	// Copy the bytes to avoid reuse issues
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())

	return result, nil
}

func (g *GzipCompressor) ContentEncoding() string {
	return "gzip"
}

type BrotliCompressor struct {
	bufferPool sync.Pool
	writerPool sync.Pool
}

func NewBrotliCompressor() *BrotliCompressor {
	return &BrotliCompressor{
		bufferPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
		writerPool: sync.Pool{
			New: func() interface{} {
				return brotli.NewWriterLevel(nil, brotli.DefaultCompression)
			},
		},
	}
}

func (b *BrotliCompressor) Compress(data []byte, level int) ([]byte, error) {
	if level < 0 || level > 11 {
		level = brotli.DefaultCompression
	}

	buf := b.bufferPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		b.bufferPool.Put(buf)
	}()

	bw := b.writerPool.Get().(*brotli.Writer)
	defer b.writerPool.Put(bw)

	bw.Reset(buf)

	if _, err := bw.Write(data); err != nil {
		return nil, err
	}

	if err := bw.Close(); err != nil {
		return nil, err
	}

	// Copy the bytes to avoid reuse issues
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())

	return result, nil
}

func (b *BrotliCompressor) ContentEncoding() string {
	return "br"
}

type CompressionManager struct {
	config *Config
	gzip   *GzipCompressor
	brotli *BrotliCompressor
	mu     sync.RWMutex
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

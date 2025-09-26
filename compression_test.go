package gostc

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestGzipCompressor(t *testing.T) {
	compressor := NewGzipCompressor()
	testData := []byte("This is test data that should be compressed. " + strings.Repeat("repeat ", 100))

	compressed, err := compressor.Compress(testData, 6)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	// Verify compression actually reduced size
	if len(compressed) >= len(testData) {
		t.Error("Compressed data should be smaller than original")
	}

	// Verify we can decompress it
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	if !bytes.Equal(decompressed, testData) {
		t.Error("Decompressed data doesn't match original")
	}
}

func TestBrotliCompressor(t *testing.T) {
	compressor := NewBrotliCompressor()
	testData := []byte("This is test data that should be compressed. " + strings.Repeat("repeat ", 100))

	compressed, err := compressor.Compress(testData, 6)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	// Verify compression actually reduced size
	if len(compressed) >= len(testData) {
		t.Error("Compressed data should be smaller than original")
	}

	// Verify we can decompress it
	reader := brotli.NewReader(bytes.NewReader(compressed))
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	if !bytes.Equal(decompressed, testData) {
		t.Error("Decompressed data doesn't match original")
	}
}

func TestCompressionManager(t *testing.T) {
	config := &Config{
		Compression:       Gzip | Brotli,
		CompressionLevel:  6,
		MinSizeToCompress: 100,
		CompressTypes: []string{
			"text/html",
			"text/css",
			"application/javascript",
		},
	}

	manager := NewCompressionManager(config)

	t.Run("ShouldCompress", func(t *testing.T) {
		// Should compress
		if !manager.ShouldCompress("text/html", 1000) {
			t.Error("Should compress text/html over minimum size")
		}

		// Should not compress - too small
		if manager.ShouldCompress("text/html", 50) {
			t.Error("Should not compress files under minimum size")
		}

		// Should not compress - wrong type
		if manager.ShouldCompress("image/png", 1000) {
			t.Error("Should not compress image/png")
		}
	})

	t.Run("GetCompressor", func(t *testing.T) {
		// Test Brotli preference
		compressor, compType := manager.GetCompressor("br, gzip")
		if compType != Brotli {
			t.Error("Should prefer Brotli when supported")
		}
		if compressor == nil {
			t.Error("Should return Brotli compressor")
		}

		// Test Gzip fallback
		compressor, compType = manager.GetCompressor("gzip, deflate")
		if compType != Gzip {
			t.Error("Should use Gzip when Brotli not supported")
		}

		// Test no compression
		compressor, compType = manager.GetCompressor("deflate")
		if compType != NoCompression {
			t.Error("Should return NoCompression when no supported encoding")
		}
		if compressor != nil {
			t.Error("Should return nil compressor")
		}
	})
}

func TestCompressionLevels(t *testing.T) {
	testData := []byte(strings.Repeat("compress this data ", 100))

	t.Run("Gzip Levels", func(t *testing.T) {
		compressor := NewGzipCompressor()

		// Test invalid levels
		_, err := compressor.Compress(testData, 0)
		if err != nil {
			t.Error("Should handle invalid level 0")
		}

		_, err = compressor.Compress(testData, 10)
		if err != nil {
			t.Error("Should handle invalid level 10")
		}

		// Test valid levels
		for level := 1; level <= 9; level++ {
			compressed, err := compressor.Compress(testData, level)
			if err != nil {
				t.Errorf("Failed at level %d: %v", level, err)
			}
			if len(compressed) >= len(testData) {
				t.Errorf("Level %d: compressed size should be smaller", level)
			}
		}
	})

	t.Run("Brotli Levels", func(t *testing.T) {
		compressor := NewBrotliCompressor()

		// Test invalid levels
		_, err := compressor.Compress(testData, -1)
		if err != nil {
			t.Error("Should handle invalid level -1")
		}

		_, err = compressor.Compress(testData, 12)
		if err != nil {
			t.Error("Should handle invalid level 12")
		}

		// Test valid levels (0-11 for Brotli)
		for level := 0; level <= 11; level++ {
			compressed, err := compressor.Compress(testData, level)
			if err != nil {
				t.Errorf("Failed at level %d: %v", level, err)
			}
			// Level 0 might not compress much
			if level > 0 && len(compressed) >= len(testData) {
				t.Errorf("Level %d: compressed size should be smaller", level)
			}
		}
	})
}

func TestCompressionPooling(t *testing.T) {
	// Test that pooling doesn't cause data corruption
	gzipComp := NewGzipCompressor()
	brotliComp := NewBrotliCompressor()

	data1 := []byte("first data set")
	data2 := []byte("second data set")

	// Compress both in sequence
	compressed1, err := gzipComp.Compress(data1, 6)
	if err != nil {
		t.Fatalf("Failed to compress data1: %v", err)
	}

	compressed2, err := gzipComp.Compress(data2, 6)
	if err != nil {
		t.Fatalf("Failed to compress data2: %v", err)
	}

	// Verify they're different
	if bytes.Equal(compressed1, compressed2) {
		t.Error("Different data should produce different compressed output")
	}

	// Same test for Brotli
	brotliCompressed1, err := brotliComp.Compress(data1, 6)
	if err != nil {
		t.Fatalf("Failed to compress with Brotli: %v", err)
	}

	brotliCompressed2, err := brotliComp.Compress(data2, 6)
	if err != nil {
		t.Fatalf("Failed to compress with Brotli: %v", err)
	}

	if bytes.Equal(brotliCompressed1, brotliCompressed2) {
		t.Error("Different data should produce different Brotli compressed output")
	}
}

func BenchmarkGzipCompressor(b *testing.B) {
	compressor := NewGzipCompressor()
	data := []byte(strings.Repeat("benchmark data ", 1000))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := compressor.Compress(data, 6)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBrotliCompression(b *testing.B) {
	compressor := NewBrotliCompressor()
	data := []byte(strings.Repeat("benchmark data ", 1000))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := compressor.Compress(data, 6)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompressionWithPooling(b *testing.B) {
	compressor := NewGzipCompressor()
	data := []byte(strings.Repeat("benchmark data ", 1000))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := compressor.Compress(data, 6)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
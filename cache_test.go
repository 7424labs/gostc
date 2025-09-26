package gostc

import (
	"testing"
	"time"
)

func TestLRUCacheBasic(t *testing.T) {
	cache, err := NewLRUCache(1024*1024, 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to create LRU cache: %v", err)
	}
	defer cache.Stop()

	key := CacheKey{Path: "/test.css", Compression: Gzip}
	entry := &CacheEntry{
		Data:        []byte("test data"),
		ContentType: "text/css",
		ETag:        "\"123456\"",
		Size:        9,
	}

	// Test Set and Get
	cache.Set(key, entry)
	retrieved, ok := cache.Get(key)
	if !ok {
		t.Error("Failed to retrieve cached entry")
	}
	if string(retrieved.Data) != "test data" {
		t.Errorf("Expected 'test data', got %s", string(retrieved.Data))
	}

	// Test cache stats
	stats := cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}

	// Test Delete
	cache.Delete(key)
	_, ok = cache.Get(key)
	if ok {
		t.Error("Entry should have been deleted")
	}
}

func TestLRUCacheSizeLimit(t *testing.T) {
	// Create a small cache (100 bytes)
	cache, err := NewLRUCache(100, 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to create LRU cache: %v", err)
	}
	defer cache.Stop()

	// Add entry that's within limit
	key1 := CacheKey{Path: "/small.txt", Compression: NoCompression}
	entry1 := &CacheEntry{
		Data: []byte("small"),
		Size: 5,
	}
	cache.Set(key1, entry1)

	// Add entry that exceeds limit (should not be cached)
	key2 := CacheKey{Path: "/large.txt", Compression: NoCompression}
	largeData := make([]byte, 200)
	entry2 := &CacheEntry{
		Data: largeData,
		Size: 200,
	}
	cache.Set(key2, entry2)

	// Small entry should be cached
	_, ok := cache.Get(key1)
	if !ok {
		t.Error("Small entry should be cached")
	}

	// Large entry should not be cached
	_, ok = cache.Get(key2)
	if ok {
		t.Error("Large entry should not be cached")
	}
}

func TestLRUCacheTTL(t *testing.T) {
	// Create cache with very short TTL
	cache, err := NewLRUCache(1024*1024, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create LRU cache: %v", err)
	}
	defer cache.Stop()

	key := CacheKey{Path: "/expire.txt", Compression: NoCompression}
	entry := &CacheEntry{
		Data:      []byte("will expire"),
		Size:      11,
		CreatedAt: time.Now(),
	}
	cache.Set(key, entry)

	// Should be available immediately
	_, ok := cache.Get(key)
	if !ok {
		t.Error("Entry should be available immediately")
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get(key)
	if ok {
		t.Error("Entry should have expired")
	}
}

func TestLFUCacheBasic(t *testing.T) {
	cache := NewLFUCache(1024*1024, 5*time.Minute)
	defer cache.Stop()

	key := CacheKey{Path: "/test.js", Compression: Brotli}
	entry := &CacheEntry{
		Data:        []byte("javascript code"),
		ContentType: "application/javascript",
		ETag:        "\"abcdef\"",
		Size:        15,
	}

	// Test Set and Get
	cache.Set(key, entry)
	retrieved, ok := cache.Get(key)
	if !ok {
		t.Error("Failed to retrieve cached entry")
	}
	if string(retrieved.Data) != "javascript code" {
		t.Errorf("Expected 'javascript code', got %s", string(retrieved.Data))
	}

	// Test frequency tracking
	cache.Get(key) // Second access
	cache.Get(key) // Third access

	stats := cache.Stats()
	if stats.Hits != 3 {
		t.Errorf("Expected 3 hits, got %d", stats.Hits)
	}

	// Test Clear
	cache.Clear()
	_, ok = cache.Get(key)
	if ok {
		t.Error("Cache should be cleared")
	}
}

func TestLFUCacheEviction(t *testing.T) {
	// Small cache to test eviction
	cache := NewLFUCache(50, 5*time.Minute)
	defer cache.Stop()

	// Add first entry
	key1 := CacheKey{Path: "/freq1.txt"}
	entry1 := &CacheEntry{Data: []byte("data1"), Size: 20}
	cache.Set(key1, entry1)

	// Access it multiple times to increase frequency
	cache.Get(key1)
	cache.Get(key1)

	// Add second entry
	key2 := CacheKey{Path: "/freq2.txt"}
	entry2 := &CacheEntry{Data: []byte("data2"), Size: 20}
	cache.Set(key2, entry2)

	// Add third entry that should cause eviction
	key3 := CacheKey{Path: "/freq3.txt"}
	entry3 := &CacheEntry{Data: []byte("data3"), Size: 20}
	cache.Set(key3, entry3)

	// First entry should still exist (higher frequency)
	_, ok := cache.Get(key1)
	if !ok {
		t.Error("High frequency entry should not be evicted")
	}

	// Second entry should be evicted (lowest frequency)
	_, ok = cache.Get(key2)
	if ok {
		t.Error("Low frequency entry should be evicted")
	}
}

func TestCacheKeyEquality(t *testing.T) {
	cache, err := NewLRUCache(1024*1024, 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Test that different compression types create different cache keys
	key1 := CacheKey{Path: "/same.css", Compression: Gzip}
	key2 := CacheKey{Path: "/same.css", Compression: Brotli}
	key3 := CacheKey{Path: "/same.css", Compression: Gzip, IsVersioned: true}

	entry := &CacheEntry{Data: []byte("data"), Size: 4}

	cache.Set(key1, entry)
	cache.Set(key2, entry)
	cache.Set(key3, entry)

	stats := cache.Stats()
	if stats.ItemCount != 3 {
		t.Errorf("Expected 3 different cache entries, got %d", stats.ItemCount)
	}
}

func TestCacheFactory(t *testing.T) {
	tests := []struct {
		name     string
		strategy CacheStrategy
		wantType string
	}{
		{"LRU Strategy", LRU, "*gostc.LRUCache"},
		{"LFU Strategy", LFU, "*gostc.LFUCache"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				CacheStrategy: tt.strategy,
				CacheSize:     1024 * 1024,
				CacheTTL:      5 * time.Minute,
			}

			cache, err := NewCache(config)
			if err != nil {
				t.Fatalf("Failed to create cache: %v", err)
			}

			// Check cache type
			switch tt.strategy {
			case LRU:
				if _, ok := cache.(*LRUCache); !ok {
					t.Errorf("Expected LRUCache, got %T", cache)
				}
			case LFU:
				if _, ok := cache.(*LFUCache); !ok {
					t.Errorf("Expected LFUCache, got %T", cache)
				}
			}
		})
	}
}

func BenchmarkLRUCacheGet(b *testing.B) {
	cache, _ := NewLRUCache(10*1024*1024, 5*time.Minute)
	defer cache.Stop()

	key := CacheKey{Path: "/bench.txt", Compression: NoCompression}
	entry := &CacheEntry{
		Data: []byte("benchmark data"),
		Size: 14,
	}
	cache.Set(key, entry)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Get(key)
		}
	})
}

func BenchmarkLFUCacheGet(b *testing.B) {
	cache := NewLFUCache(10*1024*1024, 5*time.Minute)
	defer cache.Stop()

	key := CacheKey{Path: "/bench.txt", Compression: NoCompression}
	entry := &CacheEntry{
		Data: []byte("benchmark data"),
		Size: 14,
	}
	cache.Set(key, entry)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Get(key)
		}
	})
}
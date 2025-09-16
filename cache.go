package gostc

import (
	"container/heap"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

type CacheKey struct {
	Path        string
	Compression CompressionType
}

type CacheEntry struct {
	Data         []byte
	ContentType  string
	ETag         string
	LastModified time.Time
	CreatedAt    time.Time
	AccessCount  int64
	Size         int64
}

type Cache interface {
	Get(key CacheKey) (*CacheEntry, bool)
	Set(key CacheKey, entry *CacheEntry)
	Delete(key CacheKey)
	Clear()
	Stats() CacheStats
}

type CacheStats struct {
	Hits       int64
	Misses     int64
	Evictions  int64
	Size       int64
	ItemCount  int
}

type LRUCache struct {
	cache    *lru.Cache[CacheKey, *CacheEntry]
	mu       sync.RWMutex
	stats    CacheStats
	maxSize  int64
	currentSize int64
	ttl      time.Duration
}

func NewLRUCache(maxSize int64, ttl time.Duration) (*LRUCache, error) {
	onEvicted := func(key CacheKey, value *CacheEntry) {
	}

	cache, err := lru.NewWithEvict[CacheKey, *CacheEntry](1000, onEvicted)
	if err != nil {
		return nil, err
	}

	lc := &LRUCache{
		cache:   cache,
		maxSize: maxSize,
		ttl:     ttl,
	}

	go lc.cleanupExpired()

	return lc, nil
}

func (c *LRUCache) Get(key CacheKey) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.cache.Get(key)
	if !ok {
		c.stats.Misses++
		return nil, false
	}

	if time.Since(entry.CreatedAt) > c.ttl {
		c.cache.Remove(key)
		c.currentSize -= entry.Size
		c.stats.Misses++
		return nil, false
	}

	entry.AccessCount++
	c.stats.Hits++
	return entry, true
}

func (c *LRUCache) Set(key CacheKey, entry *CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.currentSize + entry.Size > c.maxSize {
		c.evictToSize(c.maxSize - entry.Size)
	}

	if oldEntry, ok := c.cache.Get(key); ok {
		c.currentSize -= oldEntry.Size
	}

	entry.CreatedAt = time.Now()
	c.cache.Add(key, entry)
	c.currentSize += entry.Size
}

func (c *LRUCache) Delete(key CacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.cache.Get(key); ok {
		c.cache.Remove(key)
		c.currentSize -= entry.Size
	}
}

func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache.Purge()
	c.currentSize = 0
	c.stats = CacheStats{}
}

func (c *LRUCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.stats.Size = c.currentSize
	c.stats.ItemCount = c.cache.Len()
	return c.stats
}

func (c *LRUCache) evictToSize(targetSize int64) {
	for c.currentSize > targetSize && c.cache.Len() > 0 {
		c.cache.RemoveOldest()
		c.stats.Evictions++
	}
}

func (c *LRUCache) cleanupExpired() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		keys := c.cache.Keys()
		now := time.Now()

		for _, key := range keys {
			if entry, ok := c.cache.Peek(key); ok {
				if now.Sub(entry.CreatedAt) > c.ttl {
					c.cache.Remove(key)
					c.currentSize -= entry.Size
				}
			}
		}
		c.mu.Unlock()
	}
}

type LFUCache struct {
	items     map[CacheKey]*lfuEntry
	freqList  *minHeap
	mu        sync.RWMutex
	maxSize   int64
	currentSize int64
	ttl       time.Duration
	stats     CacheStats
}

type lfuEntry struct {
	key   CacheKey
	entry *CacheEntry
	freq  int
	index int
}

type minHeap []*lfuEntry

func (h minHeap) Len() int            { return len(h) }
func (h minHeap) Less(i, j int) bool  { return h[i].freq < h[j].freq }
func (h minHeap) Swap(i, j int)       {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *minHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*lfuEntry)
	item.index = n
	*h = append(*h, item)
}

func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0:n-1]
	return item
}

func NewLFUCache(maxSize int64, ttl time.Duration) *LFUCache {
	h := &minHeap{}
	heap.Init(h)

	cache := &LFUCache{
		items:    make(map[CacheKey]*lfuEntry),
		freqList: h,
		maxSize:  maxSize,
		ttl:      ttl,
	}

	go cache.cleanupExpired()
	return cache
}

func (c *LFUCache) Get(key CacheKey) (*CacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, ok := c.items[key]; ok {
		if time.Since(item.entry.CreatedAt) > c.ttl {
			c.removeItem(item)
			c.stats.Misses++
			return nil, false
		}

		item.freq++
		heap.Fix(c.freqList, item.index)
		c.stats.Hits++
		return item.entry, true
	}

	c.stats.Misses++
	return nil, false
}

func (c *LFUCache) Set(key CacheKey, entry *CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry.CreatedAt = time.Now()

	if existing, ok := c.items[key]; ok {
		c.currentSize -= existing.entry.Size
		existing.entry = entry
		existing.freq++
		heap.Fix(c.freqList, existing.index)
		c.currentSize += entry.Size
		return
	}

	for c.currentSize+entry.Size > c.maxSize && c.freqList.Len() > 0 {
		c.evictLFU()
	}

	item := &lfuEntry{
		key:   key,
		entry: entry,
		freq:  1,
	}

	heap.Push(c.freqList, item)
	c.items[key] = item
	c.currentSize += entry.Size
}

func (c *LFUCache) Delete(key CacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, ok := c.items[key]; ok {
		c.removeItem(item)
	}
}

func (c *LFUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[CacheKey]*lfuEntry)
	c.freqList = &minHeap{}
	heap.Init(c.freqList)
	c.currentSize = 0
	c.stats = CacheStats{}
}

func (c *LFUCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.stats.Size = c.currentSize
	c.stats.ItemCount = len(c.items)
	return c.stats
}

func (c *LFUCache) removeItem(item *lfuEntry) {
	heap.Remove(c.freqList, item.index)
	delete(c.items, item.key)
	c.currentSize -= item.entry.Size
}

func (c *LFUCache) evictLFU() {
	if c.freqList.Len() == 0 {
		return
	}

	item := heap.Pop(c.freqList).(*lfuEntry)
	delete(c.items, item.key)
	c.currentSize -= item.entry.Size
	c.stats.Evictions++
}

func (c *LFUCache) cleanupExpired() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()

		for key, item := range c.items {
			if now.Sub(item.entry.CreatedAt) > c.ttl {
				c.removeItem(item)
				delete(c.items, key)
			}
		}
		c.mu.Unlock()
	}
}

func NewCache(config *Config) (Cache, error) {
	switch config.CacheStrategy {
	case LFU:
		return NewLFUCache(config.CacheSize, config.CacheTTL), nil
	case LRU:
		fallthrough
	default:
		return NewLRUCache(config.CacheSize, config.CacheTTL)
	}
}
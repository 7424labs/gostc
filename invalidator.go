package gostc

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Invalidator interface {
	Start() error
	Stop() error
	InvalidatePath(path string)
	InvalidateAll()
}

type FileWatcher struct {
	watcher     *fsnotify.Watcher
	cache       Cache
	root        string
	mu          sync.RWMutex
	stopChan    chan struct{}
	compression *CompressionManager
}

func NewFileWatcher(root string, cache Cache, compression *CompressionManager) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fw := &FileWatcher{
		watcher:     watcher,
		cache:       cache,
		root:        root,
		stopChan:    make(chan struct{}),
		compression: compression,
	}

	return fw, nil
}

func (fw *FileWatcher) Start() error {
	if err := fw.watchDir(fw.root); err != nil {
		return err
	}

	go fw.watch()
	return nil
}

func (fw *FileWatcher) Stop() error {
	close(fw.stopChan)
	return fw.watcher.Close()
}

func (fw *FileWatcher) InvalidatePath(path string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	relPath, err := filepath.Rel(fw.root, path)
	if err != nil {
		return
	}

	fw.cache.Delete(CacheKey{Path: relPath, Compression: NoCompression})
	fw.cache.Delete(CacheKey{Path: relPath, Compression: Gzip})
	fw.cache.Delete(CacheKey{Path: relPath, Compression: Brotli})
}

func (fw *FileWatcher) InvalidateAll() {
	fw.cache.Clear()
}

func (fw *FileWatcher) watch() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Create == fsnotify.Create ||
				event.Op&fsnotify.Remove == fsnotify.Remove ||
				event.Op&fsnotify.Rename == fsnotify.Rename {

				fw.InvalidatePath(event.Name)

				if event.Op&fsnotify.Create == fsnotify.Create {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						fw.watchDir(event.Name)
					}
				}
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("File watcher error: %v", err)

		case <-fw.stopChan:
			return
		}
	}
}

func (fw *FileWatcher) watchDir(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return fw.watcher.Add(path)
		}

		return nil
	})
}

type TTLInvalidator struct {
	cache    Cache
	interval time.Duration
	stopChan chan struct{}
	mu       sync.RWMutex
}

func NewTTLInvalidator(cache Cache, interval time.Duration) *TTLInvalidator {
	return &TTLInvalidator{
		cache:    cache,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

func (ti *TTLInvalidator) Start() error {
	go ti.run()
	return nil
}

func (ti *TTLInvalidator) Stop() error {
	close(ti.stopChan)
	return nil
}

func (ti *TTLInvalidator) InvalidatePath(path string) {
	ti.cache.Delete(CacheKey{Path: path, Compression: NoCompression})
	ti.cache.Delete(CacheKey{Path: path, Compression: Gzip})
	ti.cache.Delete(CacheKey{Path: path, Compression: Brotli})
}

func (ti *TTLInvalidator) InvalidateAll() {
	ti.cache.Clear()
}

func (ti *TTLInvalidator) run() {
	ticker := time.NewTicker(ti.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:

		case <-ti.stopChan:
			return
		}
	}
}

type CompositeInvalidator struct {
	invalidators []Invalidator
	mu           sync.RWMutex
}

func NewCompositeInvalidator(invalidators ...Invalidator) *CompositeInvalidator {
	return &CompositeInvalidator{
		invalidators: invalidators,
	}
}

func (ci *CompositeInvalidator) Start() error {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	for _, inv := range ci.invalidators {
		if err := inv.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (ci *CompositeInvalidator) Stop() error {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	var firstErr error
	for _, inv := range ci.invalidators {
		if err := inv.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (ci *CompositeInvalidator) InvalidatePath(path string) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	for _, inv := range ci.invalidators {
		inv.InvalidatePath(path)
	}
}

func (ci *CompositeInvalidator) InvalidateAll() {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	for _, inv := range ci.invalidators {
		inv.InvalidateAll()
	}
}

func (ci *CompositeInvalidator) Add(invalidator Invalidator) {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	ci.invalidators = append(ci.invalidators, invalidator)
}

type ManualInvalidator struct {
	cache Cache
	mu    sync.RWMutex
}

func NewManualInvalidator(cache Cache) *ManualInvalidator {
	return &ManualInvalidator{
		cache: cache,
	}
}

func (mi *ManualInvalidator) Start() error {
	return nil
}

func (mi *ManualInvalidator) Stop() error {
	return nil
}

func (mi *ManualInvalidator) InvalidatePath(path string) {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	mi.cache.Delete(CacheKey{Path: path, Compression: NoCompression})
	mi.cache.Delete(CacheKey{Path: path, Compression: Gzip})
	mi.cache.Delete(CacheKey{Path: path, Compression: Brotli})
}

func (mi *ManualInvalidator) InvalidateAll() {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	mi.cache.Clear()
}
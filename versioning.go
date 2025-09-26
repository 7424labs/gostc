package gostc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

type AssetVersionManager struct {
	versionedPaths map[string]string // original -> versioned
	originalPaths  map[string]string // versioned -> original
	contentHashes  map[string]string // path -> hash
	mu             sync.RWMutex
	config         *Config
	hashLength     int
	urlPrefix      string // URL prefix for serving (e.g., "/static")
}

type HTMLProcessor struct {
	versionManager *AssetVersionManager
	linkPattern    *regexp.Regexp
	scriptPattern  *regexp.Regexp
}

func NewAssetVersionManager(config *Config) *AssetVersionManager {
	hashLength := 16
	if config.VersionHashLength > 0 {
		hashLength = config.VersionHashLength
	}

	return &AssetVersionManager{
		versionedPaths: make(map[string]string),
		originalPaths:  make(map[string]string),
		contentHashes:  make(map[string]string),
		config:         config,
		hashLength:     hashLength,
		urlPrefix:      config.URLPrefix,
	}
}

func NewHTMLProcessor(versionManager *AssetVersionManager) *HTMLProcessor {
	return &HTMLProcessor{
		versionManager: versionManager,
		linkPattern:    regexp.MustCompile(`(href|src)="([^"]*\.(css|js|mjs|png|jpg|jpeg|gif|svg|webp|ico|woff|woff2|ttf|otf))"[^>]*>`),
		scriptPattern:  regexp.MustCompile(`<script[^>]*src="([^"]*\.(?:js|mjs))"[^>]*>`),
	}
}

func (avm *AssetVersionManager) GenerateVersionedPath(originalPath string, content []byte) (string, string) {
	hash := sha256.Sum256(content)
	versionHash := hex.EncodeToString(hash[:avm.hashLength/2])

	ext := filepath.Ext(originalPath)
	base := strings.TrimSuffix(originalPath, ext)

	var versionedPath string
	if avm.config.VersioningPattern != "" {
		versionedPath = strings.ReplaceAll(avm.config.VersioningPattern, "{base}", base)
		versionedPath = strings.ReplaceAll(versionedPath, "{hash}", versionHash)
		versionedPath = strings.ReplaceAll(versionedPath, "{ext}", ext)
	} else {
		versionedPath = fmt.Sprintf("%s.%s%s", base, versionHash, ext)
	}

	return versionedPath, versionHash
}

func (avm *AssetVersionManager) RegisterAsset(originalPath string, content []byte) {
	avm.mu.Lock()
	defer avm.mu.Unlock()

	versionedPath, hash := avm.GenerateVersionedPath(originalPath, content)

	// If URL prefix is set, also register with prefixed paths for HTML matching
	if avm.urlPrefix != "" {
		prefixedOriginal := avm.urlPrefix + originalPath
		prefixedVersioned := avm.urlPrefix + versionedPath

		// Register both with and without prefix
		avm.versionedPaths[originalPath] = versionedPath
		avm.versionedPaths[prefixedOriginal] = prefixedVersioned
		avm.originalPaths[versionedPath] = originalPath
		avm.originalPaths[prefixedVersioned] = originalPath
		avm.contentHashes[originalPath] = hash
		avm.contentHashes[prefixedOriginal] = hash

		// Debug output
		fmt.Printf("  ✓ Registered: %s → %s (also as %s → %s)\n", originalPath, versionedPath, prefixedOriginal, prefixedVersioned)
	} else {
		avm.versionedPaths[originalPath] = versionedPath
		avm.originalPaths[versionedPath] = originalPath
		avm.contentHashes[originalPath] = hash

		// Debug output
		fmt.Printf("  ✓ Registered: %s → %s\n", originalPath, versionedPath)
	}
}

func (avm *AssetVersionManager) GetVersionedPath(originalPath string) (string, bool) {
	avm.mu.RLock()
	defer avm.mu.RUnlock()

	versionedPath, exists := avm.versionedPaths[originalPath]
	return versionedPath, exists
}

func (avm *AssetVersionManager) GetOriginalPath(versionedPath string) (string, bool) {
	avm.mu.RLock()
	defer avm.mu.RUnlock()

	originalPath, exists := avm.originalPaths[versionedPath]
	return originalPath, exists
}

func (avm *AssetVersionManager) GetContentHash(path string) (string, bool) {
	avm.mu.RLock()
	defer avm.mu.RUnlock()

	hash, exists := avm.contentHashes[path]
	return hash, exists
}

func (avm *AssetVersionManager) IsVersionedPath(path string) bool {
	avm.mu.RLock()
	defer avm.mu.RUnlock()

	_, exists := avm.originalPaths[path]
	return exists
}

func (avm *AssetVersionManager) RemoveAsset(originalPath string) {
	avm.mu.Lock()
	defer avm.mu.Unlock()

	if versionedPath, exists := avm.versionedPaths[originalPath]; exists {
		delete(avm.originalPaths, versionedPath)
	}

	delete(avm.versionedPaths, originalPath)
	delete(avm.contentHashes, originalPath)
}

func (avm *AssetVersionManager) ScanDirectory(rootPath string) error {
	if !avm.config.EnableVersioning {
		return nil
	}

	scannedCount := 0
	registeredCount := 0

	err := filepath.Walk(rootPath, func(fullPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relativePath := strings.TrimPrefix(fullPath, rootPath)
		relativePath = filepath.ToSlash(relativePath)
		if !strings.HasPrefix(relativePath, "/") {
			relativePath = "/" + relativePath
		}

		scannedCount++

		if !avm.shouldVersionFile(relativePath) {
			// Debug: show why file is not being versioned
			if strings.Contains(relativePath, ".css") || strings.Contains(relativePath, ".js") {
				fmt.Printf("  ⚠️ Skipping %s (not matching prefixes: %v)\n", relativePath, avm.config.StaticPrefixes)
			}
			return nil
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}

		avm.RegisterAsset(relativePath, content)
		registeredCount++
		return nil
	})

	if err == nil {
		fmt.Printf("📦 [Versioning] Scanned %d files, registered %d for versioning\n", scannedCount, registeredCount)
	}

	return err
}

func (avm *AssetVersionManager) shouldVersionFile(path string) bool {
	if len(avm.config.StaticPrefixes) == 0 {
		avm.config.StaticPrefixes = []string{"/static/", "/assets/", "/dist/", "/build/"}
	}

	for _, prefix := range avm.config.StaticPrefixes {
		if strings.HasPrefix(path, prefix) {
			return avm.isVersionableExtension(path)
		}
	}

	return false
}

func (avm *AssetVersionManager) isVersionableExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	versionableExts := []string{
		".css", ".js", ".mjs",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico",
		".woff", ".woff2", ".ttf", ".otf", ".eot",
	}

	for _, e := range versionableExts {
		if ext == e {
			return true
		}
	}
	return false
}

func (hp *HTMLProcessor) ProcessHTML(content []byte, basePath string) []byte {
	if hp.versionManager == nil || !hp.versionManager.config.EnableVersioning {
		return content
	}

	result := string(content)
	replacements := 0

	result = hp.linkPattern.ReplaceAllStringFunc(result, func(match string) string {
		processed := hp.processAssetReference(match)
		if processed != match {
			replacements++
		}
		return processed
	})

	if replacements > 0 {
		fmt.Printf("🔄 [HTML Processing] Transformed %d asset references in %s\n", replacements, basePath)
	}

	return []byte(result)
}

func (hp *HTMLProcessor) processAssetReference(match string) string {
	submatches := hp.linkPattern.FindStringSubmatch(match)
	if len(submatches) < 3 {
		return match
	}

	attributeName := submatches[1] // href or src
	originalURL := submatches[2]

	if versionedPath, exists := hp.versionManager.GetVersionedPath(originalURL); exists {
		fmt.Printf("    ➜ Replacing %s with %s\n", originalURL, versionedPath)
		return strings.Replace(match, fmt.Sprintf(`%s="%s"`, attributeName, originalURL), fmt.Sprintf(`%s="%s"`, attributeName, versionedPath), 1)
	} else {
		// Debug: show what we're looking for but not finding
		if strings.Contains(originalURL, ".css") || strings.Contains(originalURL, ".js") {
			fmt.Printf("    ⚠️ No versioned path for: %s\n", originalURL)
		}
	}

	return match
}


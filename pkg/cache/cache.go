package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/fabioroma/remarkable-cli/pkg/model"
)

// default TTL for cached metadata
const DefaultTTL = 5 * time.Minute

// Cache stores document metadata locally for fast repeated access
type Cache struct {
	dir string
}

// cached data on disk
type cacheData struct {
	Documents []model.Document `json:"documents"`
	Timestamp time.Time        `json:"timestamp"`
	Host      string           `json:"host"`
}

// New creates a cache at ~/.config/remarkable-cli/cache/
func New() *Cache {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "remarkable-cli", "cache")
	os.MkdirAll(dir, 0700)
	return &Cache{dir: dir}
}

func (c *Cache) path() string {
	return filepath.Join(c.dir, "documents.json")
}

// Get returns cached documents if fresh (within TTL)
func (c *Cache) Get(ttl time.Duration) ([]model.Document, bool) {
	data, err := os.ReadFile(c.path())
	if err != nil {
		return nil, false
	}

	var cached cacheData
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false
	}

	// check freshness
	if time.Since(cached.Timestamp) > ttl {
		return nil, false
	}

	return cached.Documents, true
}

// Set stores documents in cache
func (c *Cache) Set(docs []model.Document, host string) {
	cached := cacheData{
		Documents: docs,
		Timestamp: time.Now(),
		Host:      host,
	}
	data, _ := json.MarshalIndent(cached, "", "  ")
	os.WriteFile(c.path(), data, 0600)
}

// Invalidate clears the cache (call after mutating commands)
func (c *Cache) Invalidate() {
	os.Remove(c.path())
}

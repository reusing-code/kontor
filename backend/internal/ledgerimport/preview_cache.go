package ledgerimport

import (
	"fmt"
	"sync"
	"time"
)

const DefaultPreviewTTL = 15 * time.Minute

type PreviewCache struct {
	mu      sync.Mutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	result    *PreviewResult
	expiresAt time.Time
}

func NewPreviewCache(ttl time.Duration) *PreviewCache {
	if ttl <= 0 {
		ttl = DefaultPreviewTTL
	}
	return &PreviewCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

func (c *PreviewCache) Put(id string, result *PreviewResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	exp := time.Now().Add(c.ttl)
	result.ExpiresAt = exp
	c.entries[id] = &cacheEntry{result: result, expiresAt: exp}
}

func (c *PreviewCache) Get(id string) (*PreviewResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[id]
	if !ok {
		return nil, fmt.Errorf("preview %q not found", id)
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, id)
		return nil, fmt.Errorf("preview %q expired", id)
	}
	return entry.result, nil
}

func (c *PreviewCache) Delete(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, id)
}

func (c *PreviewCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for id, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, id)
		}
	}
}

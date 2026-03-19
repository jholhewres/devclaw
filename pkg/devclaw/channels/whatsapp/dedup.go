// Package whatsapp – dedup.go provides a message deduplication cache
// to filter out duplicate messages during WhatsApp reconnections.
package whatsapp

import (
	"sync"
	"time"
)

const (
	dedupMaxSize = 5000
	dedupTTL     = 20 * time.Minute
)

// messageDedupCache is a thread-safe cache that tracks recently seen message
// IDs to prevent duplicate processing. WhatsApp may redeliver messages
// during reconnection or network recovery.
type messageDedupCache struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

// newMessageDedupCache creates a new deduplication cache.
func newMessageDedupCache() *messageDedupCache {
	return &messageDedupCache{
		entries: make(map[string]time.Time, 256),
	}
}

// isDuplicate checks if a stanza ID has been seen recently.
// If it's new, the ID is recorded and false is returned.
// If it's a duplicate (already seen within TTL), true is returned.
// This method is safe for concurrent use.
func (c *messageDedupCache) isDuplicate(stanzaID string) bool {
	if stanzaID == "" {
		return false
	}

	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already seen and not expired.
	if seenAt, exists := c.entries[stanzaID]; exists {
		if now.Sub(seenAt) < dedupTTL {
			return true
		}
		// Entry expired, will be refreshed below.
	}

	// Evict if at capacity.
	if len(c.entries) >= dedupMaxSize {
		c.evict(now)
	}

	// Record this message.
	c.entries[stanzaID] = now
	return false
}

// evict removes expired entries. If still over capacity after removing
// expired entries, removes the entries closest to expiration (oldest first).
func (c *messageDedupCache) evict(now time.Time) {
	// First pass: remove expired entries.
	for id, seenAt := range c.entries {
		if now.Sub(seenAt) >= dedupTTL {
			delete(c.entries, id)
		}
	}

	// If still over capacity, remove the oldest 25% by timestamp.
	if len(c.entries) >= dedupMaxSize {
		toRemove := len(c.entries) / 4
		for removed := 0; removed < toRemove; removed++ {
			var oldestID string
			var oldestTime time.Time
			first := true
			for id, seenAt := range c.entries {
				if first || seenAt.Before(oldestTime) {
					oldestID = id
					oldestTime = seenAt
					first = false
				}
			}
			if oldestID == "" {
				break
			}
			delete(c.entries, oldestID)
		}
	}
}

// size returns the current number of entries (for testing).
func (c *messageDedupCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

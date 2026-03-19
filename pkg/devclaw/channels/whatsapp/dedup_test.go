package whatsapp

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMessageDedupCache_isDuplicate(t *testing.T) {
	t.Run("first occurrence returns false", func(t *testing.T) {
		c := newMessageDedupCache()
		if c.isDuplicate("msg-1") {
			t.Error("expected first occurrence to not be duplicate")
		}
	})

	t.Run("second occurrence returns true", func(t *testing.T) {
		c := newMessageDedupCache()
		c.isDuplicate("msg-1")
		if !c.isDuplicate("msg-1") {
			t.Error("expected second occurrence to be duplicate")
		}
	})

	t.Run("different IDs are not duplicates", func(t *testing.T) {
		c := newMessageDedupCache()
		c.isDuplicate("msg-1")
		if c.isDuplicate("msg-2") {
			t.Error("expected different ID to not be duplicate")
		}
	})

	t.Run("empty string is not tracked", func(t *testing.T) {
		c := newMessageDedupCache()
		if c.isDuplicate("") {
			t.Error("expected empty string to not be duplicate")
		}
		if c.isDuplicate("") {
			t.Error("expected empty string to always return false")
		}
	})
}

func TestMessageDedupCache_TTL(t *testing.T) {
	c := &messageDedupCache{
		entries: make(map[string]time.Time),
	}

	// Insert an entry that's already expired.
	c.entries["old-msg"] = time.Now().Add(-dedupTTL - time.Minute)

	// Should not be considered a duplicate (expired).
	if c.isDuplicate("old-msg") {
		t.Error("expected expired entry to not be duplicate")
	}

	// But should be re-recorded now.
	if !c.isDuplicate("old-msg") {
		t.Error("expected re-recorded entry to be duplicate")
	}
}

func TestMessageDedupCache_MaxSize(t *testing.T) {
	c := newMessageDedupCache()

	// Fill to max size.
	for i := 0; i < dedupMaxSize; i++ {
		c.isDuplicate(fmt.Sprintf("msg-%d", i))
	}

	if c.size() != dedupMaxSize {
		t.Errorf("expected size %d, got %d", dedupMaxSize, c.size())
	}

	// Adding one more should trigger eviction.
	c.isDuplicate("overflow-msg")

	if c.size() > dedupMaxSize {
		t.Errorf("expected size <= %d after eviction, got %d", dedupMaxSize, c.size())
	}
}

func TestMessageDedupCache_Concurrency(t *testing.T) {
	c := newMessageDedupCache()
	const goroutines = 50
	const messagesPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gID int) {
			defer wg.Done()
			for i := 0; i < messagesPerGoroutine; i++ {
				id := fmt.Sprintf("g%d-msg-%d", gID, i)
				c.isDuplicate(id)
			}
		}(g)
	}

	wg.Wait()

	// Should not panic and should have reasonable size.
	size := c.size()
	if size == 0 {
		t.Error("expected non-zero size after concurrent inserts")
	}
	if size > dedupMaxSize {
		t.Errorf("size %d exceeds max %d", size, dedupMaxSize)
	}
}

func TestMessageDedupCache_EvictionRemovesExpired(t *testing.T) {
	c := &messageDedupCache{
		entries: make(map[string]time.Time),
	}

	// Fill with expired entries.
	expiredTime := time.Now().Add(-dedupTTL - time.Minute)
	for i := 0; i < dedupMaxSize; i++ {
		c.entries[fmt.Sprintf("expired-%d", i)] = expiredTime
	}

	// Add a new entry — should trigger eviction of expired ones.
	c.isDuplicate("new-msg")

	// All expired entries should be removed.
	if c.size() > 1 {
		t.Errorf("expected most entries evicted, got size %d", c.size())
	}
}

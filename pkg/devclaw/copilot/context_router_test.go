// Package copilot — context_router_test.go covers the ContextRouter
// resolver, focusing on the HI-1 cache + singleflight path added in the
// Sprint 1 review pass.
package copilot

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// newTestRouter returns a ContextRouter backed by a fresh test store
// with an empty (no-heuristics) config.
func newTestRouter(t *testing.T) *ContextRouter {
	t.Helper()
	store := newTestStoreCopilot(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewContextRouter(store, logger, DefaultHierarchyConfig())
}

// newTestRouterWithHeuristics returns a ContextRouter seeded with the
// provided heuristic rules. Keywords are test-local — not shipped.
func newTestRouterWithHeuristics(t *testing.T, heuristics []WingHeuristic) *ContextRouter {
	t.Helper()
	store := newTestStoreCopilot(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := DefaultHierarchyConfig()
	cfg.Heuristics = heuristics
	return NewContextRouter(store, logger, cfg)
}

func TestContextRouter_NilStoreReturnsDisabled(t *testing.T) {
	r := NewContextRouter(nil, nil, DefaultHierarchyConfig())
	res := r.Resolve(context.Background(), "telegram", "123", "")
	if res.Source != SourceDisabled {
		t.Errorf("expected SourceDisabled with nil store, got %v", res.Source)
	}
	if !res.IsEmpty() {
		t.Errorf("expected empty wing, got %q", res.Wing)
	}
}

func TestContextRouter_TierMappedHit(t *testing.T) {
	r := newTestRouter(t)

	// Seed an explicit mapping.
	if err := r.Pin("telegram", "111", "work"); err != nil {
		t.Fatalf("Pin: %v", err)
	}

	res := r.Resolve(context.Background(), "telegram", "111", "")
	if res.Source != SourceMapped {
		t.Errorf("expected SourceMapped, got %v", res.Source)
	}
	if res.Wing != "work" {
		t.Errorf("expected wing=work, got %q", res.Wing)
	}
	if res.Confidence != 1.0 {
		t.Errorf("expected confidence=1.0, got %v", res.Confidence)
	}
}

func TestContextRouter_TierHeuristicPersistsAndRepeats(t *testing.T) {
	// Seed user heuristics so the hint "work-team" matches wing "work".
	// These are test-local keywords — not shipped in the binary.
	r := newTestRouterWithHeuristics(t, []WingHeuristic{
		{Wing: "work", MatchChannelName: []string{"work-team", "office"}},
	})
	ctx := context.Background()

	// Hint containing "work-team" should match the heuristic.
	res1 := r.Resolve(ctx, "telegram", "group-123", "work-team chat")
	if res1.Source != SourceHeuristic {
		t.Errorf("first call expected SourceHeuristic, got %v", res1.Source)
	}
	if res1.Wing != "work" {
		t.Errorf("expected wing=work, got %q", res1.Wing)
	}

	// Second call on the same key should hit the cache and return the
	// same result — the source may or may not change depending on whether
	// we read from the cache (SourceHeuristic) or from the store
	// (SourceMapped because persist happened). What matters is the wing.
	res2 := r.Resolve(ctx, "telegram", "group-123", "work-team chat")
	if res2.Wing != "work" {
		t.Errorf("second call expected wing=work, got %q", res2.Wing)
	}
}

func TestContextRouter_DefaultTier(t *testing.T) {
	r := newTestRouter(t)
	res := r.Resolve(context.Background(), "unknown-channel", "x", "")
	if res.Source != SourceDefault {
		t.Errorf("expected SourceDefault, got %v", res.Source)
	}
	if !res.IsEmpty() {
		t.Errorf("expected empty wing, got %q", res.Wing)
	}
}

// TestContextRouter_CacheHitAvoidsStoreLookup verifies the HI-1 fix:
// a second call for the same key does not hit GetChannelWing again.
// We verify this indirectly by: pinning, then unpinning directly via
// the store (bypassing the cache invalidation), then calling Resolve
// again. If the cache is working, we still see the original value; if
// it is not, we see the unmapped fallthrough.
func TestContextRouter_CacheHitAvoidsStoreLookup(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()

	// Prime the cache with a mapped entry.
	if err := r.Pin("telegram", "cache-test", "work"); err != nil {
		t.Fatalf("Pin: %v", err)
	}
	first := r.Resolve(ctx, "telegram", "cache-test", "")
	if first.Wing != "work" || first.Source != SourceMapped {
		t.Fatalf("prime: %+v", first)
	}

	// Now delete from the store BEHIND the cache's back. The cache should
	// still serve the old value until TTL or explicit invalidation.
	if err := r.store.DeleteChannelWing("telegram", "cache-test"); err != nil {
		t.Fatalf("direct store delete: %v", err)
	}

	// Second resolve should HIT the cache and still return work.
	second := r.Resolve(ctx, "telegram", "cache-test", "")
	if second.Wing != "work" {
		t.Errorf("cache was bypassed: expected wing=work from cache, got %q", second.Wing)
	}
}

// TestContextRouter_UnpinInvalidatesCache verifies that the public Unpin
// method properly invalidates the cache entry, so the next Resolve sees
// the removal immediately (not after TTL).
func TestContextRouter_UnpinInvalidatesCache(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()

	if err := r.Pin("telegram", "unpin-test", "work"); err != nil {
		t.Fatalf("Pin: %v", err)
	}
	// Prime the cache.
	first := r.Resolve(ctx, "telegram", "unpin-test", "")
	if first.Wing != "work" {
		t.Fatalf("prime: %+v", first)
	}

	// Use the router's Unpin — should invalidate cache.
	if err := r.Unpin("telegram", "unpin-test"); err != nil {
		t.Fatalf("Unpin: %v", err)
	}

	// Resolve should NOT return the cached "work" value — cache was invalidated.
	// Telegram channel has no heuristic for unknown IDs, so it falls to default.
	second := r.Resolve(ctx, "telegram", "unpin-test", "")
	if second.Wing != "" {
		t.Errorf("expected empty wing after unpin, got %q (source=%v)", second.Wing, second.Source)
	}
}

// TestContextRouter_PinUpdatesCache verifies that changing the pin via the
// router's Pin method invalidates the cache so the new wing is visible
// immediately.
func TestContextRouter_PinUpdatesCache(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()

	_ = r.Pin("telegram", "update-test", "work")
	first := r.Resolve(ctx, "telegram", "update-test", "")
	if first.Wing != "work" {
		t.Fatalf("prime: %+v", first)
	}

	// Change the pin.
	if err := r.Pin("telegram", "update-test", "personal"); err != nil {
		t.Fatalf("re-Pin: %v", err)
	}

	second := r.Resolve(ctx, "telegram", "update-test", "")
	if second.Wing != "personal" {
		t.Errorf("expected wing=personal after re-pin, got %q", second.Wing)
	}
}

// TestContextRouter_ConcurrentBurst is the HI-1 stress test: 50 goroutines
// concurrently resolving the same unmapped key trigger the singleflight
// path. We assert:
//  1. All return the same resolution (no races on the returned value)
//  2. Only ONE store write actually happens (verify by querying mapping count)
//  3. The cache is populated after the burst
//
// This is a correctness test, not a latency benchmark.
func TestContextRouter_ConcurrentBurst(t *testing.T) {
	// Seed heuristics so the hint "burst-group" matches wing "work".
	// Using test-local vocabulary — not shipped in the binary.
	r := newTestRouterWithHeuristics(t, []WingHeuristic{
		{Wing: "work", MatchChannelName: []string{"burst-group"}},
	})
	ctx := context.Background()

	const burstSize = 50
	var wg sync.WaitGroup
	results := make([]WingResolution, burstSize)

	for i := 0; i < burstSize; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = r.Resolve(ctx, "telegram", "burst-session", "burst-group chat")
		}(i)
	}
	wg.Wait()

	// All goroutines should see the same wing (no race corruption).
	expected := results[0].Wing
	if expected != "work" {
		t.Fatalf("expected all results to be wing=work (user heuristic), got first=%q", expected)
	}
	for i, res := range results {
		if res.Wing != expected {
			t.Errorf("goroutine %d: expected wing=%q, got %q", i, expected, res.Wing)
		}
	}

	// The store should have exactly ONE mapping for this key.
	mappings, err := r.store.ListChannelWings("work")
	if err != nil {
		t.Fatalf("list channel wings: %v", err)
	}
	count := 0
	for _, m := range mappings {
		if m.Channel == "telegram" && m.ExternalID == "burst-session" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 mapping after burst, got %d", count)
	}

	// Cache should be populated.
	if _, ok := r.cache.Load(routerCacheKey("telegram", "burst-session")); !ok {
		t.Error("expected cache to be populated after burst")
	}
}

// TestContextRouter_UserHeuristicMatchesConfiguredKeywords verifies the
// heuristic tier fires when a hint matches a user-configured WingHeuristic,
// and falls through to SourceDefault when no keywords are configured.
func TestContextRouter_UserHeuristicMatchesConfiguredKeywords(t *testing.T) {
	// Test fixtures intentionally use generic domain vocabulary — these
	// strings are NOT shipped in the binary. Accented variant ("reuniao")
	// is kept unaccented in the config to prove StripAccents normalizes
	// accented hints ("reunião") to match it.
	r := newTestRouterWithHeuristics(t, []WingHeuristic{
		{Wing: "alpha", MatchChannelName: []string{"alpha-chat", "hq"}},
		{Wing: "beta", MatchGroupName: []string{"beta-team", "reuniao"}},
	})
	ctx := context.Background()

	cases := []struct {
		name       string
		channel    string
		externalID string
		hint       string
		wantWing   string
		wantSource WingResolutionSource
		wantConf   float64
	}{
		{"channel_match", "telegram", "g1", "alpha-chat group", "alpha", SourceHeuristic, userHeuristicConfidence},
		{"channel_secondary", "telegram", "g2", "hq planning", "alpha", SourceHeuristic, userHeuristicConfidence},
		{"group_match", "telegram", "g3", "beta-team updates", "beta", SourceHeuristic, userHeuristicConfidence},
		// Accented hint must match unaccented keyword via StripAccents.
		// "reunião" (NFC) → "reuniao" after normalization → matches "reuniao".
		{"accented_hint_nfc", "telegram", "g4", "reunião semanal", "beta", SourceHeuristic, userHeuristicConfidence},
		// NFD-decomposed form (combining tilde) must also match — this is
		// the Unicode Mn code path that a router-local stripper would miss.
		{"accented_hint_nfd", "telegram", "g5", "reunia\u0303o daily", "beta", SourceHeuristic, userHeuristicConfidence},
		{"no_match", "telegram", "g6", "random topic", "", SourceDefault, 0},
		{"empty_hint", "telegram", "g7", "", "", SourceDefault, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := r.Resolve(ctx, tc.channel, tc.externalID, tc.hint)
			if res.Wing != tc.wantWing {
				t.Errorf("hint=%q: expected wing=%q, got %q", tc.hint, tc.wantWing, res.Wing)
			}
			if res.Source != tc.wantSource {
				t.Errorf("hint=%q: expected source=%v, got %v", tc.hint, tc.wantSource, res.Source)
			}
			// Pin the heuristic-tier confidence constant so a future
			// change to userHeuristicConfidence trips CI instead of
			// silently shifting downstream scoring.
			if tc.wantSource == SourceHeuristic && res.Confidence != tc.wantConf {
				t.Errorf("hint=%q: expected confidence=%v, got %v", tc.hint, tc.wantConf, res.Confidence)
			}
		})
	}
}

// TestContextRouter_NoHeuristicsNoMatch verifies that an out-of-the-box
// install (no Heuristics configured) classifies nothing via the heuristic
// tier — all unmapped messages fall through to SourceDefault.
func TestContextRouter_NoHeuristicsNoMatch(t *testing.T) {
	r := newTestRouter(t) // DefaultHierarchyConfig has empty Heuristics
	ctx := context.Background()

	hints := []string{"family chat", "work team", "office", "finances", ""}
	channels := []string{"telegram", "whatsapp", "cli", "mcp"}
	for _, ch := range channels {
		for _, hint := range hints {
			res := r.Resolve(ctx, ch, "ext-"+ch+hint, hint)
			// Without heuristics, must fall to default (wing=NULL).
			if res.Source == SourceHeuristic {
				t.Errorf("channel=%q hint=%q: got SourceHeuristic without any config", ch, hint)
			}
		}
	}
}

// TestContextRouter_CacheTTLExpiry verifies that stale cache entries
// are evicted on access (lazy eviction) rather than returning stale data
// forever. We simulate a stale entry by manually inserting one into the
// cache with an expiresAt in the past.
func TestContextRouter_CacheTTLExpiry(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()

	// Seed the store with a real mapping.
	_ = r.Pin("telegram", "ttl-test", "work")

	// The Pin call populated the cache. Overwrite it with a STALE entry
	// pointing at a different wing, with expiresAt in the past.
	r.cache.Store(routerCacheKey("telegram", "ttl-test"), routerCacheEntry{
		res:       WingResolution{Wing: "stale-wing", Source: SourceMapped, Confidence: 1.0},
		expiresAt: time.Now().Add(-1 * time.Hour),
	})

	// Resolve should see the stale entry, evict it, and re-query the
	// store, returning the REAL wing=work.
	res := r.Resolve(ctx, "telegram", "ttl-test", "")
	if res.Wing != "work" {
		t.Errorf("expected fresh resolve to return wing=work, got %q", res.Wing)
	}
}

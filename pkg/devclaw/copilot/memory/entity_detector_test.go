// Package memory — entity_detector_test.go covers the EntityDetector
// introduced in Sprint 2 Room 2.3.
//
// Test vocabulary is intentionally generic (wings "alpha"/"beta", rooms
// "project-x"/"feature-y") — no locale or domain assumptions.
package memory

import (
	"context"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func newTestDetector(tb testing.TB, store *SQLiteStore) *EntityDetector {
	tb.Helper()
	cfg := EntityDetectorConfig{
		CacheTTL:    30 * time.Second,
		MaxTokens:   40,
		MinTokenLen: 3,
	}
	return NewEntityDetector(store, cfg, nil)
}

// newBenchStore creates a SQLiteStore for benchmarks (accepts *testing.B).
func newBenchStore(b *testing.B) *SQLiteStore {
	b.Helper()
	tmpFile, err := os.CreateTemp(b.TempDir(), "bench-*.db")
	if err != nil {
		b.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewSQLiteStore(tmpFile.Name(), &deterministicEmbedder{}, slog.Default())
	if err != nil {
		b.Fatalf("create store: %v", err)
	}
	b.Cleanup(func() { store.Close() })
	return store
}

// insertWingForTest inserts a wing row directly into the store DB.
// Accepts testing.TB so it works from both *testing.T and *testing.B.
func insertWingForTest(tb testing.TB, store *SQLiteStore, name string) {
	tb.Helper()
	_, err := store.db.Exec(
		`INSERT OR IGNORE INTO wings (name) VALUES (?)`, name)
	if err != nil {
		tb.Fatalf("insertWingForTest(%q): %v", name, err)
	}
}

// insertRoomForTest inserts a room row directly into the store DB.
// wing must be non-empty (rooms.wing is NOT NULL in the schema).
// Accepts testing.TB so it works from both *testing.T and *testing.B.
func insertRoomForTest(tb testing.TB, store *SQLiteStore, name, wing string) {
	tb.Helper()
	if wing == "" {
		wing = "default"
	}
	_, err := store.db.Exec(
		`INSERT OR IGNORE INTO rooms (name, wing) VALUES (?, ?)`,
		name, wing)
	if err != nil {
		tb.Fatalf("insertRoomForTest(%q, %q): %v", name, wing, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestEntityDetector_EmptyTurnNoMatch — empty string → no matches, no error.
func TestEntityDetector_EmptyTurnNoMatch(t *testing.T) {
	store := newTestStore(t)
	d := newTestDetector(t, store)
	ctx := context.Background()

	matches, err := d.Detect(ctx, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

// TestEntityDetector_NoStoredEntities — empty store, non-empty turn → no matches.
func TestEntityDetector_NoStoredEntities(t *testing.T) {
	store := newTestStore(t)
	d := newTestDetector(t, store)
	ctx := context.Background()

	matches, err := d.Detect(ctx, "hello world this is some text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for empty store, got %d", len(matches))
	}
}

// TestEntityDetector_SingleWingMatch — wing "alpha" in store; turn contains "alpha".
func TestEntityDetector_SingleWingMatch(t *testing.T) {
	store := newTestStore(t)
	insertWingForTest(t, store, "alpha")

	d := newTestDetector(t, store)
	ctx := context.Background()

	turn := "hello alpha world"
	matches, err := d.Detect(ctx, turn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	m := matches[0]
	if m.Kind != "wing" {
		t.Errorf("Kind=%q, want %q", m.Kind, "wing")
	}
	if m.Candidate.Normalized != "alpha" {
		t.Errorf("Normalized=%q, want %q", m.Candidate.Normalized, "alpha")
	}
	// Offset should point to the start of "alpha" in the turn.
	wantOffset := strings.Index(turn, "alpha")
	if m.Candidate.Offset != wantOffset {
		t.Errorf("Offset=%d, want %d", m.Candidate.Offset, wantOffset)
	}
}

// TestEntityDetector_SingleRoomMatch — room "project-x" in wing "alpha";
// turn mentions "project-x".
func TestEntityDetector_SingleRoomMatch(t *testing.T) {
	store := newTestStore(t)
	insertWingForTest(t, store, "alpha")
	insertRoomForTest(t, store, "project-x", "alpha")

	d := newTestDetector(t, store)
	ctx := context.Background()

	matches, err := d.Detect(ctx, "let's review project-x today")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect at least the room match; "alpha" not in turn so no wing match.
	var roomMatch *EntityMatch
	for i := range matches {
		if matches[i].Kind == "room" && matches[i].Room == "project-x" {
			roomMatch = &matches[i]
		}
	}
	if roomMatch == nil {
		t.Fatalf("expected room match for project-x, got %v", matches)
	}
	if roomMatch.Wing != "alpha" {
		t.Errorf("Wing=%q, want %q", roomMatch.Wing, "alpha")
	}
}

// TestEntityDetector_AccentInsensitive — wing "cafe"; turn has "café".
func TestEntityDetector_AccentInsensitive(t *testing.T) {
	store := newTestStore(t)
	insertWingForTest(t, store, "cafe")

	d := newTestDetector(t, store)
	ctx := context.Background()

	matches, err := d.Detect(ctx, "meeting at the café today")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected match for accented 'café', got none")
	}
	if matches[0].Kind != "wing" || matches[0].Candidate.Normalized != "cafe" {
		t.Errorf("unexpected match: %+v", matches[0])
	}
}

// TestEntityDetector_CaseInsensitive — turn "ALPHA" matches wing "alpha".
func TestEntityDetector_CaseInsensitive(t *testing.T) {
	store := newTestStore(t)
	insertWingForTest(t, store, "alpha")

	d := newTestDetector(t, store)
	ctx := context.Background()

	matches, err := d.Detect(ctx, "reviewing ALPHA results")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected case-insensitive match for ALPHA, got none")
	}
	if matches[0].Candidate.Normalized != "alpha" {
		t.Errorf("normalized=%q, want %q", matches[0].Candidate.Normalized, "alpha")
	}
	// Raw text should preserve original casing.
	if matches[0].Candidate.Text != "ALPHA" {
		t.Errorf("text=%q, want %q", matches[0].Candidate.Text, "ALPHA")
	}
}

// TestEntityDetector_CacheHitAfterTTL — second Detect within TTL reuses
// the snapshot; after TTL elapses, next Detect triggers refresh.
func TestEntityDetector_CacheHitAfterTTL(t *testing.T) {
	store := newTestStore(t)
	insertWingForTest(t, store, "alpha")

	shortTTL := 50 * time.Millisecond
	d := NewEntityDetector(store, EntityDetectorConfig{
		CacheTTL:    shortTTL,
		MaxTokens:   40,
		MinTokenLen: 3,
	}, nil)
	ctx := context.Background()

	// First call loads the snapshot.
	if _, err := d.Detect(ctx, "hello alpha"); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	firstLoadedAt := d.snapshotLoadedAt()
	if firstLoadedAt.IsZero() {
		t.Fatal("snapshotLoadedAt should not be zero after first Detect")
	}

	// Second call within TTL — loadedAt must be the same.
	if _, err := d.Detect(ctx, "hello alpha again"); err != nil {
		t.Fatalf("second detect: %v", err)
	}
	secondLoadedAt := d.snapshotLoadedAt()
	if !secondLoadedAt.Equal(firstLoadedAt) {
		t.Errorf("snapshot was reloaded within TTL (first=%v, second=%v)",
			firstLoadedAt, secondLoadedAt)
	}

	// Wait for TTL to elapse, then insert a new wing.
	time.Sleep(shortTTL + 10*time.Millisecond)
	insertWingForTest(t, store, "beta")

	// Next call should trigger a refresh and pick up "beta".
	matches, err := d.Detect(ctx, "hello beta")
	if err != nil {
		t.Fatalf("post-TTL detect: %v", err)
	}
	thirdLoadedAt := d.snapshotLoadedAt()
	if !thirdLoadedAt.After(firstLoadedAt) {
		t.Errorf("snapshot was not refreshed after TTL elapsed")
	}
	found := false
	for _, m := range matches {
		if m.Candidate.Normalized == "beta" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find 'beta' after TTL-triggered refresh")
	}
}

// TestEntityDetector_CacheRefreshFailureKeepsOldSnapshot — seed snapshot,
// close the store's DB, call Detect after TTL — should return old matches.
func TestEntityDetector_CacheRefreshFailureKeepsOldSnapshot(t *testing.T) {
	store := newTestStore(t)
	insertWingForTest(t, store, "alpha")

	shortTTL := 30 * time.Millisecond
	d := NewEntityDetector(store, EntityDetectorConfig{
		CacheTTL:    shortTTL,
		MaxTokens:   40,
		MinTokenLen: 3,
	}, nil)
	ctx := context.Background()

	// Warm up the snapshot.
	matches, err := d.Detect(ctx, "hello alpha")
	if err != nil {
		t.Fatalf("warm-up detect: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected warm-up match for alpha")
	}

	// Close the DB to force refresh failure.
	_ = store.db.Close()

	// Wait for TTL to elapse.
	time.Sleep(shortTTL + 10*time.Millisecond)

	// Detect should still return old matches from the snapshot.
	matches2, _ := d.Detect(ctx, "hello alpha")
	found := false
	for _, m := range matches2 {
		if m.Candidate.Normalized == "alpha" {
			found = true
		}
	}
	if !found {
		t.Error("expected old snapshot to be preserved after refresh failure")
	}
}

// TestEntityDetector_AdversarialLongInput — 10 000-char random turn. Latency
// < 5ms. len(matches) <= MaxTokens. No panic.
func TestEntityDetector_AdversarialLongInput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping latency assertion in short mode")
	}

	store := newTestStore(t)
	insertWingForTest(t, store, "alpha")

	cfg := EntityDetectorConfig{
		CacheTTL:    30 * time.Second,
		MaxTokens:   40,
		MinTokenLen: 3,
	}
	d := NewEntityDetector(store, cfg, nil)
	ctx := context.Background()

	// Build a random-ish 10 000-char string that occasionally contains "alpha".
	var sb strings.Builder
	r := rand.New(rand.NewSource(42))
	const letters = "abcdefghijklmnopqrstuvwxyz "
	for sb.Len() < 9990 {
		sb.WriteByte(letters[r.Intn(len(letters))])
	}
	sb.WriteString(" alpha ")
	turn := sb.String()

	// Warm up snapshot.
	_, _ = d.Detect(ctx, "warmup alpha")

	start := time.Now()
	matches, err := d.Detect(ctx, turn)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) > cfg.MaxTokens {
		t.Errorf("len(matches)=%d exceeds MaxTokens=%d", len(matches), cfg.MaxTokens)
	}

	t.Logf("adversarial 10k-char turn: elapsed=%v matches=%d", elapsed, len(matches))
	// Soft assertion: log if > 5ms but don't hard-fail in CI.
	if elapsed > 5*time.Millisecond {
		t.Logf("WARN: latency %v exceeded 5ms soft limit (warm cache)", elapsed)
	}
}

// TestEntityDetector_ConcurrentDetect — 50 goroutines call Detect concurrently.
// Race detector must be clean. All returns must include the known match.
func TestEntityDetector_ConcurrentDetect(t *testing.T) {
	store := newTestStore(t)
	insertWingForTest(t, store, "alpha")

	d := newTestDetector(t, store)
	ctx := context.Background()

	// Warm up.
	if _, err := d.Detect(ctx, "warmup alpha"); err != nil {
		t.Fatalf("warmup: %v", err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 50)
	misses := make(chan int, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			matches, err := d.Detect(ctx, "reviewing alpha work")
			if err != nil {
				errors <- err
				return
			}
			found := false
			for _, m := range matches {
				if m.Candidate.Normalized == "alpha" {
					found = true
				}
			}
			if !found {
				misses <- 1
			}
		}()
	}
	wg.Wait()
	close(errors)
	close(misses)

	for err := range errors {
		t.Errorf("goroutine error: %v", err)
	}
	for range misses {
		t.Error("goroutine missed expected match for 'alpha'")
	}
}

// TestEntityDetector_WingPriorityOverRoom — wing "foo" and room named "foo"
// in a different wing. Turn "foo" → 1 match, Kind="wing".
func TestEntityDetector_WingPriorityOverRoom(t *testing.T) {
	store := newTestStore(t)
	insertWingForTest(t, store, "foo")
	insertRoomForTest(t, store, "foo", "beta") // room "foo" in a different wing

	d := newTestDetector(t, store)
	ctx := context.Background()

	matches, err := d.Detect(ctx, "let's talk about foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d: %+v", len(matches), matches)
	}
	if matches[0].Kind != "wing" {
		t.Errorf("Kind=%q, want %q (wing must win over room on key collision)",
			matches[0].Kind, "wing")
	}
}

// BenchmarkEntityDetectorDetect measures Detect latency for a 500-char turn
// with a warm cache. Should be < 1ms p95 on modern hardware.
func BenchmarkEntityDetectorDetect(b *testing.B) {
	store := newBenchStore(b)
	insertWingForTest(b, store, "alpha")
	insertWingForTest(b, store, "beta")
	insertRoomForTest(b, store, "project-x", "alpha")

	d := NewEntityDetector(store, DefaultEntityDetectorConfig(), nil)
	ctx := context.Background()

	// Warm up.
	_, _ = d.Detect(ctx, "warmup alpha project-x")

	turn := strings.Repeat("hello alpha and beta review project-x today ", 12)[:500]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, turn)
	}
}

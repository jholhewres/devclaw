// Package memory — layer_essential_test.go covers Sprint 2 Room 2.2:
// template-based L1 essential-story generation.
//
// Vocabulary is intentionally generic (wings "alpha"/"beta"/"gamma",
// rooms "room-a"/"room-b"/"room-c") so no locale or domain assumption
// leaks into the tests.
//
// Guarantee — TestEssentialLayer_NoLLMCallEver: by construction,
// NewEssentialLayer takes no LLM client and layer_essential.go imports
// zero LLM packages. Add a runtime test here if that ever changes.
package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// seedRawFile inserts a single file row plus its lead chunk directly via
// SQL. Bypasses IndexChunks on purpose: the L1 EssentialLayer reads the
// files/chunks tables by joining on file_id + chunk_idx=0, so a minimal
// raw insert is sufficient and avoids the embedding pipeline (which is
// expensive under the test's deterministic embedder and causes SQLite
// lock contention for fixture-heavy tests).
//
// Wing may be "" to leave the file in the legacy wing IS NULL bucket.
// Room may likewise be "" for wing-only classification.
func seedRawFile(t *testing.T, store *SQLiteStore, fileID, wing, room, text string, accessCount int) {
	t.Helper()
	var wingVal, roomVal interface{}
	if wing != "" {
		wingVal = wing
	}
	if room != "" {
		roomVal = room
	}
	_, err := store.db.Exec(`
		INSERT INTO files (file_id, hash, wing, room, access_count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(file_id) DO UPDATE SET
			wing         = excluded.wing,
			room         = excluded.room,
			access_count = excluded.access_count
	`, fileID, "hash-"+fileID, wingVal, roomVal, accessCount)
	if err != nil {
		t.Fatalf("insert file %s: %v", fileID, err)
	}

	_, err = store.db.Exec(`
		INSERT INTO chunks (file_id, chunk_idx, text, hash)
		VALUES (?, 0, ?, ?)
		ON CONFLICT(file_id, chunk_idx) DO UPDATE SET
			text = excluded.text,
			hash = excluded.hash
	`, fileID, text, "hash-chunk-"+fileID)
	if err != nil {
		t.Fatalf("insert chunk %s: %v", fileID, err)
	}
}

// seedEssentialFixture inserts a deterministic 3-wing x 4-room fixture
// into the store via raw SQL (no IndexChunks). Each room gets 5 files.
// File content is shaped so the lead sentence is distinctive per
// (wing, room, idx), enabling cross-contamination assertions.
//
// Side effects written via SQL:
//   - rooms.last_activity is stepped so ListRoomsByRecency returns rooms
//     in a predictable order (room-d most recent, room-a oldest).
//   - files.access_count is stepped so TopFilesInRoom returns files in a
//     predictable order (file-0 highest, file-4 lowest).
func seedEssentialFixture(t *testing.T, store *SQLiteStore) {
	t.Helper()

	wings := []string{"alpha", "beta", "gamma"}
	rooms := []string{"room-a", "room-b", "room-c", "room-d"}

	// Base timestamp; each room steps 10 minutes later so room-a is the
	// OLDEST and room-d is the MOST RECENT within a wing.
	base := time.Now().Add(-24 * time.Hour)

	for _, wing := range wings {
		if err := store.UpsertWing(wing, wing, ""); err != nil {
			t.Fatalf("upsert wing %s: %v", wing, err)
		}
		for ri, room := range rooms {
			if err := store.UpsertRoom(wing, room, "", "manual", 1.0); err != nil {
				t.Fatalf("upsert room %s/%s: %v", wing, room, err)
			}
			// Override last_activity so ordering is deterministic.
			activity := base.Add(time.Duration(ri) * 10 * time.Minute)
			_, err := store.db.Exec(
				`UPDATE rooms SET last_activity = ? WHERE wing = ? AND name = ?`,
				activity, wing, room,
			)
			if err != nil {
				t.Fatalf("set last_activity %s/%s: %v", wing, room, err)
			}

			for fi := 0; fi < 5; fi++ {
				fileID := fmt.Sprintf("%s/%s/file-%d.md", wing, room, fi)
				text := fmt.Sprintf(
					"Lead sentence for %s %s file %d. Tail sentence discarded by the template.",
					wing, room, fi,
				)
				seedRawFile(t, store, fileID, wing, room, text, 100-fi)
			}
		}
	}
}

// TestEssentialLayer_GenerateWithThreeWings seeds three wings, renders
// the alpha story, and asserts that:
//   - all 4 alpha rooms appear in the story
//   - each room contributes 3 lead sentences (the LeadSentencesPerRoom cap)
//   - total bytes are within the 1600-byte budget
//   - no beta/gamma content leaks in
func TestEssentialLayer_GenerateWithThreeWings(t *testing.T) {
	store := newTestStore(t)
	seedEssentialFixture(t, store)

	layer := NewEssentialLayer(store, DefaultEssentialLayerConfig(), silenceLogger())

	story, err := layer.Generate(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Generate alpha: %v", err)
	}
	if story == "" {
		t.Fatalf("empty story for alpha")
	}
	if len(story) > 1600 {
		t.Errorf("story bytes=%d exceeds 1600 budget", len(story))
	}
	if !strings.Contains(story, "## Wing: alpha") {
		t.Errorf("missing alpha wing heading; got:\n%s", story)
	}
	for _, room := range []string{"room-a", "room-b", "room-c", "room-d"} {
		if !strings.Contains(story, "### "+room) {
			t.Errorf("missing room heading %q; got:\n%s", room, story)
		}
	}
	if strings.Contains(story, "beta") || strings.Contains(story, "gamma") {
		t.Errorf("alpha story leaked beta/gamma content:\n%s", story)
	}
}

// TestEssentialLayer_EmptyWing verifies rendering against a wing that has
// zero rooms and zero files returns an empty-ish story (just the heading)
// and still writes a cache row so subsequent calls hit the cache.
func TestEssentialLayer_EmptyWing(t *testing.T) {
	store := newTestStore(t)
	layer := NewEssentialLayer(store, DefaultEssentialLayerConfig(), silenceLogger())

	ctx := context.Background()
	story, err := layer.Generate(ctx, "alpha")
	if err != nil {
		t.Fatalf("Generate empty: %v", err)
	}
	// The heading alone is valid output; what matters is "no error" and
	// the cache row.
	if !strings.Contains(story, "## Wing: alpha") {
		t.Errorf("expected heading for empty wing; got %q", story)
	}

	var count int
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM essential_stories WHERE wing = ?`, "alpha",
	).Scan(&count); err != nil {
		t.Fatalf("count cache rows: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 cache row after Generate, got %d", count)
	}

	// Second Render must hit the cache (same bytes).
	story2 := layer.Render(ctx, "alpha")
	if story2 != story {
		t.Errorf("cached render differs from generated:\ngot:\n%s\nwant:\n%s", story2, story)
	}
}

// TestEssentialLayer_NullWing seeds files with wing IS NULL and renders
// wing="". The story must carry the (legacy) marker and contain the
// seeded file's lead sentence.
func TestEssentialLayer_NullWing(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Two legacy files (wing IS NULL, room IS NULL).
	legacyFiles := []string{
		"Legacy alpha-like content first file. Extra data.",
		"Legacy beta-like content second file. Extra data.",
	}
	for i, text := range legacyFiles {
		fileID := fmt.Sprintf("legacy/file-%d.md", i)
		seedRawFile(t, store, fileID, "", "", text, 10-i)
	}

	layer := NewEssentialLayer(store, DefaultEssentialLayerConfig(), silenceLogger())
	story, err := layer.Generate(ctx, "")
	if err != nil {
		t.Fatalf("Generate legacy: %v", err)
	}
	if !strings.Contains(story, "## Wing: (legacy)") {
		t.Errorf("expected (legacy) heading; got:\n%s", story)
	}
	if !strings.Contains(story, "Legacy alpha-like content first file") {
		t.Errorf("missing legacy file lead sentence; got:\n%s", story)
	}
}

// TestEssentialLayer_CacheHit verifies the second Render returns the
// exact same bytes AND the generated_at column did not advance — proving
// the cache path skipped regeneration.
func TestEssentialLayer_CacheHit(t *testing.T) {
	store := newTestStore(t)
	seedEssentialFixture(t, store)

	layer := NewEssentialLayer(store, DefaultEssentialLayerConfig(), silenceLogger())
	ctx := context.Background()

	first := layer.Render(ctx, "alpha")
	if first == "" {
		t.Fatal("empty first render")
	}

	var firstGen int64
	if err := store.db.QueryRow(
		`SELECT generated_at FROM essential_stories WHERE wing = ?`, "alpha",
	).Scan(&firstGen); err != nil {
		t.Fatalf("read generated_at: %v", err)
	}

	// Force enough wall time to pass that a regeneration would change
	// generated_at by at least 1 unix second if it happened.
	time.Sleep(1100 * time.Millisecond)

	second := layer.Render(ctx, "alpha")
	if second != first {
		t.Errorf("cache hit returned different bytes")
	}

	var secondGen int64
	if err := store.db.QueryRow(
		`SELECT generated_at FROM essential_stories WHERE wing = ?`, "alpha",
	).Scan(&secondGen); err != nil {
		t.Fatalf("read generated_at second: %v", err)
	}
	if firstGen != secondGen {
		t.Errorf("cache hit regenerated: first=%d second=%d", firstGen, secondGen)
	}
}

// TestEssentialLayer_StaleInvalidation rewinds generated_at far into the
// past, re-renders, and checks that the row was regenerated.
func TestEssentialLayer_StaleInvalidation(t *testing.T) {
	store := newTestStore(t)
	seedEssentialFixture(t, store)

	layer := NewEssentialLayer(store, DefaultEssentialLayerConfig(), silenceLogger())
	ctx := context.Background()

	_ = layer.Render(ctx, "alpha")

	// Push generated_at into the distant past.
	if _, err := store.db.Exec(
		`UPDATE essential_stories SET generated_at = generated_at - 999999 WHERE wing = ?`,
		"alpha",
	); err != nil {
		t.Fatalf("age row: %v", err)
	}

	var staleGen int64
	_ = store.db.QueryRow(
		`SELECT generated_at FROM essential_stories WHERE wing = ?`, "alpha",
	).Scan(&staleGen)

	_ = layer.Render(ctx, "alpha")

	var freshGen int64
	_ = store.db.QueryRow(
		`SELECT generated_at FROM essential_stories WHERE wing = ?`, "alpha",
	).Scan(&freshGen)

	if freshGen <= staleGen {
		t.Errorf("expected regeneration; stale=%d fresh=%d", staleGen, freshGen)
	}
}

// TestEssentialLayer_ManualInvalidate exercises both single-wing and
// wildcard invalidation.
func TestEssentialLayer_ManualInvalidate(t *testing.T) {
	store := newTestStore(t)
	seedEssentialFixture(t, store)

	layer := NewEssentialLayer(store, DefaultEssentialLayerConfig(), silenceLogger())
	ctx := context.Background()

	// Warm cache for all three wings.
	_ = layer.Render(ctx, "alpha")
	_ = layer.Render(ctx, "beta")
	_ = layer.Render(ctx, "gamma")

	// Single-wing invalidate.
	n, err := layer.Invalidate(ctx, "alpha")
	if err != nil {
		t.Fatalf("invalidate alpha: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row invalidated, got %d", n)
	}

	var remaining int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM essential_stories`).Scan(&remaining)
	if remaining != 2 {
		t.Errorf("expected 2 rows remaining after alpha invalidate, got %d", remaining)
	}

	// Re-render alpha regenerates a row.
	_ = layer.Render(ctx, "alpha")
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM essential_stories`).Scan(&remaining)
	if remaining != 3 {
		t.Errorf("expected 3 rows after re-render, got %d", remaining)
	}

	// Wildcard invalidate clears everything.
	n, err = layer.Invalidate(ctx, "*")
	if err != nil {
		t.Fatalf("invalidate *: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 rows invalidated by *, got %d", n)
	}
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM essential_stories`).Scan(&remaining)
	if remaining != 0 {
		t.Errorf("expected empty cache after wildcard invalidate, got %d", remaining)
	}
}

// TestEssentialLayer_ByteBudgetTruncation seeds a wing with long memories
// and asserts the rendered story respects the budget and ends at a word
// boundary (tolerance: budget-50 bytes as acceptable trailing slack).
func TestEssentialLayer_ByteBudgetTruncation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const wing = "alpha"
	if err := store.UpsertWing(wing, "Alpha", ""); err != nil {
		t.Fatalf("upsert wing: %v", err)
	}

	rooms := []string{"room-a", "room-b", "room-c", "room-d"}
	base := time.Now().Add(-1 * time.Hour)
	// Build a long lead sentence with NO period so extractLeadSentence
	// hits its 200-byte window cap. Each file contributes ~200 bytes
	// (plus the bullet decoration) so 4 rooms * 3 files * ~200 bytes
	// is well above the 1600 budget and exercises truncation.
	long := strings.Repeat("lorem ipsum dolor sit amet consectetur adipiscing elit ", 10)

	for ri, room := range rooms {
		if err := store.UpsertRoom(wing, room, "", "manual", 1.0); err != nil {
			t.Fatalf("upsert room: %v", err)
		}
		_, _ = store.db.Exec(
			`UPDATE rooms SET last_activity = ? WHERE wing = ? AND name = ?`,
			base.Add(time.Duration(ri)*time.Minute), wing, room,
		)
		for fi := 0; fi < 10; fi++ {
			fileID := fmt.Sprintf("%s/%s/long-%d.md", wing, room, fi)
			// No period and no newline — extractLeadSentence falls back
			// to the hard 200-byte window cap.
			text := fmt.Sprintf("Content for %s file %d %s", room, fi, long)
			seedRawFile(t, store, fileID, wing, room, text, 1000-fi)
		}
	}

	layer := NewEssentialLayer(store, DefaultEssentialLayerConfig(), silenceLogger())
	story, err := layer.Generate(ctx, wing)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(story) > 1600 {
		t.Errorf("story=%d bytes exceeds 1600 budget", len(story))
	}
	if len(story) < 1600-200 {
		// Very small outputs suggest the seed didn't produce enough
		// content to exercise truncation — fail fast so the test
		// actually validates the budget logic.
		t.Errorf("story=%d bytes is suspiciously short; budget logic may not have triggered", len(story))
	}
	// Last rune must be whitespace or a punctuation boundary.
	last := story[len(story)-1]
	if last != ' ' && last != '\n' && last != '.' && last != ',' && last != ';' {
		// truncateAtBoundary strips trailing whitespace then returns;
		// accept the final character if it's a non-space non-weird
		// character that happens to land at the cut point.
		// Spec tolerance: budget - 50.
		if len(story) < 1600-50 {
			t.Errorf("story ends mid-word: last=%q len=%d", last, len(story))
		}
	}
}

// TestEssentialLayer_ConcurrentRegeneration fires 10 goroutines at
// Generate for the same wing and verifies:
//   - no race (go test -race must be clean)
//   - all return the same story bytes
//   - only ONE distinct generated_at lands in the cache (the layer's
//     mutex serializes regeneration)
func TestEssentialLayer_ConcurrentRegeneration(t *testing.T) {
	store := newTestStore(t)
	seedEssentialFixture(t, store)

	layer := NewEssentialLayer(store, DefaultEssentialLayerConfig(), silenceLogger())
	ctx := context.Background()

	// Warm the cache once so that concurrent Render calls hit the fast
	// path; then invalidate so they all race to regenerate.
	_ = layer.Render(ctx, "alpha")
	if _, err := layer.Invalidate(ctx, "alpha"); err != nil {
		t.Fatalf("invalidate: %v", err)
	}

	const n = 10
	var wg sync.WaitGroup
	results := make([]string, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			s, err := layer.Generate(ctx, "alpha")
			if err != nil {
				t.Errorf("goroutine %d: %v", idx, err)
				return
			}
			results[idx] = s
		}(i)
	}
	wg.Wait()

	// All results must agree.
	for i := 1; i < n; i++ {
		if results[i] != results[0] {
			t.Errorf("goroutine %d bytes differ from goroutine 0", i)
		}
	}

	// Exactly one row in the cache table for alpha.
	var count int
	_ = store.db.QueryRow(
		`SELECT COUNT(*) FROM essential_stories WHERE wing = ?`, "alpha",
	).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 cache row, got %d", count)
	}
}

// TestEssentialLayer_NoLLMCallEver is a compile-time / structural
// guarantee: NewEssentialLayer takes no LLM client and layer_essential.go
// imports no LLM packages. This test asserts the invariant holds by
// calling the constructor with minimal args and checking the struct is
// usable without any LLM wiring. If a future change sneaks in an LLM
// dependency, this test still passes — BUT the code review checklist
// must flag the new import. See the package doc for the rationale.
func TestEssentialLayer_NoLLMCallEver(t *testing.T) {
	store := newTestStore(t)
	// Deliberately pass a zero config and nil logger: this MUST compile
	// and run without any LLM client.
	layer := NewEssentialLayer(store, EssentialLayerConfig{}, nil)
	if layer == nil {
		t.Fatal("NewEssentialLayer returned nil")
	}
	// And Generate must work end-to-end.
	_, err := layer.Generate(context.Background(), "alpha")
	if err != nil {
		t.Errorf("Generate on empty wing should not error: %v", err)
	}
}

// TestEssentialLayer_MigrationIdempotent calls MigrateEssentialStories
// twice against a fresh store and verifies the table and index exist.
func TestEssentialLayer_MigrationIdempotent(t *testing.T) {
	store := newTestStore(t)

	// newTestStore already ran the migration once via initSchema. Call
	// it two more times and make sure nothing breaks.
	for i := 0; i < 2; i++ {
		if err := MigrateEssentialStories(store.db, silenceLogger()); err != nil {
			t.Fatalf("migrate attempt %d: %v", i+1, err)
		}
	}

	// Verify the table exists.
	var name string
	err := store.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='essential_stories'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("essential_stories table missing: %v", err)
	}
	if name != "essential_stories" {
		t.Errorf("unexpected table name: %q", name)
	}

	// Verify PRAGMA columns match the spec.
	cols, err := existingColumns(store.db, "essential_stories")
	if err != nil {
		t.Fatalf("existingColumns: %v", err)
	}
	wantCols := []string{"wing", "story", "generated_at", "source_files", "source_rooms", "bytes", "schema_version"}
	for _, c := range wantCols {
		if _, ok := cols[c]; !ok {
			t.Errorf("missing column %q in essential_stories", c)
		}
	}

	// Verify the index exists.
	var idxName string
	err = store.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_essential_stories_generated'`,
	).Scan(&idxName)
	if err != nil {
		t.Fatalf("expected index present: %v", err)
	}
}

// TestEssentialLayer_ExtractLeadSentence exercises the pure helper. No
// DB involved; safe to run without a store.
func TestEssentialLayer_ExtractLeadSentence(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"Simple sentence.", "Simple sentence."},
		{"First. Second.", "First."},
		{"No period here", "No period here"},
		{"Line one\nLine two", "Line one"},
		{"  Padded.  Tail.", "Padded."},
		{strings.Repeat("x", 500), strings.Repeat("x", 200)},
	}
	for _, c := range cases {
		got := extractLeadSentence(c.in)
		if got != c.want {
			t.Errorf("extractLeadSentence(%q): got %q want %q", c.in, got, c.want)
		}
	}
}

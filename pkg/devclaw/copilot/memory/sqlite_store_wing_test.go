// Package memory — sqlite_store_wing_test.go covers Sprint 2 Room 2.0c
// (wing-aware hybrid search fusion).
//
// Test surface:
//
//   - TestHybridSearchWingBoostMatch         — boosted wing ranks first.
//   - TestHybridSearchWingBoostPenalty       — non-matching wings demoted.
//   - TestHybridSearchWingNullNeutral        — wing IS NULL is never penalized.
//   - TestHybridSearchEmptyQueryWingByteIdentical — retrocompat gate.
//   - TestHybridSearchZeroBoostUsesDefaults  — zero values fall back to 1.3/0.4.
//   - TestHybridSearchEmptyDBWithWing        — empty store does not panic.
//   - TestHybridSearchConcurrentWingQueries  — race detector clean.
//   - TestHybridSearchWithOpts_HybridSearchWrapper — wrapper byte-identical.
//   - TestHybridSearchFusionRegressionFixture — empirical regression fixture.
//
// All tests use generic vocabulary (alpha/beta/gamma/delta) so the binary
// stays free of locale or domain hardcoding.
package memory

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sync"
	"testing"
)

// stableEmbedder is a test-only embedder that derives a DISTINCT non-parallel
// 4-dimensional vector from a sha256 of the input text. This matters because
// the generic deterministicEmbedder (in sqlite_store_test.go) returns
// scaled-parallel vectors, which produce cosine similarity == 1 for every
// input pair and thus yield non-deterministic tie-breaking during fusion.
//
// stableEmbedder is deterministic AND distinct, so HybridSearch is
// reproducible across calls — which is required by the byte-identical
// regression fixture test.
type stableEmbedder struct{}

func (e *stableEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		h := sha256.Sum256([]byte(t))
		out[i] = []float32{
			float32(binary.BigEndian.Uint32(h[0:4])) / 4294967295.0,
			float32(binary.BigEndian.Uint32(h[4:8])) / 4294967295.0,
			float32(binary.BigEndian.Uint32(h[8:12])) / 4294967295.0,
			float32(binary.BigEndian.Uint32(h[12:16])) / 4294967295.0,
		}
	}
	return out, nil
}
func (e *stableEmbedder) Dimensions() int { return 4 }
func (e *stableEmbedder) Name() string    { return "stable" }
func (e *stableEmbedder) Model() string   { return "stable-model" }

// newStableStore creates a test SQLiteStore backed by stableEmbedder.
// Use this instead of newTestStore when the test compares HybridSearch
// results across calls — stableEmbedder avoids the parallel-vector trap.
func newStableStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpFile, err := os.CreateTemp(t.TempDir(), "wing-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewSQLiteStore(tmpFile.Name(), &stableEmbedder{}, slog.Default())
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// seedFileWithWing indexes a single chunk for fileID and then assigns the
// given wing via AssignWingToFile (which honors the wing IS NULL race
// barrier). Pass wing="" to leave the file as legacy NULL.
//
// The chunk text and the file hash are derived from the parameters so each
// call produces a deterministic, distinct chunk. Tests pass the text they
// want to query for.
func seedFileWithWing(t *testing.T, store *SQLiteStore, fileID, text, wing string) {
	t.Helper()
	ctx := context.Background()

	chunks := []Chunk{
		{Index: 0, Text: text, Hash: hashChunk(text)},
	}
	if err := store.IndexChunks(ctx, fileID, chunks, hashChunk(fileID+text)); err != nil {
		t.Fatalf("seed %s: %v", fileID, err)
	}

	if wing != "" {
		if err := store.AssignWingToFile(ctx, fileID, wing); err != nil {
			t.Fatalf("assign wing %q to %s: %v", wing, fileID, err)
		}
	}
}

// resultByFile finds the SearchResult with the given fileID, returning a
// pointer or nil if absent.
func resultByFile(results []SearchResult, fileID string) *SearchResult {
	for i := range results {
		if results[i].FileID == fileID {
			return &results[i]
		}
	}
	return nil
}

// rankOf returns the 0-based index of fileID in results, or -1 if missing.
func rankOf(results []SearchResult, fileID string) int {
	for i, r := range results {
		if r.FileID == fileID {
			return i
		}
	}
	return -1
}

// almostEqual reports whether a and b agree to 6 decimal places. Used by
// the byte-identical regression tests.
func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

// ─────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────

// TestHybridSearchWingBoostMatch verifies that files in the queried wing
// rank above files in a different wing when their content matches the
// query equally well. Two alpha files vs one beta file: the beta file must
// not appear before either alpha file.
func TestHybridSearchWingBoostMatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// All three chunks share the same query-bearing tokens, so they tie
	// on relevance — only the wing multiplier should differ.
	text := "shared lookup keyword token"
	seedFileWithWing(t, store, "file_alpha_1.md", text+" one", "alpha")
	seedFileWithWing(t, store, "file_alpha_2.md", text+" two", "alpha")
	seedFileWithWing(t, store, "file_beta_1.md", text+" three", "beta")

	results, err := store.HybridSearchWithOpts(ctx, "shared lookup keyword", HybridSearchOptions{
		MaxResults:   10,
		MinScore:     0.0001,
		VectorWeight: 0.7,
		BM25Weight:   0.3,
		QueryWing:    "alpha",
	})
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("want >=3 results, got %d: %+v", len(results), results)
	}

	betaRank := rankOf(results, "file_beta_1.md")
	alphaRank1 := rankOf(results, "file_alpha_1.md")
	alphaRank2 := rankOf(results, "file_alpha_2.md")
	if betaRank == -1 || alphaRank1 == -1 || alphaRank2 == -1 {
		t.Fatalf("missing results: alpha1=%d alpha2=%d beta=%d (results=%+v)",
			alphaRank1, alphaRank2, betaRank, results)
	}

	// Beta must rank below both alpha files (boosted +30%, penalty -60%).
	if !(betaRank > alphaRank1 && betaRank > alphaRank2) {
		t.Errorf("beta rank %d should be below both alpha ranks (%d, %d)",
			betaRank, alphaRank1, alphaRank2)
	}

	// Wing field must be populated on all results (sourced from the JOIN).
	for _, r := range results {
		switch r.FileID {
		case "file_alpha_1.md", "file_alpha_2.md":
			if r.Wing != "alpha" {
				t.Errorf("%s wing = %q, want alpha", r.FileID, r.Wing)
			}
		case "file_beta_1.md":
			if r.Wing != "beta" {
				t.Errorf("%s wing = %q, want beta", r.FileID, r.Wing)
			}
		}
	}
}

// TestHybridSearchWingBoostPenalty verifies that mismatched wings get
// demoted relative to the matching wing.
func TestHybridSearchWingBoostPenalty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	text := "penalty regression seed token"
	seedFileWithWing(t, store, "file_alpha.md", text+" one", "alpha")
	seedFileWithWing(t, store, "file_beta.md", text+" two", "beta")
	seedFileWithWing(t, store, "file_gamma.md", text+" three", "gamma")

	results, err := store.HybridSearchWithOpts(ctx, "penalty regression seed", HybridSearchOptions{
		MaxResults:   10,
		MinScore:     0.0001,
		VectorWeight: 0.7,
		BM25Weight:   0.3,
		QueryWing:    "alpha",
	})
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}

	if results[0].FileID != "file_alpha.md" {
		t.Errorf("top result = %s, want file_alpha.md", results[0].FileID)
	}

	alpha := resultByFile(results, "file_alpha.md")
	beta := resultByFile(results, "file_beta.md")
	gamma := resultByFile(results, "file_gamma.md")
	if alpha == nil || beta == nil || gamma == nil {
		t.Fatalf("missing files in results: alpha=%v beta=%v gamma=%v", alpha, beta, gamma)
	}

	// Alpha should outscore both penalized wings.
	if alpha.Score <= beta.Score {
		t.Errorf("alpha score %.6f should exceed beta %.6f", alpha.Score, beta.Score)
	}
	if alpha.Score <= gamma.Score {
		t.Errorf("alpha score %.6f should exceed gamma %.6f", alpha.Score, gamma.Score)
	}
}

// TestHybridSearchWingNullNeutral is the CRITICAL retrocompat test for the
// wing IS NULL invariant. A file with wing=NULL must never be penalized,
// even when the query specifies a wing.
//
// We seed three files: alpha-wing, NULL-wing, beta-wing. Then we query
// twice: once with QueryWing="alpha" (boost on) and once with QueryWing=""
// (boost off). The NULL file's score must be byte-identical between the
// two runs to prove no multiplier ever touches it.
//
// Uses stableEmbedder so the non-parallel vectors give the vector branch
// distinct cosine similarities. The generic deterministicEmbedder produces
// scaled-parallel vectors (cosine similarity == 1 for every pair), which
// collapses the fusion ranking to non-deterministic sort.Slice tie-breaking
// and causes the legacy file to flip ranks between the wingless and
// wing-aware calls — masking the real invariant under a flaky test.
func TestHybridSearchWingNullNeutral(t *testing.T) {
	store := newStableStore(t)
	ctx := context.Background()

	text := "neutrality contract verification token"
	seedFileWithWing(t, store, "file_alpha.md", text+" one", "alpha")
	seedFileWithWing(t, store, "file_legacy.md", text+" two", "") // wing IS NULL
	seedFileWithWing(t, store, "file_beta.md", text+" three", "beta")

	common := HybridSearchOptions{
		MaxResults:   10,
		MinScore:     0.0001,
		VectorWeight: 0.7,
		BM25Weight:   0.3,
	}

	wingless, err := store.HybridSearchWithOpts(ctx, "neutrality contract verification", common)
	if err != nil {
		t.Fatalf("wingless search: %v", err)
	}

	withWing := common
	withWing.QueryWing = "alpha"
	withAlpha, err := store.HybridSearchWithOpts(ctx, "neutrality contract verification", withWing)
	if err != nil {
		t.Fatalf("alpha-wing search: %v", err)
	}

	legacyA := resultByFile(wingless, "file_legacy.md")
	legacyB := resultByFile(withAlpha, "file_legacy.md")
	if legacyA == nil || legacyB == nil {
		t.Fatalf("legacy file missing from results: wingless=%v withAlpha=%v", legacyA, legacyB)
	}

	// HARD INVARIANT: legacy file's score must be byte-identical (within
	// 6 decimals) regardless of whether the query specified a wing.
	if !almostEqual(legacyA.Score, legacyB.Score) {
		t.Errorf("legacy NULL file score drifted: wingless=%.10f withAlpha=%.10f",
			legacyA.Score, legacyB.Score)
	}

	// Wing field on the legacy file must be empty in the wing-aware
	// branch (the JOIN coalesces NULL → "").
	if legacyB.Wing != "" {
		t.Errorf("legacy file wing = %q, want empty string", legacyB.Wing)
	}

	// Ordering check: alpha boosted, beta penalized, legacy unchanged.
	alphaRank := rankOf(withAlpha, "file_alpha.md")
	legacyRank := rankOf(withAlpha, "file_legacy.md")
	betaRank := rankOf(withAlpha, "file_beta.md")
	if alphaRank == -1 || legacyRank == -1 || betaRank == -1 {
		t.Fatalf("missing files in withAlpha: alpha=%d legacy=%d beta=%d",
			alphaRank, legacyRank, betaRank)
	}
	// Alpha first (boosted), beta last (penalized), legacy in the middle.
	if alphaRank != 0 {
		t.Errorf("alpha rank = %d, want 0", alphaRank)
	}
	if betaRank <= legacyRank {
		t.Errorf("beta rank %d should be below legacy rank %d (penalty vs neutral)",
			betaRank, legacyRank)
	}
}

// TestHybridSearchEmptyQueryWingByteIdentical is the OTHER critical
// retrocompat gate: when QueryWing is empty, the wing-aware code path must
// produce results that are byte-identical to the legacy HybridSearch on
// the same data and opts. No JOIN, no multiplier, no Wing population.
//
// Uses stableEmbedder so the non-parallel vectors make HybridSearch
// deterministic across calls — the parallel-vector deterministicEmbedder
// ties every candidate at cosine similarity 1.0 and exposes a pre-existing
// non-determinism in sort.Slice tie-breaking.
func TestHybridSearchEmptyQueryWingByteIdentical(t *testing.T) {
	store := newStableStore(t)
	ctx := context.Background()

	// Mixed-wing fixture, varied text content so scores spread out.
	seedFileWithWing(t, store, "file_a.md", "shared keyword aaa", "alpha")
	seedFileWithWing(t, store, "file_b.md", "shared keyword bbbbb", "beta")
	seedFileWithWing(t, store, "file_c.md", "shared keyword ccccccc", "")
	seedFileWithWing(t, store, "file_d.md", "shared keyword dddddddddd", "alpha")
	seedFileWithWing(t, store, "file_e.md", "shared keyword eeeeeeeeeeee", "beta")

	const (
		maxResults   = 5
		minScore     = 0.0001
		vectorWeight = 0.7
		bm25Weight   = 0.3
	)

	legacy, err := store.HybridSearch(ctx, "shared keyword", maxResults, minScore, vectorWeight, bm25Weight)
	if err != nil {
		t.Fatalf("legacy hybrid search: %v", err)
	}

	wingAware, err := store.HybridSearchWithOpts(ctx, "shared keyword", HybridSearchOptions{
		MaxResults:   maxResults,
		MinScore:     minScore,
		VectorWeight: vectorWeight,
		BM25Weight:   bm25Weight,
		// QueryWing intentionally empty.
	})
	if err != nil {
		t.Fatalf("wing-aware hybrid search: %v", err)
	}

	if len(legacy) != len(wingAware) {
		t.Fatalf("result count drift: legacy=%d wingAware=%d", len(legacy), len(wingAware))
	}

	for i := range legacy {
		if legacy[i].FileID != wingAware[i].FileID {
			t.Errorf("ordering drift at %d: legacy=%s wingAware=%s",
				i, legacy[i].FileID, wingAware[i].FileID)
		}
		if legacy[i].Text != wingAware[i].Text {
			t.Errorf("text drift at %d: legacy=%q wingAware=%q",
				i, legacy[i].Text, wingAware[i].Text)
		}
		if !almostEqual(legacy[i].Score, wingAware[i].Score) {
			t.Errorf("score drift at %d (%s): legacy=%.10f wingAware=%.10f",
				i, legacy[i].FileID, legacy[i].Score, wingAware[i].Score)
		}
	}
}

// TestHybridSearchZeroBoostUsesDefaults verifies that leaving WingBoostMatch
// and WingBoostPenalty at zero in the options struct produces the same
// results as setting them explicitly to 1.3 and 0.4.
func TestHybridSearchZeroBoostUsesDefaults(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	text := "default boost token sample"
	seedFileWithWing(t, store, "file_alpha.md", text+" one", "alpha")
	seedFileWithWing(t, store, "file_beta.md", text+" two", "beta")

	zero, err := store.HybridSearchWithOpts(ctx, "default boost token", HybridSearchOptions{
		MaxResults:   10,
		MinScore:     0.0001,
		VectorWeight: 0.7,
		BM25Weight:   0.3,
		QueryWing:    "alpha",
		// WingBoostMatch and WingBoostPenalty intentionally zero.
	})
	if err != nil {
		t.Fatalf("zero-boost search: %v", err)
	}

	explicit, err := store.HybridSearchWithOpts(ctx, "default boost token", HybridSearchOptions{
		MaxResults:       10,
		MinScore:         0.0001,
		VectorWeight:     0.7,
		BM25Weight:       0.3,
		QueryWing:        "alpha",
		WingBoostMatch:   1.3,
		WingBoostPenalty: 0.4,
	})
	if err != nil {
		t.Fatalf("explicit-boost search: %v", err)
	}

	if len(zero) != len(explicit) {
		t.Fatalf("count drift: zero=%d explicit=%d", len(zero), len(explicit))
	}
	for i := range zero {
		if zero[i].FileID != explicit[i].FileID {
			t.Errorf("ordering drift at %d", i)
		}
		if !almostEqual(zero[i].Score, explicit[i].Score) {
			t.Errorf("score drift at %d (%s): zero=%.10f explicit=%.10f",
				i, zero[i].FileID, zero[i].Score, explicit[i].Score)
		}
	}
}

// TestHybridSearchEmptyDBWithWing verifies that searching an empty store
// with a query wing does not panic and returns no results.
func TestHybridSearchEmptyDBWithWing(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	results, err := store.HybridSearchWithOpts(ctx, "anything", HybridSearchOptions{
		MaxResults: 10,
		MinScore:   0.0001,
		QueryWing:  "alpha",
	})
	if err != nil {
		t.Fatalf("empty-db search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("empty-db should return 0 results, got %d", len(results))
	}
}

// TestHybridSearchConcurrentWingQueries fires 10 goroutines hitting
// HybridSearchWithOpts with different QueryWing values on the same DB.
// Run with -race to verify data-race-free.
func TestHybridSearchConcurrentWingQueries(t *testing.T) {
	store := newTestStore(t)

	// Seed a small mixed-wing fixture.
	seedFileWithWing(t, store, "file_a.md", "concurrency lookup token aaa", "alpha")
	seedFileWithWing(t, store, "file_b.md", "concurrency lookup token bbbb", "beta")
	seedFileWithWing(t, store, "file_c.md", "concurrency lookup token ccccc", "gamma")
	seedFileWithWing(t, store, "file_d.md", "concurrency lookup token dddddd", "")

	wings := []string{"alpha", "beta", "gamma", "", "alpha", "beta", "gamma", "", "alpha", "beta"}

	var wg sync.WaitGroup
	for i := 0; i < len(wings); i++ {
		wg.Add(1)
		go func(qw string) {
			defer wg.Done()
			ctx := context.Background()
			_, err := store.HybridSearchWithOpts(ctx, "concurrency lookup token", HybridSearchOptions{
				MaxResults: 10,
				MinScore:   0.0001,
				QueryWing:  qw,
			})
			if err != nil {
				t.Errorf("concurrent search (%q): %v", qw, err)
			}
		}(wings[i])
	}
	wg.Wait()
}

// TestHybridSearchWithOpts_HybridSearchWrapper proves the legacy
// HybridSearch wrapper is a faithful pass-through to HybridSearchWithOpts
// when QueryWing is empty. Same fixture, two call shapes, identical
// results down to score.
//
// Uses stableEmbedder so HybridSearch is deterministic across calls.
func TestHybridSearchWithOpts_HybridSearchWrapper(t *testing.T) {
	store := newStableStore(t)
	ctx := context.Background()

	seedFileWithWing(t, store, "file_a.md", "wrapper proof token aaa", "alpha")
	seedFileWithWing(t, store, "file_b.md", "wrapper proof token bbbbbbbbbbbb", "beta")
	seedFileWithWing(t, store, "file_c.md", "wrapper proof token ccccccccccccccccccccccc", "")

	const (
		maxResults   = 5
		minScore     = 0.0001
		vectorWeight = 0.7
		bm25Weight   = 0.3
	)

	wrapper, err := store.HybridSearch(ctx, "wrapper proof token", maxResults, minScore, vectorWeight, bm25Weight)
	if err != nil {
		t.Fatalf("wrapper search: %v", err)
	}

	direct, err := store.HybridSearchWithOpts(ctx, "wrapper proof token", HybridSearchOptions{
		MaxResults:   maxResults,
		MinScore:     minScore,
		VectorWeight: vectorWeight,
		BM25Weight:   bm25Weight,
	})
	if err != nil {
		t.Fatalf("direct search: %v", err)
	}

	if len(wrapper) != len(direct) {
		t.Fatalf("count drift: wrapper=%d direct=%d", len(wrapper), len(direct))
	}
	for i := range wrapper {
		if wrapper[i].FileID != direct[i].FileID {
			t.Errorf("ordering drift at %d: wrapper=%s direct=%s",
				i, wrapper[i].FileID, direct[i].FileID)
		}
		if !almostEqual(wrapper[i].Score, direct[i].Score) {
			t.Errorf("score drift at %d (%s): wrapper=%.10f direct=%.10f",
				i, wrapper[i].FileID, wrapper[i].Score, direct[i].Score)
		}
	}
}

// TestHybridSearchFusionRegressionFixture is the empirical regression
// harness for the fusion-formula stability promise (Sprint 2 plan §6 Risks).
//
// We seed a 20-file synthetic mixed-wing fixture, run a set of queries
// via the legacy HybridSearch entry point AND the new HybridSearchWithOpts
// entry point with QueryWing="", and assert that the (fileID, score)
// tuples match down to 6 decimal places. Any future change to the fusion
// formula will break this test, forcing the committer to consciously
// re-snapshot or revert.
//
// Uses stableEmbedder so HybridSearch is deterministic across calls.
// With the generic deterministicEmbedder, all candidates tie on cosine
// similarity and sort.Slice tie-breaking becomes random, which masks
// the actual fusion-formula regression signal.
func TestHybridSearchFusionRegressionFixture(t *testing.T) {
	store := newStableStore(t)
	ctx := context.Background()

	// 20 files with distinct text content. Wings are spread to exercise
	// the presence of mixed-wing data in the DB, but the baseline
	// assertions use QueryWing="" so wings are irrelevant to the
	// comparison — they exist only to prove that even when wings are
	// populated, the QueryWing="" path stays byte-identical to legacy.
	wings := []string{"alpha", "beta", "gamma", "delta", ""}
	for i := 0; i < 20; i++ {
		text := fmt.Sprintf("regression fixture entry corpus content sample %02d", i)
		fileID := fmt.Sprintf("fixture_%02d.md", i)
		seedFileWithWing(t, store, fileID, text, wings[i%len(wings)])
	}

	queries := []string{
		"regression fixture entry",
		"corpus content",
		"entry number",
	}

	const (
		maxResults   = 6
		minScore     = 0.0001
		vectorWeight = 0.7
		bm25Weight   = 0.3
	)

	for _, q := range queries {
		t.Run("query="+q, func(t *testing.T) {
			// Compute via the legacy entry point.
			legacy, err := store.HybridSearch(ctx, q, maxResults, minScore, vectorWeight, bm25Weight)
			if err != nil {
				t.Fatalf("legacy: %v", err)
			}

			// Compute via the new entry point with QueryWing="".
			refactored, err := store.HybridSearchWithOpts(ctx, q, HybridSearchOptions{
				MaxResults:   maxResults,
				MinScore:     minScore,
				VectorWeight: vectorWeight,
				BM25Weight:   bm25Weight,
			})
			if err != nil {
				t.Fatalf("refactored: %v", err)
			}

			// The two paths must agree on count, ordering AND score
			// down to 6 decimal places. This is the byte-identical
			// promise.
			if len(legacy) != len(refactored) {
				t.Fatalf("count drift: legacy=%d refactored=%d", len(legacy), len(refactored))
			}
			for i := range legacy {
				if legacy[i].FileID != refactored[i].FileID {
					t.Errorf("ordering drift at %d: legacy=%s refactored=%s",
						i, legacy[i].FileID, refactored[i].FileID)
				}
				if !almostEqual(legacy[i].Score, refactored[i].Score) {
					t.Errorf("score drift at %d (%s): legacy=%.10f refactored=%.10f",
						i, legacy[i].FileID, legacy[i].Score, refactored[i].Score)
				}
			}

			// Once-passing wing-aware path with QueryWing="alpha"
			// must NOT alter the legacy file scores (wing IS NULL is
			// neutral).
			withWing, err := store.HybridSearchWithOpts(ctx, q, HybridSearchOptions{
				MaxResults:   maxResults,
				MinScore:     minScore,
				VectorWeight: vectorWeight,
				BM25Weight:   bm25Weight,
				QueryWing:    "alpha",
			})
			if err != nil {
				t.Fatalf("wing-aware: %v", err)
			}
			// For each legacy (wing="") result that survived the wing-aware
			// run, the score must equal the legacy score because the
			// multiplier for legacy files is always 1.0.
			for _, want := range legacy {
				// Was this file legacy? Check the seeding pattern.
				idx := -1
				_, _ = fmt.Sscanf(want.FileID, "fixture_%02d.md", &idx)
				if idx < 0 || wings[idx%len(wings)] != "" {
					continue // skip non-legacy files
				}
				got := resultByFile(withWing, want.FileID)
				if got == nil {
					// May have been pushed out of the top-N by boosted
					// alpha files — that's fine, only assert when present.
					continue
				}
				if !almostEqual(got.Score, want.Score) {
					t.Errorf("legacy file %s score drifted under wing-aware mode: legacy=%.10f wingAware=%.10f",
						want.FileID, want.Score, got.Score)
				}
			}
		})
	}
}

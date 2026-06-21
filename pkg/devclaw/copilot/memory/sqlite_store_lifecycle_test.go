// Package memory — sqlite_store_lifecycle_test.go covers the read-side
// lifecycle guards: deleted_at / expires_at chunks must NEVER surface from
// SearchBM25, SearchVector, or HybridSearchWithOpts, while a normal chunk
// still returns. Since v1.22.1, low_signal chunks are NO LONGER hard-excluded
// — they are returned by every retrieval surface and instead ranked DOWN via a
// penalty in the fusion step (see TestRecallSoftDemotesLowSignal).
package memory

import (
	"context"
	"testing"
)

// TestRecallExcludesLifecycleChunks seeds three files that all match the same
// query token, marks two of them with a disqualifying lifecycle state
// (soft-deleted, expired), then asserts every recall surface drops the two and
// keeps the normal one. (low_signal is covered separately by the soft-demote
// test below — it is deliberately NOT in the hard-excluded set anymore.)
func TestRecallExcludesLifecycleChunks(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	token := "lifecycle recall guard token"
	seedFileWithWing(t, store, "file_ok.md", token+" alpha", "")
	seedFileWithWing(t, store, "file_deleted.md", token+" beta", "")
	seedFileWithWing(t, store, "file_expired.md", token+" gamma", "")

	// Apply lifecycle states directly via SQL (the write path that sets these
	// columns is owned by other stories; the recall guard is what we test).
	mustExec(t, store, `UPDATE chunks SET deleted_at = CURRENT_TIMESTAMP WHERE file_id = ?`, "file_deleted.md")
	mustExec(t, store, `UPDATE chunks SET expires_at = datetime(CURRENT_TIMESTAMP, '-1 day') WHERE file_id = ?`, "file_expired.md")

	// SearchVector scores the in-memory cache, so reload it to reflect the
	// lifecycle updates (loadVectorCache applies the same guard).
	if err := store.loadVectorCache(); err != nil {
		t.Fatalf("reload vector cache: %v", err)
	}

	excluded := []string{"file_deleted.md", "file_expired.md"}

	// SearchBM25 (FTS path).
	bm25, err := store.SearchBM25(token, 10)
	if err != nil {
		t.Fatalf("SearchBM25: %v", err)
	}
	assertPresentAbsent(t, "SearchBM25", bm25, "file_ok.md", excluded)

	// SearchVector (in-memory cosine path).
	vec, err := store.SearchVector(ctx, token, 10)
	if err != nil {
		t.Fatalf("SearchVector: %v", err)
	}
	assertPresentAbsent(t, "SearchVector", vec, "file_ok.md", excluded)

	// HybridSearchWithOpts (fuses both; inherits the guard).
	hyb, err := store.HybridSearchWithOpts(ctx, token, HybridSearchOptions{
		MaxResults:   10,
		MinScore:     0.0001,
		VectorWeight: 0.7,
		BM25Weight:   0.3,
	})
	if err != nil {
		t.Fatalf("HybridSearchWithOpts: %v", err)
	}
	assertPresentAbsent(t, "HybridSearchWithOpts", hyb, "file_ok.md", excluded)

	// Also exercise the LIKE fallback path explicitly.
	like, err := store.searchLikeFallback(token, 10)
	if err != nil {
		t.Fatalf("searchLikeFallback: %v", err)
	}
	assertPresentAbsent(t, "searchLikeFallback", like, "file_ok.md", excluded)
}

// TestRecallSoftDemotesLowSignal is the v1.22.1 guardrail: a low_signal chunk
// is NOT excluded from recall (it is still returned by BM25/Vector/LIKE), but
// it ranks below a high-signal chunk that matches the same query.
func TestRecallSoftDemotesLowSignal(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	token := "softdemote recall token"
	seedFileWithWing(t, store, "file_high.md", token+" alpha", "")
	seedFileWithWing(t, store, "file_low.md", token+" beta", "")

	mustExec(t, store, `UPDATE chunks SET curation_status = 'low_signal' WHERE file_id = ?`, "file_low.md")

	// Reload the cache so the vector path sees the updated curation_status.
	if err := store.loadVectorCache(); err != nil {
		t.Fatalf("reload vector cache: %v", err)
	}

	// low_signal must NOT be excluded from the raw BM25 surface.
	bm25, err := store.SearchBM25(token, 10)
	if err != nil {
		t.Fatalf("SearchBM25: %v", err)
	}
	if rankOf(bm25, "file_low.md") == -1 {
		t.Errorf("SearchBM25: low_signal chunk must still be returned, got %+v", bm25)
	}
	if rankOf(bm25, "file_high.md") == -1 {
		t.Errorf("SearchBM25: high_signal chunk must be returned, got %+v", bm25)
	}

	// In the fused ranking, the high-signal chunk must outrank the low_signal
	// one (the low_signal penalty drops its score).
	hyb, err := store.HybridSearchWithOpts(ctx, token, HybridSearchOptions{
		MaxResults:   10,
		MinScore:     0.0001,
		VectorWeight: 0.7,
		BM25Weight:   0.3,
	})
	if err != nil {
		t.Fatalf("HybridSearchWithOpts: %v", err)
	}
	highRank := rankOf(hyb, "file_high.md")
	lowRank := rankOf(hyb, "file_low.md")
	if highRank == -1 {
		t.Fatalf("high_signal chunk missing from hybrid results: %+v", hyb)
	}
	if lowRank == -1 {
		t.Fatalf("low_signal chunk must remain recallable from hybrid results: %+v", hyb)
	}
	if highRank >= lowRank {
		t.Errorf("high_signal (rank %d) must outrank low_signal (rank %d): %+v", highRank, lowRank, hyb)
	}
}

func mustExec(t *testing.T, store *SQLiteStore, query string, args ...any) {
	t.Helper()
	if _, err := store.db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func assertPresentAbsent(t *testing.T, surface string, results []SearchResult, wantPresent string, wantAbsent []string) {
	t.Helper()
	if rankOf(results, wantPresent) == -1 {
		t.Errorf("%s: expected %s to be present, got %+v", surface, wantPresent, results)
	}
	for _, fid := range wantAbsent {
		if rankOf(results, fid) != -1 {
			t.Errorf("%s: %s must be excluded but was present in %+v", surface, fid, results)
		}
	}
}

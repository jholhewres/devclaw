// Package memory — sqlite_store_lifecycle_test.go covers US-005 read-side
// lifecycle guards: deleted_at / expires_at / curation_status='low_signal'
// chunks must never surface from SearchBM25, SearchVector, or
// HybridSearchWithOpts, while a normal chunk still returns.
package memory

import (
	"context"
	"testing"
)

// TestRecallExcludesLifecycleChunks seeds four files that all match the same
// query token, marks three of them with a disqualifying lifecycle state
// (soft-deleted, expired, low-signal), then asserts every recall surface
// drops the three and keeps the normal one.
func TestRecallExcludesLifecycleChunks(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	token := "lifecycle recall guard token"
	seedFileWithWing(t, store, "file_ok.md", token+" alpha", "")
	seedFileWithWing(t, store, "file_deleted.md", token+" beta", "")
	seedFileWithWing(t, store, "file_expired.md", token+" gamma", "")
	seedFileWithWing(t, store, "file_lowsig.md", token+" delta", "")

	// Apply lifecycle states directly via SQL (the write path that sets these
	// columns is owned by other stories; the recall guard is what we test).
	mustExec(t, store, `UPDATE chunks SET deleted_at = CURRENT_TIMESTAMP WHERE file_id = ?`, "file_deleted.md")
	mustExec(t, store, `UPDATE chunks SET expires_at = datetime(CURRENT_TIMESTAMP, '-1 day') WHERE file_id = ?`, "file_expired.md")
	mustExec(t, store, `UPDATE chunks SET curation_status = 'low_signal' WHERE file_id = ?`, "file_lowsig.md")

	// SearchVector scores the in-memory cache, so reload it to reflect the
	// lifecycle updates (loadVectorCache applies the same guard).
	if err := store.loadVectorCache(); err != nil {
		t.Fatalf("reload vector cache: %v", err)
	}

	excluded := []string{"file_deleted.md", "file_expired.md", "file_lowsig.md"}

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

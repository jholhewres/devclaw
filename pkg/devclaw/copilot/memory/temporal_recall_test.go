// Package memory — temporal_recall_test.go proves the US-003 date-aware recall
// end to end: seeding chunks across several days (via occurred_at) and asserting
// that a query carrying a temporal cue ("o que rolou na sexta") returns only the
// matching day's chunk, while a non-temporal query is unaffected by any window.
package memory

import (
	"context"
	"testing"
	"time"
)

// setOccurredAt stamps every chunk of a file with a local-midday instant on the
// given day, then reloads the vector cache so the in-memory path observes it.
// Mirrors how the lifecycle tests apply out-of-band column updates.
func setOccurredAt(t *testing.T, store *SQLiteStore, fileID string, day time.Time) {
	t.Helper()
	// Store a real local instant (matches US-001's time.Local stamping). Use
	// midday so it sits well inside the [midnight, nextMidnight) day window.
	ts := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.Local)
	mustExec(t, store, `UPDATE chunks SET occurred_at = ? WHERE file_id = ?`, ts, fileID)
}

// TestTemporalRecallSelectsRightDay seeds four files across 06-17..06-21 and a
// non-dated relevance baseline, then asserts a "na sexta" query (resolved
// against Sunday 2026-06-21) returns only the Friday 06-19 chunk.
func TestTemporalRecallSelectsRightDay(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Shared token so all dated files match the same BM25/vector query terms;
	// only occurred_at distinguishes them.
	token := "reuniao planejamento sprint"
	seedFileWithWing(t, store, "wed.md", token+" quarta", "")
	seedFileWithWing(t, store, "thu.md", token+" quinta", "")
	seedFileWithWing(t, store, "fri.md", token+" sexta", "")
	seedFileWithWing(t, store, "today.md", token+" domingo", "")

	setOccurredAt(t, store, "wed.md", localDay(2026, 6, 17))
	setOccurredAt(t, store, "thu.md", localDay(2026, 6, 18))
	setOccurredAt(t, store, "fri.md", localDay(2026, 6, 19))
	setOccurredAt(t, store, "today.md", localDay(2026, 6, 21))

	// Reload so the in-memory vector cache picks up the occurred_at stamps.
	if err := store.loadVectorCache(); err != nil {
		t.Fatalf("reload vector cache: %v", err)
	}

	now := fixedNow() // Sunday 2026-06-21
	from, to, ok := resolveTemporalWindow("o que rolou na sexta", now)
	if !ok {
		t.Fatalf("expected temporal cue to resolve for 'na sexta'")
	}

	results, err := store.HybridSearchWithOpts(ctx, "o que rolou "+token+" na sexta", HybridSearchOptions{
		MaxResults:   10,
		MinScore:     0.0001,
		VectorWeight: 0.7,
		BM25Weight:   0.3,
		OccurredFrom: &from,
		OccurredTo:   &to,
	})
	if err != nil {
		t.Fatalf("HybridSearchWithOpts: %v", err)
	}

	if rankOf(results, "fri.md") == -1 {
		t.Errorf("expected Friday 06-19 chunk (fri.md) to be present, got %+v", results)
	}
	for _, other := range []string{"wed.md", "thu.md", "today.md"} {
		if rankOf(results, other) != -1 {
			t.Errorf("%s is outside the Friday window and must be excluded, got %+v", other, results)
		}
	}
}

// TestTemporalRecallExcludesNullOccurred confirms a chunk with NULL occurred_at
// (no known instant) is excluded from a day-scoped window.
func TestTemporalRecallExcludesNullOccurred(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	token := "evento sem data conhecida"
	seedFileWithWing(t, store, "dated.md", token+" sexta", "")
	seedFileWithWing(t, store, "undated.md", token+" qualquer", "") // occurred_at stays NULL

	setOccurredAt(t, store, "dated.md", localDay(2026, 6, 19))
	if err := store.loadVectorCache(); err != nil {
		t.Fatalf("reload vector cache: %v", err)
	}

	from, to, ok := resolveTemporalWindow("na sexta", fixedNow())
	if !ok {
		t.Fatalf("expected cue to resolve")
	}

	results, err := store.HybridSearchWithOpts(ctx, token, HybridSearchOptions{
		MaxResults:   10,
		MinScore:     0.0001,
		VectorWeight: 0.7,
		BM25Weight:   0.3,
		OccurredFrom: &from,
		OccurredTo:   &to,
	})
	if err != nil {
		t.Fatalf("HybridSearchWithOpts: %v", err)
	}
	if rankOf(results, "dated.md") == -1 {
		t.Errorf("dated chunk inside window must be present, got %+v", results)
	}
	if rankOf(results, "undated.md") != -1 {
		t.Errorf("NULL-occurred chunk must be excluded from a day window, got %+v", results)
	}
}

// TestNonTemporalQueryUnaffected confirms a query with NO window returns the
// most relevant chunk regardless of occurred_at — i.e. normal recall is intact.
func TestNonTemporalQueryUnaffected(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// One on-topic file and a few off-topic dated decoys.
	seedFileWithWing(t, store, "proposta.md", "proposta ISCB orçamento aprovado", "")
	seedFileWithWing(t, store, "decoy1.md", "lista de compras semanal", "")
	seedFileWithWing(t, store, "decoy2.md", "configuração do servidor nginx", "")

	setOccurredAt(t, store, "proposta.md", localDay(2026, 6, 10))
	setOccurredAt(t, store, "decoy1.md", localDay(2026, 6, 19))
	setOccurredAt(t, store, "decoy2.md", localDay(2026, 6, 19))
	if err := store.loadVectorCache(); err != nil {
		t.Fatalf("reload vector cache: %v", err)
	}

	// No temporal cue → no window should be derived.
	if _, _, ok := resolveTemporalWindow("proposta ISCB", fixedNow()); ok {
		t.Fatalf("non-temporal query must NOT resolve a window")
	}

	// Search WITHOUT a window (mirrors the agent path for a non-temporal query).
	results, err := store.HybridSearchWithOpts(ctx, "proposta ISCB", HybridSearchOptions{
		MaxResults:   10,
		MinScore:     0.0001,
		VectorWeight: 0.7,
		BM25Weight:   0.3,
	})
	if err != nil {
		t.Fatalf("HybridSearchWithOpts: %v", err)
	}
	// The relevant file must surface (its BM25 term match for "proposta ISCB"
	// is exact). Without a window, occurred_at plays no role — so chunks from
	// other days remain eligible too, proving no date filtering leaked in.
	if rankOf(results, "proposta.md") == -1 {
		t.Errorf("expected proposta.md present (relevance, unaffected by dates), got %+v", results)
	}
	if len(results) < 2 {
		t.Errorf("non-temporal query must not filter by occurred_at; expected decoys from other days to remain eligible, got %+v", results)
	}
	// Among the candidates, the exact-term match must beat the off-topic decoys
	// on the BM25 branch. Assert proposta.md outranks at least one dated decoy.
	if pr, dr := rankOf(results, "proposta.md"), rankOf(results, "decoy2.md"); dr != -1 && pr > dr {
		t.Errorf("proposta.md (rank %d) should outrank off-topic decoy2.md (rank %d): %+v", pr, dr, results)
	}
}

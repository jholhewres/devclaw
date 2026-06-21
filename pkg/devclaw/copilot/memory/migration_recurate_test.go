// Package memory — migration_recurate_test.go covers the v1.22.1 boot-time
// re-curation: chunks scored by an older (miscalibrated) scorer are re-judged
// with the current scorer so genuine facts demoted to low_signal become
// recallable again, while real noise stays low_signal. Idempotent.
package memory

import (
	"context"
	"log/slog"
	"testing"
)

// seedRawChunk inserts a chunk directly via SQL with explicit curation_status
// and scorer_version, bypassing the index/quality path so the test controls the
// "old scoring" state precisely.
func seedRawChunk(t *testing.T, store *SQLiteStore, fileID, text, kind, scope, curation string, scorerVersion int) {
	t.Helper()
	_, err := store.db.Exec(
		`INSERT INTO chunks (file_id, chunk_idx, text, hash, kind, scope, curation_status, curation_rule, scorer_version)
		 VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?)`,
		fileID, text, hashChunk(text), kind, scope, nullIfEmpty(curation), nullIfEmpty(curationRuleFor(curation)), scorerVersion,
	)
	if err != nil {
		t.Fatalf("seed raw chunk %s: %v", fileID, err)
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func curationRuleFor(status string) string {
	if status == CurationStatusLowSignal {
		return CurationRuleQuality
	}
	return ""
}

func chunkCuration(t *testing.T, store *SQLiteStore, fileID string) (status string, scorerVersion int) {
	t.Helper()
	var st any
	if err := store.db.QueryRow(
		`SELECT COALESCE(curation_status, ''), COALESCE(scorer_version, 0) FROM chunks WHERE file_id = ?`,
		fileID,
	).Scan(&st, &scorerVersion); err != nil {
		t.Fatalf("query curation for %s: %v", fileID, err)
	}
	if s, ok := st.(string); ok {
		status = s
	}
	return status, scorerVersion
}

// TestRecurateLowSignal_PromotesFactsKeepsNoise seeds chunks at an old scorer
// version — a genuine fact wrongly demoted to low_signal, plus real noise also
// at low_signal — and asserts recuration promotes the fact (clears status) and
// keeps the noise demoted. A second run is a no-op.
func TestRecurateLowSignal_PromotesFactsKeepsNoise(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const oldVersion = 3 // below QualityScorerVersion (4)

	// Genuine fact wrongly demoted by the old scorer (the real prod symptom).
	seedRawChunk(t, store, "fact_iscb.md",
		"As empresas envolvidas na proposta são ISCB e/ou Alta Forja.",
		"fact", "", CurationStatusLowSignal, oldVersion)

	// Real noise that must stay demoted.
	seedRawChunk(t, store, "noise_test.md",
		"go test ./... 12 passed", "fact", "", CurationStatusLowSignal, oldVersion)

	// A chunk already at the current version must be skipped (not rescored).
	seedRawChunk(t, store, "already_current.md",
		"this one is current", "fact", "", CurationStatusLowSignal, QualityScorerVersion)

	rescored, err := store.RecurateLowSignal(ctx, slog.Default())
	if err != nil {
		t.Fatalf("RecurateLowSignal: %v", err)
	}
	if rescored != 2 {
		t.Fatalf("expected 2 sub-version chunks rescored, got %d", rescored)
	}

	// The genuine fact is now recallable (curation cleared) and stamped current.
	if status, ver := chunkCuration(t, store, "fact_iscb.md"); status != "" {
		t.Errorf("fact must be promoted to recallable, got status %q", status)
	} else if ver != QualityScorerVersion {
		t.Errorf("fact scorer_version must be %d, got %d", QualityScorerVersion, ver)
	}

	// The noise stays low_signal, also stamped current.
	if status, ver := chunkCuration(t, store, "noise_test.md"); status != CurationStatusLowSignal {
		t.Errorf("noise must stay low_signal, got status %q", status)
	} else if ver != QualityScorerVersion {
		t.Errorf("noise scorer_version must be %d, got %d", QualityScorerVersion, ver)
	}

	// The already-current chunk was untouched (still low_signal, still current).
	if status, ver := chunkCuration(t, store, "already_current.md"); status != CurationStatusLowSignal || ver != QualityScorerVersion {
		t.Errorf("already-current chunk must be skipped; got status %q ver %d", status, ver)
	}

	// Idempotency: a second run rescans nothing.
	rescored2, err := store.RecurateLowSignal(ctx, slog.Default())
	if err != nil {
		t.Fatalf("RecurateLowSignal (2nd run): %v", err)
	}
	if rescored2 != 0 {
		t.Fatalf("second run must rescore 0 (idempotent), got %d", rescored2)
	}
}

package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// seedImportedChunkWithWrongDate writes a single imported-style chunk for fact
// at file_id = importedFileIDPrefix+importHash(TrimSpace(fact)), stamping its
// occurred_at to wrongDate (simulating a pre-US-001 import that recorded the
// migration date instead of the memory's real original date). It mirrors the
// import keying exactly so BackfillOccurredAt can match it.
func seedImportedChunkWithWrongDate(t *testing.T, store *SQLiteStore, fact string, wrongDate time.Time) string {
	t.Helper()
	key := importHash(strings.TrimSpace(fact))
	fileID := importedFileIDPrefix + key

	if _, err := store.db.Exec(
		`INSERT INTO files (file_id, hash, indexed_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
		fileID, key,
	); err != nil {
		t.Fatalf("seed file row: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO chunks (file_id, chunk_idx, text, hash, occurred_at) VALUES (?, 0, ?, ?, ?)`,
		fileID, fact, key, wrongDate,
	); err != nil {
		t.Fatalf("seed chunk row: %v", err)
	}
	return fileID
}

// TestBackfillOccurredAt_RestampsFromMarkdown is the US-002 acceptance test: an
// already-imported chunk stamped with the wrong (migration) date is restamped
// to the original date parsed from the .md file, the pass is idempotent, and an
// unrelated chunk with no matching .md entry is left untouched.
func TestBackfillOccurredAt_RestampsFromMarkdown(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dir := t.TempDir()

	// A durable fact whose .md line carries the real original date 2026-06-18 16:39.
	fact := "The temporal recall backfill restamps occurred_at from the untouched markdown files on the next boot."
	// The orphan chunk has NO matching .md line; its occurred_at must not change.
	orphan := "This orphan memory has no corresponding line in any markdown file so the backfill leaves it alone."

	// Wrong/migration date that the pre-US-001 import would have recorded.
	wrongDate := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	seedImportedChunkWithWrongDate(t, store, fact, wrongDate)
	orphanFileID := seedImportedChunkWithWrongDate(t, store, orphan, wrongDate)

	// Write a MEMORY.md whose matching line is dated [2026-06-18 16:39].
	memFile := filepath.Join(dir, MemoryFileName)
	content := fmt.Sprintf("- [2026-06-18 16:39] [decision] %s\n", fact)
	if err := os.WriteFile(memFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}
	before, err := os.ReadFile(memFile)
	if err != nil {
		t.Fatalf("snapshot MEMORY.md: %v", err)
	}

	// ── First run: restamps exactly the one matching chunk ──
	updated, err := store.BackfillOccurredAt(ctx, dir, nil)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 chunk restamped, got %d", updated)
	}

	// occurred_at is parsed from the .md stamp in time.Local (see parseMemoryFile),
	// so the expected instant is the same wall clock in the local zone — matching
	// how the import/backfill store it and how US-003 date windows compare.
	want := time.Date(2026, 6, 18, 16, 39, 0, 0, time.Local)
	factKey := importHash(strings.TrimSpace(fact))
	var got time.Time
	if err := store.db.QueryRow(
		"SELECT occurred_at FROM chunks WHERE file_id = ?", importedFileIDPrefix+factKey,
	).Scan(&got); err != nil {
		t.Fatalf("scan restamped occurred_at: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("occurred_at = %v, want %v (original .md date must be restored)", got, want)
	}

	// ── Orphan chunk (no .md match) is left untouched ──
	var orphanGot time.Time
	if err := store.db.QueryRow(
		"SELECT occurred_at FROM chunks WHERE file_id = ?", orphanFileID,
	).Scan(&orphanGot); err != nil {
		t.Fatalf("scan orphan occurred_at: %v", err)
	}
	if !orphanGot.Equal(wrongDate) {
		t.Fatalf("orphan occurred_at = %v, want unchanged %v", orphanGot, wrongDate)
	}

	// ── .md must be byte-identical (read-only) ──
	after, err := os.ReadFile(memFile)
	if err != nil {
		t.Fatalf("re-read MEMORY.md: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("MEMORY.md bytes changed during backfill — files must be read-only")
	}

	// ── Idempotency: second run updates nothing ──
	updated2, err := store.BackfillOccurredAt(ctx, dir, nil)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if updated2 != 0 {
		t.Fatalf("second run must be a no-op, got %d updated", updated2)
	}
}

// TestBackfillOccurredAt_VersionGate verifies the pass claims user_version 4 on
// a successful run and is a permanent no-op afterward, even when the .md still
// contains a not-yet-applied correction (gate wins).
func TestBackfillOccurredAt_VersionGate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dir := t.TempDir()

	fact := "The version gate makes the occurred_at backfill a permanent no-op once user_version reaches four."
	wrongDate := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	seedImportedChunkWithWrongDate(t, store, fact, wrongDate)

	memFile := filepath.Join(dir, MemoryFileName)
	if err := os.WriteFile(memFile, []byte(fmt.Sprintf("- [2026-06-18 16:39] [decision] %s\n", fact)), 0o600); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	if _, err := store.BackfillOccurredAt(ctx, dir, nil); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var userVersion int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&userVersion); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if userVersion != backfillOccurredAtVersion {
		t.Fatalf("user_version = %d, want %d after successful backfill", userVersion, backfillOccurredAtVersion)
	}

	// Mutate the .md to a different date; the gate must still short-circuit so
	// the chunk is NOT re-restamped.
	if err := os.WriteFile(memFile, []byte(fmt.Sprintf("- [2020-01-01 00:00] [decision] %s\n", fact)), 0o600); err != nil {
		t.Fatalf("rewrite MEMORY.md: %v", err)
	}

	updated, err := store.BackfillOccurredAt(ctx, dir, nil)
	if err != nil {
		t.Fatalf("gated backfill: %v", err)
	}
	if updated != 0 {
		t.Fatalf("version-gated run must be a no-op, got %d updated", updated)
	}

	factKey := importHash(strings.TrimSpace(fact))
	var got time.Time
	if err := store.db.QueryRow(
		"SELECT occurred_at FROM chunks WHERE file_id = ?", importedFileIDPrefix+factKey,
	).Scan(&got); err != nil {
		t.Fatalf("scan occurred_at: %v", err)
	}
	// occurred_at is parsed from the .md stamp in time.Local (see parseMemoryFile),
	// so the expected instant is the same wall clock in the local zone — matching
	// how the import/backfill store it and how US-003 date windows compare.
	want := time.Date(2026, 6, 18, 16, 39, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("occurred_at = %v, want %v (gate must prevent the second restamp)", got, want)
	}
}

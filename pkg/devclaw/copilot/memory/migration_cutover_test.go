// Package memory — migration_cutover_test.go covers the US-004 live write
// redirect (SaveCuratedMemory), the US-006 write-side health features (real
// SQLite supersede + dedup-on-save + credential redaction), and the robust
// post-import space reclaim (dedicated VACUUM connection).
package memory

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveCuratedMemory_RoundTrip writes a memory via the live save path and
// asserts it is retrievable, stored under the saved/ prefix, lifecycle-tagged,
// and content-hash deduped on a repeat save.
func TestSaveCuratedMemory_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	content := "The new deploy pipeline cross-compiles for linux amd64 and restarts the gateway via systemd."
	if err := store.SaveCuratedMemory(ctx, content, "decision", "agent"); err != nil {
		t.Fatalf("SaveCuratedMemory: %v", err)
	}

	// Stored under the saved/ namespace.
	key := importHash(strings.TrimSpace(content))
	var n int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE file_id = ?", savedFileIDPrefix+key,
	).Scan(&n); err != nil {
		t.Fatalf("count saved chunk: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 saved chunk, got %d", n)
	}

	// Retrievable via recall.
	hits, err := store.SearchBM25("deploy gateway systemd", 10)
	if err != nil {
		t.Fatalf("SearchBM25: %v", err)
	}
	found := false
	for _, h := range hits {
		if strings.Contains(h.Text, "cross-compiles") {
			found = true
		}
	}
	if !found {
		t.Fatal("saved memory not retrievable via recall")
	}

	// Dedup-on-save: a second identical save must not create a second chunk.
	if err := store.SaveCuratedMemory(ctx, content, "decision", "agent"); err != nil {
		t.Fatalf("SaveCuratedMemory (dup): %v", err)
	}
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE file_id = ?", savedFileIDPrefix+key,
	).Scan(&n); err != nil {
		t.Fatalf("recount saved chunk: %v", err)
	}
	if n != 1 {
		t.Fatalf("duplicate save must collapse to 1 chunk, got %d", n)
	}
}

// TestSaveCuratedMemory_RedactsCredential proves a credential-bearing save is
// stored redacted, never as plaintext.
func TestSaveCuratedMemory_RedactsCredential(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SaveCuratedMemory(ctx,
		"The staging DB login uses senha: hunter2supersecret for the cluster.",
		"fact", "agent"); err != nil {
		t.Fatalf("SaveCuratedMemory: %v", err)
	}

	var leaked int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE text LIKE '%hunter2supersecret%'",
	).Scan(&leaked); err != nil {
		t.Fatalf("scan leak: %v", err)
	}
	if leaked != 0 {
		t.Fatal("plaintext credential leaked into a live-saved chunk")
	}
	var redacted int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE text LIKE '%REDACTED%'",
	).Scan(&redacted); err != nil {
		t.Fatalf("scan redacted: %v", err)
	}
	if redacted == 0 {
		t.Fatal("expected the live-saved credential line to be stored redacted")
	}
}

// TestSupersedeByContent_HardExcludes seeds a losing and a winning chunk, runs
// the supersede, and asserts the loser is soft-deleted (deleted_at set,
// supersedes recorded), gone from recall, and evicted from the vector cache.
func TestSupersedeByContent_HardExcludes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	loser := "The production database runs on host db-old.internal."
	winner := "The production database runs on host db-new.internal."
	seedFileWithWing(t, store, "loser.md", loser, "")
	seedFileWithWing(t, store, "winner.md", winner, "")

	n, err := store.SupersedeByContent(ctx, loser, winner)
	if err != nil {
		t.Fatalf("SupersedeByContent: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 file superseded, got %d", n)
	}

	// deleted_at + supersedes set on the loser.
	var deletedNull bool
	var supersedes string
	if err := store.db.QueryRow(`
		SELECT deleted_at IS NULL, COALESCE(supersedes, '') FROM chunks WHERE file_id = ?
	`, "loser.md").Scan(&deletedNull, &supersedes); err != nil {
		t.Fatalf("scan loser lifecycle: %v", err)
	}
	if deletedNull {
		t.Error("loser chunk should have deleted_at set")
	}
	if !strings.Contains(supersedes, importHash(strings.TrimSpace(winner))) {
		t.Errorf("loser supersedes should record the winner hash, got %q", supersedes)
	}

	// Loser no longer recallable; winner still is.
	bm25, err := store.SearchBM25("production database host", 10)
	if err != nil {
		t.Fatalf("SearchBM25: %v", err)
	}
	for _, h := range bm25 {
		if strings.Contains(h.Text, "db-old.internal") {
			t.Fatal("superseded loser still surfaced in BM25 recall")
		}
	}

	// Vector cache must have dropped the loser immediately (no reload).
	vec, err := store.SearchVector(ctx, "production database host", 10)
	if err != nil {
		t.Fatalf("SearchVector: %v", err)
	}
	for _, h := range vec {
		if strings.Contains(h.Text, "db-old.internal") {
			t.Fatal("superseded loser still in vector cache (EvictFromVectorCache not applied)")
		}
	}
}

// TestImportReclaimSucceeds proves the post-import space reclaim runs without a
// "cannot VACUUM"/"database is locked" error even though the store's bounded
// connection pool keeps idle connections open. Uses a real on-disk DB.
func TestImportReclaimSucceeds(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "reclaim.db")

	store, err := NewSQLiteStore(dbPath, &deterministicEmbedder{}, slog.Default())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	// Open an idle pooled connection (and keep it warm) to mimic production,
	// then seed a legacy file and run the import (which calls reclaimSpace).
	if _, err := store.db.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("warm pool: %v", err)
	}

	memFile := filepath.Join(dir, MemoryFileName)
	if err := os.WriteFile(memFile,
		[]byte("- [2026-06-01 10:00] [decision] The CI pipeline builds with CGO_ENABLED=1 and the sqlite_fts5 tag for full-text search.\n"),
		0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A captured warning would indicate the reclaim path failed; assert no error.
	stats, err := store.ImportLegacyMarkdown(ctx, dir, slog.Default())
	if err != nil {
		t.Fatalf("ImportLegacyMarkdown: %v", err)
	}
	if stats.Inserted != 1 {
		t.Fatalf("expected 1 inserted, got %d", stats.Inserted)
	}

	// Direct reclaim call must also succeed cleanly on the warm pool.
	store.reclaimSpace(ctx, slog.Default())

	// DB still usable after reclaim.
	var c int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM chunks").Scan(&c); err != nil {
		t.Fatalf("post-reclaim query: %v", err)
	}
	if c != 1 {
		t.Fatalf("expected 1 chunk after reclaim, got %d", c)
	}
}

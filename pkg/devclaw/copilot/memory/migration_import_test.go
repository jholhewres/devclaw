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

// seedLegacyMemoryDir writes a MEMORY.md with a mix of normal facts,
// contradictions, exact duplicates, and a credential line, then returns the
// directory and the file path. The lines use the exact format parseMemoryFile
// recognizes: "- [YYYY-MM-DD HH:MM] [category] content".
func seedLegacyMemoryDir(t *testing.T) (dir, memFile string) {
	t.Helper()
	dir = t.TempDir()
	memFile = filepath.Join(dir, MemoryFileName)

	var b strings.Builder

	// 3 normal, durable facts (long enough + scoped category to clear the bar).
	normals := []string{
		"The production gateway runs on the openclaw-gateway VM and is restarted via systemd after each deploy build.",
		"DevClaw stores all secrets in the encrypted .devclaw.vault file in the project root, never in .env or config.yaml.",
		"The memory store uses SQLite with FTS5 and an in-memory vector cache loaded at startup for fast cosine recall.",
	}
	for _, n := range normals {
		fmt.Fprintf(&b, "- [2026-06-01 10:00] [decision] %s\n", n)
	}

	// 20 contradiction summary lines (bloat — must be dropped).
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "- [2026-06-02 11:%02d] [summary] [Contradiction] earlier note conflicts with later note number %d about the deploy target host\n", i, i)
	}

	// 2 exact-duplicate facts (must collapse to 1).
	dup := "The CI pipeline builds the Go binary with CGO_ENABLED=1 and the sqlite_fts5 build tag for full-text search support."
	fmt.Fprintf(&b, "- [2026-06-03 09:00] [fact] %s\n", dup)
	fmt.Fprintf(&b, "- [2026-06-04 09:00] [fact] %s\n", dup)

	// 1 credential line (must be stored redacted, never plaintext).
	fmt.Fprintf(&b, "- [2026-06-05 12:00] [fact] Database admin login uses senha: hunter2supersecret for the staging cluster only.\n")

	if err := os.WriteFile(memFile, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("seed MEMORY.md: %v", err)
	}
	return dir, memFile
}

func TestImportLegacyMarkdown_Curation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dir, memFile := seedLegacyMemoryDir(t)

	// Snapshot the .md bytes to prove the import never edits them.
	before, err := os.ReadFile(memFile)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	stats, err := store.ImportLegacyMarkdown(ctx, dir, nil)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// ── Stats sanity ──
	if stats.AlreadyImported {
		t.Fatal("first run must not report AlreadyImported")
	}
	if stats.ContradictionsDropped != 20 {
		t.Errorf("expected 20 contradictions dropped, got %d", stats.ContradictionsDropped)
	}
	if stats.DuplicatesDropped != 1 {
		t.Errorf("expected 1 duplicate dropped, got %d", stats.DuplicatesDropped)
	}
	// 3 normals + 1 collapsed dup + 1 credential = 5 inserted.
	if stats.Inserted != 5 {
		t.Errorf("expected 5 inserted, got %d", stats.Inserted)
	}

	// ── .md file must be byte-identical after import ──
	after, err := os.ReadFile(memFile)
	if err != nil {
		t.Fatalf("re-read seed: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("MEMORY.md bytes changed during import — files must be read-only")
	}

	// ── Contradictions must NOT be recallable ──
	hits, err := store.SearchBM25("Contradiction conflicts deploy target host", 50)
	if err != nil {
		t.Fatalf("search contradictions: %v", err)
	}
	for _, h := range hits {
		if strings.Contains(strings.ToLower(h.Text), "[contradiction]") {
			t.Fatalf("contradiction surfaced in recall: %q", h.Text)
		}
	}

	// ── Duplicate fact collapsed to exactly one stored chunk ──
	dup := "The CI pipeline builds the Go binary with CGO_ENABLED=1 and the sqlite_fts5 build tag for full-text search support."
	dupKey := importHash(strings.TrimSpace(dup))
	var dupCount int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE file_id = ?", importedFileIDPrefix+dupKey,
	).Scan(&dupCount); err != nil {
		t.Fatalf("count dup: %v", err)
	}
	if dupCount != 1 {
		t.Errorf("duplicate fact should collapse to 1 chunk, got %d", dupCount)
	}

	// ── Credential stored REDACTED, no plaintext secret anywhere ──
	var leaked int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE text LIKE '%hunter2supersecret%'",
	).Scan(&leaked); err != nil {
		t.Fatalf("scan credential leak: %v", err)
	}
	if leaked != 0 {
		t.Fatal("plaintext credential leaked into stored chunks")
	}
	var redacted int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE text LIKE '%REDACTED%'",
	).Scan(&redacted); err != nil {
		t.Fatalf("scan redacted: %v", err)
	}
	if redacted == 0 {
		t.Fatal("expected the credential line to be stored with a redaction marker")
	}

	// ── A normal fact is present with lifecycle metadata populated ──
	var kind string
	var expiresNull bool
	row := store.db.QueryRow(`
		SELECT kind, expires_at IS NULL
		FROM chunks
		WHERE text LIKE '%openclaw-gateway VM%'
		LIMIT 1
	`)
	if err := row.Scan(&kind, &expiresNull); err != nil {
		t.Fatalf("scan normal fact metadata: %v", err)
	}
	if kind != "decision" {
		t.Errorf("expected kind=decision, got %q", kind)
	}
	if !expiresNull {
		t.Error("durable decision should have NULL expires_at")
	}

	// ── Idempotency: a second run is a no-op ──
	stats2, err := store.ImportLegacyMarkdown(ctx, dir, nil)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if !stats2.AlreadyImported {
		t.Error("second run must report AlreadyImported")
	}
	if stats2.Inserted != 0 {
		t.Errorf("second run must insert nothing, got %d", stats2.Inserted)
	}

	// Total imported chunks unchanged after the no-op second run.
	var total int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE file_id LIKE ?", importedFileIDPrefix+"%",
	).Scan(&total); err != nil {
		t.Fatalf("count total imported: %v", err)
	}
	if total != 5 {
		t.Errorf("expected 5 imported chunks after idempotent re-run, got %d", total)
	}
}

func TestImportLegacyMarkdown_EventGetsTTL(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dir := t.TempDir()
	memFile := filepath.Join(dir, MemoryFileName)

	content := "- [2026-06-01 10:00] [event] Restarted the gateway service after the nightly deploy completed successfully on the VM.\n"
	if err := os.WriteFile(memFile, []byte(content), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := store.ImportLegacyMarkdown(ctx, dir, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	var expiresNull bool
	if err := store.db.QueryRow(`
		SELECT expires_at IS NULL FROM chunks WHERE text LIKE '%Restarted the gateway%' LIMIT 1
	`).Scan(&expiresNull); err != nil {
		t.Fatalf("scan event expiry: %v", err)
	}
	if expiresNull {
		t.Error("event entry without explicit TTL should get a default expires_at")
	}
}

// TestImportLegacyMarkdown_PreservesOccurredAt is the US-001 acceptance test:
// an imported chunk's occurred_at must carry the memory's ORIGINAL event date
// (parsed from the [YYYY-MM-DD HH:MM] line), NOT the import/migration date that
// created_at records. This is what makes temporal recall ("what happened
// Thursday") possible after the v2 import.
func TestImportLegacyMarkdown_PreservesOccurredAt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dir := t.TempDir()
	memFile := filepath.Join(dir, MemoryFileName)

	content := "- [2026-06-18 16:39] [fact] The deploy on Thursday shipped the temporal recall column to the production gateway VM cleanly.\n"
	if err := os.WriteFile(memFile, []byte(content), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := store.ImportLegacyMarkdown(ctx, dir, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	var occurredAt time.Time
	if err := store.db.QueryRow(`
		SELECT occurred_at FROM chunks WHERE text LIKE '%deploy on Thursday%' LIMIT 1
	`).Scan(&occurredAt); err != nil {
		t.Fatalf("scan occurred_at: %v", err)
	}

	// The original date must be preserved.
	wantY, wantM, wantD := 2026, time.June, 18
	gotY, gotM, gotD := occurredAt.Date()
	if gotY != wantY || gotM != wantM || gotD != wantD {
		t.Fatalf("occurred_at = %v, want date %04d-%02d-%02d (original timestamp must be preserved)",
			occurredAt, wantY, wantM, wantD)
	}

	// And it must NOT be today's import date.
	ty, tm, td := time.Now().UTC().Date()
	if gotY == ty && gotM == tm && gotD == td {
		t.Fatalf("occurred_at fell back to today's import date %v — original date was lost", occurredAt)
	}
}

// TestSaveCuratedMemory_SetsOccurredAtNow verifies the live save path stamps
// occurred_at ~ now (no original-date dimension exists for a fresh save).
func TestSaveCuratedMemory_SetsOccurredAtNow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	before := time.Now().UTC().Add(-2 * time.Minute)
	text := "The temporal recall feature stamps occurred_at to the wall clock on every live curated save into SQLite."
	if err := store.SaveCuratedMemory(ctx, text, "fact", "test"); err != nil {
		t.Fatalf("save: %v", err)
	}
	after := time.Now().UTC().Add(2 * time.Minute)

	var occurredAt time.Time
	if err := store.db.QueryRow(`
		SELECT occurred_at FROM chunks WHERE text LIKE '%temporal recall feature stamps%' LIMIT 1
	`).Scan(&occurredAt); err != nil {
		t.Fatalf("scan occurred_at: %v", err)
	}
	if occurredAt.IsZero() {
		t.Fatal("live save left occurred_at unset")
	}
	if occurredAt.Before(before) || occurredAt.After(after) {
		t.Fatalf("occurred_at = %v, want within [%v, %v] (≈ now)", occurredAt, before, after)
	}
}

// TestImportLegacyMarkdown_OccurredAtFallsBackWhenNoTimestamp verifies that an
// entry with no parseable [timestamp] still gets occurred_at populated (to NOW,
// matching created_at) rather than NULL — the column is never spuriously empty.
func TestImportLegacyMarkdown_OccurredAtFallsBackWhenNoTimestamp(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dir := t.TempDir()
	memFile := filepath.Join(dir, MemoryFileName)

	// No leading [YYYY-MM-DD HH:MM] — parseMemoryFile leaves Timestamp zero.
	content := "- [fact] A timestamp-less durable fact about the SQLite memory store using FTS5 and an in-memory vector cache for recall.\n"
	if err := os.WriteFile(memFile, []byte(content), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	before := time.Now().UTC().Add(-2 * time.Minute)
	if _, err := store.ImportLegacyMarkdown(ctx, dir, nil); err != nil {
		t.Fatalf("import: %v", err)
	}
	after := time.Now().UTC().Add(2 * time.Minute)

	var occurredAt time.Time
	if err := store.db.QueryRow(`
		SELECT occurred_at FROM chunks WHERE text LIKE '%timestamp-less durable fact%' LIMIT 1
	`).Scan(&occurredAt); err != nil {
		t.Fatalf("scan occurred_at: %v", err)
	}
	if occurredAt.IsZero() {
		t.Fatal("entry without a timestamp must fall back to NOW, not NULL occurred_at")
	}
	if occurredAt.Before(before) || occurredAt.After(after) {
		t.Fatalf("fallback occurred_at = %v, want ≈ now within [%v, %v]", occurredAt, before, after)
	}
}

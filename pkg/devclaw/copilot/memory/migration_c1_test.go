// Package memory — migration_c1_test.go proves the C1 fix: the RAW chunks that
// IndexMemoryDir writes for the legacy flat-markdown files (file_id = bare
// basename, NULL lifecycle columns, un-redacted text) are deleted after the
// curated import runs, leaving only curated/redacted chunks recallable.
package memory

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestC1_RawLegacyChunksDeletedAfterImport indexes a legacy dir raw (via
// IndexMemoryDir), runs the curated import (ImportLegacyMarkdown), then
// DeleteRawLegacyChunks, and asserts:
//  1. no chunk keyed by a bare legacy basename (file_id NOT LIKE 'memory/imported/%')
//     survives for the legacy files, and
//  2. a credential string present in the raw .md is NOT recallable via SearchBM25
//     (only the curated, redacted copy remains).
func TestC1_RawLegacyChunksDeletedAfterImport(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dir := t.TempDir()

	// A legacy MEMORY.md whose entry carries a plaintext credential. The raw
	// IndexMemoryDir pass stores it verbatim; only curateEntry redacts it.
	const secret = "hunter2supersecretvalue"
	memFile := filepath.Join(dir, MemoryFileName)
	line := "- [2026-06-01 10:00] [decision] The staging cluster login uses senha: " + secret + " for bootstrap.\n"
	if err := os.WriteFile(memFile, []byte(line), 0o600); err != nil {
		t.Fatalf("seed MEMORY.md: %v", err)
	}

	chunkCfg := ChunkConfig{MaxTokens: 500, Overlap: 100}

	// 1. Raw index pass (first boot): writes the credential VERBATIM under the
	// bare basename file_id with NULL lifecycle columns.
	if err := store.IndexMemoryDir(ctx, dir, chunkCfg); err != nil {
		t.Fatalf("IndexMemoryDir: %v", err)
	}

	// Sanity: the plaintext credential is present (and recallable) BEFORE cleanup,
	// confirming the raw pass is the leak vector C1 fixes.
	var rawLeak int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE file_id = ? AND text LIKE '%'||?||'%'",
		MemoryFileName, secret,
	).Scan(&rawLeak); err != nil {
		t.Fatalf("scan raw leak: %v", err)
	}
	if rawLeak == 0 {
		t.Fatal("precondition failed: raw IndexMemoryDir did not store the legacy file under its bare basename")
	}

	// 2. Curated import (redacts the credential, namespaces under memory/imported/).
	stats, err := store.ImportLegacyMarkdown(ctx, dir, slog.Default())
	if err != nil {
		t.Fatalf("ImportLegacyMarkdown: %v", err)
	}
	if stats.Inserted != 1 {
		t.Fatalf("expected 1 curated chunk inserted, got %d", stats.Inserted)
	}

	// 3. Delete the raw legacy chunks (what assistant.go does after a first import).
	deleted, err := store.DeleteRawLegacyChunks(ctx, dir)
	if err != nil {
		t.Fatalf("DeleteRawLegacyChunks: %v", err)
	}
	if deleted == 0 {
		t.Fatal("expected DeleteRawLegacyChunks to remove at least one raw chunk")
	}

	// Assert 1: no surviving chunk is keyed by a bare legacy basename — every
	// remaining chunk must live under the curated namespace.
	var rawSurvivors int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE file_id NOT LIKE ?",
		importedFileIDPrefix+"%",
	).Scan(&rawSurvivors); err != nil {
		t.Fatalf("scan raw survivors: %v", err)
	}
	if rawSurvivors != 0 {
		t.Fatalf("expected 0 non-curated chunks after cleanup, got %d", rawSurvivors)
	}

	// Assert 2: the plaintext credential is NOT recallable via BM25; only the
	// redacted curated copy remains.
	hits, err := store.SearchBM25("staging cluster login bootstrap", 10)
	if err != nil {
		t.Fatalf("SearchBM25: %v", err)
	}
	for _, h := range hits {
		if strings.Contains(h.Text, secret) {
			t.Fatalf("plaintext credential %q still recallable after cleanup", secret)
		}
	}

	// And the curated copy IS present, redacted.
	var redacted int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE file_id LIKE ? AND text LIKE '%REDACTED%'",
		importedFileIDPrefix+"%",
	).Scan(&redacted); err != nil {
		t.Fatalf("scan redacted curated: %v", err)
	}
	if redacted == 0 {
		t.Fatal("expected the curated chunk to retain a redacted credential marker")
	}

	// Belt-and-suspenders: the plaintext secret must not exist in ANY chunk row.
	var anyLeak int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE text LIKE '%'||?||'%'", secret,
	).Scan(&anyLeak); err != nil {
		t.Fatalf("scan any leak: %v", err)
	}
	if anyLeak != 0 {
		t.Fatalf("plaintext credential leaked in %d chunk(s) after cleanup", anyLeak)
	}
}

// TestC1_DeletesPathPrefixedRawChunks guards the exact production regression the
// VPS dry-run caught: prod indexed the legacy file under a PATH-PREFIXED file_id
// ("data/memory/MEMORY.md"), not a bare basename, so a basename-only delete left
// the un-redacted raw chunk (with the real password) recallable. DeleteRawLegacyChunks
// must remove ANY non-curated chunk regardless of file_id form.
func TestC1_DeletesPathPrefixedRawChunks(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	const secret = "081082se"

	// A raw chunk exactly as production stored it: path-prefixed file_id, NULL
	// lifecycle columns, un-redacted plaintext password.
	if _, err := store.db.Exec(
		"INSERT INTO chunks (file_id, chunk_idx, text, hash) VALUES (?, 0, ?, 'h1')",
		"data/memory/MEMORY.md", "User's password for integrabot.ai is "+secret,
	); err != nil {
		t.Fatalf("insert raw path-prefixed chunk: %v", err)
	}
	// A legitimate curated chunk that must SURVIVE.
	if _, err := store.db.Exec(
		"INSERT INTO chunks (file_id, chunk_idx, text, hash) VALUES (?, 0, ?, 'h2')",
		importedFileIDPrefix+"abc", "a normal curated memory about the gateway deploy",
	); err != nil {
		t.Fatalf("insert curated chunk: %v", err)
	}

	deleted, err := store.DeleteRawLegacyChunks(ctx, "/some/memory/dir")
	if err != nil {
		t.Fatalf("DeleteRawLegacyChunks: %v", err)
	}
	if deleted < 1 {
		t.Fatalf("expected the path-prefixed raw chunk to be deleted, got %d", deleted)
	}

	// The path-prefixed raw chunk (and its secret) must be gone.
	var leak int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE text LIKE '%'||?||'%'", secret,
	).Scan(&leak); err != nil {
		t.Fatalf("scan leak: %v", err)
	}
	if leak != 0 {
		t.Fatalf("path-prefixed raw chunk survived: %d leaking chunk(s)", leak)
	}

	// The curated chunk must remain.
	var curated int
	if err := store.db.QueryRow(
		"SELECT COUNT(1) FROM chunks WHERE file_id LIKE ?", importedFileIDPrefix+"%",
	).Scan(&curated); err != nil {
		t.Fatalf("scan curated survivor: %v", err)
	}
	if curated != 1 {
		t.Fatalf("expected the curated chunk to survive, got %d", curated)
	}
}

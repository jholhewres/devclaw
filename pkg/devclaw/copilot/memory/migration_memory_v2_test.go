package memory

import (
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// minimalChunksDDL mirrors the chunks table as created by SQLiteStore.initSchema
// (without the FTS5 mirror), enough to exercise the v2 lifecycle migration.
const minimalChunksDDL = `
	CREATE TABLE IF NOT EXISTS chunks (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id    TEXT NOT NULL,
		chunk_idx  INTEGER NOT NULL,
		text       TEXT NOT NULL,
		hash       TEXT NOT NULL,
		embedding  TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(file_id, chunk_idx)
	);
`

// expectedV2Columns are the 14 lifecycle columns the migration must add.
var expectedV2Columns = []string{
	"deleted_at", "expires_at", "supersedes", "curation_status", "curation_rule",
	"importance", "confidence", "memory_type", "kind", "scope",
	"injected_count", "used_count", "last_used_at", "scorer_version",
}

func chunkColumns(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(chunks)")
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	cols := make(map[string]bool)
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return cols
}

func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return v
}

func TestMigrateMemoryV2_CleanDB(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(minimalChunksDDL); err != nil {
		t.Fatalf("create chunks: %v", err)
	}

	if err := MigrateMemoryV2(db, slog.Default()); err != nil {
		t.Fatalf("MigrateMemoryV2: %v", err)
	}

	if v := userVersion(t, db); v != 2 {
		t.Fatalf("user_version = %d, want 2", v)
	}

	cols := chunkColumns(t, db)
	for _, c := range expectedV2Columns {
		if !cols[c] {
			t.Errorf("column %s missing after migration", c)
		}
	}
}

func TestMigrateMemoryV2_Idempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(minimalChunksDDL); err != nil {
		t.Fatalf("create chunks: %v", err)
	}

	if err := MigrateMemoryV2(db, slog.Default()); err != nil {
		t.Fatalf("first run: %v", err)
	}
	firstCols := len(chunkColumns(t, db))

	// Second run is a pure no-op (gated by user_version).
	if err := MigrateMemoryV2(db, slog.Default()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if err := MigrateMemoryV2(db, slog.Default()); err != nil {
		t.Fatalf("third run: %v", err)
	}

	if v := userVersion(t, db); v != 2 {
		t.Fatalf("user_version = %d, want 2 after repeats", v)
	}
	cols := chunkColumns(t, db)
	if got := len(cols); got != firstCols {
		t.Fatalf("column count changed: got %d, want %d", got, firstCols)
	}
	// Re-verify the 14 v2 columns are still present after the repeated no-op runs
	// (guards against a regression where the count is stable but the wrong columns).
	for _, c := range expectedV2Columns {
		if !cols[c] {
			t.Errorf("column %s missing after repeated runs", c)
		}
	}
}

func TestMigrateMemoryV2_LegacyUpgrade(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer db.Close()

	// Pre-create a chunks table without the new columns, user_version 0.
	if _, err := db.Exec(minimalChunksDDL); err != nil {
		t.Fatalf("create legacy chunks: %v", err)
	}
	if v := userVersion(t, db); v != 0 {
		t.Fatalf("expected fresh user_version 0, got %d", v)
	}

	// Insert legacy data that must survive the upgrade.
	if _, err := db.Exec(
		"INSERT INTO chunks (file_id, chunk_idx, text, hash) VALUES (?, ?, ?, ?)",
		"memory/2026-04-01.md", 0, "hello", "deadbeef",
	); err != nil {
		t.Fatalf("insert legacy chunk: %v", err)
	}

	if err := MigrateMemoryV2(db, slog.Default()); err != nil {
		t.Fatalf("MigrateMemoryV2 on legacy db: %v", err)
	}

	if v := userVersion(t, db); v != 2 {
		t.Fatalf("user_version = %d, want 2", v)
	}
	cols := chunkColumns(t, db)
	for _, c := range expectedV2Columns {
		if !cols[c] {
			t.Errorf("column %s missing after legacy upgrade", c)
		}
	}

	// Legacy data intact.
	var txt string
	if err := db.QueryRow("SELECT text FROM chunks WHERE file_id = ?", "memory/2026-04-01.md").Scan(&txt); err != nil {
		t.Fatalf("legacy row missing: %v", err)
	}
	if txt != "hello" {
		t.Fatalf("legacy data corrupted: got %q", txt)
	}
}

func TestMigrateMemoryV2_NilDB(t *testing.T) {
	err := MigrateMemoryV2(nil, slog.Default())
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

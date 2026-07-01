package memory

import (
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestMigrateKgSchema_CleanDB(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := MigrateKgSchema(db, slog.Default()); err != nil {
		t.Fatalf("MigrateKgSchema: %v", err)
	}

	tables := []string{"kg_entities", "kg_entity_aliases", "kg_predicates", "kg_triples"}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", tbl, err)
		}
	}
}

func TestMigrateKgSchema_Idempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 3; i++ {
		if err := MigrateKgSchema(db, slog.Default()); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
}

func TestMigrateKgSchema_LegacyUpgrade(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(v117CoreSchema); err != nil {
		t.Fatalf("apply v1.17.0 core schema: %v", err)
	}

	// Insert legacy data that must survive the upgrade.
	_, err = db.Exec(
		"INSERT INTO files (file_id, hash, indexed_at) VALUES (?, ?, CURRENT_TIMESTAMP)",
		"memory/2026-04-01.md", "deadbeef",
	)
	if err != nil {
		t.Fatalf("insert legacy file: %v", err)
	}

	// Run the KG migration on top of the legacy database.
	if err := MigrateKgSchema(db, slog.Default()); err != nil {
		t.Fatalf("MigrateKgSchema on legacy db: %v", err)
	}

	// Verify legacy data intact.
	var fid string
	err = db.QueryRow("SELECT file_id FROM files WHERE file_id = ?", "memory/2026-04-01.md").Scan(&fid)
	if err != nil {
		t.Fatalf("legacy row missing: %v", err)
	}
	if fid != "memory/2026-04-01.md" {
		t.Fatalf("legacy data corrupted: got %q", fid)
	}

	// Verify KG tables coexist.
	tables := []string{"kg_entities", "kg_entity_aliases", "kg_predicates", "kg_triples"}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Errorf("kg table %s not found after legacy upgrade: %v", tbl, err)
		}
	}
}

func TestDownKgSchema_LosslessCycle(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Up
	if err := MigrateKgSchema(db, slog.Default()); err != nil {
		t.Fatalf("up: %v", err)
	}

	// Down
	if err := downKgSchema(db); err != nil {
		t.Fatalf("down: %v", err)
	}

	// Up again — tables recreated without error.
	if err := MigrateKgSchema(db, slog.Default()); err != nil {
		t.Fatalf("up-again: %v", err)
	}

	tables := []string{"kg_entities", "kg_entity_aliases", "kg_predicates", "kg_triples"}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found after up-down-up cycle: %v", tbl, err)
		}
	}
}

func TestMigrateKgSchema_NilDB(t *testing.T) {
	err := MigrateKgSchema(nil, slog.Default())
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

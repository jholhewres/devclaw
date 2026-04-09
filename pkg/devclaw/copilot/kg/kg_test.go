package kg

import (
	"database/sql"
	"log/slog"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestKgSchema_AppliesOnCleanDB(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(KgSchema); err != nil {
		t.Fatalf("apply schema: %v", err)
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

func TestKgSchema_Idempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 3; i++ {
		if _, err := db.Exec(KgSchema); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
	}
}

func TestNewKG_SucceedsOnMigratedDB(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(KgSchema); err != nil {
		t.Fatal(err)
	}

	kgInstance, err := NewKG(db, slog.Default())
	if err != nil {
		t.Fatalf("NewKG: %v", err)
	}
	if kgInstance == nil {
		t.Fatal("expected non-nil KG")
	}
}

func TestNewKG_NilDB(t *testing.T) {
	_, err := NewKG(nil, slog.Default())
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestNewKG_NilLogger(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = NewKG(db, nil)
	if err == nil {
		t.Fatal("expected error for nil logger")
	}
}

func TestKgDropSchema_CleansUp(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Exec(KgSchema)
	db.Exec(KgDropSchema)

	tables := []string{"kg_entities", "kg_entity_aliases", "kg_predicates", "kg_triples"}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err == nil {
			t.Errorf("table %s should not exist after drop", tbl)
		}
	}
}

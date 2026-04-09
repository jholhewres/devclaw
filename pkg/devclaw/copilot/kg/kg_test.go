package kg

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newTestKG(t *testing.T) *KG {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(KgSchema); err != nil {
		t.Fatal(err)
	}
	kg, err := NewKG(db, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return kg
}

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

func TestKG_AddTriple_SimpleInsert(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	tid, err := k.AddTriple(ctx, "Maria", "works_at", "ACME", TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple: %v", err)
	}
	if tid <= 0 {
		t.Fatalf("expected positive triple_id, got %d", tid)
	}

	var validUntil sql.NullString
	err = k.db.QueryRow(
		"SELECT valid_until FROM kg_triples WHERE triple_id = ?", tid,
	).Scan(&validUntil)
	if err != nil {
		t.Fatalf("query triple: %v", err)
	}
	if validUntil.Valid {
		t.Error("expected valid_until to be NULL for new triple")
	}
}

func TestKG_AddTriple_FunctionalPredicateAutoInvalidates(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	_, err := k.EnsurePredicate(ctx, "works_at", true, "where someone works")
	if err != nil {
		t.Fatalf("EnsurePredicate: %v", err)
	}

	tid1, err := k.AddTriple(ctx, "Maria", "works_at", "ACME", TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple 1: %v", err)
	}

	tid2, err := k.AddTriple(ctx, "Maria", "works_at", "Globex", TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple 2: %v", err)
	}

	var vu1, vu2 sql.NullString
	if err := k.db.QueryRow("SELECT valid_until FROM kg_triples WHERE triple_id = ?", tid1).Scan(&vu1); err != nil {
		t.Fatal(err)
	}
	if err := k.db.QueryRow("SELECT valid_until FROM kg_triples WHERE triple_id = ?", tid2).Scan(&vu2); err != nil {
		t.Fatal(err)
	}

	if !vu1.Valid {
		t.Error("first triple should have valid_until set after functional replacement")
	}
	if vu2.Valid {
		t.Error("second triple should have valid_until NULL")
	}
}

func TestKG_AddTriple_NonFunctionalPredicateKeepsBoth(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	_, err := k.EnsurePredicate(ctx, "likes", false, "things someone likes")
	if err != nil {
		t.Fatalf("EnsurePredicate: %v", err)
	}

	tid1, err := k.AddTriple(ctx, "Maria", "likes", "coffee", TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple 1: %v", err)
	}
	tid2, err := k.AddTriple(ctx, "Maria", "likes", "tea", TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple 2: %v", err)
	}

	var vu1, vu2 sql.NullString
	if err := k.db.QueryRow("SELECT valid_until FROM kg_triples WHERE triple_id = ?", tid1).Scan(&vu1); err != nil {
		t.Fatal(err)
	}
	if err := k.db.QueryRow("SELECT valid_until FROM kg_triples WHERE triple_id = ?", tid2).Scan(&vu2); err != nil {
		t.Fatal(err)
	}

	if vu1.Valid {
		t.Error("first triple should have valid_until NULL for non-functional predicate")
	}
	if vu2.Valid {
		t.Error("second triple should have valid_until NULL for non-functional predicate")
	}
}

func TestKG_Timeline_AsOfPastDate(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	subjectID, err := k.EnsureEntity(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	predID, err := k.EnsurePredicate(ctx, "status", false, "status of entity")
	if err != nil {
		t.Fatal(err)
	}

	dates := []string{
		"2025-01-01T00:00:00Z",
		"2025-02-01T00:00:00Z",
		"2025-03-01T00:00:00Z",
	}
	for i, d := range dates {
		_, err := k.db.ExecContext(ctx,
			`INSERT INTO kg_triples (subject_entity_id, predicate_id, object_text, confidence, source_memory_id, valid_from)
			 VALUES (?, ?, ?, 0.5, '', ?)`,
			subjectID, predID, fmt.Sprintf("status-%d", i), d,
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	triples, err := k.Timeline(ctx, TimelineOpts{
		Subject:   "Alice",
		From:      "2025-01-01T00:00:00Z",
		Until:     "2025-02-01T00:00:00Z",
		Direction: Out,
	})
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}

	if len(triples) != 2 {
		t.Fatalf("expected 2 triples in Jan-Feb range, got %d", len(triples))
	}
}

func TestKG_Aliases_CaseAndAccentMerge(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	_, err := k.EnsurePredicate(ctx, "likes", false, "things someone likes")
	if err != nil {
		t.Fatal(err)
	}

	tid1, err := k.AddTriple(ctx, "Maria", "likes", "coffee", TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple 1: %v", err)
	}

	tid2, err := k.AddTriple(ctx, "maría", "likes", "tea", TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple 2: %v", err)
	}

	if tid1 == tid2 {
		t.Error("triples should have different IDs")
	}

	var count int
	err = k.db.QueryRow(
		"SELECT COUNT(*) FROM kg_triples WHERE subject_entity_id = (SELECT entity_id FROM kg_entities WHERE canonical_name = 'Maria')",
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 triples for Maria (alias merged), got %d", count)
	}
}

func TestKG_QueryEntity_DirectionOut(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	_, err := k.EnsurePredicate(ctx, "works_at", false, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = k.EnsurePredicate(ctx, "located_in", false, "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = k.AddTriple(ctx, "Maria", "works_at", "ACME", TripleOpts{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = k.AddTriple(ctx, "ACME", "located_in", "São Paulo", TripleOpts{})
	if err != nil {
		t.Fatal(err)
	}

	triples, err := k.QueryEntity(ctx, "Maria", Out)
	if err != nil {
		t.Fatalf("QueryEntity: %v", err)
	}
	if len(triples) != 1 {
		t.Fatalf("expected 1 triple for Maria Out, got %d", len(triples))
	}

	triples, err = k.QueryEntity(ctx, "Maria", In)
	if err != nil {
		t.Fatalf("QueryEntity In: %v", err)
	}
	if len(triples) != 0 {
		t.Fatalf("expected 0 triples for Maria In, got %d", len(triples))
	}
}

func TestKG_QueryEntity_DirectionIn(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	_, err := k.EnsurePredicate(ctx, "works_at", false, "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = k.AddTriple(ctx, "Maria", "works_at", "ACME", TripleOpts{ObjectEntityName: "ACME"})
	if err != nil {
		t.Fatal(err)
	}

	triples, err := k.QueryEntity(ctx, "ACME", In)
	if err != nil {
		t.Fatalf("QueryEntity: %v", err)
	}
	if len(triples) != 1 {
		t.Fatalf("expected 1 triple for ACME In, got %d", len(triples))
	}
	if triples[0].PredicateName != "works_at" {
		t.Errorf("expected predicate works_at, got %s", triples[0].PredicateName)
	}
}

func TestKG_QueryEntity_DirectionBoth(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	_, err := k.EnsurePredicate(ctx, "works_at", false, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = k.EnsurePredicate(ctx, "located_in", false, "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = k.AddTriple(ctx, "Maria", "works_at", "ACME", TripleOpts{ObjectEntityName: "ACME"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = k.AddTriple(ctx, "ACME", "located_in", "São Paulo", TripleOpts{})
	if err != nil {
		t.Fatal(err)
	}

	triples, err := k.QueryEntity(ctx, "ACME", Both)
	if err != nil {
		t.Fatalf("QueryEntity: %v", err)
	}
	if len(triples) != 2 {
		t.Fatalf("expected 2 triples for ACME Both, got %d", len(triples))
	}
}

func TestKG_Invalidate_ExplicitSetsValidUntil(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	tid, err := k.AddTriple(ctx, "Bob", "status", "active", TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple: %v", err)
	}

	err = k.InvalidateTriple(ctx, tid)
	if err != nil {
		t.Fatalf("InvalidateTriple: %v", err)
	}

	var vu sql.NullString
	err = k.db.QueryRow("SELECT valid_until FROM kg_triples WHERE triple_id = ?", tid).Scan(&vu)
	if err != nil {
		t.Fatal(err)
	}
	if !vu.Valid {
		t.Error("expected valid_until to be set after InvalidateTriple")
	}
}

func TestKG_ParameterizedSQL(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	malicious := "'; DROP TABLE kg_triples; --"
	_, err := k.AddTriple(ctx, malicious, "status", "safe", TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple with malicious name: %v", err)
	}

	var count int
	err = k.db.QueryRow("SELECT COUNT(*) FROM kg_triples").Scan(&count)
	if err != nil {
		t.Fatalf("query triples: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}

	var name string
	err = k.db.QueryRow("SELECT canonical_name FROM kg_entities WHERE canonical_name = ?", malicious).Scan(&name)
	if err != nil {
		t.Fatalf("query entity: %v", err)
	}
	if name != malicious {
		t.Errorf("expected entity name %q, got %q", malicious, name)
	}
}

func TestKG_AddTriple_Concurrent(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/kg_concurrent.db"
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=30000&_foreign_keys=ON&_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(KgSchema); err != nil {
		t.Fatal(err)
	}

	k, err := NewKG(db, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			subject := fmt.Sprintf("subject-%d", idx)
			_, err := k.AddTriple(ctx, subject, "status", "active", TripleOpts{})
			if err != nil {
				errors <- err
			}
		}(i)
	}
	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent AddTriple error: %v", err)
	}

	var count int
	err = k.db.QueryRow("SELECT COUNT(*) FROM kg_triples").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 10 {
		t.Errorf("expected 10 triples, got %d", count)
	}
}

func TestKG_DeleteEntity_SoftDeletesTriples(t *testing.T) {
	k := newTestKG(t)
	ctx := context.Background()

	_, err := k.EnsurePredicate(ctx, "works_at", false, "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = k.AddTriple(ctx, "Eve", "works_at", "ACME", TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple: %v", err)
	}

	err = k.DeleteEntity(ctx, "Eve")
	if err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}

	var entityCount int
	err = k.db.QueryRow("SELECT COUNT(*) FROM kg_entities WHERE canonical_name = 'Eve'").Scan(&entityCount)
	if err != nil {
		t.Fatal(err)
	}
	if entityCount != 0 {
		t.Error("entity should be deleted from kg_entities")
	}

	var tripleCount int
	err = k.db.QueryRow("SELECT COUNT(*) FROM kg_triples").Scan(&tripleCount)
	if err != nil {
		t.Fatal(err)
	}
	if tripleCount != 0 {
		t.Errorf("expected 0 triples after DeleteEntity, got %d", tripleCount)
	}
}

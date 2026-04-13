package copilot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

func newTestStoreWithKGForTools(t *testing.T) (*memory.SQLiteStore, *kg.KG) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "kg_tools_test.db")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := memory.NewSQLiteStore(dbPath, &memory.NullEmbedder{}, logger)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	k, err := kg.NewKG(store.DB(), logger)
	if err != nil {
		t.Fatalf("create KG: %v", err)
	}
	store.SetKG(k)
	return store, k
}

func TestKGTool_Query_HappyPath(t *testing.T) {
	_, k := newTestStoreWithKGForTools(t)
	ctx := context.Background()

	_, err := k.AddTriple(ctx, "Maria", "works_at", "ACME", kg.TripleOpts{Wing: "work", Confidence: 0.5})
	if err != nil {
		t.Fatalf("AddTriple: %v", err)
	}
	_, err = k.AddTriple(ctx, "Maria", "likes", "coffee", kg.TripleOpts{Confidence: 0.4})
	if err != nil {
		t.Fatalf("AddTriple: %v", err)
	}

	result, err := handleKGQuery(ctx, k, map[string]any{"entity": "Maria"})
	if err != nil {
		t.Fatalf("handleKGQuery: %v", err)
	}
	out, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.Contains(out, "Maria (works_at) ACME") {
		t.Errorf("expected works_at triple in output, got: %s", out)
	}
	if !strings.Contains(out, "Maria (likes) coffee") {
		t.Errorf("expected likes triple in output, got: %s", out)
	}
	if !strings.Contains(out, "wing=work") {
		t.Errorf("expected wing=work in output, got: %s", out)
	}
}

func TestKGTool_Add_NormalizesWing(t *testing.T) {
	_, k := newTestStoreWithKGForTools(t)
	ctx := context.Background()

	result, err := handleKGAdd(ctx, k, map[string]any{
		"subject":    "Alice",
		"predicate":  "located_in",
		"object":     "Berlin",
		"wing":       "Work  ",
		"confidence": 0.8,
	})
	if err != nil {
		t.Fatalf("handleKGAdd: %v", err)
	}
	out, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.Contains(out, "Added triple #") {
		t.Errorf("expected confirmation, got: %s", out)
	}

	triples, err := k.QueryEntity(ctx, "Alice", kg.Out)
	if err != nil {
		t.Fatalf("QueryEntity: %v", err)
	}
	if len(triples) != 1 {
		t.Fatalf("expected 1 triple, got %d", len(triples))
	}
	if triples[0].Wing != "work" {
		t.Errorf("expected wing normalized to %q, got %q", "work", triples[0].Wing)
	}
}

func TestKGTool_Invalidate_RequiresConfirmOrPreview(t *testing.T) {
	_, k := newTestStoreWithKGForTools(t)
	ctx := context.Background()

	tid, err := k.AddTriple(ctx, "Bob", "status", "active", kg.TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple: %v", err)
	}

	result, err := handleKGInvalidate(ctx, k, map[string]any{
		"triple_id": float64(tid),
	})
	if err != nil {
		t.Fatalf("handleKGInvalidate without confirm: %v", err)
	}
	out, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.Contains(out, "Set confirm=true to proceed") {
		t.Errorf("expected preview message, got: %s", out)
	}

	result, err = handleKGInvalidate(ctx, k, map[string]any{
		"triple_id": float64(tid),
		"confirm":   true,
	})
	if err != nil {
		t.Fatalf("handleKGInvalidate with confirm: %v", err)
	}
	out, ok = result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.Contains(out, "invalidated") {
		t.Errorf("expected invalidated message, got: %s", out)
	}
}

func TestKGTool_Timeline_TruncatesAt100(t *testing.T) {
	_, k := newTestStoreWithKGForTools(t)
	ctx := context.Background()

	subjectID, err := k.EnsureEntity(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	predID, err := k.EnsurePredicate(ctx, "status", false, "")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 150; i++ {
		_, err := k.DB().ExecContext(ctx,
			`INSERT INTO kg_triples (subject_entity_id, predicate_id, object_text, confidence, source_memory_id, valid_from)
			 VALUES (?, ?, ?, 0.5, '', ?)`,
			subjectID, predID, fmt.Sprintf("status-%d", i), "2025-01-01T00:00:00Z",
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	result, err := handleKGTimeline(ctx, k, map[string]any{
		"entity":    "Alice",
		"direction": "out",
		"from":      "2024-01-01T00:00:00Z",
		"until":     "2026-12-31T23:59:59Z",
		"limit":     float64(150),
	})
	if err != nil {
		t.Fatalf("handleKGTimeline: %v", err)
	}
	out, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}

	lineCount := strings.Count(out, "\n")
	// 100 triple lines + 1 has_more line (possibly trailing newline)
	if lineCount > 102 {
		t.Errorf("expected at most 102 lines (100 triples + has_more), got %d", lineCount)
	}
	if !strings.Contains(out, "has_more: true") {
		t.Errorf("expected has_more: true in output")
	}
}

func TestKGTool_MergeEntities_RequiresConfirm(t *testing.T) {
	_, k := newTestStoreWithKGForTools(t)
	ctx := context.Background()

	_, err := k.AddTriple(ctx, "Eve", "works_at", "ACME", kg.TripleOpts{ObjectEntityName: "ACME"})
	if err != nil {
		t.Fatalf("AddTriple: %v", err)
	}

	result, err := handleKGMergeEntities(ctx, k, map[string]any{
		"source_entity": "Eve",
		"target_entity": "Evelyn",
	})
	if err != nil {
		t.Fatalf("handleKGMergeEntities without confirm: %v", err)
	}
	out, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.Contains(out, "Set confirm=true to proceed") {
		t.Errorf("expected preview message, got: %s", out)
	}

	result, err = handleKGMergeEntities(ctx, k, map[string]any{
		"source_entity": "Eve",
		"target_entity": "Evelyn",
		"confirm":       true,
	})
	if err != nil {
		t.Fatalf("handleKGMergeEntities with confirm: %v", err)
	}
	out, ok = result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.Contains(out, "Merged") {
		t.Errorf("expected merged message, got: %s", out)
	}

	triples, err := k.QueryEntity(ctx, "Evelyn", kg.Out)
	if err != nil {
		t.Fatalf("QueryEntity after merge: %v", err)
	}
	if len(triples) != 1 {
		t.Fatalf("expected 1 triple for Evelyn after merge, got %d", len(triples))
	}
	if triples[0].PredicateName != "works_at" {
		t.Errorf("expected predicate works_at, got %s", triples[0].PredicateName)
	}

	aliasID, err := k.ResolveAlias(ctx, "Eve")
	if err != nil {
		t.Fatalf("ResolveAlias after merge: %v", err)
	}
	targetID, _ := k.EnsureEntity(ctx, "Evelyn")
	if aliasID != targetID {
		t.Errorf("alias should point to target (id=%d), got id=%d", targetID, aliasID)
	}
}

func TestKGTool_Stats_ReturnsCounts(t *testing.T) {
	_, k := newTestStoreWithKGForTools(t)
	ctx := context.Background()

	_, err := k.AddTriple(ctx, "Alice", "works_at", "ACME", kg.TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple 1: %v", err)
	}
	_, err = k.AddTriple(ctx, "Bob", "likes", "coffee", kg.TripleOpts{})
	if err != nil {
		t.Fatalf("AddTriple 2: %v", err)
	}

	result, err := handleKGStats(ctx, k)
	if err != nil {
		t.Fatalf("handleKGStats: %v", err)
	}
	out, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.Contains(out, "entities=") {
		t.Errorf("expected entities count in output, got: %s", out)
	}
	if !strings.Contains(out, "predicates=") {
		t.Errorf("expected predicates count in output, got: %s", out)
	}
	if !strings.Contains(out, "active_triples=2") {
		t.Errorf("expected active_triples=2 in output, got: %s", out)
	}
}

func TestKGTool_Query_EmptyResult(t *testing.T) {
	_, k := newTestStoreWithKGForTools(t)
	ctx := context.Background()

	result, err := handleKGQuery(ctx, k, map[string]any{"entity": "Nobody"})
	if err != nil {
		t.Fatalf("handleKGQuery: %v", err)
	}
	out, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.Contains(out, "No triples found") {
		t.Errorf("expected empty result message, got: %s", out)
	}
}

func TestKGTool_Add_MissingRequiredArgs(t *testing.T) {
	_, k := newTestStoreWithKGForTools(t)
	ctx := context.Background()

	_, err := handleKGAdd(ctx, k, map[string]any{
		"predicate": "likes",
		"object":    "coffee",
	})
	if err == nil {
		t.Fatal("expected error for missing subject")
	}
	if !strings.Contains(err.Error(), "subject is required") {
		t.Errorf("expected subject required error, got: %v", err)
	}

	_, err = handleKGAdd(ctx, k, map[string]any{
		"subject": "Alice",
		"object":  "coffee",
	})
	if err == nil {
		t.Fatal("expected error for missing predicate")
	}
	if !strings.Contains(err.Error(), "predicate is required") {
		t.Errorf("expected predicate required error, got: %v", err)
	}

	_, err = handleKGAdd(ctx, k, map[string]any{
		"subject":   "Alice",
		"predicate": "likes",
	})
	if err == nil {
		t.Fatal("expected error for missing object")
	}
	if !strings.Contains(err.Error(), "object is required") {
		t.Errorf("expected object required error, got: %v", err)
	}
}

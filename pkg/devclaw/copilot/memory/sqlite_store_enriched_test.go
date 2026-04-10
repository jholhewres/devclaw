package memory

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg"
)

func newTestStoreWithKG(t *testing.T) (*SQLiteStore, *kg.KG) {
	t.Helper()
	s := newTestStore(t)
	k, err := kg.NewKG(s.db, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	s.SetKG(k)
	return s, k
}

func seedKGTriple(t *testing.T, k *kg.KG, subject, predicate, object string, opts kg.TripleOpts) {
	t.Helper()
	ctx := context.Background()
	if opts.Confidence == 0 {
		opts.Confidence = 0.9
	}
	_, err := k.AddTriple(ctx, subject, predicate, object, opts)
	if err != nil {
		t.Fatalf("seed triple (%s %s %s): %v", subject, predicate, object, err)
	}
}

func TestHybridSearchEnriched_NoKGDataBehavesLikePlain(t *testing.T) {
	store, _ := newTestStoreWithKG(t)
	ctx := context.Background()

	chunks := []Chunk{
		{Index: 0, Text: "alpha beta gamma", Hash: hashChunk("alpha beta gamma")},
	}
	if err := store.IndexChunks(ctx, "fileA", chunks, hashChunk("fileA")); err != nil {
		t.Fatal(err)
	}

	opts := HybridSearchOptions{MaxResults: 10}
	enriched, err := store.HybridSearchEnriched(ctx, "alpha", opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(enriched.Memories) == 0 {
		t.Fatal("expected memories from search")
	}
	if len(enriched.KGFacts) != 0 {
		t.Fatalf("expected 0 KG facts, got %d", len(enriched.KGFacts))
	}
	if len(enriched.EntityMatches) != 0 {
		t.Fatalf("expected 0 entity matches, got %d", len(enriched.EntityMatches))
	}

	plain, err := store.HybridSearchWithOpts(ctx, "alpha", opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(enriched.Memories) != len(plain) {
		t.Fatalf("enriched memories %d != plain %d", len(enriched.Memories), len(plain))
	}
}

func TestHybridSearchEnriched_EntityInQueryLoadsFacts(t *testing.T) {
	store, k := newTestStoreWithKG(t)
	ctx := context.Background()

	chunks := []Chunk{
		{Index: 0, Text: "Maria works at ACME Corp", Hash: hashChunk("Maria works at ACME Corp")},
	}
	if err := store.IndexChunks(ctx, "fileM", chunks, hashChunk("fileM")); err != nil {
		t.Fatal(err)
	}

	seedKGTriple(t, k, "Maria", "works_at", "ACME", kg.TripleOpts{})

	enriched, err := store.HybridSearchEnriched(ctx, "onde Maria trabalha", HybridSearchOptions{MaxResults: 10})
	if err != nil {
		t.Fatal(err)
	}

	if len(enriched.KGFacts) == 0 {
		t.Fatal("expected KG facts for Maria")
	}

	found := false
	for _, f := range enriched.KGFacts {
		if f.SubjectName == "Maria" && f.PredicateName == "works_at" && f.ObjectText == "ACME" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected (Maria, works_at, ACME) in facts, got %v", enriched.KGFacts)
	}

	if len(enriched.EntityMatches) == 0 {
		t.Fatal("expected entity matches")
	}
	foundMatch := false
	for _, em := range enriched.EntityMatches {
		if em.Name == "Maria" {
			foundMatch = true
			break
		}
	}
	if !foundMatch {
		t.Errorf("expected Maria in entity matches, got %v", enriched.EntityMatches)
	}
}

func TestHybridSearchEnriched_WingNeutralityPreserved(t *testing.T) {
	store, k := newTestStoreWithKG(t)
	ctx := context.Background()

	seedKGTriple(t, k, "Alice", "lives_in", "Wonderland", kg.TripleOpts{Wing: ""})

	enriched, err := store.HybridSearchEnriched(ctx, "Alice", HybridSearchOptions{
		MaxResults: 10,
		QueryWing:  "project-x",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(enriched.KGFacts) == 0 {
		t.Fatal("expected KG facts for Alice regardless of wing filter")
	}

	found := false
	for _, f := range enriched.KGFacts {
		if f.SubjectName == "Alice" && f.PredicateName == "lives_in" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected (Alice, lives_in, Wonderland) in facts, got %v", enriched.KGFacts)
	}
}

func TestHybridSearchEnriched_BudgetRespectsFactsPerEntity(t *testing.T) {
	store, k := newTestStoreWithKG(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		pred := "skill_" + string(rune('A'+i))
		seedKGTriple(t, k, "Bob", pred, "value", kg.TripleOpts{})
	}

	enriched, err := store.HybridSearchEnriched(ctx, "Bob", HybridSearchOptions{
		MaxResults:       10,
		KGFactsPerEntity: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(enriched.KGFacts) > 3 {
		t.Fatalf("expected at most 3 KG facts (budget), got %d", len(enriched.KGFacts))
	}
}

func TestHybridSearchEnriched_NilKGReturnsMemoriesOnly(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "nokg-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	store, err := NewSQLiteStore(tmpFile.Name(), &deterministicEmbedder{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.kg = nil

	ctx := context.Background()
	chunks := []Chunk{
		{Index: 0, Text: "delta epsilon zeta", Hash: hashChunk("delta epsilon zeta")},
	}
	if err := store.IndexChunks(ctx, "fileD", chunks, hashChunk("fileD")); err != nil {
		t.Fatal(err)
	}

	enriched, err := store.HybridSearchEnriched(ctx, "delta", HybridSearchOptions{MaxResults: 10})
	if err != nil {
		t.Fatal(err)
	}

	if len(enriched.Memories) == 0 {
		t.Fatal("expected memories even when kg=nil")
	}
	if len(enriched.KGFacts) != 0 {
		t.Fatalf("expected 0 KG facts when kg=nil, got %d", len(enriched.KGFacts))
	}
	if len(enriched.EntityMatches) != 0 {
		t.Fatalf("expected 0 entity matches when kg=nil, got %d", len(enriched.EntityMatches))
	}
}

func TestHybridSearchEnriched_LegacyPathUnchanged(t *testing.T) {
	s := newStableStore(t)
	ctx := context.Background()

	chunks := []Chunk{
		{Index: 0, Text: "alpha beta gamma delta", Hash: hashChunk("alpha beta gamma delta")},
		{Index: 1, Text: "epsilon zeta eta theta", Hash: hashChunk("epsilon zeta eta theta")},
	}
	if err := s.IndexChunks(ctx, "fileLegacy", chunks, hashChunk("fileLegacy")); err != nil {
		t.Fatal(err)
	}

	plainBefore, err := s.HybridSearchWithOpts(ctx, "alpha beta", HybridSearchOptions{MaxResults: 10})
	if err != nil {
		t.Fatal(err)
	}

	plainAfter, err := s.HybridSearchWithOpts(ctx, "alpha beta", HybridSearchOptions{MaxResults: 10})
	if err != nil {
		t.Fatal(err)
	}

	if len(plainBefore) != len(plainAfter) {
		t.Fatalf("legacy path result count changed: %d vs %d", len(plainBefore), len(plainAfter))
	}
	for i := range plainBefore {
		if plainBefore[i].FileID != plainAfter[i].FileID {
			t.Errorf("result[%d] FileID changed: %s vs %s", i, plainBefore[i].FileID, plainAfter[i].FileID)
		}
		if plainBefore[i].Score != plainAfter[i].Score {
			t.Errorf("result[%d] Score changed: %f vs %f", i, plainBefore[i].Score, plainAfter[i].Score)
		}
		if plainBefore[i].Text != plainAfter[i].Text {
			t.Errorf("result[%d] Text changed", i)
		}
		if plainBefore[i].Wing != plainAfter[i].Wing {
			t.Errorf("result[%d] Wing changed: %q vs %q", i, plainBefore[i].Wing, plainAfter[i].Wing)
		}
	}
}

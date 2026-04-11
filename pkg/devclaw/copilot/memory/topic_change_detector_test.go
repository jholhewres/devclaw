package memory

import (
	"math/rand"
	"testing"
)

func em(text, normalized string) EntityMatch {
	return EntityMatch{Candidate: EntityCandidate{Text: text, Normalized: normalized}}
}

func TestTopicChange_SameTopic_EntityOverlap(t *testing.T) {
	d := NewTopicChangeDetector(0.65, 0.3, nil, 5)

	entities1 := []EntityMatch{em("React", "react"), em("TypeScript", "typescript"), em("Vite", "vite")}
	d.UpdateTopic(entities1, nil)

	entities2 := []EntityMatch{em("React", "react"), em("TypeScript", "typescript")}
	result := d.Detect(entities2, nil)
	if result.Changed {
		t.Error("expected Changed=false for overlapping entities")
	}
}

func TestTopicChange_DifferentTopic_Cascade(t *testing.T) {
	d := NewTopicChangeDetector(0.65, 0.3, nil, 5)
	rng := rand.New(rand.NewSource(42))

	entitiesA := []EntityMatch{em("React", "react"), em("Frontend", "frontend")}
	embA := makeRandomVector(rng, 128)
	d.UpdateTopic(entitiesA, embA)

	entitiesB := []EntityMatch{em("PostgreSQL", "postgresql"), em("Database", "database")}
	embB := makeRandomVector(rng, 128)
	result := d.Detect(entitiesB, embB)
	if !result.Changed {
		t.Error("expected Changed=true for different entities + different embedding")
	}
	if result.Confidence <= 0 {
		t.Error("expected positive confidence")
	}
}

func TestTopicChange_LowEntityOverlap_HighCosine(t *testing.T) {
	d := NewTopicChangeDetector(0.65, 0.3, nil, 5)
	emb := []float32{0.1, 0.2, 0.3, 0.4, 0.5}

	d.UpdateTopic([]EntityMatch{em("A", "a")}, emb)
	result := d.Detect([]EntityMatch{em("B", "b")}, emb) // identical embedding → cosine = 1.0
	if result.Changed {
		t.Error("expected Changed=false when cosine is high despite entity change")
	}
}

func TestTopicChange_FirstTurn_NeverChanged(t *testing.T) {
	d := NewTopicChangeDetector(0.65, 0.3, nil, 5)
	result := d.Detect([]EntityMatch{em("React", "react")}, []float32{0.1, 0.2, 0.3})
	if result.Changed {
		t.Error("first turn should never detect change (no previous state)")
	}
}

func TestTopicChange_NoExtraEmbeddingCall(t *testing.T) {
	d := NewTopicChangeDetector(0.65, 0.3, nil, 5)
	d.UpdateTopic([]EntityMatch{em("A", "a")}, nil)
	result := d.Detect([]EntityMatch{em("B", "b")}, nil)
	if !result.Changed {
		t.Error("expected Changed=true with zero overlap and no embeddings")
	}
}

func TestTopicChange_UpdateTopic_Synchronous(t *testing.T) {
	d := NewTopicChangeDetector(0.65, 0.3, nil, 5)
	entities := []EntityMatch{em("Go", "go"), em("Backend", "backend")}
	emb := []float32{1, 2, 3}
	d.UpdateTopic(entities, emb)

	d.mu.RLock()
	if len(d.lastEntities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(d.lastEntities))
	}
	if len(d.lastEmbedding) != 3 {
		t.Errorf("expected 3-dim embedding, got %d", len(d.lastEmbedding))
	}
	d.mu.RUnlock()
}

func TestMergeOnDemandResults(t *testing.T) {
	t.Run("deduplication by fileID keeps higher score", func(t *testing.T) {
		a := []onDemandResult{{fileID: "f1", text: "a", score: 0.9, wing: "w"}}
		b := []onDemandResult{{fileID: "f1", text: "b", score: 0.5, wing: "w"}}
		got := mergeOnDemandResults(a, b, 10)
		if len(got) != 1 {
			t.Fatalf("expected 1 result after dedup, got %d", len(got))
		}
		if got[0].score != 0.9 {
			t.Errorf("expected higher score 0.9, got %f", got[0].score)
		}
	})

	t.Run("sorted by score descending", func(t *testing.T) {
		a := []onDemandResult{
			{fileID: "f1", text: "a", score: 0.3, wing: "w"},
			{fileID: "f2", text: "b", score: 0.8, wing: "w"},
		}
		b := []onDemandResult{
			{fileID: "f3", text: "c", score: 0.6, wing: "w"},
		}
		got := mergeOnDemandResults(a, b, 10)
		if len(got) != 3 {
			t.Fatalf("expected 3 results, got %d", len(got))
		}
		for i := 1; i < len(got); i++ {
			if got[i].score > got[i-1].score {
				t.Errorf("results not sorted: got[%d].score=%f > got[%d].score=%f", i, got[i].score, i-1, got[i-1].score)
			}
		}
	})

	t.Run("maxResults cap is respected", func(t *testing.T) {
		a := []onDemandResult{
			{fileID: "f1", text: "a", score: 0.9, wing: "w"},
			{fileID: "f2", text: "b", score: 0.8, wing: "w"},
			{fileID: "f3", text: "c", score: 0.7, wing: "w"},
		}
		b := []onDemandResult{
			{fileID: "f4", text: "d", score: 0.6, wing: "w"},
		}
		got := mergeOnDemandResults(a, b, 2)
		if len(got) != 2 {
			t.Fatalf("expected 2 results (capped), got %d", len(got))
		}
		// Should keep the top 2 by score.
		if got[0].score != 0.9 || got[1].score != 0.8 {
			t.Errorf("expected scores 0.9 and 0.8, got %f and %f", got[0].score, got[1].score)
		}
	})
}

func TestEntityOverlap(t *testing.T) {
	tests := []struct {
		name string
		a, b map[string]bool
		want float32
	}{
		{"both empty", map[string]bool{}, map[string]bool{}, 1.0},
		{"identical", map[string]bool{"a": true, "b": true}, map[string]bool{"a": true, "b": true}, 1.0},
		{"no overlap", map[string]bool{"a": true}, map[string]bool{"b": true}, 0.0},
		{"partial", map[string]bool{"a": true, "b": true, "c": true}, map[string]bool{"a": true, "b": true}, 2.0 / 3.0},
		{"one empty", map[string]bool{"a": true}, map[string]bool{}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := entityOverlap(tt.a, tt.b)
			if diff := got - tt.want; diff > 0.01 || diff < -0.01 {
				t.Errorf("entityOverlap = %f, want %f", got, tt.want)
			}
		})
	}
}

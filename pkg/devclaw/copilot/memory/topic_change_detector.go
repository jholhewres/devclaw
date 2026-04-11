// Package memory – topic_change_detector.go detects when the user changes
// conversation topic and provides relevant context for the new topic.
//
// Uses a two-stage cascade with ZERO extra API calls:
//   - Stage 1: Entity overlap (free, ~0ms) — compares entity sets between turns
//   - Stage 2: Cosine similarity (free, reuses embedding from L2 search)
//
// Both inputs are already computed by the OnDemandLayer pipeline.
package memory

import (
	"context"
	"sync"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg"
)

// TopicChangeDetector detects topic changes using a two-stage cascade.
// Thread-safe via RWMutex.
type TopicChangeDetector struct {
	mu sync.RWMutex

	lastEntities  map[string]bool // normalized entity names from last turn
	lastEmbedding []float32       // embedding from last turn (reused from L2)

	// Thresholds
	cosineThreshold    float32 // cosine similarity below this = topic changed (default 0.65)
	entityOverlapThresh float32 // entity overlap below this triggers cosine check (default 0.3)

	// Optional KG for fact lookup on topic change.
	kg               *kg.KG
	factsPerInjection int
}

// TopicChangeResult holds the detection outcome.
type TopicChangeResult struct {
	Changed    bool
	Confidence float32 // 1 - cosine similarity (higher = more different)
}

// NewTopicChangeDetector creates a detector with the given thresholds.
// kg may be nil — the detector works without it (no fact injection).
func NewTopicChangeDetector(cosineThreshold, entityOverlap float32, kgStore *kg.KG, factsPerInjection int) *TopicChangeDetector {
	if cosineThreshold <= 0 {
		cosineThreshold = 0.65
	}
	if entityOverlap <= 0 {
		entityOverlap = 0.3
	}
	if factsPerInjection <= 0 {
		factsPerInjection = 5
	}
	return &TopicChangeDetector{
		cosineThreshold:     cosineThreshold,
		entityOverlapThresh: entityOverlap,
		lastEntities:        make(map[string]bool),
		kg:                  kgStore,
		factsPerInjection:   factsPerInjection,
	}
}

// Detect uses a two-stage cascade to detect topic changes.
//
// Stage 1 (free): Entity overlap — if overlap >= threshold, same topic (fast path).
// Stage 2 (free): Cosine similarity of embeddings — confirms topic change.
//
// CRITICAL: Both inputs are already computed by the L2 pipeline:
//   - currentEntities from EntityDetector.Detect()
//   - currentEmbedding from SearchVector() query embedding
//
// NO extra embedding API calls are made.
func (d *TopicChangeDetector) Detect(currentEntities []EntityMatch, currentEmbedding []float32) TopicChangeResult {
	d.mu.RLock()
	lastEntities := d.lastEntities
	lastEmbedding := d.lastEmbedding
	d.mu.RUnlock()

	// First turn — no previous state to compare.
	if len(lastEntities) == 0 && len(lastEmbedding) == 0 {
		return TopicChangeResult{Changed: false}
	}

	// Stage 1: Entity overlap (free, ~0ms).
	currentSet := make(map[string]bool, len(currentEntities))
	for _, e := range currentEntities {
		currentSet[e.Candidate.Normalized] = true
	}

	overlap := entityOverlap(lastEntities, currentSet)
	if overlap >= d.entityOverlapThresh {
		// High overlap → same topic. Skip cosine check.
		return TopicChangeResult{Changed: false, Confidence: 0}
	}

	// Stage 2: Cosine similarity (free — reuses embedding already computed for search).
	if len(currentEmbedding) > 0 && len(lastEmbedding) > 0 {
		sim := cosineSimilarity(currentEmbedding, lastEmbedding)
		if sim >= float64(d.cosineThreshold) {
			// Embeddings are similar despite entity change — not a real topic change.
			return TopicChangeResult{Changed: false, Confidence: float32(1 - sim)}
		}
		return TopicChangeResult{Changed: true, Confidence: float32(1 - sim)}
	}

	// No embeddings available — rely on entity overlap alone.
	return TopicChangeResult{Changed: true, Confidence: 1 - overlap}
}

// LookupKGFacts queries the KG for facts related to the current entities.
// Returns empty if KG is nil or no entities provided.
func (d *TopicChangeDetector) LookupKGFacts(ctx context.Context, entities []EntityMatch) []kg.Triple {
	if d.kg == nil || len(entities) == 0 {
		return nil
	}

	var allFacts []kg.Triple
	for _, e := range entities {
		if len(allFacts) >= d.factsPerInjection {
			break
		}
		facts, err := d.kg.CurrentFacts(ctx, e.Candidate.Text)
		if err != nil {
			continue
		}
		allFacts = append(allFacts, facts...)
	}

	if len(allFacts) > d.factsPerInjection {
		allFacts = allFacts[:d.factsPerInjection]
	}
	return allFacts
}

// UpdateTopic stores the current turn's state for the next comparison.
// This is a synchronous struct copy — NO API call, NO goroutine needed.
func (d *TopicChangeDetector) UpdateTopic(entities []EntityMatch, embedding []float32) {
	newSet := make(map[string]bool, len(entities))
	for _, e := range entities {
		newSet[e.Candidate.Normalized] = true
	}

	d.mu.Lock()
	d.lastEntities = newSet
	d.lastEmbedding = embedding
	d.mu.Unlock()
}

// entityOverlap computes Jaccard-like overlap: |A ∩ B| / max(|A|, |B|).
// Returns 1.0 when both sets are empty (no change detectable).
func entityOverlap(a, b map[string]bool) float32 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	var intersection int
	for k := range a {
		if b[k] {
			intersection++
		}
	}
	return float32(intersection) / float32(maxLen)
}

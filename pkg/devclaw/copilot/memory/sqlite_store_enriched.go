// Package memory — sqlite_store_enriched.go implements the Sprint 3 Room 3.5
// SearchEnriched method. It composes the existing HybridSearchWithOpts result
// with Knowledge Graph facts for entities detected in the query.
//
// The enriched path is completely separate from the legacy search path:
// HybridSearchWithOpts is NOT modified. Callers opt in by calling
// HybridSearchEnriched instead of HybridSearchWithOpts.
package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg"
)

type EnrichedSearchResult struct {
	Memories      []SearchResult
	KGFacts       []KGFact
	EntityMatches []EnrichedEntityMatch
}

type KGFact struct {
	SubjectName    string
	PredicateName  string
	ObjectText     string
	Confidence     float64
	Wing           string
	SourceMemoryID string
}

type EnrichedEntityMatch struct {
	Name     string
	EntityID int64
	Matched  string
}

// defaultKGFactsPerEntity is the cap on facts returned per detected entity
// when HybridSearchOpts.KGFactsPerEntity is zero.
const defaultKGFactsPerEntity = 3

// HybridSearchEnriched runs the existing HybridSearchWithOpts and then
// enriches the result with KG facts for any entities detected in the query.
//
// When s.kg is nil (KG disabled), the method returns memories-only result
// with no error — identical to calling HybridSearchWithOpts directly plus
// empty KGFacts and EntityMatches slices.
//
// Entity detection uses a lightweight token-to-kg_entities lookup instead of
// the full EntityDetector (which is bound to wings/rooms). Each query token
// is checked against kg_entities.canonical_name and kg_entity_aliases.alias_name.
func (s *SQLiteStore) HybridSearchEnriched(ctx context.Context, query string, opts HybridSearchOptions) (*EnrichedSearchResult, error) {
	// Step 1 — run existing hybrid search (path untouched).
	memories, err := s.HybridSearchWithOpts(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("enriched search base: %w", err)
	}

	result := &EnrichedSearchResult{
		Memories: memories,
	}

	// Step 2 — if KG is disabled, return memories-only.
	if s.kg == nil {
		return result, nil
	}

	// Step 3 — detect entities in the query via direct SQL lookup.
	capPerEntity := opts.KGFactsPerEntity
	if capPerEntity <= 0 {
		capPerEntity = defaultKGFactsPerEntity
	}

	entities := s.detectKGEntities(ctx, query)
	if len(entities) == 0 {
		return result, nil
	}
	result.EntityMatches = enrichedMatchesFromInternal(entities)

	// Step 4 — for each detected entity, pull current KG facts.
	seenFacts := make(map[string]struct{}) // dedupe by subject+predicate+object
	for _, em := range entities {
		facts, err := s.kg.CurrentFacts(ctx, em.Name)
		if err != nil {
			continue
		}

		added := 0
		for _, tr := range facts {
			if capPerEntity > 0 && added >= capPerEntity {
				break
			}
			key := tr.SubjectName + "|" + tr.PredicateName + "|" + tr.ObjectText
			if _, ok := seenFacts[key]; ok {
				continue
			}
			seenFacts[key] = struct{}{}

			result.KGFacts = append(result.KGFacts, KGFact{
				SubjectName:    tr.SubjectName,
				PredicateName:  tr.PredicateName,
				ObjectText:     tr.ObjectText,
				Confidence:     tr.Confidence,
				Wing:           tr.Wing,
				SourceMemoryID: tr.SourceMemoryID,
			})
			added++
		}
	}

	return result, nil
}

// kgEntityMatch represents a KG entity detected in the search query.
type kgEntityMatch struct {
	Name     string
	EntityID int64
	Matched  string // the substring that matched
}

// detectKGEntities extracts tokens from the query and checks each against
// kg_entities (canonical_name) and kg_entity_aliases (alias_name). Returns
// deduplicated matches ordered by first appearance in the query.
func (s *SQLiteStore) detectKGEntities(ctx context.Context, query string) []kgEntityMatch {
	tokens := extractKGQueryTokens(query)
	if len(tokens) == 0 {
		return nil
	}

	seen := make(map[int64]struct{})
	var matches []kgEntityMatch

	for _, tok := range tokens {
		normalized := StripAccents(strings.ToLower(tok))

		var entityID int64
		var canonicalName string

		// Try exact match on canonical_name first.
		err := s.db.QueryRowContext(ctx,
			"SELECT entity_id, canonical_name FROM kg_entities WHERE canonical_name = ?",
			normalized,
		).Scan(&entityID, &canonicalName)
		if err == nil {
			if _, ok := seen[entityID]; !ok {
				seen[entityID] = struct{}{}
				matches = append(matches, kgEntityMatch{
					Name:     canonicalName,
					EntityID: entityID,
					Matched:  tok,
				})
			}
			continue
		}

		// Try alias lookup.
		err = s.db.QueryRowContext(ctx,
			`SELECT e.entity_id, e.canonical_name
			 FROM kg_entity_aliases a
			 JOIN kg_entities e ON e.entity_id = a.entity_id
			 WHERE a.alias_name = ?`,
			normalized,
		).Scan(&entityID, &canonicalName)
		if err == nil {
			if _, ok := seen[entityID]; !ok {
				seen[entityID] = struct{}{}
				matches = append(matches, kgEntityMatch{
					Name:     canonicalName,
					EntityID: entityID,
					Matched:  tok,
				})
			}
		}
	}

	return matches
}

func enrichedMatchesFromInternal(in []kgEntityMatch) []EnrichedEntityMatch {
	out := make([]EnrichedEntityMatch, len(in))
	for i, m := range in {
		out[i] = EnrichedEntityMatch{
			Name:     m.Name,
			EntityID: m.EntityID,
			Matched:  m.Matched,
		}
	}
	return out
}

// extractKGQueryTokens splits a query into candidate tokens for KG entity
// lookup. Uses the same tokenRe as EntityDetector (Unicode word tokens of 3+
// characters) but returns the raw tokens without normalization.
func extractKGQueryTokens(query string) []string {
	locs := tokenRe.FindAllStringIndex(query, -1)
	if len(locs) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	var tokens []string
	for _, loc := range locs {
		raw := query[loc[0]:loc[1]]
		lower := strings.ToLower(raw)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		tokens = append(tokens, raw)
	}
	return tokens
}

// SetKG injects a KG reference into the store. Pass nil to disable KG
// enrichment. The KG instance is inert until someone calls
// HybridSearchEnriched.
func (s *SQLiteStore) SetKG(k *kg.KG) {
	s.kg = k
}

// KG returns the Knowledge Graph instance, or nil if KG is not configured.
func (s *SQLiteStore) KG() *kg.KG {
	return s.kg
}

// Package memory — legacy_classifier.go implements pattern-based
// classification of legacy (wing=NULL) memory files.
//
// Sprint 1 amendment (2026-04-08): per user directive "incremental
// improvement felt", the dream system will opportunistically classify
// legacy files into wings using keyword-based pattern matching. This
// reverses the strict "never backfill" stance of ADR-006 — but only for
// the safe, free, deterministic path. LLM-based classification remains
// out of scope (ADR-009 security + cost concerns).
//
// Design principles:
//
//  1. Pattern-only. No LLM calls. Zero token cost. Deterministic.
//  2. Conservative. Only classify when the signal is strong; leave
//     ambiguous files as wing=NULL (still first-class).
//  3. Idempotent. Running the classifier multiple times on the same file
//     never changes its result.
//  4. Auditable. Every classification records the matched keywords in
//     the source field ('auto-legacy') for user review.
//  5. One-way. Once a file has wing != NULL, the classifier NEVER touches
//     it again. User's explicit classification is sacred.
//  6. Reversible. User can /wing unset or wipe wing=<name> for a file
//     to put it back in the legacy pool.
//
// This file contains the pure classifier (no DB access). The batch
// runner that applies it to the database lives in legacy_classifier_pass.go.
package memory

import (
	"strings"
	"unicode"
)

// ClassifierResult describes the outcome of classifying a single file's
// content. When Confidence is 0, the classifier could not decide and the
// file should remain as wing=NULL.
type ClassifierResult struct {
	// Wing is the canonical wing identifier (normalized form).
	// Empty string means "could not classify — leave as NULL".
	Wing string

	// Confidence is a score in [0, 1] reflecting how strongly the content
	// matched the wing's keyword set. Classifier calling code should only
	// apply classifications where Confidence >= ClassifierMinConfidence.
	Confidence float64

	// MatchedKeywords lists the keywords that contributed to the decision.
	// Used for audit logging and user-facing explanations.
	MatchedKeywords []string

	// TopWingHits is the number of distinct keyword hits for the chosen wing.
	// Exposed for telemetry and test assertions.
	TopWingHits int

	// SecondWingHits is the number of hits for the next-best wing. Used in
	// the margin check (top must dominate second).
	SecondWingHits int
}

// ClassifierMinConfidence is the threshold below which the classifier
// refuses to label a file. Files scoring below this stay as wing=NULL.
const ClassifierMinConfidence = 0.65

// ClassifierMinHits is the absolute minimum number of keyword matches
// for the winning wing. A file with only 1-2 weak signals is always
// left alone, regardless of margin over the second place.
const ClassifierMinHits = 3

// ClassifierDominanceFactor requires the winning wing to have at least
// this many times the hit count of the second wing. A 2× margin means
// "work" with 6 hits beats "personal" with 3 hits, but doesn't beat
// "personal" with 4 hits (4 × 2 = 8 > 6).
const ClassifierDominanceFactor = 2.0

// ClassifyLegacyContent applies the pattern-based classifier to a blob of
// text and returns its best guess at a wing assignment.
//
// The function is stateless and pure — same input always yields same output.
// It never touches the database or the filesystem.
//
// keywords is the user-provided wing → keyword slice map (typically sourced
// from HierarchyConfig.LegacyKeywords). If nil or empty, the classifier
// cannot decide and returns ("", 0.0, nil) — not an error, just "no config".
// This is intentional: the binary ships zero default keywords to remain
// locale and domain neutral for open-source deployments.
//
// If no wing has enough signal, the result has Wing="" and Confidence=0.
// Callers should treat that as "leave as legacy".
func ClassifyLegacyContent(content string, keywords map[string][]string) ClassifierResult {
	if strings.TrimSpace(content) == "" || len(keywords) == 0 {
		return ClassifierResult{}
	}

	normalized := normalizeForMatching(content)

	// Count hits per wing.
	hits := make(map[string]int, len(keywords))
	matchedByWing := make(map[string][]string, len(keywords))
	for wing, kws := range keywords {
		for _, kw := range kws {
			if kw == "" {
				continue
			}
			// Keywords must go through the SAME normalization as the
			// content so that accented and non-accented variants match
			// each other (accent-stripped keywords match accent-stripped content).
			kwNorm := strings.ToLower(strings.TrimSpace(kw))
			kwNorm = StripAccents(kwNorm)
			if kwNorm == "" {
				continue
			}
			if strings.Contains(normalized, kwNorm) {
				hits[wing]++
				matchedByWing[wing] = append(matchedByWing[wing], kw)
			}
		}
	}

	if len(hits) == 0 {
		return ClassifierResult{}
	}

	// Find top and second.
	topWing := ""
	topHits := 0
	secondHits := 0
	for wing, h := range hits {
		if h > topHits {
			secondHits = topHits
			topHits = h
			topWing = wing
		} else if h > secondHits {
			secondHits = h
		}
	}

	// Apply minimum-hits floor.
	if topHits < ClassifierMinHits {
		return ClassifierResult{
			TopWingHits:    topHits,
			SecondWingHits: secondHits,
		}
	}

	// Apply dominance factor: top must beat second by the required margin.
	// A tie (topHits == secondHits) is ambiguous and gets rejected.
	if float64(topHits) < float64(secondHits)*ClassifierDominanceFactor {
		return ClassifierResult{
			TopWingHits:    topHits,
			SecondWingHits: secondHits,
		}
	}

	// Confidence scales with dominance. A file with 10 work hits and 0
	// personal hits scores higher than a file with 4 work and 2 personal.
	// Formula: 0.65 (floor) + up to 0.3 extra for dominance.
	confidence := ClassifierMinConfidence
	if secondHits == 0 {
		// Pure signal — no competition. Cap at 0.95.
		confidence = 0.85 + min(float64(topHits)/20.0, 0.1)
	} else {
		// Weighted by margin.
		ratio := float64(topHits) / float64(secondHits)
		// ratio == 2 → +0 extra, ratio == 6 → +0.2 extra
		extra := min((ratio-ClassifierDominanceFactor)*0.05, 0.25)
		confidence = ClassifierMinConfidence + extra
	}
	if confidence > 0.95 {
		confidence = 0.95
	}

	return ClassifierResult{
		Wing:            NormalizeWing(topWing),
		Confidence:      confidence,
		MatchedKeywords: matchedByWing[topWing],
		TopWingHits:     topHits,
		SecondWingHits:  secondHits,
	}
}

// normalizeForMatching lowercases and strips accents from content so
// that keyword matching is case-insensitive and accent-insensitive.
// Does NOT tokenize — keywords can span multiple words.
func normalizeForMatching(s string) string {
	s = strings.ToLower(s)
	s = StripAccents(s)

	// Replace non-alphanumeric with spaces for word-boundary matching.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	// Collapse consecutive spaces.
	out := b.String()
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return " " + strings.TrimSpace(out) + " " // space-pad for boundary matches
}


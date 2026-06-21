// Package memory — quality.go ports the mechanical quality scorer used by the
// Memory v2 curation pipeline (US-002/US-003 legacy import).
//
// The scorer is a pure, deterministic heuristic: it never calls an LLM and has
// no DB or filesystem dependencies. It assigns a [0,1] score to a memory's text
// given its category and whether it carries a scope/wing signal, then a thin
// classifier maps a sub-threshold score (or an obvious bloat pattern) to a
// curation status the read-side lifecycle guard already understands
// (curation_status='low_signal'; see chunkLifecycleGuard in sqlite_store.go).
//
// Ported from ../anchored/pkg/memory/quality.go (ScoreQuality + RecurateMetadata
// curation branch). DevClaw has no first-class "project" scope, so the original
// hasProject boost is fed by scope/wing presence instead (see ScoreQuality doc).
// The weights are preserved verbatim so the two implementations agree.
package memory

import (
	"regexp"
	"strings"
	"unicode"
)

// LowSignalThreshold is the quality score below which a memory is demoted to
// curation_status='low_signal'. Kept identical to the anchored scorer threshold
// so import and serve-time curation never diverge.
const LowSignalThreshold = 0.55

// Curation status / rule string constants. These are the literal values the
// read-side lifecycle guard filters on (curation_status='low_signal'); keep
// them in sync with chunkLifecycleGuard in sqlite_store.go.
const (
	CurationStatusLowSignal = "low_signal"

	CurationRuleQuality       = "quality"
	CurationRuleContradiction = "contradiction"
)

// QualityScorerVersion identifies the scoring algorithm revision. Persisted in
// chunks.scorer_version so a future weight change can re-curate selectively.
const QualityScorerVersion = 3

var (
	// testOutputRe matches transient test-run chatter ("12 passed", "go test",
	// "pytest", "suite completa", …) that pollutes long-term memory.
	testOutputRe = regexp.MustCompile(`(?i)\b(\d+\s+(passed|failed|skipped)|0\s+failures?|testes?\s+passando|suite\s+completa|rodando\s+suite|go\s+test|pytest|npm\s+test)\b`)
	// progressRe matches ephemeral progress narration.
	progressRe = regexp.MustCompile(`(?i)\b(corrigido|rodando|testando|retestar)\b`)
	// terminalRe matches raw terminal error / stack-trace fragments.
	terminalRe = regexp.MustCompile(`(?i)(^|\n)\s*(error:|warning:|panic:|traceback|stack trace|expected|actual|assert)\b`)
)

// ScoreQuality assigns a mechanical quality score in [0,1] to a memory.
//
// category is the entry category (fact, decision, learning, summary, plan,
// event, preference, …). hasScope reports whether the memory carries a
// scope/wing signal — DevClaw's analogue of the anchored scorer's "hasProject"
// boost (a scoped memory is more durable than a free-floating one).
//
// Weights are preserved from the anchored implementation:
//   - base 0.62
//   - +0.18 decision/learning, +0.08 summary/plan, -0.20 event/preference
//   - +0.12 if scoped, -0.08 if not
//   - length: -0.42 (<40 chars), -0.24 (<90), +0.08 (>220)
//   - -0.18 if fewer than 6 words
//   - -0.32 test-output, -0.28 progress, -0.18 terminal-error regex
//   - -0.10 fact with >12 newlines; -0.12 punctuation-heavy short text
//   - clamped to [0,1]
func ScoreQuality(content, category string, hasScope bool) float64 {
	text := strings.TrimSpace(content)
	if text == "" {
		return 0
	}

	score := 0.62
	chars := len([]rune(text))
	words := strings.Fields(text)

	switch category {
	case "decision", "learning":
		score += 0.18
	case "summary", "plan":
		score += 0.08
	case "event", "preference":
		score -= 0.2
	}

	if hasScope {
		score += 0.12
	} else {
		score -= 0.08
	}

	switch {
	case chars < 40:
		score -= 0.42
	case chars < 90:
		score -= 0.24
	case chars > 220:
		score += 0.08
	}

	if len(words) < 6 {
		score -= 0.18
	}
	if testOutputRe.MatchString(text) {
		score -= 0.32
	}
	if progressRe.MatchString(text) {
		score -= 0.28
	}
	if terminalRe.MatchString(text) {
		score -= 0.18
	}
	if strings.Count(text, "\n") > 12 && category == "fact" {
		score -= 0.1
	}
	if punctuationRatio(text) > 0.24 && chars < 180 {
		score -= 0.12
	}

	return clampQuality(score)
}

// QualityVerdict is the curation outcome for a single memory.
type QualityVerdict struct {
	// Score is the mechanical quality score in [0,1].
	Score float64
	// CurationStatus is "" (keep) or CurationStatusLowSignal (hide on recall).
	CurationStatus string
	// CurationRule names the rule that produced a non-empty CurationStatus.
	CurationRule string
}

// ClassifyQuality scores the content and decides whether it should be demoted
// to low_signal. Pinned memories are never demoted (mirrors the anchored
// RecurateMetadata pin exemption).
func ClassifyQuality(content, category string, hasScope, pinned bool) QualityVerdict {
	score := ScoreQuality(content, category, hasScope)
	v := QualityVerdict{Score: score}
	if score < LowSignalThreshold && !pinned {
		v.CurationStatus = CurationStatusLowSignal
		v.CurationRule = CurationRuleQuality
	}
	return v
}

func punctuationRatio(s string) float64 {
	if s == "" {
		return 0
	}
	total := 0
	punct := 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			punct++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(punct) / float64(total)
}

func clampQuality(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

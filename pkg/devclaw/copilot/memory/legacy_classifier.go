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

// defaultLegacyKeywords is the default keyword-to-wing mapping used by
// ClassifyLegacyContent. Each wing has a list of keywords that strongly
// suggest that wing when they appear in a memory's content.
//
// This variable is INTENTIONALLY unexported so that package clients cannot
// mutate it (which would create data races and test pollution). Clients
// that need to read the defaults should call LegacyKeywordsCopy().
// Clients that need custom keywords should call ClassifyLegacyContentWith
// with their own map.
//
// COUPLING NOTE (ME-4 architectural concern from Sprint 1 review):
//
// Every keyword added here MUST survive the classifier normalization
// pipeline: strings.ToLower + stripAccents. Keywords like "CEO" or "2FA"
// would silently fail to match because:
//   - "CEO" normalizes to "ceo", which then gets matched case-insensitively → OK
//   - "2FA" normalizes to "2fa" → OK  (digits are preserved)
//   - "café" normalizes to "cafe" → OK (accent stripping is symmetric)
//
// What does NOT work:
//   - Keywords containing non-ASCII letters outside Latin-1 (e.g., "仕事",
//     "Μόσχα", "مرحبا"): the classifier's normalizeForMatching preserves
//     them but ASCII-only content never matches. Safe to include but
//     won't fire.
//   - Keywords with trailing/leading whitespace: trimmed by ClassifyLegacyContentWith.
//   - Empty strings: skipped.
//
// Keywords are matched case-insensitively on word boundaries. Portuguese
// and English terms are mixed because DevClaw users are primarily
// Brazilian developers.
//
// Adding new keywords is safe as long as each keyword is specific enough
// that its appearance genuinely signals the wing. Generic words like
// "dia", "hora", "fazer" are intentionally excluded — they appear in
// all contexts and would pollute the signal.
var defaultLegacyKeywords = map[string][]string{
	"work": {
		// Software development
		"sprint", "standup", "retro", "retrospective", "backlog",
		"pull request", "pr ", "merge request", "mr ", "commit",
		"deploy", "deployment", "rollback", "hotfix", "patch",
		"bug", "issue", "ticket", "jira", "github", "gitlab",
		"release", "changelog", "version", "semver",
		"unit test", "integration test", "e2e", "ci ", "cd ",
		"pipeline", "build", "staging", "production",
		"kubernetes", "docker", "terraform", "ansible",
		"database", "migration", "schema", "query",
		"api", "endpoint", "rest", "graphql", "webhook",
		"auth", "oauth", "jwt", "session",
		"refactor", "rewrite", "feature flag",
		// Meetings / corp
		"meeting", "reunião", "1:1", "one on one", "sync",
		"manager", "chefe", "gerente", "diretor",
		"deadline", "prazo", "milestone", "okr", "kpi",
		"sprint planning", "grooming", "refinement",
		// Work tools
		"slack channel", "jira ticket", "confluence", "notion",
		"linear", "asana", "trello",
	},
	"personal": {
		"hobby", "game", "gaming", "stream", "twitch",
		"book", "livro", "ler ", "lendo",
		"filme", "movie", "série", "series", "netflix", "disney",
		"workout", "gym", "academia", "corrida", "correr",
		"projeto pessoal", "side project", "side-project",
		"estudo pessoal", "curso ", "course",
		"amigo", "amigos", "friend", "friends",
	},
	"family": {
		// Portuguese family terms
		"filha", "filho", "filhos", "filhas",
		"esposa", "marido", "parceira", "parceiro",
		"mãe", "pai", "avó", "avô", "vovó", "vovô",
		"irmã", "irmão", "tia", "tio", "primo", "prima",
		"sobrinha", "sobrinho",
		"escola", "colégio", "professor", "professora",
		"boletim", "reunião de pais", "aniversário", "festa",
		// English
		"mom ", "dad ", "mother", "father",
		"wife", "husband", "spouse", "partner",
		"son ", "daughter", "kids", "children",
		"sister", "brother", "aunt", "uncle", "grandma", "grandpa",
		"school", "parent-teacher", "birthday party",
	},
	"finance": {
		// Portuguese finance terms
		"fatura", "boleto", "conta", "banco", "cartão",
		"crédito", "débito", "dívida", "empréstimo",
		"investimento", "poupança", "renda",
		"imposto", "irpf", "receita federal",
		"corretora", "ações", "fundos imobiliários", "fii",
		"dólar", "euro", "bitcoin", "cripto",
		// English
		"invoice", "bill", "bank", "credit card",
		"loan", "mortgage", "savings", "budget",
		"investment", "portfolio", "stocks", "bonds",
		"tax", "irs", "paycheck", "salary",
	},
	"health": {
		"médico", "doctor", "consulta", "appointment",
		"exame", "exam", "blood test", "checkup",
		"remédio", "medicine", "medication", "prescription",
		"dor", "pain", "sintoma", "symptom",
		"hospital", "clínica", "clinic",
		"dentista", "dentist",
		"fisioterapia", "physio", "therapy",
		"pressão", "blood pressure", "glicemia",
	},
	"learning": {
		"tutorial", "curso online", "coursera", "udemy",
		"bootcamp", "workshop", "seminar", "webinar",
		"livro técnico", "textbook", "reference",
		"blog post", "artigo", "paper", "whitepaper",
		"documentação", "documentation", "docs",
	},
}

// ClassifyLegacyContent applies the pattern-based classifier to a blob
// of text and returns its best guess at a wing assignment.
//
// The function is stateless and pure — same input always yields same
// output. It never touches the database or the filesystem.
//
// If no wing has enough signal, the result has Wing="" and Confidence=0.
// Callers should treat that as "leave as legacy".
//
// Content is matched against the default keyword dictionary. Pass a
// custom map via ClassifyLegacyContentWith to override.
func ClassifyLegacyContent(content string) ClassifierResult {
	return ClassifyLegacyContentWith(content, defaultLegacyKeywords)
}

// LegacyKeywordsCopy returns a defensive copy of the default keyword
// dictionary. Callers that need to INSPECT the defaults (e.g., an admin
// tool showing "which keywords trigger which wings?") should use this.
//
// Callers that need to CUSTOMIZE the dictionary should pass their own
// map to ClassifyLegacyContentWith — the default set is immutable by
// design.
func LegacyKeywordsCopy() map[string][]string {
	out := make(map[string][]string, len(defaultLegacyKeywords))
	for wing, kws := range defaultLegacyKeywords {
		out[wing] = append([]string(nil), kws...)
	}
	return out
}

// ClassifyLegacyContentWith is the configurable variant of
// ClassifyLegacyContent. It accepts a custom keyword map, which is
// useful for tests and for per-user customization in the future.
func ClassifyLegacyContentWith(content string, keywords map[string][]string) ClassifierResult {
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
			// each other. Without this, "mãe" in the dictionary never
			// matches a content that normalizes to "mae".
			kwNorm := strings.ToLower(strings.TrimSpace(kw))
			kwNorm = stripAccents(kwNorm)
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
	// A tie (topHits == secondHits, e.g., 4 work + 4 family) is ambiguous
	// and gets rejected.
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
	s = stripAccents(s)

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


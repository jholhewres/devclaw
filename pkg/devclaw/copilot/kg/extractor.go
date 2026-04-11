package kg

import (
	"context"
	_ "embed"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg/patterns"
)

//go:embed patterns/pt-br.yaml
var defaultPatternsYAML []byte

var (
	defaultPatternSetsOnce sync.Once
	defaultPatternSetsVal  []*patterns.PatternSet
)

// DefaultPatternSets returns the built-in pattern sets (pt-br).
// Parsed once and cached via sync.Once. Returns nil on parse error.
func DefaultPatternSets() []*patterns.PatternSet {
	defaultPatternSetsOnce.Do(func() {
		ps, err := patterns.Load(defaultPatternsYAML)
		if err == nil {
			defaultPatternSetsVal = []*patterns.PatternSet{ps}
		}
	})
	return defaultPatternSetsVal
}

// Extractor scans free text for SPO triples using configurable regex patterns.
type Extractor struct {
	patterns   []*compiledPattern
	clauseSeps []string
	logger     *slog.Logger
}

type compiledPattern struct {
	Name       string
	Predicate  string
	Functional bool
	Confidence float64
	Regexps    []*regexp.Regexp
}

// ExtractionResult is a single (subject, predicate, object) triple found in text.
type ExtractionResult struct {
	Subject    string
	Predicate  string
	Object     string
	Confidence float64
	RawText    string
}

// NewExtractor compiles all templates from the given pattern sets into regexps.
// Templates are accent-stripped so that normalized input text matches regardless
// of diacritics. Clause separators from each PatternSet are collected for
// sentence splitting before pattern matching.
func NewExtractor(patternSets []*patterns.PatternSet, logger *slog.Logger) (*Extractor, error) {
	if logger == nil {
		logger = slog.Default()
	}
	var compiled []*compiledPattern
	var seps []string
	for _, ps := range patternSets {
		for _, s := range ps.ClauseSeparators {
			normalized := stripAccents(strings.ToLower(s))
			if normalized != "" {
				seps = append(seps, normalized)
			}
		}
		for _, p := range ps.Patterns {
			cp := &compiledPattern{
				Name:       p.Name,
				Predicate:  p.Predicate,
				Functional: p.Functional,
				Confidence: p.Confidence,
			}
			if cp.Confidence <= 0 {
				cp.Confidence = 0.5
			}
			for _, tmpl := range p.Templates {
				expanded := stripAccents(patterns.ExpandTemplate(tmpl))
				re, err := regexp.Compile(expanded)
				if err != nil {
					return nil, err
				}
				cp.Regexps = append(cp.Regexps, re)
			}
			compiled = append(compiled, cp)
		}
	}
	return &Extractor{patterns: compiled, clauseSeps: seps, logger: logger}, nil
}

// Extract scans text and returns all SPO triples matched by the compiled patterns.
// The text is first split into clauses on the configured clause separators so that
// multi-predicate sentences are handled correctly. Subsequent clauses inherit the
// subject from the preceding clause when the clause begins with a verb phrase.
func (e *Extractor) Extract(text string) []ExtractionResult {
	clauses := e.splitClauses(text)
	var results []ExtractionResult
	var lastSubject string

	for _, clause := range clauses {
		normalized := stripAccents(strings.ToLower(clause))
		clauseResults := e.matchClause(normalized)
		if len(clauseResults) > 0 {
			for i := range clauseResults {
				if clauseResults[i].Subject != "" {
					lastSubject = clauseResults[i].Subject
				}
			}
			results = append(results, clauseResults...)
		} else if lastSubject != "" {
			// Clause produced no results — try prepending last known subject.
			injected := lastSubject + " " + normalized
			clauseResults = e.matchClause(injected)
			if len(clauseResults) > 0 {
				results = append(results, clauseResults...)
			}
		}
	}

	// Set RawText to original (non-normalized) input.
	for i := range results {
		results[i].RawText = text
	}
	return results
}

func (e *Extractor) matchClause(clause string) []ExtractionResult {
	clause = strings.TrimSpace(clause)
	if clause == "" {
		return nil
	}
	var results []ExtractionResult
	for _, cp := range e.patterns {
		for _, re := range cp.Regexps {
			match := re.FindStringSubmatchIndex(clause)
			if match == nil {
				continue
			}
			subject := extractNamedGroup(re, clause, match, "subject")
			object := extractNamedGroup(re, clause, match, "object")
			subject = strings.TrimSpace(subject)
			object = strings.TrimSpace(object)
			if subject == "" || object == "" {
				continue
			}
			results = append(results, ExtractionResult{
				Subject:    subject,
				Predicate:  cp.Predicate,
				Object:     object,
				Confidence: cp.Confidence,
			})
		}
	}
	return results
}

func (e *Extractor) splitClauses(text string) []string {
	if len(e.clauseSeps) == 0 {
		return []string{text}
	}
	parts := []string{text}
	for _, sep := range e.clauseSeps {
		var next []string
		for _, p := range parts {
			split := strings.Split(p, sep)
			next = append(next, split...)
		}
		parts = next
	}
	return parts
}

// ExtractAndStore extracts triples from text and persists them to the knowledge graph.
// Returns the count of successfully inserted triples. Individual insertion errors are
// logged but do not abort the batch.
func (e *Extractor) ExtractAndStore(ctx context.Context, kg *KG, text string, wing string, sourceMemoryID string) (int, error) {
	results := e.Extract(text)
	inserted := 0
	for _, r := range results {
		_, err := kg.AddTriple(ctx, r.Subject, r.Predicate, r.Object, TripleOpts{
			Confidence:     r.Confidence,
			SourceMemoryID: sourceMemoryID,
			Wing:           wing,
			RawText:        r.RawText,
		})
		if err != nil {
			e.logger.Error("extractor: failed to store triple",
				"subject", r.Subject,
				"predicate", r.Predicate,
				"object", r.Object,
				"error", err)
			continue
		}
		inserted++
	}
	return inserted, nil
}

// extractNamedGroup returns the value of the named capture group from a submatch.
func extractNamedGroup(re *regexp.Regexp, s string, loc []int, name string) string {
	for i, groupName := range re.SubexpNames() {
		if groupName == name {
			if loc[2*i] >= 0 && loc[2*i+1] >= 0 {
				return s[loc[2*i]:loc[2*i+1]]
			}
		}
	}
	return ""
}

package kg

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// LLMProvider is the interface for LLM calls (mockable for tests).
type LLMProvider interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// ExtractionConfig holds LLM extractor configuration.
type ExtractionConfig struct {
	// AutoExtract controls which extraction modes are active.
	// Values: "off" (default), "pattern", "llm", "both".
	AutoExtract string

	// LLMBudgetPerCycle is the max memories to process per dream cycle.
	// Default: 20.
	LLMBudgetPerCycle int

	// LLMConsentACK must be true when AutoExtract contains "llm".
	// This is a safety gate — the operator must explicitly acknowledge
	// that memory contents will be sent to an external LLM API.
	LLMConsentACK bool
}

// LLMExtractor extracts triples using an LLM provider.
//
// It is off by default (AutoExtract="off") and requires explicit consent
// (LLMConsentACK=true) before any LLM call is made. A circuit breaker
// prevents cascading failures: after 3 consecutive errors the extractor
// pauses for 30 minutes.
type LLMExtractor struct {
	kg         *KG
	provider   LLMProvider
	config     ExtractionConfig
	promptTmpl string
	logger     *slog.Logger

	// Circuit breaker state (protected by mu).
	mu               sync.Mutex
	consecutiveFails int
	circuitOpen      bool
	circuitOpenUntil time.Time
}

// TripleJSON is a single (subject, predicate, object) triple from the LLM response.
type TripleJSON struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
}

// ExtractionResponse is the structured output format expected from the LLM.
type ExtractionResponse struct {
	Triples []TripleJSON `json:"triples"`
}

const (
	// circuitBreakerThreshold is the number of consecutive failures before
	// the circuit opens.
	circuitBreakerThreshold = 3

	// circuitBreakerCooldown is how long the circuit stays open.
	circuitBreakerCooldown = 30 * time.Minute

	// defaultLLMBudgetPerCycle is the default number of memories to process
	// per dream cycle.
	defaultLLMBudgetPerCycle = 20

	// promptPlaceholderEntities is the template placeholder for known entities.
	promptPlaceholderEntities = "{{KNOWN_ENTITIES}}"

	// promptPlaceholderText is the template placeholder for input text.
	promptPlaceholderText = "{{TEXT}}"
)

// NewLLMExtractor creates a new LLM-backed triple extractor.
//
// Validation rules:
//   - kg must not be nil
//   - provider must not be nil
//   - If config.AutoExtract contains "llm" then config.LLMConsentACK must be true
//   - The prompt template at prompts/extractor.txt must be loadable via
//     the embedded FS or a default is used
func NewLLMExtractor(kg *KG, provider LLMProvider, config ExtractionConfig, logger *slog.Logger) (*LLMExtractor, error) {
	if kg == nil {
		return nil, fmt.Errorf("llm_extractor: nil kg")
	}
	if provider == nil {
		return nil, fmt.Errorf("llm_extractor: nil provider")
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Validate consent when LLM mode is requested.
	if isLLMMode(config.AutoExtract) && !config.LLMConsentACK {
		return nil, fmt.Errorf("llm_extractor: LLM consent not acknowledged — set llm_consent_acknowledged=true to enable LLM extraction")
	}

	// Set defaults.
	if config.LLMBudgetPerCycle <= 0 {
		config.LLMBudgetPerCycle = defaultLLMBudgetPerCycle
	}
	if config.AutoExtract == "" {
		config.AutoExtract = "off"
	}

	tmpl := defaultPromptTemplate

	return &LLMExtractor{
		kg:         kg,
		provider:   provider,
		config:     config,
		promptTmpl: tmpl,
		logger:     logger.With("component", "llm_extractor"),
	}, nil
}

// ExtractFromText calls the LLM provider with the configured prompt template
// and parses the JSON response into structured triples.
//
// The caller is responsible for budget enforcement — this method does not
// track per-cycle counts.
func (e *LLMExtractor) ExtractFromText(ctx context.Context, text string, knownEntities []string) ([]TripleJSON, error) {
	if e.isCircuitOpen() {
		return nil, fmt.Errorf("llm_extractor: circuit breaker open")
	}

	prompt := e.buildPrompt(text, knownEntities)

	raw, err := e.provider.Complete(ctx, prompt)
	if err != nil {
		e.recordFailure()
		return nil, fmt.Errorf("llm_extractor: LLM call failed: %w", err)
	}

	resp, err := parseResponse(raw)
	if err != nil {
		e.recordFailure()
		return nil, fmt.Errorf("llm_extractor: parse response: %w", err)
	}

	e.recordSuccess()
	return resp.Triples, nil
}

// ExtractAndStore runs the full pipeline: extract triples from text via LLM,
// filter out PII predicates, and store valid triples in the knowledge graph.
//
// Returns the count of successfully inserted triples. Individual insertion
// errors are logged but do not abort the batch. PII-blacklisted predicates
// are silently dropped.
func (e *LLMExtractor) ExtractAndStore(ctx context.Context, text, wing, sourceMemoryID string, knownEntities []string) (int, error) {
	triples, err := e.ExtractFromText(ctx, text, knownEntities)
	if err != nil {
		return 0, err
	}

	inserted := 0
	for _, t := range triples {
		// PII filter — drop blacklisted predicates.
		if IsPIIPredicate(t.Predicate) {
			e.logger.Warn("llm_extractor: dropping PII predicate",
				"predicate", t.Predicate,
				"subject", t.Subject,
				"source_memory_id", sourceMemoryID,
			)
			continue
		}

		// Skip triples with empty fields (malformed output).
		if t.Subject == "" || t.Predicate == "" || t.Object == "" {
			e.logger.Warn("llm_extractor: skipping triple with empty field",
				"subject", t.Subject,
				"predicate", t.Predicate,
				"object", t.Object,
			)
			continue
		}

		_, err := e.kg.AddTriple(ctx, t.Subject, t.Predicate, t.Object, TripleOpts{
			Confidence:     0.7, // LLM-extracted triples get a fixed confidence
			SourceMemoryID: sourceMemoryID,
			Wing:           wing,
			RawText:        text,
		})
		if err != nil {
			e.logger.Error("llm_extractor: failed to store triple",
				"subject", t.Subject,
				"predicate", t.Predicate,
				"object", t.Object,
				"error", err,
			)
			continue
		}
		inserted++
	}

	return inserted, nil
}

// Config returns a copy of the extraction config.
func (e *LLMExtractor) Config() ExtractionConfig {
	return e.config
}

// isCircuitOpen reports whether the circuit breaker is currently open.
// When open, all extraction calls are rejected without hitting the LLM.
func (e *LLMExtractor) isCircuitOpen() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.circuitOpen {
		return false
	}

	// Auto-recover after cooldown.
	if time.Now().After(e.circuitOpenUntil) {
		e.circuitOpen = false
		e.consecutiveFails = 0
		return false
	}

	return true
}

// recordFailure increments the consecutive failure counter. After
// circuitBreakerThreshold failures the circuit opens for
// circuitBreakerCooldown duration.
func (e *LLMExtractor) recordFailure() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.consecutiveFails++
	if e.consecutiveFails >= circuitBreakerThreshold {
		e.circuitOpen = true
		e.circuitOpenUntil = time.Now().Add(circuitBreakerCooldown)
		e.logger.Warn("llm_extractor: circuit breaker opened",
			"consecutive_fails", e.consecutiveFails,
			"open_until", e.circuitOpenUntil,
		)
	}
}

// recordSuccess resets the consecutive failure counter and closes the circuit.
func (e *LLMExtractor) recordSuccess() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.consecutiveFails = 0
	e.circuitOpen = false
}

// buildPrompt replaces template placeholders with actual values.
func (e *LLMExtractor) buildPrompt(text string, knownEntities []string) string {
	prompt := e.promptTmpl

	entitiesStr := "none"
	if len(knownEntities) > 0 {
		entitiesStr = strings.Join(knownEntities, ", ")
	}

	prompt = strings.ReplaceAll(prompt, promptPlaceholderEntities, entitiesStr)
	prompt = strings.ReplaceAll(prompt, promptPlaceholderText, text)

	return prompt
}

// parseResponse extracts the JSON object from the LLM output.
// Fail-closed: any parse error returns an empty extraction, not a crash.
func parseResponse(raw string) (*ExtractionResponse, error) {
	// Trim whitespace and any surrounding markdown code fences.
	cleaned := strings.TrimSpace(raw)

	// Strip markdown code fences if present.
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	} else if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	var resp ExtractionResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	return &resp, nil
}

// isLLMMode returns true if the auto-extract config enables LLM extraction.
func isLLMMode(mode string) bool {
	return mode == "llm" || mode == "both"
}

// IsAutoExtractEnabled returns true if any extraction mode is enabled.
func IsAutoExtractEnabled(mode string) bool {
	return mode == "pattern" || mode == "llm" || mode == "both"
}

// defaultPromptTemplate is the inline fallback when the external file
// is not available. Kept in sync with prompts/extractor.txt.
const defaultPromptTemplate = `You are a knowledge graph extractor. Given a text snippet, extract factual triples in the form (subject, predicate, object).

Rules:
1. Only extract explicit facts stated in the text
2. Use snake_case for predicates (e.g., works_at, lives_in)
3. Normalize names to their canonical form
4. Do NOT extract: SSN, credit card numbers, passwords, addresses, CPF, RG, phone numbers, email addresses, bank accounts
5. If no triples can be extracted, return an empty array
6. Each triple must have a non-empty subject, predicate, and object
7. Use lowercase for all predicate names
8. Prefer existing entity names from the known entities list when they match

Respond ONLY with valid JSON in this exact format:
{"triples": [{"subject": "...", "predicate": "...", "object": "..."}]}

Do not include any explanation, markdown formatting, or text outside the JSON object.

Known entities (prefer these canonical names):
` + promptPlaceholderEntities + `

Text to analyze:
` + promptPlaceholderText

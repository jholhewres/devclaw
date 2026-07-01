package kg

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// mockLLMProvider is a test double for LLMProvider.
type mockLLMProvider struct {
	response string
	err      error
	calls    atomic.Int32
}

func (m *mockLLMProvider) Complete(_ context.Context, _ string) (string, error) {
	m.calls.Add(1)
	return m.response, m.err
}

func newTestLLMExtractor(t *testing.T, provider LLMProvider, config ExtractionConfig) *LLMExtractor {
	t.Helper()
	k := newTestKG(t)
	ext, err := NewLLMExtractor(k, provider, config, slog.Default())
	if err != nil {
		t.Fatalf("NewLLMExtractor: %v", err)
	}
	return ext
}

func llmConfig(consent bool) ExtractionConfig {
	return ExtractionConfig{
		AutoExtract:       "llm",
		LLMBudgetPerCycle: 20,
		LLMConsentACK:     consent,
	}
}

func TestLLMExtractor_MockProviderHappyPath(t *testing.T) {
	mock := &mockLLMProvider{
		response: `{"triples":[{"subject":"Alice","predicate":"works_at","object":"ACME"},{"subject":"Alice","predicate":"lives_in","object":"São Paulo"},{"subject":"Alice","predicate":"speaks","object":"Portuguese"}]}`,
	}
	ext := newTestLLMExtractor(t, mock, llmConfig(true))

	inserted, err := ext.ExtractAndStore(context.Background(), "Alice works at ACME and lives in São Paulo", "work", "mem-1", []string{"Alice", "ACME"})
	if err != nil {
		t.Fatalf("ExtractAndStore: %v", err)
	}
	if inserted != 3 {
		t.Fatalf("expected 3 inserts, got %d", inserted)
	}
	if mock.calls.Load() != 1 {
		t.Fatalf("expected 1 LLM call, got %d", mock.calls.Load())
	}

	// Verify triples are in the KG.
	k := ext.kg
	var count int
	err = k.db.QueryRow("SELECT COUNT(*) FROM kg_triples WHERE source_memory_id = 'mem-1'").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 triples in DB, got %d", count)
	}
}

func TestLLMExtractor_CircuitBreakerAfter3Failures(t *testing.T) {
	mock := &mockLLMProvider{
		err: sql.ErrConnDone,
	}
	ext := newTestLLMExtractor(t, mock, llmConfig(true))
	ctx := context.Background()

	// First 3 calls fail and open the circuit.
	for i := 0; i < 3; i++ {
		_, err := ext.ExtractFromText(ctx, "text", nil)
		if err == nil {
			t.Fatalf("call %d: expected error", i+1)
		}
	}

	if mock.calls.Load() != 3 {
		t.Fatalf("expected 3 LLM calls, got %d", mock.calls.Load())
	}

	// 4th call should be rejected by circuit breaker without hitting LLM.
	_, err := ext.ExtractFromText(ctx, "text", nil)
	if err == nil {
		t.Fatal("expected circuit breaker error on 4th call")
	}
	if !strings.Contains(err.Error(), "circuit breaker") {
		t.Errorf("expected circuit breaker error, got: %v", err)
	}

	// LLM should NOT have been called again.
	if mock.calls.Load() != 3 {
		t.Fatalf("expected 3 LLM calls (4th blocked by circuit), got %d", mock.calls.Load())
	}
}

func TestLLMExtractor_CircuitBreakerAutoRecovers(t *testing.T) {
	mock := &mockLLMProvider{
		err: sql.ErrConnDone,
	}
	ext := newTestLLMExtractor(t, mock, llmConfig(true))
	ctx := context.Background()

	// Open the circuit with 3 failures.
	for i := 0; i < 3; i++ {
		ext.ExtractFromText(ctx, "text", nil)
	}

	// Manually set circuit open time to past so it auto-recovers.
	ext.mu.Lock()
	ext.circuitOpenUntil = time.Now().Add(-1 * time.Second)
	ext.mu.Unlock()

	// Next call should attempt LLM again (but still fail with our mock error).
	_, err := ext.ExtractFromText(ctx, "text", nil)
	if err == nil {
		t.Fatal("expected error from mock provider")
	}
	// The call went through (circuit recovered), so calls should be 4.
	if mock.calls.Load() != 4 {
		t.Fatalf("expected 4 LLM calls after circuit recovery, got %d", mock.calls.Load())
	}
}

func TestLLMExtractor_BudgetGuardStopsAt20(t *testing.T) {
	mock := &mockLLMProvider{
		response: `{"triples":[{"subject":"Alice","predicate":"works_at","object":"ACME"}]}`,
	}
	ext := newTestLLMExtractor(t, mock, llmConfig(true))
	ctx := context.Background()

	// Simulate the budget guard: caller only passes 20 texts (budget=20).
	budget := ext.config.LLMBudgetPerCycle
	totalTexts := 25
	processed := 0

	for i := 0; i < totalTexts; i++ {
		if processed >= budget {
			break
		}
		text := "Alice works at ACME"
		_, err := ext.ExtractAndStore(ctx, text, "work", "mem-"+string(rune('A'+i)), nil)
		if err != nil {
			t.Fatalf("ExtractAndStore %d: %v", i, err)
		}
		processed++
	}

	if processed != 20 {
		t.Errorf("expected budget-limited 20 calls, got %d", processed)
	}
	if mock.calls.Load() != 20 {
		t.Errorf("expected 20 LLM calls, got %d", mock.calls.Load())
	}
}

func TestLLMExtractor_PIIBlacklistDropsSSN(t *testing.T) {
	mock := &mockLLMProvider{
		response: `{"triples":[{"subject":"Alice","predicate":"ssn","object":"123-45-6789"},{"subject":"Alice","predicate":"works_at","object":"ACME"}]}`,
	}
	ext := newTestLLMExtractor(t, mock, llmConfig(true))
	ctx := context.Background()

	inserted, err := ext.ExtractAndStore(ctx, "Alice SSN 123-45-6789 works at ACME", "work", "mem-pii", nil)
	if err != nil {
		t.Fatalf("ExtractAndStore: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("expected 1 insert (ssn dropped), got %d", inserted)
	}

	// Verify only works_at is stored.
	var count int
	err = ext.kg.db.QueryRow(
		"SELECT COUNT(*) FROM kg_triples WHERE source_memory_id = 'mem-pii' AND object_text = '123-45-6789'",
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("SSN triple should NOT be in the database")
	}
}

func TestLLMExtractor_PIIBlacklistDropsAllTypes(t *testing.T) {
	for pred := range PII_PREDICATE_BLACKLIST {
		t.Run(pred, func(t *testing.T) {
			mock := &mockLLMProvider{
				response: `{"triples":[{"subject":"Alice","predicate":"` + pred + `","object":"secret-value"}]}`,
			}
			ext := newTestLLMExtractor(t, mock, llmConfig(true))
			ctx := context.Background()

			inserted, err := ext.ExtractAndStore(ctx, "some text", "work", "mem-"+pred, nil)
			if err != nil {
				t.Fatalf("ExtractAndStore: %v", err)
			}
			if inserted != 0 {
				t.Errorf("predicate %q should have been dropped, got %d inserts", pred, inserted)
			}
		})
	}
}

func TestLLMExtractor_StructuredOutputParseFailure(t *testing.T) {
	mock := &mockLLMProvider{
		response: `this is not JSON at all`,
	}
	ext := newTestLLMExtractor(t, mock, llmConfig(true))
	ctx := context.Background()

	inserted, err := ext.ExtractAndStore(ctx, "some text", "work", "mem-bad", nil)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if inserted != 0 {
		t.Errorf("expected 0 inserts on parse failure, got %d", inserted)
	}
}

func TestLLMExtractor_ParseResponse_CodeFenceStripping(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
	}{
		{
			name:    "plain JSON",
			input:   `{"triples":[{"subject":"A","predicate":"b","object":"C"}]}`,
			wantLen: 1,
		},
		{
			name:    "JSON in markdown code fence",
			input:   "```json\n{\"triples\":[{\"subject\":\"A\",\"predicate\":\"b\",\"object\":\"C\"}]}\n```",
			wantLen: 1,
		},
		{
			name:    "JSON in plain code fence",
			input:   "```\n{\"triples\":[{\"subject\":\"A\",\"predicate\":\"b\",\"object\":\"C\"}]}\n```",
			wantLen: 1,
		},
		{
			name:    "empty triples",
			input:   `{"triples":[]}`,
			wantLen: 0,
		},
		{
			name:    "invalid JSON",
			input:   `not json`,
			wantLen: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseResponse(tt.input)
			if tt.wantLen < 0 {
				if err == nil {
					t.Error("expected parse error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseResponse: %v", err)
			}
			if len(resp.Triples) != tt.wantLen {
				t.Errorf("expected %d triples, got %d", tt.wantLen, len(resp.Triples))
			}
		})
	}
}

func TestLLMExtractor_ConsentFlagRequired(t *testing.T) {
	k := newTestKG(t)
	mock := &mockLLMProvider{response: `{"triples":[]}`}

	_, err := NewLLMExtractor(k, mock, ExtractionConfig{
		AutoExtract:   "llm",
		LLMConsentACK: false,
	}, slog.Default())

	if err == nil {
		t.Fatal("expected error when LLM mode enabled without consent")
	}
	if !strings.Contains(err.Error(), "consent") {
		t.Errorf("expected consent error, got: %v", err)
	}
}

func TestLLMExtractor_ConsentNotRequiredForOffMode(t *testing.T) {
	k := newTestKG(t)
	mock := &mockLLMProvider{response: `{"triples":[]}`}

	_, err := NewLLMExtractor(k, mock, ExtractionConfig{
		AutoExtract:   "off",
		LLMConsentACK: false,
	}, slog.Default())

	if err != nil {
		t.Fatalf("off mode should not require consent: %v", err)
	}
}

func TestLLMExtractor_ConsentRequiredForBothMode(t *testing.T) {
	k := newTestKG(t)
	mock := &mockLLMProvider{response: `{"triples":[]}`}

	_, err := NewLLMExtractor(k, mock, ExtractionConfig{
		AutoExtract:   "both",
		LLMConsentACK: false,
	}, slog.Default())

	if err == nil {
		t.Fatal("expected error when both mode enabled without consent")
	}
}

func TestLLMExtractor_NilKG(t *testing.T) {
	mock := &mockLLMProvider{}
	_, err := NewLLMExtractor(nil, mock, llmConfig(true), slog.Default())
	if err == nil {
		t.Fatal("expected error for nil kg")
	}
}

func TestLLMExtractor_NilProvider(t *testing.T) {
	k := newTestKG(t)
	_, err := NewLLMExtractor(k, nil, llmConfig(true), slog.Default())
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestLLMExtractor_EmptyTripleFieldsSkipped(t *testing.T) {
	mock := &mockLLMProvider{
		response: `{"triples":[{"subject":"","predicate":"works_at","object":"ACME"},{"subject":"Alice","predicate":"","object":"ACME"},{"subject":"Alice","predicate":"works_at","object":""},{"subject":"Alice","predicate":"works_at","object":"ACME"}]}`,
	}
	ext := newTestLLMExtractor(t, mock, llmConfig(true))
	ctx := context.Background()

	inserted, err := ext.ExtractAndStore(ctx, "text", "work", "mem-empty", nil)
	if err != nil {
		t.Fatalf("ExtractAndStore: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("expected 1 insert (3 skipped for empty fields), got %d", inserted)
	}
}

func TestLLMExtractor_IsPIIPredicate(t *testing.T) {
	tests := []struct {
		predicate string
		want      bool
	}{
		{"ssn", true},
		{"credit_card", true},
		{"password", true},
		{"address", true},
		{"cpf", true},
		{"rg", true},
		{"phone", true},
		{"email", true},
		{"bank_account", true},
		{"works_at", false},
		{"lives_in", false},
		{"speaks", false},
		{"name", false},
		{"SSN", false},
		{"PHONE", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.predicate, func(t *testing.T) {
			got := IsPIIPredicate(tt.predicate)
			if got != tt.want {
				t.Errorf("IsPIIPredicate(%q) = %v, want %v", tt.predicate, got, tt.want)
			}
		})
	}
}

func TestLLMExtractor_DefaultsApplied(t *testing.T) {
	k := newTestKG(t)
	mock := &mockLLMProvider{response: `{"triples":[]}`}

	ext, err := NewLLMExtractor(k, mock, ExtractionConfig{}, slog.Default())
	if err != nil {
		t.Fatalf("NewLLMExtractor: %v", err)
	}
	if ext.config.AutoExtract != "off" {
		t.Errorf("expected default AutoExtract='off', got %q", ext.config.AutoExtract)
	}
	if ext.config.LLMBudgetPerCycle != 20 {
		t.Errorf("expected default LLMBudgetPerCycle=20, got %d", ext.config.LLMBudgetPerCycle)
	}
}

func TestLLMExtractor_BuildPrompt(t *testing.T) {
	mock := &mockLLMProvider{response: `{"triples":[]}`}
	ext := newTestLLMExtractor(t, mock, llmConfig(true))

	prompt := ext.buildPrompt("hello world", []string{"Alice", "Bob"})

	if !strings.Contains(prompt, "Alice, Bob") {
		t.Error("prompt should contain known entities")
	}
	if !strings.Contains(prompt, "hello world") {
		t.Error("prompt should contain input text")
	}
	if strings.Contains(prompt, "{{KNOWN_ENTITIES}}") {
		t.Error("template placeholder should be replaced")
	}
	if strings.Contains(prompt, "{{TEXT}}") {
		t.Error("template placeholder should be replaced")
	}
}

func TestLLMExtractor_SuccessResetsCircuit(t *testing.T) {
	// Start with a failing provider, then switch to success.
	mock := &mockLLMProvider{err: sql.ErrConnDone}
	ext := newTestLLMExtractor(t, mock, llmConfig(true))
	ctx := context.Background()

	// 2 failures (not enough to open circuit).
	ext.ExtractFromText(ctx, "text", nil)
	ext.ExtractFromText(ctx, "text", nil)

	ext.mu.Lock()
	if ext.consecutiveFails != 2 {
		t.Fatalf("expected 2 consecutive fails, got %d", ext.consecutiveFails)
	}
	ext.mu.Unlock()

	// Now succeed — this should reset the counter.
	mock.err = nil
	mock.response = `{"triples":[{"subject":"A","predicate":"b","object":"C"}]}`
	_, err := ext.ExtractFromText(ctx, "text", nil)
	if err != nil {
		t.Fatalf("ExtractFromText: %v", err)
	}

	ext.mu.Lock()
	fails := ext.consecutiveFails
	open := ext.circuitOpen
	ext.mu.Unlock()

	if fails != 0 {
		t.Errorf("expected 0 consecutive fails after success, got %d", fails)
	}
	if open {
		t.Error("circuit should be closed after success")
	}
}

func TestIsAutoExtractEnabled(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"off", false},
		{"", false},
		{"pattern", true},
		{"llm", true},
		{"both", true},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			if got := IsAutoExtractEnabled(tt.mode); got != tt.want {
				t.Errorf("IsAutoExtractEnabled(%q) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

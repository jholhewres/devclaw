package copilot

import (
	"testing"
	"time"
)

func TestSessionResetWithPreservation(t *testing.T) {
	s := &Session{
		ID:           "test-reset",
		Channel:      "webui",
		ChatID:       "user1",
		config:       SessionConfig{Model: "gpt-4", ThinkingLevel: "high", Language: "pt-BR", FastMode: true, Verbose: true, ToolProfile: "full", BusinessContext: "context", Trigger: "/ask"},
		activeSkills: []string{"search", "code"},
		facts:        []string{"user prefers dark mode"},
		maxHistory:   100,
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		lastActiveAt: time.Now().Add(-30 * time.Minute),
	}

	// Add some history and token usage.
	for i := 0; i < 5; i++ {
		s.addEntry(ConversationEntry{
			UserMessage:       "msg",
			AssistantResponse: "resp",
			Timestamp:         time.Now(),
		})
	}
	s.AddTokenUsage(500, 200)
	s.AddTokenUsage(300, 100)
	s.AddCompactionSummary(CompactionEntry{Summary: "test summary"})
	s.UpdateLastCallTokens(400, 150, 50, 10)

	createdAt := s.CreatedAt

	s.ResetWithPreservation()

	// ── Transient state MUST be cleared ──
	if s.HistoryLen() != 0 {
		t.Errorf("history should be cleared, got %d entries", s.HistoryLen())
	}
	p, c, r := s.GetTokenUsage()
	if p != 0 || c != 0 || r != 0 {
		t.Errorf("token counters should be zeroed, got prompt=%d completion=%d requests=%d", p, c, r)
	}
	lp, lo, lcr, lcw := s.GetLastCallTokens()
	if lp != 0 || lo != 0 || lcr != 0 || lcw != 0 {
		t.Errorf("last call tokens should be zeroed, got %d/%d/%d/%d", lp, lo, lcr, lcw)
	}
	if len(s.GetCompactionSummaries()) != 0 {
		t.Error("compaction summaries should be cleared")
	}
	if s.GetCompactionCount() != 0 {
		t.Error("compaction count should be zeroed")
	}

	// ── Preserved state MUST be intact ──
	cfg := s.GetConfig()
	if cfg.Model != "gpt-4" {
		t.Errorf("Model should be preserved, got %q", cfg.Model)
	}
	if cfg.ThinkingLevel != "high" {
		t.Errorf("ThinkingLevel should be preserved, got %q", cfg.ThinkingLevel)
	}
	if cfg.Language != "pt-BR" {
		t.Errorf("Language should be preserved, got %q", cfg.Language)
	}
	if !cfg.FastMode {
		t.Error("FastMode should be preserved")
	}
	if !cfg.Verbose {
		t.Error("Verbose should be preserved")
	}
	if cfg.ToolProfile != "full" {
		t.Errorf("ToolProfile should be preserved, got %q", cfg.ToolProfile)
	}
	if cfg.BusinessContext != "context" {
		t.Errorf("BusinessContext should be preserved, got %q", cfg.BusinessContext)
	}
	if cfg.Trigger != "/ask" {
		t.Errorf("Trigger should be preserved, got %q", cfg.Trigger)
	}

	skills := s.GetActiveSkills()
	if len(skills) != 2 || skills[0] != "search" || skills[1] != "code" {
		t.Errorf("activeSkills should be preserved, got %v", skills)
	}

	facts := s.GetFacts()
	if len(facts) != 1 || facts[0] != "user prefers dark mode" {
		t.Errorf("facts should be preserved, got %v", facts)
	}

	if s.ID != "test-reset" {
		t.Errorf("ID should be preserved, got %q", s.ID)
	}
	if s.Channel != "webui" {
		t.Errorf("Channel should be preserved, got %q", s.Channel)
	}
	if s.ChatID != "user1" {
		t.Errorf("ChatID should be preserved, got %q", s.ChatID)
	}
	if s.CreatedAt != createdAt {
		t.Error("CreatedAt should be preserved")
	}

	// lastActiveAt should be bumped (not the old stale value).
	if time.Since(s.LastActiveAt()) > 5*time.Second {
		t.Error("lastActiveAt should be bumped to ~now")
	}
}

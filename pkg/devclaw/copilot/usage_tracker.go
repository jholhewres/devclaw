// Package copilot – usage_tracker.go records LLM token usage and estimated costs
// per session and globally.
package copilot

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ModelCost holds pricing per 1M tokens for a model.
type ModelCost struct {
	InputPer1M  float64 `yaml:"input_per_1m"`  // USD per 1M input tokens
	OutputPer1M float64 `yaml:"output_per_1m"` // USD per 1M output tokens
}

// SessionUsage holds token and cost stats for a session.
type SessionUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	Requests         int64
	EstimatedCostUSD float64
	FirstRequestAt   time.Time
	LastRequestAt    time.Time
}

// UsageTracker records usage per session and globally.
type UsageTracker struct {
	mu sync.RWMutex

	sessions   map[string]*SessionUsage
	global     *SessionUsage
	modelCosts map[string]ModelCost

	logger *slog.Logger
}

var defaultModelCosts = map[string]ModelCost{
	// OpenAI
	"gpt-4o":          {InputPer1M: 2.50, OutputPer1M: 10.00},
	"gpt-4o-mini":     {InputPer1M: 0.15, OutputPer1M: 0.60},
	"gpt-4.5-preview": {InputPer1M: 75.00, OutputPer1M: 150.00},
	"gpt-5":           {InputPer1M: 2.00, OutputPer1M: 8.00},
	"gpt-5-mini":      {InputPer1M: 0.15, OutputPer1M: 0.60},
	// Anthropic
	"claude-opus-4.6":   {InputPer1M: 5.00, OutputPer1M: 25.00},
	"claude-opus-4.5":   {InputPer1M: 5.00, OutputPer1M: 25.00},
	"claude-sonnet-4.5": {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-3.5-sonnet": {InputPer1M: 3.00, OutputPer1M: 15.00},
	// GLM (Z.AI)
	"glm-5":           {InputPer1M: 1.00, OutputPer1M: 3.20},
	"glm-5-code":      {InputPer1M: 1.20, OutputPer1M: 5.00},
	"glm-4.7":         {InputPer1M: 0.50, OutputPer1M: 1.50},
	"glm-4.7-flash":   {InputPer1M: 0.10, OutputPer1M: 0.40},
	"glm-4.7-flashx":  {InputPer1M: 0.10, OutputPer1M: 0.40},
}

// NewUsageTracker creates a new UsageTracker.
func NewUsageTracker(logger *slog.Logger) *UsageTracker {
	if logger == nil {
		logger = slog.Default()
	}
	return &UsageTracker{
		sessions:   make(map[string]*SessionUsage),
		global:     &SessionUsage{},
		modelCosts: make(map[string]ModelCost),
		logger:     logger.With("component", "usage_tracker"),
	}
}

func (u *UsageTracker) init() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.modelCosts == nil {
		u.modelCosts = make(map[string]ModelCost)
	}
	if u.sessions == nil {
		u.sessions = make(map[string]*SessionUsage)
	}
	if u.global == nil {
		u.global = &SessionUsage{}
	}
}

// initModelCosts copies default costs if not already set.
func (u *UsageTracker) initModelCosts() {
	for model, cost := range defaultModelCosts {
		if _, ok := u.modelCosts[model]; !ok {
			u.modelCosts[model] = cost
		}
	}
}

// Record adds usage for a session and globally.
func (u *UsageTracker) Record(sessionID, model string, usage LLMUsage) {
	u.init()
	u.mu.Lock()
	defer u.mu.Unlock()
	u.initModelCosts()

	now := time.Now()

	// Session
	su, ok := u.sessions[sessionID]
	if !ok {
		su = &SessionUsage{FirstRequestAt: now}
		u.sessions[sessionID] = su
	}
	su.PromptTokens += int64(usage.PromptTokens)
	su.CompletionTokens += int64(usage.CompletionTokens)
	su.TotalTokens += int64(usage.TotalTokens)
	su.Requests++
	su.LastRequestAt = now

	cost := u.estimateCost(model, usage.PromptTokens, usage.CompletionTokens)
	su.EstimatedCostUSD += cost

	// Global
	u.global.PromptTokens += int64(usage.PromptTokens)
	u.global.CompletionTokens += int64(usage.CompletionTokens)
	u.global.TotalTokens += int64(usage.TotalTokens)
	u.global.Requests++
	if u.global.FirstRequestAt.IsZero() {
		u.global.FirstRequestAt = now
	}
	u.global.LastRequestAt = now
	u.global.EstimatedCostUSD += cost
}

func (u *UsageTracker) estimateCost(model string, prompt, completion int) float64 {
	cost, ok := u.modelCosts[model]
	if !ok {
		// Try prefix match for model variants (e.g. gpt-4o-2024-04-09)
		for k, v := range u.modelCosts {
			if len(model) >= len(k) && model[:len(k)] == k {
				cost = v
				ok = true
				break
			}
		}
	}
	if !ok {
		return 0
	}
	return (float64(prompt)/1e6)*cost.InputPer1M + (float64(completion)/1e6)*cost.OutputPer1M
}

// GetSession returns a copy of the session's usage stats, or nil if not found.
func (u *UsageTracker) GetSession(sessionID string) *SessionUsage {
	u.mu.RLock()
	defer u.mu.RUnlock()

	su, ok := u.sessions[sessionID]
	if !ok {
		return nil
	}
	return &SessionUsage{
		PromptTokens:     su.PromptTokens,
		CompletionTokens: su.CompletionTokens,
		TotalTokens:      su.TotalTokens,
		Requests:         su.Requests,
		EstimatedCostUSD: su.EstimatedCostUSD,
		FirstRequestAt:   su.FirstRequestAt,
		LastRequestAt:    su.LastRequestAt,
	}
}

// GetGlobal returns a copy of global usage.
func (u *UsageTracker) GetGlobal() *SessionUsage {
	u.mu.RLock()
	defer u.mu.RUnlock()

	g := u.global
	return &SessionUsage{
		PromptTokens:     g.PromptTokens,
		CompletionTokens: g.CompletionTokens,
		TotalTokens:      g.TotalTokens,
		Requests:         g.Requests,
		EstimatedCostUSD: g.EstimatedCostUSD,
		FirstRequestAt:   g.FirstRequestAt,
		LastRequestAt:    g.LastRequestAt,
	}
}

// ResetSession clears usage for a session.
func (u *UsageTracker) ResetSession(sessionID string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.sessions, sessionID)
}

// FormatUsage returns a human-readable usage report for a session.
func (u *UsageTracker) FormatUsage(sessionID string) string {
	su := u.GetSession(sessionID)
	if su == nil {
		return fmt.Sprintf("No usage recorded for session %s.", sessionID)
	}
	return formatSessionUsage(sessionID, su)
}

// FormatGlobalUsage returns a human-readable global usage report.
func (u *UsageTracker) FormatGlobalUsage() string {
	g := u.GetGlobal()
	return formatSessionUsage("global", g)
}

func formatSessionUsage(label string, su *SessionUsage) string {
	var b string
	if su.Requests == 0 {
		b = fmt.Sprintf("*Usage (%s)*\n\nNo requests yet.", label)
		return b
	}
	b = fmt.Sprintf("*Usage (%s)*\n\n", label)
	b += fmt.Sprintf("Prompt tokens: %d\n", su.PromptTokens)
	b += fmt.Sprintf("Completion tokens: %d\n", su.CompletionTokens)
	b += fmt.Sprintf("Total tokens: %d\n", su.TotalTokens)
	b += fmt.Sprintf("Requests: %d\n", su.Requests)
	b += fmt.Sprintf("Est. cost: $%.4f\n", su.EstimatedCostUSD)
	if !su.FirstRequestAt.IsZero() {
		b += fmt.Sprintf("First request: %s\n", su.FirstRequestAt.Format("2006-01-02 15:04"))
	}
	if !su.LastRequestAt.IsZero() {
		b += fmt.Sprintf("Last request: %s", su.LastRequestAt.Format("2006-01-02 15:04"))
	}
	return b
}

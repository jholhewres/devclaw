// Package copilot – lcm_assembler.go builds the model's context window from
// the LCM DAG (root summaries + fresh tail messages), with budget-aware eviction.
package copilot

import (
	"fmt"
	"log/slog"
	"strings"
)

// LCMAssembler builds []chatMessage from the LCM store.
type LCMAssembler struct {
	store  *LCMStore
	cfg    LCMConfig
	logger *slog.Logger
}

// NewLCMAssembler creates a new assembler.
func NewLCMAssembler(store *LCMStore, cfg LCMConfig, logger *slog.Logger) *LCMAssembler {
	return &LCMAssembler{store: store, cfg: cfg, logger: logger}
}

// AssembleContext builds the message list for the LLM from DAG summaries + fresh tail.
func (a *LCMAssembler) AssembleContext(convID, systemPrompt, userMessage string, tokenBudget int) ([]chatMessage, error) {
	roots, err := a.store.GetRootSummaries(convID)
	if err != nil {
		return nil, fmt.Errorf("lcm assemble: get root summaries: %w", err)
	}

	tail, err := a.store.GetFreshTailMessages(convID, a.cfg.FreshTailCount)
	if err != nil {
		return nil, fmt.Errorf("lcm assemble: get fresh tail: %w", err)
	}

	// Calculate token budget: reserve 20% for model response.
	budget := int(float64(tokenBudget) * 0.8)

	// Estimate system prompt tokens.
	sysTotalTokens := EstimateTokens(systemPrompt) + EstimateTokens(a.systemPromptAddition(len(roots)))

	// Build summary items with token tracking. Rendered text is cached to
	// avoid a second renderSummary call (and its DB query) during message building.
	type summaryItem struct {
		summary  *LCMSummary
		rendered string
		tokens   int
	}
	var summaryItems []summaryItem
	for _, s := range roots {
		rendered := a.renderSummary(s)
		summaryItems = append(summaryItems, summaryItem{summary: s, rendered: rendered, tokens: EstimateTokens(rendered)})
	}

	// Calculate tail tokens.
	tailTokens := 0
	for _, m := range tail {
		tailTokens += m.TokenCount
	}

	// Calculate user message tokens.
	userMsgTokens := EstimateTokens(userMessage)

	// Total = system + summaries + tail + user message.
	totalTokens := sysTotalTokens + tailTokens + userMsgTokens
	for _, si := range summaryItems {
		totalTokens += si.tokens
	}

	// Evict oldest summaries until within budget.
	for totalTokens > budget && len(summaryItems) > 0 {
		evicted := summaryItems[0]
		summaryItems = summaryItems[1:]
		totalTokens -= evicted.tokens
		a.logger.Debug("lcm assemble: evicted oldest summary",
			"id", evicted.summary.ID, "tokens", evicted.tokens)
	}

	// Build context items for persistence.
	var contextItems []LCMContextItem
	ordinal := 0
	for _, si := range summaryItems {
		id := si.summary.ID
		contextItems = append(contextItems, LCMContextItem{
			ConversationID: convID,
			Ordinal:        ordinal,
			ItemType:        "summary",
			SummaryID:      &id,
		})
		ordinal++
	}
	for _, m := range tail {
		mid := m.ID
		contextItems = append(contextItems, LCMContextItem{
			ConversationID: convID,
			Ordinal:        ordinal,
			ItemType:        "message",
			MessageID:      &mid,
		})
		ordinal++
	}

	// Persist context items (best-effort).
	if err := a.store.ReplaceContextItems(convID, contextItems); err != nil {
		a.logger.Warn("lcm assemble: failed to persist context items", "err", err)
	}

	// Build []chatMessage.
	var msgs []chatMessage

	// System prompt + LCM guidance.
	if systemPrompt != "" {
		sysContent := systemPrompt
		if guidance := a.systemPromptAddition(len(summaryItems)); guidance != "" {
			sysContent += "\n\n" + guidance
		}
		msgs = append(msgs, chatMessage{Role: "system", Content: sysContent})
	}

	// Summaries in chronological order (use cached rendered text).
	// Injected as "user" role — this is the most portable approach across
	// LLM providers (some don't support mid-conversation system messages).
	// The <summary> XML tags clearly mark this as system-injected context.
	for _, si := range summaryItems {
		msgs = append(msgs, chatMessage{
			Role:    "user",
			Content: "[context: compacted conversation history]\n" + si.rendered,
		})
	}

	// Fresh tail messages.
	for _, m := range tail {
		msgs = append(msgs, chatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// Current user message.
	if userMessage != "" {
		msgs = append(msgs, chatMessage{Role: "user", Content: userMessage})
	}

	return msgs, nil
}

// renderSummary formats a summary as an XML-tagged block.
func (a *LCMAssembler) renderSummary(s *LCMSummary) string {
	// Count source messages for leaf summaries.
	msgCount := 0
	if s.Kind == "leaf" {
		msgs, err := a.store.GetSummaryMessages(s.ID)
		if err == nil {
			msgCount = len(msgs)
		}
	} else {
		msgCount = s.DescendantCount
	}

	return fmt.Sprintf(
		`<summary id="%s" kind="%s" depth="%d" tokens="%d" messages="%d" earliest="%s" latest="%s">
%s
</summary>`,
		s.ID, s.Kind, s.Depth, s.TokenCount, msgCount,
		lcmFormatTime(s.EarliestAt),
		lcmFormatTime(s.LatestAt),
		s.Content,
	)
}

// systemPromptAddition returns the LCM tool guidance to append to the system prompt.
// Returns "" when there are no summaries (no need for guidance).
func (a *LCMAssembler) systemPromptAddition(summaryCount int) string {
	if summaryCount == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## LCM — Lossless Context Memory\n\n")
	b.WriteString("Summaries above (in <summary> tags) are compressed earlier conversation.\n")
	b.WriteString("They preserve key info but may lack specific details.\n\n")
	b.WriteString("To recover details, use the `lcm` tool:\n")
	b.WriteString("- `lcm grep query=\"search term\"` — search across all messages and summaries\n")
	b.WriteString("- `lcm describe summary_id=\"sum_xxx\"` — inspect a summary's structure and metadata\n")
	b.WriteString("- `lcm describe summary_id=\"tree\"` — see the full DAG overview\n")
	b.WriteString("- `lcm expand summary_id=\"sum_xxx\"` — recover original messages behind a summary\n\n")
	b.WriteString("**When to use:** Before asserting specific facts (SHAs, paths, timestamps, config values) from a summary, expand it first to verify.")
	return b.String()
}

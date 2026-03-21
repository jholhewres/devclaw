// Package copilot – lcm_compaction.go implements the DAG-based compaction
// engine for the Lossless Compaction Module. Two passes: leaf (messages → summaries)
// and condensed (summaries → higher-level summaries), cascading until stable.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// LCMSummarizeFn is the function signature for LLM-based summarization.
// aggressive=true requests a shorter, more compressed output.
type LCMSummarizeFn func(ctx context.Context, text string, aggressive bool) (string, error)

// LCMCompactor runs leaf and condensed compaction passes.
type LCMCompactor struct {
	store  *LCMStore
	cfg    LCMConfig
	logger *slog.Logger
}

// NewLCMCompactor creates a new compactor.
func NewLCMCompactor(store *LCMStore, cfg LCMConfig, logger *slog.Logger) *LCMCompactor {
	return &LCMCompactor{store: store, cfg: cfg, logger: logger}
}

// ShouldCompact checks whether compaction is needed based on unsummarized token count.
// Returns (shouldCompact, triggerReason). Skips compaction for sessions with no real
// conversation (heartbeat-only or system-only sessions).
func (c *LCMCompactor) ShouldCompact(convID string, contextWindowTokens int) (bool, string) {
	unsummarized, err := c.store.CountUnsummarizedTokens(convID, c.cfg.FreshTailCount)
	if err != nil {
		c.logger.Warn("lcm: failed to count unsummarized tokens", "err", err)
		return false, ""
	}

	hardThreshold := int(float64(contextWindowTokens) * c.cfg.HardTriggerRatio)
	softThreshold := int(float64(contextWindowTokens) * c.cfg.SoftTriggerRatio)

	if unsummarized < softThreshold {
		return false, ""
	}

	// Content-aware guard: verify the session has real conversation content
	// before spending LLM tokens on compaction. Require at least 2 meaningful
	// user+assistant exchanges (excluding heartbeats and empty messages).
	if !c.hasRealConversation(convID) {
		c.logger.Debug("lcm: skipping compaction — no real conversation detected", "conv", convID)
		return false, ""
	}

	if unsummarized >= hardThreshold {
		return true, "hard_trigger"
	}
	return true, "soft_trigger"
}

// hasRealConversation checks whether a conversation has at least 2 meaningful
// user+assistant exchanges (not heartbeats or system messages).
func (c *LCMCompactor) hasRealConversation(convID string) bool {
	msgs, err := c.store.GetRecentMessages(convID, 200)
	if err != nil {
		return true // On error, assume real conversation (fail open).
	}

	userCount := 0
	assistantCount := 0
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(m.Content))
		// Skip heartbeat messages.
		if strings.HasPrefix(lower, "[heartbeat") || lower == "heartbeat_ok" {
			continue
		}
		// Skip empty or near-empty messages.
		if len(lower) < 5 {
			continue
		}
		switch m.Role {
		case "user":
			userCount++
		case "assistant":
			assistantCount++
		}
	}

	return userCount >= 2 && assistantCount >= 2
}

// LeafPass groups unsummarized messages into chunks and summarizes each into
// depth-0 leaf summary nodes.
func (c *LCMCompactor) LeafPass(ctx context.Context, convID string, summarizeFn LCMSummarizeFn) ([]*LCMSummary, error) {
	msgs, err := c.store.GetUnsummarizedMessages(convID, c.cfg.FreshTailCount)
	if err != nil {
		return nil, fmt.Errorf("lcm leaf pass: get unsummarized: %w", err)
	}
	if len(msgs) < 4 {
		return nil, nil // Too few to summarize.
	}

	chunks := c.chunkMessages(msgs)
	var summaries []*LCMSummary

	for i, chunk := range chunks {
		if ctx.Err() != nil {
			return summaries, ctx.Err()
		}

		text := formatMessagesForSummary(chunk)
		summary, err := c.trySummarize(ctx, summarizeFn, text)
		if err != nil {
			// trySummarize already includes L3 deterministic fallback,
			// so this path is only reached on context cancellation.
			c.logger.Warn("lcm leaf: summarization failed completely",
				"chunk", i, "msgs", len(chunk), "err", err)
			summary = deterministicFallback(chunk)
		}

		now := time.Now().UTC()
		var totalMsgTokens int
		var msgIDs []int64
		for _, m := range chunk {
			totalMsgTokens += m.TokenCount
			msgIDs = append(msgIDs, m.ID)
		}

		sum := &LCMSummary{
			ID:                      GenerateSummaryID(summary, now),
			ConversationID:          convID,
			Kind:                    "leaf",
			Depth:                   0,
			Content:                 summary,
			TokenCount:              EstimateTokens(summary),
			SourceMessageTokenCount: totalMsgTokens,
			EarliestAt:              chunk[0].CreatedAt,
			LatestAt:                chunk[len(chunk)-1].CreatedAt,
			CreatedAt:               now,
		}

		if err := c.store.InsertSummary(sum); err != nil {
			return summaries, fmt.Errorf("lcm leaf: insert summary: %w", err)
		}
		if err := c.store.LinkSummaryMessages(sum.ID, msgIDs); err != nil {
			return summaries, fmt.Errorf("lcm leaf: link messages: %w", err)
		}
		summaries = append(summaries, sum)
	}

	return summaries, nil
}

// CondensedPass groups orphan summaries at each depth into higher-level condensed nodes.
func (c *LCMCompactor) CondensedPass(ctx context.Context, convID string, summarizeFn LCMSummarizeFn) ([]*LCMSummary, error) {
	maxDepth, err := c.store.GetMaxDepth(convID)
	if err != nil {
		return nil, fmt.Errorf("lcm condensed pass: get max depth: %w", err)
	}

	var created []*LCMSummary

	for depth := 0; depth <= maxDepth; depth++ {
		if ctx.Err() != nil {
			return created, ctx.Err()
		}

		orphans, err := c.store.GetOrphanSummaries(convID, depth)
		if err != nil {
			return created, fmt.Errorf("lcm condensed: get orphans at depth %d: %w", depth, err)
		}
		if len(orphans) < c.cfg.CondensedMinChildren {
			continue
		}

		// Batch orphans into groups of CondensedMaxChildren.
		for batchStart := 0; batchStart < len(orphans); batchStart += c.cfg.CondensedMaxChildren {
			batchEnd := batchStart + c.cfg.CondensedMaxChildren
			if batchEnd > len(orphans) {
				batchEnd = len(orphans)
			}
			batch := orphans[batchStart:batchEnd]
			if len(batch) < c.cfg.CondensedMinChildren {
				break // Remaining batch too small.
			}

			text := formatSummariesForCondensation(batch)
			// Prepend depth-aware prompt for the target condensed depth.
			targetDepth := depth + 1
			prefixedText := lcmPromptForDepth(targetDepth) + "\n\n" + text
			summary, err := c.trySummarize(ctx, summarizeFn, prefixedText)
			if err != nil {
				c.logger.Warn("lcm condensed: summarization failed, skipping batch",
					"depth", depth, "batch_size", len(batch), "err", err)
				continue
			}

			now := time.Now().UTC()
			var descCount, descTokens, srcMsgTokens int
			var childIDs []string
			earliest := batch[0].EarliestAt
			latest := batch[0].LatestAt
			for _, child := range batch {
				childIDs = append(childIDs, child.ID)
				descCount += child.DescendantCount + 1
				descTokens += child.TokenCount + child.DescendantTokenCount
				srcMsgTokens += child.SourceMessageTokenCount
				if child.EarliestAt.Before(earliest) {
					earliest = child.EarliestAt
				}
				if child.LatestAt.After(latest) {
					latest = child.LatestAt
				}
			}

			sum := &LCMSummary{
				ID:                      GenerateSummaryID(summary, now),
				ConversationID:          convID,
				Kind:                    "condensed",
				Depth:                   depth + 1,
				Content:                 summary,
				TokenCount:              EstimateTokens(summary),
				SourceMessageTokenCount: srcMsgTokens,
				DescendantCount:         descCount,
				DescendantTokenCount:    descTokens,
				EarliestAt:              earliest,
				LatestAt:                latest,
				CreatedAt:               now,
			}

			if err := c.store.InsertSummary(sum); err != nil {
				return created, fmt.Errorf("lcm condensed: insert summary: %w", err)
			}
			if err := c.store.LinkSummaryChildren(sum.ID, childIDs); err != nil {
				return created, fmt.Errorf("lcm condensed: link children: %w", err)
			}
			created = append(created, sum)
		}
	}

	return created, nil
}

// maxCondensedIterations caps the cascading condensed passes to prevent
// infinite loops in pathological cases.
const maxCondensedIterations = 20

// FullSweep runs leaf pass then cascading condensed passes until stable.
func (c *LCMCompactor) FullSweep(ctx context.Context, convID string, summarizeFn LCMSummarizeFn) ([]*LCMSummary, error) {
	var allNew []*LCMSummary

	leaves, err := c.LeafPass(ctx, convID, summarizeFn)
	if err != nil {
		return allNew, fmt.Errorf("lcm full sweep: leaf pass: %w", err)
	}
	allNew = append(allNew, leaves...)

	// Cascade condensed passes until no more grouping is possible.
	for i := 0; i < maxCondensedIterations; i++ {
		if ctx.Err() != nil {
			return allNew, ctx.Err()
		}
		condensed, err := c.CondensedPass(ctx, convID, summarizeFn)
		if err != nil {
			return allNew, fmt.Errorf("lcm full sweep: condensed pass: %w", err)
		}
		if len(condensed) == 0 {
			break
		}
		allNew = append(allNew, condensed...)
		if i == maxCondensedIterations-1 {
			c.logger.Warn("lcm full sweep: hit condensed iteration limit", "limit", maxCondensedIterations)
		}
	}

	return allNew, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// trySummarize uses a three-level escalation strategy aligned with lossless-claw:
// L1: Normal (standard prompt), L2: Aggressive (tighter prompt, lower targets),
// L3: Deterministic fallback (~512 tokens, metadata-only).
func (c *LCMCompactor) trySummarize(ctx context.Context, fn LCMSummarizeFn, text string) (string, error) {
	// L1: Normal.
	result, err := fn(ctx, text, false)
	if err == nil && result != "" {
		return result, nil
	}
	c.logger.Debug("lcm: L1 normal failed, trying L2 aggressive", "err", err)

	// L2: Aggressive.
	result, err = fn(ctx, text, true)
	if err == nil && result != "" {
		return result, nil
	}
	c.logger.Debug("lcm: L2 aggressive failed, using L3 deterministic fallback", "err", err)

	// L3: Deterministic fallback — truncate to ~512 tokens with metadata.
	return deterministicFallbackCapped(text, 512), nil
}

// deterministicFallbackCapped creates a token-capped fallback summary from raw text
// when LLM summarization fails entirely. Used as L3 in the escalation strategy.
func deterministicFallbackCapped(text string, maxTokens int) string {
	maxChars := maxTokens * 4

	var b strings.Builder
	b.WriteString("## Decisions\n(LLM summarization failed — deterministic fallback)\n\n")
	b.WriteString("## Open TODOs\n(unknown)\n\n")
	b.WriteString("## Constraints/Rules\n(unknown)\n\n")
	b.WriteString("## Pending user asks\n")

	// Extract last user-like line from the text.
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "user]") || strings.Contains(lines[i], "user ") {
			preview := lines[i]
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			b.WriteString(preview)
			break
		}
	}
	b.WriteString("\n\n## Exact identifiers\n")
	b.WriteString("(deterministic fallback, first ~512 tokens preserved)\n\n")

	// Append raw truncated content for signal preservation.
	remaining := maxChars - b.Len()
	if remaining > 0 && len(text) > 0 {
		snippet := text
		if len(snippet) > remaining {
			snippet = snippet[:remaining]
		}
		b.WriteString(snippet)
	}

	return b.String()
}

// chunkMessages groups messages into chunks respecting the max token limit.
// Avoids splitting tool_call/tool_result pairs.
func (c *LCMCompactor) chunkMessages(msgs []*LCMMessage) [][]*LCMMessage {
	maxTokens := c.cfg.LeafChunkMaxTokens
	if maxTokens <= 0 {
		maxTokens = 20000
	}

	var chunks [][]*LCMMessage
	var current []*LCMMessage
	currentTokens := 0

	for _, m := range msgs {
		if currentTokens+m.TokenCount > maxTokens && len(current) > 0 {
			// Find safe cut point: don't split tool results from their calls.
			cutIdx := findSafeChunkCut(current)
			chunks = append(chunks, current[:cutIdx])
			remaining := current[cutIdx:]
			current = make([]*LCMMessage, len(remaining))
			copy(current, remaining)
			currentTokens = 0
			for _, r := range current {
				currentTokens += r.TokenCount
			}
		}
		current = append(current, m)
		currentTokens += m.TokenCount
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

// findSafeChunkCut finds a cut point that doesn't orphan tool results.
// It walks backward from the end looking for a point where the next message
// is not a "tool" role (which would be orphaned from its assistant call).
func findSafeChunkCut(msgs []*LCMMessage) int {
	n := len(msgs)
	if n <= 1 {
		return n
	}
	// Start from the end and walk back until we find a safe cut.
	for i := n; i > 1; i-- {
		if msgs[i-1].Role != "tool" {
			return i
		}
	}
	return n // Can't find safe cut; include all.
}

// formatMessagesForSummary concatenates messages with timestamps for LLM input.
func formatMessagesForSummary(msgs []*LCMMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "[%s %s] %s\n\n", lcmFormatTime(m.CreatedAt), m.Role, m.Content)
	}
	return b.String()
}

// formatSummariesForCondensation concatenates summaries for the condensed pass.
func formatSummariesForCondensation(sums []*LCMSummary) string {
	var b strings.Builder
	for _, s := range sums {
		fmt.Fprintf(&b, "[%s depth=%d %s→%s]\n%s\n\n",
			s.ID, s.Depth,
			lcmFormatTime(s.EarliestAt),
			lcmFormatTime(s.LatestAt),
			s.Content)
	}
	return b.String()
}

// deterministicFallback creates a minimal summary when LLM summarization fails.
func deterministicFallback(msgs []*LCMMessage) string {
	if len(msgs) == 0 {
		return "## Decisions\n(no messages)\n## Open TODOs\n(none)\n## Constraints/Rules\n(none)\n## Pending user asks\n(none)\n## Exact identifiers\n(none)\n"
	}

	var b strings.Builder
	b.WriteString("## Decisions\n(summarization failed — metadata only)\n\n")
	b.WriteString("## Open TODOs\n(unknown)\n\n")
	b.WriteString("## Constraints/Rules\n(unknown)\n\n")
	b.WriteString("## Pending user asks\n")
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			preview := msgs[i].Content
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			b.WriteString(preview)
			break
		}
	}
	b.WriteString("\n\n## Exact identifiers\n")
	fmt.Fprintf(&b, "(%d messages, seq %d→%d)\n", len(msgs), msgs[0].Seq, msgs[len(msgs)-1].Seq)
	return b.String()
}

// ── Depth-Aware Summarization Prompts ────────────────────────────────────────

// lcmPromptForDepth returns a specialized summarization prompt for the given
// DAG depth level. Aligns with lossless-claw's four-tier prompt hierarchy.
func lcmPromptForDepth(depth int) string {
	switch depth {
	case 0:
		return lcmLeafPrompt
	case 1:
		return lcmD1Prompt
	case 2:
		return lcmD2Prompt
	default:
		return lcmD3PlusPrompt
	}
}

// lcmLeafPrompt is for depth-0 leaf summaries: event-level detail.
const lcmLeafPrompt = `You are summarizing a segment of raw conversation messages (event-level detail).
Preserve:
- Exact tool invocations and their results (success/failure)
- File paths, URLs, SHAs, identifiers verbatim
- Error messages and resolution steps
- User requests and assistant responses
Use these section headings:
## Decisions
## Open TODOs
## Constraints/Rules
## Pending user asks
## Exact identifiers`

// lcmD1Prompt is for depth-1 condensed summaries: session-level focus.
const lcmD1Prompt = `You are condensing multiple conversation summaries into a session-level summary.
Focus on:
- Goals pursued in this session and their completion status
- Key decisions made and their rationale
- Blockers encountered and resolutions
- Active tasks carried forward
Omit per-tool-call detail; keep identifiers that are still relevant.
Use these section headings:
## Goals & Status
## Decisions
## Blockers & Resolutions
## Carried Forward
## Key Identifiers`

// lcmD2Prompt is for depth-2 condensed summaries: arc-level scope.
const lcmD2Prompt = `You are creating an arc-level summary spanning multiple sessions.
Focus on:
- Project arcs and milestones reached
- Architectural decisions and design patterns chosen
- Recurring themes or constraints
- Outstanding commitments
Use these section headings:
## Arcs & Milestones
## Architecture Decisions
## Persistent Constraints
## Outstanding Commitments
## Key Identifiers`

// lcmD3PlusPrompt is for depth-3+ condensed summaries: durable memory.
const lcmD3PlusPrompt = `You are distilling durable memory from multiple project arcs.
Focus on:
- Core knowledge: technologies, patterns, conventions in use
- Persistent facts: user preferences, environment details, access patterns
- Long-lived constraints and non-negotiables
- Definitions that should never be forgotten
Be maximally dense. Every sentence must carry lasting value.
Use these section headings:
## Core Knowledge
## Persistent Facts
## Constraints
## Definitions`

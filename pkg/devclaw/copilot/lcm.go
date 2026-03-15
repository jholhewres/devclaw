// Package copilot – lcm.go is the coordinator for the Lossless Compaction Module.
// It ties together store, compactor, assembler, and retrieval into a single engine.
package copilot

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// LCMEngine is the top-level coordinator for lossless compaction.
type LCMEngine struct {
	store     *LCMStore
	compactor *LCMCompactor
	assembler *LCMAssembler
	retrieval *LCMRetrieval
	cfg       LCMConfig
	ccfg      CompactionConfig
	logger    *slog.Logger
}

// NewLCMEngine creates a new LCM engine wired to the given database.
func NewLCMEngine(db *sql.DB, cfg LCMConfig, ccfg CompactionConfig, logger *slog.Logger) *LCMEngine {
	store := NewLCMStore(db, logger.With("component", "lcm-store"))
	return &LCMEngine{
		store:     store,
		compactor: NewLCMCompactor(store, cfg, logger.With("component", "lcm-compact")),
		assembler: NewLCMAssembler(store, cfg, logger.With("component", "lcm-assemble")),
		retrieval: NewLCMRetrieval(store, logger.With("component", "lcm-retrieval")),
		cfg:       cfg,
		ccfg:      ccfg,
		logger:    logger,
	}
}

// Bootstrap initializes an LCM conversation for the given session ID.
// Returns the conversation ID.
func (e *LCMEngine) Bootstrap(sessionID string) (string, error) {
	conv, err := e.store.GetOrCreateConversation(sessionID)
	if err != nil {
		return "", fmt.Errorf("lcm bootstrap: %w", err)
	}
	return conv.ID, nil
}

// Ingest persists new messages from the agent loop into the LCM store.
// Messages are deduplicated by tracking the last ingested index externally.
func (e *LCMEngine) Ingest(ctx context.Context, convID string, messages []chatMessage) error {
	for _, m := range messages {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Skip system messages — they're reconstructed by the assembler.
		if m.Role == "system" {
			continue
		}

		content, ok := m.Content.(string)
		if !ok {
			// Non-string content (multimodal): serialize as placeholder.
			content = fmt.Sprintf("[non-text content: %T]", m.Content)
		}
		if content == "" {
			continue
		}

		tokenCount := EstimateTokens(content)
		if _, err := e.store.IngestMessage(convID, m.Role, content, tokenCount); err != nil {
			return fmt.Errorf("lcm ingest: %w", err)
		}
	}
	return nil
}

// Compact evaluates whether compaction is needed and runs a full sweep if so.
// Returns true if compaction was performed.
func (e *LCMEngine) Compact(ctx context.Context, convID string, contextWindowTokens int, summarizeFn LCMSummarizeFn) (bool, error) {
	shouldCompact, reason := e.compactor.ShouldCompact(convID, contextWindowTokens)
	if !shouldCompact {
		return false, nil
	}

	e.logger.Info("lcm compaction triggered",
		"reason", reason,
		"context_window", contextWindowTokens,
	)

	summaries, err := e.compactor.FullSweep(ctx, convID, summarizeFn)
	if err != nil {
		return false, fmt.Errorf("lcm compact: %w", err)
	}

	if err := e.store.UpdateLastCompactAt(convID); err != nil {
		e.logger.Warn("lcm compact: failed to update last_compact_at", "err", err)
	}

	e.logger.Info("lcm compaction complete",
		"new_summaries", len(summaries),
	)

	return true, nil
}

// Assemble builds the model's context window from DAG summaries + fresh tail.
func (e *LCMEngine) Assemble(ctx context.Context, convID, systemPrompt, userMessage string, tokenBudget int) ([]chatMessage, error) {
	return e.assembler.AssembleContext(convID, systemPrompt, userMessage, tokenBudget)
}

// Retrieval returns the retrieval engine for tool use.
func (e *LCMEngine) Retrieval() *LCMRetrieval {
	return e.retrieval
}

// Store returns the underlying LCM store.
func (e *LCMEngine) Store() *LCMStore {
	return e.store
}

// buildLCMSummarizeFn constructs the summarization function that delegates
// to the existing LLM client, reusing the structured compaction prompt.
func buildLCMSummarizeFn(llm *LLMClient, ccfg CompactionConfig, modelOverride string, logger *slog.Logger) LCMSummarizeFn {
	return func(ctx context.Context, text string, aggressive bool) (string, error) {
		model := ccfg.CompactionModel
		if model == "" {
			model = modelOverride
		}

		prompt := buildStructuredCompactionPrompt(ccfg, nil, nil, nil)
		if aggressive {
			prompt += "\n\nBe extremely concise. Target 50% of the input size."
		}

		// Truncate input if absurdly large to prevent API errors.
		const maxInputChars = 200000
		if len(text) > maxInputChars {
			text = text[:maxInputChars] + "\n...(truncated)"
		}

		// Use the LLM Complete API with the structured prompt as system and text as user.
		messages := []chatMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: text},
		}

		var resp *LLMResponse
		var err error
		if model != "" {
			resp, err = llm.CompleteWithToolsUsingModel(ctx, model, messages, nil)
		} else {
			resp, err = llm.CompleteWithTools(ctx, messages, nil)
		}
		if err != nil {
			return "", fmt.Errorf("lcm summarize: %w", err)
		}
		if resp.Content == "" {
			return "", fmt.Errorf("lcm summarize: empty response")
		}

		logger.Debug("lcm summarization complete",
			"input_chars", len(text),
			"output_chars", len(resp.Content),
			"model", model,
			"aggressive", aggressive,
		)
		return resp.Content, nil
	}
}

// lcmConversationID returns the conversation ID for the given agent, or "".
func lcmConversationID(a *AgentRun) string {
	if a == nil {
		return ""
	}
	return a.lcmConversationID
}

// SetLCMEngine wires the LCM engine into the agent run.
func (a *AgentRun) SetLCMEngine(engine *LCMEngine, conversationID string) {
	a.lcmEngine = engine
	a.lcmConversationID = conversationID
}

// lcmIngestNew ingests messages not yet tracked by the LCM store.
func (a *AgentRun) lcmIngestNew(ctx context.Context, messages []chatMessage) error {
	if a.lcmEngine == nil || a.lcmConversationID == "" {
		return nil
	}
	// Guard against slice out-of-bounds: if the messages slice was rebuilt
	// (e.g. after compaction reassembly), the old seq no longer applies.
	// Skip all current messages rather than resetting to 0 — re-ingesting
	// the assembled context would create duplicates (summaries as messages).
	if a.lcmIngestedSeq > len(messages) {
		a.logger.Warn("lcm: lcmIngestedSeq exceeds message count, skipping ingest",
			"seq", a.lcmIngestedSeq, "len", len(messages))
		a.lcmIngestedSeq = len(messages)
		return nil
	}
	newMsgs := messages[a.lcmIngestedSeq:]
	if len(newMsgs) == 0 {
		return nil
	}
	if err := a.lcmEngine.Ingest(ctx, a.lcmConversationID, newMsgs); err != nil {
		return err
	}
	a.lcmIngestedSeq = len(messages)
	return nil
}

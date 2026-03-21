// Package copilot – lcm.go is the coordinator for the Lossless Compaction Module.
// It ties together store, compactor, assembler, and retrieval into a single engine.
package copilot

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
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
// If sessionHistory is provided, reconciles with the LCM store to import
// any messages that were missed (e.g. from a previous session that wasn't
// fully ingested). Returns the conversation ID.
func (e *LCMEngine) Bootstrap(sessionID string, sessionHistory []ConversationEntry) (string, error) {
	conv, err := e.store.GetOrCreateConversation(sessionID)
	if err != nil {
		return "", fmt.Errorf("lcm bootstrap: %w", err)
	}

	// Reconcile session history with LCM store (best-effort).
	if len(sessionHistory) > 0 {
		if err := e.reconcileHistory(conv.ID, sessionHistory); err != nil {
			e.logger.Warn("lcm bootstrap: reconciliation failed (non-fatal)", "err", err)
		}
	}

	return conv.ID, nil
}

// reconcileHistory imports messages from session history that are missing
// from the LCM store. Uses anchor-based matching: walks backward through
// the last 10 LCM messages to find a match in session history, then imports
// everything after the anchor point.
func (e *LCMEngine) reconcileHistory(convID string, history []ConversationEntry) error {
	// Get recent messages from LCM store for anchor matching.
	lcmMsgs, err := e.store.GetRecentMessages(convID, 10)
	if err != nil {
		return fmt.Errorf("get recent messages: %w", err)
	}

	// If the LCM store has no messages, import all session history.
	if len(lcmMsgs) == 0 {
		return e.importHistoryEntries(convID, history)
	}

	// Find the last LCM message content to anchor against session history.
	lastLCM := lcmMsgs[len(lcmMsgs)-1]

	// Walk backward through session history to find the anchor.
	anchorIdx := -1
	for i := len(history) - 1; i >= 0; i-- {
		entry := history[i]
		// Match against either user or assistant content.
		if lastLCM.Role == "user" && lastLCM.Content == entry.UserMessage {
			anchorIdx = i
			break
		}
		if lastLCM.Role == "assistant" && lastLCM.Content == entry.AssistantResponse {
			anchorIdx = i
			break
		}
	}

	if anchorIdx < 0 {
		e.logger.Debug("lcm reconcile: no anchor found, skipping import")
		return nil // Can't anchor — don't import to avoid duplicates.
	}

	// Import entries after the anchor.
	toImport := history[anchorIdx+1:]
	if len(toImport) == 0 {
		return nil // Already up to date.
	}

	e.logger.Info("lcm reconcile: importing missing entries",
		"anchor_idx", anchorIdx, "importing", len(toImport))
	return e.importHistoryEntries(convID, toImport)
}

// importHistoryEntries imports ConversationEntry items into the LCM store.
func (e *LCMEngine) importHistoryEntries(convID string, entries []ConversationEntry) error {
	for _, entry := range entries {
		if entry.UserMessage != "" {
			tokens := EstimateTokens(entry.UserMessage)
			if _, err := e.store.IngestMessage(convID, "user", entry.UserMessage, tokens); err != nil {
				return fmt.Errorf("import user msg: %w", err)
			}
		}
		if entry.AssistantResponse != "" {
			tokens := EstimateTokens(entry.AssistantResponse)
			if _, err := e.store.IngestMessage(convID, "assistant", entry.AssistantResponse, tokens); err != nil {
				return fmt.Errorf("import assistant msg: %w", err)
			}
		}
	}
	return nil
}

// Ingest persists new messages from the agent loop into the LCM store.
// Messages are deduplicated by tracking the last ingested index externally.
func (e *LCMEngine) Ingest(ctx context.Context, convID string, messages []chatMessage) error {
	pruneHeartbeats := e.cfg.PruneHeartbeatOK == nil || *e.cfg.PruneHeartbeatOK
	var lastUserMsgID int64
	var lastUserIsHeartbeat bool

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

		// Large file interception: if content exceeds the token threshold,
		// store original on disk and replace with a compact reference.
		if e.cfg.LargeFileTokenThreshold > 0 && tokenCount > e.cfg.LargeFileTokenThreshold {
			ref, err := e.interceptLargeContent(convID, content, tokenCount)
			if err != nil {
				e.logger.Warn("lcm: large file interception failed, ingesting as-is", "err", err)
			} else {
				content = ref
				tokenCount = EstimateTokens(content)
			}
		}

		msg, err := e.store.IngestMessage(convID, m.Role, content, tokenCount)
		if err != nil {
			return fmt.Errorf("lcm ingest: %w", err)
		}

		// Heartbeat pruning: detect user heartbeat + assistant HEARTBEAT_OK
		// pairs and delete both from the store retroactively.
		if pruneHeartbeats {
			lower := strings.ToLower(strings.TrimSpace(content))
			if m.Role == "user" {
				lastUserIsHeartbeat = strings.HasPrefix(lower, "[heartbeat")
				if lastUserIsHeartbeat {
					lastUserMsgID = msg.ID
				}
			} else if m.Role == "assistant" && lastUserIsHeartbeat {
				if lower == "heartbeat_ok" || strings.HasPrefix(lower, "heartbeat_ok") {
					// Delete both the heartbeat user message and the OK response.
					if err := e.store.DeleteMessage(lastUserMsgID); err != nil {
						e.logger.Debug("lcm: failed to prune heartbeat user msg", "id", lastUserMsgID, "err", err)
					}
					if err := e.store.DeleteMessage(msg.ID); err != nil {
						e.logger.Debug("lcm: failed to prune heartbeat assistant msg", "id", msg.ID, "err", err)
					}
				}
				lastUserIsHeartbeat = false
			}
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
// to the existing LLM client. Model priority: LCM SummaryModel > CompactionModel > agent model.
func buildLCMSummarizeFn(llm *LLMClient, ccfg CompactionConfig, lcmCfg LCMConfig, modelOverride string, logger *slog.Logger) LCMSummarizeFn {
	return func(ctx context.Context, text string, aggressive bool) (string, error) {
		// Model priority: LCM SummaryModel > CompactionModel > agent's current model.
		model := lcmCfg.SummaryModel
		if model == "" {
			model = ccfg.CompactionModel
		}
		if model == "" {
			model = modelOverride
		}

		// Use the leaf-level depth-aware prompt for LCM summarization.
		// Condensed passes prepend their own depth-specific prompt to the input text.
		prompt := lcmPromptForDepth(0)
		if aggressive {
			prompt += "\n\nBe extremely concise. Target 60% of the input size. Temperature should be very low."
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

// interceptLargeContent stores oversized content on disk and returns a compact
// reference string to replace it in the LCM message store. The reference includes
// a deterministic preview (first ~800 chars) plus metadata.
func (e *LCMEngine) interceptLargeContent(convID, content string, tokenCount int) (string, error) {
	fileID := "file_" + uuid.New().String()[:8]

	// Determine storage directory.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(homeDir, ".devclaw", "lcm-files", convID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create lcm-files dir: %w", err)
	}

	// Write content to disk.
	filePath := filepath.Join(dir, fileID+".txt")
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	// Generate deterministic preview.
	preview := content
	if len(preview) > 800 {
		preview = preview[:800] + "..."
	}

	// Store metadata in DB.
	lcmFile := &LCMFile{
		ID:             fileID,
		ConversationID: convID,
		OriginalTokens: tokenCount,
		OriginalChars:  len(content),
		Summary:        preview,
		FilePath:       filePath,
		CreatedAt:      time.Now(),
	}
	if err := e.store.InsertFile(lcmFile); err != nil {
		// Clean up disk file on DB failure.
		os.Remove(filePath)
		return "", fmt.Errorf("insert file metadata: %w", err)
	}

	// Build compact reference.
	ref := fmt.Sprintf("[Large content intercepted: %s (%d tokens, %d chars). "+
		"Use `lcm describe_file file_id=\"%s\"` to retrieve full content.]\n\n"+
		"Preview:\n%s", fileID, tokenCount, len(content), fileID, preview)

	e.logger.Info("lcm: intercepted large content",
		"file_id", fileID, "tokens", tokenCount, "chars", len(content))

	return ref, nil
}

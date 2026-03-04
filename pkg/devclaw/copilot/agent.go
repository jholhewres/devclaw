// Package copilot – agent.go implements the agentic loop that orchestrates
// LLM calls with tool execution. The agent iterates: call LLM → if tool_calls
// → execute tools → append results → call LLM again, until the LLM produces
// a final text response with no tool calls.
//
// Architecture:
//   - No fixed max turns — the loop runs until the LLM stops calling tools.
//   - Single run timeout (default: 600s = 10min) controls the whole run.
//   - Per-LLM-call safety timeout (5min) prevents individual hung requests.
//   - Reflection nudge every 15 turns for budget awareness.
//   - Auto-compaction on context overflow (up to 3 attempts).
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	// DefaultRunTimeout is the maximum duration for an entire agent run.
	// Set to 20 minutes to accommodate coding tasks that invoke Claude Code CLI
	// (which itself can take 5-15 minutes for complex projects).
	// This is the PRIMARY timeout — no per-turn limit.
	DefaultRunTimeout = 1200 * time.Second

	// DefaultLLMCallTimeout is the safety-net timeout for a single LLM API call.
	// This only prevents hung HTTP connections — it should be generous enough
	// that even large contexts complete. 5 minutes covers worst-case scenarios.
	DefaultLLMCallTimeout = 5 * time.Minute

	// reflectionInterval is how often (in turns) the agent receives a budget nudge.
	// Reduced from 15 to 5 to catch stuck patterns earlier.
	reflectionInterval = 5

	// DefaultMaxCompactionAttempts is how many times to retry after context overflow compaction.
	DefaultMaxCompactionAttempts = 3

	// compactionSafetyTimeout is the maximum time allowed for the summarization
	// step during compaction. If the LLM hangs, we fall back to trim-by-count.
	compactionSafetyTimeout = 5 * time.Minute
)

// ScaledMaxRetries computes a retry budget scaled by the number of available
// auth profiles. More profiles = more credential rotation capacity, so the
// system can afford more retries before giving up on a transient failure.
// Formula: min(160, max(32, 24 + profileCount * 8))
func ScaledMaxRetries(profileCount int) int {
	n := 24 + profileCount*8
	if n < 32 {
		n = 32
	}
	if n > 160 {
		n = 160
	}
	return n
}

// AgentConfig holds configurable agent loop parameters.
type AgentConfig struct {
	// RunTimeoutSeconds is the max seconds for the entire agent run (default: 600).
	// One timer for the whole run, not per-turn.
	RunTimeoutSeconds int `yaml:"run_timeout_seconds"`

	// LLMCallTimeoutSeconds is the safety-net timeout per individual LLM call
	// (default: 300). Only catches hung connections — not the primary timeout.
	LLMCallTimeoutSeconds int `yaml:"llm_call_timeout_seconds"`

	// MaxTurns is a soft safety limit on LLM round-trips (default: 0 = unlimited).
	// When > 0, the agent will request a summary after this many turns.
	MaxTurns int `yaml:"max_turns"`

	// MaxContinuations is how many auto-continue rounds are allowed when
	// MaxTurns is hit and the agent is still using tools.
	// Only relevant when MaxTurns > 0. Default: 2.
	MaxContinuations int `yaml:"max_continuations"`

	// ReflectionEnabled enables periodic budget awareness nudges (default: true).
	ReflectionEnabled bool `yaml:"reflection_enabled"`

	// MaxCompactionAttempts is how many times to retry after context overflow (default: 3).
	MaxCompactionAttempts int `yaml:"max_compaction_attempts"`

	// ContextTokens overrides the auto-detected context window size for the model.
	// When > 0, this value is used instead of the built-in model-specific lookup.
	// Set this for custom/fine-tuned models or when using an unusual context window.
	ContextTokens int `yaml:"context_tokens"`

	// ToolLoop configures tool loop detection thresholds.
	ToolLoop ToolLoopConfig `yaml:"tool_loop"`

	// MemoryFlush configures pre-compaction memory flush behavior.
	MemoryFlush MemoryFlushConfig `yaml:"memory_flush"`

	// Compaction configures how context compaction preserves important information.
	Compaction CompactionConfig `yaml:"compaction"`
}

// MemoryFlushConfig configures pre-compaction memory flush behavior.
// This triggers a silent turn before compaction to save important memories.
type MemoryFlushConfig struct {
	// Enabled activates memory flush before compaction (default: true).
	Enabled bool `yaml:"enabled"`

	// ProactiveEnabled activates proactive flush before each run (default: true).
	// Uses token projection to decide whether flush is needed.
	ProactiveEnabled bool `yaml:"proactive_enabled"`

	// ProjectionThreshold is the fraction of context window that triggers proactive flush (default: 0.85).
	ProjectionThreshold float64 `yaml:"projection_threshold"`

	// ReserveTokensFloor is the minimum token buffer to maintain (default: 20000).
	ReserveTokensFloor int `yaml:"reserve_tokens_floor"`

	// FlushThreshold is the number of tokens before triggering flush (default: 4000).
	// Flush triggers when: tokenEstimate >= contextWindow - reserveFloor - flushThreshold
	FlushThreshold int `yaml:"flush_threshold"`

	// SystemPrompt is an optional custom system prompt for the flush turn.
	SystemPrompt string `yaml:"system_prompt"`

	// Prompt is the user prompt for the flush turn (default: standard prompt).
	Prompt string `yaml:"prompt"`
}

// DefaultAgentConfig returns sensible defaults for agent autonomy.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		RunTimeoutSeconds:     int(DefaultRunTimeout / time.Second),
		LLMCallTimeoutSeconds: int(DefaultLLMCallTimeout / time.Second),
		MaxTurns:              0, // Unlimited
		MaxContinuations:      2,
		ReflectionEnabled:     true,
		MaxCompactionAttempts: DefaultMaxCompactionAttempts,
		MemoryFlush: MemoryFlushConfig{
			Enabled:             true,
			ProactiveEnabled:    true,
			ProjectionThreshold: 0.85,
			ReserveTokensFloor:  20000,
			FlushThreshold:      4000,
		},
	}
}

// AgentRun encapsulates a single agent execution with its dependencies.
type AgentRun struct {
	llm                   *LLMClient
	executor              *ToolExecutor
	runTimeout            time.Duration // Total run timeout (default: 600s)
	llmCallTimeout        time.Duration // Per-LLM-call safety timeout (default: 5min)
	maxTurns              int           // 0 = unlimited
	maxContinuations      int           // Auto-continue rounds after maxTurns (default: 2)
	reflectionOn          bool
	maxCompactionAttempts int
	streamCallback        StreamCallback
	modelOverride         string                             // When set, use this model instead of default.
	usageRecorder         func(model string, usage LLMUsage) // Called after each successful LLM response.
	cfg                   AgentConfig                        // Agent configuration including memory flush settings.
	sessionPersistence    SessionPersister                   // For persisting compaction summaries
	sessionID             string                             // Session ID for compaction summary persistence

	// interruptCh receives follow-up user messages that should be injected into
	// the active agent loop. Between turns, the agent drains this channel and
	// appends the messages to the conversation before the next LLM call.
	// This enables Claude Code-style live message injection.
	interruptCh <-chan string

	// onBeforeToolExec is called right before tool execution starts.
	// Used to flush any buffered stream text so the user sees the LLM's
	// intermediate reasoning before tools run.
	onBeforeToolExec func()

	// onToolResult is called after each tool execution completes.
	// Used to auto-send media (e.g. generated images) to the channel.
	onToolResult func(name string, result ToolResult)

	// compactionSummaries holds persisted compaction summaries from prior runs.
	// Injected at run start from session state so buildMessages() can restore context.
	compactionSummaries []CompactionEntry

	// loopDetector tracks tool call history and detects repetitive patterns.
	loopDetector *ToolLoopDetector

	// usageAcc tracks token usage across LLM calls with last-call snapshots.
	usageAcc UsageAccumulator

	// lastRunToolSummary accumulates tool names called during this run.
	lastRunToolSummary string

	logger *slog.Logger
}

// UsageAccumulator tracks token usage across multiple LLM calls within a run.
// It maintains both cumulative totals and the most recent call's values.
// Using last-call values for context size estimation avoids the inflation problem
// where accumulated cacheRead across N calls = N × context_size.
type UsageAccumulator struct {
	TotalInput      int // Sum of all PromptTokens across calls
	TotalOutput     int // Sum of all CompletionTokens
	TotalCacheRead  int // Sum of all CacheReadTokens
	TotalCacheWrite int // Sum of all CacheWriteTokens
	LastInput       int // Most recent call's PromptTokens
	LastOutput      int // Most recent call's CompletionTokens
	LastCacheRead   int // Most recent call's CacheReadTokens
	LastCacheWrite  int // Most recent call's CacheWriteTokens
	CallCount       int // Number of LLM calls in this run
}

// Record accumulates a single LLM call's usage and updates last-call snapshots.
func (ua *UsageAccumulator) Record(usage LLMUsage) {
	ua.TotalInput += usage.PromptTokens
	ua.TotalOutput += usage.CompletionTokens
	ua.TotalCacheRead += usage.CacheReadTokens
	ua.TotalCacheWrite += usage.CacheWriteTokens
	ua.LastInput = usage.PromptTokens
	ua.LastOutput = usage.CompletionTokens
	ua.LastCacheRead = usage.CacheReadTokens
	ua.LastCacheWrite = usage.CacheWriteTokens
	ua.CallCount++
}

// EffectiveContextTokens returns the actual context size as reported by the
// provider in the most recent call. This is more accurate than character-based
// estimation because it uses real token counts from the API.
func (ua *UsageAccumulator) EffectiveContextTokens() int {
	return ua.LastInput + ua.LastCacheRead + ua.LastCacheWrite
}

// ToLLMUsage converts the accumulator totals to an LLMUsage for backward compat.
func (ua *UsageAccumulator) ToLLMUsage() LLMUsage {
	return LLMUsage{
		PromptTokens:     ua.TotalInput,
		CompletionTokens: ua.TotalOutput,
		TotalTokens:      ua.TotalInput + ua.TotalOutput,
		CacheReadTokens:  ua.TotalCacheRead,
		CacheWriteTokens: ua.TotalCacheWrite,
	}
}

// ToolSummary returns a digest of all tools called during this run.
func (a *AgentRun) ToolSummary() string {
	return strings.TrimSuffix(a.lastRunToolSummary, "; ")
}

// NewAgentRun creates a new agent runner.
func NewAgentRun(llm *LLMClient, executor *ToolExecutor, logger *slog.Logger) *AgentRun {
	return &AgentRun{
		llm:                   llm,
		executor:              executor,
		runTimeout:            DefaultRunTimeout,
		llmCallTimeout:        DefaultLLMCallTimeout,
		maxTurns:              0, // Unlimited
		reflectionOn:          true,
		maxCompactionAttempts: DefaultMaxCompactionAttempts,
		logger:                logger.With("component", "agent"),
	}
}

// NewAgentRunWithConfig creates a new agent runner with explicit configuration.
func NewAgentRunWithConfig(llm *LLMClient, executor *ToolExecutor, cfg AgentConfig, logger *slog.Logger) *AgentRun {
	ar := NewAgentRun(llm, executor, logger)
	if cfg.RunTimeoutSeconds > 0 {
		ar.runTimeout = time.Duration(cfg.RunTimeoutSeconds) * time.Second
	}
	if cfg.LLMCallTimeoutSeconds > 0 {
		ar.llmCallTimeout = time.Duration(cfg.LLMCallTimeoutSeconds) * time.Second
	}
	if cfg.MaxTurns >= 0 {
		ar.maxTurns = cfg.MaxTurns
	}
	ar.maxContinuations = cfg.MaxContinuations
	ar.reflectionOn = cfg.ReflectionEnabled
	if cfg.MaxCompactionAttempts > 0 {
		ar.maxCompactionAttempts = cfg.MaxCompactionAttempts
	}
	ar.cfg = cfg
	return ar
}

// SetStreamCallback sets the callback for streaming text deltas.
// When set, the agent uses CompleteWithToolsStream; only text content is forwarded,
// tool calls are accumulated silently.
func (a *AgentRun) SetStreamCallback(cb StreamCallback) {
	a.streamCallback = cb
}

// SetSessionPersistence wires session persistence for compaction summary storage.
func (a *AgentRun) SetSessionPersistence(p SessionPersister, sessionID string) {
	a.sessionPersistence = p
	a.sessionID = sessionID
}

// SetModelOverride sets the model to use instead of the default.
// Empty string means use the LLM client's default.
func (a *AgentRun) SetModelOverride(model string) {
	a.modelOverride = model
}

// SetUsageRecorder sets a callback invoked after each successful LLM response.
func (a *AgentRun) SetUsageRecorder(fn func(model string, usage LLMUsage)) {
	a.usageRecorder = fn
}

// SetOnBeforeToolExec sets a callback fired right before tool execution starts
// in the agent loop. Used by the block streamer to flush buffered text so the
// user sees intermediate reasoning before tools run.
func (a *AgentRun) SetOnBeforeToolExec(fn func()) {
	a.onBeforeToolExec = fn
}

// SetOnToolResult sets a callback fired after each tool execution completes.
// Used to auto-send media (e.g. generated images) to the channel.
func (a *AgentRun) SetOnToolResult(fn func(name string, result ToolResult)) {
	a.onToolResult = fn
}

// SetLoopDetector sets the tool loop detector for this run.
func (a *AgentRun) SetLoopDetector(d *ToolLoopDetector) {
	a.loopDetector = d
}

// SetInterruptChannel sets the channel for receiving follow-up user messages
// during agent execution. Messages received on this channel are injected into
// the conversation between agent turns, allowing users to steer the agent
// mid-run (similar to Claude Code behavior).
func (a *AgentRun) SetInterruptChannel(ch <-chan string) {
	a.interruptCh = ch
}

// Run executes the agent loop: builds the initial message list from conversation
// history, then iterates LLM calls and tool executions until a final response
// is produced or the turn limit is exhausted.
//
// If auto-continue is enabled and the agent is still using tools when the
// budget runs out, it will automatically start a continuation round.
func (a *AgentRun) Run(ctx context.Context, systemPrompt string, history []ConversationEntry, userMessage string) (string, error) {
	content, _, err := a.RunWithUsage(ctx, systemPrompt, history, userMessage)
	return content, err
}

// RunWithUsage is like Run but also returns aggregated token usage from all LLM calls.
//
// Architecture:
//   - The loop runs until the LLM produces a response with no tool calls.
//   - A single run-level timeout controls the entire execution (default: 600s).
//   - Individual LLM calls have a safety-net timeout (5min) to catch hung connections.
//   - No fixed turn limit — the agent keeps going as long as it has tools to call.
func (a *AgentRun) RunWithUsage(ctx context.Context, systemPrompt string, history []ConversationEntry, userMessage string) (string, *LLMUsage, error) {
	// ── Context window guard: block runs on models with insufficient capacity ──
	ctxTokens := ResolveContextWindowTokens(a.cfg.ContextTokens, a.modelOverride)
	guard := EvaluateContextWindowGuard(ctxTokens)
	if guard.ShouldBlock {
		return "", nil, fmt.Errorf("context window guard: %s", guard.Message)
	}
	if guard.ShouldWarn {
		a.logger.Warn("context window guard warning", "message", guard.Message)
	}

	// ── Run-level timeout (single timer for the whole run) ──
	runCtx, runCancel := context.WithTimeout(ctx, a.runTimeout)
	defer runCancel()

	runStart := time.Now()

	// Build initial messages from history.
	messages := a.buildMessages(systemPrompt, history, userMessage)

	// Collect tool definitions from the executor, filtered by profile if present.
	allTools := a.executor.Tools()
	var tools []ToolDefinition

	profile := ToolProfileFromContext(runCtx)
	if profile != nil {
		// Filter tools by profile
		allToolNames := a.executor.ToolNames()
		checker := NewProfileChecker(profile.Allow, profile.Deny, allToolNames)
		for _, tool := range allTools {
			if allowed, _ := checker.Check(tool.Function.Name); allowed {
				tools = append(tools, tool)
			}
		}
		a.logger.Debug("tools filtered by profile",
			"profile", profile.Name,
			"total_tools", len(allTools),
			"allowed_tools", len(tools),
		)
	} else {
		tools = allTools
	}

	// Compact tool descriptions to save tokens in the tool definitions payload.
	const maxDescLen = 120
	for i := range tools {
		if len(tools[i].Function.Description) > maxDescLen {
			tools[i].Function.Description = tools[i].Function.Description[:maxDescLen-3] + "..."
		}
	}

	// Limit tools to 128 for OpenAI API compatibility.
	const maxTools = 128
	if len(tools) > maxTools {
		a.logger.Warn("too many tools, truncating to max",
			"total", len(tools),
			"max", maxTools,
		)
		tools = tools[:maxTools]
	}

	a.logger.Debug("agent run started",
		"history_entries", len(history),
		"tools_available", len(tools),
		"run_timeout_s", int(a.runTimeout.Seconds()),
		"max_turns", a.maxTurns,
	)

	// If no tools are registered, do a single completion and return.
	if len(tools) == 0 {
		resp, err := a.doLLMCallWithOverflowRetry(runCtx, messages, nil)
		if err != nil {
			return "", nil, err
		}
		var totalUsage LLMUsage
		a.accumulateUsage(&totalUsage, resp)
		return resp.Content, &totalUsage, nil
	}

	var totalUsage LLMUsage
	totalTurns := 0

	// Progress cooldown: avoid flooding the user with tool progress messages.
	// Short 3s cooldown for faster feedback while avoiding message spam.
	const progressCooldown = 3 * time.Second
	var lastProgressAt time.Time
	var continuationRounds int

	// ── Main agent loop ──
	// Loop until: (1) LLM produces no tool calls, (2) run timeout fires, or
	// (3) optional soft turn limit + continuation budget is exhausted.
	for {
		totalTurns++
		turnStart := time.Now()

		a.logger.Debug("agent turn start",
			"turn", totalTurns,
			"messages", len(messages),
			"run_elapsed_s", int(time.Since(runStart).Seconds()),
		)

		// ── Soft turn limit with continuation budget ──
		if a.maxTurns > 0 && totalTurns > a.maxTurns {
			continuationRounds++

			if continuationRounds > a.maxContinuations {
				// Hard stop: request final summary and return.
				a.logger.Warn("agent exhausted continuation budget, forcing summary",
					"total_turns", totalTurns,
					"continuations", continuationRounds-1,
				)
				messages = append(messages, chatMessage{
					Role: "user",
					Content: "[System: You have exhausted your turn budget. " +
						"Provide your best final response NOW with the information gathered so far.]",
				})
				resp, err := a.doLLMCallWithOverflowRetry(runCtx, messages, nil)
				if err != nil {
					return "", nil, fmt.Errorf("final summary call failed: %w", err)
				}
				a.accumulateUsage(&totalUsage, resp)
				return resp.Content, &totalUsage, nil
			}

			// Soft nudge: ask to wrap up but allow continued tool use.
			a.logger.Warn("agent reached soft turn limit, continuation round",
				"total_turns", totalTurns,
				"max_turns", a.maxTurns,
				"continuation", continuationRounds,
				"max_continuations", a.maxContinuations,
			)
			messages = append(messages, chatMessage{
				Role: "user",
				Content: fmt.Sprintf(
					"[System: Turn limit reached (continuation %d/%d). "+
						"Wrap up your current task. If you have enough information, "+
						"provide your final response without further tool calls.]",
					continuationRounds, a.maxContinuations,
				),
			})
		}

		// ── Run timeout check ──
		if runCtx.Err() != nil {
			return "", &totalUsage, fmt.Errorf("agent run timeout (%s) after %d turns: %w",
				a.runTimeout, totalTurns, runCtx.Err())
		}

		// ── Interrupt injection ──
		// Check for follow-up user messages sent while the agent was working.
		if totalTurns > 1 {
			if interrupts := a.drainInterrupts(); len(interrupts) > 0 {
				for _, interrupt := range interrupts {
					messages = append(messages, chatMessage{
						Role:    "user",
						Content: "[Follow-up from user while processing]\n" + interrupt,
					})
				}
				a.logger.Info("injected interrupt messages into agent loop",
					"count", len(interrupts),
					"turn", totalTurns,
				)
			}
		}

		// ── Proactive context compaction ──
		// Instead of dropping old messages entirely (which causes amnesia), we
		// compact the context if it grows too large.
		if totalTurns > 5 && len(messages) > 15 {
			a.logger.Debug("checking context size for compaction", "messages_len", len(messages))
			messages = a.managedCompaction(runCtx, messages)
		}

		// Inject reflection nudge periodically so the agent is aware of duration.
		// More aggressive messaging to catch stuck patterns early.
		if a.reflectionOn && totalTurns > 1 && totalTurns%reflectionInterval == 0 {
			elapsed := time.Since(runStart).Seconds()
			remaining := a.runTimeout.Seconds() - elapsed
			messages = append(messages, chatMessage{
				Role: "user",
				Content: fmt.Sprintf(
					"[System: Turn %d checkpoint (%.0fs elapsed, ~%.0fs remaining). "+
						"If you're stuck or repeating the same approach, STOP and investigate the root cause. "+
						"Don't repeat failed approaches — think before acting.]",
					totalTurns, elapsed, remaining,
				),
			})
		}

		// ── Call LLM ──
		llmStart := time.Now()
		resp, err := a.doLLMCallWithOverflowRetry(runCtx, messages, tools)
		llmDuration := time.Since(llmStart)
		if err != nil {
			// If the parent/run context was cancelled, propagate immediately.
			if runCtx.Err() != nil {
				// Distinguish user abort from run timeout.
				if ctx.Err() != nil {
					return "", &totalUsage, fmt.Errorf("agent cancelled by user: %w", ctx.Err())
				}
				return "", &totalUsage, fmt.Errorf("agent run timeout (%s) at turn %d: %w",
					a.runTimeout, totalTurns, runCtx.Err())
			}

			// Timeout or transient error on a later turn: try compacting
			// the context and retrying once before giving up.
			errStr := err.Error()
			isTimeout := strings.Contains(errStr, "deadline exceeded") || strings.Contains(errStr, "context canceled") || strings.Contains(errStr, "rate limit")

			if isTimeout && totalTurns > 2 && len(messages) > 10 {
				a.logger.Warn("LLM call timed out or rate limited, aggressive compaction and retry",
					"turn", totalTurns,
					"messages_before", len(messages),
					"llm_ms", llmDuration.Milliseconds(),
				)
				messages = a.aggressiveCompaction(runCtx, messages)

				// Retry the LLM call with compacted context.
				llmStart = time.Now()
				resp, err = a.doLLMCallWithOverflowRetry(runCtx, messages, tools)
				llmDuration = time.Since(llmStart)
			}

			if err != nil {
				return "", &totalUsage, fmt.Errorf("LLM call failed (turn %d, llm_ms=%d): %w",
					totalTurns, llmDuration.Milliseconds(), err)
			}
		}
		a.accumulateUsage(&totalUsage, resp)

		a.logger.Info("LLM call complete",
			"turn", totalTurns,
			"llm_ms", llmDuration.Milliseconds(),
			"tool_calls", len(resp.ToolCalls),
			"prompt_tokens", resp.Usage.PromptTokens,
			"completion_tokens", resp.Usage.CompletionTokens,
		)

		// ── Strict <think> Parsing ──
		if strings.Contains(resp.Content, "<think>") && !strings.Contains(resp.Content, "</think>") {
			a.logger.Warn("llm missed closing </think> tag, prompting retry without executing tools")

			// Append assistant message so the user message makes sense in context
			messages = append(messages, chatMessage{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})

			messages = append(messages, chatMessage{
				Role:    "user",
				Content: "[System: You opened a <think> tag but did not close it with </think>. Please close your <think> tag, and place any tool calls or final responses AFTER the </think> tag. Do not execute tools until you finish thinking.]",
			})
			// Loop again without executing any returned tool calls or triggering final response
			continue
		}

		// ── Validate tool calls from LLM response ──
		// Discard incomplete tool calls (empty ID or missing function name)
		// to prevent transcript corruption.
		if len(resp.ToolCalls) > 0 {
			valid := make([]ToolCall, 0, len(resp.ToolCalls))
			for _, tc := range resp.ToolCalls {
				if tc.ID == "" || tc.Function.Name == "" {
					a.logger.Warn("discarding incomplete tool call",
						"id", tc.ID, "name", tc.Function.Name)
					continue
				}
				valid = append(valid, tc)
			}
			resp.ToolCalls = valid
		}

		// ── No tool calls → final response ──
		if len(resp.ToolCalls) == 0 {
			a.logger.Info("agent completed",
				"total_turns", totalTurns,
				"response_len", len(resp.Content),
				"run_elapsed_ms", time.Since(runStart).Milliseconds(),
			)
			return resp.Content, &totalUsage, nil
		}

		// Sanitize tool call IDs for provider compatibility (OpenAI, Mistral, etc.).
		idMode := ProviderToolCallIDMode(a.llm.provider)
		for i := range resp.ToolCalls {
			resp.ToolCalls[i].ID = SanitizeToolCallID(resp.ToolCalls[i].ID, idMode)
		}

		// Append assistant message with tool calls to the conversation.
		messages = append(messages, chatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// ── Tool Loop Detection ──
		// Record tool calls and check for repetitive patterns before execution.
		// Warnings/criticals are deferred until AFTER tool results to maintain
		// valid message ordering (assistant→tool→user, not assistant→user→tool).
		var loopWarning string
		if a.loopDetector != nil {
			for _, tc := range resp.ToolCalls {
				args, _ := parseToolArgs(tc.Function.Arguments)
				result := a.loopDetector.RecordAndCheck(tc.Function.Name, args)

				switch result.Severity {
				case LoopBreaker:
					a.logger.Error("tool loop circuit breaker",
						"tool", tc.Function.Name, "streak", result.Streak, "pattern", result.Pattern)
					return result.Message, &totalUsage, nil

				case LoopCritical, LoopWarning:
					loopWarning = result.Message
				}
			}
		}

		// Execute all requested tool calls.
		toolStart := time.Now()
		toolNames := make([]string, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			toolNames[i] = tc.Function.Name
		}
		a.logger.Info("executing tool calls",
			"count", len(resp.ToolCalls),
			"tools", strings.Join(toolNames, ","),
			"turn", totalTurns,
		)

		// Flush any buffered stream text before tools start — ensures the user
		// sees the LLM's intermediate reasoning/thoughts immediately.
		if a.onBeforeToolExec != nil {
			a.onBeforeToolExec()
		}

		// Send progress to the user so they see what the agent is doing
		// while tools execute (especially for long-running tools).
		// When a stream callback (BlockStreamer) is active, it already delivers
		// the LLM's text progressively — sending resp.Content via ProgressSender
		// would duplicate messages. Only send tool descriptions as progress.
		if ps := ProgressSenderFromContext(runCtx); ps != nil {
			now := time.Now()
			if now.Sub(lastProgressAt) >= progressCooldown {
				var progressMsg string

				if a.streamCallback != nil {
					// BlockStreamer is active and already sent resp.Content
					// to the channel — only send a tool description as progress.
					progressMsg = formatToolProgressMessage(resp.ToolCalls)
				} else if resp.Content != "" && len(resp.Content) < 1000 {
					// No streamer — send the LLM's own text as progress.
					progressMsg = resp.Content
				} else if resp.Content != "" {
					progressMsg = resp.Content[:500] + "..."
				} else {
					progressMsg = formatToolProgressMessage(resp.ToolCalls)
				}

				if progressMsg != "" {
					ps(runCtx, progressMsg)
					lastProgressAt = now
				}
			}
		}

		results := a.executor.Execute(runCtx, resp.ToolCalls)

		a.logger.Info("tool calls complete",
			"count", len(results),
			"tools_ms", time.Since(toolStart).Milliseconds(),
			"turn_ms", time.Since(turnStart).Milliseconds(),
		)

		// Synthesize results for orphan tool_use blocks (interrupted execution).
		// Providers reject requests with tool_use lacking a matching tool_result.
		if len(results) < len(resp.ToolCalls) {
			resultIDs := make(map[string]bool, len(results))
			for _, r := range results {
				resultIDs[r.ToolCallID] = true
			}
			for _, tc := range resp.ToolCalls {
				if !resultIDs[tc.ID] {
					a.logger.Warn("synthesizing tool_result for orphaned tool_use",
						"tool", tc.Function.Name, "id", tc.ID)
					results = append(results, ToolResult{
						ToolCallID: tc.ID,
						Name:       tc.Function.Name,
						Content:    "[Interrupted by user]",
						Error:      fmt.Errorf("interrupted"),
					})
				}
			}
		}

		// Append each tool result as a message.
		for _, result := range results {
			content := result.Content
			if result.Error != nil && isRecoverableToolError(content) {
				a.logger.Debug("recoverable tool error (model should retry)",
					"tool", result.Name,
					"error_preview", truncateStr(content, 80),
				)
			}
			messages = append(messages, chatMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: result.ToolCallID,
			})

			// Track tool output for progress-aware loop detection.
			if a.loopDetector != nil {
				a.loopDetector.RecordToolOutcome(content)
			}

			// Notify hook (e.g. auto-send media for generate_image).
			if a.onToolResult != nil && result.Error == nil {
				a.onToolResult(result.Name, result)
			}
		}

		// Accumulate tool names for provenance tracking.
		if len(resp.ToolCalls) > 0 {
			names := make([]string, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				names[i] = tc.Function.Name
			}
			a.lastRunToolSummary += strings.Join(names, ",") + "; "
		}

		// Inject deferred loop warning AFTER tool results (valid message order:
		// assistant→tool→user). This ensures providers that validate message
		// sequences don't reject the request.
		if loopWarning != "" {
			messages = append(messages, chatMessage{
				Role:    "user",
				Content: "[System] " + loopWarning,
			})
		}
	}
}

// formatToolProgressMessage creates a clean, concise, user-facing message about
// what the agent is doing. Designed for chat apps (WhatsApp, Telegram).
// Unlike step-by-step output, this shows a single summarized line.
// Format: emoji + label + optional detail.
func formatToolProgressMessage(toolCalls []ToolCall) string {
	if len(toolCalls) == 0 {
		return ""
	}

	// For a single tool call, show a concise description.
	if len(toolCalls) == 1 {
		name := toolCalls[0].Function.Name
		args, _ := parseToolArgs(toolCalls[0].Function.Arguments)
		return describeToolAction(name, args)
	}

	// For multiple parallel tool calls, summarize them.
	// Show the most interesting one (longest description) and count the rest.
	var best string
	count := 0
	for _, tc := range toolCalls {
		name := tc.Function.Name
		args, _ := parseToolArgs(tc.Function.Arguments)
		desc := describeToolAction(name, args)
		if desc != "" {
			count++
			if len(desc) > len(best) {
				best = desc
			}
		}
	}

	if count == 0 {
		return ""
	}
	if count == 1 {
		return best
	}
	return fmt.Sprintf("%s (+%d)", best, count-1)
}

// describeToolAction returns a human-friendly, emoji-prefixed description
// of a tool call. Empty string means "skip this tool in progress output".
func describeToolAction(name string, args map[string]any) string {
	switch name {
	// ── Shell / commands ──
	case "bash", "exec":
		cmd, _ := args["command"].(string)
		if cmd == "" {
			return "💻 Executando comando..."
		}
		if len(cmd) > 60 {
			cmd = cmd[:60] + "..."
		}
		return "💻 `" + cmd + "`"

	// ── File operations ──
	case "read_file":
		p, _ := args["path"].(string)
		if p != "" {
			return "📖 Lendo " + shortPath(p)
		}
		return "📖 Lendo arquivo..."

	case "write_file":
		p, _ := args["path"].(string)
		if p != "" {
			return "✍️ Escrevendo " + shortPath(p)
		}
		return "✍️ Escrevendo arquivo..."

	case "edit_file":
		p, _ := args["path"].(string)
		if p != "" {
			return "✏️ Editando " + shortPath(p)
		}
		return "✏️ Editando arquivo..."

	case "list_files", "glob_files":
		p, _ := args["path"].(string)
		if p == "" {
			p, _ = args["pattern"].(string)
		}
		if p != "" {
			return "📂 Listando " + shortPath(p)
		}
		return "📂 Listando arquivos..."

	case "search_files":
		q, _ := args["query"].(string)
		if q == "" {
			q, _ = args["pattern"].(string)
		}
		if q != "" {
			return "🔎 Buscando: " + q
		}
		return "🔎 Buscando nos arquivos..."

	// ── Web ──
	case "web_search", "brave-search_execute", "brave-search_run_search":
		q, _ := args["query"].(string)
		if q != "" {
			if len(q) > 60 {
				q = q[:60] + "..."
			}
			return "🔍 Pesquisando: " + q
		}
		return "🔍 Pesquisando na web..."

	case "web_fetch", "web-fetch_fetch_url":
		u, _ := args["url"].(string)
		if u != "" {
			if len(u) > 55 {
				u = u[:55] + "..."
			}
			return "🌐 Acessando " + u
		}
		return "🌐 Acessando página..."

	// ── Memory ──
	case "memory":
		action, _ := args["action"].(string)
		switch action {
		case "save":
			return "💾 Saving to memory..."
		case "search":
			q, _ := args["query"].(string)
			if q != "" {
				return "🧠 Recalling: " + q
			}
			return "🧠 Searching memory..."
		case "list", "index":
			return "🧠 Organizing memories..."
		default:
			return "🧠 Memory..."
		}

	// ── Remote ──
	case "ssh":
		host, _ := args["host"].(string)
		cmd, _ := args["command"].(string)
		if host != "" && cmd != "" {
			if len(cmd) > 40 {
				cmd = cmd[:40] + "..."
			}
			return "🔗 " + host + ": `" + cmd + "`"
		}
		if host != "" {
			return "🔗 Conectando em " + host + "..."
		}
		return "🔗 Conectando via SSH..."

	case "scp":
		src, _ := args["source"].(string)
		dst, _ := args["destination"].(string)
		if src != "" && dst != "" {
			return "📤 Transferindo " + shortPath(src) + " → " + shortPath(dst)
		}
		return "📤 Transferindo arquivo..."

	// ── Coding ──
	case "claude-code_execute":
		p, _ := args["prompt"].(string)
		if p != "" {
			if len(p) > 55 {
				p = p[:55] + "..."
			}
			return "🤖 Codificando: " + p
		}
		return "🤖 Executando Claude Code..."
	case "claude-code_check":
		return "🤖 Verificando Claude Code..."

	// ── Images ──
	case "describe_image":
		return "👁️ Analisando imagem..."
	case "image-gen_generate_image":
		p, _ := args["prompt"].(string)
		if p != "" {
			if len(p) > 50 {
				p = p[:50] + "..."
			}
			return "🎨 Gerando imagem: " + p
		}
		return "🎨 Gerando imagem..."

	// ── Audio ──
	case "transcribe_audio":
		return "🎤 Transcrevendo áudio..."

	// ── Scheduler dispatcher ──
	case "scheduler":
		action, _ := args["action"].(string)
		switch action {
		case "add":
			return "⏰ Criando agendamento..."
		case "list":
			return "⏰ Listando agendamentos..."
		case "remove":
			return "⏰ Removendo agendamento..."
		default:
			return "⏰ Gerenciando agendamentos..."
		}

	// ── Vault dispatcher ──
	case "vault":
		action, _ := args["action"].(string)
		switch action {
		case "save":
			return "🔐 Salvando no cofre..."
		case "get":
			return "🔐 Buscando no cofre..."
		case "list":
			return "🔐 Listando cofre..."
		case "delete":
			return "🔐 Removendo do cofre..."
		default:
			return "🔐 Gerenciando cofre..."
		}

	// ── Skills dispatcher ──
	case "skill_manage":
		action, _ := args["action"].(string)
		switch action {
		case "install":
			s, _ := args["name"].(string)
			if s != "" {
				return "📦 Instalando skill: " + s
			}
			return "📦 Instalando skill..."
		case "list", "search":
			return "📋 Listando skills..."
		default:
			return "📦 Gerenciando skills..."
		}

	// ── Subagents ──
	case "spawn_subagent":
		label, _ := args["label"].(string)
		if label == "" {
			label, _ = args["task"].(string)
			if len(label) > 40 {
				label = label[:40] + "..."
			}
		}
		if label != "" {
			return "🧵 Iniciando subagente: " + label
		}
		return "🧵 Iniciando subagente..."
	case "list_subagents":
		return "🧵 Verificando subagentes..."
	case "wait_subagent":
		return "⏳ Aguardando subagente..."
	case "stop_subagent":
		return "🛑 Parando subagente..."

	// ── Project Manager ──
	case "project-manager_activate":
		p, _ := args["name"].(string)
		if p != "" {
			return "📁 Ativando projeto: " + p
		}
		return "📁 Ativando projeto..."
	case "project-manager_list":
		return "📁 Listando projetos..."
	case "project-manager_scan", "project-manager_tree":
		return "📁 Escaneando projeto..."
	case "project-manager_register":
		return "📁 Registrando projeto..."

	// ── Calculator / DateTime ──
	case "calculator_calculate":
		return "" // silent — too trivial
	case "datetime_current_time":
		return "" // silent

	default:
		// For skill tools (prefixed), make it cleaner.
		if strings.Contains(name, "_execute") {
			skillName := strings.TrimSuffix(name, "_execute")
			skillName = strings.ReplaceAll(skillName, "_", " ")
			skillName = strings.ReplaceAll(skillName, "-", " ")
			return "⚡ Executando " + skillName + "..."
		}
		if strings.Contains(name, "_run_") {
			parts := strings.SplitN(name, "_run_", 2)
			skillName := strings.ReplaceAll(parts[0], "-", " ")
			action := strings.ReplaceAll(parts[1], "_", " ")
			return "⚡ " + skillName + ": " + action + "..."
		}
		return "⚙️ " + strings.ReplaceAll(name, "_", " ") + "..."
	}
}

// shortPath returns the last 2 segments of a path for display.
// "/home/user/projects/app/src/index.ts" → "src/index.ts"
func shortPath(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) <= 2 {
		return p
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

// isRecoverableToolError checks if a tool error is likely transient or due to
// incorrect parameters, so the model should retry without surfacing it to the user.
// Classifies errors that the model can recover from by retrying or adjusting parameters.
func isRecoverableToolError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	patterns := []string{
		"required",       // "path is required", "prompt is required"
		"missing",        // "missing parameter"
		"not found",      // "file not found" (model can fix path)
		"invalid",        // "invalid argument"
		"parsing",        // "error parsing arguments"
		"no such file",   // fs errors
		"does not exist", // resource not found
		"permission denied",
		"timed out", // transient timeout
		"connection refused",
		"empty", // "command is empty"
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// truncateStr truncates a string to n characters for logging.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// drainInterrupts reads all pending messages from the interrupt channel
// without blocking. Returns nil if no messages are available.
func (a *AgentRun) drainInterrupts() []string {
	if a.interruptCh == nil {
		return nil
	}
	var msgs []string
	for {
		select {
		case msg, ok := <-a.interruptCh:
			if !ok {
				return msgs // Channel closed.
			}
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

// accumulateUsage adds resp.Usage into the run's UsageAccumulator and a legacy total.
func (a *AgentRun) accumulateUsage(total *LLMUsage, resp *LLMResponse) {
	if resp == nil {
		return
	}
	total.PromptTokens += resp.Usage.PromptTokens
	total.CompletionTokens += resp.Usage.CompletionTokens
	total.TotalTokens += resp.Usage.TotalTokens
	total.CacheReadTokens += resp.Usage.CacheReadTokens
	total.CacheWriteTokens += resp.Usage.CacheWriteTokens
	// Also record in the structured accumulator for last-call tracking.
	a.usageAcc.Record(resp.Usage)
}

// buildMessages converts conversation history into the chat message format.
func (a *AgentRun) buildMessages(systemPrompt string, history []ConversationEntry, userMessage string) []chatMessage {
	messages := make([]chatMessage, 0, len(history)*2+2)

	if systemPrompt != "" {
		messages = append(messages, chatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// Inject most recent compaction summary if available (restores context from prior compaction).
	// Uses "user" role to avoid double "system" messages which break Anthropic's API.
	if len(a.compactionSummaries) > 0 {
		latest := a.compactionSummaries[len(a.compactionSummaries)-1]
		if latest.Summary != "" {
			messages = append(messages, chatMessage{
				Role:    "user",
				Content: "[System: The following is a summary of earlier steps from a prior compaction.]\n" + latest.Summary,
			})
		}
	}

	for _, entry := range history {
		messages = append(messages, chatMessage{
			Role:    "user",
			Content: entry.UserMessage,
		})
		if entry.AssistantResponse != "" {
			content := entry.AssistantResponse
			// Annotate with tool provenance so the LLM knows what was
			// actually verified vs. inferred in previous turns.
			// Uses XML tags to avoid the LLM mimicking the annotation
			// as plain text in its own responses.
			if entry.ToolSummary != "" {
				content = "<tool_provenance>" + entry.ToolSummary + "</tool_provenance>\n" + content
			}
			messages = append(messages, chatMessage{
				Role:    "assistant",
				Content: content,
			})
		}
	}

	messages = append(messages, chatMessage{
		Role:    "user",
		Content: userMessage,
	})

	return messages
}

// isContextOverflow checks if an error indicates context length exceeded.
// Delegates to the comprehensive IsLikelyContextOverflowError for pattern matching.
func isContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	return IsLikelyContextOverflowError(err.Error())
}

// DefaultImagePruneAfterTurns is how many turns back to keep images.
// Images older than this are replaced with a text placeholder to prevent
// token accumulation from multimodal content.
const DefaultImagePruneAfterTurns = 5

// pruneOldImages replaces image content blocks with text placeholders
// in messages older than pruneAfterTurns from the end. This prevents
// token accumulation from multimodal content in long conversations.
func pruneOldImages(messages []chatMessage, pruneAfterTurns int) []chatMessage {
	if pruneAfterTurns <= 0 {
		pruneAfterTurns = DefaultImagePruneAfterTurns
	}

	// Count user turns from the end to determine the cutoff.
	userTurnCount := 0
	cutoffIdx := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			userTurnCount++
			if userTurnCount >= pruneAfterTurns {
				cutoffIdx = i
				break
			}
		}
	}

	if cutoffIdx == 0 {
		return messages // Not enough turns to prune.
	}

	result := make([]chatMessage, len(messages))
	copy(result, messages)

	for i := 0; i < cutoffIdx; i++ {
		parts, ok := result[i].Content.([]contentPart)
		if !ok {
			continue
		}
		var newParts []contentPart
		hasImage := false
		for _, p := range parts {
			if p.Type == "image_url" {
				hasImage = true
				newParts = append(newParts, contentPart{
					Type: "text",
					Text: "[image removed to save context]",
				})
			} else {
				newParts = append(newParts, p)
			}
		}
		if hasImage {
			result[i].Content = newParts
		}
	}

	return result
}

// RepairToolUseResultPairing scans messages and removes orphan tool results
// (tool_result messages whose corresponding tool_use was removed) and orphan
// tool_use calls (assistant tool calls with no matching tool result).
// This prevents API rejections from providers that require strict pairing.
func RepairToolUseResultPairing(messages []chatMessage) []chatMessage {
	// Pass 1: collect all tool call IDs from assistant messages.
	toolCallIDs := make(map[string]bool)
	for _, m := range messages {
		if m.Role == "assistant" {
			for _, tc := range m.ToolCalls {
				if tc.ID != "" {
					toolCallIDs[tc.ID] = true
				}
			}
		}
	}

	// Pass 2: collect all tool result IDs.
	toolResultIDs := make(map[string]bool)
	for _, m := range messages {
		if m.Role == "tool" && m.ToolCallID != "" {
			toolResultIDs[m.ToolCallID] = true
		}
	}

	// Pass 3: filter — remove orphan tool results and strip orphan tool calls.
	var result []chatMessage
	for _, m := range messages {
		switch m.Role {
		case "tool":
			// Keep only if the corresponding tool_use exists.
			if m.ToolCallID == "" || toolCallIDs[m.ToolCallID] {
				result = append(result, m)
			}
		case "assistant":
			if len(m.ToolCalls) > 0 {
				// Filter tool calls to only those with matching results.
				var validCalls []ToolCall
				for _, tc := range m.ToolCalls {
					if tc.ID == "" || toolResultIDs[tc.ID] {
						validCalls = append(validCalls, tc)
					}
				}
				if len(validCalls) != len(m.ToolCalls) {
					// Some calls were orphaned — create a copy with filtered calls.
					cleaned := m
					cleaned.ToolCalls = validCalls
					result = append(result, cleaned)
				} else {
					result = append(result, m)
				}
			} else {
				result = append(result, m)
			}
		default:
			result = append(result, m)
		}
	}

	return result
}

// findSafeCutPoint ensures we don't cut in the middle of a tool call/result sequence.
// It returns an adjusted index that points to a user or assistant message (not tool).
func (a *AgentRun) findSafeCutPoint(messages []chatMessage, proposedIdx int) int {
	// Walk backward from proposed index to find a safe starting point
	for i := proposedIdx; i > 0; i-- {
		if messages[i].Role == "user" || (messages[i].Role == "assistant" && len(messages[i].ToolCalls) == 0) {
			// Found a safe starting point (user message or assistant without pending tool calls)
			return i
		}
		if messages[i].Role == "assistant" && len(messages[i].ToolCalls) > 0 {
			// This assistant has tool calls, so we need to include it
			return i
		}
		// If it's a tool message, keep going backward
	}
	return proposedIdx
}

// truncateToolResults shortens tool result messages that exceed maxLen.
func (a *AgentRun) truncateToolResults(messages []chatMessage, maxLen int) []chatMessage {
	if maxLen <= 0 {
		maxLen = 2000
	}
	truncSuffix := "... [truncated]"
	keepChars := 1000
	if keepChars+len(truncSuffix) > maxLen {
		keepChars = maxLen - len(truncSuffix)
	}

	result := make([]chatMessage, len(messages))
	for i, m := range messages {
		result[i] = m
		if m.Role == "tool" {
			if s, ok := m.Content.(string); ok && len(s) > maxLen {
				result[i].Content = s[:keepChars] + truncSuffix
			}
		}
	}
	return result
}

// pruneOldToolResults implements proactive context trimming.
// Tool results are tagged with their turn number. Older results are progressively
// truncated or removed to keep the context lean without waiting for overflow.
func (a *AgentRun) pruneOldToolResults(messages []chatMessage, currentTurn int) []chatMessage {
	const (
		softTrimAge   = 5   // Turns before soft trim (truncate to 500 chars)
		hardTrimAge   = 10  // Turns before hard trim (remove entirely)
		softTrimChars = 500 // Max chars after soft trim
	)

	// Estimate the "turn" of each message based on position. Tool messages
	// between two assistant messages belong to the same turn.
	msgCount := len(messages)
	if msgCount < 10 {
		return messages
	}

	// Count tool result messages from the end to estimate age.
	toolResultCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" {
			toolResultCount++
		}
	}

	if toolResultCount < softTrimAge {
		return messages // Not enough tool results to prune.
	}

	// Walk through messages, trimming old tool results.
	result := make([]chatMessage, 0, len(messages))
	toolIdx := 0
	for _, m := range messages {
		if m.Role == "tool" {
			age := toolResultCount - toolIdx
			toolIdx++

			if age > hardTrimAge {
				// Hard trim: skip this tool result entirely.
				result = append(result, chatMessage{
					Role:       m.Role,
					Content:    "[tool result removed — too old]",
					ToolCallID: m.ToolCallID,
				})
				continue
			}

			if age > softTrimAge {
				// Soft trim: truncate to 500 chars.
				if s, ok := m.Content.(string); ok && len(s) > softTrimChars {
					m.Content = s[:softTrimChars] + "... [truncated — old result]"
				}
			}
		}
		result = append(result, m)
	}

	return result
}

// doLLMCallWithOverflowRetry runs the LLM call and retries with compaction on context overflow.
// The per-call timeout is a safety net (llmCallTimeout, default 5min) — the primary timeout
// is the run-level context passed in ctx.
//
// Compaction strategy:
//  1. First attempt: truncate oversized tool results (>4K chars).
//  2. Second attempt: compact messages (keep last N) + truncate tool results harder.
//  3. Third attempt: aggressive compaction (keep fewer messages).
//  4. Final attempt: emergency compression (keep only system + last user message).
func (a *AgentRun) doLLMCallWithOverflowRetry(ctx context.Context, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	toolResultTruncated := false

	// Pre-LLM context guard: cap oversized tool results and compact oldest
	// when total tool content exceeds the context budget.
	ctxTokens := ResolveContextWindowTokens(a.cfg.ContextTokens, a.modelOverride)
	messages = GuardToolResultContext(messages, ctxTokens)

	// Prune old images to prevent token accumulation from multimodal content.
	messages = pruneOldImages(messages, DefaultImagePruneAfterTurns)

	for attempt := 0; attempt < a.maxCompactionAttempts; attempt++ {
		// Use the shorter of: run context deadline or llmCallTimeout safety net.
		callCtx, cancel := context.WithTimeout(ctx, a.llmCallTimeout)
		var resp *LLMResponse
		var err error
		if a.streamCallback != nil {
			sanitizer := NewStreamSanitizer(a.streamCallback)
			resp, err = a.llm.CompleteWithToolsStreamUsingModel(callCtx, a.modelOverride, messages, tools, sanitizer.Callback())
			sanitizer.Flush()
		} else {
			resp, err = a.llm.CompleteWithFallbackUsingModel(callCtx, a.modelOverride, messages, tools)
		}
		cancel()

		if err == nil {
			if a.usageRecorder != nil && resp.Usage.TotalTokens > 0 {
				a.usageRecorder(resp.ModelUsed, resp.Usage)
			}
			return resp, nil
		}

		if !isContextOverflow(err) {
			return nil, err
		}

		a.logger.Info("context overflow detected",
			"attempt", attempt+1,
			"max_attempts", a.maxCompactionAttempts,
			"messages_before", len(messages),
		)

		// ── Compaction strategy ──
		// Step 1: Try truncating oversized tool results first (cheap operation).
		if !toolResultTruncated {
			if hasOversizedToolResults(messages, 4000) {
				a.logger.Info("truncating oversized tool results before compaction")
				messages = a.truncateToolResults(messages, 4000)
				toolResultTruncated = true
				continue // Retry without compacting messages.
			}
		}

		// Step 2: Memory flush before first compaction (if enabled)
		// This gives the agent a chance to save important memories before compaction
		if attempt == 0 && a.cfg.MemoryFlush.Enabled {
			// Estimate tokens based on message content
			tokenEstimate := a.estimateTokens(messages)
			a.maybeMemoryFlush(ctx, messages, tokenEstimate)
		}

		// Step 3+4: Compact messages using LLM summarization.
		a.logger.Info("performing managed compaction due to overflow")
		if attempt == 0 { // First compaction attempt after initial truncation
			messages = a.managedCompaction(ctx, messages)
		} else if attempt < a.maxCompactionAttempts-1 { // Subsequent attempts, use aggressive compaction
			messages = a.aggressiveCompaction(ctx, messages)
		} else {
			// Before emergency compression: try progressive tool result truncation.
			// This is cheaper than discarding entire conversation history.
			for _, limit := range []int{2000, 500} {
				if hasOversizedToolResults(messages, limit) {
					var n int
					messages, n = a.truncateOversizedToolResultsInHistory(messages, limit)
					a.logger.Info("progressive tool result truncation",
						"limit", limit, "truncated_count", n)
				}
			}
			// Final attempt: emergency compression
			a.logger.Warn("using emergency compression as last resort")
			messages = a.emergencyCompression(messages)
		}
	}

	return nil, fmt.Errorf("context overflow: compacted %d times but still exceeded context limit", a.maxCompactionAttempts)
}

// truncateOversizedToolResultsInHistory applies progressive truncation to oversized tool results.
// It tries progressively smaller limits (4000 → 2000 → 500) and returns the truncated messages
// along with how many tool results were truncated.
func (a *AgentRun) truncateOversizedToolResultsInHistory(messages []chatMessage, maxLen int) ([]chatMessage, int) {
	truncated := 0
	truncSuffix := "... [truncated]"

	result := make([]chatMessage, len(messages))
	for i, m := range messages {
		result[i] = m
		if m.Role == "tool" {
			if s, ok := m.Content.(string); ok && len(s) > maxLen {
				keepChars := maxLen - len(truncSuffix)
				if keepChars < 50 {
					keepChars = 50
				}
				result[i].Content = s[:keepChars] + truncSuffix
				truncated++
			}
		}
	}
	return result, truncated
}

// hasOversizedToolResults checks if any tool result message exceeds maxLen.
func hasOversizedToolResults(messages []chatMessage, maxLen int) bool {
	for _, m := range messages {
		if m.Role == "tool" {
			if s, ok := m.Content.(string); ok && len(s) > maxLen {
				return true
			}
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// Compaction logic
// ----------------------------------------------------------------------------

// managedCompaction takes the current message history and safely compacts the older half
// of the conversation into a summary block, preserving recent context and system prompts.
// maybeMemoryFlush triggers a silent agent turn before compaction to save important memories.
// This is called when context is nearing the limit, giving the agent a chance to persist
// critical information before messages are compacted/summarized.
func (a *AgentRun) maybeMemoryFlush(ctx context.Context, messages []chatMessage, tokenEstimate int) {
	if !a.cfg.MemoryFlush.Enabled {
		return
	}

	if a.llm == nil {
		a.logger.Warn("memory flush skipped: no LLM client available")
		return
	}

	contextWindow := a.getModelContextWindow()
	reserveFloor := a.cfg.MemoryFlush.ReserveTokensFloor
	if reserveFloor <= 0 {
		reserveFloor = 20000
	}
	flushThreshold := a.cfg.MemoryFlush.FlushThreshold
	if flushThreshold <= 0 {
		flushThreshold = 4000
	}

	// Check if we're nearing the context limit
	if tokenEstimate < contextWindow-reserveFloor-flushThreshold {
		return
	}

	// Build flush prompt
	flushPrompt := a.cfg.MemoryFlush.Prompt
	if flushPrompt == "" {
		flushPrompt = `Session nearing compaction. Review the conversation and write any important facts, decisions, or context to long-term memory using the memory tool (action='save', category='summary'). Save to memory/YYYY-MM-DD.md if needed. If there's nothing worth saving, reply with NO_REPLY.`
	}

	sysPrompt := a.cfg.MemoryFlush.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = "You are a memory preservation assistant. Your task is to save important information before context compaction. Be selective - only save truly valuable information."
	}

	// Build conversation for the silent flush turn
	flushMessages := make([]chatMessage, 0, len(messages)+1)
	flushMessages = append(flushMessages, chatMessage{Role: "system", Content: sysPrompt})

	// Include recent conversation context (last 10 messages)
	startIdx := len(messages) - 10
	if startIdx < 0 {
		startIdx = 0
	}
	for i := startIdx; i < len(messages); i++ {
		flushMessages = append(flushMessages, messages[i])
	}
	flushMessages = append(flushMessages, chatMessage{Role: "user", Content: flushPrompt})

	// Execute silent turn
	a.logger.Info("executing pre-compaction memory flush", "token_estimate", tokenEstimate)

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := a.llm.CompleteWithFallbackUsingModel(callCtx, a.modelOverride, flushMessages, nil)
	if err != nil {
		a.logger.Warn("memory flush failed", "error", err)
		return
	}

	// Check for NO_REPLY - if so, don't process further
	if strings.TrimSpace(strings.ToUpper(resp.Content)) == "NO_REPLY" {
		a.logger.Debug("memory flush: no memories to save")
		return
	}

	// The LLM may have triggered memory saves via tool calls
	// We don't need to do anything special here - the tools executed normally
	a.logger.Info("memory flush completed", "response_length", len(resp.Content))
}

// estimateTokens provides a rough token estimate for a slice of messages.
// Uses model-aware chars-per-token ratio for more accurate estimation.
func (a *AgentRun) estimateTokens(messages []chatMessage) int {
	charCount := 0
	for _, m := range messages {
		charCount += len(m.Role)
		if s, ok := m.Content.(string); ok {
			charCount += len(s)
		} else if m.Content != nil {
			charCount += len(fmt.Sprintf("%v", m.Content))
		}
		if m.ToolCallID != "" {
			charCount += len(m.ToolCallID)
		}
	}
	ratio := charsPerToken(a.modelOverride)
	return int(float64(charCount)/ratio + 0.5)
}

// getModelContextWindow returns the context window size for the current model.
func (a *AgentRun) getModelContextWindow() int {
	return getModelContextWindowByName(a.modelOverride)
}

// getModelContextWindowByName returns the context window size for a given model name.
func getModelContextWindowByName(modelName string) int {
	model := strings.ToLower(modelName)
	if model == "" {
		model = "default"
	}

	switch {
	case strings.Contains(model, "gpt-4o") || strings.Contains(model, "gpt-5"):
		return 128000
	case strings.Contains(model, "gpt-4-turbo"):
		return 128000
	case strings.Contains(model, "gpt-4"):
		return 8192
	case strings.Contains(model, "claude-opus-4") || strings.Contains(model, "claude-sonnet-4"):
		return 200000
	case strings.Contains(model, "claude-4"):
		return 200000
	case strings.Contains(model, "claude-3-opus"):
		return 200000
	case strings.Contains(model, "claude-3.5") || strings.Contains(model, "claude-3.7"):
		return 200000
	case strings.Contains(model, "claude-3"):
		return 200000
	case strings.Contains(model, "glm-4"):
		return 128000
	default:
		return 128000
	}
}

func (a *AgentRun) managedCompaction(ctx context.Context, messages []chatMessage) []chatMessage {
	if len(messages) < 10 {
		return messages
	}

	// Keep system prompt(s) and the first user message (usually the goal)
	var header []chatMessage
	var body []chatMessage

	for _, m := range messages {
		if m.Role == "system" {
			header = append(header, m)
		} else {
			body = append(body, m)
		}
	}

	if len(body) < 8 {
		return messages
	}

	goal := body[0]

	// We want to compact the middle section and keep the most recent N messages
	keepRecent := a.computeAdaptiveKeepRecent(body)
	if len(body)-1 <= keepRecent {
		return messages // Too small to compact
	}

	// Find safe cut point to avoid orphan tool messages
	cutIdx := len(body) - keepRecent
	cutIdx = a.findSafeCutPoint(body, cutIdx)

	middle := body[1:cutIdx]
	if len(middle) == 0 {
		return messages // Nothing to summarize after safe cut adjustment.
	}
	recent := body[cutIdx:]

	// Wrap summarization with a safety timeout (5 min) to prevent hung compactions.
	// If the LLM summarization hangs, fall back to a simple trim-by-count strategy.
	compactionCtx, compactionCancel := context.WithTimeout(ctx, compactionSafetyTimeout)
	summary := a.summarizeInStages(compactionCtx, middle)
	compactionCancel()

	if compactionCtx.Err() != nil {
		a.logger.Warn("compaction timed out, falling back to trim-by-count")
		summary = "Compaction timed out. Earlier conversation context was discarded."
	}

	// Enrich summary with structured context (tool failures, file operations).
	summary = a.buildStructuredCompactionSummary(summary, middle)

	// Add identifier preservation instruction.
	ccfg := resolvedCompactionConfig(a.cfg.Compaction)
	if instr := compactionIdentifierInstruction(ccfg.IdentifierPolicy); instr != "" {
		summary += instr
	}

	var compacted []chatMessage
	compacted = append(compacted, header...)
	compacted = append(compacted, goal)
	compacted = append(compacted, chatMessage{
		Role:    "user",
		Content: "[System: The following is a summary of earlier steps. " + summary + "]",
	})
	compacted = append(compacted, recent...)

	// Repair orphan tool use/result pairs that may have been split by compaction.
	compacted = RepairToolUseResultPairing(compacted)

	a.logger.Info("context compacted",
		"original_len", len(messages),
		"compacted_len", len(compacted),
	)

	// Persist compaction summary if persistence is available
	a.persistCompactionSummary(summary, len(messages), len(compacted))

	return compacted
}

// collectToolFailures extracts recent tool failure summaries from messages.
// Returns up to maxCount entries in format "tool_name: error_preview".
// previewChars controls how many characters of each error to include (default 240).
func (a *AgentRun) collectToolFailures(messages []chatMessage, maxCount int) []string {
	previewChars := resolvedCompactionConfig(a.cfg.Compaction).ToolFailurePreviewChars
	if previewChars <= 0 {
		previewChars = 240
	}

	// Build ToolCallID → tool name map from assistant messages.
	toolNames := make(map[string]string)
	for _, m := range messages {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				if tc.ID != "" {
					toolNames[tc.ID] = tc.Function.Name
				}
			}
		}
	}

	var failures []string
	for _, m := range messages {
		if m.Role == "tool" {
			if s, ok := m.Content.(string); ok {
				lower := strings.ToLower(s)
				if strings.Contains(lower, "error") || strings.Contains(lower, "failed") ||
					strings.Contains(lower, "not found") || strings.Contains(lower, "permission denied") {
					toolName := toolNames[m.ToolCallID]
					if toolName == "" {
						toolName = "unknown_tool"
					}
					preview := s
					if len(preview) > previewChars {
						preview = preview[:previewChars] + "..."
					}
					failures = append(failures, toolName+": "+preview)
				}
			}
		}
	}
	// Return the most recent failures.
	if len(failures) > maxCount {
		failures = failures[len(failures)-maxCount:]
	}
	return failures
}

// collectFileOperations extracts file paths from tool calls (read_file, write_file, edit_file).
func (a *AgentRun) collectFileOperations(messages []chatMessage) []string {
	seen := make(map[string]bool)
	var files []string
	for _, m := range messages {
		for _, tc := range m.ToolCalls {
			switch tc.Function.Name {
			case "read_file", "write_file", "edit_file", "create_file":
				args, err := parseToolArgs(tc.Function.Arguments)
				if err != nil {
					continue
				}
				for _, key := range []string{"path", "file_path"} {
					if path, ok := args[key]; ok {
						if p, ok := path.(string); ok && !seen[p] {
							seen[p] = true
							files = append(files, p)
						}
					}
				}
			}
		}
	}
	return files
}

// buildStructuredCompactionSummary enriches a base summary with tool failure and file operation context.
func (a *AgentRun) buildStructuredCompactionSummary(baseSummary string, messages []chatMessage) string {
	var b strings.Builder
	b.WriteString(baseSummary)

	failures := a.collectToolFailures(messages, 8)
	if len(failures) > 0 {
		b.WriteString("\n[Tool Failures] ")
		b.WriteString(strings.Join(failures, "; "))
	}

	files := a.collectFileOperations(messages)
	if len(files) > 0 {
		b.WriteString("\n[Files Touched] ")
		// Limit to 15 most recent files to avoid bloat.
		if len(files) > 15 {
			files = files[len(files)-15:]
		}
		b.WriteString(strings.Join(files, ", "))
	}

	return b.String()
}

// persistCompactionSummary saves the compaction summary to session persistence.
func (a *AgentRun) persistCompactionSummary(summary string, before, after int) {
	if a.sessionPersistence == nil || a.sessionID == "" || summary == "" {
		return
	}

	entry := CompactionEntry{
		Type:           "compaction_summary",
		Summary:        summary,
		CompactedAt:    time.Now(),
		MessagesBefore: before,
		MessagesAfter:  after,
	}

	if err := a.sessionPersistence.SaveCompaction(a.sessionID, entry); err != nil {
		a.logger.Warn("failed to persist compaction summary", "session", a.sessionID, "err", err)
	} else {
		a.logger.Debug("compaction summary persisted", "session", a.sessionID, "summary_len", len(summary), "before", before, "after", after)
	}
}

// aggressiveCompaction is used when the context still overflows despite managed compaction.
// It cuts the recent context even shorter and truncates large text heavily.
func (a *AgentRun) aggressiveCompaction(ctx context.Context, messages []chatMessage) []chatMessage {
	// First run standard truncation on tool results
	truncated := a.truncateToolResults(messages, 1500)

	// Then run managed compaction but with a much shorter keepRecent window (done inline to force it)
	var header []chatMessage
	var body []chatMessage
	for _, m := range truncated {
		if m.Role == "system" {
			header = append(header, m)
		} else {
			body = append(body, m)
		}
	}

	if len(body) < 4 {
		return truncated
	}

	goal := body[0]
	// Aggressive: keep 1/3 of the adaptive value, minimum 2.
	keepRecent := a.computeAdaptiveKeepRecent(body) / 3
	if keepRecent < 2 {
		keepRecent = 2
	}

	var summary string
	if len(body)-1 > keepRecent {
		middle := body[1 : len(body)-keepRecent]
		summary = a.summarizeInStages(ctx, middle)
	} else {
		summary = "History was aggressively truncated."
	}

	var compacted []chatMessage
	compacted = append(compacted, header...)
	compacted = append(compacted, goal)
	if summary != "" {
		compacted = append(compacted, chatMessage{
			Role:    "user",
			Content: "[System: Aggressive fallback compaction of earlier steps. " + summary + "]",
		})
	}
	compacted = append(compacted, body[len(body)-keepRecent:]...)

	// Repair orphan tool use/result pairs after aggressive compaction.
	compacted = RepairToolUseResultPairing(compacted)

	// Persist aggressive compaction summary
	a.persistCompactionSummary(summary, len(messages), len(compacted))

	return compacted
}

// emergencyCompression is the last resort when all other compaction methods fail.
// It keeps only the system prompt and the most recent user message, discarding
// everything in between. This is inspired by PicoClaw's approach.
//
// This function should only be called when:
// 1. Managed compaction failed
// 2. Aggressive compaction failed
// 3. Context still overflows
func (a *AgentRun) emergencyCompression(messages []chatMessage) []chatMessage {
	var header []chatMessage
	var lastUserMsg *chatMessage
	var lastAssistantMsg *chatMessage

	// Find system messages and the last user message
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == "system" {
			header = append([]chatMessage{m}, header...)
		} else if lastUserMsg == nil && m.Role == "user" {
			lastUserMsg = &messages[i]
		} else if lastAssistantMsg == nil && m.Role == "assistant" && lastUserMsg != nil {
			lastAssistantMsg = &messages[i]
		}
	}

	// If no user message found, just return the header
	if lastUserMsg == nil {
		a.logger.Warn("emergency compression: no user message found")
		return header
	}

	var compacted []chatMessage
	compacted = append(compacted, header...)

	// Add compression notice
	compacted = append(compacted, chatMessage{
		Role: "system",
		Content: "[System: Emergency context compression applied. " +
			"Previous conversation history was discarded to prevent context overflow. " +
			"The user's last message is preserved below. Please continue assisting.]",
	})

	// Add the last assistant message if available (provides context)
	if lastAssistantMsg != nil {
		compacted = append(compacted, *lastAssistantMsg)
	}

	// Add the last user message
	compacted = append(compacted, *lastUserMsg)

	a.logger.Warn("emergency compression applied",
		"original_len", len(messages),
		"compacted_len", len(compacted),
	)

	// Persist emergency compression summary
	a.persistCompactionSummary(
		"Emergency context compression applied. Previous conversation history was discarded.",
		len(messages), len(compacted),
	)

	return compacted
}

// computeAdaptiveKeepRecent calculates how many recent messages to keep during compaction
// based on the average message size and context window capacity.
// Small messages (<200 tokens avg) → keep up to 8; large messages (>2000 tokens avg) → keep min 3.
func (a *AgentRun) computeAdaptiveKeepRecent(body []chatMessage) int {
	if len(body) == 0 {
		return 6 // default
	}

	totalTokens := 0
	for _, m := range body {
		totalTokens += a.estimateMessageTokens(m)
	}
	avgTokens := totalTokens / len(body)

	contextWindow := a.getModelContextWindow()
	const safetyMargin = 1.2

	// Formula: max(3, min(8, contextWindow / (avgTokens * safetyMargin * 4)))
	// The factor 4 accounts for system prompt + summary + safety buffer
	computed := int(float64(contextWindow) / (float64(avgTokens) * safetyMargin * 4))
	if computed < 3 {
		computed = 3
	}
	if computed > 8 {
		computed = 8
	}

	return computed
}

// summarizeInStages implements multi-stage summarization with chunking and retry.
// For small message sets, it behaves like summarizeMiddle. For large sets,
// it chunks messages, summarizes each chunk independently, then runs a merge pass.
func (a *AgentRun) summarizeInStages(ctx context.Context, messages []chatMessage) string {
	const (
		maxSummarizeRetries         = 3
		summarizeRetryBackoffBase   = 2 * time.Second
		summarizationOverheadTokens = 4096
		maxChunkTokens              = 8000 // Max tokens per chunk for summarization
	)

	chunks := a.chunkMessagesByMaxTokens(messages, maxChunkTokens)
	if len(chunks) == 0 {
		return "No messages to summarize."
	}

	// Single chunk: summarize directly with retry.
	if len(chunks) == 1 {
		summary, err := a.summarizeChunkWithRetry(ctx, chunks[0], maxSummarizeRetries, summarizeRetryBackoffBase)
		if err != nil {
			a.logger.Warn("single-chunk summarization failed, falling back to truncation", "error", err)
			return a.fallbackTruncateSummary(messages)
		}
		return summary
	}

	// Multi-chunk: summarize each chunk, then merge.
	a.logger.Info("multi-stage summarization", "chunks", len(chunks), "total_messages", len(messages))
	partialSummaries := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		summary, err := a.summarizeChunkWithRetry(ctx, chunk, maxSummarizeRetries, summarizeRetryBackoffBase)
		if err != nil {
			a.logger.Warn("chunk summarization failed", "chunk", i, "error", err)
			summary = fmt.Sprintf("(Chunk %d: summarization failed, %d messages lost)", i+1, len(chunk))
		}
		partialSummaries = append(partialSummaries, summary)
	}

	// Merge pass: combine partial summaries into final.
	mergePrompt := []chatMessage{
		{
			Role: "system",
			Content: "You are a summarizing assistant. Combine these partial conversation summaries " +
				"into a single coherent summary. Keep it concise (max 5-6 sentences). " +
				"Preserve key facts, tool results, and current status. " +
				"NEVER use text formatting like bold or headers.",
		},
		{
			Role:    "user",
			Content: "Combine these partial summaries:\n\n" + strings.Join(partialSummaries, "\n---\n"),
		},
	}

	mergeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := a.llm.CompleteWithFallbackUsingModel(mergeCtx, "", mergePrompt, nil)
	if err != nil {
		a.logger.Warn("merge pass failed, concatenating partials", "error", err)
		return strings.Join(partialSummaries, " ")
	}

	return resp.Content
}

// chunkMessagesByMaxTokens splits messages into chunks where each chunk's
// estimated token count doesn't exceed maxTokens.
func (a *AgentRun) chunkMessagesByMaxTokens(messages []chatMessage, maxTokens int) [][]chatMessage {
	if len(messages) == 0 {
		return nil
	}

	var chunks [][]chatMessage
	var current []chatMessage
	currentTokens := 0

	for _, m := range messages {
		msgTokens := a.estimateMessageTokens(m)
		if currentTokens+msgTokens > maxTokens && len(current) > 0 {
			chunks = append(chunks, current)
			current = nil
			currentTokens = 0
		}
		current = append(current, m)
		currentTokens += msgTokens
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}

	return chunks
}

// estimateMessageTokens estimates token count for a single message.
func (a *AgentRun) estimateMessageTokens(m chatMessage) int {
	content := ""
	if s, ok := m.Content.(string); ok {
		content = s
	}
	// Rough estimate: ~4 chars per token + overhead for role/tool calls.
	tokens := len(content) / 4
	if len(m.ToolCalls) > 0 {
		tokens += len(m.ToolCalls) * 50 // ~50 tokens per tool call metadata
	}
	if tokens < 10 {
		tokens = 10
	}
	return tokens
}

// summarizeChunkWithRetry summarizes a chunk of messages with exponential backoff retry.
func (a *AgentRun) summarizeChunkWithRetry(ctx context.Context, chunk []chatMessage, maxRetries int, backoffBase time.Duration) (string, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := backoffBase * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}

		summary := a.summarizeMiddle(ctx, chunk)
		if summary != "" && !strings.HasPrefix(summary, "Failed to summarize") {
			return summary, nil
		}
		// Bail early if context was cancelled during summarization.
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		lastErr = fmt.Errorf("summarization returned empty or error result on attempt %d", attempt+1)
	}
	return "", fmt.Errorf("summarization failed after %d retries: %w", maxRetries, lastErr)
}

// fallbackTruncateSummary creates a simple truncated summary when LLM summarization fails.
func (a *AgentRun) fallbackTruncateSummary(messages []chatMessage) string {
	var b strings.Builder
	b.WriteString("Earlier conversation history (truncated): ")
	for _, m := range messages {
		if s, ok := m.Content.(string); ok {
			preview := s
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			b.WriteString(fmt.Sprintf("[%s]: %s | ", m.Role, preview))
		}
		if b.Len() > 500 {
			b.WriteString("...(further history omitted)")
			break
		}
	}
	return b.String()
}

func (a *AgentRun) summarizeMiddle(ctx context.Context, middle []chatMessage) string {
	// Build a fast summary prompt
	var textBuilder strings.Builder
	for _, m := range middle {
		role := m.Role
		content := ""
		if s, ok := m.Content.(string); ok {
			// truncate content going into summarizer to avoid inception loops
			if len(s) > 1000 {
				content = s[:1000] + "...(truncated)"
			} else {
				content = s
			}
		}

		if len(m.ToolCalls) > 0 {
			info := "Used tools: "
			for _, tc := range m.ToolCalls {
				info += tc.Function.Name + ", "
			}
			content += " " + info
		}

		textBuilder.WriteString(fmt.Sprintf("[%s]: %s\n", role, content))
	}

	prompt := []chatMessage{
		{
			Role:    "system",
			Content: "You are a summarizing assistant. Your job is to read a truncated transcript of an agent's past actions " +
				"and summarize what was attempted, what the results were, and what the current status is. " +
				"Keep your summary extremely concise (max 3-4 sentences). " +
				"Focus on CONFIRMED facts from tool results — do NOT speculate or invent outcomes. " +
				"If a tool result was ambiguous or errored, say so explicitly. " +
				"Do NOT assert that something was done successfully unless the tool result confirmed it. " +
				"NEVER use text formatting like bold or headers.",
		},
		{
			Role:    "user",
			Content: "Summarize this history:\n\n" + textBuilder.String(),
		},
	}

	// Make a quick call using the fast model (e.g., flash/haiku equivalent) if available
	// For now we use the standard model but without tools
	sumCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := a.llm.CompleteWithFallbackUsingModel(sumCtx, "", prompt, nil)
	if err != nil {
		a.logger.Warn("compaction summary failed", "error", err)
		return "Failed to summarize earlier steps due to error."
	}

	return resp.Content
}

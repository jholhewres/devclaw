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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg"
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

)

// ErrAgentYield is returned when the agent's sessions_yield tool is invoked.
// The caller should collect pending subagent results and re-invoke the agent.
var ErrAgentYield = errors.New("agent yielded turn")

// ctxKeyAgentRun is the context key for passing the AgentRun to tools.
type ctxKeyAgentRun struct{}

// ContextWithAgentRun returns a context carrying the AgentRun.
func ContextWithAgentRun(ctx context.Context, a *AgentRun) context.Context {
	return context.WithValue(ctx, ctxKeyAgentRun{}, a)
}

// AgentRunFromCtx extracts the AgentRun from context.
func AgentRunFromCtx(ctx context.Context) *AgentRun {
	if v, ok := ctx.Value(ctxKeyAgentRun{}).(*AgentRun); ok {
		return v
	}
	return nil
}

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

// VerboseLevel controls tool progress message visibility, matching
// OpenClaw's verboseLevel semantics:
//   - "off"  → no tool progress messages (default)
//   - "on"   → tool name summaries are shown
//   - "full" → tool names + detailed output are shown
type VerboseLevel string

const (
	VerboseOff  VerboseLevel = "off"
	VerboseOn   VerboseLevel = "on"
	VerboseFull VerboseLevel = "full"
)

// ShouldEmitToolProgress returns true when tool progress messages should
// be sent to the user (verbose "on" or "full").
func (v VerboseLevel) ShouldEmitToolProgress() bool {
	return v == VerboseOn || v == VerboseFull
}

// AgentConfig holds configurable agent loop parameters.
type AgentConfig struct {
	// RunTimeoutSeconds is the max seconds for the entire agent run (default: 600).
	// One timer for the whole run, not per-turn.
	RunTimeoutSeconds int `yaml:"run_timeout_seconds"`

	// LLMCallTimeoutSeconds is the safety-net timeout per individual LLM call
	// (default: 300). Only catches hung connections — not the primary timeout.
	LLMCallTimeoutSeconds int `yaml:"llm_call_timeout_seconds"`

	// MaxTurns is a soft safety limit on LLM round-trips (default: 25).
	// When > 0, the agent will request a summary after this many turns.
	// Set to 0 for unlimited (not recommended in production).
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

	// ToolVerbose controls whether tool progress messages are sent to the user.
	// Default "off" means only the final response is shown (OpenClaw-aligned).
	ToolVerbose VerboseLevel `yaml:"tool_verbose"`

	// ToolLoop configures tool loop detection thresholds.
	ToolLoop ToolLoopConfig `yaml:"tool_loop"`

	// MemoryFlush configures pre-compaction memory flush behavior.
	MemoryFlush MemoryFlushConfig `yaml:"memory_flush"`

	// Compaction configures how context compaction preserves important information.
	Compaction CompactionConfig `yaml:"compaction"`

	// KG configures Knowledge Graph extraction during compaction and dream cycles.
	KG KGAgentConfig `yaml:"kg"`
}

// KGAgentConfig mirrors the relevant KGConfig fields for use in AgentConfig.
type KGAgentConfig struct {
	AutoExtract       string `yaml:"auto_extract"`
	LLMBudgetPerCycle int    `yaml:"llm_budget_per_cycle"`
	LLMConsentACK     bool   `yaml:"llm_consent_acknowledged"`
	FactsPerInjection int    `yaml:"facts_per_injection"`
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
		MaxTurns:              25, // Safety limit — prevents runaway agent loops
		MaxContinuations:      2,
		ReflectionEnabled:     true,
		MaxCompactionAttempts: DefaultMaxCompactionAttempts,
		ToolVerbose:           VerboseOff,
		MemoryFlush: MemoryFlushConfig{
			Enabled:             true,
			ProactiveEnabled:    true,
			ProjectionThreshold: 0.85,
			ReserveTokensFloor:  20000,
			FlushThreshold:      4000,
		},
	}
}

// EscalationSignal indicates that a plugin agent wants to escalate to the main agent.
type EscalationSignal struct {
	Reason  string // Why the agent is escalating.
	Summary string // Summary of context/work done so far.
}

// ErrEscalation is returned when an agent run is terminated due to escalation.
type ErrEscalation struct {
	Signal *EscalationSignal
}

func (e *ErrEscalation) Error() string {
	return fmt.Sprintf("escalation: %s", e.Signal.Reason)
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

	// session is the active session, used for persisting compaction count.
	session *Session

	// memoryIndexer is used for post-compaction memory sync.
	memoryIndexer *MemoryIndexer

	// reactionSender sends/removes emoji reactions on the triggering message.
	// Used during compaction to show status (e.g. ✍ while compacting).
	reactionSender func(emoji string, remove bool)

	// yieldRequested is set by the sessions_yield tool to signal the agent
	// should end its current turn and return control so pending subagent
	// results can be collected and re-injected.
	// Atomic because tool execution may run in parallel goroutines.
	yieldRequested atomic.Bool

	// lcmEngine is the Lossless Compaction Module engine (nil when LCM is disabled).
	lcmEngine *LCMEngine
	// lcmConversationID is the LCM conversation tied to this agent run's session.
	lcmConversationID string
	// lcmIngestedSeq tracks the last ingested message index to avoid double-ingest.
	lcmIngestedSeq int
	// lcmRunCtx holds the run-level context for LCM operations that need
	// cancellation support (e.g. Assemble in buildMessages).
	lcmRunCtx context.Context

	// escalationChecker is called after each LLM response to check if the
	// plugin agent should escalate to the main agent. Set by plugin agent spawner.
	escalationChecker func(turn int, lastResponse string) *EscalationSignal

	// compactionPipeline manages multi-level proactive context compaction.
	compactionPipeline *CompactionPipeline

	// loopDetector tracks tool call history and detects repetitive patterns.
	loopDetector *ToolLoopDetector

	// usageAcc tracks token usage across LLM calls with last-call snapshots.
	usageAcc UsageAccumulator

	// lastRunToolSummary accumulates tool names called during this run.
	lastRunToolSummary string

	// collectedToolCalls stores individual tool invocations for session history.
	// Populated during the agent loop and retrieved via CollectedToolCalls().
	collectedToolCalls []ToolCallRecord

	// assistantFragments collects assistant response content from each turn.
	// Used by subagent timeout handling to synthesize partial progress.
	assistantFragments []string

	logger *slog.Logger

	// kg is the Knowledge Graph for context-residual extraction during compaction.
	// Nil when KG is disabled. Set via SetKG().
	kg *kg.KG
}

// SetKG sets the Knowledge Graph reference for context-residual extraction.
func (a *AgentRun) SetKG(k *kg.KG) {
	a.kg = k
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

// CollectedToolCalls returns the individual tool call records collected during the run.
func (a *AgentRun) CollectedToolCalls() []ToolCallRecord {
	return a.collectedToolCalls
}

// CollectedAssistantFragments returns assistant response fragments collected
// during the agent run, for partial progress synthesis on timeout.
func (a *AgentRun) CollectedAssistantFragments() []string {
	return a.assistantFragments
}

// SynthesizeProgressSummary builds a concise summary from collected assistant
// fragments. Used by subagent timeout handling to report partial progress
// instead of a generic timeout message.
func SynthesizeProgressSummary(fragments []string) string {
	if len(fragments) == 0 {
		return ""
	}

	const maxChars = 4000
	var b strings.Builder
	b.WriteString("Partial progress before timeout:\n\n")

	for i, f := range fragments {
		if b.Len() > maxChars {
			b.WriteString("\n...(additional fragments truncated)")
			break
		}
		// Truncate individual fragments to keep summary manageable.
		if len(f) > 500 {
			f = f[:500] + "..."
		}
		fmt.Fprintf(&b, "[Turn %d] %s\n", i+1, f)
	}

	result := b.String()
	if len(result) > maxChars {
		result = result[:maxChars] + "\n...(truncated)"
	}
	return result
}

// NewAgentRun creates a new agent runner.
func NewAgentRun(llm *LLMClient, executor *ToolExecutor, logger *slog.Logger) *AgentRun {
	return &AgentRun{
		llm:                   llm,
		executor:              executor,
		runTimeout:            DefaultRunTimeout,
		llmCallTimeout:        DefaultLLMCallTimeout,
		maxTurns:              25, // Safety limit, matches DefaultAgentConfig
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
	if cfg.MaxTurns > 0 {
		ar.maxTurns = cfg.MaxTurns
	} else if cfg.MaxTurns < 0 {
		ar.maxTurns = 0 // Explicit -1 means unlimited
	}
	ar.maxContinuations = cfg.MaxContinuations
	ar.reflectionOn = cfg.ReflectionEnabled
	if cfg.MaxCompactionAttempts > 0 {
		ar.maxCompactionAttempts = cfg.MaxCompactionAttempts
	}
	ar.cfg = cfg
	ar.compactionPipeline = NewCompactionPipeline(DefaultCompactThresholds())
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

// SetSession sets the active session for compaction count tracking.
func (a *AgentRun) SetSession(s *Session) {
	a.session = s
}

// SetMemoryIndexer sets the memory indexer for post-compaction sync.
func (a *AgentRun) SetMemoryIndexer(m *MemoryIndexer) {
	a.memoryIndexer = m
}

// SetReactionSender sets the function used to send/remove emoji reactions.
func (a *AgentRun) SetReactionSender(fn func(emoji string, remove bool)) {
	a.reactionSender = fn
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

	// Inject AgentRun into context so tools (e.g. sessions_yield) can access it.
	runCtx = ContextWithAgentRun(runCtx, a)

	runStart := time.Now()

	// Store run context for LCM operations that need cancellation support.
	a.lcmRunCtx = runCtx

	// Build initial messages from history.
	messages := a.buildMessages(systemPrompt, history, userMessage)

	// LCM: ingest the new user message and mark the assembled context as already
	// ingested. This prevents double-ingestion of summaries and tail messages that
	// the assembler pulled from the store (they're already persisted there).
	if a.lcmEngine != nil && a.lcmConversationID != "" && userMessage != "" {
		content := userMessage
		tokenCount := EstimateTokens(content)
		if _, err := a.lcmEngine.Store().IngestMessage(a.lcmConversationID, "user", content, tokenCount); err != nil {
			a.logger.Warn("lcm: failed to ingest initial user message", "err", err)
		}
		// Mark everything in the assembled context as already ingested so
		// managedCompaction only processes messages added during the loop.
		a.lcmIngestedSeq = len(messages)
	}

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
	// 400 chars preserves enough detail for the LLM to understand tool behavior
	// while capping excessively long descriptions from plugins/skills.
	// With ~25 visible tools (down from ~90), we can afford slightly longer descriptions.
	const maxDescLen = 400
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

	// Consecutive blocked turns: if ALL tool calls are blocked for N turns
	// in a row, the LLM is stuck calling tools it's been told to stop using.
	// Terminate the run early to avoid wasting tokens.
	const maxConsecutiveBlockedTurns = 3
	var consecutiveBlockedTurns int
	var thinkingFallbackApplied bool
	var thinkRetries int

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

		// ── Proactive context compaction (multi-level pipeline) ──
		// Evaluate context pressure and apply the appropriate compaction level.
		// Cheaper levels (collapse, micro-compact) run first to avoid expensive
		// LLM summarization calls unless truly necessary.
		if totalTurns > 3 && len(messages) > 8 && a.compactionPipeline != nil {
			estimatedTokens := a.estimateTokens(messages)
			pressure := a.compactionPipeline.Evaluate(estimatedTokens, ctxTokens)

			if pressure.RecommendedLevel > CompactNone && a.compactionPipeline.ShouldCompact(pressure.RecommendedLevel) {
				a.logger.Info("proactive compaction triggered",
					"level", pressure.RecommendedLevel.String(),
					"ratio", fmt.Sprintf("%.1f%%", pressure.Ratio*100),
					"tokens", estimatedTokens,
					"window", ctxTokens,
					"tokens_until_blocking", pressure.TokensUntilBlocking,
				)

				switch pressure.RecommendedLevel {
				case CompactCollapse:
					n := CollapseToolResults(messages, 4000)
					if n > 0 {
						a.logger.Info("collapse compaction applied", "collapsed", n)
					}

				case CompactMicro:
					// First collapse, then micro-compact for maximum effect.
					CollapseToolResults(messages, 4000)
					var n int
					messages, n = MicroCompact(messages, 10)
					if n > 0 {
						a.logger.Info("micro-compact applied", "cleared", n)
					}

				case CompactAuto:
					// Apply cheap levels first, then LLM summarization.
					CollapseToolResults(messages, 3000)
					messages, _ = MicroCompact(messages, 10)
					before := len(messages)
					messages = a.managedCompaction(runCtx, messages)
					if len(messages) < before {
						a.compactionPipeline.RecordSuccess()
					} else {
						a.compactionPipeline.RecordFailure()
					}

				case CompactMemory:
					// Extract memories before compacting.
					CollapseToolResults(messages, 2000)
					messages, _ = MicroCompact(messages, 6)
					a.maybeMemoryFlush(runCtx, messages, estimatedTokens)
					before := len(messages)
					messages = a.aggressiveCompaction(runCtx, messages)
					if len(messages) < before {
						a.compactionPipeline.RecordSuccess()
					} else {
						a.compactionPipeline.RecordFailure()
					}
				}

				a.compactionPipeline.SetLastLevel(pressure.RecommendedLevel)
			}
		}

		// Inject reflection nudge periodically so the agent is aware of duration.
		// More aggressive messaging to catch stuck patterns early.
		if a.reflectionOn && totalTurns > 1 && totalTurns%reflectionInterval == 0 {
			elapsed := time.Since(runStart).Seconds()
			remaining := a.runTimeout.Seconds() - elapsed
			maxT := a.maxTurns
			if maxT <= 0 {
				maxT = 25 // default for display
			}
			messages = append(messages, chatMessage{
				Role: "user",
				Content: fmt.Sprintf(
					"[System: Turn %d/%d. If you have enough information to respond, do so now. "+
						"Do not continue searching unless the user's question is still unanswered. "+
						"(%.0fs elapsed, ~%.0fs remaining)]",
					totalTurns, maxT, elapsed, remaining,
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

			// Thinking level fallback: if the model doesn't support extended
			// thinking, strip thinking instructions from system prompt and retry.
			// Aligned with OpenClaw's pickFallbackThinkingLevel pattern.
			if isThinkingError(err) && !thinkingFallbackApplied {
				a.logger.Warn("thinking level unsupported, stripping thinking instructions and retrying")
				messages = stripThinkingInstructions(messages)
				thinkingFallbackApplied = true

				llmStart = time.Now()
				resp, err = a.doLLMCallWithOverflowRetry(runCtx, messages, tools)
				llmDuration = time.Since(llmStart)
			}

			// Timeout or transient error on a later turn: try compacting
			// the context and retrying once before giving up.
			if err != nil {
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
			thinkRetries++
			if thinkRetries > 3 {
				// Give up retrying: strip the unclosed tag and proceed normally.
				a.logger.Warn("llm repeatedly missed closing </think> tag, stripping and proceeding",
					"retries", thinkRetries)
				resp.Content = strings.Replace(resp.Content, "<think>", "", 1)
			} else {
				a.logger.Warn("llm missed closing </think> tag, prompting retry without executing tools",
					"retry", thinkRetries)

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
			// Check for context overflow returned as finish_reason (safety net).
			// Some providers (Z.AI Anthropic proxy) may return this as a
			// stop_reason in a 200 OK instead of an HTTP error. The LLM client
			// converts it to an error, but if it somehow slips through, catch it here.
			if resp.Content == "" && IsLikelyContextOverflowError(resp.FinishReason) {
				a.logger.Warn("context overflow detected via finish_reason, compacting",
					"finish_reason", resp.FinishReason,
					"turn", totalTurns,
				)
				messages = a.aggressiveCompaction(runCtx, messages)
				continue
			}

			// Check for truncated response (finish_reason="length").
			// If the response was cut mid-generation by context limit, compact
			// and retry instead of returning a truncated answer.
			if resp.FinishReason == "length" && len(messages) > 4 {
				a.logger.Warn("response truncated (finish_reason=length), compacting and retrying",
					"turn", totalTurns,
					"response_len", len(resp.Content),
				)
				messages = a.aggressiveCompaction(runCtx, messages)
				continue
			}

			a.logger.Info("agent completed",
				"total_turns", totalTurns,
				"response_len", len(resp.Content),
				"finish_reason", resp.FinishReason,
				"run_elapsed_ms", time.Since(runStart).Milliseconds(),
			)

			// LCM: ingest the final assistant response so it's available
			// for future sessions' context assembly.
			if a.lcmEngine != nil && a.lcmConversationID != "" && resp.Content != "" {
				tokenCount := EstimateTokens(resp.Content)
				if _, err := a.lcmEngine.Store().IngestMessage(a.lcmConversationID, "assistant", resp.Content, tokenCount); err != nil {
					a.logger.Warn("lcm: failed to ingest final response", "err", err)
				}
			}

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

		// Collect assistant fragment for partial progress synthesis.
		if resp.Content != "" {
			a.assistantFragments = append(a.assistantFragments, resp.Content)
		}

		// ── Tool Loop Detection ──
		// Record tool calls and check for repetitive patterns before execution.
		// LoopBreaker → hard stop the entire agent run.
		// LoopCritical → block that specific tool call (skip execution, return
		//   a synthetic error result so the LLM sees the block reason).
		// LoopWarning → inject a nudge message after tool results.
		var loopWarning string
		blockedCalls := make(map[string]string) // tool call ID → block reason
		if a.loopDetector != nil {
			for _, tc := range resp.ToolCalls {
				args, _ := parseToolArgs(tc.Function.Arguments)
				result := a.loopDetector.RecordAndCheck(tc.Function.Name, args)

				switch result.Severity {
				case LoopBreaker:
					a.logger.Error("tool loop circuit breaker",
						"tool", tc.Function.Name, "streak", result.Streak, "pattern", result.Pattern)
					return result.Message, &totalUsage, nil

				case LoopCritical:
					// Block this specific tool call — do NOT execute it.
					// The LLM will receive an error result forcing it to change strategy.
					blockedCalls[tc.ID] = result.Message
					loopWarning = result.Message
					a.logger.Warn("tool call blocked by loop detector",
						"tool", tc.Function.Name, "id", tc.ID, "streak", result.Streak, "pattern", result.Pattern)

				case LoopWarning:
					loopWarning = result.Message
				}
			}
		}

		// Filter out blocked tool calls — only execute non-blocked ones.
		execCalls := resp.ToolCalls
		var blockedResults []ToolResult
		if len(blockedCalls) > 0 {
			execCalls = make([]ToolCall, 0, len(resp.ToolCalls))
			for _, tc := range resp.ToolCalls {
				if reason, blocked := blockedCalls[tc.ID]; blocked {
					blockedResults = append(blockedResults, ToolResult{
						ToolCallID: tc.ID,
						Name:       tc.Function.Name,
						Content:    "ERROR: " + reason,
						Error:      fmt.Errorf("blocked by loop detector"),
					})
				} else {
					execCalls = append(execCalls, tc)
				}
			}
		}

		// Execute non-blocked tool calls.
		toolStart := time.Now()
		toolNames := make([]string, len(execCalls))
		for i, tc := range execCalls {
			toolNames[i] = tc.Function.Name
		}
		a.logger.Info("executing tool calls",
			"count", len(execCalls),
			"blocked", len(blockedCalls),
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
		// Gated by ToolVerbose: when "off" (default, OpenClaw-aligned), no
		// tool progress messages are sent — only the final response.
		// ProgressSender stays in the context for tools that need it internally
		// (e.g. claude-code sending progress during long operations).
		if ps := ProgressSenderFromContext(runCtx); ps != nil && a.cfg.ToolVerbose.ShouldEmitToolProgress() {
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

		// Execute only non-blocked calls; prepend blocked synthetic results.
		var results []ToolResult
		if len(execCalls) > 0 {
			results = a.executor.Execute(runCtx, execCalls)
		}
		if len(blockedResults) > 0 {
			results = append(blockedResults, results...)
		}

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
		// Use tool mutation classification for error severity:
		//   - Mutating tool errors → Warn (user should know)
		//   - Read-only tool errors → Debug (expected, model retries)
		for _, result := range results {
			content := result.Content
			if result.Error != nil {
				// Find the matching tool call to check its args for mutation classification.
				var toolArgs map[string]any
				for _, tc := range resp.ToolCalls {
					if tc.ID == result.ToolCallID {
						toolArgs, _ = parseToolArgs(tc.Function.Arguments)
						break
					}
				}
				isMutating := IsMutatingToolCall(result.Name, toolArgs)
				recoverable := isRecoverableToolError(content)

				if isMutating && !recoverable {
					a.logger.Warn("mutating tool error",
						"tool", result.Name,
						"error_preview", truncateStr(content, 120),
					)
				} else {
					a.logger.Debug("tool error (model should retry)",
						"tool", result.Name,
						"mutating", isMutating,
						"recoverable", recoverable,
						"error_preview", truncateStr(content, 80),
					)
				}
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

		// Inject anti-hallucination reminder when tools returned errors.
		// Placed after tool results (valid message order: assistant→tool→user).
		{
			var errorTools []string
			for _, result := range results {
				if result.Error != nil {
					errorTools = append(errorTools, result.Name)
				}
			}
			if len(errorTools) > 0 {
				toolList := strings.Join(errorTools, ", ")
				messages = append(messages, chatMessage{
					Role: "user",
					Content: fmt.Sprintf(
						"[System] Tools returned errors: %s. "+
							"Report the exact error to the user. "+
							"Do NOT fabricate results, file names, IDs, or URLs that were not in the tool output.",
						toolList,
					),
				})
			}
		}

		// Accumulate tool names for provenance tracking + collect ToolCallRecords.
		if len(resp.ToolCalls) > 0 {
			names := make([]string, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				names[i] = tc.Function.Name
			}
			a.lastRunToolSummary += strings.Join(names, ",") + "; "

			// Build ToolCallRecords for session history fidelity.
			resultMap := make(map[string]string, len(results))
			for _, r := range results {
				resultMap[r.ToolCallID] = r.Content
			}
			for _, tc := range resp.ToolCalls {
				rec := ToolCallRecord{
					ID:   tc.ID,
					Name: tc.Function.Name,
					Args: truncateStr(tc.Function.Arguments, 200),
				}
				if res, ok := resultMap[tc.ID]; ok {
					rec.Result = truncateStr(res, 500)
				}
				a.collectedToolCalls = append(a.collectedToolCalls, rec)
			}
		}

		// ── Preemptive context overflow guard ──
		// After each tool execution round, estimate total tokens. If above
		// 85% of the context window, truncate old tool results in-place to
		// prevent the next LLM call from hitting context_length_exceeded.
		// If still over budget after tool result pruning, escalate to
		// managed compaction of the full conversation history.
		{
			ctxWindow := ResolveContextWindowTokens(a.cfg.ContextTokens, a.modelOverride)
			estimated := a.estimateTokens(messages)
			highWater := int(float64(ctxWindow) * 0.85)
			if estimated > highWater {
				a.logger.Info("preemptive context guard: truncating old tool results",
					"estimated_tokens", estimated, "high_water", highWater)
				messages = GuardToolResultContext(messages, ctxWindow)
				pcfg := resolvedCompactionConfig(a.cfg.Compaction).ContextPruning
				messages = pruneByContextRatio(messages, a.estimateTokens(messages), ctxWindow, pcfg)

				// If tool result pruning wasn't enough, escalate to managed compaction.
				afterPrune := a.estimateTokens(messages)
				if afterPrune > ctxWindow {
					a.logger.Warn("preemptive context guard: still over budget after tool pruning, compacting messages",
						"estimated_tokens", afterPrune, "context_window", ctxWindow)
					messages = a.managedCompaction(runCtx, messages)
				}
			}
		}

		// ── LCM incremental ingestion ──
		// Ingest new messages (assistant response + tool results) that were
		// appended during this turn. This ensures the LCM store stays current
		// even when compaction doesn't trigger.
		if a.lcmEngine != nil && a.lcmConversationID != "" {
			if err := a.lcmIngestNew(runCtx, messages); err != nil {
				a.logger.Warn("lcm incremental ingest failed", "turn", totalTurns, "err", err)
			}
		}

		// ── Escalation check (plugin agents) ──
		// If an escalation checker is set, evaluate after each turn.
		if a.escalationChecker != nil {
			if signal := a.escalationChecker(totalTurns, resp.Content); signal != nil {
				a.logger.Info("agent escalation triggered",
					"turn", totalTurns,
					"reason", signal.Reason,
				)
				return resp.Content, &totalUsage, &ErrEscalation{Signal: signal}
			}
		}

		// ── Yield check ──
		// If sessions_yield was called during tool execution, exit the loop.
		// The caller (assistant) will collect pending subagent results and
		// re-invoke the agent with those results injected.
		if a.yieldRequested.Load() {
			a.logger.Info("agent yielding turn", "total_turns", totalTurns)
			return resp.Content, &totalUsage, ErrAgentYield
		}

		// Track consecutive turns where ALL tool calls were blocked.
		// If the LLM keeps calling blocked tools, terminate early to avoid
		// wasting tokens on an LLM that ignores loop detection errors.
		if len(blockedResults) > 0 && len(execCalls) == 0 {
			consecutiveBlockedTurns++
			if consecutiveBlockedTurns >= maxConsecutiveBlockedTurns {
				a.logger.Warn("agent terminated: all tool calls blocked for consecutive turns",
					"consecutive_blocked", consecutiveBlockedTurns,
					"total_turns", totalTurns,
				)
				// Force a final LLM call without tools to get a text response.
				messages = append(messages, chatMessage{
					Role: "user",
					Content: "[System: All your recent tool calls have been BLOCKED by the loop detector. " +
						"You MUST stop using tools and provide your best response using only what you know. " +
						"Do NOT attempt any more tool calls.]",
				})
				resp, err := a.doLLMCallWithOverflowRetry(runCtx, messages, nil)
				if err != nil {
					return "I was unable to complete this task — I got stuck in a loop calling the same tools repeatedly.", &totalUsage, nil
				}
				a.accumulateUsage(&totalUsage, resp)
				return resp.Content, &totalUsage, nil
			}
		} else {
			consecutiveBlockedTurns = 0
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
			return "💻 Running command..."
		}
		if len(cmd) > 60 {
			cmd = cmd[:60] + "..."
		}
		return "💻 `" + cmd + "`"

	// ── File operations ──
	case "read_file":
		p, _ := args["path"].(string)
		if p != "" {
			return "📖 Reading " + shortPath(p)
		}
		return "📖 Reading file..."

	case "write_file":
		p, _ := args["path"].(string)
		if p != "" {
			return "✍️ Writing " + shortPath(p)
		}
		return "✍️ Writing file..."

	case "edit_file":
		p, _ := args["path"].(string)
		if p != "" {
			return "✏️ Editing " + shortPath(p)
		}
		return "✏️ Editing file..."

	case "list_files", "glob_files":
		p, _ := args["path"].(string)
		if p == "" {
			p, _ = args["pattern"].(string)
		}
		if p != "" {
			return "📂 Listing " + shortPath(p)
		}
		return "📂 Listing files..."

	case "search_files":
		q, _ := args["query"].(string)
		if q == "" {
			q, _ = args["pattern"].(string)
		}
		if q != "" {
			return "🔎 Searching: " + q
		}
		return "🔎 Searching files..."

	// ── Web ──
	case "web_search", "brave-search_execute", "brave-search_run_search":
		q, _ := args["query"].(string)
		if q != "" {
			if len(q) > 60 {
				q = q[:60] + "..."
			}
			return "🔍 Searching: " + q
		}
		return "🔍 Searching the web..."

	case "web_fetch", "web-fetch_fetch_url":
		u, _ := args["url"].(string)
		if u != "" {
			if len(u) > 55 {
				u = u[:55] + "..."
			}
			return "🌐 Fetching " + u
		}
		return "🌐 Fetching page..."

	// ── Memory ──
	case "memory_save":
		return "💾 Saving to memory..."
	case "memory_search":
		q, _ := args["query"].(string)
		if q != "" {
			return "🧠 Recalling: " + q
		}
		return "🧠 Searching memory..."
	case "memory_list", "memory_index":
		return "🧠 Organizing memories..."
	case "memory": // legacy alias
		action, _ := args["action"].(string)
		switch action {
		case "save":
			return "💾 Saving to memory..."
		case "search":
			return "🧠 Searching memory..."
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
			return "🔗 Connecting to " + host + "..."
		}
		return "🔗 Connecting via SSH..."

	case "scp":
		src, _ := args["source"].(string)
		dst, _ := args["destination"].(string)
		if src != "" && dst != "" {
			return "📤 Transferring " + shortPath(src) + " → " + shortPath(dst)
		}
		return "📤 Transferring file..."

	// ── Coding ──
	case "claude-code_execute":
		p, _ := args["prompt"].(string)
		if p != "" {
			if len(p) > 55 {
				p = p[:55] + "..."
			}
			return "🤖 Coding: " + p
		}
		return "🤖 Running Claude Code..."
	case "claude-code_check":
		return "🤖 Checking Claude Code..."

	// ── Images ──
	case "describe_image":
		return "👁️ Analyzing image..."
	case "image-gen_generate_image":
		p, _ := args["prompt"].(string)
		if p != "" {
			if len(p) > 50 {
				p = p[:50] + "..."
			}
			return "🎨 Generating image: " + p
		}
		return "🎨 Generating image..."

	// ── Audio ──
	case "transcribe_audio":
		return "🎤 Transcribing audio..."

	// ── Scheduler ──
	case "scheduler_add":
		return "⏰ Creating schedule..."
	case "scheduler_list":
		return "⏰ Listing schedules..."
	case "scheduler_remove":
		return "⏰ Removing schedule..."
	case "scheduler_search":
		return "⏰ Searching schedules..."
	case "scheduler": // legacy alias
		return "⏰ Managing schedules..."

	// ── Vault ──
	case "vault_status":
		return "🔐 Checking vault..."
	case "vault_save":
		return "🔐 Saving to vault..."
	case "vault_get":
		return "🔐 Reading from vault..."
	case "vault_list":
		return "🔐 Listing vault..."
	case "vault_delete":
		return "🔐 Removing from vault..."
	case "vault": // legacy alias
		return "🔐 Managing vault..."

	// ── Skills ──
	case "skill_install":
		s, _ := args["name"].(string)
		if s != "" {
			return "📦 Installing skill: " + s
		}
		return "📦 Installing skill..."
	case "skill_list", "skill_defaults_list":
		return "📋 Listing skills..."
	case "skill_init":
		return "📦 Creating skill..."
	case "skill_edit", "skill_add_script":
		return "📦 Editing skill..."
	case "skill_test":
		return "📦 Testing skill..."
	case "skill_remove":
		return "📦 Removing skill..."
	case "skill_defaults_install":
		return "📦 Installing default skills..."
	case "skill_manage": // legacy alias
		return "📦 Managing skills..."

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
			return "🧵 Starting subagent: " + label
		}
		return "🧵 Starting subagent..."
	case "list_subagents":
		return "🧵 Checking subagents..."
	case "wait_subagent":
		return "⏳ Waiting for subagent..."
	case "stop_subagent":
		return "🛑 Stopping subagent..."

	// ── Project Manager ──
	case "project-manager_activate":
		p, _ := args["name"].(string)
		if p != "" {
			return "📁 Activating project: " + p
		}
		return "📁 Activating project..."
	case "project-manager_list":
		return "📁 Listing projects..."
	case "project-manager_scan", "project-manager_tree":
		return "📁 Scanning project..."
	case "project-manager_register":
		return "📁 Registering project..."

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
			return "⚡ Running " + skillName + "..."
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

// isRecoverableToolError delegates to the exported IsRecoverableToolError
// from tool_mutation.go (single source of truth for error classification).
func isRecoverableToolError(errMsg string) bool {
	return IsRecoverableToolError(errMsg)
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
	// LCM path: assemble context from DAG summaries + fresh tail.
	if a.lcmEngine != nil && a.lcmConversationID != "" {
		tokenBudget := a.getModelContextWindow()
		assembleCtx := a.lcmRunCtx
		if assembleCtx == nil {
			assembleCtx = context.Background()
		}
		msgs, err := a.lcmEngine.Assemble(assembleCtx, a.lcmConversationID, systemPrompt, userMessage, tokenBudget)
		if err != nil {
			a.logger.Warn("lcm assembly failed, using legacy path", "err", err)
		} else {
			return msgs
		}
	}

	// Legacy path.
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
			Content: scrubAnthropicRefusalMagic(entry.UserMessage),
		})
		if entry.AssistantResponse != "" {
			content := entry.AssistantResponse

			// When ToolCalls are available, reconstruct them as readable context
			// so the LLM knows what was actually executed in previous turns.
			if len(entry.ToolCalls) > 0 {
				var tcSummary strings.Builder
				tcSummary.WriteString("[Previous tool calls in this turn:]\n")
				for _, tc := range entry.ToolCalls {
					result := tc.Result
					if result == "" {
						result = "(no output)"
					}
					tcSummary.WriteString(fmt.Sprintf("- %s → %q\n", tc.Name, result))
				}
				tcSummary.WriteString("\n")
				content = tcSummary.String() + content
			} else if entry.ToolSummary != "" {
				// Fallback: use ToolSummary/tool_provenance for old entries without ToolCalls.
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
		Content: scrubAnthropicRefusalMagic(userMessage),
	})

	return messages
}

// anthropicRefusalMagicString is the exact token that Anthropic uses in
// internal refusal testing. If this string appears verbatim in user input
// or session transcripts, it can trigger unexpected model refusals.
// Aligned with OpenClaw's scrubAnthropicRefusalMagic.
const anthropicRefusalMagicString = "ANTHROPIC_MAGIC_STRING_TRIGGER_REFUSAL"

// scrubAnthropicRefusalMagic replaces the Anthropic refusal test token with
// a harmless redacted version, preventing it from poisoning sessions.
func scrubAnthropicRefusalMagic(s string) string {
	if !strings.Contains(s, anthropicRefusalMagicString) {
		return s
	}
	return strings.ReplaceAll(s, anthropicRefusalMagicString, "ANTHROPIC MAGIC STRING TRIGGER REFUSAL (redacted)")
}

// isThinkingError checks if an error indicates the model does not support
// the requested thinking/reasoning level. Used to trigger graceful fallback.
func isThinkingError(err error) bool {
	if err == nil {
		return false
	}
	var apierr *apiError
	if !errors.As(err, &apierr) {
		return false
	}
	return classifyAPIError(apierr.statusCode, apierr.body) == LLMErrorThinking
}

// stripThinkingInstructions removes thinking-related instructions from the
// system prompt in messages. This is the fallback when a model does not
// support extended thinking — the agent retries without thinking instructions.
func stripThinkingInstructions(messages []chatMessage) []chatMessage {
	result := make([]chatMessage, len(messages))
	copy(result, messages)

	for i, m := range result {
		if m.Role != "system" {
			continue
		}
		s, ok := m.Content.(string)
		if !ok {
			continue
		}
		// Remove the ## Thinking Mode section from the system prompt.
		if idx := strings.Index(s, "## Thinking Mode"); idx >= 0 {
			// Find the next section heading or end of string.
			rest := s[idx:]
			endIdx := len(rest)
			// Look for the next ## heading after the first line.
			if nextSection := strings.Index(rest[1:], "\n## "); nextSection >= 0 {
				endIdx = nextSection + 1
			}
			result[i].Content = s[:idx] + s[idx+endIdx:]
		}
	}
	return result
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

// deduplicateToolCallIDs removes duplicate tool_call_id references from messages.
// Some providers (OpenAI-compatible) reject requests with duplicate IDs. Duplicates
// can arise after compaction reassembly or if the LLM reuses IDs. The last occurrence
// of each ID wins (both for assistant tool calls and tool result messages).
func deduplicateToolCallIDs(messages []chatMessage) []chatMessage {
	// Collect all tool call IDs and their last occurrence index.
	type idEntry struct {
		lastAssistantIdx int
		lastToolIdx      int
	}
	seen := make(map[string]*idEntry)

	for i, m := range messages {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				if tc.ID == "" {
					continue
				}
				if _, ok := seen[tc.ID]; !ok {
					seen[tc.ID] = &idEntry{lastAssistantIdx: -1, lastToolIdx: -1}
				}
				seen[tc.ID].lastAssistantIdx = i
			}
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			if _, ok := seen[m.ToolCallID]; !ok {
				seen[m.ToolCallID] = &idEntry{lastAssistantIdx: -1, lastToolIdx: -1}
			}
			seen[m.ToolCallID].lastToolIdx = i
		}
	}

	// Check if any ID appears more than once.
	hasDuplicates := false
	idCount := make(map[string]int)
	for _, m := range messages {
		if m.Role == "assistant" {
			for _, tc := range m.ToolCalls {
				if tc.ID != "" {
					idCount[tc.ID]++
					if idCount[tc.ID] > 1 {
						hasDuplicates = true
					}
				}
			}
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			idCount[m.ToolCallID]++
			if idCount[m.ToolCallID] > 1 {
				hasDuplicates = true
			}
		}
	}

	if !hasDuplicates {
		return messages
	}

	// Remove earlier occurrences, keeping only the last.
	idSeen := make(map[string]bool)
	var result []chatMessage

	// Walk backwards to keep only last occurrences.
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]

		if m.Role == "tool" && m.ToolCallID != "" {
			key := "tool:" + m.ToolCallID
			if idSeen[key] {
				continue // Skip earlier duplicate.
			}
			idSeen[key] = true
		}

		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			// Filter tool calls to remove duplicates.
			var keptCalls []ToolCall
			for _, tc := range m.ToolCalls {
				key := "call:" + tc.ID
				if tc.ID != "" && idSeen[key] {
					continue
				}
				if tc.ID != "" {
					idSeen[key] = true
				}
				keptCalls = append(keptCalls, tc)
			}
			if len(keptCalls) == 0 && len(m.ToolCalls) > 0 {
				// All tool calls were duplicates — skip this message entirely
				// only if it has no content.
				if s, ok := m.Content.(string); ok && s == "" {
					continue
				}
			}
			m.ToolCalls = keptCalls
		}

		result = append(result, m)
	}

	// Reverse to restore chronological order.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
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
		softTrimAge   = 3   // Turns before soft trim (truncate to 500 chars)
		hardTrimAge   = 6   // Turns before hard trim (remove entirely)
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
//  0. Pre-flight: if estimated tokens (messages + tool defs) exceed 90% of context, thin tool definitions.
//  1. On overflow: thin tool definitions (strip descriptions, then param descriptions).
//  2. Truncate oversized tool results (>4K chars).
//  3. Managed compaction: compact messages (keep last N) + truncate tool results harder.
//  4. Aggressive compaction (keep fewer messages).
//  5. Emergency compression (keep only system + last user message).
func (a *AgentRun) doLLMCallWithOverflowRetry(ctx context.Context, messages []chatMessage, tools []ToolDefinition) (*LLMResponse, error) {
	toolResultTruncated := false
	toolDefThinLevel := -1 // -1 = not thinned, 0 = descriptions stripped, 1 = param descriptions stripped

	// Deduplicate tool call IDs: some providers (OpenAI-compatible) reject
	// requests with duplicate tool_call_id values. This can happen after
	// compaction reassembles messages or if the LLM reuses IDs.
	messages = deduplicateToolCallIDs(messages)

	// Pre-LLM context guard: cap oversized tool results and compact oldest
	// when total tool content exceeds the context budget.
	ctxTokens := ResolveContextWindowTokens(a.cfg.ContextTokens, a.modelOverride)
	messages = GuardToolResultContext(messages, ctxTokens)

	// Ratio-based context pruning (soft trim / hard clear tool results).
	pcfg := resolvedCompactionConfig(a.cfg.Compaction).ContextPruning
	messages = pruneByContextRatio(messages, a.estimateTokens(messages), ctxTokens, pcfg)

	// Prune old images to prevent token accumulation from multimodal content.
	messages = pruneOldImages(messages, DefaultImagePruneAfterTurns)

	// Pre-flight: estimate total tokens including tool definitions.
	// If the base payload (system prompt + tools + messages) is already near
	// the context window, proactively thin tool definitions before the first
	// LLM call. Message compaction won't help when tools dominate the budget.
	toolDefTokens := estimateToolDefTokens(tools, a.modelOverride)
	msgTokens := a.estimateTokens(messages)
	if toolDefTokens+msgTokens > int(float64(ctxTokens)*0.90) {
		a.logger.Info("pre-flight: estimated tokens near context limit, thinning tool definitions",
			"tool_def_tokens", toolDefTokens,
			"msg_tokens", msgTokens,
			"total_estimated", toolDefTokens+msgTokens,
			"context_window", ctxTokens,
		)
		tools = thinToolDefinitions(tools, 0)
		toolDefThinLevel = 0
		newToolDefTokens := estimateToolDefTokens(tools, a.modelOverride)
		if newToolDefTokens+msgTokens > int(float64(ctxTokens)*0.90) {
			tools = thinToolDefinitions(tools, 1)
			toolDefThinLevel = 1
		}
	}

	prevTokenEstimate := a.estimateTokens(messages)

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
			"tool_def_thin_level", toolDefThinLevel,
		)

		// ── Compaction strategy ──

		// Step 0: Thin tool definitions (cheap, no LLM call).
		// Tool definitions can consume 30-50% of the context window with 80+ tools.
		// Thinning is applied alongside message compaction in the same attempt so
		// it doesn't consume the limited compaction budget on its own.
		if toolDefThinLevel < 1 {
			toolDefThinLevel++
			tools = thinToolDefinitions(tools, toolDefThinLevel)
			a.logger.Info("thinned tool definitions to reduce context",
				"level", toolDefThinLevel,
				"tools_count", len(tools),
			)
		}

		// Step 1: Try truncating oversized tool results first (cheap operation).
		if !toolResultTruncated {
			if hasOversizedToolResults(messages, 2000) {
				a.logger.Info("truncating oversized tool results before compaction")
				messages = a.truncateToolResults(messages, 2000)
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
			messages = a.emergencyCompression(ctx, messages)
		}

		// Progress check: if compaction didn't reduce tokens by at least 10%,
		// escalate to the next strategy level to avoid spinning on the same approach.
		currentTokens := a.estimateTokens(messages)
		reduction := float64(prevTokenEstimate-currentTokens) / float64(prevTokenEstimate)
		if reduction < 0.10 && attempt < a.maxCompactionAttempts-1 {
			a.logger.Warn("compaction made insufficient progress, escalating",
				"prev_tokens", prevTokenEstimate,
				"current_tokens", currentTokens,
				"reduction_pct", fmt.Sprintf("%.1f%%", reduction*100),
				"attempt", attempt+1,
			)
			// Skip ahead: if we were at managed, jump to aggressive; if aggressive, jump to emergency.
			if attempt == 0 {
				messages = a.aggressiveCompaction(ctx, messages)
			}
		}
		prevTokenEstimate = a.estimateTokens(messages)
	}

	return nil, fmt.Errorf("context overflow: compacted %d times but still exceeded context limit", a.maxCompactionAttempts)
}

// ---------------------------------------------------------------------------
// Tool definition thinning — reduces the token cost of tool definitions
// when context overflow cannot be resolved by message compaction alone.
// ---------------------------------------------------------------------------

// estimateToolDefTokens estimates the token cost of tool definitions.
// Tool definitions are serialized as JSON in the API request and count against
// the context window, but message-based compaction never touches them.
func estimateToolDefTokens(tools []ToolDefinition, model string) int {
	totalChars := 0
	for _, t := range tools {
		// Fixed JSON overhead: {"type":"function","function":{...}}
		totalChars += 40 + len(t.Function.Name) + len(t.Function.Description)
		totalChars += len(t.Function.Parameters)
	}
	// JSON/schema content tokenizes at ~60% of natural language rate
	// due to repeated structural tokens ({, }, "type", "string", etc.).
	ratio := charsPerToken(model) * 0.65
	if ratio < 1.5 {
		ratio = 1.5
	}
	return int(float64(totalChars)/ratio + 0.5)
}

// thinToolDefinitions progressively strips tool definitions to reduce token cost.
//
//	level 0: strip tool descriptions (biggest win — up to 400 chars × N tools).
//	level 1: also strip parameter descriptions from JSON schemas.
//
// Returns a new slice; the originals are not modified.
func thinToolDefinitions(tools []ToolDefinition, level int) []ToolDefinition {
	result := make([]ToolDefinition, len(tools))
	for i, t := range tools {
		result[i] = ToolDefinition{
			Type: t.Type,
			Function: FunctionDef{
				Name:       t.Function.Name,
				Description: t.Function.Description,
				Parameters: append(json.RawMessage(nil), t.Function.Parameters...),
			},
		}
	}

	if level >= 0 {
		// Strip tool descriptions.
		for i := range result {
			result[i].Function.Description = ""
		}
	}

	if level >= 1 {
		// Strip parameter descriptions from JSON schemas.
		for i := range result {
			result[i].Function.Parameters = stripParamDescriptions(result[i].Function.Parameters)
		}
	}

	return result
}

// stripParamDescriptions removes "description" fields from a JSON Schema
// parameter definition to reduce token cost. Falls back to the original
// if parsing fails.
func stripParamDescriptions(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return raw
	}
	stripDescriptionsRecursive(schema)
	out, err := json.Marshal(schema)
	if err != nil {
		return raw
	}
	return out
}

// stripDescriptionsRecursive removes "description" keys from a nested map,
// handling "properties" and "items" sub-schemas.
func stripDescriptionsRecursive(m map[string]interface{}) {
	delete(m, "description")
	if props, ok := m["properties"].(map[string]interface{}); ok {
		for _, v := range props {
			if prop, ok := v.(map[string]interface{}); ok {
				stripDescriptionsRecursive(prop)
			}
		}
	}
	if items, ok := m["items"].(map[string]interface{}); ok {
		stripDescriptionsRecursive(items)
	}
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

	a.logger.Info("executing pre-compaction memory extraction", "token_estimate", tokenEstimate)

	// Use structured memory extraction: extract categorized memories
	// (decisions, preferences, facts, learnings) from the conversation.
	extractor := NewMemoryExtractor(a.llm, a.logger)
	memories := extractor.Extract(ctx, messages, a.modelOverride)

	if len(memories) == 0 {
		a.logger.Debug("memory flush: no memories extracted")
		return
	}

	// Format and persist the extracted memories to the memory directory.
	// The MemoryIndexer watches this directory and will index the file,
	// making extracted memories searchable in future sessions.
	formatted := FormatMemoriesForStorage(memories, a.sessionID)
	if formatted != "" && a.memoryIndexer != nil && a.memoryIndexer.MemoryDir() != "" {
		filename := fmt.Sprintf("pre-compact-%s-%s.md",
			a.sessionID, time.Now().Format("20060102-150405"))
		filePath := filepath.Join(a.memoryIndexer.MemoryDir(), filename)
		if err := os.WriteFile(filePath, []byte(formatted), 0o600); err != nil {
			a.logger.Warn("failed to write extracted memories", "path", filePath, "error", err)
		} else {
			a.logger.Info("pre-compaction memories saved",
				"count", len(memories),
				"path", filePath,
			)
			// Trigger async memory index so the file is indexed immediately.
			go a.memoryIndexer.IndexNow()
		}
	}

	a.logger.Info("memory flush completed", "extracted", len(memories))

	// KG extraction as "context residual" — captures structured facts
	// that survive compaction even when the narrative is summarized away.
	// Runs AFTER MemoryExtractor (which produces flat markdown memories).
	// Best-effort: errors logged, never block compaction.
	if a.kg != nil && a.cfg.KG.AutoExtract != "off" {
		a.kgExtractFromMemories(ctx, memories)
	}
}

// kgExtractFromMemories extracts structured KG triples from pre-compaction
// memories. Pattern-based extraction is fast (~1ms/memory) and always runs.
// LLM-based extraction is optional, budget-capped, and circuit-breaker gated.
// This is best-effort: any error is logged and swallowed.
func (a *AgentRun) kgExtractFromMemories(ctx context.Context, memories []ExtractedMemory) {
	patternSets := kg.DefaultPatternSets()
	if patternSets == nil {
		a.logger.Warn("kg extraction: no default pattern sets available")
		return
	}

	// Pattern-based extraction (fast, always runs when enabled).
	mode := a.cfg.KG.AutoExtract
	if mode == "pattern" || mode == "both" {
		patternCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		extractor, err := kg.NewExtractor(patternSets, a.logger)
		if err != nil {
			a.logger.Warn("kg extraction: failed to create extractor", "error", err)
			return
		}

		var totalExtracted int
		for _, mem := range memories {
			if patternCtx.Err() != nil {
				break
			}
			if mem.Importance >= 3 {
				n, err := extractor.ExtractAndStore(patternCtx, a.kg, mem.Content, "", a.sessionID)
				if err != nil {
					a.logger.Warn("kg pattern extraction failed", "error", err)
					continue
				}
				totalExtracted += n
			}
		}
		if totalExtracted > 0 {
			a.logger.Info("kg pattern extraction complete", "triples", totalExtracted)
		}
	}

	// LLM-based extraction is supported when the LLM client implements
	// kg.LLMProvider. Currently deferred — pattern extraction covers the
	// primary use case. Future: add adapter for LLMClient → kg.LLMProvider.
}

// estimateTokens provides a rough token estimate for a slice of messages.
// Uses model-aware chars-per-token ratio with role-specific weighting:
// tool results use ~2 chars/token (structured data), others use model ratio.
func (a *AgentRun) estimateTokens(messages []chatMessage) int {
	ratio := charsPerToken(a.modelOverride)
	totalTokens := 0
	for _, m := range messages {
		charCount := len(m.Role)
		if s, ok := m.Content.(string); ok {
			charCount += len(s)
		} else if m.Content != nil {
			charCount += len(fmt.Sprintf("%v", m.Content))
		}
		if m.ToolCallID != "" {
			charCount += len(m.ToolCallID)
		}
		// Tool results tokenize at higher density (~2 chars/token).
		if m.Role == "tool" {
			totalTokens += int(float64(charCount)/2.0 + 0.5)
		} else {
			totalTokens += int(float64(charCount)/ratio + 0.5)
		}
	}
	return totalTokens
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
	case strings.Contains(model, "glm-5.1"):
		return 1048576
	case strings.Contains(model, "glm-5"):
		return 202752
	case strings.Contains(model, "glm-4"):
		return 128000
	default:
		return 128000
	}
}

func (a *AgentRun) managedCompaction(ctx context.Context, messages []chatMessage) []chatMessage {
	// LCM path: ingest + compact + reassemble.
	if a.lcmEngine != nil && a.lcmConversationID != "" {
		if err := a.lcmIngestNew(ctx, messages); err != nil {
			a.logger.Warn("lcm ingest failed, falling back to legacy compaction", "err", err)
		} else {
			summarizeFn := buildLCMSummarizeFn(a.llm, a.cfg.Compaction, a.cfg.Compaction.LCM, a.modelOverride, a.logger)
			ctxWindow := a.getModelContextWindow()
			compacted, err := a.lcmEngine.Compact(ctx, a.lcmConversationID, ctxWindow, summarizeFn)
			if err != nil {
				a.logger.Warn("lcm compaction failed, falling back to legacy", "err", err)
			} else if compacted {
				// Extract system prompt from the current messages so the
				// reassembled context preserves it.
				sysPrompt := ""
				if len(messages) > 0 && messages[0].Role == "system" {
					if s, ok := messages[0].Content.(string); ok {
						sysPrompt = s
					}
				}
				assembled, err := a.lcmEngine.Assemble(ctx, a.lcmConversationID, sysPrompt, "", ctxWindow)
				if err != nil {
					a.logger.Warn("lcm assembly after compaction failed, falling back to legacy", "err", err)
				} else {
					a.lcmIngestedSeq = len(assembled)
					a.persistCompactionSummary("[LCM DAG compaction]", len(messages), len(assembled))
					return assembled
				}
			} else {
				return messages // No compaction needed.
			}
		}
	}

	// Legacy path.
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

	// Send "writing" reaction while compaction is in progress.
	if a.reactionSender != nil {
		a.reactionSender("\u270d", false) // ✍
		defer a.reactionSender("\u270d", true)

		// Stall detection: send ⏳ if compaction takes longer than 30s.
		var stallFired atomic.Bool
		stallTimer := time.AfterFunc(30*time.Second, func() {
			stallFired.Store(true)
			a.reactionSender("\u23f3", false) // ⏳
		})
		defer func() {
			stallTimer.Stop()
			if stallFired.Load() {
				a.reactionSender("\u23f3", true) // remove ⏳
			}
		}()
	}

	// Wrap summarization with a configurable safety timeout to prevent hung compactions.
	// If the LLM summarization hangs, fall back to a simple trim-by-count strategy.
	compactionTimeout := time.Duration(resolvedCompactionConfig(a.cfg.Compaction).TimeoutSeconds) * time.Second
	compactionCtx, compactionCancel := context.WithTimeout(ctx, compactionTimeout)
	summary := a.summarizeInStages(compactionCtx, middle)
	compactionCancel()

	if compactionCtx.Err() != nil {
		a.logger.Warn("compaction timed out, falling back to trim-by-count")
		summary = "Compaction timed out. Earlier conversation context was discarded."
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

// persistCompactionSummary saves the compaction summary to session persistence.
func (a *AgentRun) persistCompactionSummary(summary string, before, after int) {
	if summary == "" {
		return
	}

	if a.sessionPersistence != nil && a.sessionID != "" {
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

			// Truncate the session file to prevent unbounded JSONL growth.
			if err := a.sessionPersistence.TruncateAfterCompaction(a.sessionID, after); err != nil {
				a.logger.Warn("failed to truncate session after compaction", "session", a.sessionID, "err", err)
			}
		}
	}

	// Increment in-memory compaction count on the session.
	if a.session != nil {
		a.session.IncrementCompactionCount()
	}

	// Post-compaction memory sync: trigger indexer if configured.
	syncMode := resolvedCompactionConfig(a.cfg.Compaction).PostIndexSync
	if a.memoryIndexer != nil && (syncMode == "async" || syncMode == "await") {
		a.logger.Debug("post-compaction memory sync triggered", "mode", syncMode)
		go a.memoryIndexer.IndexNow()
	}
}

// aggressiveCompaction is used when the context still overflows despite managed compaction.
// It cuts the recent context even shorter and truncates large text heavily.
func (a *AgentRun) aggressiveCompaction(ctx context.Context, messages []chatMessage) []chatMessage {
	// LCM path: ingest pending messages and trigger a forced compaction sweep.
	if a.lcmEngine != nil && a.lcmConversationID != "" {
		if err := a.lcmIngestNew(ctx, messages); err != nil {
			a.logger.Warn("lcm aggressive: ingest failed, falling back to legacy", "err", err)
		} else {
			summarizeFn := buildLCMSummarizeFn(a.llm, a.cfg.Compaction, a.cfg.Compaction.LCM, a.modelOverride, a.logger)
			ctxWindow := a.getModelContextWindow()
			// Force a sweep even if ShouldCompact says no.
			if _, err := a.lcmEngine.compactor.FullSweep(ctx, a.lcmConversationID, summarizeFn); err != nil {
				a.logger.Warn("lcm aggressive: sweep failed, falling back to legacy", "err", err)
			} else {
				// Update last_compact_at since we bypassed LCMEngine.Compact().
				if err := a.lcmEngine.Store().UpdateLastCompactAt(a.lcmConversationID); err != nil {
					a.logger.Warn("lcm aggressive: failed to update last_compact_at", "err", err)
				}
				sysPrompt := ""
				if len(messages) > 0 && messages[0].Role == "system" {
					if s, ok := messages[0].Content.(string); ok {
						sysPrompt = s
					}
				}
				assembled, err := a.lcmEngine.Assemble(ctx, a.lcmConversationID, sysPrompt, "", ctxWindow)
				if err != nil {
					a.logger.Warn("lcm aggressive: assembly failed, falling back to legacy", "err", err)
				} else {
					a.lcmIngestedSeq = len(assembled)
					a.persistCompactionSummary("[LCM aggressive compaction]", len(messages), len(assembled))
					return assembled
				}
			}
		}
	}

	// Legacy path.
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
// Instead of discarding all history, it attempts a fast LLM summarization with
// aggressive truncation (200 chars per message) and a short timeout. If the LLM
// fails, it falls back to buildMinimalFallbackSummary() which preserves metadata
// (message counts, tool names, identifiers) instead of discarding everything.
//
// This function should only be called when:
// 1. Managed compaction failed
// 2. Aggressive compaction failed
// 3. Context still overflows
func (a *AgentRun) emergencyCompression(ctx context.Context, messages []chatMessage) []chatMessage {
	var header []chatMessage
	var body []chatMessage

	for _, m := range messages {
		if m.Role == "system" {
			header = append(header, m)
		} else {
			body = append(body, m)
		}
	}

	if len(body) == 0 {
		return header
	}

	goal := body[0]

	// Keep the last 2 messages for continuity.
	keepLast := 2
	if keepLast > len(body)-1 {
		keepLast = len(body) - 1
	}
	var recent []chatMessage
	if keepLast > 0 {
		recent = body[len(body)-keepLast:]
	}

	// Try to summarize the middle with aggressive truncation.
	middle := body[1:]
	if keepLast > 0 && len(body)-1-keepLast > 0 {
		middle = body[1 : len(body)-keepLast]
	}

	var summary string
	if len(middle) > 0 {
		// Aggressively truncate all messages to 200 chars for the summarizer.
		truncated := make([]chatMessage, len(middle))
		for i, m := range middle {
			truncated[i] = m
			if s, ok := m.Content.(string); ok && len(s) > 200 {
				truncated[i].Content = s[:200] + "...(truncated)"
			}
		}

		// Short timeout for emergency summarization.
		emergCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		summary = a.summarizeMiddle(emergCtx, truncated)
		cancel()

		if summary == "" || strings.HasPrefix(summary, "Failed to summarize") {
			a.logger.Warn("emergency LLM summarization failed, using metadata fallback")
			summary = buildMinimalFallbackSummary(messages)
		}
	} else {
		summary = buildMinimalFallbackSummary(messages)
	}

	var compacted []chatMessage
	compacted = append(compacted, header...)
	compacted = append(compacted, goal)
	compacted = append(compacted, chatMessage{
		Role:    "user",
		Content: "[System: Emergency context compression applied. " + summary + "]",
	})
	compacted = append(compacted, recent...)

	// Repair orphan tool use/result pairs.
	compacted = RepairToolUseResultPairing(compacted)

	a.logger.Warn("emergency compression applied",
		"original_len", len(messages),
		"compacted_len", len(compacted),
	)

	// Persist emergency compression summary
	a.persistCompactionSummary(summary, len(messages), len(compacted))

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
	ccfg := resolvedCompactionConfig(a.cfg.Compaction)
	mergePrompt := []chatMessage{
		{
			Role: "system",
			Content: "You are a summarizing assistant. Combine these partial conversation summaries " +
				"into a single coherent summary preserving all section headings " +
				"(## Decisions, ## Open TODOs, ## Constraints/Rules, ## Pending user asks, ## Exact identifiers). " +
				"Merge entries under the same heading. Keep it concise. " +
				"Preserve key facts, tool results, and current status. " +
				"NEVER use text formatting like bold.",
		},
		{
			Role:    "user",
			Content: "Combine these partial summaries:\n\n" + strings.Join(partialSummaries, "\n---\n"),
		},
	}

	mergeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := a.llm.CompleteWithFallbackUsingModel(mergeCtx, ccfg.CompactionModel, mergePrompt, nil)
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
// Applies a 1.2x safety margin to compensate for chars/4 underestimation
// (multi-byte chars, code tokens, special tokens, JSON overhead).
//
// Tool results use a higher token density (~2 chars/token vs ~4 for text)
// because structured data (JSON, code, stack traces) tokenizes less efficiently.
// Aligned with OpenClaw's TOOL_RESULT_CHARS_PER_TOKEN_ESTIMATE = 2.
func (a *AgentRun) estimateMessageTokens(m chatMessage) int {
	content := ""
	if s, ok := m.Content.(string); ok {
		content = s
	}

	var tokens int
	if m.Role == "tool" {
		// Tool results: ~2 chars per token (structured data, code, JSON).
		tokens = len(content) / 2
	} else {
		// Regular messages: ~4 chars per token.
		tokens = len(content) / 4
	}

	if len(m.ToolCalls) > 0 {
		tokens += len(m.ToolCalls) * 50 // ~50 tokens per tool call metadata
	}
	if tokens < 10 {
		tokens = 10
	}
	// Safety margin: chars/token underestimates for multi-byte and special tokens.
	tokens = int(float64(tokens) * 1.2)
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

		summary := a.summarizeWithQualityGuard(ctx, chunk)
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
	return a.summarizeMiddleWithFeedback(ctx, middle, "")
}

// summarizeMiddleWithFeedback summarizes a chunk of messages using the structured
// compaction prompt. If qualityFeedback is non-empty, it is appended to the user
// prompt to guide the LLM on a retry attempt.
func (a *AgentRun) summarizeMiddleWithFeedback(ctx context.Context, middle []chatMessage, qualityFeedback string) string {
	ccfg := resolvedCompactionConfig(a.cfg.Compaction)

	// Collect tool failures and file operations from the middle section.
	toolFailures := a.collectToolFailures(middle, ccfg.MaxToolFailures)
	readFiles, modifiedFiles := a.collectFileOperationsSeparated(middle)

	// Build structured compaction prompt.
	systemPrompt := buildStructuredCompactionPrompt(ccfg, toolFailures, readFiles, modifiedFiles)

	// Build transcript for the LLM.
	var textBuilder strings.Builder
	for _, m := range middle {
		content := ""
		if s, ok := m.Content.(string); ok {
			maxLen := 1000
			if m.Role == "tool" {
				maxLen = 500
			}
			if len(s) > maxLen {
				content = s[:maxLen] + "...(truncated)"
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

		textBuilder.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, content))
	}

	userContent := "Summarize this history:\n\n" + textBuilder.String()
	if qualityFeedback != "" {
		userContent += "\n\n[Quality feedback from previous attempt]\n" + qualityFeedback
	}

	prompt := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	sumCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := a.llm.CompleteWithFallbackUsingModel(sumCtx, ccfg.CompactionModel, prompt, nil)
	if err != nil {
		a.logger.Warn("compaction summary failed", "error", err)
		return "Failed to summarize earlier steps due to error."
	}

	return resp.Content
}

// summarizeWithQualityGuard wraps summarizeMiddle with quality auditing and retry.
// If quality guard is disabled, it falls through to summarizeMiddle directly.
func (a *AgentRun) summarizeWithQualityGuard(ctx context.Context, chunk []chatMessage) string {
	ccfg := resolvedCompactionConfig(a.cfg.Compaction)

	summary := a.summarizeMiddle(ctx, chunk)
	if summary == "" || strings.HasPrefix(summary, "Failed to summarize") {
		return summary
	}

	if !ccfg.QualityGuard.qualityGuardEnabled() {
		return summary
	}

	// Extract identifiers and last user ask for quality audit.
	identifiers := extractIdentifiers(chunk, 20)
	lastUserAsk := ""
	recentUserTurns := collectRecentUserTurns(chunk, 1)
	if len(recentUserTurns) > 0 {
		if s, ok := recentUserTurns[0].Content.(string); ok {
			lastUserAsk = s
		}
	}

	audit := auditSummaryQuality(summary, identifiers, lastUserAsk, ccfg.QualityGuard.StrictIdentifiers)
	if audit.Passed {
		return summary
	}

	a.logger.Info("compaction quality audit failed, retrying",
		"failures", audit.Failures,
		"max_retries", ccfg.QualityGuard.MaxRetries,
	)

	// Retry with feedback.
	for attempt := 0; attempt < ccfg.QualityGuard.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			break
		}

		feedback := "Your previous summary had these issues:\n"
		for _, f := range audit.Failures {
			feedback += "- " + f + "\n"
		}
		feedback += "Please fix these issues and produce a corrected summary."

		summary = a.summarizeMiddleWithFeedback(ctx, chunk, feedback)
		if summary == "" || strings.HasPrefix(summary, "Failed to summarize") {
			break
		}

		audit = auditSummaryQuality(summary, identifiers, lastUserAsk, ccfg.QualityGuard.StrictIdentifiers)
		if audit.Passed {
			a.logger.Info("compaction quality audit passed on retry", "attempt", attempt+1)
			return summary
		}
	}

	// Return best effort summary even if audit still fails.
	a.logger.Warn("compaction quality audit still failing after retries, using best-effort summary",
		"remaining_failures", audit.Failures,
	)
	return summary
}

// collectFileOperationsSeparated extracts file paths from tool calls, separating
// read operations from write/edit/create operations. Files that were both read and
// modified only appear in the modified list.
func (a *AgentRun) collectFileOperationsSeparated(messages []chatMessage) (readFiles []string, modifiedFiles []string) {
	readSet := make(map[string]bool)
	modifiedSet := make(map[string]bool)

	for _, m := range messages {
		for _, tc := range m.ToolCalls {
			args, err := parseToolArgs(tc.Function.Arguments)
			if err != nil {
				continue
			}

			var path string
			for _, key := range []string{"path", "file_path"} {
				if p, ok := args[key]; ok {
					if s, ok := p.(string); ok {
						path = s
						break
					}
				}
			}
			if path == "" {
				continue
			}

			switch tc.Function.Name {
			case "read_file":
				readSet[path] = true
			case "write_file", "edit_file", "create_file":
				modifiedSet[path] = true
			}
		}
	}

	// Files that were read and then modified only appear in modified.
	for path := range readSet {
		if !modifiedSet[path] {
			readFiles = append(readFiles, path)
		}
	}
	for path := range modifiedSet {
		modifiedFiles = append(modifiedFiles, path)
	}

	sort.Strings(readFiles)
	sort.Strings(modifiedFiles)
	return readFiles, modifiedFiles
}

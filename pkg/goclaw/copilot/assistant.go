// Package copilot implements the main orchestrator for GoClaw.
// Coordinates channels, skills, scheduler, access control, workspaces,
// and security to process user messages and generate LLM responses.
package copilot

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/memory"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/security"
	"github.com/jholhewres/goclaw/pkg/goclaw/sandbox"
	"github.com/jholhewres/goclaw/pkg/goclaw/scheduler"
	"github.com/jholhewres/goclaw/pkg/goclaw/skills"
)

// Assistant is the main orchestrator for GoClaw.
// Message flow: receive → access check → command check → trigger check →
// workspace resolve → input validation → context build → agent → output validation → send.
type Assistant struct {
	config *Config

	// channelMgr manages communication channels.
	channelMgr *channels.Manager

	// accessMgr manages access control (who can use the bot).
	accessMgr *AccessManager

	// workspaceMgr manages isolated workspaces/profiles.
	workspaceMgr *WorkspaceManager

	// llmClient communicates with the LLM provider API.
	llmClient *LLMClient

	// toolExecutor manages tool registration and dispatches tool calls from the LLM.
	toolExecutor *ToolExecutor

	// approvalMgr manages pending tool approvals for RequireConfirmation tools.
	approvalMgr *ApprovalManager

	// skillRegistry manages available skills.
	skillRegistry *skills.Registry

	// scheduler manages scheduled tasks.
	scheduler *scheduler.Scheduler

	// sessionStore manages sessions for the default workspace (backward compat).
	sessionStore *SessionStore

	// promptComposer builds layered prompts.
	promptComposer *PromptComposer

	// inputGuard validates inputs before processing.
	inputGuard *security.InputGuardrail

	// outputGuard validates outputs before sending.
	outputGuard *security.OutputGuardrail

	// memoryStore provides persistent long-term memory (file-based, always available).
	memoryStore *memory.FileStore

	// sqliteMemory provides advanced memory with FTS5 + vector search.
	sqliteMemory *memory.SQLiteStore

	// subagentMgr orchestrates subagent spawning and lifecycle.
	subagentMgr *SubagentManager

	// heartbeat runs periodic proactive checks (stored for config hot-reload).
	heartbeat *Heartbeat

	// messageQueue handles message bursts with debouncing per session.
	messageQueue *MessageQueue

	// activeRuns tracks cancel functions for in-flight agent runs (key: workspaceID:sessionID).
	activeRuns   map[string]context.CancelFunc
	activeRunsMu sync.Mutex

	// usageTracker records token usage and estimated costs per session.
	usageTracker *UsageTracker

	// configMu protects hot-reloadable config fields.
	configMu sync.RWMutex

	logger *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new Assistant with all dependencies.
func New(cfg *Config, logger *slog.Logger) *Assistant {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	te := NewToolExecutor(logger)
	te.Configure(cfg.Security.ToolExecutor)

	// Initialize the tool security guard.
	toolGuard := NewToolGuard(cfg.Security.ToolGuard, logger)
	te.SetGuard(toolGuard)

	// Initialize approval manager for RequireConfirmation tools.
	approvalMgr := NewApprovalManager(logger)

	// Create assistant first (needed for onDrain closure).
	a := &Assistant{
		config:         cfg,
		channelMgr:     channels.NewManager(logger.With("component", "channels")),
		accessMgr:      NewAccessManager(cfg.Access, logger),
		workspaceMgr:   NewWorkspaceManager(cfg, cfg.Workspaces, logger),
		llmClient:      NewLLMClient(cfg, logger),
		toolExecutor:   te,
		approvalMgr:    approvalMgr,
		skillRegistry:  skills.NewRegistry(logger.With("component", "skills")),
		sessionStore:   NewSessionStore(logger.With("component", "sessions")),
		promptComposer: NewPromptComposer(cfg),
		inputGuard:     security.NewInputGuardrail(cfg.Security.MaxInputLength, cfg.Security.RateLimit),
		outputGuard:    security.NewOutputGuardrail(),
		subagentMgr:    NewSubagentManager(cfg.Subagents, logger),
		activeRuns:     make(map[string]context.CancelFunc),
		usageTracker:   NewUsageTracker(logger.With("component", "usage")),
		logger:         logger,
	}

	// Wire message queue with onDrain callback (requires assistant reference).
	debounceMs := cfg.Queue.DebounceMs
	if debounceMs <= 0 {
		debounceMs = 1000
	}
	maxPending := cfg.Queue.MaxPending
	if maxPending <= 0 {
		maxPending = 20
	}
	a.messageQueue = NewMessageQueue(debounceMs, maxPending, a.handleDrainedMessages, logger)

	// Wire confirmation requester for tools in RequireConfirmation list.
	te.SetConfirmationRequester(func(sessionID, callerJID, toolName string, args map[string]any) (bool, error) {
		sendMsg := func(msg string) {
			channel, chatID, ok := strings.Cut(sessionID, ":")
			if !ok {
				return
			}
			_ = a.channelMgr.Send(a.ctx, channel, chatID, &channels.OutgoingMessage{Content: msg})
		}
		return approvalMgr.Request(sessionID, callerJID, toolName, args, sendMsg)
	})

	return a
}

// Start initializes and starts all subsystems.
func (a *Assistant) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)

	a.logger.Info("starting GoClaw Copilot",
		"name", a.config.Name,
		"model", a.config.Model,
		"access_policy", a.config.Access.DefaultPolicy,
		"workspaces", a.workspaceMgr.Count(),
	)

	// 0. Initialize memory stores.
	memDir := filepath.Join(filepath.Dir(a.config.Memory.Path), "memory")
	memStore, err := memory.NewFileStore(memDir)
	if err != nil {
		a.logger.Warn("memory store not available", "error", err)
	} else {
		a.memoryStore = memStore
	}

	// 0a. Initialize SQLite memory with FTS5 + vector search (if configured).
	if a.config.Memory.Type == "sqlite" {
		embedCfg := a.config.Memory.Embedding
		// Use main API key if embedding key not set.
		if embedCfg.APIKey == "" {
			embedCfg.APIKey = a.config.API.APIKey
		}
		embedder := memory.NewEmbeddingProvider(embedCfg)

		dbPath := a.config.Memory.Path
		if dbPath == "" {
			dbPath = "./data/memory.db"
		}

		sqlStore, err := memory.NewSQLiteStore(dbPath, embedder, a.logger.With("component", "memory-index"))
		if err != nil {
			a.logger.Warn("SQLite memory store not available, falling back to file-based",
				"error", err)
		} else {
			a.sqliteMemory = sqlStore
			a.logger.Info("SQLite memory store initialized",
				"embedding_provider", embedder.Name(),
				"db", dbPath,
			)

			// Index memory files in background (fire-and-forget).
			if a.config.Memory.Index.Auto {
				go func() {
					chunkCfg := memory.ChunkConfig{
						MaxTokens: a.config.Memory.Index.ChunkMaxTokens,
						Overlap:   100,
					}
					if chunkCfg.MaxTokens <= 0 {
						chunkCfg.MaxTokens = 500
					}
					if err := sqlStore.IndexMemoryDir(a.ctx, memDir, chunkCfg); err != nil {
						a.logger.Warn("initial memory indexing failed", "error", err)
					}
				}()
			}
		}
	}

	// 0b. Connect memory store and skill getter to prompt composer.
	if a.memoryStore != nil {
		a.promptComposer.SetMemoryStore(a.memoryStore)
	}
	if a.sqliteMemory != nil {
		a.promptComposer.SetSQLiteMemory(a.sqliteMemory)
	}
	a.promptComposer.SetSkillGetter(func(name string) (interface{ SystemPrompt() string }, bool) {
		skill, ok := a.skillRegistry.Get(name)
		if !ok {
			return nil, false
		}
		return skill, true
	})

	// 0c. Wire session persistence (JSONL on disk).
	sessDir := filepath.Join(filepath.Dir(a.config.Memory.Path), "sessions")
	if sessDir == "" {
		sessDir = "./data/sessions"
	}
	sessPersist, err := NewSessionPersistence(sessDir, a.logger.With("component", "session-persist"))
	if err != nil {
		a.logger.Warn("session persistence not available", "error", err)
	} else {
		a.sessionStore.SetPersistence(sessPersist)
		a.logger.Info("session persistence enabled", "dir", sessDir)
	}

	// 1. Register skill loaders and load all skills.
	a.registerSkillLoaders()
	if err := a.skillRegistry.LoadAll(a.ctx); err != nil {
		a.logger.Error("failed to load skills", "error", err)
	}

	// 1b. Initialize skills with sandbox runner.
	a.initializeSkills()

	// 1c. Register skill tools + system tools in the executor.
	a.registerSkillTools()

	// 1d. Create and start scheduler if enabled.
	if a.config.Scheduler.Enabled {
		a.initScheduler()
	}

	// 1e. Register system tools (needs scheduler to be created first).
	a.registerSystemTools()

	// 2. Start channel manager (allows 0 channels for CLI mode).
	if err := a.channelMgr.Start(a.ctx); err != nil {
		return fmt.Errorf("failed to start channels: %w", err)
	}

	// 3. Start session pruners for all workspaces.
	a.workspaceMgr.StartPruners(a.ctx)

	// 4. Start scheduler if created.
	if a.scheduler != nil {
		if err := a.scheduler.Start(a.ctx); err != nil {
			a.logger.Error("failed to start scheduler", "error", err)
		}
	}

	// 5. Start heartbeat if enabled.
	if a.config.Heartbeat.Enabled {
		a.heartbeat = NewHeartbeat(a.config.Heartbeat, a, a.logger)
		a.heartbeat.Start(a.ctx)
	}

	// 6. Start main message processing loop.
	go a.messageLoop()

	// 7. Run BOOT.md if present (like OpenClaw's gateway:startup → BOOT.md).
	// Executes after all channels are connected, with a short delay for stabilization.
	go a.runBootOnce()

	a.logger.Info("GoClaw Copilot started successfully")
	return nil
}

// runBootOnce executes BOOT.md instructions once after startup.
// If BOOT.md exists in the workspace, its content is fed to the agent as a
// startup command. This enables proactive behaviors like "check emails" or
// "review today's calendar" on boot.
func (a *Assistant) runBootOnce() {
	// Short delay to let channels stabilize (like OpenClaw's 250ms hook delay).
	time.Sleep(500 * time.Millisecond)

	// Search for BOOT.md in the workspace directories.
	searchDirs := []string{"."}
	if a.config.Heartbeat.WorkspaceDir != "" && a.config.Heartbeat.WorkspaceDir != "." {
		searchDirs = append([]string{a.config.Heartbeat.WorkspaceDir}, searchDirs...)
	}
	searchDirs = append(searchDirs, "configs")

	var bootContent string
	for _, dir := range searchDirs {
		data, err := os.ReadFile(filepath.Join(dir, "BOOT.md"))
		if err == nil && len(strings.TrimSpace(string(data))) > 0 {
			bootContent = strings.TrimSpace(string(data))
			break
		}
	}

	if bootContent == "" {
		return
	}

	a.logger.Info("executing BOOT.md startup instructions")

	// Create a dedicated session for boot.
	session := a.sessionStore.GetOrCreate("system", "boot")
	prompt := a.promptComposer.Compose(session, bootContent)

	agent := NewAgentRunWithConfig(a.llmClient, a.toolExecutor, a.config.Agent, a.logger)
	result, err := agent.Run(a.ctx, prompt, nil, bootContent)
	if err != nil {
		a.logger.Error("BOOT.md execution failed", "error", err)
		return
	}

	session.AddMessage(bootContent, result)
	a.logger.Info("BOOT.md execution completed",
		"result_preview", truncate(result, 200),
	)
}

// Stop gracefully shuts down all subsystems.
func (a *Assistant) Stop() {
	a.logger.Info("stopping GoClaw Copilot...")

	if a.cancel != nil {
		a.cancel()
	}

	// Shut down in reverse initialization order.
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	a.channelMgr.Stop()
	a.skillRegistry.ShutdownAll()

	// Close SQLite memory store.
	if a.sqliteMemory != nil {
		if err := a.sqliteMemory.Close(); err != nil {
			a.logger.Warn("error closing SQLite memory", "error", err)
		}
	}

	a.logger.Info("GoClaw Copilot stopped")
}

// ApplyConfigUpdate applies hot-reloadable config changes. Updates: access control,
// instructions, tool guard, heartbeat, token budget. Does NOT update: API, channels,
// model, plugins (require restart).
func (a *Assistant) ApplyConfigUpdate(newCfg *Config) {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	a.config.Instructions = newCfg.Instructions
	a.config.Access = newCfg.Access
	a.config.Security.ToolGuard = newCfg.Security.ToolGuard
	a.config.Security.ToolExecutor = newCfg.Security.ToolExecutor
	a.config.Heartbeat = newCfg.Heartbeat
	a.config.TokenBudget = newCfg.TokenBudget

	a.accessMgr.ApplyConfig(newCfg.Access)
	a.toolExecutor.UpdateGuardConfig(newCfg.Security.ToolGuard)
	a.toolExecutor.Configure(newCfg.Security.ToolExecutor)
	if a.heartbeat != nil {
		a.heartbeat.UpdateConfig(newCfg.Heartbeat)
	}

	a.logger.Info("config hot-reload applied",
		"updated", []string{"access", "instructions", "tool_guard", "heartbeat", "token_budget"},
	)
}

// ChannelManager returns the channel manager for external registration.
func (a *Assistant) ChannelManager() *channels.Manager {
	return a.channelMgr
}

// AccessManager returns the access manager.
func (a *Assistant) AccessManager() *AccessManager {
	return a.accessMgr
}

// WorkspaceManager returns the workspace manager.
func (a *Assistant) WorkspaceManager() *WorkspaceManager {
	return a.workspaceMgr
}

// SkillRegistry returns the skills registry.
func (a *Assistant) SkillRegistry() *skills.Registry {
	return a.skillRegistry
}

// SetScheduler configures the assistant's scheduler.
func (a *Assistant) SetScheduler(s *scheduler.Scheduler) {
	a.scheduler = s
}

// handleDrainedMessages processes messages drained from the queue after debounce.
// Called by MessageQueue when the debounce timer fires.
func (a *Assistant) handleDrainedMessages(sessionID string, msgs []*channels.IncomingMessage) {
	if len(msgs) == 0 {
		return
	}
	combined := a.messageQueue.CombineMessages(msgs)
	// Use first message as base for metadata; replace content with combined.
	synthetic := *msgs[0]
	synthetic.Content = combined
	synthetic.ID = msgs[0].ID + "-combined"
	a.handleMessage(&synthetic)
}

// messageLoop is the main loop that processes messages from all channels.
func (a *Assistant) messageLoop() {
	for {
		select {
		case msg, ok := <-a.channelMgr.Messages():
			if !ok {
				return
			}
			go a.handleMessage(msg)

		case <-a.ctx.Done():
			return
		}
	}
}

// handleMessage processes an individual message following the full flow:
// access check → command → trigger → workspace → validate → build → execute → validate → send.
func (a *Assistant) handleMessage(msg *channels.IncomingMessage) {
	start := time.Now()
	logger := a.logger.With(
		"channel", msg.Channel,
		"chat_id", msg.ChatID,
		"from", msg.From,
		"msg_id", msg.ID,
	)

	logger.Info("incoming message",
		"content_preview", truncate(msg.Content, 50),
		"type", msg.Type,
		"is_group", msg.IsGroup,
	)

	// ── Step 0: Access control ──
	// Check if the sender is authorized BEFORE anything else.
	// This is the OpenClaw-style behavior: unknown contacts are silently ignored.
	accessResult := a.accessMgr.Check(msg)

	if !accessResult.Allowed {
		// If policy is "ask", send a one-time message.
		if accessResult.ShouldAsk {
			a.sendReply(msg, a.accessMgr.PendingMessage())
			a.accessMgr.MarkAsked(msg.From)
			logger.Info("access pending, sent request message",
				"from", msg.From)
		} else {
			logger.Info("message ignored (access denied)",
				"reason", accessResult.Reason,
				"from_raw", msg.From)
		}
		return
	}

	logger.Info("access granted", "level", accessResult.Level)

	// ── Step 1: Admin commands ──
	// Check for /commands BEFORE trigger check (commands always work).
	if IsCommand(msg.Content) {
		result := a.HandleCommand(msg)
		if result.Handled {
			if result.Response != "" {
				a.sendReply(msg, result.Response)
			}
			logger.Info("admin command processed",
				"duration_ms", time.Since(start).Milliseconds())
			return
		}
	}

	// ── Step 1b: Message queue (if session already processing, enqueue and return) ──
	sessionID := msg.Channel + ":" + msg.ChatID
	if a.messageQueue.IsProcessing(sessionID) {
		if a.messageQueue.Enqueue(sessionID, msg) {
			logger.Info("message enqueued (session busy)", "session", sessionID)
		}
		return
	}
	a.messageQueue.SetProcessing(sessionID, true)
	defer a.messageQueue.SetProcessing(sessionID, false)

	// ── Step 2: Resolve workspace ──
	// Determine which workspace this message belongs to.
	resolved := a.workspaceMgr.Resolve(
		msg.Channel, msg.ChatID, msg.From, msg.IsGroup)

	workspace := resolved.Workspace
	session := resolved.Session

	logger = logger.With("workspace", workspace.ID)

	// ── Step 3: Check trigger ──
	// Use workspace trigger if set, otherwise global.
	trigger := a.config.Trigger
	if workspace.Trigger != "" {
		trigger = workspace.Trigger
	}
	if !a.matchesTrigger(msg.Content, trigger, msg.IsGroup) {
		return
	}

	logger.Info("message received, processing...",
		"access_level", accessResult.Level)

	// ── Step 3b: React, send typing indicator, and mark as read ──
	// React with 👀 to acknowledge receipt (like OpenClaw).
	a.channelMgr.SendReaction(a.ctx, msg.Channel, msg.ChatID, msg.ID, "👀")
	a.channelMgr.SendTyping(a.ctx, msg.Channel, msg.ChatID)
	a.channelMgr.MarkRead(a.ctx, msg.Channel, msg.ChatID, []string{msg.ID})

	// ── Step 4: Enrich content with media (images → description, audio → transcript) ──
	userContent := a.enrichMessageContent(a.ctx, msg, logger)

	// ── Step 5: Validate input ──
	if err := a.inputGuard.Validate(msg.From, userContent); err != nil {
		logger.Warn("input rejected", "error", err)
		a.sendReply(msg, fmt.Sprintf("Sorry, I can't process that: %v", err))
		return
	}

	// ── Step 6: Set caller and session context for tool security / approval ──
	a.toolExecutor.SetCallerContext(accessResult.Level, msg.From)
	a.toolExecutor.SetSessionContext(sessionID)

	// ── Step 7: Build prompt with workspace context ──
	prompt := a.composeWorkspacePrompt(workspace, session, userContent)

	// ── Step 8: Execute agent (with optional block streaming) ──
	bsCfg := a.config.BlockStream.Effective()
	var blockStreamer *BlockStreamer
	if bsCfg.Enabled {
		blockStreamer = NewBlockStreamer(bsCfg, a.channelMgr, msg.Channel, msg.ChatID, msg.ID)
	}

	response := a.executeAgentWithStream(a.ctx, workspace.ID, session, prompt, userContent, blockStreamer)

	// Finalize the block streamer (flush remaining text).
	if blockStreamer != nil {
		blockStreamer.Finish()
	}

	// ── Step 9: Validate output ──
	if err := a.outputGuard.Validate(response); err != nil {
		logger.Warn("output rejected, applying fallback", "error", err)
		response = "Sorry, I encountered an issue generating the response. Could you rephrase?"
	}

	// ── Step 10: Update session ──
	session.AddMessage(userContent, response)

	// ── Step 10b: Check if session needs compaction ──
	a.maybeCompactSession(session)

	// ── Step 11: Send reply (skip if block streamer already sent everything) ──
	if blockStreamer == nil || !blockStreamer.HasSentBlocks() {
		a.sendReply(msg, response)
	}

	// React with ✅ to signal processing is complete.
	a.channelMgr.SendReaction(a.ctx, msg.Channel, msg.ChatID, msg.ID, "✅")

	logger.Info("message processed",
		"duration_ms", time.Since(start).Milliseconds(),
		"workspace", workspace.ID,
	)
}

// matchesTrigger checks if a message matches the activation keyword.
// In DMs, the trigger is optional (always responds).
// In groups, the trigger is required unless the group has its own trigger.
func (a *Assistant) matchesTrigger(content, trigger string, isGroup bool) bool {
	// No trigger configured = always respond.
	if trigger == "" {
		return true
	}

	// In DMs, respond even without trigger.
	if !isGroup {
		return true
	}

	// In groups, require the trigger.
	content = strings.TrimSpace(content)
	return len(content) >= len(trigger) &&
		strings.EqualFold(content[:len(trigger)], trigger)
}

// composeWorkspacePrompt builds the prompt using workspace overrides.
func (a *Assistant) composeWorkspacePrompt(ws *Workspace, session *Session, input string) string {
	// If workspace has custom instructions, inject them as business context.
	if ws.Instructions != "" {
		cfg := session.GetConfig()
		if cfg.BusinessContext != ws.Instructions {
			cfg.BusinessContext = ws.Instructions
			session.SetConfig(cfg)
		}
	}

	return a.promptComposer.Compose(session, input)
}

// executeAgentWithStream runs the agentic loop, optionally streaming text
// progressively to the channel via a BlockStreamer.
func (a *Assistant) executeAgentWithStream(ctx context.Context, workspaceID string, session *Session, systemPrompt string, userMessage string, streamer *BlockStreamer) string {
	runKey := workspaceID + ":" + session.ID

	runCtx, cancel := context.WithCancel(ctx)
	defer func() {
		a.activeRunsMu.Lock()
		delete(a.activeRuns, runKey)
		a.activeRunsMu.Unlock()
		cancel()
	}()

	a.activeRunsMu.Lock()
	a.activeRuns[runKey] = cancel
	a.activeRunsMu.Unlock()

	history := session.RecentHistory(20)

	modelOverride := session.GetConfig().Model
	agent := NewAgentRunWithConfig(a.llmClient, a.toolExecutor, a.config.Agent, a.logger)
	agent.SetModelOverride(modelOverride)

	// Wire block streaming if provided.
	if streamer != nil {
		agent.SetStreamCallback(streamer.StreamCallback())
	}

	if a.usageTracker != nil {
		agent.SetUsageRecorder(func(model string, usage LLMUsage) {
			a.usageTracker.Record(session.ID, model, usage)
		})
	}

	response, usage, err := agent.RunWithUsage(runCtx, systemPrompt, history, userMessage)
	if err != nil {
		if runCtx.Err() != nil {
			return "Agent stopped."
		}
		a.logger.Error("agent failed", "error", err)
		return fmt.Sprintf("Sorry, I encountered an error: %v", err)
	}

	if usage != nil {
		session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens)
	}

	return response
}

// executeAgent runs the agentic loop with tool use support.
// Uses a cancelable context so /stop can abort the run.
func (a *Assistant) executeAgent(ctx context.Context, workspaceID string, session *Session, systemPrompt string, userMessage string) string {
	runKey := workspaceID + ":" + session.ID

	runCtx, cancel := context.WithCancel(ctx)
	defer func() {
		a.activeRunsMu.Lock()
		delete(a.activeRuns, runKey)
		a.activeRunsMu.Unlock()
		cancel()
	}()

	a.activeRunsMu.Lock()
	a.activeRuns[runKey] = cancel
	a.activeRunsMu.Unlock()

	history := session.RecentHistory(20)

	modelOverride := session.GetConfig().Model
	agent := NewAgentRunWithConfig(a.llmClient, a.toolExecutor, a.config.Agent, a.logger)
	agent.SetModelOverride(modelOverride)
	if a.usageTracker != nil {
		agent.SetUsageRecorder(func(model string, usage LLMUsage) {
			a.usageTracker.Record(session.ID, model, usage)
		})
	}

	response, usage, err := agent.RunWithUsage(runCtx, systemPrompt, history, userMessage)
	if err != nil {
		if runCtx.Err() != nil {
			return "Agent stopped."
		}
		a.logger.Error("agent failed", "error", err)
		return fmt.Sprintf("Sorry, I encountered an error: %v", err)
	}

	if usage != nil {
		session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens)
	}

	return response
}

// ToolExecutor returns the tool executor for external tool registration.
func (a *Assistant) ToolExecutor() *ToolExecutor {
	return a.toolExecutor
}

// UsageTracker returns the usage tracker for token/cost stats.
func (a *Assistant) UsageTracker() *UsageTracker {
	return a.usageTracker
}

// Config returns the assistant configuration.
func (a *Assistant) Config() *Config {
	return a.config
}

// LLMClient returns the LLM client (for gateway chat completions).
func (a *Assistant) LLMClient() *LLMClient {
	return a.llmClient
}

// ForceCompactSession runs compaction immediately, returns old and new history length.
func (a *Assistant) ForceCompactSession(session *Session) (oldLen, newLen int) {
	return a.forceCompactSession(session)
}

// SchedulerEnabled returns true if the scheduler is running.
func (a *Assistant) SchedulerEnabled() bool {
	return a.scheduler != nil
}

// MemoryEnabled returns true if the memory store is available.
func (a *Assistant) MemoryEnabled() bool {
	return a.memoryStore != nil
}

// SQLiteMemory returns the SQLite memory store (for advanced search), or nil.
func (a *Assistant) SQLiteMemory() *memory.SQLiteStore {
	return a.sqliteMemory
}

// SessionStore returns the session store (used by CLI chat).
func (a *Assistant) SessionStore() *SessionStore {
	return a.sessionStore
}

// ComposePrompt builds a system prompt for the given session and input.
// Convenience method for CLI and external callers.
func (a *Assistant) ComposePrompt(session *Session, input string) string {
	return a.promptComposer.Compose(session, input)
}

// ExecuteAgent runs the agent loop with tools and returns the response text.
// Public wrapper for CLI and external callers. Uses "default" as workspace ID.
func (a *Assistant) ExecuteAgent(ctx context.Context, systemPrompt string, session *Session, userMessage string) string {
	return a.executeAgent(ctx, "default", session, systemPrompt, userMessage)
}

// StopActiveRun cancels the active agent run for the given workspace and session.
// Returns true if a run was stopped, false if none was active.
func (a *Assistant) StopActiveRun(workspaceID, sessionID string) bool {
	runKey := workspaceID + ":" + sessionID
	a.activeRunsMu.Lock()
	cancel, ok := a.activeRuns[runKey]
	if ok {
		delete(a.activeRuns, runKey)
	}
	a.activeRunsMu.Unlock()
	if ok && cancel != nil {
		cancel()
		return true
	}
	return false
}

// initScheduler creates and configures the scheduler with file-based storage.
func (a *Assistant) initScheduler() {
	storagePath := a.config.Scheduler.Storage
	if storagePath == "" {
		storagePath = "./data/scheduler.json"
	}

	storage, err := scheduler.NewFileJobStorage(storagePath)
	if err != nil {
		a.logger.Error("failed to create scheduler storage", "error", err)
		return
	}

	// Job handler: runs the command as an agent turn.
	handler := func(ctx context.Context, job *scheduler.Job) (string, error) {
		a.logger.Info("scheduler executing job", "id", job.ID, "command", job.Command)

		// Get or create a session for this scheduled job.
		session := a.sessionStore.GetOrCreate("scheduler", job.ID)

		prompt := a.promptComposer.Compose(session, job.Command)

		agent := NewAgentRunWithConfig(a.llmClient, a.toolExecutor, a.config.Agent, a.logger)
		result, err := agent.Run(ctx, prompt, session.RecentHistory(10), job.Command)
		if err != nil {
			return "", err
		}

		// Save to session history.
		session.AddMessage(job.Command, result)

		// If job has a target channel/chat, send the result.
		if job.Channel != "" && job.ChatID != "" {
			outMsg := &channels.OutgoingMessage{Content: result}
			if sendErr := a.channelMgr.Send(ctx, job.Channel, job.ChatID, outMsg); sendErr != nil {
				a.logger.Error("failed to deliver scheduled message",
					"job_id", job.ID, "error", sendErr)
			}
		}

		return result, nil
	}

	a.scheduler = scheduler.New(storage, handler, a.logger)
	a.logger.Info("scheduler initialized", "storage", storagePath)
}

// registerSkillLoaders registers the builtin and clawdhub skill loaders
// in the skill registry based on configuration.
func (a *Assistant) registerSkillLoaders() {
	// Builtin skills loader.
	if len(a.config.Skills.Builtin) > 0 {
		builtinLoader := skills.NewBuiltinLoader(a.config.Skills.Builtin, a.logger)
		a.skillRegistry.AddLoader(builtinLoader)
	}

	// ClawdHub (OpenClaw-compatible) skills loader.
	if len(a.config.Skills.ClawdHubDirs) > 0 {
		clawdHubLoader := skills.NewClawdHubLoader(a.config.Skills.ClawdHubDirs, a.logger)
		a.skillRegistry.AddLoader(clawdHubLoader)
	}
}

// initializeSkills initializes all loaded skills, passing the sandbox runner
// and other configuration via the config map.
func (a *Assistant) initializeSkills() {
	// Create sandbox runner if configured.
	var sandboxRunner *sandbox.Runner
	runner, err := sandbox.NewRunner(a.config.Sandbox, a.logger)
	if err != nil {
		a.logger.Warn("sandbox runner not available", "error", err)
	} else {
		sandboxRunner = runner
	}

	initConfig := map[string]any{}
	if sandboxRunner != nil {
		initConfig["_sandbox_runner"] = sandboxRunner
	}

	allSkills := a.skillRegistry.List()
	for _, meta := range allSkills {
		skill, ok := a.skillRegistry.Get(meta.Name)
		if !ok {
			continue
		}
		if err := skill.Init(a.ctx, initConfig); err != nil {
			a.logger.Warn("skill init failed", "name", meta.Name, "error", err)
		}
	}
}

// registerSkillTools iterates all loaded skills and registers their tools
// in the tool executor so the agent loop can use them.
func (a *Assistant) registerSkillTools() {
	allSkills := a.skillRegistry.List()
	registered := 0

	for _, meta := range allSkills {
		skill, ok := a.skillRegistry.Get(meta.Name)
		if !ok {
			continue
		}

		tools := skill.Tools()
		if len(tools) == 0 {
			continue
		}

		a.toolExecutor.RegisterSkillTools(skill)
		registered += len(tools)
	}

	a.logger.Info("skill tools registered",
		"skills", len(allSkills),
		"tools", registered,
	)
}

// registerSystemTools registers core system tools (web_fetch, exec, file I/O)
// that are always available to the agent regardless of skills configuration.
func (a *Assistant) registerSystemTools() {
	// Create sandbox runner for the exec tool.
	var sandboxRunner *sandbox.Runner
	runner, err := sandbox.NewRunner(a.config.Sandbox, a.logger)
	if err != nil {
		a.logger.Warn("sandbox runner not available for system tools", "error", err)
	} else {
		sandboxRunner = runner
	}

	dataDir := a.config.Memory.Path
	if dataDir == "" {
		dataDir = "./data"
	}
	// Use the parent dir of the memory path as the data directory.
	dataDir = filepath.Dir(dataDir)

	ssrfGuard := security.NewSSRFGuard(a.config.Security.SSRF, a.logger)
	RegisterSystemTools(a.toolExecutor, sandboxRunner, a.memoryStore, a.sqliteMemory, a.config.Memory, a.scheduler, dataDir, ssrfGuard)

	// Register skill creator tools (including install_skill, search_skills, remove_skill).
	skillsDir := "./skills"
	if len(a.config.Skills.ClawdHubDirs) > 0 {
		skillsDir = a.config.Skills.ClawdHubDirs[0]
	}
	RegisterSkillCreatorTools(a.toolExecutor, a.skillRegistry, skillsDir, a.logger)

	// Register subagent tools (spawn, list, wait, stop).
	RegisterSubagentTools(a.toolExecutor, a.subagentMgr, a.llmClient, a.promptComposer, a.logger)

	// Register media tools (describe_image, transcribe_audio).
	RegisterMediaTools(a.toolExecutor, a.llmClient, a.config, a.logger)

	a.logger.Info("system tools registered",
		"tools", a.toolExecutor.ToolNames(),
	)
}

// maybeCompactSession checks if the session history is too large and compacts it.
func (a *Assistant) maybeCompactSession(session *Session) {
	threshold := a.config.Memory.MaxMessages
	if threshold <= 0 {
		threshold = 100
	}

	histLen := session.HistoryLen()

	// Preventive compaction: start at 80% of threshold to avoid hitting
	// the hard limit during active conversation.
	preventiveThreshold := threshold * 80 / 100
	if preventiveThreshold < 10 {
		preventiveThreshold = 10
	}

	if histLen < preventiveThreshold {
		return
	}

	a.logger.Info("preventive compaction triggered",
		"session", session.ID,
		"history_len", histLen,
		"threshold", threshold,
		"preventive_at", preventiveThreshold,
	)

	a.doCompactSession(session)
}

// forceCompactSession runs compaction immediately (used by /compact command).
// Skips threshold check; returns old and new history length.
func (a *Assistant) forceCompactSession(session *Session) (oldLen, newLen int) {
	oldLen = session.HistoryLen()
	if oldLen < 5 {
		return oldLen, oldLen
	}
	a.doCompactSession(session)
	return oldLen, session.HistoryLen()
}

// doCompactSession performs compaction using the configured CompressionStrategy.
//
// Strategies:
//   - "summarize" (default): LLM summarizes old history → single summary entry + recent.
//   - "truncate": simply drops the oldest entries, keeping the most recent.
//   - "sliding": keeps a fixed window of the N most recent entries (no summary).
func (a *Assistant) doCompactSession(session *Session) {
	strategy := a.config.Memory.CompressionStrategy
	if strategy == "" {
		strategy = "summarize"
	}

	a.logger.Info("session compaction",
		"session", session.ID,
		"strategy", strategy,
		"history_len", session.HistoryLen(),
	)

	threshold := a.config.Memory.MaxMessages
	if threshold <= 0 {
		threshold = 100
	}

	switch strategy {
	case "truncate":
		a.compactTruncate(session, threshold)
	case "sliding":
		a.compactSliding(session, threshold)
	default: // "summarize"
		a.compactSummarize(session, threshold)
	}
}

// compactSummarize uses the LLM to generate a summary of older conversation
// and replaces old entries with the summary, keeping recent entries.
func (a *Assistant) compactSummarize(session *Session, threshold int) {
	// Step 1: Memory flush — extract important facts before discarding.
	if a.memoryStore != nil {
		flushPrompt := "Extract the most important facts, preferences, and information from this conversation that should be remembered long-term. Save them using the memory_save tool. If nothing important, reply with NO_REPLY."

		agent := NewAgentRunWithConfig(a.llmClient, a.toolExecutor, a.config.Agent, a.logger)
		systemPrompt := a.promptComposer.Compose(session, flushPrompt)

		flushCtx, cancel := context.WithTimeout(a.ctx, 60*time.Second)
		_, err := agent.Run(flushCtx, systemPrompt, session.RecentHistory(20), flushPrompt)
		cancel()

		if err != nil {
			a.logger.Warn("memory flush failed", "error", err)
		} else {
			a.logger.Info("memory flush completed before compaction")
		}
	}

	// Step 2: LLM summarizes the conversation.
	summaryPrompt := "Summarize the key points of this conversation in 2-3 sentences. Focus on decisions made, tasks completed, and important context."
	summary, err := a.llmClient.Complete(a.ctx, "", session.RecentHistory(20), summaryPrompt)
	if err != nil {
		summary = "Previous conversation context was compacted."
	}

	// Step 3: Keep 25% of threshold as recent history.
	keepRecent := threshold / 4
	if keepRecent < 5 {
		keepRecent = 5
	}

	oldEntries := session.CompactHistory(summary, keepRecent)

	// Step 4: Save the old entries to daily log.
	if a.memoryStore != nil && len(oldEntries) > 0 {
		var logContent strings.Builder
		logContent.WriteString(fmt.Sprintf("### Compacted session: %s\n\n", session.ID))
		logContent.WriteString(fmt.Sprintf("Summary: %s\n\n", summary))
		logContent.WriteString(fmt.Sprintf("Entries compacted: %d\n", len(oldEntries)))

		_ = a.memoryStore.SaveDailyLog(time.Now(), logContent.String())
	}

	a.logger.Info("session compacted (summarize)",
		"session", session.ID,
		"entries_removed", len(oldEntries),
		"new_history_len", session.HistoryLen(),
	)
}

// compactTruncate simply drops the oldest entries, keeping the N most recent.
// No LLM call needed — fast and cost-free.
func (a *Assistant) compactTruncate(session *Session, threshold int) {
	keepRecent := threshold / 2
	if keepRecent < 10 {
		keepRecent = 10
	}

	oldEntries := session.CompactHistory("", keepRecent)

	a.logger.Info("session compacted (truncate)",
		"session", session.ID,
		"entries_removed", len(oldEntries),
		"new_history_len", session.HistoryLen(),
	)
}

// compactSliding keeps a fixed sliding window of the most recent entries.
// Drops everything outside the window — no summary, no LLM call.
func (a *Assistant) compactSliding(session *Session, threshold int) {
	windowSize := threshold / 2
	if windowSize < 10 {
		windowSize = 10
	}

	oldEntries := session.CompactHistory("", windowSize)

	a.logger.Info("session compacted (sliding)",
		"session", session.ID,
		"entries_removed", len(oldEntries),
		"new_history_len", session.HistoryLen(),
	)
}

// enrichMessageContent downloads media when present, describes images via vision API,
// transcribes audio via Whisper, and returns the enriched content for the agent.
// If no media or enrichment fails, returns the original msg.Content.
func (a *Assistant) enrichMessageContent(ctx context.Context, msg *channels.IncomingMessage, logger *slog.Logger) string {
	if msg.Media == nil {
		return msg.Content
	}

	media := a.config.Media.Effective()
	ch, ok := a.channelMgr.Channel(msg.Channel)
	if !ok {
		return msg.Content
	}
	mc, ok := ch.(channels.MediaChannel)
	if !ok {
		return msg.Content
	}

	data, mimeType, err := mc.DownloadMedia(ctx, msg)
	if err != nil {
		logger.Warn("failed to download media", "error", err)
		return msg.Content
	}

	switch msg.Media.Type {
	case channels.MessageImage:
		if !media.VisionEnabled {
			return msg.Content
		}
		if int64(len(data)) > media.MaxImageSize {
			logger.Warn("image too large to process", "size", len(data), "max", media.MaxImageSize)
			return msg.Content
		}
		imgBase64 := base64.StdEncoding.EncodeToString(data)
		if mimeType == "" {
			mimeType = "image/jpeg"
		}
		desc, err := a.llmClient.CompleteWithVision(ctx, "", imgBase64, mimeType, "Describe this image in detail. Include any text visible.", media.VisionDetail)
		if err != nil {
			logger.Warn("vision description failed", "error", err)
			return msg.Content
		}
		logger.Info("image described via vision API", "desc_len", len(desc))
		if msg.Content != "" {
			return fmt.Sprintf("[Image: %s]\n\n%s", desc, msg.Content)
		}
		return fmt.Sprintf("[Image: %s]", desc)

	case channels.MessageAudio:
		if !media.TranscriptionEnabled {
			return msg.Content
		}
		if int64(len(data)) > media.MaxAudioSize {
			logger.Warn("audio too large to process", "size", len(data), "max", media.MaxAudioSize)
			return msg.Content
		}
		filename := msg.Media.Filename
		if filename == "" {
			filename = "audio.ogg"
		}
		transcript, err := a.llmClient.TranscribeAudio(ctx, data, filename, media.TranscriptionModel)
		if err != nil {
			logger.Warn("audio transcription failed", "error", err)
			return msg.Content
		}
		logger.Info("audio transcribed via Whisper", "transcript_len", len(transcript))
		content := msg.Content
		content = strings.ReplaceAll(content, "[audio]", transcript)
		content = strings.ReplaceAll(content, "[voice note]", transcript)
		return content
	}

	return msg.Content
}

// truncate returns the first n characters of s, adding "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// summarizeAndSaveSessionFromHistory uses the LLM to summarize a pre-captured
// history snapshot and saves it to memory/YYYY-MM-DD-slug.md (like OpenClaw's
// session-memory hook). The history must be captured before session.ClearHistory()
// to avoid race conditions.
func (a *Assistant) summarizeAndSaveSessionFromHistory(history []ConversationEntry) {
	if len(history) < 2 {
		return // Too short to summarize.
	}

	// Build a conversation transcript for the LLM.
	var transcript strings.Builder
	for _, entry := range history {
		transcript.WriteString(fmt.Sprintf("User: %s\nAssistant: %s\n\n",
			truncate(entry.UserMessage, 500),
			truncate(entry.AssistantResponse, 1000),
		))
	}

	prompt := `Summarize this conversation in 2-5 bullet points. Focus on key decisions, facts learned, and tasks completed. Be concise. Output only the bullet points, no preamble.

Conversation:
` + transcript.String()

	ctx, cancel := context.WithTimeout(a.ctx, 60*time.Second)
	defer cancel()

	agent := NewAgentRun(a.llmClient, a.toolExecutor, a.logger)
	summary, err := agent.Run(ctx, "You are a conversation summarizer. Output only concise bullet points.", nil, prompt)
	if err != nil {
		a.logger.Warn("session summary generation failed", "error", err)
		return
	}

	// Generate a slug from the first few words of the summary.
	slug := generateSlug(summary, 5)
	now := time.Now()
	filename := fmt.Sprintf("%s-%s.md", now.Format("2006-01-02"), slug)

	// Write to memory directory.
	memDir := filepath.Join(filepath.Dir(a.config.Memory.Path), "memory")
	_ = os.MkdirAll(memDir, 0o755)

	content := fmt.Sprintf("# Session Summary — %s\n\n%s\n",
		now.Format("2006-01-02 15:04"), summary)

	filePath := filepath.Join(memDir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		a.logger.Warn("failed to save session summary", "path", filePath, "error", err)
		return
	}

	a.logger.Info("session summary saved", "path", filePath)

	// Re-index if SQLite memory is available.
	if a.sqliteMemory != nil && a.config.Memory.Index.Auto {
		chunkCfg := memory.ChunkConfig{MaxTokens: a.config.Memory.Index.ChunkMaxTokens, Overlap: 100}
		if chunkCfg.MaxTokens <= 0 {
			chunkCfg.MaxTokens = 500
		}
		_ = a.sqliteMemory.IndexMemoryDir(a.ctx, memDir, chunkCfg)
	}
}

// generateSlug creates a URL-safe slug from the first n words of text.
func generateSlug(text string, maxWords int) string {
	words := strings.Fields(text)
	if len(words) > maxWords {
		words = words[:maxWords]
	}
	slug := strings.Join(words, "-")
	slug = strings.ToLower(slug)

	// Keep only alphanumeric and hyphens.
	var clean strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			clean.WriteRune(r)
		}
	}

	result := clean.String()
	if len(result) > 40 {
		result = result[:40]
	}
	if result == "" {
		result = "session"
	}
	return strings.TrimRight(result, "-")
}

// sendReply sends a response to the original message's channel.
// Long messages are split into chunks respecting the channel limit (default 4000 chars).
func (a *Assistant) sendReply(original *channels.IncomingMessage, content string) {
	content = FormatForChannel(content, original.Channel)

	maxLen := MaxMessageDefault
	// Could be per-channel configurable later (e.g. WhatsApp: MaxMessageWhatsApp)

	chunks := SplitMessage(content, maxLen)
	if chunks == nil {
		chunks = []string{content}
	}
	for _, chunk := range chunks {
		outMsg := &channels.OutgoingMessage{
			Content: chunk,
			ReplyTo: original.ID,
		}
		if err := a.channelMgr.Send(a.ctx, original.Channel, original.ChatID, outMsg); err != nil {
			a.logger.Error("failed to send reply chunk",
				"channel", original.Channel,
				"chat_id", original.ChatID,
				"error", err,
			)
		}
	}
}


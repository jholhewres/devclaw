package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// ── Registrar Interfaces (avoid circular imports with copilot) ──

// ToolRegistrar registers and unregisters tools with the executor.
type ToolRegistrar interface {
	RegisterPluginTool(reg ToolRegistration)
	UnregisterTool(name string) bool
}

// ToolRegistration describes a tool to register with the executor.
type ToolRegistration struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Hidden      bool
	Permission  string
	Handler     func(ctx context.Context, args map[string]any) (any, error)
}

// HookRegistrar registers hooks with the hook system.
type HookRegistrar interface {
	RegisterPluginHook(pluginID string, events []string, priority int, handler func(event string, data map[string]any)) error
}

// ChannelRegistrar registers channels with the channel manager.
type ChannelRegistrar interface {
	Register(ch any) error
}

// SkillRegistrar registers skills.
type SkillRegistrar interface {
	Register(name, description, skillMDPath string) error
}

// ── Registry ──

// Registry is the central plugin registry that coordinates registration
// of all plugin-provided components (tools, hooks, skills, channels, agents).
type Registry struct {
	plugins map[string]*PluginInstance
	logger  *slog.Logger
	mu      sync.RWMutex

	toolRegistrar    ToolRegistrar
	hookRegistrar    HookRegistrar
	channelRegistrar ChannelRegistrar
	skillRegistrar   SkillRegistrar

	// Agent runtime (Sprint 2).
	followupEnqueuer FollowupEnqueuer
	agentRegistrar   AgentRegistrar
	agentIndex       map[string]*resolvedAgent // "pluginID/agentID" -> resolved agent
}

// NewRegistry creates a new plugin registry.
func NewRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		plugins:    make(map[string]*PluginInstance),
		agentIndex: make(map[string]*resolvedAgent),
		logger:     logger.With("component", "plugin-registry"),
	}
}

// SetToolRegistrar wires the tool registrar (typically the ToolExecutor).
func (r *Registry) SetToolRegistrar(tr ToolRegistrar) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toolRegistrar = tr
}

// SetHookRegistrar wires the hook registrar.
func (r *Registry) SetHookRegistrar(hr HookRegistrar) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hookRegistrar = hr
}

// SetChannelRegistrar wires the channel registrar.
func (r *Registry) SetChannelRegistrar(cr ChannelRegistrar) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channelRegistrar = cr
}

// SetSkillRegistrar wires the skill registrar.
func (r *Registry) SetSkillRegistrar(sr SkillRegistrar) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skillRegistrar = sr
}

// AddLoadedPlugins imports all loaded plugins from the Loader into the registry.
func (r *Registry) AddLoadedPlugins(loader *Loader) {
	// Fetch from loader outside registry lock to avoid lock-ordering dependency.
	instances := loader.All()

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, inst := range instances {
		if inst.State == StateLoaded && inst.Enabled {
			r.plugins[inst.Manifest.ID] = inst
		}
	}
}

// RegisterAll registers tools, hooks, skills, and channels from all loaded plugins.
func (r *Registry) RegisterAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, inst := range r.plugins {
		if inst.State != StateLoaded {
			continue
		}

		if err := r.registerPlugin(inst); err != nil {
			r.logger.Error("plugins: registration failed",
				"id", id, "error", err)
			inst.State = StateError
			inst.ErrorMsg = fmt.Sprintf("registration: %v", err)
			continue
		}

		inst.State = StateRegistered
		r.logger.Info("plugins: registered",
			"id", id,
			"tools", len(inst.RegisteredTools),
			"hooks", len(inst.RegisteredHooks),
			"agents", len(inst.RegisteredAgents),
			"skills", len(inst.RegisteredSkills),
		)
	}

	return nil
}

// registerPlugin registers all components of a single plugin.
// Must be called under r.mu lock.
func (r *Registry) registerPlugin(inst *PluginInstance) error {
	manifest := inst.Manifest

	// Register tools.
	if r.toolRegistrar != nil {
		for _, toolDef := range manifest.Tools {
			handler, err := BuildToolHandler(toolDef, inst)
			if err != nil {
				return fmt.Errorf("tool %q: %w", toolDef.Name, err)
			}

			// Namespace tool: pluginID_toolName.
			namespacedName := manifest.ID + "_" + toolDef.Name

			var params json.RawMessage
			if toolDef.Parameters != nil {
				var err error
				params, err = json.Marshal(toolDef.Parameters)
				if err != nil {
					return fmt.Errorf("tool %q: marshaling parameters: %w", toolDef.Name, err)
				}
			} else {
				params = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
			}

			r.toolRegistrar.RegisterPluginTool(ToolRegistration{
				Name:        namespacedName,
				Description: toolDef.Description,
				Parameters:  params,
				Hidden:      toolDef.Hidden,
				Permission:  toolDef.Permission,
				Handler:     handler,
			})

			inst.RegisteredTools = append(inst.RegisteredTools, namespacedName)
		}
	}

	// Register hooks.
	if r.hookRegistrar != nil {
		for _, hookDef := range manifest.Hooks {
			priority := hookDef.Priority
			if priority == 0 {
				priority = 100
			}

			var handler func(event string, data map[string]any)
			if hookDef.Script != "" {
				scriptHandler := buildScriptHandler(ToolDef{Script: hookDef.Script}, inst.Dir)
				handler = func(event string, data map[string]any) {
					data["event"] = event
					if _, err := scriptHandler(context.Background(), data); err != nil {
						r.logger.Warn("plugins: hook script error",
							"plugin", manifest.ID,
							"hook", hookDef.Name,
							"event", event,
							"error", err)
					}
				}
			}

			if handler == nil {
				continue
			}

			if err := r.hookRegistrar.RegisterPluginHook(manifest.ID, hookDef.Events, priority, handler); err != nil {
				return fmt.Errorf("hook %q: %w", hookDef.Name, err)
			}

			inst.RegisteredHooks = append(inst.RegisteredHooks, hookDef.Name)
		}
	}

	// Register skills.
	if r.skillRegistrar != nil {
		for _, skillDef := range manifest.Skills {
			skillPath := ""
			if skillDef.SkillMD != "" {
				skillPath = filepath.Join(inst.Dir, skillDef.SkillMD)
				if err := validatePathWithinDir(skillPath, inst.Dir); err != nil {
					return fmt.Errorf("skill %q: %w", skillDef.Name, err)
				}
			}
			if err := r.skillRegistrar.Register(skillDef.Name, skillDef.Description, skillPath); err != nil {
				r.logger.Warn("plugins: skill registration failed",
					"plugin", manifest.ID,
					"skill", skillDef.Name,
					"error", err)
			}
			inst.RegisteredSkills = append(inst.RegisteredSkills, skillDef.Name)
		}
	}

	// Resolve and index agents.
	for _, agentDef := range manifest.Agents {
		prompt, err := resolveInstructionsPath(agentDef.Instructions, inst.Dir)
		if err != nil {
			r.logger.Warn("plugins: failed to resolve agent instructions",
				"plugin", manifest.ID,
				"agent", agentDef.ID,
				"error", err)
			prompt = agentDef.Instructions // Fall back to inline.
		}

		key := manifest.ID + "/" + agentDef.ID
		r.agentIndex[key] = &resolvedAgent{
			AgentDef:     agentDef,
			pluginID:     manifest.ID,
			systemPrompt: prompt,
		}

		inst.RegisteredAgents = append(inst.RegisteredAgents, agentDef.ID)
	}

	return nil
}

// StartAll starts services for all registered plugins.
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, inst := range r.plugins {
		if inst.State != StateRegistered {
			continue
		}

		// Start service context.
		svcCtx, cancel := context.WithCancel(ctx)
		inst.serviceCtx = svcCtx
		inst.serviceCancel = cancel

		inst.State = StateStarted
		r.logger.Info("plugins: started", "id", id)
	}

	return nil
}

// StopAll stops services for all started plugins in reverse registration order.
func (r *Registry) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Collect started plugin IDs and reverse them so the last-registered
	// plugin is stopped first.
	ids := make([]string, 0, len(r.plugins))
	for id, inst := range r.plugins {
		if inst.State == StateStarted {
			ids = append(ids, id)
		}
	}
	slices.Reverse(ids)

	for _, id := range ids {
		inst := r.plugins[id]

		if inst.serviceCancel != nil {
			inst.serviceCancel()
		}

		// Unregister tools.
		if r.toolRegistrar != nil {
			for _, toolName := range inst.RegisteredTools {
				r.toolRegistrar.UnregisterTool(toolName)
			}
		}

		inst.State = StateStopped
		r.logger.Info("plugins: stopped", "id", id)
	}
}

// Get returns a plugin instance by ID.
func (r *Registry) Get(id string) *PluginInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.plugins[id]
}

// List returns info for all plugins.
func (r *Registry) List() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(r.plugins))
	for _, inst := range r.plugins {
		infos = append(infos, inst.Info())
	}
	return infos
}

// HasPlugins returns true if any plugins are loaded.
func (r *Registry) HasPlugins() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins) > 0
}

// ConfigurePlugin updates the resolved config for a plugin.
// Secret fields sent as the redaction placeholder are preserved from the original.
func (r *Registry) ConfigurePlugin(id string, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, ok := r.plugins[id]
	if !ok {
		return fmt.Errorf("plugin %q not found", id)
	}
	if inst.Config == nil {
		inst.Config = make(map[string]any)
	}

	// Build set of secret field keys.
	secrets := make(map[string]bool)
	if inst.Manifest.Config != nil {
		for _, f := range inst.Manifest.Config.Fields {
			if f.Type == "secret" {
				secrets[f.Key] = true
			}
		}
	}

	for k, v := range updates {
		// Skip redacted placeholder — keep original secret value.
		if secrets[k] {
			if s, ok := v.(string); ok && s == "••••••••" {
				continue
			}
		}
		inst.Config[k] = v
	}

	return nil
}

// Enable enables a plugin by ID.
func (r *Registry) Enable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, ok := r.plugins[id]
	if !ok {
		return fmt.Errorf("plugin %q not found", id)
	}
	inst.Enabled = true
	return nil
}

// Disable disables a plugin by ID.
func (r *Registry) Disable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, ok := r.plugins[id]
	if !ok {
		return fmt.Errorf("plugin %q not found", id)
	}
	inst.Enabled = false
	return nil
}

// ── Agent Runtime (Sprint 2) ──

// SubagentSpawner spawns subagents with custom executors and prompts.
type SubagentSpawner interface {
	SpawnPluginAgent(ctx context.Context, executor any, llmClient any, prompt, task string, params any) (string, error)
}

// FollowupEnqueuer injects messages into a session's follow-up queue.
type FollowupEnqueuer interface {
	Enqueue(sessionID, content, channel, chatID string)
}

// AgentRegistrar registers agent profiles at runtime.
type AgentRegistrar interface {
	AddProfile(p any)
}

// resolvedAgent holds a fully resolved agent definition with loaded instructions.
type resolvedAgent struct {
	AgentDef
	pluginID     string
	systemPrompt string // resolved from .md file or inline
}

// TriggerMatch represents a matched trigger for a plugin agent.
type TriggerMatch struct {
	PluginID string
	AgentID  string
	Score    float64
}

// SetFollowupEnqueuer wires the follow-up message enqueuer.
func (r *Registry) SetFollowupEnqueuer(fe FollowupEnqueuer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.followupEnqueuer = fe
}

// SetAgentRegistrar wires the agent registrar.
func (r *Registry) SetAgentRegistrar(ar AgentRegistrar) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agentRegistrar = ar
}

// MatchTrigger evaluates all plugin agent triggers against message content and channel.
// Returns the best match or nil if no triggers fire.
func (r *Registry) MatchTrigger(content, channel string) *TriggerMatch {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.agentIndex) == 0 {
		return nil
	}

	contentLower := strings.ToLower(content)
	var bestMatch *TriggerMatch

	for key, agent := range r.agentIndex {
		// Check channel filter.
		if len(agent.Channels) > 0 && !slices.ContainsFunc(agent.Channels, func(ch string) bool {
			return strings.EqualFold(ch, channel)
		}) {
			continue
		}

		// Check triggers.
		for _, trigger := range agent.Triggers {
			triggerLower := strings.ToLower(trigger)
			if strings.Contains(contentLower, triggerLower) {
				score := float64(len(trigger)) / float64(len(content)+1)
				if bestMatch == nil || score > bestMatch.Score {
					parts := strings.SplitN(key, "/", 2)
					bestMatch = &TriggerMatch{
						PluginID: parts[0],
						AgentID:  agent.ID,
						Score:    score,
					}
				}
			}
		}
	}

	// Require minimum score threshold to avoid false positives.
	if bestMatch != nil && bestMatch.Score < 0.01 {
		return nil
	}

	return bestMatch
}

// GetResolvedAgent returns the resolved agent for a given plugin/agent ID pair.
func (r *Registry) GetResolvedAgent(pluginID, agentID string) *resolvedAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agentIndex[pluginID+"/"+agentID]
}

// ResolvedAgentDef returns the AgentDef for a resolved agent (exported accessor).
func (ra *resolvedAgent) ResolvedAgentDef() AgentDef {
	return ra.AgentDef
}

// ResolvedSystemPrompt returns the resolved system prompt.
func (ra *resolvedAgent) ResolvedSystemPrompt() string {
	return ra.systemPrompt
}

// ResolvedPluginID returns the plugin ID.
func (ra *resolvedAgent) ResolvedPluginID() string {
	return ra.pluginID
}

// SendToMainAgent sends an escalation message to the main agent's session.
func (r *Registry) SendToMainAgent(parentSessionID string, agent *resolvedAgent, reason, summary string) {
	r.mu.RLock()
	fe := r.followupEnqueuer
	r.mu.RUnlock()

	if fe == nil {
		r.logger.Warn("plugins: cannot escalate — no followup enqueuer set")
		return
	}

	msg := fmt.Sprintf("[Escalation from plugin:%s agent:%s]\nReason: %s\nContext: %s",
		agent.pluginID, agent.ID, reason, summary)

	fe.Enqueue(parentSessionID, msg, "", "")
}

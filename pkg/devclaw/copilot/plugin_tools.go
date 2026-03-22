package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/plugins"
)

// RegisterPluginManagementTools registers tools for managing plugins at runtime.
func RegisterPluginManagementTools(executor *ToolExecutor, registry *plugins.Registry) {
	// plugin_list — list all loaded plugins.
	executor.RegisterHidden(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "plugin_list",
			Description: "List all loaded plugins with their status, tools, agents, and capabilities.",
			Parameters: mustJSON(map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			}),
		},
	}, func(_ context.Context, _ map[string]any) (any, error) {
		infos := registry.List()
		if len(infos) == 0 {
			return "No plugins loaded.", nil
		}
		data, _ := json.MarshalIndent(infos, "", "  ")
		return string(data), nil
	})

	// plugin_info — get detailed info about a specific plugin.
	executor.RegisterHidden(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "plugin_info",
			Description: "Get detailed information about a specific plugin by ID.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Plugin ID"},
				},
				"required":             []string{"id"},
				"additionalProperties": false,
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("plugin id is required")
		}
		inst := registry.Get(id)
		if inst == nil {
			return nil, fmt.Errorf("plugin %q not found", id)
		}
		data, _ := json.MarshalIndent(inst.Info(), "", "  ")
		return string(data), nil
	})

	// plugin_enable — enable a plugin.
	executor.RegisterHidden(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "plugin_enable",
			Description: "Enable a disabled plugin by ID.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Plugin ID"},
				},
				"required":             []string{"id"},
				"additionalProperties": false,
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		if err := registry.Enable(id); err != nil {
			return nil, err
		}
		return fmt.Sprintf("Plugin %q enabled.", id), nil
	})

	// plugin_disable — disable a plugin.
	executor.RegisterHidden(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "plugin_disable",
			Description: "Disable a plugin by ID. Its tools and hooks will be unregistered.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Plugin ID"},
				},
				"required":             []string{"id"},
				"additionalProperties": false,
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		if err := registry.Disable(id); err != nil {
			return nil, err
		}
		return fmt.Sprintf("Plugin %q disabled.", id), nil
	})

	// delegate_to_plugin_agent — delegate a task to a plugin agent.
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "delegate_to_plugin_agent",
			Description: "Delegate a task to a plugin agent. The agent runs with its own tools and instructions.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"plugin": map[string]any{"type": "string", "description": "Plugin ID"},
					"agent":  map[string]any{"type": "string", "description": "Agent ID within the plugin"},
					"task":   map[string]any{"type": "string", "description": "Task description for the agent"},
					"wait":   map[string]any{"type": "boolean", "description": "If true, wait for result. If false, return immediately.", "default": true},
				},
				"required":             []string{"plugin", "agent", "task"},
				"additionalProperties": false,
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		pluginID, _ := args["plugin"].(string)
		agentID, _ := args["agent"].(string)

		resolved := registry.GetResolvedAgent(pluginID, agentID)
		if resolved == nil {
			return nil, fmt.Errorf("plugin agent %s/%s not found", pluginID, agentID)
		}

		// This stub is replaced by RegisterPluginAgentDelegation when the subagent manager is available.
		return nil, fmt.Errorf("plugin agent delegation not available: subagent manager not initialized")
	})
}

// buildEscalationChecker creates an escalation checker function from config.
func buildEscalationChecker(keywords []string, maxTurns int) func(int, string) *EscalationSignal {
	return func(turn int, lastResponse string) *EscalationSignal {
		// Check max turns.
		if maxTurns > 0 && turn >= maxTurns {
			return &EscalationSignal{
				Reason:  fmt.Sprintf("reached max turns (%d)", maxTurns),
				Summary: lastResponse,
			}
		}

		// Check keywords.
		responseLower := strings.ToLower(lastResponse)
		for _, kw := range keywords {
			if strings.Contains(responseLower, strings.ToLower(kw)) {
				return &EscalationSignal{
					Reason:  fmt.Sprintf("keyword match: %q", kw),
					Summary: lastResponse,
				}
			}
		}

		return nil
	}
}

// RegisterPluginAgentDelegation registers the full plugin agent delegation
// with access to the SubagentManager and LLMClient. Called after Start().
func RegisterPluginAgentDelegation(
	executor *ToolExecutor,
	registry *plugins.Registry,
	subagentMgr *SubagentManager,
	llmClient *LLMClient,
) {
	if subagentMgr == nil || llmClient == nil {
		return
	}

	// Override the delegate_to_plugin_agent with the real implementation.
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "delegate_to_plugin_agent",
			Description: "Delegate a task to a plugin agent. The agent runs with its own tools and instructions.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"plugin": map[string]any{"type": "string", "description": "Plugin ID"},
					"agent":  map[string]any{"type": "string", "description": "Agent ID within the plugin"},
					"task":   map[string]any{"type": "string", "description": "Task description for the agent"},
					"wait":   map[string]any{"type": "boolean", "description": "If true, wait for result (blocking). If false, return immediately.", "default": true},
				},
				"required":             []string{"plugin", "agent", "task"},
				"additionalProperties": false,
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		pluginID, _ := args["plugin"].(string)
		agentID, _ := args["agent"].(string)
		task, _ := args["task"].(string)
		wait := true
		if w, ok := args["wait"].(bool); ok {
			wait = w
		}

		resolved := registry.GetResolvedAgent(pluginID, agentID)
		if resolved == nil {
			return nil, fmt.Errorf("plugin agent %s/%s not found", pluginID, agentID)
		}

		agentDef := resolved.ResolvedAgentDef()

		// Build executor with agent's tool profile.
		childExecutor := subagentMgr.CreateChildExecutorWithProfile(
			executor, 1,
			agentDef.Tools.Allow, agentDef.Tools.Deny,
		)

		// Inject escalate_to_main tool.
		sessionID := SessionIDFromContext(ctx)
		registerEscalateToMainTool(childExecutor)

		// Build system prompt.
		prompt := resolved.ResolvedSystemPrompt()
		if prompt == "" {
			prompt = fmt.Sprintf("You are %s. %s", agentDef.Name, agentDef.Description)
		}

		// Build escalation checker.
		var escalationChecker func(int, string) *EscalationSignal
		if esc := agentDef.Escalation; esc != nil && esc.Enabled && !esc.ExplicitOnly {
			escalationChecker = buildEscalationChecker(esc.Keywords, esc.MaxTurns)
		}

		// Resolve model — copy all value fields (except mutex) and override model.
		childLLM := llmClient
		if agentDef.Model != "" && agentDef.Model != llmClient.model {
			childLLM = &LLMClient{
				baseURL:           llmClient.baseURL,
				provider:          llmClient.provider,
				apiKey:            llmClient.apiKey,
				model:             agentDef.Model,
				fallback:          llmClient.fallback,
				params:            llmClient.params,
				httpClient:        llmClient.httpClient,
				logger:            llmClient.logger,
				oauthTokenManager: llmClient.oauthTokenManager,
				failoverCoord:     llmClient.failoverCoord,
			}
		}

		delivery := DeliveryTargetFromContext(ctx)
		params := SpawnParams{
			Task:              task,
			Label:             fmt.Sprintf("plugin:%s/%s", pluginID, agentID),
			ParentSessionID:   sessionID,
			OriginChannel:     delivery.Channel,
			OriginTo:          delivery.ChatID,
			MaxTurns:          agentDef.MaxTurns,
			EscalationChecker: escalationChecker,
		}
		if agentDef.TimeoutSec > 0 {
			params.TimeoutSeconds = agentDef.TimeoutSec
		}

		run, err := subagentMgr.SpawnWithExecutor(ctx, params, childLLM, childExecutor, prompt)
		if err != nil {
			return nil, fmt.Errorf("spawn plugin agent: %w", err)
		}

		if !wait {
			return fmt.Sprintf("Plugin agent %s/%s spawned (run: %s). Task: %s",
				pluginID, agentID, run.ID, task), nil
		}

		// Wait for completion using the agent's configured timeout, or 15min default.
		waitTimeout := 15 * time.Minute
		if agentDef.TimeoutSec > 0 {
			waitTimeout = time.Duration(agentDef.TimeoutSec) * time.Second
		}
		select {
		case <-run.Done():
		case <-time.After(waitTimeout):
			return nil, fmt.Errorf("plugin agent %s/%s timed out after %s", pluginID, agentID, waitTimeout)
		}

		if run.Status == SubagentStatusFailed {
			// Check if it was an escalation.
			if strings.Contains(run.Error, "escalation:") {
				registry.SendToMainAgent(sessionID, resolved, run.Error, run.Result)
				return fmt.Sprintf("Plugin agent escalated to main agent: %s", run.Error), nil
			}
			return nil, fmt.Errorf("plugin agent failed: %s", run.Error)
		}

		return run.Result, nil
	})
}

// registerEscalateToMainTool injects the escalate_to_main tool into a child executor.
func registerEscalateToMainTool(executor *ToolExecutor) {
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "escalate_to_main",
			Description: "Escalate the current task to the main agent. Use when you cannot handle the request or the user asks to talk to the main agent.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"reason":  map[string]any{"type": "string", "description": "Why you are escalating"},
					"summary": map[string]any{"type": "string", "description": "Summary of what you've done so far"},
				},
				"required":             []string{"reason"},
				"additionalProperties": false,
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		reason, _ := args["reason"].(string)
		summary, _ := args["summary"].(string)

		// Trigger escalation by returning an error that the agent loop will catch.
		return nil, &ErrEscalation{Signal: &EscalationSignal{
			Reason:  reason,
			Summary: summary,
		}}
	})
}

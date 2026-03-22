// Package copilot – agent_tools.go registers the agent_manage dispatcher tool
// for creating and managing agents (workspaces) via the AI.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RegisterAgentTools registers the agent_manage dispatcher tool.
func RegisterAgentTools(executor *ToolExecutor, wsMgr *WorkspaceManager) {
	if wsMgr == nil {
		return
	}

	executor.RegisterHidden(
		MakeToolDefinition("agent_manage",
			"Manage agents (isolated assistant profiles). Actions: create, list, get, update, delete, set_default. Each agent has its own instructions, model, skills, sessions, and routing.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"create", "list", "get", "update", "delete", "set_default"},
						"description": "Action to perform",
					},
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Agent ID (required for get/update/delete/set_default)",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Agent display name (for create/update)",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Short description (for create/update)",
					},
					"model": map[string]any{
						"type":        "string",
						"description": "LLM model override (for create/update)",
					},
					"instructions": map[string]any{
						"type":        "string",
						"description": "System prompt (for create/update)",
					},
					"emoji": map[string]any{
						"type":        "string",
						"description": "Identity emoji name (for create/update)",
					},
					"channels": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Channels to route to this agent (for create/update)",
					},
					"skills": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Enabled skill names (for create/update)",
					},
					"tool_profile": map[string]any{
						"type":        "string",
						"description": "Tool profile: minimal, coding, messaging, full (for create/update)",
					},
					"active": map[string]any{
						"type":        "boolean",
						"description": "Enable/disable (for update)",
					},
				},
				"required": []string{"action"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			switch action {
			case "create":
				return handleAgentCreate(wsMgr, args)
			case "list":
				return handleAgentList(wsMgr)
			case "get":
				return handleAgentGet(wsMgr, args)
			case "update":
				return handleAgentUpdate(wsMgr, args)
			case "delete":
				return handleAgentDelete(wsMgr, args)
			case "set_default":
				return handleAgentSetDefault(wsMgr, args)
			default:
				return nil, fmt.Errorf("unknown action: %s (valid: create, list, get, update, delete, set_default)", action)
			}
		},
	)
}

func handleAgentCreate(wsMgr *WorkspaceManager, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required for create")
	}

	// Generate slug ID from name
	id := Slugify(name)
	if id == "" {
		return nil, fmt.Errorf("could not generate ID from name %q", name)
	}

	ws := Workspace{
		ID:     id,
		Name:   name,
		Active: true,
		Source: "ai",
	}

	if desc, ok := args["description"].(string); ok && desc != "" {
		ws.Description = desc
	}
	if model, ok := args["model"].(string); ok && model != "" {
		ws.Model = model
	}
	if instr, ok := args["instructions"].(string); ok && instr != "" {
		ws.Instructions = instr
	}
	if emoji, ok := args["emoji"].(string); ok && emoji != "" {
		ws.Identity = &IdentityConfig{Emoji: emoji}
	}
	if channels, ok := args["channels"].([]any); ok {
		for _, ch := range channels {
			if s, ok := ch.(string); ok {
				ws.Channels = append(ws.Channels, s)
			}
		}
	}
	if skills, ok := args["skills"].([]any); ok {
		for _, sk := range skills {
			if s, ok := sk.(string); ok {
				ws.Skills = append(ws.Skills, s)
			}
		}
	}
	if tp, ok := args["tool_profile"].(string); ok && tp != "" {
		ws.ToolProfile = tp
	}

	if err := wsMgr.Create(ws, "ai"); err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	return fmt.Sprintf("Agent created: **%s** (ID: `%s`)", name, id), nil
}

func handleAgentList(wsMgr *WorkspaceManager) (any, error) {
	workspaces := wsMgr.List()
	if len(workspaces) == 0 {
		return "No agents configured.", nil
	}

	type agentSummary struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Model       string   `json:"model,omitempty"`
		Channels    []string `json:"channels,omitempty"`
		Active      bool     `json:"active"`
		Default     bool     `json:"default"`
		Source      string   `json:"source,omitempty"`
		MemberCount int      `json:"member_count"`
		Sessions    int      `json:"session_count"`
	}

	summaries := make([]agentSummary, 0, len(workspaces))
	for _, ws := range workspaces {
		summaries = append(summaries, agentSummary{
			ID:          ws.ID,
			Name:        ws.Name,
			Model:       ws.Model,
			Channels:    ws.Channels,
			Active:      ws.Active,
			Default:     ws.Default,
			Source:      ws.Source,
			MemberCount: len(ws.Members),
			Sessions:    wsMgr.SessionCountForWorkspace(ws.ID),
		})
	}

	data, _ := json.MarshalIndent(summaries, "", "  ")
	return string(data), nil
}

func handleAgentGet(wsMgr *WorkspaceManager, args map[string]any) (any, error) {
	id, _ := args["agent_id"].(string)
	if id == "" {
		return nil, fmt.Errorf("agent_id is required for get")
	}

	ws, ok := wsMgr.Get(id)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", id)
	}

	data, _ := json.MarshalIndent(ws, "", "  ")
	return string(data), nil
}

func handleAgentUpdate(wsMgr *WorkspaceManager, args map[string]any) (any, error) {
	id, _ := args["agent_id"].(string)
	if id == "" {
		return nil, fmt.Errorf("agent_id is required for update")
	}

	err := wsMgr.Update(id, func(ws *Workspace) {
		if name, ok := args["name"].(string); ok && name != "" {
			ws.Name = name
		}
		if desc, ok := args["description"].(string); ok {
			ws.Description = desc
		}
		if model, ok := args["model"].(string); ok {
			ws.Model = model
		}
		if instr, ok := args["instructions"].(string); ok {
			ws.Instructions = instr
		}
		if emoji, ok := args["emoji"].(string); ok && emoji != "" {
			if ws.Identity == nil {
				ws.Identity = &IdentityConfig{}
			}
			ws.Identity.Emoji = emoji
		}
		if channels, ok := args["channels"].([]any); ok {
			ws.Channels = nil
			for _, ch := range channels {
				if s, ok := ch.(string); ok {
					ws.Channels = append(ws.Channels, s)
				}
			}
		}
		if skills, ok := args["skills"].([]any); ok {
			ws.Skills = nil
			for _, sk := range skills {
				if s, ok := sk.(string); ok {
					ws.Skills = append(ws.Skills, s)
				}
			}
		}
		if tp, ok := args["tool_profile"].(string); ok {
			ws.ToolProfile = tp
		}
		if active, ok := args["active"].(bool); ok {
			ws.Active = active
		}
	})
	if err != nil {
		return nil, fmt.Errorf("update agent: %w", err)
	}

	// Rebuild routing maps after update
	wsMgr.RebuildMaps()

	return fmt.Sprintf("Agent `%s` updated.", id), nil
}

func handleAgentDelete(wsMgr *WorkspaceManager, args map[string]any) (any, error) {
	id, _ := args["agent_id"].(string)
	if id == "" {
		return nil, fmt.Errorf("agent_id is required for delete")
	}

	if err := wsMgr.Delete(id, "ai"); err != nil {
		return nil, fmt.Errorf("delete agent: %w", err)
	}

	return fmt.Sprintf("Agent `%s` deleted.", id), nil
}

func handleAgentSetDefault(wsMgr *WorkspaceManager, args map[string]any) (any, error) {
	id, _ := args["agent_id"].(string)
	if id == "" {
		return nil, fmt.Errorf("agent_id is required for set_default")
	}

	if err := wsMgr.SetDefault(id); err != nil {
		return nil, fmt.Errorf("set default: %w", err)
	}

	return fmt.Sprintf("Agent `%s` is now the default.", id), nil
}

// Slugify converts a name to a URL-friendly slug (lowercase, hyphens).
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '_' || r == '-' {
			return '-'
		}
		return -1
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

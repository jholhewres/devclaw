// Package copilot – agent_tools.go registers the agent_manage dispatcher tool
// for creating and managing agents (workspaces) via the AI.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/paths"
)

// RegisterAgentTools registers the agent_manage dispatcher tool.
func RegisterAgentTools(executor *ToolExecutor, wsMgr *WorkspaceManager) {
	if wsMgr == nil {
		return
	}

	executor.Register(
		MakeToolDefinition("agent_manage",
			"Manage agents (isolated assistant profiles). Actions: create, list, get, update, delete, set_default, sessions. Each agent has its own instructions, model, skills, sessions, and routing.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"create", "list", "get", "update", "delete", "set_default", "sessions"},
						"description": "Action to perform",
					},
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Agent ID or display name — resolved automatically (required for get/update/delete/set_default/sessions)",
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
					"soul": map[string]any{
						"type":        "string",
						"description": "Agent personality/persona definition (written to SOUL.md in workspace)",
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
			case "sessions":
				return handleAgentSessions(wsMgr, args)
			default:
				return nil, fmt.Errorf("unknown action: %s (valid: create, list, get, update, delete, set_default, sessions)", action)
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
	if soul, ok := args["soul"].(string); ok && soul != "" {
		ws.Soul = soul
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
		FileBacked  bool     `json:"file_backed"`
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
			FileBacked:  ws.ID != wsMgr.DefaultID() && hasWorkspaceDir(ws.ID),
			MemberCount: len(ws.Members),
			Sessions:    wsMgr.SessionCountForWorkspace(ws.ID),
		})
	}

	data, _ := json.MarshalIndent(summaries, "", "  ")
	return string(data), nil
}

// resolveAgent looks up an agent by ID or display name and returns the
// canonical workspace. This allows users to refer to agents by name
// (e.g. "@agentdev" finds "AgentDev" with ID "tester").
func resolveAgent(wsMgr *WorkspaceManager, args map[string]any, field string) (*Workspace, error) {
	input, _ := args[field].(string)
	if input == "" {
		return nil, fmt.Errorf("%s is required", field)
	}
	ws, ok := wsMgr.ResolveByNameOrID(input)
	if !ok {
		return nil, fmt.Errorf("agent %q not found (searched by ID and name)", input)
	}
	return ws, nil
}

func handleAgentGet(wsMgr *WorkspaceManager, args map[string]any) (any, error) {
	ws, err := resolveAgent(wsMgr, args, "agent_id")
	if err != nil {
		return nil, err
	}

	type agentDetail struct {
		*Workspace
		WorkspaceDir   string            `json:"workspace_dir,omitempty"`
		FileBacked     bool              `json:"file_backed"`
		WorkspaceFiles map[string]string `json:"workspace_files,omitempty"`
	}

	detail := agentDetail{Workspace: ws}
	if ws.ID != wsMgr.DefaultID() && hasWorkspaceDir(ws.ID) {
		wsDir := paths.ResolveWorkspaceDir(ws.ID)
		detail.WorkspaceDir = wsDir
		detail.FileBacked = true
		detail.WorkspaceFiles = readWorkspaceFiles(wsDir)
	}
	data, _ := json.MarshalIndent(detail, "", "  ")
	return string(data), nil
}

func readWorkspaceFiles(dir string) map[string]string {
	files := map[string]string{}
	for _, name := range []string{"SOUL.md", "IDENTITY.md", "TOOLS.md", "MEMORY.md", "AGENTS.md", "HEARTBEAT.md"} {
		if content, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			files[name] = strings.TrimSpace(string(content))
		}
	}
	return files
}

func handleAgentUpdate(wsMgr *WorkspaceManager, args map[string]any) (any, error) {
	resolved, err := resolveAgent(wsMgr, args, "agent_id")
	if err != nil {
		return nil, err
	}
	id := resolved.ID

	var soulToSync string
	err = wsMgr.Update(id, func(ws *Workspace) {
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
		if soul, ok := args["soul"].(string); ok && soul != "" {
			ws.Soul = soul
			if ws.ID != wsMgr.DefaultID() && hasWorkspaceDir(ws.ID) {
				soulToSync = soul
			}
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

	// Sync soul to workspace file (outside closure to handle errors)
	if soulToSync != "" {
		wsDir := paths.ResolveWorkspaceDir(id)
		if err := os.WriteFile(filepath.Join(wsDir, "SOUL.md"), []byte(soulToSync), 0600); err != nil {
			return nil, fmt.Errorf("sync SOUL.md: %w", err)
		}
	}

	return fmt.Sprintf("Agent `%s` updated.", id), nil
}

func handleAgentDelete(wsMgr *WorkspaceManager, args map[string]any) (any, error) {
	ws, err := resolveAgent(wsMgr, args, "agent_id")
	if err != nil {
		return nil, err
	}

	if err := wsMgr.Delete(ws.ID, "ai"); err != nil {
		return nil, fmt.Errorf("delete agent: %w", err)
	}

	return fmt.Sprintf("Agent `%s` deleted.", ws.ID), nil
}

func handleAgentSetDefault(wsMgr *WorkspaceManager, args map[string]any) (any, error) {
	ws, err := resolveAgent(wsMgr, args, "agent_id")
	if err != nil {
		return nil, err
	}

	if err := wsMgr.SetDefault(ws.ID); err != nil {
		return nil, fmt.Errorf("set default: %w", err)
	}

	return fmt.Sprintf("Agent `%s` is now the default.", ws.ID), nil
}

func handleAgentSessions(wsMgr *WorkspaceManager, args map[string]any) (any, error) {
	ws, err := resolveAgent(wsMgr, args, "agent_id")
	if err != nil {
		return nil, err
	}
	agentID := ws.ID

	sessions := wsMgr.ListSessionsForWorkspace(agentID)
	if len(sessions) == 0 {
		return fmt.Sprintf("No active sessions for agent %q.", agentID), nil
	}

	var b strings.Builder
	for _, s := range sessions {
		ago := time.Since(s.LastActiveAt).Round(time.Second)
		fmt.Fprintf(&b, "- [%s] %s — %d msgs — %s ago\n",
			s.Channel, s.ChatID, s.MessageCount, ago)
	}
	return fmt.Sprintf("Sessions for agent %q (%d):\n%s", agentID, len(sessions), b.String()), nil
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

// Package copilot – legacy_aliases.go registers dispatcher tools that route to
// individual tools based on the "action" parameter. The memory, vault, and
// scheduler dispatchers are the PRIMARY visible tools; individual tools are
// hidden but callable. skill_manage remains hidden.
package copilot

import (
	"context"
	"fmt"
)

// RegisterLegacyAliases registers dispatcher tools. Memory, vault, and scheduler
// are visible (primary tools); skill_manage is hidden (backward compat only).
func RegisterLegacyAliases(executor *ToolExecutor) {
	registerLegacyMemory(executor)
	registerLegacyVault(executor)
	registerLegacySkillManage(executor)
	registerLegacyScheduler(executor)
}

// legacyDispatch resolves an action to a target tool name and executes it via
// executor.executeByName (thread-safe). Returns a clear error when the target
// tool is not registered (e.g. vault tools when vault is nil).
func legacyDispatch(ctx context.Context, executor *ToolExecutor, toolMap map[string]string, action, aliasName string, args map[string]any) (any, error) {
	target, ok := toolMap[action]
	if !ok {
		return nil, fmt.Errorf("unknown %s action: %s", aliasName, action)
	}
	result, err := executor.executeByName(ctx, target, args)
	if err != nil && !executor.HasTool(target) {
		return nil, fmt.Errorf("%s not available (tool %q not registered — check server configuration)", aliasName, target)
	}
	return result, err
}

func registerLegacyMemory(executor *ToolExecutor) {
	executor.Register(
		MakeToolDefinition("memory",
			"Long-term memory: save facts, search recalled information, list entries, or rebuild index.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":   map[string]any{"type": "string", "description": "Action: save, search, list, index", "enum": []string{"save", "search", "list", "index"}},
					"content":  map[string]any{"type": "string", "description": "Content to save (for save)"},
					"category": map[string]any{"type": "string", "description": "Category (for save)"},
					"query":    map[string]any{"type": "string", "description": "Search query (for search)"},
					"limit":    map[string]any{"type": "integer", "description": "Max results (for search/list)"},
				},
				"required": []string{"action"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			return legacyDispatch(ctx, executor, map[string]string{
				"save":   "memory_save",
				"search": "memory_search",
				"list":   "memory_list",
				"index":  "memory_index",
			}, action, "memory", args)
		},
	)
}

func registerLegacyVault(executor *ToolExecutor) {
	executor.Register(
		MakeToolDefinition("vault",
			"Encrypted secret storage: check status, save/get/list/delete secrets.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "description": "Action: status, save, get, list, delete", "enum": []string{"status", "save", "get", "list", "delete"}},
					"name":   map[string]any{"type": "string", "description": "Secret name (for save/get/delete)"},
					"value":  map[string]any{"type": "string", "description": "Secret value (for save)"},
				},
				"required": []string{"action"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			return legacyDispatch(ctx, executor, map[string]string{
				"status": "vault_status",
				"save":   "vault_save",
				"get":    "vault_get",
				"list":   "vault_list",
				"delete": "vault_delete",
			}, action, "vault", args)
		},
	)
}

func registerLegacySkillManage(executor *ToolExecutor) {
	executor.RegisterHidden(
		MakeToolDefinition("skill_manage",
			"[DEPRECATED: use skill_init, skill_edit, skill_add_script, skill_list, skill_test, skill_install, skill_defaults_list, skill_defaults_install, skill_remove] Legacy skill management dispatcher.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":        map[string]any{"type": "string", "description": "Action to perform", "enum": []string{"init", "edit", "add_script", "list", "test", "install", "defaults_list", "defaults_install", "remove"}},
					"name":          map[string]any{"type": "string", "description": "Skill name (for init/edit/test/remove)"},
					"description":   map[string]any{"type": "string", "description": "Skill description (for init)"},
					"instructions":  map[string]any{"type": "string", "description": "Skill instructions (for init)"},
					"content":       map[string]any{"type": "string", "description": "Content (for edit/add_script)"},
					"input":         map[string]any{"type": "string", "description": "Test input (for test)"},
					"source":        map[string]any{"type": "string", "description": "Source URL (for install)"},
					"script_name":   map[string]any{"type": "string", "description": "Script name (for add_script)"},
					"skill_name":    map[string]any{"type": "string", "description": "Skill name (for add_script)"},
					"emoji":         map[string]any{"type": "string", "description": "Emoji (for init)"},
					"with_database": map[string]any{"type": "boolean", "description": "Include database (for init)"},
					"names":         map[string]any{"type": "array", "description": "Skill names to install (for defaults_install)", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"action"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			return legacyDispatch(ctx, executor, map[string]string{
				"init":             "skill_init",
				"edit":             "skill_edit",
				"add_script":       "skill_add_script",
				"list":             "skill_list",
				"test":             "skill_test",
				"install":          "skill_install",
				"defaults_list":    "skill_defaults_list",
				"defaults_install": "skill_defaults_install",
				"remove":           "skill_remove",
			}, action, "skill_manage", args)
		},
	)
}

func registerLegacyScheduler(executor *ToolExecutor) {
	executor.Register(
		MakeToolDefinition("scheduler",
			"Manage scheduled tasks and reminders: add, list, remove, or search jobs.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":          map[string]any{"type": "string", "description": "Action: add, list, remove, search", "enum": []string{"add", "list", "remove", "search"}},
					"id":              map[string]any{"type": "string", "description": "Unique job ID (kebab-case, e.g. 'daily-standup')"},
					"schedule":        map[string]any{"type": "string", "description": "Cron expression or natural language ('daily at 9am', 'every 5 minutes', 'today 13:30')"},
					"command":         map[string]any{"type": "string", "description": "Prompt/command to execute when job fires"},
					"type":            map[string]any{"type": "string", "description": "'cron' = recurring, 'at' = one-shot, 'every' = interval", "enum": []string{"cron", "every", "at"}},
					"channel":         map[string]any{"type": "string", "description": "Target channel for response (e.g. 'whatsapp')"},
					"chat_id":         map[string]any{"type": "string", "description": "Target chat/group ID"},
					"isolate_session": map[string]any{"type": "boolean", "description": "Run each execution in its own session (prevents cron output in conversation history)"},
					"as_subagent":     map[string]any{"type": "boolean", "description": "Run as subagent for full isolation (own session, filtered tools)"},
					"query":           map[string]any{"type": "string", "description": "Search query (for search action)"},
					"include_removed": map[string]any{"type": "boolean", "description": "Include removed jobs in search results (for search action)"},
					"limit":           map[string]any{"type": "integer", "description": "Max results for search (default: 20)"},
					"confirm":         map[string]any{"type": "boolean", "description": "Confirm removal (for remove action)"},
				},
				"required": []string{"action"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			return legacyDispatch(ctx, executor, map[string]string{
				"add":    "scheduler_add",
				"list":   "scheduler_list",
				"remove": "scheduler_remove",
				"search": "scheduler_search",
			}, action, "scheduler", args)
		},
	)
}

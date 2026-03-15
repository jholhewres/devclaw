// Package copilot – lcm_tools.go registers the `lcm` dispatcher tool with
// three actions: grep, describe, expand. Follows the same pattern as
// RegisterSessionsDispatcher in session_tools.go.
package copilot

import (
	"context"
	"fmt"
)

// RegisterLCMDispatcher registers the `lcm` tool for lossless memory retrieval.
func RegisterLCMDispatcher(executor *ToolExecutor, engine *LCMEngine) {
	if engine == nil {
		return
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"grep", "describe", "expand"},
				"description": "grep=search messages/summaries, describe=inspect DAG structure, expand=recover detail",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query (for grep)",
			},
			"summary_id": map[string]any{
				"type":        "string",
				"description": "Summary ID (for describe/expand). Use 'tree' with describe to see full DAG.",
			},
			"regex": map[string]any{
				"type":        "boolean",
				"description": "Treat query as regex (for grep). Default: false.",
			},
			"depth": map[string]any{
				"type":        "integer",
				"description": "Expansion depth (for expand). 0=messages, 1+=children. Default: 0.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max results (for grep). Default: 20.",
			},
		},
		"required": []string{"action"},
	}

	executor.Register(
		MakeToolDefinition("lcm",
			"Lossless Context Memory: search, inspect, and expand compaction summaries. "+
				"Use to recover details from earlier conversation that was compacted.",
			schema),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			if action == "" {
				return nil, fmt.Errorf("action is required")
			}

			// Get conversation ID from the agent context.
			agent := AgentRunFromCtx(ctx)
			convID := lcmConversationID(agent)
			if convID == "" {
				return nil, fmt.Errorf("lcm: no active LCM conversation (LCM may not be enabled)")
			}

			retrieval := engine.Retrieval()

			switch action {
			case "grep":
				query, _ := args["query"].(string)
				if query == "" {
					return nil, fmt.Errorf("query is required for grep action")
				}
				isRegex, _ := args["regex"].(bool)
				limit := intArg(args, "limit", 20)
				return retrieval.Grep(convID, query, isRegex, limit)

			case "describe":
				summaryID, _ := args["summary_id"].(string)
				if summaryID == "" {
					summaryID = "tree" // Default to tree overview.
				}
				return retrieval.Describe(convID, summaryID)

			case "expand":
				summaryID, _ := args["summary_id"].(string)
				if summaryID == "" {
					return nil, fmt.Errorf("summary_id is required for expand action")
				}
				depth := intArg(args, "depth", 0)
				return retrieval.Expand(convID, summaryID, depth)

			default:
				return nil, fmt.Errorf("unknown action: %s (valid: grep, describe, expand)", action)
			}
		},
	)
}

// intArg extracts an integer argument with a default value.
func intArg(args map[string]any, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return defaultVal
}

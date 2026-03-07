// Package copilot – tool_mutation.go classifies tool calls as mutating vs read-only.
// Aligned with OpenClaw's tool-mutation.ts pattern. Used for:
// - More granular error reporting (warn only on mutating tool errors)
// - Action fingerprinting for deduplication
package copilot

import (
	"strings"
)

// mutatingToolNames are tools that modify state (filesystem, messages, processes).
var mutatingToolNames = map[string]bool{
	"write_file":   true,
	"edit_file":    true,
	"bash":         true,
	"exec":         true,
	"ssh":          true,
	"scp":          true,
	"set_env":      true,
	"send_message": true,
	"message":      true,
	"cron":         true,
	"gateway":      true,
	"apply_patch":  true,
}

// readOnlyActions are action parameter values that indicate read-only operations.
var readOnlyActions = map[string]bool{
	"get": true, "list": true, "read": true, "status": true,
	"show": true, "fetch": true, "search": true, "query": true,
	"view": true, "poll": true, "log": true, "inspect": true,
	"check": true, "probe": true,
}

// IsMutatingToolName returns true if the tool is generally a mutating tool.
func IsMutatingToolName(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if mutatingToolNames[normalized] {
		return true
	}
	if strings.HasSuffix(normalized, "_actions") {
		return true
	}
	if strings.HasPrefix(normalized, "message_") || strings.Contains(normalized, "send") {
		return true
	}
	return false
}

// IsMutatingToolCall checks if a specific tool call is mutating by examining
// the tool name and its action parameter (if present).
func IsMutatingToolCall(name string, args map[string]any) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))

	// Always mutating regardless of action.
	switch normalized {
	case "write_file", "edit_file", "bash", "exec", "apply_patch", "ssh", "scp":
		return true
	}

	// Check action parameter for tools that can be both read/write.
	action, _ := args["action"].(string)
	actionNorm := strings.ToLower(strings.TrimSpace(action))

	switch normalized {
	case "message", "send_message":
		return actionNorm == "" || !readOnlyActions[actionNorm]
	case "cron", "gateway":
		return actionNorm == "" || !readOnlyActions[actionNorm]
	}

	if strings.HasSuffix(normalized, "_actions") {
		return actionNorm == "" || !readOnlyActions[actionNorm]
	}

	return false
}

// IsRecoverableToolError returns true if the error message suggests a
// recoverable issue (missing params, invalid input, transient failure)
// vs a hard failure that the model cannot recover from.
func IsRecoverableToolError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	keywords := []string{
		"required",          // "path is required", "prompt is required"
		"missing",           // "missing parameter"
		"invalid",           // "invalid argument"
		"must be",           // schema validation
		"must have",         // schema validation
		"needs",             // "needs authentication"
		"requires",          // "requires admin"
		"not found",         // "file not found" (model can fix path)
		"parsing",           // "error parsing arguments"
		"no such file",      // fs errors
		"does not exist",    // resource not found
		"permission denied", // recoverable with different params
		"timed out",         // transient timeout
		"connection refused",
		"empty", // "command is empty"
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

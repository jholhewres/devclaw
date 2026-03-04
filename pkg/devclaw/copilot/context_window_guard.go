// Package copilot – context_window_guard.go provides pre-run context window
// validation to prevent agent runs from starting with models that have
// insufficient context capacity.
package copilot

import (
	"fmt"
	"sync"
)

const (
	// ContextWindowHardMinTokens is the absolute minimum context window required
	// for a useful agent run. Below this, the run is blocked.
	ContextWindowHardMinTokens = 16_000

	// ContextWindowWarnBelowTokens triggers a warning when the context window
	// is usable but small. This helps users understand potential issues with
	// compaction, long tool outputs, and multi-turn conversations.
	ContextWindowWarnBelowTokens = 32_000
)

// ContextWindowGuardResult holds the outcome of a context window evaluation.
type ContextWindowGuardResult struct {
	// Tokens is the resolved context window size.
	Tokens int

	// ShouldBlock is true when the context window is too small for a useful run.
	ShouldBlock bool

	// ShouldWarn is true when the context window is usable but small.
	ShouldWarn bool

	// Message describes the issue (only set when ShouldBlock or ShouldWarn).
	Message string
}

// ResolveContextWindowTokens determines the effective context window for a model.
// Priority: configOverride > discovered (via provider discovery) > static lookup > default.
func ResolveContextWindowTokens(configOverride int, modelName string) int {
	if configOverride > 0 {
		return configOverride
	}
	// Check dynamic provider discovery cache (set at startup for Ollama/vLLM).
	discoveredContextWindowMu.RLock()
	fn := discoveredContextWindowFn
	discoveredContextWindowMu.RUnlock()
	if fn != nil {
		if w := fn(modelName); w > 0 {
			return w
		}
	}
	return getModelContextWindowByName(modelName)
}

// discoveredContextWindowFn is an optional callback set by ProviderDiscovery
// to supply dynamically discovered context windows. nil when discovery is disabled.
// Protected by discoveredContextWindowMu for thread-safe access.
var (
	discoveredContextWindowMu sync.RWMutex
	discoveredContextWindowFn func(model string) int
)

// setDiscoveredContextWindowFn safely sets the discovery callback.
func setDiscoveredContextWindowFn(fn func(string) int) {
	discoveredContextWindowMu.Lock()
	discoveredContextWindowFn = fn
	discoveredContextWindowMu.Unlock()
}

// EvaluateContextWindowGuard checks the context window size and returns
// whether the agent should block or warn before starting a run.
func EvaluateContextWindowGuard(tokens int) ContextWindowGuardResult {
	result := ContextWindowGuardResult{Tokens: tokens}

	if tokens < ContextWindowHardMinTokens {
		result.ShouldBlock = true
		result.Message = fmt.Sprintf(
			"context window too small (%d tokens, minimum %d). "+
				"The agent cannot operate effectively with this model. "+
				"Use a model with a larger context window or set context_tokens in config.",
			tokens, ContextWindowHardMinTokens,
		)
		return result
	}

	if tokens < ContextWindowWarnBelowTokens {
		result.ShouldWarn = true
		result.Message = fmt.Sprintf(
			"small context window (%d tokens). "+
				"The agent may struggle with compaction and long tool outputs. "+
				"Consider using a model with at least %d tokens.",
			tokens, ContextWindowWarnBelowTokens,
		)
		return result
	}

	return result
}

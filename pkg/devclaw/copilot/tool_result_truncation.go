// Package copilot – tool_result_truncation.go provides two-layer tool result
// truncation to prevent context overflow while preserving important content.
//
// Layer 1 (per-result): Smart head+tail truncation that detects error summaries,
// JSON closing brackets, and other important content at the end of tool results.
//
// Layer 2 (pre-LLM context guard): Caps individual tool results and compacts
// oldest tool results when total content exceeds the context budget.
package copilot

import (
	"fmt"
	"strings"
)

const (
	// MaxToolResultContextShare is the maximum fraction of context window that
	// a single tool result should occupy.
	MaxToolResultContextShare = 0.3

	// MinKeepChars is the minimum number of characters to keep in a truncated
	// tool result, even when aggressive truncation is applied.
	MinKeepChars = 2000

	// TailBudgetRatio is the fraction of the truncation budget allocated to
	// the tail portion when important tail content is detected.
	TailBudgetRatio = 0.3

	// MaxTailChars is the absolute maximum characters to keep from the tail.
	MaxTailChars = 4000

	// TailDetectionWindow is how many characters from the end to scan for
	// important tail content.
	TailDetectionWindow = 2000

	// ContextBudgetRatio is the fraction of context window (in chars) that
	// all tool results combined should not exceed.
	ContextBudgetRatio = 0.75

	// PerResultContextCap is the fraction of context window chars that a single
	// tool result should not exceed in the pre-LLM guard.
	PerResultContextCap = 0.5

	// CompactedPlaceholder replaces tool results that are compacted to free context.
	CompactedPlaceholder = "[compacted: tool output removed to free context]"
)

// tailKeywords are patterns that indicate important content at the end of a tool result.
var tailKeywords = []string{
	"error", "Error", "ERROR",
	"fail", "Fail", "FAIL",
	"exception", "Exception",
	"panic", "Panic",
	"warning", "Warning", "WARN",
	"summary", "Summary",
	"total", "Total",
	"result", "Result",
	"conclusion", "Conclusion",
	"}", "]", // JSON/array closings
	"exit code", "Exit Code",
	"status:", "Status:",
}

// HasImportantTail returns true if the last portion of text contains patterns
// that suggest important content (errors, JSON closings, summaries, etc.).
func HasImportantTail(text string) bool {
	if len(text) <= TailDetectionWindow {
		return false // Short enough to keep entirely.
	}
	tail := text[len(text)-TailDetectionWindow:]
	for _, kw := range tailKeywords {
		if strings.Contains(tail, kw) {
			return true
		}
	}
	return false
}

// TruncateToolResult applies smart truncation to a single tool result.
// When important tail content is detected, it uses a head+tail strategy;
// otherwise it uses head-only truncation.
func TruncateToolResult(text string, maxChars int) string {
	if maxChars < MinKeepChars {
		maxChars = MinKeepChars
	}
	if len(text) <= maxChars {
		return text
	}

	if HasImportantTail(text) {
		return truncateHeadTail(text, maxChars)
	}
	return truncateHeadOnly(text, maxChars)
}

// truncateHeadTail keeps content from both the start and end of the text.
func truncateHeadTail(text string, maxChars int) string {
	// Calculate tail budget.
	tailBudget := int(float64(maxChars) * TailBudgetRatio)
	if tailBudget > MaxTailChars {
		tailBudget = MaxTailChars
	}
	if tailBudget < 500 {
		tailBudget = 500
	}

	// Head gets whatever is left after tail + separator.
	separator := fmt.Sprintf("\n\n... [truncated %d chars, showing head+tail] ...\n\n", len(text)-maxChars)
	headBudget := maxChars - tailBudget - len(separator)
	if headBudget < 500 {
		headBudget = 500
	}

	head := text[:headBudget]
	tail := text[len(text)-tailBudget:]

	return head + separator + tail
}

// truncateHeadOnly keeps only the beginning of the text.
func truncateHeadOnly(text string, maxChars int) string {
	suffix := fmt.Sprintf("\n\n... [truncated: %d → %d chars]", len(text), maxChars)
	keepChars := maxChars - len(suffix)
	if keepChars < 500 {
		keepChars = 500
	}
	return text[:keepChars] + suffix
}

// GuardToolResultContext applies the pre-LLM context guard to all messages.
// It caps individual tool results and compacts oldest tool results when the
// total content exceeds the context budget.
//
// contextWindowTokens is the model's context window in tokens. We estimate
// 1 token ≈ 4 chars for budget calculation.
func GuardToolResultContext(messages []chatMessage, contextWindowTokens int) []chatMessage {
	if contextWindowTokens <= 0 || len(messages) == 0 {
		return messages
	}

	contextChars := contextWindowTokens * 4
	perResultCap := int(float64(contextChars) * PerResultContextCap)
	totalBudget := int(float64(contextChars) * ContextBudgetRatio)

	if perResultCap < MinKeepChars {
		perResultCap = MinKeepChars
	}

	// First pass: cap individual oversized tool results.
	result := make([]chatMessage, len(messages))
	copy(result, messages)

	for i := range result {
		if result[i].Role != "tool" {
			continue
		}
		s, ok := result[i].Content.(string)
		if !ok {
			continue
		}
		if len(s) > perResultCap {
			result[i].Content = TruncateToolResult(s, perResultCap)
		}
	}

	// Second pass: check total tool result chars.
	totalToolChars := 0
	type toolIdx struct {
		index int
		chars int
	}
	var toolResults []toolIdx

	for i, m := range result {
		if m.Role != "tool" {
			continue
		}
		s, ok := m.Content.(string)
		if !ok {
			continue
		}
		totalToolChars += len(s)
		toolResults = append(toolResults, toolIdx{index: i, chars: len(s)})
	}

	// If within budget, return as-is.
	if totalToolChars <= totalBudget {
		return result
	}

	// Compact oldest tool results first until we're within budget.
	for _, tr := range toolResults {
		if totalToolChars <= totalBudget {
			break
		}
		s, ok := result[tr.index].Content.(string)
		if !ok {
			continue
		}
		if s == CompactedPlaceholder {
			continue
		}
		freed := len(s) - len(CompactedPlaceholder)
		totalToolChars -= freed
		result[tr.index].Content = CompactedPlaceholder
	}

	return result
}

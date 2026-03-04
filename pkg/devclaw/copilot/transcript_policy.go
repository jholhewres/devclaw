// Package copilot – transcript_policy.go defines per-provider transcript
// sanitization policies. Different LLM providers have different requirements
// for message format, turn ordering, and content types. This policy system
// ensures that the conversation transcript is compatible with each provider
// before sending.
package copilot

import "strings"

// TranscriptPolicy defines the sanitization rules for a provider's API.
type TranscriptPolicy struct {
	// StrictToolCallIDs requires that tool_call IDs are exactly formatted.
	// Mistral requires strict UUID-like IDs (no prefixes, lowercase).
	StrictToolCallIDs bool

	// EnforceTurnOrdering ensures user/assistant turns alternate properly.
	// Google (Gemini) rejects consecutive same-role messages.
	EnforceTurnOrdering bool

	// DropThinkingBlocks removes <thinking> and <reasoning> content blocks.
	// Some providers reject or misinterpret these meta-reasoning blocks.
	DropThinkingBlocks bool

	// MergeConsecutiveSameRole merges consecutive messages from the same role
	// into a single message. Required by some providers that reject duplicates.
	MergeConsecutiveSameRole bool

	// RequireToolResultsAfterToolUse ensures every tool_use has a matching
	// tool_result immediately following it. Anthropic requires this.
	RequireToolResultsAfterToolUse bool

	// StripCacheControl removes cache_control fields from messages.
	// Only Anthropic and compatible proxies support this.
	StripCacheControl bool

	// MaxSystemMessages is the maximum number of system messages allowed.
	// Some providers only support a single system message.
	// 0 = unlimited.
	MaxSystemMessages int

	// CollapseMultipleSystemMessages merges multiple system messages into one.
	CollapseMultipleSystemMessages bool
}

// GetTranscriptPolicy returns the policy for a given provider.
func GetTranscriptPolicy(provider string) TranscriptPolicy {
	provider = strings.ToLower(provider)
	switch provider {
	case "anthropic", "zai-anthropic":
		return TranscriptPolicy{
			RequireToolResultsAfterToolUse: true,
			EnforceTurnOrdering:            true,
		}
	case "google", "gemini":
		return TranscriptPolicy{
			EnforceTurnOrdering:            true,
			MergeConsecutiveSameRole:        true,
			DropThinkingBlocks:             true,
			StripCacheControl:              true,
			CollapseMultipleSystemMessages: true,
			MaxSystemMessages:              1,
		}
	case "mistral":
		return TranscriptPolicy{
			StrictToolCallIDs:              true,
			EnforceTurnOrdering:            true,
			DropThinkingBlocks:             true,
			StripCacheControl:              true,
			CollapseMultipleSystemMessages: true,
			MaxSystemMessages:              1,
		}
	case "ollama":
		return TranscriptPolicy{
			DropThinkingBlocks:             true,
			StripCacheControl:              true,
			CollapseMultipleSystemMessages: true,
			MaxSystemMessages:              1,
		}
	case "openrouter":
		// OpenRouter proxies to multiple providers; use minimal sanitization.
		// The downstream provider handles its own requirements.
		return TranscriptPolicy{}
	default: // openai, xai, deepseek, etc.
		return TranscriptPolicy{
			StripCacheControl: true,
		}
	}
}

// ApplyTranscriptPolicy sanitizes messages according to the provider's policy.
func ApplyTranscriptPolicy(messages []chatMessage, policy TranscriptPolicy) []chatMessage {
	result := make([]chatMessage, 0, len(messages))
	result = append(result, messages...)

	// 1. Drop thinking blocks.
	if policy.DropThinkingBlocks {
		result = dropThinkingBlocks(result)
	}

	// 2. Strip cache_control.
	if policy.StripCacheControl {
		result = stripCacheControlFromMessages(result)
	}

	// 3. Collapse multiple system messages.
	if policy.CollapseMultipleSystemMessages {
		result = collapseSystemMessages(result)
	}

	// 4. Merge consecutive same-role messages.
	if policy.MergeConsecutiveSameRole {
		result = mergeConsecutiveSameRole(result)
	}

	// 5. Enforce turn ordering (insert placeholder if needed).
	if policy.EnforceTurnOrdering {
		result = enforceTurnOrdering(result)
	}

	// 6. Repair tool use/result pairing.
	if policy.RequireToolResultsAfterToolUse {
		result = RepairToolUseResultPairing(result)
	}

	return result
}

// dropThinkingBlocks removes <thinking> and <reasoning> content from messages.
func dropThinkingBlocks(messages []chatMessage) []chatMessage {
	for i, m := range messages {
		if s, ok := m.Content.(string); ok {
			cleaned := removeXMLTags(s, "thinking")
			cleaned = removeXMLTags(cleaned, "reasoning")
			if cleaned != s {
				messages[i].Content = strings.TrimSpace(cleaned)
			}
		}
	}
	return messages
}

// removeXMLTags removes matched XML tags and their content from text.
func removeXMLTags(text, tag string) string {
	for {
		start := strings.Index(text, "<"+tag+">")
		if start == -1 {
			start = strings.Index(text, "<"+tag+" ")
		}
		if start == -1 {
			break
		}
		end := strings.Index(text[start:], "</"+tag+">")
		if end == -1 {
			break
		}
		text = text[:start] + text[start+end+len("</"+tag+">"):]
	}
	return text
}

// stripCacheControlFromMessages removes cache_control fields.
func stripCacheControlFromMessages(messages []chatMessage) []chatMessage {
	for i := range messages {
		messages[i].CacheControl = nil
	}
	return messages
}

// collapseSystemMessages merges all system messages into a single one.
func collapseSystemMessages(messages []chatMessage) []chatMessage {
	var systemParts []string
	var nonSystem []chatMessage

	for _, m := range messages {
		if m.Role == "system" {
			if s, ok := m.Content.(string); ok && s != "" {
				systemParts = append(systemParts, s)
			}
		} else {
			nonSystem = append(nonSystem, m)
		}
	}

	if len(systemParts) == 0 {
		return nonSystem
	}

	result := make([]chatMessage, 0, 1+len(nonSystem))
	result = append(result, chatMessage{
		Role:    "system",
		Content: strings.Join(systemParts, "\n\n"),
	})
	result = append(result, nonSystem...)
	return result
}

// mergeConsecutiveSameRole merges consecutive messages from the same role.
func mergeConsecutiveSameRole(messages []chatMessage) []chatMessage {
	if len(messages) <= 1 {
		return messages
	}

	var result []chatMessage
	result = append(result, messages[0])

	for i := 1; i < len(messages); i++ {
		prev := &result[len(result)-1]
		curr := messages[i]

		// Only merge text-only messages of the same role (skip tool messages).
		if curr.Role == prev.Role && curr.Role != "tool" && len(curr.ToolCalls) == 0 && len(prev.ToolCalls) == 0 {
			prevStr, prevOk := prev.Content.(string)
			currStr, currOk := curr.Content.(string)
			if prevOk && currOk {
				prev.Content = prevStr + "\n\n" + currStr
				continue
			}
		}
		result = append(result, curr)
	}
	return result
}

// enforceTurnOrdering ensures user/assistant messages alternate properly.
// Inserts placeholder messages when consecutive same-role messages are found.
func enforceTurnOrdering(messages []chatMessage) []chatMessage {
	if len(messages) <= 1 {
		return messages
	}

	var result []chatMessage
	result = append(result, messages[0])

	for i := 1; i < len(messages); i++ {
		curr := messages[i]
		prev := result[len(result)-1]

		// Skip system and tool messages for turn ordering checks.
		if curr.Role == "system" || curr.Role == "tool" || prev.Role == "system" || prev.Role == "tool" {
			result = append(result, curr)
			continue
		}

		// If two consecutive user or assistant messages, insert a placeholder.
		if curr.Role == prev.Role {
			if curr.Role == "user" {
				result = append(result, chatMessage{
					Role:    "assistant",
					Content: "(continuing)",
				})
			} else {
				result = append(result, chatMessage{
					Role:    "user",
					Content: "(continue)",
				})
			}
		}
		result = append(result, curr)
	}
	return result
}

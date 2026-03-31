// Package copilot – memory_extractor.go extracts structured memories from
// conversation history before context compaction. This preserves valuable
// information (decisions, preferences, facts, learnings) that would otherwise
// be lost when messages are summarized or discarded.
//
// Aligned with Claude Code's sessionMemoryCompact pattern: extract first,
// then compact safely knowing nothing important is lost.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ExtractedMemory represents a single piece of information extracted from
// conversation history that should be preserved in long-term memory.
type ExtractedMemory struct {
	// Type classifies the memory for retrieval and organization.
	// Values: "decision", "preference", "fact", "learning", "context"
	Type string `json:"type"`

	// Content is the extracted information in concise, self-contained form.
	Content string `json:"content"`

	// Importance is a 1-5 score indicating how valuable this memory is.
	// 5 = critical decision, 1 = minor observation.
	Importance int `json:"importance"`
}

// MemoryExtractor analyzes conversation history and extracts structured
// memories before compaction.
type MemoryExtractor struct {
	llm    *LLMClient
	logger *slog.Logger
}

// NewMemoryExtractor creates a new extractor.
func NewMemoryExtractor(llm *LLMClient, logger *slog.Logger) *MemoryExtractor {
	return &MemoryExtractor{
		llm:    llm,
		logger: logger.With("component", "memory_extractor"),
	}
}

const memoryExtractionPrompt = `Analyze the conversation below and extract important information that should be preserved in long-term memory. Focus on:

1. **Decisions** — choices made about architecture, tools, approaches, or direction
2. **Preferences** — user preferences about code style, communication, workflow
3. **Facts** — project-specific facts, constraints, or requirements discovered
4. **Learnings** — debugging insights, gotchas, or patterns that would be useful later
5. **Context** — important background information about the project or task

Rules:
- Each memory must be self-contained (understandable without the original conversation)
- Be concise but specific (include file names, function names, concrete details)
- Skip trivial or obvious information
- Rate importance 1-5 (5 = critical, would cause problems if forgotten)
- Return an empty array if nothing is worth preserving

Respond with ONLY a JSON array, no other text:
[{"type":"decision","content":"...","importance":4},{"type":"fact","content":"...","importance":3}]`

// Extract analyzes messages and returns structured memories.
// Uses a focused LLM call with a structured extraction prompt.
// Returns nil (no error) if extraction fails — this is best-effort.
func (e *MemoryExtractor) Extract(ctx context.Context, messages []chatMessage, modelOverride string) []ExtractedMemory {
	if e.llm == nil || len(messages) < 4 {
		return nil
	}

	// Build a focused conversation for extraction.
	// Include system context + conversation body, excluding tool results
	// (they're verbose and usually not memory-worthy).
	extractMsgs := make([]chatMessage, 0, len(messages)/2+2)
	extractMsgs = append(extractMsgs, chatMessage{
		Role:    "system",
		Content: "You are a memory extraction assistant. Extract important information from conversations into structured JSON.",
	})

	// Condense the conversation: keep user/assistant messages, summarize tool results.
	for _, m := range messages {
		switch m.Role {
		case "system":
			continue // Skip system prompts — they're template, not memory.
		case "user":
			content, _ := m.Content.(string)
			if content == "" || isSystemInjectedMessage(content) {
				continue
			}
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			extractMsgs = append(extractMsgs, chatMessage{Role: "user", Content: content})
		case "assistant":
			content, _ := m.Content.(string)
			if content == "" {
				continue
			}
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			extractMsgs = append(extractMsgs, chatMessage{Role: "assistant", Content: content})
		case "tool":
			// Summarize tool results to save tokens.
			content, _ := m.Content.(string)
			if content == "" {
				continue
			}
			summary := content
			if len(summary) > 100 {
				summary = summary[:100] + "..."
			}
			extractMsgs = append(extractMsgs, chatMessage{
				Role:       "assistant",
				Content:    fmt.Sprintf("[Tool result: %s]", summary),
			})
		}
	}

	// Add the extraction prompt.
	extractMsgs = append(extractMsgs, chatMessage{
		Role:    "user",
		Content: memoryExtractionPrompt,
	})

	// Call LLM with short timeout — this is best-effort.
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := e.llm.CompleteWithFallbackUsingModel(callCtx, modelOverride, extractMsgs, nil)
	if err != nil {
		e.logger.Warn("memory extraction LLM call failed", "error", err)
		return nil
	}

	// Parse the JSON response.
	memories := parseExtractedMemories(resp.Content)
	if len(memories) == 0 {
		e.logger.Debug("no memories extracted from conversation")
		return nil
	}

	e.logger.Info("memories extracted from conversation",
		"count", len(memories),
		"types", memoryTypeSummary(memories),
	)

	return memories
}

// parseExtractedMemories parses the LLM response into ExtractedMemory structs.
// Handles common LLM response quirks (markdown code blocks, extra text).
func parseExtractedMemories(response string) []ExtractedMemory {
	response = strings.TrimSpace(response)

	// Strip markdown code blocks if present.
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		response = strings.Join(jsonLines, "\n")
	}

	// Find the JSON array in the response.
	startIdx := strings.Index(response, "[")
	endIdx := strings.LastIndex(response, "]")
	if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
		return nil
	}
	response = response[startIdx : endIdx+1]

	var memories []ExtractedMemory
	if err := json.Unmarshal([]byte(response), &memories); err != nil {
		return nil
	}

	// Filter out invalid entries.
	valid := make([]ExtractedMemory, 0, len(memories))
	for _, m := range memories {
		if m.Content == "" || m.Type == "" {
			continue
		}
		if m.Importance < 1 {
			m.Importance = 1
		}
		if m.Importance > 5 {
			m.Importance = 5
		}
		// Normalize type.
		m.Type = strings.ToLower(m.Type)
		valid = append(valid, m)
	}

	return valid
}

// memoryTypeSummary returns a summary string like "2 decisions, 1 fact, 1 learning".
func memoryTypeSummary(memories []ExtractedMemory) string {
	counts := make(map[string]int)
	for _, m := range memories {
		counts[m.Type]++
	}
	var parts []string
	for t, c := range counts {
		parts = append(parts, fmt.Sprintf("%d %s", c, t))
	}
	return strings.Join(parts, ", ")
}

// FormatMemoriesForStorage formats extracted memories into a text block
// suitable for saving to the memory store.
func FormatMemoriesForStorage(memories []ExtractedMemory, sessionID string) string {
	if len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Pre-compaction memory extraction (session: %s)\n\n", sessionID))

	for _, m := range memories {
		importance := strings.Repeat("!", m.Importance)
		sb.WriteString(fmt.Sprintf("- [%s] (%s) %s\n", m.Type, importance, m.Content))
	}

	return sb.String()
}

// isSystemInjectedMessage detects system-injected messages that shouldn't
// be considered for memory extraction (e.g. reflection nudges, heartbeats).
func isSystemInjectedMessage(content string) bool {
	if strings.HasPrefix(content, "[System:") {
		return true
	}
	if strings.TrimSpace(strings.ToUpper(content)) == "HEARTBEAT" {
		return true
	}
	if strings.Contains(content, "HEARTBEAT_OK") {
		return true
	}
	return false
}

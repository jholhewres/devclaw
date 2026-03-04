// Package copilot – compaction_safeguard.go provides safeguards for the
// context compaction process to ensure critical information is preserved
// when the conversation is summarized to free context window space.
package copilot

import (
	"fmt"
	"strings"
)

// CompactionConfig controls how the compaction process preserves important context.
type CompactionConfig struct {
	// KeepRecentUserTurns is how many recent user turns to preserve verbatim
	// in the compacted output. Default: 3, Max: 12.
	KeepRecentUserTurns int `yaml:"keep_recent_user_turns"`

	// MaxToolFailures is how many tool failure entries to include in the
	// compaction summary. Default: 8.
	MaxToolFailures int `yaml:"max_tool_failures"`

	// ToolFailurePreviewChars is the max chars per tool failure preview.
	// Default: 240.
	ToolFailurePreviewChars int `yaml:"tool_failure_preview_chars"`

	// IdentifierPolicy controls whether the compaction summary instruction
	// includes guidance to preserve identifiers. Default: "preserve".
	// Values: "preserve" (include instruction), "none" (omit instruction).
	IdentifierPolicy string `yaml:"identifier_policy"`
}

// DefaultCompactionConfig returns sensible defaults.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		KeepRecentUserTurns:     3,
		MaxToolFailures:         8,
		ToolFailurePreviewChars: 240,
		IdentifierPolicy:        "preserve",
	}
}

// resolvedCompactionConfig returns the config with defaults applied for zero values.
func resolvedCompactionConfig(cfg CompactionConfig) CompactionConfig {
	if cfg.KeepRecentUserTurns <= 0 {
		cfg.KeepRecentUserTurns = 3
	}
	if cfg.KeepRecentUserTurns > 12 {
		cfg.KeepRecentUserTurns = 12
	}
	if cfg.MaxToolFailures <= 0 {
		cfg.MaxToolFailures = 8
	}
	if cfg.ToolFailurePreviewChars <= 0 {
		cfg.ToolFailurePreviewChars = 240
	}
	if cfg.IdentifierPolicy == "" {
		cfg.IdentifierPolicy = "preserve"
	}
	return cfg
}

// compactionIdentifierInstruction returns the instruction text for preserving
// identifiers during compaction, or empty string if policy is "none".
func compactionIdentifierInstruction(policy string) string {
	if policy == "none" {
		return ""
	}
	return "\n[Identifier Preservation] When summarizing, preserve ALL identifiers verbatim: " +
		"UUIDs, tokens, API keys (masked), IP addresses, file paths, URLs, " +
		"session IDs, commit hashes, and branch names. " +
		"Do NOT paraphrase or abbreviate these values."
}

// collectRecentUserTurns extracts the last N user messages from a message slice.
// Each entry includes the message index for reference.
func collectRecentUserTurns(messages []chatMessage, keepCount int) []chatMessage {
	if keepCount <= 0 {
		return nil
	}

	var userMsgs []chatMessage
	for _, m := range messages {
		if m.Role == "user" {
			userMsgs = append(userMsgs, m)
		}
	}

	if len(userMsgs) <= keepCount {
		return userMsgs
	}
	return userMsgs[len(userMsgs)-keepCount:]
}

// buildCompactionInstruction creates the structured compaction prompt that
// instructs the LLM how to summarize while preserving critical information.
func buildCompactionInstruction(cfg CompactionConfig, toolFailures []string, fileOps []string) string {
	cfg = resolvedCompactionConfig(cfg)

	var b strings.Builder
	b.WriteString("Summarize the conversation so far, preserving:")
	b.WriteString("\n- Key decisions and their rationale")
	b.WriteString("\n- Current task state and progress")
	b.WriteString("\n- Important error messages and their resolutions")
	b.WriteString("\n- Active constraints and requirements from the user")

	// Identifier preservation instruction.
	if instr := compactionIdentifierInstruction(cfg.IdentifierPolicy); instr != "" {
		b.WriteString(instr)
	}

	// Tool failures section.
	if len(toolFailures) > 0 {
		b.WriteString("\n\n[Recent Tool Failures]")
		for i, f := range toolFailures {
			b.WriteString(fmt.Sprintf("\n%d. %s", i+1, f))
		}
	}

	// File operations section.
	if len(fileOps) > 0 {
		b.WriteString("\n\n[Files Touched]")
		// Cap at 15 for readability.
		shown := fileOps
		if len(shown) > 15 {
			shown = shown[len(shown)-15:]
		}
		b.WriteString("\n" + strings.Join(shown, ", "))
	}

	return b.String()
}

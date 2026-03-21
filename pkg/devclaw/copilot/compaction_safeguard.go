// Package copilot – compaction_safeguard.go provides safeguards for the
// context compaction process to ensure critical information is preserved
// when the conversation is summarized to free context window space.
package copilot

import (
	"fmt"
	"regexp"
	"sort"
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

	// CompactionModel overrides the model used for summarization LLM calls.
	// If empty, uses the agent's current model.
	CompactionModel string `yaml:"compaction_model"`

	// QualityGuard configures post-summarization quality auditing.
	QualityGuard QualityGuardConfig `yaml:"quality_guard"`

	// ContextPruning configures ratio-based in-memory context pruning.
	ContextPruning ContextPruningConfig `yaml:"context_pruning"`

	// TimeoutSeconds is the maximum time (in seconds) allowed for the LLM
	// summarization step during compaction. If exceeded, falls back to
	// trim-by-count. Default: 900 (15 minutes).
	TimeoutSeconds int `yaml:"timeout_seconds"`

	// PostIndexSync controls whether a memory indexer sync is triggered after
	// compaction completes. Values: "off" (default), "async" (fire-and-forget),
	// "await" (block until indexing completes, not yet implemented — treated as async).
	PostIndexSync string `yaml:"post_index_sync"`

	// LCMEnabled enables the Lossless Compaction Module (DAG-based memory).
	// When enabled, messages are persisted verbatim and compaction builds a
	// hierarchical summary DAG instead of a flat summary string.
	// Default: true (nil pointer = true).
	LCMEnabled *bool `yaml:"lcm_enabled"`

	// LCM holds configuration for the Lossless Compaction Module.
	LCM LCMConfig `yaml:"lcm"`
}

// LCMConfig controls the Lossless Compaction Module behavior.
type LCMConfig struct {
	// FreshTailCount is how many recent messages to keep unsummarized. Default: 32.
	FreshTailCount int `yaml:"fresh_tail_count"`

	// LeafChunkMaxTokens is the max tokens per leaf chunk. Default: 20000.
	LeafChunkMaxTokens int `yaml:"leaf_chunk_max_tokens"`

	// CondensedMinChildren is the minimum orphan summaries to trigger condensation. Default: 4.
	CondensedMinChildren int `yaml:"condensed_min_children"`

	// CondensedMaxChildren is the max summaries per condensed batch. Default: 8.
	CondensedMaxChildren int `yaml:"condensed_max_children"`

	// SoftTriggerRatio is the context usage fraction for soft compaction trigger. Default: 0.6.
	SoftTriggerRatio float64 `yaml:"soft_trigger_ratio"`

	// HardTriggerRatio is the context usage fraction for hard compaction trigger. Default: 0.85.
	HardTriggerRatio float64 `yaml:"hard_trigger_ratio"`

	// MaxSummaryTokens is the max tokens per individual summary. Default: 2000.
	MaxSummaryTokens int `yaml:"max_summary_tokens"`

	// SummaryModel overrides the model used for LCM summarization calls.
	// Priority: SummaryModel > CompactionConfig.CompactionModel > agent's current model.
	// If empty, falls back to the next available model in the chain.
	SummaryModel string `yaml:"summary_model"`

	// SummaryProvider overrides the provider for LCM summarization calls.
	// If empty, uses the provider from the session's LLM client.
	SummaryProvider string `yaml:"summary_provider"`

	// LargeFileTokenThreshold is the token count above which ingested content
	// is intercepted and stored as a separate file with an exploration summary.
	// Default: 25000. Set to 0 to disable.
	LargeFileTokenThreshold int `yaml:"large_file_token_threshold"`

	// PruneHeartbeatOK removes heartbeat turn cycles (user heartbeat prompt +
	// assistant HEARTBEAT_OK response) from the LCM store during ingest.
	// Default: true (nil pointer = true).
	PruneHeartbeatOK *bool `yaml:"prune_heartbeat_ok"`
}

// QualityGuardConfig controls the post-summarization audit and retry mechanism.
type QualityGuardConfig struct {
	// Enabled turns on quality guard (audit + retry). Default: true.
	Enabled *bool `yaml:"enabled"`

	// MaxRetries is the maximum number of retry attempts on audit failure. Default: 1, Max: 3.
	MaxRetries int `yaml:"max_retries"`

	// StrictIdentifiers requires that extracted identifiers appear in the summary. Default: false.
	StrictIdentifiers bool `yaml:"strict_identifiers"`
}

// qualityGuardEnabled returns whether quality guard is enabled (default true).
func (c QualityGuardConfig) qualityGuardEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// ContextPruningConfig controls ratio-based in-memory pruning of tool results.
type ContextPruningConfig struct {
	// SoftTrimRatio is the context usage ratio above which tool results are soft-trimmed
	// (head+tail). Default: 0.3.
	SoftTrimRatio float64 `yaml:"soft_trim_ratio"`

	// HardClearRatio is the context usage ratio above which old tool results are
	// replaced with a placeholder. Default: 0.5.
	HardClearRatio float64 `yaml:"hard_clear_ratio"`

	// SoftTrimMaxChars is the max chars for a tool result before soft-trim kicks in.
	// Default: 4096.
	SoftTrimMaxChars int `yaml:"soft_trim_max_chars"`

	// ProtectRecentTurns is how many recent assistant turns to protect from pruning.
	// Default: 3.
	ProtectRecentTurns int `yaml:"protect_recent_turns"`
}

// DefaultCompactionConfig returns sensible defaults.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		KeepRecentUserTurns:     3,
		MaxToolFailures:         8,
		ToolFailurePreviewChars: 240,
		IdentifierPolicy:        "preserve",
		QualityGuard: QualityGuardConfig{
			MaxRetries: 1,
		},
		ContextPruning: ContextPruningConfig{
			SoftTrimRatio:      0.3,
			HardClearRatio:     0.5,
			SoftTrimMaxChars:   4096,
			ProtectRecentTurns: 3,
		},
		TimeoutSeconds: 900,
	}
}

// lcmEnabled returns whether the LCM is enabled (default true when nil).
func (c CompactionConfig) lcmEnabled() bool {
	if c.LCMEnabled == nil {
		return true
	}
	return *c.LCMEnabled
}

// resolvedLCMConfig returns the LCM config with defaults applied for zero values.
func resolvedLCMConfig(cfg LCMConfig) LCMConfig {
	if cfg.FreshTailCount <= 0 {
		cfg.FreshTailCount = 32
	}
	if cfg.LeafChunkMaxTokens <= 0 {
		cfg.LeafChunkMaxTokens = 20000
	}
	if cfg.CondensedMinChildren <= 0 {
		cfg.CondensedMinChildren = 4
	}
	if cfg.CondensedMaxChildren <= 0 {
		cfg.CondensedMaxChildren = 8
	}
	if cfg.SoftTriggerRatio <= 0 {
		cfg.SoftTriggerRatio = 0.6
	}
	if cfg.HardTriggerRatio <= 0 {
		cfg.HardTriggerRatio = 0.85
	}
	if cfg.MaxSummaryTokens <= 0 {
		cfg.MaxSummaryTokens = 2000
	}
	if cfg.LargeFileTokenThreshold == 0 {
		cfg.LargeFileTokenThreshold = 25000
	}
	// PruneHeartbeatOK defaults to true (nil = true).
	// SummaryModel and SummaryProvider default to "" (use fallback chain).
	return cfg
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
	// Quality guard defaults.
	if cfg.QualityGuard.MaxRetries <= 0 {
		cfg.QualityGuard.MaxRetries = 1
	}
	if cfg.QualityGuard.MaxRetries > 3 {
		cfg.QualityGuard.MaxRetries = 3
	}
	// Context pruning defaults.
	if cfg.ContextPruning.SoftTrimRatio <= 0 {
		cfg.ContextPruning.SoftTrimRatio = 0.3
	}
	if cfg.ContextPruning.HardClearRatio <= 0 {
		cfg.ContextPruning.HardClearRatio = 0.5
	}
	if cfg.ContextPruning.SoftTrimMaxChars <= 0 {
		cfg.ContextPruning.SoftTrimMaxChars = 4096
	}
	if cfg.ContextPruning.ProtectRecentTurns <= 0 {
		cfg.ContextPruning.ProtectRecentTurns = 3
	}
	// Compaction timeout defaults.
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 900
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

// --- Structured Compaction Prompt ---

// requiredCompactionSections lists the mandatory section headings for structured summaries.
var requiredCompactionSections = []string{
	"## Decisions",
	"## Open TODOs",
	"## Constraints/Rules",
	"## Pending user asks",
	"## Exact identifiers",
}

// buildStructuredCompactionPrompt creates the structured compaction prompt that
// instructs the LLM to produce a summary with mandatory sections. This replaces
// the old flat "3-4 sentences" approach, ported from OpenClaw's compaction-safeguard.
func buildStructuredCompactionPrompt(cfg CompactionConfig, toolFailures []string, readFiles []string, modifiedFiles []string) string {
	cfg = resolvedCompactionConfig(cfg)

	var b strings.Builder

	b.WriteString("You are a compaction assistant. Produce a compact, factual summary of the conversation so far.\n\n")
	b.WriteString("Use EXACTLY these section headings (include all even if empty):\n")
	for _, s := range requiredCompactionSections {
		b.WriteString(s + "\n")
	}

	b.WriteString("\nRules:\n")
	b.WriteString("- Under '## Decisions': list key decisions and their rationale.\n")
	b.WriteString("- Under '## Open TODOs': list incomplete tasks and their status.\n")
	b.WriteString("- Under '## Constraints/Rules': list active constraints from the user or system.\n")
	b.WriteString("- Under '## Pending user asks': describe what the user most recently asked for.\n")
	b.WriteString("- Under '## Exact identifiers': list ALL file paths, UUIDs, URLs, commit hashes, branch names, session IDs, and API keys (masked) verbatim.\n")
	b.WriteString("- Focus ONLY on CONFIRMED facts from tool results. Do NOT speculate or invent outcomes.\n")
	b.WriteString("- If a tool result was ambiguous or errored, say so explicitly.\n")
	b.WriteString("- Do NOT assert that something was done successfully unless the tool result confirmed it.\n")
	b.WriteString("- NEVER use bold text formatting.\n")

	b.WriteString("\nCRITICAL — LANGUAGE & PERSONA CONTINUITY:\n")
	b.WriteString("- The summary MUST be written in the SAME LANGUAGE as the original conversation (e.g., Portuguese, English, Spanish).\n")
	b.WriteString("- Preserve the assistant's persona, tone, and communication style.\n")
	b.WriteString("- If the user has been addressed in a specific way (formal/informal), maintain that style.\n")

	b.WriteString("\nMUST PRESERVE:\n")
	b.WriteString("- Active tasks and their current status\n")
	b.WriteString("- Batch progress (e.g. 'processed 45/100 items')\n")
	b.WriteString("- Decisions and their rationale\n")
	b.WriteString("- Error messages and their resolutions\n")
	b.WriteString("- User preferences stated during the conversation\n")

	// Identifier preservation instruction.
	if instr := compactionIdentifierInstruction(cfg.IdentifierPolicy); instr != "" {
		b.WriteString(instr)
		b.WriteString("\n")
	}

	// Tool failures section.
	if len(toolFailures) > 0 {
		b.WriteString("\n<tool_failures>\n")
		for i, f := range toolFailures {
			if i >= cfg.MaxToolFailures {
				break
			}
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
		}
		b.WriteString("</tool_failures>\n")
	}

	// File operations sections (separated read vs modified, like OpenClaw).
	if len(readFiles) > 0 {
		b.WriteString("\n<files_read>\n")
		shown := readFiles
		if len(shown) > 15 {
			shown = shown[len(shown)-15:]
		}
		b.WriteString(strings.Join(shown, "\n"))
		b.WriteString("\n</files_read>\n")
	}

	if len(modifiedFiles) > 0 {
		b.WriteString("\n<files_modified>\n")
		shown := modifiedFiles
		if len(shown) > 15 {
			shown = shown[len(shown)-15:]
		}
		b.WriteString(strings.Join(shown, "\n"))
		b.WriteString("\n</files_modified>\n")
	}

	return b.String()
}

// --- Quality Guard ---

// qualityAuditResult holds the result of a summary quality audit.
type qualityAuditResult struct {
	// Passed is true if the summary meets all quality criteria.
	Passed bool
	// Failures describes which checks failed.
	Failures []string
}

// auditSummaryQuality checks if a compaction summary contains all required sections,
// preserves extracted identifiers (strict mode), and reflects the last user ask.
func auditSummaryQuality(summary string, identifiers []string, lastUserAsk string, strict bool) qualityAuditResult {
	var failures []string
	lower := strings.ToLower(summary)

	// 1. Check required section headings.
	for _, section := range requiredCompactionSections {
		if !strings.Contains(lower, strings.ToLower(section)) {
			failures = append(failures, fmt.Sprintf("missing section: %s", section))
		}
	}

	// 2. Check identifier preservation (strict mode only).
	if strict && len(identifiers) > 0 {
		for _, id := range identifiers {
			if !strings.Contains(summary, id) {
				failures = append(failures, fmt.Sprintf("missing identifier: %s", id))
			}
		}
	}

	// 3. Check that the last user ask is reflected (token overlap).
	if lastUserAsk != "" {
		overlap := tokenOverlap(lastUserAsk, summary)
		if overlap < 0.15 {
			failures = append(failures, fmt.Sprintf("last user ask poorly reflected (overlap=%.2f)", overlap))
		}
	}

	return qualityAuditResult{
		Passed:   len(failures) == 0,
		Failures: failures,
	}
}

// tokenOverlap computes the fraction of words from source that appear in target.
// Word-level, case-insensitive comparison.
func tokenOverlap(source, target string) float64 {
	sourceWords := strings.Fields(strings.ToLower(source))
	if len(sourceWords) == 0 {
		return 1.0
	}

	targetSet := make(map[string]struct{})
	for _, w := range strings.Fields(strings.ToLower(target)) {
		targetSet[w] = struct{}{}
	}

	matches := 0
	for _, w := range sourceWords {
		if _, ok := targetSet[w]; ok {
			matches++
		}
	}

	return float64(matches) / float64(len(sourceWords))
}

// --- Identifier Extraction ---

// identifierPatterns matches file paths, UUIDs, URLs, and other opaque identifiers.
var identifierPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:^|[\s"'` + "`" + `])(/[a-zA-Z0-9._/-]{3,})`),                               // absolute file paths
	regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`),                 // UUIDs
	regexp.MustCompile(`https?://[^\s"'<>)]+`),                                                          // URLs
	regexp.MustCompile(`[0-9a-f]{7,40}`),                                                                // commit hashes (7-40 hex chars)
	regexp.MustCompile(`(?:^|[\s"'` + "`" + `])((?:\./|\.\./)(?:[a-zA-Z0-9._/-]+[a-zA-Z0-9._/]){1,})`), // relative paths
}

// extractIdentifiers extracts opaque identifiers (file paths, UUIDs, URLs) from
// the last few messages. Returns up to maxCount unique identifiers.
func extractIdentifiers(messages []chatMessage, maxCount int) []string {
	if maxCount <= 0 {
		maxCount = 20
	}

	seen := make(map[string]struct{})
	var identifiers []string

	// Process messages from end (most recent first).
	for i := len(messages) - 1; i >= 0 && len(identifiers) < maxCount; i-- {
		s, ok := messages[i].Content.(string)
		if !ok {
			continue
		}

		for _, re := range identifierPatterns {
			matches := re.FindAllStringSubmatch(s, -1)
			for _, match := range matches {
				// Use submatch if available (group 1), otherwise full match.
				id := match[0]
				if len(match) > 1 && match[1] != "" {
					id = match[1]
				}
				id = strings.TrimSpace(id)
				if id == "" || len(id) < 3 {
					continue
				}
				if _, exists := seen[id]; !exists {
					seen[id] = struct{}{}
					identifiers = append(identifiers, id)
					if len(identifiers) >= maxCount {
						return identifiers
					}
				}
			}
		}
	}

	return identifiers
}

// --- Context Pruning ---

// pruneByContextRatio applies ratio-based in-memory pruning of tool results.
// Phase 1 (>= softTrimRatio): soft trim tool results exceeding softTrimMaxChars
// to head 1500 + tail 1500 chars.
// Phase 2 (>= hardClearRatio): replace old tool results with a placeholder.
// Protected messages (first user, recent assistant turns) are never pruned.
func pruneByContextRatio(messages []chatMessage, estimatedTokens, contextWindow int, cfg ContextPruningConfig) []chatMessage {
	if contextWindow <= 0 || estimatedTokens <= 0 {
		return messages
	}

	ratio := float64(estimatedTokens) / float64(contextWindow)

	if ratio < cfg.SoftTrimRatio {
		return messages // Below threshold, no pruning needed.
	}

	protected := buildProtectedSet(messages, cfg.ProtectRecentTurns)
	result := make([]chatMessage, len(messages))
	copy(result, messages)

	const (
		softKeepHead = 1500
		softKeepTail = 1500
	)

	// Phase 1: Soft trim — truncate oversized tool results to head+tail.
	// Always runs first to reduce size without losing all content.
	trimmed := 0
	for i := range result {
		if protected[i] || result[i].Role != "tool" {
			continue
		}
		s, ok := result[i].Content.(string)
		if !ok || len(s) <= cfg.SoftTrimMaxChars {
			continue
		}
		keepHead := softKeepHead
		keepTail := softKeepTail
		if keepHead+keepTail >= len(s) {
			continue
		}
		head := s[:keepHead]
		tail := s[len(s)-keepTail:]
		result[i].Content = head + "\n...[trimmed " + fmt.Sprintf("%d", len(s)-keepHead-keepTail) + " chars]...\n" + tail
		trimmed++
	}

	// Phase 2: Hard clear — replace old tool results with a placeholder.
	// Only runs if ratio is above HardClearRatio AND after soft trim has been applied.
	if ratio >= cfg.HardClearRatio {
		for i := range result {
			if protected[i] || result[i].Role != "tool" {
				continue
			}
			if _, ok := result[i].Content.(string); !ok {
				continue
			}
			result[i] = chatMessage{
				Role:       result[i].Role,
				Content:    "[Old tool result content cleared]",
				ToolCallID: result[i].ToolCallID,
			}
		}
	}

	return result
}

// buildProtectedSet returns a set of message indices that should never be pruned.
// Protected: first user message (goal), last N assistant turns, and any multimodal content.
func buildProtectedSet(messages []chatMessage, protectRecentTurns int) map[int]bool {
	protected := make(map[int]bool)

	// Protect the first user message (goal).
	for i, m := range messages {
		if m.Role == "user" {
			protected[i] = true
			break
		}
	}

	// Protect system messages.
	for i, m := range messages {
		if m.Role == "system" {
			protected[i] = true
		}
	}

	// Protect last N assistant turns and their associated tool results.
	assistantCount := 0
	for i := len(messages) - 1; i >= 0 && assistantCount < protectRecentTurns; i-- {
		protected[i] = true
		if messages[i].Role == "assistant" {
			assistantCount++
		}
	}

	// Protect multimodal content (non-string content).
	for i, m := range messages {
		if _, ok := m.Content.(string); !ok && m.Content != nil {
			protected[i] = true
		}
	}

	return protected
}

// buildMinimalFallbackSummary creates a metadata-only summary when LLM summarization fails
// during emergency compression. Lists message counts and tool names instead of discarding
// all history without context.
func buildMinimalFallbackSummary(messages []chatMessage) string {
	var b strings.Builder
	b.WriteString("## Decisions\n(LLM summarization failed — metadata only)\n\n")
	b.WriteString("## Open TODOs\n(unknown — summarization failed)\n\n")
	b.WriteString("## Constraints/Rules\n(unknown — summarization failed)\n\n")
	b.WriteString("## Pending user asks\n")

	// Find the last user message.
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			if s, ok := messages[i].Content.(string); ok {
				preview := s
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				b.WriteString(preview)
			}
			break
		}
	}
	b.WriteString("\n\n")

	b.WriteString("## Exact identifiers\n")

	// Collect tool names used.
	toolNames := make(map[string]int)
	userCount, assistantCount, toolCount := 0, 0, 0
	for _, m := range messages {
		switch m.Role {
		case "user":
			userCount++
		case "assistant":
			assistantCount++
			for _, tc := range m.ToolCalls {
				toolNames[tc.Function.Name]++
			}
		case "tool":
			toolCount++
		}
	}

	b.WriteString(fmt.Sprintf("\nConversation stats: %d user, %d assistant, %d tool messages.\n", userCount, assistantCount, toolCount))
	if len(toolNames) > 0 {
		b.WriteString("Tools used: ")
		names := make([]string, 0, len(toolNames))
		for name, count := range toolNames {
			names = append(names, fmt.Sprintf("%s(%d)", name, count))
		}
		sort.Strings(names)
		b.WriteString(strings.Join(names, ", "))
		b.WriteString("\n")
	}

	// Extract identifiers from messages.
	ids := extractIdentifiers(messages, 20)
	if len(ids) > 0 {
		for _, id := range ids {
			b.WriteString("- " + id + "\n")
		}
	}

	return b.String()
}

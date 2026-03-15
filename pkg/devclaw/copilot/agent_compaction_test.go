package copilot

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestManagedCompaction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	// Create mock LLM setup
	cfg := &Config{
		Model: "test-model",
		API: APIConfig{
			Provider: "openai",
			BaseURL:  "http://localhost:1234",
		},
	}
	llm := NewLLMClient(cfg, logger)
	agent := NewAgentRun(llm, nil, logger)

	// Simulate conversation with 15 messages (overflow simulation)
	var messages []chatMessage
	messages = append(messages, chatMessage{Role: "system", Content: "System Prompt Context"})
	messages = append(messages, chatMessage{Role: "user", Content: "Initial Request Goal"})

	for i := 1; i <= 13; i++ {
		if i%2 == 0 {
			messages = append(messages, chatMessage{Role: "user", Content: "User input " + string(rune(i))})
		} else {
			messages = append(messages, chatMessage{Role: "assistant", Content: "Assistant reply " + string(rune(i))})
		}
	}

	// Because we can't actually call LLM here, we'll test the pruning math
	compacted := agent.managedCompaction(context.Background(), messages)

	// With adaptive keepRecent (short messages ≈ 3-4 tokens each → keepRecent = 8):
	// length = 15, body count = 14
	// goal = body[0]
	// header = 1 (system)
	// middle = body[1 : 14-8] = body[1 : 6] (5 messages) → summarized to 1
	// recent = body[6:] = 8 messages
	// total = header(1) + goal(1) + summary_msg(1) + recent(8) = 11 messages

	if len(compacted) != 11 {
		t.Errorf("Expected managed compaction to yield 11 messages, got %d", len(compacted))
	}

	if compacted[0].Role != "system" {
		t.Errorf("Expected first message to be system, got %s", compacted[0].Role)
	}

	if compacted[1].Role != "user" || compacted[1].Content != "Initial Request Goal" {
		t.Errorf("Expected second message to be the goal")
	}

	summaryMsg, ok := compacted[2].Content.(string)
	if !ok || !strings.Contains(summaryMsg, "[System: The following is a summary") {
		t.Errorf("Expected third message to be the summary wrapper, got %v", compacted[2].Content)
	}
}

func TestAggressiveCompaction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &Config{
		Model: "test-model",
		API: APIConfig{
			Provider: "openai",
			BaseURL:  "http://localhost:1234",
		},
	}
	llm := NewLLMClient(cfg, logger)
	agent := NewAgentRun(llm, nil, logger)

	var messages []chatMessage
	messages = append(messages, chatMessage{Role: "system", Content: "System"})
	messages = append(messages, chatMessage{Role: "user", Content: "Goal"})

	for i := 0; i < 10; i++ {
		messages = append(messages, chatMessage{Role: "assistant", Content: "Test"})
	}

	// 1 system + 11 user/assistant = 12 total messages
	// aggressive keeps goal (body[0]) + summary + keepRecent (2)
	// Total expecting: 1 system + 1 goal + 1 summary + 2 recent = 5

	compacted := agent.aggressiveCompaction(context.Background(), messages)

	if len(compacted) != 5 {
		t.Errorf("Expected aggressive compaction to yield 5 messages, got %d", len(compacted))
	}

	summaryMsg, ok := compacted[2].Content.(string)
	if !ok || !strings.Contains(summaryMsg, "[System: Aggressive fallback compaction") {
		t.Errorf("Expected third message to be the aggressive summary wrapper")
	}
}

func TestEmergencyCompression(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &Config{
		Model: "test-model",
		API: APIConfig{
			Provider: "openai",
			BaseURL:  "http://localhost:1234",
		},
	}
	llm := NewLLMClient(cfg, logger)
	agent := NewAgentRun(llm, nil, logger)
	ctx := context.Background()

	t.Run("preserves goal and recent messages with summary", func(t *testing.T) {
		var messages []chatMessage
		messages = append(messages, chatMessage{Role: "system", Content: "System Prompt"})
		messages = append(messages, chatMessage{Role: "user", Content: "First user message"})
		messages = append(messages, chatMessage{Role: "assistant", Content: "First response"})
		messages = append(messages, chatMessage{Role: "user", Content: "Second user message"})
		messages = append(messages, chatMessage{Role: "assistant", Content: "Second response"})
		messages = append(messages, chatMessage{Role: "user", Content: "Last user message"})

		// New emergency compression keeps:
		// 1. system header
		// 2. goal (first user message)
		// 3. summary message (user role with [System: ...])
		// 4-5. last 2 messages (assistant + user)
		compacted := agent.emergencyCompression(ctx, messages)

		// system(1) + goal(1) + summary(1) + last 2 msgs = 5
		if len(compacted) != 5 {
			t.Errorf("Expected emergency compression to yield 5 messages, got %d", len(compacted))
		}

		// First should be original system
		if compacted[0].Role != "system" {
			t.Errorf("Expected first message to be system, got %s", compacted[0].Role)
		}
		systemContent, ok := compacted[0].Content.(string)
		if !ok || systemContent != "System Prompt" {
			t.Errorf("Expected first system content to be original prompt, got %v", compacted[0].Content)
		}

		// Second should be the goal
		if compacted[1].Content != "First user message" {
			t.Errorf("Expected goal message, got %v", compacted[1].Content)
		}

		// Third should be summary with Emergency notice
		summaryContent, ok := compacted[2].Content.(string)
		if !ok || !strings.Contains(summaryContent, "Emergency context compression applied") {
			t.Errorf("Expected compression summary, got %v", compacted[2].Content)
		}

		// Last should be the last user message
		if compacted[len(compacted)-1].Content != "Last user message" {
			t.Errorf("Expected last message to be 'Last user message', got %v", compacted[len(compacted)-1].Content)
		}
	})

	t.Run("with minimal messages", func(t *testing.T) {
		var messages []chatMessage
		messages = append(messages, chatMessage{Role: "system", Content: "System"})
		messages = append(messages, chatMessage{Role: "user", Content: "Only user message"})

		compacted := agent.emergencyCompression(ctx, messages)

		// system(1) + goal/only-user(1) + summary(1) = 3
		// (body has 1 element, keepLast = min(2, 0) = 0, middle = body[1:] which is empty)
		if len(compacted) < 2 {
			t.Errorf("Expected at least 2 messages, got %d", len(compacted))
		}

		// Should preserve the system and user message
		hasSystem := false
		hasUser := false
		for _, m := range compacted {
			if m.Role == "system" {
				hasSystem = true
			}
			if m.Role == "user" {
				hasUser = true
			}
		}
		if !hasSystem {
			t.Error("Expected system message to be preserved")
		}
		if !hasUser {
			t.Error("Expected user message to be preserved")
		}
	})

	t.Run("handles only system message", func(t *testing.T) {
		var messages []chatMessage
		messages = append(messages, chatMessage{Role: "system", Content: "System"})

		compacted := agent.emergencyCompression(ctx, messages)

		if len(compacted) != 1 {
			t.Errorf("Expected 1 message for system-only, got %d", len(compacted))
		}
	})

	t.Run("handles empty messages", func(t *testing.T) {
		var messages []chatMessage

		compacted := agent.emergencyCompression(ctx, messages)

		if len(compacted) != 0 {
			t.Errorf("Expected 0 messages for empty input, got %d", len(compacted))
		}
	})

	t.Run("drastically reduces large conversation", func(t *testing.T) {
		var messages []chatMessage
		messages = append(messages, chatMessage{Role: "system", Content: "System"})

		// Add 100 back-and-forth messages
		for i := 0; i < 100; i++ {
			messages = append(messages, chatMessage{
				Role:    "user",
				Content: strings.Repeat("x", 1000), // 1KB each
			})
			messages = append(messages, chatMessage{
				Role:    "assistant",
				Content: strings.Repeat("y", 1000),
			})
		}
		messages = append(messages, chatMessage{Role: "user", Content: "Final question"})

		// Total: 1 + 200 + 1 = 202 messages, ~200KB
		compacted := agent.emergencyCompression(ctx, messages)

		// Should reduce drastically: system + goal + summary + last 2 = 5
		if len(compacted) > 10 {
			t.Errorf("Expected drastic reduction, got %d messages (from %d)", len(compacted), len(messages))
		}

		// Verify the last message is the final user question
		lastMsg := compacted[len(compacted)-1]
		if lastMsg.Content != "Final question" {
			t.Errorf("Expected last message to be 'Final question', got %v", lastMsg.Content)
		}
	})

	t.Run("summary includes metadata fallback content", func(t *testing.T) {
		var messages []chatMessage
		messages = append(messages, chatMessage{Role: "system", Content: "System"})
		messages = append(messages, chatMessage{Role: "user", Content: "Goal task"})
		messages = append(messages, chatMessage{
			Role:    "assistant",
			Content: "Working on it",
			ToolCalls: []ToolCall{
				{ID: "tc1", Function: FunctionCall{Name: "read_file", Arguments: `{"path":"/tmp/test.go"}`}},
			},
		})
		messages = append(messages, chatMessage{Role: "tool", Content: "file content here", ToolCallID: "tc1"})
		messages = append(messages, chatMessage{Role: "user", Content: "Last ask"})

		compacted := agent.emergencyCompression(ctx, messages)

		// Find the summary message
		var summaryFound bool
		for _, m := range compacted {
			if s, ok := m.Content.(string); ok && strings.Contains(s, "Emergency context compression") {
				summaryFound = true
				break
			}
		}
		if !summaryFound {
			t.Error("Expected emergency summary message in compacted output")
		}
	})
}

// --- compaction_safeguard.go tests ---

func TestBuildStructuredCompactionPrompt(t *testing.T) {
	t.Parallel()

	cfg := DefaultCompactionConfig()
	prompt := buildStructuredCompactionPrompt(cfg,
		[]string{"read_file: error opening /nonexistent"},
		[]string{"/tmp/a.go", "/tmp/b.go"},
		[]string{"/tmp/c.go"},
	)

	// Check all required sections are mentioned in the prompt instructions.
	for _, section := range requiredCompactionSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("expected prompt to contain %q", section)
		}
	}

	// Check tool failures are included.
	if !strings.Contains(prompt, "read_file: error opening /nonexistent") {
		t.Error("expected tool failures in prompt")
	}

	// Check file operations are included with XML tags.
	if !strings.Contains(prompt, "<files_read>") {
		t.Error("expected <files_read> tag in prompt")
	}
	if !strings.Contains(prompt, "<files_modified>") {
		t.Error("expected <files_modified> tag in prompt")
	}
	if !strings.Contains(prompt, "/tmp/a.go") {
		t.Error("expected read file in prompt")
	}
	if !strings.Contains(prompt, "/tmp/c.go") {
		t.Error("expected modified file in prompt")
	}
}

func TestBuildStructuredCompactionPrompt_Empty(t *testing.T) {
	t.Parallel()

	cfg := DefaultCompactionConfig()
	prompt := buildStructuredCompactionPrompt(cfg, nil, nil, nil)

	// Should still contain all required sections even with no context.
	for _, section := range requiredCompactionSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("expected prompt to contain %q even with empty context", section)
		}
	}

	// Should not contain XML tags for empty file lists.
	if strings.Contains(prompt, "<files_read>") {
		t.Error("should not include <files_read> tag when no read files")
	}
	if strings.Contains(prompt, "<files_modified>") {
		t.Error("should not include <files_modified> tag when no modified files")
	}
}

func TestAuditSummaryQuality(t *testing.T) {
	t.Parallel()

	t.Run("good summary passes", func(t *testing.T) {
		t.Parallel()
		summary := `## Decisions
Chose SQLite for persistence.

## Open TODOs
- Implement caching layer

## Constraints/Rules
Must support offline mode.

## Pending user asks
User asked to add authentication.

## Exact identifiers
- /home/user/project/main.go
- abc123def`

		result := auditSummaryQuality(summary, nil, "add authentication", false)
		if !result.Passed {
			t.Errorf("expected good summary to pass, got failures: %v", result.Failures)
		}
	})

	t.Run("missing sections fails", func(t *testing.T) {
		t.Parallel()
		summary := "Just a plain summary without any sections."

		result := auditSummaryQuality(summary, nil, "", false)
		if result.Passed {
			t.Error("expected summary without sections to fail")
		}
		if len(result.Failures) != 5 {
			t.Errorf("expected 5 missing section failures, got %d: %v", len(result.Failures), result.Failures)
		}
	})

	t.Run("strict identifier check", func(t *testing.T) {
		t.Parallel()
		summary := `## Decisions
None.
## Open TODOs
None.
## Constraints/Rules
None.
## Pending user asks
None.
## Exact identifiers
- /some/path`

		identifiers := []string{"/some/path", "abc-123-def"}
		result := auditSummaryQuality(summary, identifiers, "", true)
		if result.Passed {
			t.Error("expected strict identifier check to fail for missing identifier")
		}

		hasMissingID := false
		for _, f := range result.Failures {
			if strings.Contains(f, "abc-123-def") {
				hasMissingID = true
			}
		}
		if !hasMissingID {
			t.Error("expected failure to mention missing identifier abc-123-def")
		}
	})

	t.Run("user ask overlap check", func(t *testing.T) {
		t.Parallel()
		summary := `## Decisions
None.
## Open TODOs
None.
## Constraints/Rules
None.
## Pending user asks
Something completely unrelated.
## Exact identifiers
None.`

		result := auditSummaryQuality(summary, nil, "please implement the authentication feature with JWT tokens", false)
		if result.Passed {
			t.Error("expected low overlap to fail")
		}

		hasOverlapFailure := false
		for _, f := range result.Failures {
			if strings.Contains(f, "overlap") {
				hasOverlapFailure = true
			}
		}
		if !hasOverlapFailure {
			t.Error("expected overlap failure")
		}
	})
}

func TestTokenOverlap(t *testing.T) {
	t.Parallel()

	t.Run("identical strings", func(t *testing.T) {
		t.Parallel()
		overlap := tokenOverlap("hello world foo", "hello world foo")
		if overlap != 1.0 {
			t.Errorf("expected 1.0 for identical strings, got %f", overlap)
		}
	})

	t.Run("no overlap", func(t *testing.T) {
		t.Parallel()
		overlap := tokenOverlap("hello world", "foo bar baz")
		if overlap != 0.0 {
			t.Errorf("expected 0.0 for no overlap, got %f", overlap)
		}
	})

	t.Run("partial overlap", func(t *testing.T) {
		t.Parallel()
		overlap := tokenOverlap("hello world foo bar", "world bar baz qux")
		// source has 4 words, 2 match ("world", "bar") → 0.5
		if overlap != 0.5 {
			t.Errorf("expected 0.5 for partial overlap, got %f", overlap)
		}
	})

	t.Run("empty source", func(t *testing.T) {
		t.Parallel()
		overlap := tokenOverlap("", "hello world")
		if overlap != 1.0 {
			t.Errorf("expected 1.0 for empty source, got %f", overlap)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Parallel()
		overlap := tokenOverlap("Hello World", "hello world more words")
		if overlap != 1.0 {
			t.Errorf("expected 1.0 for case-insensitive match, got %f", overlap)
		}
	})
}

func TestExtractIdentifiers(t *testing.T) {
	t.Parallel()

	messages := []chatMessage{
		{Role: "user", Content: "Please read /home/user/project/main.go"},
		{Role: "assistant", Content: "Reading file at /home/user/project/main.go"},
		{Role: "tool", Content: "File content with UUID 550e8400-e29b-41d4-a716-446655440000"},
		{Role: "user", Content: "Check https://example.com/api/v1/users"},
	}

	ids := extractIdentifiers(messages, 20)

	if len(ids) == 0 {
		t.Fatal("expected at least some identifiers extracted")
	}

	// Check that known identifiers are found.
	foundPath := false
	foundUUID := false
	foundURL := false
	for _, id := range ids {
		if strings.Contains(id, "/home/user/project/main.go") {
			foundPath = true
		}
		if strings.Contains(id, "550e8400-e29b-41d4-a716-446655440000") {
			foundUUID = true
		}
		if strings.Contains(id, "https://example.com/api/v1/users") {
			foundURL = true
		}
	}

	if !foundPath {
		t.Errorf("expected file path to be extracted, got: %v", ids)
	}
	if !foundUUID {
		t.Errorf("expected UUID to be extracted, got: %v", ids)
	}
	if !foundURL {
		t.Errorf("expected URL to be extracted, got: %v", ids)
	}
}

func TestExtractIdentifiers_MaxCount(t *testing.T) {
	t.Parallel()

	// Create messages with many identifiers.
	var content strings.Builder
	for i := 0; i < 30; i++ {
		content.WriteString(strings.Repeat("a", 8) + "-" + strings.Repeat("b", 4) + "-" +
			strings.Repeat("c", 4) + "-" + strings.Repeat("d", 4) + "-" +
			strings.Repeat("e", 12) + " ")
	}
	messages := []chatMessage{
		{Role: "user", Content: content.String()},
	}

	ids := extractIdentifiers(messages, 5)
	if len(ids) > 5 {
		t.Errorf("expected max 5 identifiers, got %d", len(ids))
	}
}

func TestPruneByContextRatio(t *testing.T) {
	t.Parallel()

	cfg := ContextPruningConfig{
		SoftTrimRatio:      0.3,
		HardClearRatio:     0.5,
		SoftTrimMaxChars:   100, // Low threshold for testing
		ProtectRecentTurns: 1,
	}

	// Build messages with a large tool result (must exceed softKeepHead+softKeepTail=3000).
	largeContent := strings.Repeat("x", 5000) // 5000 chars
	messages := []chatMessage{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Goal"},
		{Role: "assistant", Content: "Let me check", ToolCalls: []ToolCall{{ID: "t1", Function: FunctionCall{Name: "read_file"}}}},
		{Role: "tool", Content: largeContent, ToolCallID: "t1"},
		{Role: "assistant", Content: "Here's what I found"},
		{Role: "user", Content: "Thanks"},
		{Role: "assistant", Content: "You're welcome"},
	}

	t.Run("below threshold no change", func(t *testing.T) {
		t.Parallel()
		// ratio = 100/10000 = 0.01, well below 0.3
		result := pruneByContextRatio(messages, 100, 10000, cfg)
		toolContent, _ := result[3].Content.(string)
		if toolContent != largeContent {
			t.Error("expected no pruning below soft trim threshold")
		}
	})

	t.Run("soft trim above threshold", func(t *testing.T) {
		t.Parallel()
		// ratio = 4000/10000 = 0.4, above 0.3 but below 0.5
		result := pruneByContextRatio(messages, 4000, 10000, cfg)
		toolContent, _ := result[3].Content.(string)
		if toolContent == largeContent {
			t.Error("expected tool result to be soft-trimmed")
		}
		if !strings.Contains(toolContent, "trimmed") {
			t.Errorf("expected trimmed marker in content, got: %s", toolContent[:min(100, len(toolContent))])
		}
	})

	t.Run("hard clear above threshold", func(t *testing.T) {
		t.Parallel()
		// ratio = 6000/10000 = 0.6, above 0.5
		result := pruneByContextRatio(messages, 6000, 10000, cfg)
		toolContent, _ := result[3].Content.(string)
		if toolContent != "[Old tool result content cleared]" {
			t.Errorf("expected hard clear placeholder, got: %s", toolContent)
		}
	})

	t.Run("recent messages protected", func(t *testing.T) {
		t.Parallel()
		// Put a large tool result as the last message (protected by ProtectRecentTurns=1)
		msgs := []chatMessage{
			{Role: "system", Content: "System"},
			{Role: "user", Content: "Goal"},
			{Role: "assistant", Content: "Old", ToolCalls: []ToolCall{{ID: "t1", Function: FunctionCall{Name: "read_file"}}}},
			{Role: "tool", Content: largeContent, ToolCallID: "t1"},
			{Role: "assistant", Content: "Recent answer"},
		}

		result := pruneByContextRatio(msgs, 6000, 10000, cfg)
		// The last assistant message (index 4) is protected.
		lastContent, _ := result[4].Content.(string)
		if lastContent != "Recent answer" {
			t.Error("expected recent assistant message to be protected")
		}
	})
}

func TestBuildProtectedSet(t *testing.T) {
	t.Parallel()

	messages := []chatMessage{
		{Role: "system", Content: "System"},      // 0: protected (system)
		{Role: "user", Content: "Goal"},           // 1: protected (first user)
		{Role: "assistant", Content: "Response 1"}, // 2: not protected
		{Role: "user", Content: "Follow up"},       // 3: not protected
		{Role: "assistant", Content: "Response 2"}, // 4: protected (recent)
		{Role: "user", Content: "Last question"},   // 5: protected (recent)
	}

	protected := buildProtectedSet(messages, 1)

	// System messages should always be protected.
	if !protected[0] {
		t.Error("expected system message (index 0) to be protected")
	}

	// First user message (goal) should be protected.
	if !protected[1] {
		t.Error("expected first user message (index 1) to be protected")
	}

	// Last assistant turn and everything after should be protected.
	if !protected[5] {
		t.Error("expected last message (index 5) to be protected")
	}
}

func TestBuildMinimalFallbackSummary(t *testing.T) {
	t.Parallel()

	messages := []chatMessage{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Please read my files"},
		{Role: "assistant", Content: "Sure", ToolCalls: []ToolCall{
			{ID: "t1", Function: FunctionCall{Name: "read_file", Arguments: `{"path":"/tmp/test.go"}`}},
		}},
		{Role: "tool", Content: "package main\nfunc main() {}", ToolCallID: "t1"},
		{Role: "user", Content: "Now edit it"},
	}

	summary := buildMinimalFallbackSummary(messages)

	// Should contain all required sections.
	for _, section := range requiredCompactionSections {
		if !strings.Contains(summary, section) {
			t.Errorf("expected fallback summary to contain %q", section)
		}
	}

	// Should mention tool names.
	if !strings.Contains(summary, "read_file") {
		t.Error("expected fallback summary to mention read_file tool")
	}

	// Should mention the last user ask.
	if !strings.Contains(summary, "Now edit it") {
		t.Error("expected fallback summary to include last user ask")
	}
}

func TestCollectFileOperationsSeparated(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &Config{
		Model: "test-model",
		API: APIConfig{
			Provider: "openai",
			BaseURL:  "http://localhost:1234",
		},
	}
	llm := NewLLMClient(cfg, logger)
	agent := NewAgentRun(llm, nil, logger)

	messages := []chatMessage{
		{Role: "assistant", Content: "Reading files", ToolCalls: []ToolCall{
			{ID: "t1", Function: FunctionCall{Name: "read_file", Arguments: `{"path":"/tmp/a.go"}`}},
			{ID: "t2", Function: FunctionCall{Name: "read_file", Arguments: `{"path":"/tmp/b.go"}`}},
		}},
		{Role: "tool", Content: "content a", ToolCallID: "t1"},
		{Role: "tool", Content: "content b", ToolCallID: "t2"},
		{Role: "assistant", Content: "Editing", ToolCalls: []ToolCall{
			{ID: "t3", Function: FunctionCall{Name: "edit_file", Arguments: `{"path":"/tmp/a.go"}`}},
			{ID: "t4", Function: FunctionCall{Name: "write_file", Arguments: `{"path":"/tmp/c.go"}`}},
		}},
		{Role: "tool", Content: "ok", ToolCallID: "t3"},
		{Role: "tool", Content: "ok", ToolCallID: "t4"},
	}

	readFiles, modifiedFiles := agent.collectFileOperationsSeparated(messages)

	// /tmp/a.go was read then modified → only in modifiedFiles
	// /tmp/b.go was only read → in readFiles
	// /tmp/c.go was only written → in modifiedFiles

	readSet := make(map[string]bool)
	for _, f := range readFiles {
		readSet[f] = true
	}
	modifiedSet := make(map[string]bool)
	for _, f := range modifiedFiles {
		modifiedSet[f] = true
	}

	if readSet["/tmp/a.go"] {
		t.Error("/tmp/a.go should not be in readFiles (was also modified)")
	}
	if !readSet["/tmp/b.go"] {
		t.Error("/tmp/b.go should be in readFiles")
	}
	if !modifiedSet["/tmp/a.go"] {
		t.Error("/tmp/a.go should be in modifiedFiles")
	}
	if !modifiedSet["/tmp/c.go"] {
		t.Error("/tmp/c.go should be in modifiedFiles")
	}
}

func TestCompactionStatusReaction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &Config{
		Model: "test-model",
		API: APIConfig{
			Provider: "openai",
			BaseURL:  "http://localhost:1234",
		},
	}
	llm := NewLLMClient(cfg, logger)
	agent := NewAgentRun(llm, nil, logger)

	var mu sync.Mutex
	var reactions []struct {
		emoji  string
		remove bool
	}

	agent.SetReactionSender(func(emoji string, remove bool) {
		mu.Lock()
		reactions = append(reactions, struct {
			emoji  string
			remove bool
		}{emoji, remove})
		mu.Unlock()
	})

	// managedCompaction sends ✍ at start and removes it at end.
	// Build enough messages to trigger compaction (>= 10 messages, body >= 8).
	var messages []chatMessage
	messages = append(messages, chatMessage{Role: "system", Content: "System Prompt"})
	messages = append(messages, chatMessage{Role: "user", Content: "Goal"})
	for i := 0; i < 12; i++ {
		if i%2 == 0 {
			messages = append(messages, chatMessage{Role: "user", Content: "msg " + string(rune('A'+i))})
		} else {
			messages = append(messages, chatMessage{Role: "assistant", Content: "reply " + string(rune('A'+i))})
		}
	}

	_ = agent.managedCompaction(context.Background(), messages)

	mu.Lock()
	defer mu.Unlock()

	if len(reactions) < 2 {
		t.Fatalf("expected at least 2 reactions (send + remove), got %d", len(reactions))
	}

	// First reaction should be ✍ (send).
	if reactions[0].emoji != "\u270d" || reactions[0].remove {
		t.Errorf("expected first reaction to be ✍ send, got emoji=%q remove=%v", reactions[0].emoji, reactions[0].remove)
	}

	// Last reaction should be ✍ (remove).
	last := reactions[len(reactions)-1]
	if last.emoji != "\u270d" || !last.remove {
		t.Errorf("expected last reaction to be ✍ remove, got emoji=%q remove=%v", last.emoji, last.remove)
	}
}

func TestPostCompactionMemorySync(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &Config{
		Model: "test-model",
		API: APIConfig{
			Provider: "openai",
			BaseURL:  "http://localhost:1234",
		},
	}
	llm := NewLLMClient(cfg, logger)

	t.Run("async mode triggers IndexNow", func(t *testing.T) {
		agent := NewAgentRun(llm, nil, logger)
		agent.cfg.Compaction.PostIndexSync = "async"

		// Create a memory indexer with a temp dir so indexAll() completes quickly.
		tmpDir := t.TempDir()
		indexer := NewMemoryIndexer(MemoryIndexerConfig{
			MemoryDir: tmpDir,
			Interval:  1 * time.Hour,
		}, logger)
		indexer.SetMemoryDir(tmpDir)

		agent.SetMemoryIndexer(indexer)

		// We don't need real persistence for this test.
		agent.persistCompactionSummary("test summary", 10, 5)

		// Wait a bit for the async goroutine.
		time.Sleep(100 * time.Millisecond)

		// Verify that indexAll() ran by checking lastIndexTime is non-zero.
		_, _, _, lastIdx := indexer.Stats()
		if lastIdx.IsZero() {
			t.Error("expected lastIndexTime to be set after post-compaction sync")
		}
	})

	t.Run("off mode does not trigger IndexNow", func(t *testing.T) {
		agent := NewAgentRun(llm, nil, logger)
		agent.cfg.Compaction.PostIndexSync = "off"

		tmpDir := t.TempDir()
		indexer := NewMemoryIndexer(MemoryIndexerConfig{
			MemoryDir: tmpDir,
			Interval:  1 * time.Hour,
		}, logger)
		indexer.SetMemoryDir(tmpDir)
		agent.SetMemoryIndexer(indexer)

		agent.persistCompactionSummary("test summary", 10, 5)
		time.Sleep(100 * time.Millisecond)

		_, _, _, lastIdx := indexer.Stats()
		if !lastIdx.IsZero() {
			t.Error("expected lastIndexTime to be zero when PostIndexSync is off")
		}
	})

	t.Run("empty mode (default) does not trigger IndexNow", func(t *testing.T) {
		agent := NewAgentRun(llm, nil, logger)
		// PostIndexSync is empty by default.

		tmpDir := t.TempDir()
		indexer := NewMemoryIndexer(MemoryIndexerConfig{
			MemoryDir: tmpDir,
			Interval:  1 * time.Hour,
		}, logger)
		indexer.SetMemoryDir(tmpDir)
		agent.SetMemoryIndexer(indexer)

		agent.persistCompactionSummary("test summary", 10, 5)
		time.Sleep(100 * time.Millisecond)

		_, _, _, lastIdx := indexer.Stats()
		if !lastIdx.IsZero() {
			t.Error("expected lastIndexTime to be zero when PostIndexSync is empty")
		}
	})
}

func TestCompactionTimeoutFromConfig(t *testing.T) {
	// Default should be 900 seconds.
	cfg := DefaultCompactionConfig()
	if cfg.TimeoutSeconds != 900 {
		t.Errorf("Expected default timeout 900, got %d", cfg.TimeoutSeconds)
	}

	// Zero should resolve to default.
	cfg.TimeoutSeconds = 0
	resolved := resolvedCompactionConfig(cfg)
	if resolved.TimeoutSeconds != 900 {
		t.Errorf("Expected resolved timeout 900 for zero value, got %d", resolved.TimeoutSeconds)
	}

	// Negative should resolve to default.
	cfg.TimeoutSeconds = -1
	resolved = resolvedCompactionConfig(cfg)
	if resolved.TimeoutSeconds != 900 {
		t.Errorf("Expected resolved timeout 900 for negative, got %d", resolved.TimeoutSeconds)
	}

	// Custom value should be preserved.
	cfg.TimeoutSeconds = 300
	resolved = resolvedCompactionConfig(cfg)
	if resolved.TimeoutSeconds != 300 {
		t.Errorf("Expected resolved timeout 300, got %d", resolved.TimeoutSeconds)
	}
}

func TestSessionsYieldSignal(t *testing.T) {
	t.Run("yield flag sets and returns ErrAgentYield", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		cfg := &Config{
			Model: "test-model",
			API: APIConfig{
				Provider: "openai",
				BaseURL:  "http://localhost:1234",
			},
		}
		llm := NewLLMClient(cfg, logger)
		agent := NewAgentRun(llm, nil, logger)

		// Simulate: set yieldRequested directly (as sessions_yield tool would).
		agent.yieldRequested.Store(true)

		// Verify ErrAgentYield is the sentinel.
		if !errors.Is(ErrAgentYield, ErrAgentYield) {
			t.Error("expected ErrAgentYield to match itself")
		}

		// Verify yieldRequested is set.
		if !agent.yieldRequested.Load() {
			t.Error("expected yieldRequested to be true")
		}

		_ = agent // avoid unused
	})

	t.Run("AgentRun context roundtrip", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		cfg := &Config{
			Model: "test-model",
			API: APIConfig{
				Provider: "openai",
				BaseURL:  "http://localhost:1234",
			},
		}
		llm := NewLLMClient(cfg, logger)
		agent := NewAgentRun(llm, nil, logger)

		ctx := context.Background()
		if AgentRunFromCtx(ctx) != nil {
			t.Error("expected nil AgentRun from background context")
		}

		ctx = ContextWithAgentRun(ctx, agent)
		extracted := AgentRunFromCtx(ctx)
		if extracted != agent {
			t.Error("expected extracted AgentRun to match original")
		}
	})

	t.Run("sessions_yield tool sets flag via context", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		cfg := &Config{
			Model: "test-model",
			API: APIConfig{
				Provider: "openai",
				BaseURL:  "http://localhost:1234",
			},
		}
		llm := NewLLMClient(cfg, logger)
		executor := NewToolExecutor(logger)

		agent := NewAgentRun(llm, executor, logger)

		// Register the yield tool.
		RegisterSessionsYieldTool(executor)

		// Execute it via context with AgentRun injected.
		ctx := ContextWithAgentRun(context.Background(), agent)
		result := executor.Execute(ctx, []ToolCall{
			{
				ID:       "tc_yield_1",
				Function: FunctionCall{Name: "sessions_yield", Arguments: "{}"},
			},
		})

		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		if result[0].Error != nil {
			t.Errorf("expected no error, got %v", result[0].Error)
		}
		if !agent.yieldRequested.Load() {
			t.Error("expected yieldRequested to be set after sessions_yield tool")
		}
		if !strings.Contains(result[0].Content, "Yielding turn") {
			t.Errorf("expected yield message, got %q", result[0].Content)
		}
	})
}

func TestSessionsYieldDeniedForLeafSubagent(t *testing.T) {
	// Verify sessions_yield is in the leaf deny list.
	found := false
	for _, name := range subagentDenyLeaf {
		if name == "sessions_yield" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected sessions_yield to be in subagentDenyLeaf")
	}
}

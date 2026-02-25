package copilot

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
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

	// In the logic:
	// length = 15
	// body count = 14
	// goal = body[0]
	// header = 1 (system)
	// middle = body[1 : len(body)-6] = body[1 : 14-6] = body[1 : 8] (7 messages)
	// recent = body[14-6:] = body[8:] (6 messages)
	// total = header (1) + goal(1) + summary_msg(1) + recent(6) = 9 messages

	if len(compacted) != 9 {
		t.Errorf("Expected managed compaction to yield 9 messages, got %d", len(compacted))
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

	t.Run("keeps system header, compression notice, last assistant and last user message", func(t *testing.T) {
		var messages []chatMessage
		messages = append(messages, chatMessage{Role: "system", Content: "System Prompt"})
		messages = append(messages, chatMessage{Role: "user", Content: "First user message"})
		messages = append(messages, chatMessage{Role: "assistant", Content: "First response"})
		messages = append(messages, chatMessage{Role: "user", Content: "Second user message"})
		messages = append(messages, chatMessage{Role: "assistant", Content: "Second response"})
		messages = append(messages, chatMessage{Role: "user", Content: "Last user message"})

		// Emergency compression produces:
		// 1. original system message
		// 2. compression notice (system)
		// 3. last assistant message (if found before last user)
		// 4. last user message
		compacted := agent.emergencyCompression(messages)

		if len(compacted) != 4 {
			t.Errorf("Expected emergency compression to yield 4 messages, got %d", len(compacted))
		}

		// First should be original system
		if compacted[0].Role != "system" {
			t.Errorf("Expected first message to be system, got %s", compacted[0].Role)
		}
		systemContent, ok := compacted[0].Content.(string)
		if !ok || systemContent != "System Prompt" {
			t.Errorf("Expected first system content to be original prompt, got %v", compacted[0].Content)
		}

		// Second should be compression notice
		if compacted[1].Role != "system" {
			t.Errorf("Expected second message to be system (notice), got %s", compacted[1].Role)
		}
		noticeContent, ok := compacted[1].Content.(string)
		if !ok || !strings.Contains(noticeContent, "Emergency context compression applied") {
			t.Errorf("Expected compression notice, got %v", compacted[1].Content)
		}

		// Third should be last assistant message
		if compacted[2].Role != "assistant" {
			t.Errorf("Expected third message to be assistant, got %s", compacted[2].Role)
		}
		if compacted[2].Content != "Second response" {
			t.Errorf("Expected last assistant content, got %v", compacted[2].Content)
		}

		// Last should be the last user message
		if compacted[3].Role != "user" {
			t.Errorf("Expected last message to be user, got %s", compacted[3].Role)
		}
		if compacted[3].Content != "Last user message" {
			t.Errorf("Expected last user content, got %v", compacted[3].Content)
		}
	})

	t.Run("without assistant message before last user", func(t *testing.T) {
		var messages []chatMessage
		messages = append(messages, chatMessage{Role: "system", Content: "System"})
		messages = append(messages, chatMessage{Role: "user", Content: "Only user message"})

		// Emergency compression produces:
		// 1. original system message
		// 2. compression notice (system)
		// 3. last user message (no assistant)
		compacted := agent.emergencyCompression(messages)

		if len(compacted) != 3 {
			t.Errorf("Expected 3 messages without assistant, got %d", len(compacted))
		}

		// Last should be user message
		if compacted[2].Role != "user" {
			t.Errorf("Expected last message to be user, got %s", compacted[2].Role)
		}
		if compacted[2].Content != "Only user message" {
			t.Errorf("Expected user content, got %v", compacted[2].Content)
		}
	})

	t.Run("handles only system message", func(t *testing.T) {
		var messages []chatMessage
		messages = append(messages, chatMessage{Role: "system", Content: "System"})

		compacted := agent.emergencyCompression(messages)

		if len(compacted) != 1 {
			t.Errorf("Expected 1 message for system-only, got %d", len(compacted))
		}
	})

	t.Run("handles empty messages", func(t *testing.T) {
		var messages []chatMessage

		compacted := agent.emergencyCompression(messages)

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
		compacted := agent.emergencyCompression(messages)

		// Should reduce to: 1 system + 1 notice + 1 assistant + 1 user = 4 messages
		if len(compacted) != 4 {
			t.Errorf("Expected 4 messages after emergency compression, got %d", len(compacted))
		}

		// Verify the last message is the final user question
		if compacted[3].Content != "Final question" {
			t.Errorf("Expected last message to be 'Final question', got %v", compacted[3].Content)
		}
	})
}

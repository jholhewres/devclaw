package copilot

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		baseURL  string
		expected string
	}{
		{"https://api.openai.com/v1", "openai"},
		{"https://api.openai.com", "openai"},
		{"https://api.anthropic.com/v1", "anthropic"},
		{"https://api.anthropic.com", "anthropic"},
		{"https://api.z.ai/api/coding", "zai-coding"},
		{"https://api.z.ai/api/paas", "zai"},
		{"https://api.z.ai/api/anthropic", "zai-anthropic"},
		{"https://openrouter.ai/api/v1", "openrouter"},
		{"https://api.x.ai/v1", "xai"},
		{"http://localhost:11434/v1", "ollama"},
		{"http://127.0.0.1:11434", "ollama"},
		{"http://myserver.com/ollama/v1", "ollama"},
		{"https://custom-llm.example.com/v1", "openai"}, // Default to openai-compatible
		{"https://api.example.com/chat", "openai"},      // Default to openai-compatible
	}

	for _, tt := range tests {
		t.Run(tt.baseURL, func(t *testing.T) {
			result := detectProvider(tt.baseURL)
			if result != tt.expected {
				t.Errorf("detectProvider(%q) = %q, want %q", tt.baseURL, result, tt.expected)
			}
		})
	}
}

func TestLLMClientIsAnthropicAPI(t *testing.T) {
	tests := []struct {
		provider string
		expected bool
	}{
		{"anthropic", true},
		{"zai-anthropic", true},
		{"openai", false},
		{"zai", false},
		{"zai-coding", false},
		{"ollama", false},
		{"openrouter", false},
		{"xai", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			client := &LLMClient{provider: tt.provider}
			result := client.isAnthropicAPI()
			if result != tt.expected {
				t.Errorf("isAnthropicAPI() for provider %q = %v, want %v", tt.provider, result, tt.expected)
			}
		})
	}
}

func TestLLMClientChatEndpoint(t *testing.T) {
	tests := []struct {
		baseURL  string
		provider string
		expected string
	}{
		{"https://api.openai.com/v1", "openai", "https://api.openai.com/v1/chat/completions"},
		{"https://api.anthropic.com", "anthropic", "https://api.anthropic.com/v1/messages"},
		{"https://api.z.ai/api/coding", "zai-coding", "https://api.z.ai/api/coding/chat/completions"},
		{"https://api.z.ai/api/anthropic", "zai-anthropic", "https://api.z.ai/api/anthropic/v1/messages"},
		{"https://custom.example.com/api", "openai", "https://custom.example.com/api/chat/completions"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			client := &LLMClient{
				baseURL:  tt.baseURL,
				provider: tt.provider,
			}
			result := client.chatEndpoint()
			if result != tt.expected {
				t.Errorf("chatEndpoint() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLLMClientSupportsWhisper(t *testing.T) {
	tests := []struct {
		provider string
		expected bool
	}{
		{"openai", true},
		{"openrouter", true},
		{"ollama", false},
		{"anthropic", false},
		{"zai", false},
		{"zai-coding", false},
		{"zai-anthropic", false},
		{"xai", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			client := &LLMClient{provider: tt.provider}
			result := client.supportsWhisper()
			if result != tt.expected {
				t.Errorf("supportsWhisper() for provider %q = %v, want %v", tt.provider, result, tt.expected)
			}
		})
	}
}

func TestLLMClientCooldownTracking(t *testing.T) {
	client := &LLMClient{
		provider:         "openai",
		probeMinInterval: 1 * time.Second,
	}

	t.Run("initial state has no cooldown", func(t *testing.T) {
		client.cooldownMu.Lock()
		expires := client.cooldownExpires
		client.cooldownMu.Unlock()

		if !expires.IsZero() {
			t.Error("expected no cooldown initially")
		}
	})

	t.Run("can set and check cooldown", func(t *testing.T) {
		client.cooldownMu.Lock()
		client.cooldownExpires = time.Now().Add(30 * time.Second)
		client.cooldownModel = "gpt-4"
		client.cooldownMu.Unlock()

		client.cooldownMu.Lock()
		if client.cooldownModel != "gpt-4" {
			t.Error("expected cooldown model to be set")
		}
		client.cooldownMu.Unlock()
	})
}

func TestNormalizeGeminiModelID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Gemini 3.1 aliases
		{"gemini-3.1-pro", "gemini-3.1-pro-preview"},
		{"gemini-3.1-flash", "gemini-3.1-flash-preview"},
		// Gemini 3 aliases
		{"gemini-3-pro", "gemini-3-pro-preview"},
		{"gemini-3-flash", "gemini-3-flash-preview"},
		// Already fully specified (no change)
		{"gemini-3.1-pro-preview", "gemini-3.1-pro-preview"},
		{"gemini-3.1-flash-preview", "gemini-3.1-flash-preview"},
		{"gemini-3-pro-preview", "gemini-3-pro-preview"},
		{"gemini-3-flash-preview", "gemini-3-flash-preview"},
		// Older Gemini models (no change)
		{"gemini-2.5-pro-preview", "gemini-2.5-pro-preview"},
		{"gemini-2.0-flash", "gemini-2.0-flash"},
		{"gemini-1.5-pro", "gemini-1.5-pro"},
		// Non-Gemini models (no change)
		{"gpt-4o", "gpt-4o"},
		{"claude-sonnet-4-20250514", "claude-sonnet-4-20250514"},
		{"llama-3.3-70b", "llama-3.3-70b"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeGeminiModelID(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeGeminiModelID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNonNativeOpenAIStripStrict(t *testing.T) {
	nonNativeProviders := []string{"ollama", "lmstudio", "vllm", "huggingface"}
	nativeProviders := []string{"openai", "zai", "zai-coding", "anthropic", "openrouter"}

	for _, p := range nonNativeProviders {
		t.Run(p+"_isNonNative", func(t *testing.T) {
			client := &LLMClient{provider: p}
			if !client.isNonNativeOpenAICompat() {
				t.Errorf("isNonNativeOpenAICompat() for %q = false, want true", p)
			}
		})
		t.Run(p+"_needsSchemaStrip", func(t *testing.T) {
			client := &LLMClient{provider: p}
			if !client.needsSchemaStrip("some-model") {
				t.Errorf("needsSchemaStrip() for %q = false, want true", p)
			}
		})
	}

	for _, p := range nativeProviders {
		t.Run(p+"_isNative", func(t *testing.T) {
			client := &LLMClient{provider: p}
			if client.isNonNativeOpenAICompat() {
				t.Errorf("isNonNativeOpenAICompat() for %q = true, want false", p)
			}
		})
	}

	// xAI still needs schema strip even though it's not "non-native"
	t.Run("xai_needsSchemaStrip", func(t *testing.T) {
		client := &LLMClient{provider: "xai"}
		if !client.needsSchemaStrip("grok-3") {
			t.Error("needsSchemaStrip() for xai should be true")
		}
	})
}

func TestFastModePayload(t *testing.T) {
	t.Run("context_roundtrip", func(t *testing.T) {
		ctx := context.Background()
		if fastModeFromCtx(ctx) {
			t.Error("expected fast mode to be false by default")
		}
		ctx = ContextWithFastMode(ctx, true)
		if !fastModeFromCtx(ctx) {
			t.Error("expected fast mode to be true after setting")
		}
		ctx = ContextWithFastMode(ctx, false)
		if fastModeFromCtx(ctx) {
			t.Error("expected fast mode to be false after unsetting")
		}
	})

	t.Run("openai_fast_mode_sets_fields", func(t *testing.T) {
		client := &LLMClient{provider: "openai"}
		ctx := ContextWithFastMode(context.Background(), true)
		req := &chatRequest{}
		client.applyFastMode(ctx, req)
		if req.ServiceTier != "priority" {
			t.Errorf("ServiceTier = %q, want %q", req.ServiceTier, "priority")
		}
		if req.ReasoningEffort != "low" {
			t.Errorf("ReasoningEffort = %q, want %q", req.ReasoningEffort, "low")
		}
	})

	t.Run("openai_no_fast_mode_leaves_empty", func(t *testing.T) {
		client := &LLMClient{provider: "openai"}
		ctx := context.Background()
		req := &chatRequest{}
		client.applyFastMode(ctx, req)
		if req.ServiceTier != "" {
			t.Errorf("ServiceTier should be empty, got %q", req.ServiceTier)
		}
		if req.ReasoningEffort != "" {
			t.Errorf("ReasoningEffort should be empty, got %q", req.ReasoningEffort)
		}
	})

	t.Run("non_native_skips_fast_mode", func(t *testing.T) {
		for _, p := range []string{"ollama", "lmstudio", "vllm", "huggingface"} {
			client := &LLMClient{provider: p}
			ctx := ContextWithFastMode(context.Background(), true)
			req := &chatRequest{}
			client.applyFastMode(ctx, req)
			if req.ServiceTier != "" || req.ReasoningEffort != "" {
				t.Errorf("provider %q: fast mode should be skipped for non-native, got tier=%q effort=%q",
					p, req.ServiceTier, req.ReasoningEffort)
			}
		}
	})
}

func newAnthropicClient(retention string) *LLMClient {
	params := map[string]any{}
	if retention != "" {
		params["cache_retention"] = retention
	}
	return &LLMClient{
		provider: "anthropic",
		params:   params,
	}
}

func TestApplyAnthropicCaching_SystemPrompt(t *testing.T) {
	client := newAnthropicClient("")
	req := &anthropicRequest{
		System: "You are a helpful assistant.",
		Messages: []anthropicMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi!"},
			{Role: "user", Content: "How are you?"},
		},
	}

	client.applyAnthropicCaching(req)

	blocks, ok := req.System.([]anthropicSystemBlock)
	if !ok {
		t.Fatalf("System should be []anthropicSystemBlock, got %T", req.System)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 system block, got %d", len(blocks))
	}
	if blocks[0].Text != "You are a helpful assistant." {
		t.Errorf("system text = %q, want %q", blocks[0].Text, "You are a helpful assistant.")
	}
	if blocks[0].CacheControl == nil || blocks[0].CacheControl.Type != "ephemeral" {
		t.Error("system block should have cache_control ephemeral")
	}
}

func TestApplyAnthropicCaching_Tools(t *testing.T) {
	client := newAnthropicClient("")
	req := &anthropicRequest{
		System: "System",
		Tools: []anthropicTool{
			{Name: "tool_a", Description: "A"},
			{Name: "tool_b", Description: "B"},
		},
		Messages: []anthropicMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	client.applyAnthropicCaching(req)

	if req.Tools[0].CacheControl != nil {
		t.Error("first tool should NOT have cache_control")
	}
	if req.Tools[1].CacheControl == nil || req.Tools[1].CacheControl.Type != "ephemeral" {
		t.Error("last tool should have cache_control ephemeral")
	}
}

func TestApplyAnthropicCaching_UserMessages(t *testing.T) {
	client := newAnthropicClient("short")
	req := &anthropicRequest{
		System: "System",
		Messages: []anthropicMessage{
			{Role: "user", Content: "First question"},
			{Role: "assistant", Content: "First answer"},
			{Role: "user", Content: "Second question"},
			{Role: "assistant", Content: "Second answer"},
			{Role: "user", Content: "Third question"},
		},
	}

	client.applyAnthropicCaching(req)

	// Second-to-last user = "Second question" (index 2)
	blocks, ok := req.Messages[2].Content.([]anthropicContent)
	if !ok {
		t.Fatalf("second-to-last user should be []anthropicContent, got %T", req.Messages[2].Content)
	}
	if blocks[0].CacheControl == nil {
		t.Error("second-to-last user should have cache_control")
	}

	// First and last user should NOT have cache_control in short mode.
	if _, ok := req.Messages[0].Content.([]anthropicContent); ok {
		t.Error("first user should remain string in short mode")
	}
}

func TestApplyAnthropicCaching_LongRetention(t *testing.T) {
	client := newAnthropicClient("long")
	req := &anthropicRequest{
		System: "System",
		Messages: []anthropicMessage{
			{Role: "user", Content: "First question"},
			{Role: "assistant", Content: "First answer"},
			{Role: "user", Content: "Second question"},
			{Role: "assistant", Content: "Second answer"},
			{Role: "user", Content: "Third question"},
		},
	}

	client.applyAnthropicCaching(req)

	// First user should have cache_control in long mode.
	firstBlocks, ok := req.Messages[0].Content.([]anthropicContent)
	if !ok {
		t.Fatalf("first user should be []anthropicContent in long mode, got %T", req.Messages[0].Content)
	}
	if firstBlocks[0].CacheControl == nil {
		t.Error("first user should have cache_control in long mode")
	}

	// Second-to-last user = "Second question" (index 2)
	secondBlocks, ok := req.Messages[2].Content.([]anthropicContent)
	if !ok {
		t.Fatalf("second-to-last user should be []anthropicContent, got %T", req.Messages[2].Content)
	}
	if secondBlocks[0].CacheControl == nil {
		t.Error("second-to-last user should have cache_control in long mode")
	}
}

func TestApplyAnthropicCaching_LongRetention_TwoUserMessages(t *testing.T) {
	client := newAnthropicClient("long")
	req := &anthropicRequest{
		System: "System",
		Messages: []anthropicMessage{
			{Role: "user", Content: "First question"},
			{Role: "assistant", Content: "First answer"},
			{Role: "user", Content: "Second question"},
		},
	}

	client.applyAnthropicCaching(req)

	// First user is also the second-to-last user — should be marked once, not double-marked.
	firstBlocks, ok := req.Messages[0].Content.([]anthropicContent)
	if !ok {
		t.Fatalf("first user should be []anthropicContent in long mode, got %T", req.Messages[0].Content)
	}
	if firstBlocks[0].CacheControl == nil {
		t.Error("first user should have cache_control")
	}

	// Last user should NOT be marked (it's the most recent, not second-to-last).
	if _, ok := req.Messages[2].Content.([]anthropicContent); ok {
		t.Error("last user should not be converted to blocks when it is the only remaining unmarked user")
	}
}

func TestApplyAnthropicCaching_SingleUserMessage(t *testing.T) {
	client := newAnthropicClient("short")
	req := &anthropicRequest{
		System: "System",
		Messages: []anthropicMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	client.applyAnthropicCaching(req)

	// System should be cached.
	if _, ok := req.System.([]anthropicSystemBlock); !ok {
		t.Error("system should be converted to blocks")
	}

	// Single user message should NOT be marked (no second-to-last exists).
	if _, ok := req.Messages[0].Content.(string); !ok {
		t.Errorf("single user message should remain string, got %T", req.Messages[0].Content)
	}
}

func TestApplyAnthropicCaching_NoneRetention(t *testing.T) {
	client := newAnthropicClient("none")
	req := &anthropicRequest{
		System: "System",
		Tools: []anthropicTool{
			{Name: "tool_a", Description: "A"},
		},
		Messages: []anthropicMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
			{Role: "user", Content: "Bye"},
		},
	}

	client.applyAnthropicCaching(req)

	// System should remain string.
	if _, ok := req.System.(string); !ok {
		t.Errorf("system should remain string with none retention, got %T", req.System)
	}
	// No tool caching.
	if req.Tools[0].CacheControl != nil {
		t.Error("tool should NOT have cache_control with none retention")
	}
}

func TestApplyAnthropicCaching_SerializationJSON(t *testing.T) {
	client := newAnthropicClient("short")
	req := &anthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 4096,
		System:    "You are helpful.",
		Tools: []anthropicTool{
			{Name: "search", Description: "Search", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		Messages: []anthropicMessage{
			{Role: "user", Content: "First"},
			{Role: "assistant", Content: "Reply"},
			{Role: "user", Content: "Second"},
		},
	}

	client.applyAnthropicCaching(req)

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify it round-trips to valid JSON.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	jsonStr := string(data)
	if !jsonContains(jsonStr, `"cache_control"`) {
		t.Error("serialized JSON should contain cache_control")
	}
	if !jsonContains(jsonStr, `"ephemeral"`) {
		t.Error("serialized JSON should contain ephemeral")
	}

	// Verify system is array of objects.
	var sysBlocks []anthropicSystemBlock
	if err := json.Unmarshal(raw["system"], &sysBlocks); err != nil {
		t.Fatalf("system should unmarshal as []anthropicSystemBlock: %v", err)
	}
	if len(sysBlocks) != 1 || sysBlocks[0].CacheControl == nil {
		t.Error("system block should have cache_control after roundtrip")
	}
}

func jsonContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

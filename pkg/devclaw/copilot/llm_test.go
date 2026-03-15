package copilot

import (
	"context"
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

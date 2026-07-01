// Package copilot — prompt_layers_recall_gate_test.go covers the US-005
// greeting / trivial-input gate that skips long-term memory recall (and the
// "## Recalled Memories" section) for openers like "Oi" while still letting
// genuine short questions run hybrid search.
package copilot

import "testing"

func TestShouldSkipMemoryRecall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Greetings (any casing, trailing punctuation) → skip.
		{"oi", "Oi", true},
		{"oi bang", "Oi!", true},
		{"ola accent", "Olá", true},
		{"ola plain", "ola", true},
		{"hello dot", "hello.", true},
		{"hey", "hey", true},
		{"bom dia", "Bom dia", true},
		{"boa noite", "boa noite", true},
		{"e ai", "e aí", true},
		{"eai", "eai", true},

		// Empty / whitespace → skip.
		{"empty", "", true},
		{"spaces", "   ", true},

		// Extremely short non-greeting inputs (<=3 words AND <=15 runes) → skip.
		{"ok", "ok", true},
		{"two short words", "valeu mano", true},

		// Genuine short questions must still search (>3 words OR >15 runes).
		{"short question runes", "qual meu nome?", false},
		{"four words", "como faço o deploy", false},
		{"long single concept", "PostgreSQLconnectionstring", false},
		{"real query", "Quais foram as decisões de arquitetura?", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSkipMemoryRecall(tt.input); got != tt.want {
				t.Errorf("shouldSkipMemoryRecall(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

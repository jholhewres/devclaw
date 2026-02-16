package copilot

import (
	"testing"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels"
)

func TestParseQueueMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  QueueMode
		ok    bool
	}{
		{"collect", QueueModeCollect, true},
		{"steer", QueueModeSteer, true},
		{"followup", QueueModeFollowup, true},
		{"interrupt", QueueModeInterrupt, true},
		{"steer-backlog", QueueModeSteerBacklog, true},
		{"COLLECT", QueueModeCollect, true},     // case insensitive
		{"  steer  ", QueueModeSteer, true},     // whitespace trimmed
		{"invalid", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseQueueMode(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseQueueMode(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("ParseQueueMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEffectiveQueueMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		qc      QueueConfig
		channel string
		want    QueueMode
	}{
		{
			name:    "channel override",
			qc:      QueueConfig{DefaultMode: QueueModeCollect, ByChannel: map[string]QueueMode{"whatsapp": QueueModeInterrupt}},
			channel: "whatsapp",
			want:    QueueModeInterrupt,
		},
		{
			name:    "fallback to default",
			qc:      QueueConfig{DefaultMode: QueueModeCollect, ByChannel: map[string]QueueMode{}},
			channel: "telegram",
			want:    QueueModeCollect,
		},
		{
			name:    "empty default falls to steer",
			qc:      QueueConfig{},
			channel: "any",
			want:    QueueModeSteer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EffectiveQueueMode(tt.qc, tt.channel)
			if got != tt.want {
				t.Errorf("EffectiveQueueMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatCollectedMessages(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		got := FormatCollectedMessages(nil)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("single message", func(t *testing.T) {
		t.Parallel()
		msgs := []*channels.IncomingMessage{{Content: "hello"}}
		got := FormatCollectedMessages(msgs)
		if got != "hello" {
			t.Errorf("single msg should return content directly, got %q", got)
		}
	})

	t.Run("multiple messages", func(t *testing.T) {
		t.Parallel()
		msgs := []*channels.IncomingMessage{
			{Content: "first"},
			{Content: "second"},
		}
		got := FormatCollectedMessages(msgs)
		if got == "" {
			t.Fatal("expected non-empty")
		}
		if !qmContains(got, "Queued #1") || !qmContains(got, "Queued #2") {
			t.Errorf("expected numbered queued messages, got:\n%s", got)
		}
		if !qmContains(got, "first") || !qmContains(got, "second") {
			t.Errorf("expected message contents, got:\n%s", got)
		}
	})
}

func qmContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

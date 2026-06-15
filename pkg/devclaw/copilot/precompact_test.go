package copilot

import (
	"strings"
	"testing"
	"time"
)

func TestBuildPreCompactSnapshot(t *testing.T) {
	now := time.Now()
	history := []ConversationEntry{
		{
			UserMessage:       "fix the JID bug in scheduled delivery",
			AssistantResponse: "patched parseJID in events.go",
			Timestamp:         now.Add(-time.Hour),
			ToolCalls:         []ToolCallRecord{{Name: "read_file"}, {Name: "edit_file"}},
		},
		{
			UserMessage:       "now run the whatsapp tests",
			AssistantResponse: "all tests pass",
			Timestamp:         now,
			ToolCalls:         []ToolCallRecord{{Name: "bash"}},
		},
	}

	snap, ok := buildPreCompactSnapshot(history, now)
	if !ok {
		t.Fatal("expected a snapshot from non-empty history")
	}
	if snap.Origin != "precompact" {
		t.Errorf("Origin = %q, want precompact", snap.Origin)
	}
	if snap.MemoryType != "operational" {
		t.Errorf("MemoryType = %q, want operational", snap.MemoryType)
	}
	if snap.ContextTier != "L1" {
		t.Errorf("ContextTier = %q, want L1", snap.ContextTier)
	}
	if snap.Importance <= 0 {
		t.Errorf("Importance = %v, want > 0", snap.Importance)
	}
	if snap.ExpiresAt == nil {
		t.Fatal("snapshot must have a TTL (ExpiresAt)")
	}
	if got := snap.ExpiresAt.Sub(now); got != preCompactSnapshotTTL {
		t.Errorf("TTL = %v, want %v", got, preCompactSnapshotTTL)
	}
	// Goal is the most recent user message.
	if !strings.Contains(snap.Content, "now run the whatsapp tests") {
		t.Errorf("snapshot should capture the latest goal, got:\n%s", snap.Content)
	}
	// Recent tools across turns are captured.
	for _, want := range []string{"bash", "read_file", "edit_file"} {
		if !strings.Contains(snap.Content, want) {
			t.Errorf("snapshot should mention recent tool %q, got:\n%s", want, snap.Content)
		}
	}
	// Last action is captured.
	if !strings.Contains(snap.Content, "all tests pass") {
		t.Errorf("snapshot should capture last action, got:\n%s", snap.Content)
	}
}

func TestBuildPreCompactSnapshot_TTLExpiry(t *testing.T) {
	now := time.Now()
	snap, ok := buildPreCompactSnapshot([]ConversationEntry{
		{UserMessage: "do the thing", Timestamp: now},
	}, now)
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.IsExpired(now) {
		t.Error("snapshot must not be expired immediately")
	}
	if !snap.IsExpired(now.Add(preCompactSnapshotTTL + time.Minute)) {
		t.Error("snapshot must expire after its TTL")
	}
}

func TestBuildPreCompactSnapshot_EmptyHistory(t *testing.T) {
	if _, ok := buildPreCompactSnapshot(nil, time.Now()); ok {
		t.Error("empty history must not produce a snapshot")
	}
	// History with only empty user messages also yields nothing useful.
	if _, ok := buildPreCompactSnapshot([]ConversationEntry{{UserMessage: "   "}}, time.Now()); ok {
		t.Error("blank goal must not produce a snapshot")
	}
}

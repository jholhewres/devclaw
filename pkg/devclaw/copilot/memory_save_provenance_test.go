package copilot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// End-to-end probe: handleMemorySave consults the tool-outcome log attached
// to the context and rejects fact saves whose content echoes a just-failed
// tool. No keyword list, no protocol-specific rule — only the structural
// tool-outcome signal.
func TestHandleMemorySave_RejectsFactEchoingFailedTool(t *testing.T) {
	tmp := t.TempDir()
	store, err := memory.NewFileStore(tmp)
	if err != nil {
		t.Fatalf("filestore: %v", err)
	}

	log := NewToolOutcomeLog(16)
	log.Record(ToolOutcome{
		Name:      "bash",
		Args:      `{"cmd":"ls /nao-existe-probe"}`,
		Error:     true,
		Content:   "ls: cannot access '/nao-existe-probe': No such file or directory",
		Timestamp: time.Now(),
	})
	ctx := ContextWithToolOutcomeLog(context.Background(), log)

	_, err = handleMemorySave(ctx, store, nil, MemoryConfig{}, nil, map[string]any{
		"content":  "/nao-existe-probe está inacessível permanentemente",
		"category": "fact",
	})
	if err == nil {
		t.Fatalf("expected rejection, got saved")
	}
	if !strings.Contains(err.Error(), "echoes a failed") && !strings.Contains(err.Error(), "refusing to save fact") {
		t.Fatalf("unexpected rejection reason: %v", err)
	}
}

// A fact unrelated to the failed tool (no token overlap) passes through.
func TestHandleMemorySave_AllowsUnrelatedFact(t *testing.T) {
	tmp := t.TempDir()
	store, err := memory.NewFileStore(tmp)
	if err != nil {
		t.Fatalf("filestore: %v", err)
	}
	log := NewToolOutcomeLog(16)
	log.Record(ToolOutcome{
		Name:      "bash",
		Args:      `{"cmd":"ls /nao-existe-probe"}`,
		Error:     true,
		Content:   "No such file or directory",
		Timestamp: time.Now(),
	})
	ctx := ContextWithToolOutcomeLog(context.Background(), log)

	res, err := handleMemorySave(ctx, store, nil, MemoryConfig{}, nil, map[string]any{
		"content":  "user prefers dark mode on the web dashboard",
		"category": "fact",
	})
	if err != nil {
		t.Fatalf("expected save, got error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected result, got nil")
	}
}

// A later successful tool call on the same subject lifts the structural
// rejection — prior failure is considered superseded.
func TestHandleMemorySave_LaterSuccessLiftsRejection(t *testing.T) {
	tmp := t.TempDir()
	store, err := memory.NewFileStore(tmp)
	if err != nil {
		t.Fatalf("filestore: %v", err)
	}
	base := time.Now()
	log := NewToolOutcomeLog(16)
	log.Record(ToolOutcome{
		Name:      "bash",
		Args:      `{"cmd":"ls /nao-existe-probe"}`,
		Error:     true,
		Content:   "No such file or directory",
		Timestamp: base,
	})
	log.Record(ToolOutcome{
		Name:      "bash",
		Args:      `{"cmd":"mkdir -p /nao-existe-probe && ls /nao-existe-probe"}`,
		Error:     false,
		Content:   "",
		Timestamp: base.Add(time.Second),
	})
	ctx := ContextWithToolOutcomeLog(context.Background(), log)

	res, err := handleMemorySave(ctx, store, nil, MemoryConfig{}, nil, map[string]any{
		"content":  "/nao-existe-probe agora existe",
		"category": "fact",
	})
	if err != nil {
		t.Fatalf("later success should lift rejection, got: %v", err)
	}
	if res == nil {
		t.Fatalf("expected result, got nil")
	}
}

// Preference category is never subject to the provenance check — only facts are.
func TestHandleMemorySave_PreferenceNeverBlocked(t *testing.T) {
	tmp := t.TempDir()
	store, err := memory.NewFileStore(tmp)
	if err != nil {
		t.Fatalf("filestore: %v", err)
	}
	log := NewToolOutcomeLog(16)
	log.Record(ToolOutcome{
		Name:      "bash",
		Args:      `{"cmd":"ls /nao-existe-probe"}`,
		Error:     true,
		Content:   "No such file or directory",
		Timestamp: time.Now(),
	})
	ctx := ContextWithToolOutcomeLog(context.Background(), log)

	res, err := handleMemorySave(ctx, store, nil, MemoryConfig{}, nil, map[string]any{
		"content":  "/nao-existe-probe is a path the user dislikes",
		"category": "preference",
	})
	if err != nil {
		t.Fatalf("preference must pass through, got: %v", err)
	}
	if res == nil {
		t.Fatalf("expected result, got nil")
	}
}

// The parser's existing [stale] filter plus the hardened expireStaleEntries
// format-aware matcher work together: an indented bullet with [stale] at
// arbitrary prefix position is ignored on read, regardless of indent.
func TestStaleMarker_IgnoredOnRead_AcrossIndents(t *testing.T) {
	tmp := t.TempDir()
	store, err := memory.NewFileStore(tmp)
	if err != nil {
		t.Fatalf("filestore: %v", err)
	}

	payload := strings.Join([]string{
		"# Test",
		"",
		"- [2026-04-16 12:00] [fact] normal entry one",
		"    - [stale] [2026-04-16 12:01] [fact] indented stale entry",
		"- [stale] [2026-04-16 12:02] [fact] top-level stale entry",
		"- [2026-04-16 12:03] [fact] normal entry two",
		"",
	}, "\n")

	path := fmt.Sprintf("%s/%s", tmp, memory.MemoryFileName)
	if err := writeFileForTest(path, payload); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 non-stale entries, got %d: %+v", len(got), got)
	}
	for _, e := range got {
		if strings.Contains(e.Content, "stale") {
			t.Errorf("stale content leaked through: %q", e.Content)
		}
	}
}

func writeFileForTest(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

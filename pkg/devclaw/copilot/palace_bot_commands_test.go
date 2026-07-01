// Package copilot — palace_bot_commands_test.go covers the slash-command
// parser for palace-aware memory operations. Most tests exercise the
// "disabled" and "no store" paths because those are the ones most likely
// to be hit in production during the opt-in rollout.
package copilot

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// newTestStoreCopilot creates a memory.SQLiteStore in a temp directory
// using the NullEmbedder so tests don't need network access.
//
// Distinct from the memory package's internal newTestStore helper —
// the unexported helper can't be imported from copilot package.
func newTestStoreCopilot(t *testing.T) *memory.SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := memory.NewSQLiteStore(dbPath, &memory.NullEmbedder{}, logger)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// TestHandlePalaceBotCommandNotACommand asserts non-slash messages pass through.
func TestHandlePalaceBotCommandNotACommand(t *testing.T) {
	cfg := PalaceBotConfig{Enabled: true}
	cases := []string{
		"",
		"hello",
		"how are you?",
		"wing set work", // missing leading slash
		"  ",
	}
	for _, input := range cases {
		reply := HandlePalaceBotCommand(context.Background(), cfg, "telegram", "123", input)
		if reply.Handled {
			t.Errorf("expected not handled for %q, got %+v", input, reply)
		}
	}
}

// TestHandleDisabledFeature confirms that all palace commands return the
// disabled message when Enabled=false.
func TestHandleDisabledFeature(t *testing.T) {
	cfg := PalaceBotConfig{Enabled: false}
	commands := []string{
		"/wing",
		"/wing list",
		"/wing set work",
		"/wing unset",
		"/wing merge a b",
		"/room",
		"/room list",
		"/tree",
	}
	for _, cmd := range commands {
		reply := HandlePalaceBotCommand(context.Background(), cfg, "telegram", "123", cmd)
		if !reply.Handled {
			t.Errorf("expected handled=true for disabled command %q", cmd)
		}
		if !strings.Contains(reply.Reply, "disabled") {
			t.Errorf("expected 'disabled' in reply for %q, got %q", cmd, reply.Reply)
		}
	}
}

// TestPalaceHelpAlwaysWorks confirms /palace help is independent of flag state.
func TestPalaceHelpAlwaysWorks(t *testing.T) {
	// With flag on.
	cfgOn := PalaceBotConfig{Enabled: true}
	reply := HandlePalaceBotCommand(context.Background(), cfgOn, "telegram", "123", "/palace help")
	if !reply.Handled {
		t.Error("expected /palace help to be handled")
	}
	if !strings.Contains(reply.Reply, "Palace-aware memory commands") {
		t.Errorf("expected help text, got %q", reply.Reply)
	}

	// With flag off.
	cfgOff := PalaceBotConfig{Enabled: false}
	reply = HandlePalaceBotCommand(context.Background(), cfgOff, "telegram", "123", "/palace help")
	if !reply.Handled {
		t.Error("expected /palace help to be handled even when disabled")
	}
}

// TestUnknownWingSubcommand returns a usage message.
func TestUnknownWingSubcommand(t *testing.T) {
	cfg := PalaceBotConfig{Enabled: true}
	reply := HandlePalaceBotCommand(context.Background(), cfg, "telegram", "123", "/wing bogus")
	if !reply.Handled {
		t.Error("expected handled=true for unknown subcommand")
	}
	if !strings.Contains(reply.Reply, "unknown") {
		t.Errorf("expected 'unknown' in reply, got %q", reply.Reply)
	}
}

// TestWingSetUsageError verifies that /wing set without a name fails gracefully.
func TestWingSetUsageError(t *testing.T) {
	cfg := PalaceBotConfig{Enabled: true}
	reply := HandlePalaceBotCommand(context.Background(), cfg, "telegram", "123", "/wing set")
	if !reply.Handled {
		t.Error("expected handled=true for /wing set with no args")
	}
	if !strings.Contains(reply.Reply, "usage") {
		t.Errorf("expected usage message, got %q", reply.Reply)
	}
}

// TestWingMergeUsageError verifies that /wing merge with insufficient args fails.
func TestWingMergeUsageError(t *testing.T) {
	cfg := PalaceBotConfig{Enabled: true}
	cases := []string{
		"/wing merge",
		"/wing merge only-one",
	}
	for _, cmd := range cases {
		reply := HandlePalaceBotCommand(context.Background(), cfg, "telegram", "123", cmd)
		if !reply.Handled {
			t.Errorf("expected handled=true for %q", cmd)
		}
		if !strings.Contains(reply.Reply, "usage") {
			t.Errorf("expected usage message for %q, got %q", cmd, reply.Reply)
		}
	}
}

// TestWingSetInvalidName rejects malformed wing names.
func TestWingSetInvalidName(t *testing.T) {
	cfg := PalaceBotConfig{Enabled: true}
	invalid := []string{
		"/wing set __system",
		"/wing set  ",
		"/wing set 🎉",
	}
	for _, cmd := range invalid {
		reply := HandlePalaceBotCommand(context.Background(), cfg, "telegram", "123", cmd)
		if !reply.Handled {
			t.Errorf("expected handled=true for %q", cmd)
		}
		// "invalid" OR "usage" depending on how the args parse.
		// Invalid name should produce "invalid wing name" message.
		if !strings.Contains(reply.Reply, "invalid") && !strings.Contains(reply.Reply, "usage") {
			t.Errorf("expected invalid/usage message for %q, got %q", cmd, reply.Reply)
		}
	}
}

// TestWingMergeSameWingRejected confirms we can't merge a wing into itself.
func TestWingMergeSameWingRejected(t *testing.T) {
	// Use a real store so the merge path gets as far as the self-check.
	store := newTestStoreCopilot(t)
	cfg := PalaceBotConfig{Store: store, Enabled: true}
	reply := HandlePalaceBotCommand(context.Background(), cfg, "telegram", "123", "/wing merge work work")
	if !reply.Handled {
		t.Error("expected handled=true")
	}
	if !strings.Contains(reply.Reply, "cannot merge") {
		t.Errorf("expected self-merge rejection, got %q", reply.Reply)
	}
}

// TestCaseInsensitiveCommand verifies that /WING and /wing behave identically.
func TestCaseInsensitiveCommand(t *testing.T) {
	cfg := PalaceBotConfig{Enabled: false}
	cases := []string{"/wing", "/WING", "/Wing", "/wING"}
	for _, cmd := range cases {
		reply := HandlePalaceBotCommand(context.Background(), cfg, "telegram", "123", cmd)
		if !reply.Handled {
			t.Errorf("expected case-insensitive handling for %q", cmd)
		}
	}
}

// TestHandleWingMergeHappyPath covers HI-2 from the Sprint 1 code review:
// the actual SUCCESS path of `/wing merge from to` was untested, only
// usage errors and self-merge rejection were covered.
//
// This test:
//  1. Creates a real store with two channel_wing_map entries pinned to
//     wing "trabalho"
//  2. Calls /wing merge trabalho work
//  3. Verifies:
//     - Both channel entries are remapped to "work"
//     - The "trabalho" wing registry entry is deleted
//     - Reply text mentions the count of remapped mappings
//     - /wing list no longer shows "trabalho"
func TestHandleWingMergeHappyPath(t *testing.T) {
	store := newTestStoreCopilot(t)
	ctx := context.Background()

	// Seed two channel mappings that should be remapped.
	if err := store.SetChannelWing("telegram", "111", "trabalho", "manual", 1.0); err != nil {
		t.Fatalf("seed mapping 1: %v", err)
	}
	if err := store.SetChannelWing("whatsapp", "222", "trabalho", "manual", 1.0); err != nil {
		t.Fatalf("seed mapping 2: %v", err)
	}
	// Also seed an unrelated mapping that must NOT be touched.
	if err := store.SetChannelWing("cli", "333", "personal", "manual", 1.0); err != nil {
		t.Fatalf("seed unrelated mapping: %v", err)
	}

	cfg := PalaceBotConfig{Store: store, Enabled: true}
	reply := HandlePalaceBotCommand(ctx, cfg, "cli", "local", "/wing merge trabalho work")

	if !reply.Handled {
		t.Fatalf("expected handled=true, got %+v", reply)
	}
	if reply.Err != nil {
		t.Fatalf("unexpected error: %v", reply.Err)
	}
	// Reply should mention "Merged 2 channel mapping(s)".
	if !strings.Contains(reply.Reply, "Merged 2") {
		t.Errorf("expected reply to report 2 remaps, got: %q", reply.Reply)
	}

	// Verify both original mappings now point at "work".
	m1, err := store.GetChannelWing("telegram", "111")
	if err != nil {
		t.Fatalf("lookup telegram:111 after merge: %v", err)
	}
	if m1.Wing != "work" {
		t.Errorf("telegram:111 expected wing=work, got %q", m1.Wing)
	}
	m2, err := store.GetChannelWing("whatsapp", "222")
	if err != nil {
		t.Fatalf("lookup whatsapp:222 after merge: %v", err)
	}
	if m2.Wing != "work" {
		t.Errorf("whatsapp:222 expected wing=work, got %q", m2.Wing)
	}

	// Verify the unrelated mapping is untouched.
	m3, err := store.GetChannelWing("cli", "333")
	if err != nil {
		t.Fatalf("lookup cli:333 after merge: %v", err)
	}
	if m3.Wing != "personal" {
		t.Errorf("cli:333 wing changed unexpectedly: got %q, want personal", m3.Wing)
	}

	// Verify the source wing is gone from the registry.
	wings, err := store.ListWings()
	if err != nil {
		t.Fatalf("list wings after merge: %v", err)
	}
	for _, w := range wings {
		if w.Name == "trabalho" {
			t.Errorf("source wing %q should be deleted from registry after merge", w.Name)
		}
	}

	// Verify the target wing exists in the registry.
	foundWork := false
	for _, w := range wings {
		if w.Name == "work" {
			foundWork = true
			break
		}
	}
	if !foundWork {
		t.Errorf("target wing 'work' should exist in registry after merge")
	}
}

// TestHandleWingMergeNonexistentSource verifies that merging from an empty
// (nonexistent) wing still succeeds gracefully — the remap count is 0,
// nothing happens on the DB, and the user gets a clear message.
func TestHandleWingMergeNonexistentSource(t *testing.T) {
	store := newTestStoreCopilot(t)
	cfg := PalaceBotConfig{Store: store, Enabled: true}
	reply := HandlePalaceBotCommand(context.Background(), cfg, "cli", "local",
		"/wing merge nonexistent work")

	if !reply.Handled {
		t.Fatalf("expected handled=true")
	}
	if reply.Err != nil {
		t.Errorf("expected no error for empty merge, got: %v", reply.Err)
	}
	if !strings.Contains(reply.Reply, "Merged 0") {
		t.Errorf("expected 'Merged 0' in reply for empty source, got: %q", reply.Reply)
	}
}

package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEntryMetaV2_RoundTrip verifies v2 lifecycle metadata survives a
// save -> parse round-trip through MEMORY.md.
func TestEntryMetaV2_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := Entry{
		Content:      "user prefers Go on ARM",
		Source:       "agent",
		Category:     "fact",
		Timestamp:    time.Now().Truncate(time.Minute),
		Supersedes:   []string{"abc123", "def456"},
		Consolidates: []string{"ghi789"},
		Importance:   0.82,
		Pinned:       true,
		Origin:       "dream",
		MemoryType:   "semantic",
		ContextTier:  "L1",
		Superseded:   true,
	}
	if err := fs.Save(want); err != nil {
		t.Fatal(err)
	}

	entries, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0]

	if got.Content != want.Content {
		t.Errorf("Content = %q, want %q", got.Content, want.Content)
	}
	if strings.Join(got.Supersedes, ",") != "abc123,def456" {
		t.Errorf("Supersedes = %v, want [abc123 def456]", got.Supersedes)
	}
	if strings.Join(got.Consolidates, ",") != "ghi789" {
		t.Errorf("Consolidates = %v, want [ghi789]", got.Consolidates)
	}
	if got.Importance != 0.82 {
		t.Errorf("Importance = %v, want 0.82", got.Importance)
	}
	if !got.Pinned || !got.IsPinned() {
		t.Errorf("Pinned = %v, want true", got.Pinned)
	}
	if got.Origin != "dream" {
		t.Errorf("Origin = %q, want dream", got.Origin)
	}
	if got.MemoryType != "semantic" {
		t.Errorf("MemoryType = %q, want semantic", got.MemoryType)
	}
	if got.ContextTier != "L1" {
		t.Errorf("ContextTier = %q, want L1", got.ContextTier)
	}
	if !got.Superseded || !got.IsSuperseded() {
		t.Errorf("Superseded = %v, want true", got.Superseded)
	}
}

// TestEntryMetaV2_PlainEntryNoMetaTag verifies entries without any v2 field are
// written byte-for-byte in the legacy format (no [meta:] tag), preserving
// backward compatibility and keeping the file readable.
func TestEntryMetaV2_PlainEntryNoMetaTag(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := fs.Save(Entry{
		Content:   "plain fact",
		Source:    "agent",
		Category:  "fact",
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, MemoryFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "[meta:") {
		t.Errorf("plain entry must not emit a [meta:] tag, file was:\n%s", raw)
	}
}

// TestEntryMetaV2_LegacyParsesUnchanged verifies a legacy line with no metadata
// parses with all v2 fields at their zero values.
func TestEntryMetaV2_LegacyParsesUnchanged(t *testing.T) {
	dir := t.TempDir()
	memFile := filepath.Join(dir, MemoryFileName)
	legacy := "# DevClaw Memory\n\n- [2025-01-01 10:00] [fact] legacy entry\n"
	if err := os.WriteFile(memFile, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Content != "legacy entry" {
		t.Errorf("Content = %q, want %q", e.Content, "legacy entry")
	}
	if e.Pinned || e.Superseded || e.Importance != 0 || e.Origin != "" ||
		e.MemoryType != "" || e.ContextTier != "" || len(e.Supersedes) != 0 || len(e.Consolidates) != 0 {
		t.Errorf("legacy entry should have zero-value v2 metadata, got %+v", e)
	}
}

// TestSaveSanitizesTagInjection verifies Save() escapes tag prefixes in user
// content so a fact containing literal "[meta:...]" or "[expires:...]" is not
// re-parsed as a structural tag (which would silently corrupt the entry).
func TestSaveSanitizesTagInjection(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Content that, unsanitized, would be parsed as v2 metadata / expiry.
	evil := "remember [meta:eyJwIjp0cnVlfQ==] and [expires:2020-01-01] literally"
	if err := fs.Save(Entry{Content: evil, Category: "fact", Source: "user", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}

	entries, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	// The injected tags must NOT have taken effect.
	if e.Pinned {
		t.Error("injected [meta:] must not set Pinned")
	}
	if e.ExpiresAt != nil {
		t.Error("injected [expires:] must not set ExpiresAt")
	}
	// The visible content must still mention the (escaped) tags, not be empty.
	if !strings.Contains(e.Content, "meta") || !strings.Contains(e.Content, "literally") {
		t.Errorf("content should survive sanitization, got %q", e.Content)
	}
}

// TestEntryHelpers verifies the lifecycle helper predicates.
func TestEntryHelpers(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	if (Entry{}).IsExpired(now) {
		t.Error("entry with nil ExpiresAt must never be expired")
	}
	if !(Entry{ExpiresAt: &past}).IsExpired(now) {
		t.Error("entry with past ExpiresAt must be expired")
	}
	if (Entry{ExpiresAt: &future}).IsExpired(now) {
		t.Error("entry with future ExpiresAt must not be expired")
	}
	if !(Entry{Pinned: true}).IsPinned() {
		t.Error("IsPinned must reflect Pinned field")
	}
	if !(Entry{Superseded: true}).IsSuperseded() {
		t.Error("IsSuperseded must reflect Superseded field")
	}

	// ContentKey is stable and category-sensitive.
	a := Entry{Category: "fact", Content: "x"}
	if a.ContentKey() != a.ContentKey() {
		t.Error("ContentKey must be deterministic")
	}
	if a.ContentKey() == (Entry{Category: "summary", Content: "x"}).ContentKey() {
		t.Error("ContentKey must differ across categories")
	}
}

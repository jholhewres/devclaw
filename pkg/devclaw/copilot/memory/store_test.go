package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEntryTTL_EventExpires(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Save an event with a timestamp 8 days in the past.
	past := time.Now().Add(-8 * 24 * time.Hour)
	err = fs.Save(Entry{
		Content:   "reunião com o time",
		Source:    "agent",
		Category:  "event",
		Timestamp: past,
	})
	if err != nil {
		t.Fatal(err)
	}

	// GetAll should filter it out (event TTL = 7 days).
	entries, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Content == "reunião com o time" {
			t.Error("expired event should be filtered from GetAll")
		}
	}

	// GetRecent should also filter it.
	recent, err := fs.GetRecent(100)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range recent {
		if e.Content == "reunião com o time" {
			t.Error("expired event should be filtered from GetRecent")
		}
	}
}

func TestEntryTTL_FactNeverExpires(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Save a fact with a timestamp 365 days in the past.
	past := time.Now().Add(-365 * 24 * time.Hour)
	err = fs.Save(Entry{
		Content:   "user prefers dark mode",
		Source:    "agent",
		Category:  "fact",
		Timestamp: past,
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Content == "user prefers dark mode" {
			found = true
			if e.ExpiresAt != nil {
				t.Error("fact should have nil ExpiresAt (never expires)")
			}
		}
	}
	if !found {
		t.Error("fact should be present in GetAll regardless of age")
	}
}

func TestEntryTTL_SummaryExpires30d(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Save a summary 31 days ago.
	past := time.Now().Add(-31 * 24 * time.Hour)
	err = fs.Save(Entry{
		Content:   "daily summary of tasks",
		Source:    "agent",
		Category:  "summary",
		Timestamp: past,
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Content == "daily summary of tasks" {
			t.Error("expired summary (31d) should be filtered from GetAll")
		}
	}
}

func TestEntryTTL_BackwardCompat_NoExpiresAt(t *testing.T) {
	dir := t.TempDir()

	// Write a legacy MEMORY.md without expires tag.
	memFile := filepath.Join(dir, MemoryFileName)
	legacy := "# DevClaw Memory\n\n- [2025-01-01 10:00] [fact] legacy entry without TTL\n"
	if err := os.WriteFile(memFile, []byte(legacy), 0644); err != nil {
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
	if entries[0].ExpiresAt != nil {
		t.Error("legacy entry should have nil ExpiresAt")
	}
	if entries[0].Content != "legacy entry without TTL" {
		t.Errorf("unexpected content: %q", entries[0].Content)
	}
}

func TestEntryTTL_FreshEventNotExpired(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Save an event just now — should NOT be expired.
	err = fs.Save(Entry{
		Content:   "meeting today",
		Source:    "agent",
		Category:  "event",
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Content == "meeting today" {
			found = true
		}
	}
	if !found {
		t.Error("fresh event should be present in GetAll")
	}
}

func TestDefaultTTLForCategory(t *testing.T) {
	if DefaultTTLForCategory("event") != 7*24*time.Hour {
		t.Error("event TTL should be 7 days")
	}
	if DefaultTTLForCategory("summary") != 30*24*time.Hour {
		t.Error("summary TTL should be 30 days")
	}
	if DefaultTTLForCategory("fact") != 0 {
		t.Error("fact TTL should be 0 (never)")
	}
	if DefaultTTLForCategory("preference") != 0 {
		t.Error("preference TTL should be 0 (never)")
	}
	if DefaultTTLForCategory("unknown") != 0 {
		t.Error("unknown TTL should be 0 (never)")
	}
}

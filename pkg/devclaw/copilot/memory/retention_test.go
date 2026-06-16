package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedRetentionStore(t *testing.T) (*FileStore, time.Time) {
	t.Helper()
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-30 * 24 * time.Hour)
	ancient := now.Add(-120 * 24 * time.Hour)

	// Category=fact avoids Save's auto-TTL so we exercise age-based retention.
	mustSaveEntry(t, fs, Entry{Content: "pinned operational note", Category: "fact", MemoryType: "operational", Pinned: true, Timestamp: old})
	mustSaveEntry(t, fs, Entry{Content: "old operational scratch", Category: "fact", MemoryType: "operational", Timestamp: old})
	mustSaveEntry(t, fs, Entry{Content: "old episodic event detail", Category: "fact", MemoryType: "episodic", Timestamp: ancient})
	mustSaveEntry(t, fs, Entry{Content: "durable semantic fact", Category: "fact", Timestamp: now})
	mustSaveEntry(t, fs, Entry{Content: "fresh operational scratch", Category: "fact", MemoryType: "operational", Timestamp: now})
	return fs, now
}

func TestRetentionSweep_DryRunReportsWithoutDeleting(t *testing.T) {
	fs, now := seedRetentionStore(t)

	report, err := fs.RetentionSweep(now, DefaultRetentionOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !report.DryRun {
		t.Error("DefaultRetentionOptions must be dry-run")
	}
	if len(report.Candidates) != 2 {
		t.Fatalf("expected 2 candidates (old operational + old episodic), got %d: %+v", len(report.Candidates), report.Candidates)
	}
	for _, c := range report.Candidates {
		if strings.Contains(c.Content, "pinned") {
			t.Error("pinned entry must never be a retention candidate")
		}
		if strings.Contains(c.Content, "durable semantic") {
			t.Error("semantic entries must be kept")
		}
		if strings.Contains(c.Content, "fresh") {
			t.Error("within-TTL operational entries must be kept")
		}
	}

	// Dry-run must not modify the file: all 5 entries still present.
	all, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Errorf("dry-run must not delete; expected 5 entries, got %d", len(all))
	}
}

func TestRetentionSweep_ApplyArchivesAndPreservesPinned(t *testing.T) {
	fs, now := seedRetentionStore(t)

	opts := DefaultRetentionOptions()
	opts.DryRun = false
	report, err := fs.RetentionSweep(now, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Archived != 2 {
		t.Fatalf("expected 2 archived, got %d", report.Archived)
	}

	all, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	var sawPinned, sawOldOp, sawEpisodic bool
	for _, e := range all {
		switch {
		case strings.Contains(e.Content, "pinned"):
			sawPinned = true
		case strings.Contains(e.Content, "old operational scratch"):
			sawOldOp = true
		case strings.Contains(e.Content, "old episodic"):
			sawEpisodic = true
		}
	}
	if !sawPinned {
		t.Error("pinned entry must be preserved after apply")
	}
	if sawOldOp || sawEpisodic {
		t.Error("aged entries must be removed from MEMORY.md after apply")
	}

	// Archived entries are recoverable from the archive file (soft-delete).
	archive, err := os.ReadFile(filepath.Join(fs.BaseDir(), MemoryArchiveFileName))
	if err != nil {
		t.Fatalf("archive file should exist: %v", err)
	}
	if !strings.Contains(string(archive), "old operational scratch") ||
		!strings.Contains(string(archive), "old episodic event detail") {
		t.Errorf("archived entries must be recoverable, archive was:\n%s", archive)
	}
}

func mustSaveEntry(t *testing.T, fs *FileStore, e Entry) {
	t.Helper()
	if err := fs.Save(e); err != nil {
		t.Fatalf("save: %v", err)
	}
}

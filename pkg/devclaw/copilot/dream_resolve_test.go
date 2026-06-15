package copilot

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// TestDreamResolvesContradictionBySuperseding verifies the dream cycle RESOLVES
// a contradiction by superseding the older entry (soft-removing it) and keeping
// the newer one — instead of the old behavior of only appending an
// "[Contradiction] A vs B" report while the stale fact stayed retrievable.
func TestDreamResolvesContradictionBySuperseding(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	fs, err := memory.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	older := time.Now().Add(-48 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)

	// Older fact + newer contradicting fact about the same topic, plus filler to
	// satisfy the >=3 entries consolidation gate.
	mustSave(t, fs, memory.Entry{Content: "User prefers the project to use PostgreSQL database for storage", Category: "preference", Source: "user", Timestamp: older})
	mustSave(t, fs, memory.Entry{Content: "User prefers the project to not use PostgreSQL database for storage", Category: "preference", Source: "user", Timestamp: newer})
	mustSave(t, fs, memory.Entry{Content: "API rate limit is 100 requests per minute", Category: "fact", Source: "user", Timestamp: newer})

	d := NewDreamConsolidator(DefaultDreamConfig(), fs, dir, logger)
	result := d.Run(context.Background())
	if result.Error != nil {
		t.Fatalf("dream run: %v", result.Error)
	}
	if result.ContradictionsResolved < 1 {
		t.Fatalf("expected >=1 contradiction resolved, got %d", result.ContradictionsResolved)
	}

	all, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	var sawOld, sawNew bool
	for _, e := range all {
		if e.Content == "User prefers the project to use PostgreSQL database for storage" {
			sawOld = true
		}
		if strings.Contains(e.Content, "not use PostgreSQL") {
			sawNew = true
		}
	}
	if sawOld {
		t.Error("older contradicted entry should be superseded and no longer returned by search")
	}
	if !sawNew {
		t.Error("newer entry should survive contradiction resolution")
	}
}

// TestDreamDoesNotSupersedePinned verifies a pinned memory is never superseded
// by contradiction resolution.
func TestDreamDoesNotSupersedePinned(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	fs, err := memory.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	older := time.Now().Add(-48 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)

	// Pinned older fact that contradicts a newer one — must be preserved.
	mustSave(t, fs, memory.Entry{Content: "User prefers the project to use PostgreSQL database for storage", Category: "preference", Source: "user", Timestamp: older, Pinned: true})
	mustSave(t, fs, memory.Entry{Content: "User prefers the project to not use PostgreSQL database for storage", Category: "preference", Source: "user", Timestamp: newer})
	mustSave(t, fs, memory.Entry{Content: "API rate limit is 100 requests per minute", Category: "fact", Source: "user", Timestamp: newer})

	d := NewDreamConsolidator(DefaultDreamConfig(), fs, dir, logger)
	if result := d.Run(context.Background()); result.Error != nil {
		t.Fatalf("dream run: %v", result.Error)
	}

	all, err := fs.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	var sawPinned bool
	for _, e := range all {
		if e.Content == "User prefers the project to use PostgreSQL database for storage" {
			sawPinned = true
		}
	}
	if !sawPinned {
		t.Error("pinned entry must never be superseded by contradiction resolution")
	}
}

func mustSave(t *testing.T, fs *memory.FileStore, e memory.Entry) {
	t.Helper()
	if err := fs.Save(e); err != nil {
		t.Fatalf("save entry: %v", err)
	}
}

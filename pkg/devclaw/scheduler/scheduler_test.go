package scheduler

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecuteJob_SpinLoopGuard(t *testing.T) {
	t.Parallel()

	var runCount atomic.Int32

	s := New(nil, func(ctx context.Context, job *Job) (string, error) {
		runCount.Add(1)
		return "ok", nil
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.ctx = ctx

	now := time.Now()
	job := &Job{
		ID:       "test-spin",
		Schedule: "@every 1s",
		Type:     "every",
		Command:  "echo test",
		Enabled:  true,
	}
	s.jobs[job.ID] = job

	// First execution — should run.
	s.executeJob(job)
	if runCount.Load() != 1 {
		t.Fatalf("expected 1 run, got %d", runCount.Load())
	}

	// Immediate second execution — spin loop guard should skip.
	s.executeJob(job)
	if runCount.Load() != 1 {
		t.Fatalf("expected still 1 run (spin loop guard), got %d", runCount.Load())
	}

	// Verify LastRunAt was set.
	if job.LastRunAt == nil {
		t.Fatal("LastRunAt should be set after execution")
	}
	if job.LastRunAt.Before(now) {
		t.Error("LastRunAt should be after test start time")
	}
}

func TestExecuteJob_DuplicateGuard(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	block := make(chan struct{})
	var runCount atomic.Int32

	s := New(nil, func(ctx context.Context, job *Job) (string, error) {
		runCount.Add(1)
		started <- struct{}{}
		<-block
		return "ok", nil
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.ctx = ctx

	job := &Job{
		ID:      "test-dup",
		Command: "echo test",
		Enabled: true,
	}
	s.jobs[job.ID] = job

	// Start first execution in background.
	go s.executeJob(job)
	<-started

	// Try to execute again — should be skipped (already running).
	s.executeJob(job)
	if runCount.Load() != 1 {
		t.Fatalf("expected 1 run (duplicate guard), got %d", runCount.Load())
	}

	close(block)
}

func TestMinJobInterval_Value(t *testing.T) {
	t.Parallel()

	if minJobInterval < 1*time.Second {
		t.Errorf("minJobInterval should be at least 1s, got %s", minJobInterval)
	}
	if minJobInterval > 10*time.Second {
		t.Errorf("minJobInterval should be reasonable (<=10s), got %s", minJobInterval)
	}
}

func TestParseOneShotTime_DateTimeUsesLocalTimezone(t *testing.T) {
	t.Parallel()

	// "2006-01-02 15:04" format must be parsed in local timezone, not UTC.
	// This test verifies the fix for the 3-hour offset bug where times like
	// "2026-03-19 07:50" were parsed as UTC instead of local time.
	t.Run("date space time uses local", func(t *testing.T) {
		target, err := parseOneShotTime("2099-12-31 14:30")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if target.Location() != time.Local {
			t.Errorf("expected local timezone %v, got %v", time.Local, target.Location())
		}
		if target.Hour() != 14 || target.Minute() != 30 {
			t.Errorf("expected 14:30, got %02d:%02d", target.Hour(), target.Minute())
		}
	})

	t.Run("date T time uses local", func(t *testing.T) {
		target, err := parseOneShotTime("2099-12-31T14:30:00")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if target.Location() != time.Local {
			t.Errorf("expected local timezone %v, got %v", time.Local, target.Location())
		}
		if target.Hour() != 14 || target.Minute() != 30 {
			t.Errorf("expected 14:30, got %02d:%02d", target.Hour(), target.Minute())
		}
	})

	t.Run("HH:MM uses local", func(t *testing.T) {
		target, err := parseOneShotTime("23:59")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if target.Location() != time.Local {
			t.Errorf("expected local timezone %v, got %v", time.Local, target.Location())
		}
	})

	t.Run("RFC3339 with Z stays UTC", func(t *testing.T) {
		target, err := parseOneShotTime("2099-12-31T14:30:00Z")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// RFC3339 with "Z" should be UTC — this is intentional.
		if target.Location() != time.UTC {
			t.Errorf("expected UTC for RFC3339 with Z, got %v", target.Location())
		}
	})
}

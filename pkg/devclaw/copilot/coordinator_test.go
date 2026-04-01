package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoordinatorPhaseOrder(t *testing.T) {
	order := PhaseOrder()
	if len(order) != 4 {
		t.Fatalf("expected 4 phases, got %d", len(order))
	}
	if order[0] != PhaseResearch {
		t.Errorf("expected first phase Research, got %s", order[0])
	}
	if order[3] != PhaseVerification {
		t.Errorf("expected last phase Verification, got %s", order[3])
	}
}

func TestCoordinatorGetPhaseTools(t *testing.T) {
	cfg := DefaultCoordinatorConfig()
	c := NewCoordinator(cfg, nil, coordTestLogger())

	t.Run("research has read-only tools", func(t *testing.T) {
		tools := c.GetPhaseTools(PhaseResearch)
		if len(tools) == 0 {
			t.Error("expected research tools")
		}
		for _, tool := range tools {
			if tool == "write_file" || tool == "edit_file" || tool == "apply_patch" {
				t.Errorf("research phase should not have write tool: %s", tool)
			}
		}
	})

	t.Run("impl has write tools", func(t *testing.T) {
		tools := c.GetPhaseTools(PhaseImplementation)
		hasWrite := false
		for _, tool := range tools {
			if tool == "write_file" || tool == "edit_file" {
				hasWrite = true
			}
		}
		if !hasWrite {
			t.Error("implementation phase should have write tools")
		}
	})

	t.Run("synthesis has no tools", func(t *testing.T) {
		tools := c.GetPhaseTools(PhaseSynthesis)
		if tools != nil {
			t.Error("synthesis phase should have nil tools (coordinator handles it)")
		}
	})

	t.Run("verify has bash for tests", func(t *testing.T) {
		tools := c.GetPhaseTools(PhaseVerification)
		hasBash := false
		for _, tool := range tools {
			if tool == "bash" {
				hasBash = true
			}
		}
		if !hasBash {
			t.Error("verification phase should have bash for running tests")
		}
	})
}

func coordTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestCoordinatorRunPhaseParallel(t *testing.T) {
	var concurrentCount atomic.Int32
	var maxConcurrent atomic.Int32

	spawner := func(ctx context.Context, task WorkerTask, tools []string) (string, error) {
		cur := concurrentCount.Add(1)
		defer concurrentCount.Add(-1)

		// Track max concurrent workers.
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}

		time.Sleep(30 * time.Millisecond)
		return fmt.Sprintf("result for %s", task.Label), nil
	}

	cfg := DefaultCoordinatorConfig()
	cfg.MaxResearchWorkers = 3
	c := NewCoordinator(cfg, spawner, coordTestLogger())

	tasks := []WorkerTask{
		{ID: "1", Label: "task-a", Prompt: "research A"},
		{ID: "2", Label: "task-b", Prompt: "research B"},
		{ID: "3", Label: "task-c", Prompt: "research C"},
		{ID: "4", Label: "task-d", Prompt: "research D"},
	}

	result := c.RunPhase(context.Background(), PhaseResearch, tasks)

	if len(result.Results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(result.Results))
	}

	// All should succeed.
	for i, r := range result.Results {
		if r.Error != nil {
			t.Errorf("result[%d] unexpected error: %v", i, r.Error)
		}
		if !strings.Contains(r.Result, r.Label) {
			t.Errorf("result[%d] expected label in result, got %q", i, r.Result)
		}
	}

	// Should have run with concurrency (max 3 workers).
	mc := maxConcurrent.Load()
	if mc > 3 {
		t.Errorf("max concurrent exceeded limit: %d > 3", mc)
	}
	if mc < 2 {
		t.Errorf("expected parallel execution, max concurrent was only %d", mc)
	}
}

func TestCoordinatorRunPhaseWithError(t *testing.T) {
	spawner := func(ctx context.Context, task WorkerTask, tools []string) (string, error) {
		if task.ID == "2" {
			return "", fmt.Errorf("task 2 failed")
		}
		return "ok", nil
	}

	cfg := DefaultCoordinatorConfig()
	c := NewCoordinator(cfg, spawner, coordTestLogger())

	tasks := []WorkerTask{
		{ID: "1", Label: "good-task"},
		{ID: "2", Label: "bad-task"},
		{ID: "3", Label: "another-good"},
	}

	result := c.RunPhase(context.Background(), PhaseImplementation, tasks)

	if result.Results[0].Error != nil {
		t.Error("task 1 should succeed")
	}
	if result.Results[1].Error == nil {
		t.Error("task 2 should fail")
	}
	if result.Results[2].Error != nil {
		t.Error("task 3 should succeed")
	}
}

func TestCoordinatorRunPhaseWithContextCancel(t *testing.T) {
	spawner := func(ctx context.Context, task WorkerTask, tools []string) (string, error) {
		select {
		case <-time.After(5 * time.Second):
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	cfg := DefaultCoordinatorConfig()
	c := NewCoordinator(cfg, spawner, coordTestLogger())

	tasks := []WorkerTask{
		{ID: "1", Label: "slow", Timeout: 50 * time.Millisecond},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := c.RunPhase(ctx, PhaseResearch, tasks)

	if result.Results[0].Error == nil {
		t.Error("expected error from context cancellation")
	}
}

func TestCoordinatorEmptyPhase(t *testing.T) {
	cfg := DefaultCoordinatorConfig()
	c := NewCoordinator(cfg, nil, coordTestLogger())

	result := c.RunPhase(context.Background(), PhaseResearch, nil)
	if len(result.Results) != 0 {
		t.Errorf("expected 0 results for empty phase, got %d", len(result.Results))
	}
}

func TestSynthesizeFindings(t *testing.T) {
	results := []WorkerResult{
		{TaskID: "1", Label: "Find API endpoints", Result: "Found 3 endpoints: /api/v1/users, /api/v1/auth, /api/v1/data"},
		{TaskID: "2", Label: "Check dependencies", Error: fmt.Errorf("timeout")},
		{TaskID: "3", Label: "Review auth flow", Result: "Auth uses JWT with 1h expiry"},
	}

	summary := SynthesizeFindings(results)

	if !strings.Contains(summary, "Find API endpoints") {
		t.Error("expected task 1 label in summary")
	}
	if !strings.Contains(summary, "3 endpoints") {
		t.Error("expected task 1 result in summary")
	}
	if !strings.Contains(summary, "timeout") {
		t.Error("expected task 2 error in summary")
	}
	if !strings.Contains(summary, "JWT") {
		t.Error("expected task 3 result in summary")
	}
}

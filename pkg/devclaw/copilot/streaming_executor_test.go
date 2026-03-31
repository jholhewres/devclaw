package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func newTestExecutor() *ToolExecutor {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return NewToolExecutor(logger)
}

// registerTestTool registers a simple tool with configurable delay and concurrency.
func registerTestTool(exec *ToolExecutor, name string, concurrent bool, delay time.Duration, result string) {
	def := ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        name,
			Description: "test tool " + name,
			Parameters:  []byte(`{"type":"object","properties":{}}`),
		},
	}
	exec.Register(def, func(ctx context.Context, args map[string]any) (any, error) {
		select {
		case <-time.After(delay):
			return result, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	if concurrent {
		exec.MarkConcurrentSafe(name)
	}
}

// registerFailingTool registers a tool that returns an error after a delay.
func registerFailingTool(exec *ToolExecutor, name string, concurrent bool, delay time.Duration, errMsg string) {
	def := ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        name,
			Description: "failing test tool " + name,
			Parameters:  []byte(`{"type":"object","properties":{}}`),
		},
	}
	exec.Register(def, func(ctx context.Context, args map[string]any) (any, error) {
		select {
		case <-time.After(delay):
			return nil, fmt.Errorf("%s", errMsg)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	if concurrent {
		exec.MarkConcurrentSafe(name)
	}
}

func TestConcurrentSafeAnnotation(t *testing.T) {
	exec := newTestExecutor()

	registerTestTool(exec, "read_tool", true, 0, "ok")
	registerTestTool(exec, "write_tool", false, 0, "ok")

	t.Run("marked tool is concurrent-safe", func(t *testing.T) {
		if !exec.IsConcurrentSafe("read_tool") {
			t.Error("expected read_tool to be concurrent-safe")
		}
	})

	t.Run("unmarked tool is not concurrent-safe", func(t *testing.T) {
		if exec.IsConcurrentSafe("write_tool") {
			t.Error("expected write_tool to NOT be concurrent-safe")
		}
	})

	t.Run("unknown tool is not concurrent-safe", func(t *testing.T) {
		if exec.IsConcurrentSafe("nonexistent") {
			t.Error("expected nonexistent tool to NOT be concurrent-safe")
		}
	})
}

func TestApplyDefaultConcurrency(t *testing.T) {
	exec := newTestExecutor()

	// Register tools whose names match defaultConcurrentSafeTools.
	registerTestTool(exec, "grep", false, 0, "ok")
	registerTestTool(exec, "read_file", false, 0, "ok")
	registerTestTool(exec, "bash", false, 0, "ok")

	exec.ApplyDefaultConcurrency()

	if !exec.IsConcurrentSafe("grep") {
		t.Error("expected grep to be marked concurrent-safe by defaults")
	}
	if !exec.IsConcurrentSafe("read_file") {
		t.Error("expected read_file to be marked concurrent-safe by defaults")
	}
	if exec.IsConcurrentSafe("bash") {
		t.Error("expected bash to remain serial after ApplyDefaultConcurrency")
	}
}

func TestConcurrentToolsRunInParallel(t *testing.T) {
	exec := newTestExecutor()

	// Each tool sleeps 50ms. If sequential, total > 150ms. If parallel, ~50ms.
	registerTestTool(exec, "tool_a", true, 50*time.Millisecond, "result_a")
	registerTestTool(exec, "tool_b", true, 50*time.Millisecond, "result_b")
	registerTestTool(exec, "tool_c", true, 50*time.Millisecond, "result_c")

	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: "tool_a", Arguments: "{}"}},
		{ID: "2", Function: FunctionCall{Name: "tool_b", Arguments: "{}"}},
		{ID: "3", Function: FunctionCall{Name: "tool_c", Arguments: "{}"}},
	}

	start := time.Now()
	results := exec.Execute(context.Background(), calls)
	elapsed := time.Since(start)

	// Should complete in ~50ms (parallel), not ~150ms (sequential).
	if elapsed > 120*time.Millisecond {
		t.Errorf("concurrent tools took too long (%v), expected parallel execution", elapsed)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Results must be in original order.
	for i, r := range results {
		if r.Error != nil {
			t.Errorf("result[%d] unexpected error: %v", i, r.Error)
		}
	}
	if results[0].ToolCallID != "1" || results[1].ToolCallID != "2" || results[2].ToolCallID != "3" {
		t.Error("results not in original call order")
	}
}

func TestSerialToolsBlockParallel(t *testing.T) {
	exec := newTestExecutor()

	registerTestTool(exec, "read_tool", true, 30*time.Millisecond, "read_ok")
	registerTestTool(exec, "write_tool", false, 30*time.Millisecond, "write_ok")

	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: "read_tool", Arguments: "{}"}},
		{ID: "2", Function: FunctionCall{Name: "write_tool", Arguments: "{}"}},
		{ID: "3", Function: FunctionCall{Name: "read_tool", Arguments: "{}"}},
	}

	results := exec.Execute(context.Background(), calls)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// All should succeed.
	for i, r := range results {
		if r.Error != nil {
			t.Errorf("result[%d] unexpected error: %v", i, r.Error)
		}
	}

	// Results in original order.
	if results[0].ToolCallID != "1" || results[1].ToolCallID != "2" || results[2].ToolCallID != "3" {
		t.Error("results not in original call order")
	}
}

func TestMixedBatchGrouping(t *testing.T) {
	exec := newTestExecutor()

	// Track execution order via atomic counter.
	var counter atomic.Int32

	// Register with execution order tracking.
	for _, name := range []string{"r1", "r2", "r3"} {
		n := name
		def := ToolDefinition{
			Type: "function",
			Function: FunctionDef{
				Name:       n,
				Parameters: []byte(`{"type":"object","properties":{}}`),
			},
		}
		exec.Register(def, func(ctx context.Context, args map[string]any) (any, error) {
			order := counter.Add(1)
			time.Sleep(10 * time.Millisecond)
			return fmt.Sprintf("%s:order=%d", n, order), nil
		})
		exec.MarkConcurrentSafe(n)
	}

	def := ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:       "serial_w",
			Parameters: []byte(`{"type":"object","properties":{}}`),
		},
	}
	exec.Register(def, func(ctx context.Context, args map[string]any) (any, error) {
		order := counter.Add(1)
		return fmt.Sprintf("serial_w:order=%d", order), nil
	})

	// Batch: [r1, r2] (concurrent group) → [serial_w] (serial) → [r3] (concurrent single)
	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: "r1", Arguments: "{}"}},
		{ID: "2", Function: FunctionCall{Name: "r2", Arguments: "{}"}},
		{ID: "3", Function: FunctionCall{Name: "serial_w", Arguments: "{}"}},
		{ID: "4", Function: FunctionCall{Name: "r3", Arguments: "{}"}},
	}

	results := exec.Execute(context.Background(), calls)

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Error != nil {
			t.Errorf("result[%d] unexpected error: %v", i, r.Error)
		}
	}
	// Verify order is preserved.
	if results[0].ToolCallID != "1" {
		t.Errorf("expected result[0] ID=1, got %s", results[0].ToolCallID)
	}
	if results[2].ToolCallID != "3" {
		t.Errorf("expected result[2] ID=3, got %s", results[2].ToolCallID)
	}
}

func TestAbortCascadeCancelsSiblings(t *testing.T) {
	exec := newTestExecutor()

	var cancelledCount atomic.Int32

	// Register a fast-failing tool.
	failDef := ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:       "fast_fail",
			Parameters: []byte(`{"type":"object","properties":{}}`),
		},
	}
	exec.Register(failDef, func(ctx context.Context, args map[string]any) (any, error) {
		time.Sleep(10 * time.Millisecond)
		// Return a non-recoverable error to trigger abort cascade.
		return nil, fmt.Errorf("fatal: disk full")
	})
	exec.MarkConcurrentSafe("fast_fail")

	// Register slow tools that should get cancelled.
	for _, name := range []string{"slow_a", "slow_b"} {
		n := name
		def := ToolDefinition{
			Type: "function",
			Function: FunctionDef{
				Name:       n,
				Parameters: []byte(`{"type":"object","properties":{}}`),
			},
		}
		exec.Register(def, func(ctx context.Context, args map[string]any) (any, error) {
			select {
			case <-time.After(2 * time.Second):
				return "completed", nil
			case <-ctx.Done():
				cancelledCount.Add(1)
				return nil, ctx.Err()
			}
		})
		exec.MarkConcurrentSafe(n)
	}

	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: "fast_fail", Arguments: "{}"}},
		{ID: "2", Function: FunctionCall{Name: "slow_a", Arguments: "{}"}},
		{ID: "3", Function: FunctionCall{Name: "slow_b", Arguments: "{}"}},
	}

	start := time.Now()
	results := exec.Execute(context.Background(), calls)
	elapsed := time.Since(start)

	// Should complete much faster than 2s (the slow tools should be cancelled).
	if elapsed > 500*time.Millisecond {
		t.Errorf("abort cascade didn't cancel siblings fast enough (%v)", elapsed)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// The failing tool should have an error.
	if results[0].Error == nil {
		t.Error("expected fast_fail to have an error")
	}

	// At least one sibling should have been cancelled.
	cancelled := cancelledCount.Load()
	if cancelled == 0 {
		t.Error("expected at least one sibling to be cancelled by abort cascade")
	}
}

func TestRecoverableErrorDoesNotTriggerAbort(t *testing.T) {
	exec := newTestExecutor()

	// Register a tool that returns a recoverable error.
	recoverDef := ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:       "recoverable",
			Parameters: []byte(`{"type":"object","properties":{}}`),
		},
	}
	exec.Register(recoverDef, func(ctx context.Context, args map[string]any) (any, error) {
		return nil, fmt.Errorf("file not found: /tmp/missing.txt")
	})
	exec.MarkConcurrentSafe("recoverable")

	// Register a slow tool that should NOT be cancelled.
	slowDef := ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:       "slow_reader",
			Parameters: []byte(`{"type":"object","properties":{}}`),
		},
	}
	exec.Register(slowDef, func(ctx context.Context, args map[string]any) (any, error) {
		select {
		case <-time.After(80 * time.Millisecond):
			return "slow_done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	exec.MarkConcurrentSafe("slow_reader")

	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: "recoverable", Arguments: "{}"}},
		{ID: "2", Function: FunctionCall{Name: "slow_reader", Arguments: "{}"}},
	}

	results := exec.Execute(context.Background(), calls)

	// Recoverable error should NOT cancel the slow tool.
	if results[1].Error != nil {
		t.Errorf("slow_reader should have completed successfully, got error: %v", results[1].Error)
	}
	if results[1].Content == "[Cancelled: sibling tool failed]" {
		t.Error("slow_reader was cancelled by abort cascade, but error was recoverable")
	}
}

func TestResultsInOriginalOrder(t *testing.T) {
	exec := newTestExecutor()

	// Tools with varying delays — order of completion differs from call order.
	registerTestTool(exec, "slow", true, 80*time.Millisecond, "slow_result")
	registerTestTool(exec, "medium", true, 40*time.Millisecond, "medium_result")
	registerTestTool(exec, "fast", true, 5*time.Millisecond, "fast_result")

	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: "slow", Arguments: "{}"}},
		{ID: "2", Function: FunctionCall{Name: "medium", Arguments: "{}"}},
		{ID: "3", Function: FunctionCall{Name: "fast", Arguments: "{}"}},
	}

	results := exec.Execute(context.Background(), calls)

	// Despite different completion times, results must match input order.
	if results[0].ToolCallID != "1" || results[0].Name != "slow" {
		t.Errorf("result[0] expected slow (ID=1), got %s (ID=%s)", results[0].Name, results[0].ToolCallID)
	}
	if results[1].ToolCallID != "2" || results[1].Name != "medium" {
		t.Errorf("result[1] expected medium (ID=2), got %s (ID=%s)", results[1].Name, results[1].ToolCallID)
	}
	if results[2].ToolCallID != "3" || results[2].Name != "fast" {
		t.Errorf("result[2] expected fast (ID=3), got %s (ID=%s)", results[2].Name, results[2].ToolCallID)
	}
}

func TestSingleCallBypassesStreaming(t *testing.T) {
	exec := newTestExecutor()
	registerTestTool(exec, "solo", true, 10*time.Millisecond, "solo_result")

	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: "solo", Arguments: "{}"}},
	}

	results := exec.Execute(context.Background(), calls)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Errorf("unexpected error: %v", results[0].Error)
	}
}

func TestContextCancellationStopsExecution(t *testing.T) {
	exec := newTestExecutor()

	registerTestTool(exec, "long_a", true, 5*time.Second, "never")
	registerTestTool(exec, "long_b", true, 5*time.Second, "never")

	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: "long_a", Arguments: "{}"}},
		{ID: "2", Function: FunctionCall{Name: "long_b", Arguments: "{}"}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	results := exec.Execute(ctx, calls)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("context cancellation didn't stop execution fast enough (%v)", elapsed)
	}

	// Both results should have errors from context cancellation.
	for i, r := range results {
		if r.Error == nil {
			t.Errorf("result[%d] expected error from cancellation, got nil", i)
		}
	}
}

func TestParallelDisabledForcesSequential(t *testing.T) {
	exec := newTestExecutor()
	exec.Configure(ToolExecutorConfig{
		Parallel:    false,
		MaxParallel: 5,
	})

	registerTestTool(exec, "a", true, 30*time.Millisecond, "ok")
	registerTestTool(exec, "b", true, 30*time.Millisecond, "ok")

	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: "a", Arguments: "{}"}},
		{ID: "2", Function: FunctionCall{Name: "b", Arguments: "{}"}},
	}

	start := time.Now()
	results := exec.Execute(context.Background(), calls)
	elapsed := time.Since(start)

	// With parallel=false, should take ~60ms (sequential), not ~30ms.
	if elapsed < 55*time.Millisecond {
		t.Errorf("expected sequential execution (~60ms), got %v", elapsed)
	}

	for i, r := range results {
		if r.Error != nil {
			t.Errorf("result[%d] unexpected error: %v", i, r.Error)
		}
	}
}

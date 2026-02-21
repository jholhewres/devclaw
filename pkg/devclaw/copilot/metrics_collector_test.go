// Package copilot – metrics_collector_test.go tests the metrics collector.
package copilot

import (
	"context"
	"testing"
	"time"
)

func TestMetricsCollector_RecordMessage(t *testing.T) {
	cfg := MetricsCollectorConfig{
		Enabled:  true,
		Interval: 1 * time.Second,
	}
	mc := NewMetricsCollector(cfg, nil)

	// Record messages
	mc.RecordMessage()
	mc.RecordMessage()
	mc.RecordMessage()

	if mc.messagesTotal.Load() != 3 {
		t.Errorf("Expected 3 messages, got %d", mc.messagesTotal.Load())
	}
}

func TestMetricsCollector_RecordTokens(t *testing.T) {
	cfg := MetricsCollectorConfig{
		Enabled:  true,
		Interval: 1 * time.Second,
	}
	mc := NewMetricsCollector(cfg, nil)

	mc.RecordTokens(100)
	mc.RecordTokens(50)

	if mc.tokensTotal.Load() != 150 {
		t.Errorf("Expected 150 tokens, got %d", mc.tokensTotal.Load())
	}
}

func TestMetricsCollector_RecordAgentRun(t *testing.T) {
	cfg := MetricsCollectorConfig{
		Enabled:  true,
		Interval: 1 * time.Second,
	}
	mc := NewMetricsCollector(cfg, nil)

	// Start run
	mc.RecordAgentRunStart()
	if mc.agentRunsActive.Load() != 1 {
		t.Errorf("Expected 1 active run, got %d", mc.agentRunsActive.Load())
	}

	// Complete successfully
	mc.RecordAgentRunComplete(true, false)
	if mc.agentRunsActive.Load() != 0 {
		t.Errorf("Expected 0 active runs, got %d", mc.agentRunsActive.Load())
	}
	if mc.agentRunsSuccess.Load() != 1 {
		t.Errorf("Expected 1 success, got %d", mc.agentRunsSuccess.Load())
	}

	// Start and fail
	mc.RecordAgentRunStart()
	mc.RecordAgentRunComplete(false, false)
	if mc.agentRunsFailed.Load() != 1 {
		t.Errorf("Expected 1 failed, got %d", mc.agentRunsFailed.Load())
	}

	// Start and timeout
	mc.RecordAgentRunStart()
	mc.RecordAgentRunComplete(false, true)
	if mc.agentRunsTimeout.Load() != 1 {
		t.Errorf("Expected 1 timeout, got %d", mc.agentRunsTimeout.Load())
	}
}

func TestMetricsCollector_RecordToolCall(t *testing.T) {
	cfg := MetricsCollectorConfig{
		Enabled:  true,
		Interval: 1 * time.Second,
	}
	mc := NewMetricsCollector(cfg, nil)

	mc.RecordToolCall(true)
	mc.RecordToolCall(true)
	mc.RecordToolCall(false)

	if mc.toolCallsTotal.Load() != 3 {
		t.Errorf("Expected 3 total tool calls, got %d", mc.toolCallsTotal.Load())
	}
	if mc.toolCallsSuccess.Load() != 2 {
		t.Errorf("Expected 2 success, got %d", mc.toolCallsSuccess.Load())
	}
	if mc.toolCallsFailed.Load() != 1 {
		t.Errorf("Expected 1 failed, got %d", mc.toolCallsFailed.Load())
	}
}

func TestMetricsCollector_RecordSubagent(t *testing.T) {
	cfg := MetricsCollectorConfig{
		Enabled:  true,
		Interval: 1 * time.Second,
	}
	mc := NewMetricsCollector(cfg, nil)

	mc.RecordSubagentSpawn()
	mc.RecordSubagentSpawn()
	if mc.subagentsActive.Load() != 2 {
		t.Errorf("Expected 2 active subagents, got %d", mc.subagentsActive.Load())
	}

	mc.RecordSubagentComplete(true)
	if mc.subagentsSuccess.Load() != 1 {
		t.Errorf("Expected 1 success, got %d", mc.subagentsSuccess.Load())
	}

	mc.RecordSubagentComplete(false)
	if mc.subagentsFailed.Load() != 1 {
		t.Errorf("Expected 1 failed, got %d", mc.subagentsFailed.Load())
	}
}

func TestMetricsCollector_RecordLatency(t *testing.T) {
	cfg := MetricsCollectorConfig{
		Enabled:  true,
		Interval: 1 * time.Second,
	}
	mc := NewMetricsCollector(cfg, nil)

	mc.RecordLatency(100)
	mc.RecordLatency(200)
	mc.RecordLatency(300)

	mc.latenciesMu.Lock()
	if len(mc.latencies) != 3 {
		t.Errorf("Expected 3 latency recordings, got %d", len(mc.latencies))
	}
	mc.latenciesMu.Unlock()
}

func TestMetricsCollector_Collect(t *testing.T) {
	cfg := MetricsCollectorConfig{
		Enabled:  true,
		Interval: 1 * time.Second,
	}
	mc := NewMetricsCollector(cfg, nil)

	// Set callbacks
	mc.SetSessionsCountFunc(func() int64 { return 5 })

	// Record some data
	mc.RecordMessage()
	mc.RecordMessage()
	mc.RecordTokens(100)
	mc.RecordError()

	// Wait a bit for time diff
	time.Sleep(10 * time.Millisecond)

	// Collect
	snapshot := mc.collect()

	if snapshot.MessagesTotal != 2 {
		t.Errorf("Expected 2 messages total, got %d", snapshot.MessagesTotal)
	}
	if snapshot.TokensTotal != 100 {
		t.Errorf("Expected 100 tokens total, got %d", snapshot.TokensTotal)
	}
	if snapshot.ErrorsTotal != 1 {
		t.Errorf("Expected 1 error total, got %d", snapshot.ErrorsTotal)
	}
	if snapshot.SessionsActive != 5 {
		t.Errorf("Expected 5 sessions active, got %d", snapshot.SessionsActive)
	}
	if snapshot.Goroutines == 0 {
		t.Error("Goroutines should be > 0")
	}
	// Uptime can be 0 for a freshly created collector that runs in < 1 second
	if snapshot.UptimeSeconds < 0 {
		t.Error("Uptime should be >= 0")
	}
}

func TestMetricsCollector_Subscribe(t *testing.T) {
	cfg := MetricsCollectorConfig{
		Enabled:  true,
		Interval: 100 * time.Millisecond,
	}
	mc := NewMetricsCollector(cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mc.Start(ctx)

	// Subscribe
	ch := mc.Subscribe()

	// Wait for snapshot
	select {
	case snapshot := <-ch:
		if snapshot.Timestamp.IsZero() {
			t.Error("Snapshot timestamp should not be zero")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for snapshot")
	}

	// Unsubscribe
	mc.Unsubscribe(ch)
}

func TestMetricsCollector_Latest(t *testing.T) {
	cfg := MetricsCollectorConfig{
		Enabled:  true,
		Interval: 1 * time.Second,
	}
	mc := NewMetricsCollector(cfg, nil)

	// Initially nil
	if mc.Latest() != nil {
		t.Error("Expected nil latest before collect")
	}

	// Collect
	mc.collect()

	// Now should have data
	latest := mc.Latest()
	if latest == nil {
		t.Fatal("Expected non-nil latest after collect")
	}
	if latest.Timestamp.IsZero() {
		t.Error("Latest timestamp should not be zero")
	}
}

func TestMetricsCollector_Stop(t *testing.T) {
	cfg := MetricsCollectorConfig{
		Enabled:  true,
		Interval: 1 * time.Second,
	}
	mc := NewMetricsCollector(cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- mc.Start(ctx)
	}()

	// Stop after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for collector to stop")
	}
}

func TestCalculateAvg(t *testing.T) {
	tests := []struct {
		values   []int64
		expected int64
	}{
		{[]int64{}, 0},
		{[]int64{100}, 100},
		{[]int64{100, 200, 300}, 200},
		{[]int64{50, 50, 50, 50}, 50},
	}

	for _, tt := range tests {
		result := calculateAvg(tt.values)
		if result != tt.expected {
			t.Errorf("calculateAvg(%v) = %d, expected %d", tt.values, result, tt.expected)
		}
	}
}

func TestCalculatePercentile(t *testing.T) {
	values := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}

	tests := []struct {
		percentile int
		expected   int64
	}{
		{50, 50},  // median
		{0, 10},   // min
		{100, 100}, // max
		{99, 100},
		{10, 10},
	}

	for _, tt := range tests {
		result := calculatePercentile(values, tt.percentile)
		if result != tt.expected {
			t.Errorf("calculatePercentile(%d) = %d, expected %d", tt.percentile, result, tt.expected)
		}
	}

	// Empty slice
	if calculatePercentile([]int64{}, 50) != 0 {
		t.Error("calculatePercentile of empty slice should be 0")
	}
}

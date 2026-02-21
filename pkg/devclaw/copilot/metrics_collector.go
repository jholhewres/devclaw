// Package copilot – metrics_collector.go provides background metrics collection.
package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsSnapshot represents a point-in-time collection of system metrics.
type MetricsSnapshot struct {
	Timestamp time.Time `json:"timestamp"`

	// Message metrics
	MessagesTotal     int64 `json:"messages_total"`
	MessagesPerMinute int64 `json:"messages_per_minute"`

	// Token metrics
	TokensTotal     int64 `json:"tokens_total"`
	TokensPerMinute int64 `json:"tokens_per_minute"`

	// Agent metrics
	AgentRunsTotal   int64 `json:"agent_runs_total"`
	AgentRunsActive  int64 `json:"agent_runs_active"`
	AgentRunsSuccess int64 `json:"agent_runs_success"`
	AgentRunsFailed  int64 `json:"agent_runs_failed"`
	AgentRunsTimeout int64 `json:"agent_runs_timeout"`

	// Tool metrics
	ToolCallsTotal   int64 `json:"tool_calls_total"`
	ToolCallsSuccess int64 `json:"tool_calls_success"`
	ToolCallsFailed  int64 `json:"tool_calls_failed"`

	// Subagent metrics
	SubagentsTotal   int64 `json:"subagents_total"`
	SubagentsActive  int64 `json:"subagents_active"`
	SubagentsSuccess int64 `json:"subagents_success"`
	SubagentsFailed  int64 `json:"subagents_failed"`

	// System metrics
	Goroutines    int64 `json:"goroutines"`
	MemoryAllocMB int64 `json:"memory_alloc_mb"`
	MemorySysMB   int64 `json:"memory_sys_mb"`

	// Session metrics
	SessionsActive int64 `json:"sessions_active"`
	SessionsTotal  int64 `json:"sessions_total"`

	// Error metrics
	ErrorsTotal  int64 `json:"errors_total"`
	ErrorsRecent int64 `json:"errors_recent"` // last interval

	// Latency metrics (milliseconds)
	LatencyAvgMs int64 `json:"latency_avg_ms"`
	LatencyP50Ms int64 `json:"latency_p50_ms"`
	LatencyP99Ms int64 `json:"latency_p99_ms"`

	// Database metrics
	DBSizeMB    int64 `json:"db_size_mb"`
	DBQueries   int64 `json:"db_queries"`
	DBSlowQuery int64 `json:"db_slow_query"`

	// Uptime
	UptimeSeconds int64 `json:"uptime_seconds"`
}

// MetricsCollector collects and aggregates system metrics periodically.
type MetricsCollector struct {
	interval time.Duration
	logger   *slog.Logger
	webhook  string // optional webhook for external reporting

	// Counters (atomic)
	messagesTotal    atomic.Int64
	tokensTotal      atomic.Int64
	agentRunsTotal   atomic.Int64
	agentRunsSuccess atomic.Int64
	agentRunsFailed  atomic.Int64
	agentRunsTimeout atomic.Int64
	toolCallsTotal   atomic.Int64
	toolCallsSuccess atomic.Int64
	toolCallsFailed  atomic.Int64
	subagentsTotal   atomic.Int64
	subagentsSuccess atomic.Int64
	subagentsFailed  atomic.Int64
	errorsTotal      atomic.Int64
	sessionsTotal    atomic.Int64
	dbQueries        atomic.Int64
	dbSlowQuery      atomic.Int64

	// Gauges (atomic)
	agentRunsActive atomic.Int64
	subagentsActive atomic.Int64

	// Latency tracking
	latenciesMu sync.Mutex
	latencies   []int64

	// Previous snapshot for rate calculations
	prevMessages int64
	prevTokens   int64
	prevErrors   int64
	prevTime     time.Time

	// Start time for uptime
	startTime time.Time

	// Callbacks for external data
	sessionsCountFunc  func() int64
	dbSizeFunc         func() int64
	messagesQueueFunc  func() int64
	subagentsCountFunc func() int64

	// Latest snapshot
	latestMu     sync.RWMutex
	latest       *MetricsSnapshot
	subscribers  []chan MetricsSnapshot

	ctx    context.Context
	cancel context.CancelFunc
}

// MetricsCollectorConfig configures the metrics collector.
type MetricsCollectorConfig struct {
	Enabled  bool          `yaml:"enabled" json:"enabled"`
	Interval time.Duration `yaml:"interval" json:"interval"`
	Webhook  string        `yaml:"webhook" json:"webhook"`
}

// DefaultMetricsCollectorConfig returns default configuration.
func DefaultMetricsCollectorConfig() MetricsCollectorConfig {
	return MetricsCollectorConfig{
		Enabled:  false,
		Interval: 1 * time.Minute,
		Webhook:  "",
	}
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(cfg MetricsCollectorConfig, logger *slog.Logger) *MetricsCollector {
	if logger == nil {
		logger = slog.Default()
	}

	interval := cfg.Interval
	if interval <= 0 {
		interval = 1 * time.Minute
	}

	return &MetricsCollector{
		interval:  interval,
		logger:    logger.With("component", "metrics-collector"),
		webhook:   cfg.Webhook,
		latencies: make([]int64, 0, 1000),
		startTime: time.Now(),
		prevTime:  time.Now(),
	}
}

// SetSessionsCountFunc sets the callback for getting active sessions count.
func (m *MetricsCollector) SetSessionsCountFunc(fn func() int64) {
	m.sessionsCountFunc = fn
}

// SetDBSizeFunc sets the callback for getting database size.
func (m *MetricsCollector) SetDBSizeFunc(fn func() int64) {
	m.dbSizeFunc = fn
}

// SetMessagesQueueFunc sets the callback for getting queued messages count.
func (m *MetricsCollector) SetMessagesQueueFunc(fn func() int64) {
	m.messagesQueueFunc = fn
}

// SetSubagentsCountFunc sets the callback for getting active subagents count.
func (m *MetricsCollector) SetSubagentsCountFunc(fn func() int64) {
	m.subagentsCountFunc = fn
}

// Start begins periodic metrics collection.
func (m *MetricsCollector) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	m.logger.Info("metrics collector started",
		"interval", m.interval.String(),
		"webhook_configured", m.webhook != "",
	)

	// Collect initial snapshot
	m.collect()

	for {
		select {
		case <-ticker.C:
			snapshot := m.collect()
			m.notifySubscribers(snapshot)
			m.sendWebhook(snapshot)
		case <-m.ctx.Done():
			m.logger.Info("metrics collector stopped")
			return m.ctx.Err()
		}
	}
}

// Stop stops the metrics collector.
func (m *MetricsCollector) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// collect gathers all metrics and returns a snapshot.
func (m *MetricsCollector) collect() MetricsSnapshot {
	now := time.Now()
	snapshot := MetricsSnapshot{
		Timestamp: now,
	}

	// Message metrics
	snapshot.MessagesTotal = m.messagesTotal.Load()
	timeDiffSeconds := now.Sub(m.prevTime).Seconds()
	if timeDiffSeconds >= 1.0 { // Only calculate rate if at least 1 second has passed
		snapshot.MessagesPerMinute = int64(float64(snapshot.MessagesTotal-m.prevMessages) * 60.0 / timeDiffSeconds)
	}

	// Token metrics
	snapshot.TokensTotal = m.tokensTotal.Load()
	if timeDiffSeconds >= 1.0 {
		snapshot.TokensPerMinute = int64(float64(snapshot.TokensTotal-m.prevTokens) * 60.0 / timeDiffSeconds)
	}

	// Agent run metrics
	snapshot.AgentRunsTotal = m.agentRunsTotal.Load()
	snapshot.AgentRunsActive = m.agentRunsActive.Load()
	snapshot.AgentRunsSuccess = m.agentRunsSuccess.Load()
	snapshot.AgentRunsFailed = m.agentRunsFailed.Load()
	snapshot.AgentRunsTimeout = m.agentRunsTimeout.Load()

	// Tool metrics
	snapshot.ToolCallsTotal = m.toolCallsTotal.Load()
	snapshot.ToolCallsSuccess = m.toolCallsSuccess.Load()
	snapshot.ToolCallsFailed = m.toolCallsFailed.Load()

	// Subagent metrics
	snapshot.SubagentsTotal = m.subagentsTotal.Load()
	snapshot.SubagentsActive = m.subagentsActive.Load()
	snapshot.SubagentsSuccess = m.subagentsSuccess.Load()
	snapshot.SubagentsFailed = m.subagentsFailed.Load()

	// Error metrics
	snapshot.ErrorsTotal = m.errorsTotal.Load()
	snapshot.ErrorsRecent = snapshot.ErrorsTotal - m.prevErrors

	// System metrics
	snapshot.Goroutines = int64(runtime.NumGoroutine())
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	snapshot.MemoryAllocMB = int64(memStats.Alloc / 1024 / 1024)
	snapshot.MemorySysMB = int64(memStats.Sys / 1024 / 1024)

	// Session metrics
	if m.sessionsCountFunc != nil {
		snapshot.SessionsActive = m.sessionsCountFunc()
	}
	snapshot.SessionsTotal = m.sessionsTotal.Load()

	// Latency metrics
	m.latenciesMu.Lock()
	if len(m.latencies) > 0 {
		snapshot.LatencyAvgMs = calculateAvg(m.latencies)
		snapshot.LatencyP50Ms = calculatePercentile(m.latencies, 50)
		snapshot.LatencyP99Ms = calculatePercentile(m.latencies, 99)
		// Clear latencies for next interval
		m.latencies = m.latencies[:0]
	}
	m.latenciesMu.Unlock()

	// Database metrics
	if m.dbSizeFunc != nil {
		snapshot.DBSizeMB = m.dbSizeFunc()
	}
	snapshot.DBQueries = m.dbQueries.Load()
	snapshot.DBSlowQuery = m.dbSlowQuery.Load()

	// Uptime
	snapshot.UptimeSeconds = int64(now.Sub(m.startTime).Seconds())

	// Update previous values for rate calculations
	m.prevMessages = snapshot.MessagesTotal
	m.prevTokens = snapshot.TokensTotal
	m.prevErrors = snapshot.ErrorsTotal
	m.prevTime = now

	// Store latest snapshot
	m.latestMu.Lock()
	m.latest = &snapshot
	m.latestMu.Unlock()

	return snapshot
}

// notifySubscribers sends snapshot to all subscribers.
func (m *MetricsCollector) notifySubscribers(snapshot MetricsSnapshot) {
	m.latestMu.RLock()
	subscribers := make([]chan MetricsSnapshot, len(m.subscribers))
	copy(subscribers, m.subscribers)
	m.latestMu.RUnlock()

	for _, ch := range subscribers {
		select {
		case ch <- snapshot:
		default:
			// Channel full, skip
		}
	}
}

// sendWebhook sends metrics to configured webhook endpoint.
func (m *MetricsCollector) sendWebhook(snapshot MetricsSnapshot) {
	if m.webhook == "" {
		return
	}

	// Send asynchronously to not block collection
	go func() {
		payload, err := json.Marshal(snapshot)
		if err != nil {
			m.logger.Warn("failed to marshal metrics for webhook", "error", err)
			return
		}

		// Use HTTP client with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "POST", m.webhook, bytes.NewReader(payload))
		if err != nil {
			m.logger.Warn("failed to create webhook request", "error", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			m.logger.Warn("failed to send metrics webhook", "error", err)
			return
		}
		resp.Body.Close()

		m.logger.Debug("metrics webhook sent", "status", resp.StatusCode)
	}()
}

// Subscribe returns a channel that receives metrics snapshots.
func (m *MetricsCollector) Subscribe() <-chan MetricsSnapshot {
	ch := make(chan MetricsSnapshot, 10)
	m.latestMu.Lock()
	m.subscribers = append(m.subscribers, ch)
	m.latestMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber.
func (m *MetricsCollector) Unsubscribe(ch <-chan MetricsSnapshot) {
	m.latestMu.Lock()
	for i, sub := range m.subscribers {
		// Compare the receive-only channel with the send channel
		if sub == ch {
			m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
			close(sub)
			break
		}
	}
	m.latestMu.Unlock()
}

// Latest returns the most recent metrics snapshot.
func (m *MetricsCollector) Latest() *MetricsSnapshot {
	m.latestMu.RLock()
	defer m.latestMu.RUnlock()
	return m.latest
}

// --- Recording methods ---

// RecordMessage increments message counter.
func (m *MetricsCollector) RecordMessage() {
	m.messagesTotal.Add(1)
}

// RecordTokens increments token counter.
func (m *MetricsCollector) RecordTokens(count int64) {
	m.tokensTotal.Add(count)
}

// RecordAgentRunStart increments active runs.
func (m *MetricsCollector) RecordAgentRunStart() {
	m.agentRunsTotal.Add(1)
	m.agentRunsActive.Add(1)
}

// RecordAgentRunComplete decrements active runs and records outcome.
func (m *MetricsCollector) RecordAgentRunComplete(success bool, timedOut bool) {
	m.agentRunsActive.Add(-1)
	if success {
		m.agentRunsSuccess.Add(1)
	} else if timedOut {
		m.agentRunsTimeout.Add(1)
	} else {
		m.agentRunsFailed.Add(1)
	}
}

// RecordToolCall records a tool execution.
func (m *MetricsCollector) RecordToolCall(success bool) {
	m.toolCallsTotal.Add(1)
	if success {
		m.toolCallsSuccess.Add(1)
	} else {
		m.toolCallsFailed.Add(1)
	}
}

// RecordSubagentSpawn records a subagent creation.
func (m *MetricsCollector) RecordSubagentSpawn() {
	m.subagentsTotal.Add(1)
	m.subagentsActive.Add(1)
}

// RecordSubagentComplete records a subagent completion.
func (m *MetricsCollector) RecordSubagentComplete(success bool) {
	m.subagentsActive.Add(-1)
	if success {
		m.subagentsSuccess.Add(1)
	} else {
		m.subagentsFailed.Add(1)
	}
}

// RecordError increments error counter.
func (m *MetricsCollector) RecordError() {
	m.errorsTotal.Add(1)
}

// RecordLatency records a latency measurement in milliseconds.
func (m *MetricsCollector) RecordLatency(ms int64) {
	m.latenciesMu.Lock()
	m.latencies = append(m.latencies, ms)
	// Keep last 1000 measurements
	if len(m.latencies) > 1000 {
		m.latencies = m.latencies[len(m.latencies)-1000:]
	}
	m.latenciesMu.Unlock()
}

// RecordDBQuery records a database query.
func (m *MetricsCollector) RecordDBQuery(slow bool) {
	m.dbQueries.Add(1)
	if slow {
		m.dbSlowQuery.Add(1)
	}
}

// RecordSessionCreated records a new session.
func (m *MetricsCollector) RecordSessionCreated() {
	m.sessionsTotal.Add(1)
}

// Helper functions for percentile calculations
func calculateAvg(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	var sum int64
	for _, v := range values {
		sum += v
	}
	return sum / int64(len(values))
}

func calculatePercentile(values []int64, percentile int) int64 {
	if len(values) == 0 {
		return 0
	}

	// Sort a copy
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	n := len(sorted)

	// Nearest-rank method: r = ceil(p/100 * n)
	// Using integer math: (p*n - 1)/100 + 1 gives us ceil(p*n/100) for positive integers
	rank := (percentile*n-1)/100 + 1
	if rank > n {
		rank = n
	}

	return sorted[rank-1] // Convert to 0-indexed
}

// Package copilot – techops_types.go provides data structures for TechOps commands.
package copilot

import (
	"time"
)

// MaintenanceMode represents the system maintenance state.
type MaintenanceMode struct {
	Enabled bool      `json:"enabled"`
	Message string    `json:"message"`
	SetBy   string    `json:"set_by"`
	SetAt   time.Time `json:"set_at"`
}

// SystemStatus represents comprehensive system status for TechOps commands.
type SystemStatus struct {
	Version       string                  `json:"version"`
	Uptime        string                  `json:"uptime"`
	UptimeSeconds int64                   `json:"uptime_seconds"`
	MemoryMB      float64                 `json:"memory_mb"`
	GoRoutines    int                     `json:"goroutines"`
	Channels      map[string]ChannelHealth `json:"channels"`
	Sessions      SessionStats            `json:"sessions"`
	Scheduler     SchedulerStats          `json:"scheduler"`
	Skills        int                     `json:"skills"`
	Maintenance   *MaintenanceMode        `json:"maintenance,omitempty"`
}

// ChannelHealth represents health status of a single channel.
type ChannelHealth struct {
	Connected     bool              `json:"connected"`
	LastMessageAt time.Time         `json:"last_message_at,omitempty"`
	ErrorCount    int               `json:"error_count"`
	LatencyMs     int64             `json:"latency_ms"`
	Details       map[string]any    `json:"details,omitempty"`
}

// SessionStats holds session-related statistics.
type SessionStats struct {
	Active      int `json:"active"`
	Total       int `json:"total"`
	WithHistory int `json:"with_history"`
}

// SchedulerStats holds scheduler statistics.
type SchedulerStats struct {
	Enabled bool   `json:"enabled"`
	Jobs    int    `json:"jobs"`
	NextRun string `json:"next_run,omitempty"`
	Running int    `json:"running"`
}

// DiagnosticsResult represents comprehensive system diagnostics.
type DiagnosticsResult struct {
	Database     DatabaseHealth      `json:"database"`
	Config       ConfigHealth        `json:"config"`
	Channels     []ChannelDiagnostic `json:"channels"`
	RecentErrors []AuditRecordShort  `json:"recent_errors,omitempty"`
	Memory       MemoryStats         `json:"memory"`
	Disk         DiskStats           `json:"disk"`
}

// DatabaseHealth represents database health status.
type DatabaseHealth struct {
	Connected bool              `json:"connected"`
	SizeMB    float64           `json:"size_mb"`
	Tables    map[string]int    `json:"tables"`
	Error     string            `json:"error,omitempty"`
}

// ConfigHealth represents configuration health status.
type ConfigHealth struct {
	Valid    bool     `json:"valid"`
	Path     string   `json:"path"`
	Errors   []string `json:"errors,omitempty"`
	Sections []string `json:"sections"`
}

// ChannelDiagnostic represents diagnostic result for a single channel.
type ChannelDiagnostic struct {
	Name       string `json:"name"`
	Connected  bool   `json:"connected"`
	TestResult string `json:"test_result"`
	LatencyMs  int64  `json:"latency_ms"`
}

// MemoryStats represents runtime memory statistics.
type MemoryStats struct {
	AllocMB      float64 `json:"alloc_mb"`
	TotalAllocMB float64 `json:"total_alloc_mb"`
	SysMB        float64 `json:"sys_mb"`
	NumGC        uint32  `json:"num_gc"`
}

// DiskStats represents disk usage statistics.
type DiskStats struct {
	TotalGB float64 `json:"total_gb"`
	FreeGB  float64 `json:"free_gb"`
	UsedPct float64 `json:"used_pct"`
}

// AuditRecordShort is a shortened version of AuditRecord for display.
type AuditRecordShort struct {
	ID          int64     `json:"id"`
	Tool        string    `json:"tool"`
	Caller      string    `json:"caller"`
	Level       string    `json:"level"`
	Allowed     bool      `json:"allowed"`
	ArgsSummary string    `json:"args_summary"`
	CreatedAt   time.Time `json:"created_at"`
}

// MetricsResult represents usage metrics for a time period.
type MetricsResult struct {
	Period    string                   `json:"period"`
	StartTime time.Time                `json:"start_time"`
	EndTime   time.Time                `json:"end_time"`
	Total     MetricsTotals            `json:"total"`
	ByModel   map[string]MetricsTotals `json:"by_model"`
	TopUsers  []UserUsage              `json:"top_users"`
}

// MetricsTotals holds aggregated usage totals.
type MetricsTotals struct {
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Requests         int64   `json:"requests"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// UserUsage represents usage for a single user/session.
type UserUsage struct {
	SessionID string  `json:"session_id"`
	Tokens    int64   `json:"tokens"`
	Requests  int64   `json:"requests"`
	CostUSD   float64 `json:"cost_usd"`
}

// HealthCheckResult represents the result of a health check.
type HealthCheckResult struct {
	Component string `json:"component"`
	Status    string `json:"status"` // "PASS", "FAIL", "WARN"
	Message   string `json:"message"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}

// ExecQueueItem represents a pending execution approval.
type ExecQueueItem struct {
	ID          string    `json:"id"`
	Tool        string    `json:"tool"`
	Caller      string    `json:"caller"`
	SessionID   string    `json:"session_id"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// Package copilot – lanes.go implements a lane-based concurrency system.
// Each lane has its own queue and concurrency limit, preventing contention
// between different types of work (sessions, cron, subagents).
//
// Lane types:
//   - session:{id}  — one lane per session, maxConcurrent=1 (serialized)
//   - global         — shared lane for cross-session work, maxConcurrent=3
//   - cron           — scheduled jobs, maxConcurrent=2
//   - subagent       — subagent runs, maxConcurrent=8
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

// LaneConfig configures default concurrency limits per lane type.
type LaneConfig struct {
	SessionMax   int `yaml:"session_max"`   // Default: 1 (one agent run per session)
	GlobalMax    int `yaml:"global_max"`    // Default: 3
	CronMax      int `yaml:"cron_max"`      // Default: 2
	SubagentMax  int `yaml:"subagent_max"`  // Default: 8
}

// DefaultLaneConfig returns sensible defaults.
func DefaultLaneConfig() LaneConfig {
	return LaneConfig{
		SessionMax:  1,
		GlobalMax:   3,
		CronMax:     2,
		SubagentMax: 8,
	}
}

// LaneTask represents a unit of work to be executed in a lane.
type LaneTask struct {
	ID       string
	Fn       func(ctx context.Context) error
	Priority int // Lower = higher priority (0 is highest)
}

// Lane manages a queue with bounded concurrency.
type Lane struct {
	Name          string
	MaxConcurrent int

	mu      sync.Mutex
	queue   []LaneTask
	active  atomic.Int32
	closed  bool
	notify  chan struct{} // Signals that a slot is available.
}

// NewLane creates a lane with the given concurrency limit.
func NewLane(name string, maxConcurrent int) *Lane {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Lane{
		Name:          name,
		MaxConcurrent: maxConcurrent,
		notify:        make(chan struct{}, maxConcurrent),
	}
}

// Enqueue adds a task to the lane. If a slot is available, the task starts
// immediately. Otherwise, it's queued and will run when a slot opens.
func (l *Lane) Enqueue(ctx context.Context, task LaneTask) error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return fmt.Errorf("lane %s is closed", l.Name)
	}

	// Try to run immediately if there's capacity.
	if int(l.active.Load()) < l.MaxConcurrent {
		l.active.Add(1)
		l.mu.Unlock()
		go l.runTask(ctx, task)
		return nil
	}

	// Queue the task.
	l.queue = append(l.queue, task)
	l.mu.Unlock()
	return nil
}

// QueueLen returns the number of tasks waiting in the queue.
func (l *Lane) QueueLen() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.queue)
}

// ActiveCount returns the number of currently running tasks.
func (l *Lane) ActiveCount() int {
	return int(l.active.Load())
}

// IsBusy returns true if the lane is at capacity.
func (l *Lane) IsBusy() bool {
	return int(l.active.Load()) >= l.MaxConcurrent
}

// Close prevents new tasks from being enqueued. Active tasks finish normally.
func (l *Lane) Close() {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
}

// runTask executes a task and then pumps the queue for the next one.
func (l *Lane) runTask(ctx context.Context, task LaneTask) {
	defer func() {
		l.active.Add(-1)
		l.pump(ctx)
	}()

	_ = task.Fn(ctx)
}

// pump drains the queue: starts the next task if there's capacity.
func (l *Lane) pump(ctx context.Context) {
	l.mu.Lock()
	if len(l.queue) == 0 || int(l.active.Load()) >= l.MaxConcurrent {
		l.mu.Unlock()
		return
	}

	// Pop the highest-priority task (lowest Priority value).
	bestIdx := 0
	for i, t := range l.queue {
		if t.Priority < l.queue[bestIdx].Priority {
			bestIdx = i
		}
	}
	task := l.queue[bestIdx]
	l.queue = append(l.queue[:bestIdx], l.queue[bestIdx+1:]...)
	l.active.Add(1)
	l.mu.Unlock()

	go l.runTask(ctx, task)
}

// LaneManager manages multiple lanes, creating them on demand.
type LaneManager struct {
	lanes  sync.Map // laneName → *Lane
	config LaneConfig
	logger *slog.Logger
}

// NewLaneManager creates a lane manager with the given configuration.
func NewLaneManager(config LaneConfig, logger *slog.Logger) *LaneManager {
	if config.SessionMax <= 0 {
		config.SessionMax = 1
	}
	if config.GlobalMax <= 0 {
		config.GlobalMax = 3
	}
	if config.CronMax <= 0 {
		config.CronMax = 2
	}
	if config.SubagentMax <= 0 {
		config.SubagentMax = 8
	}
	return &LaneManager{
		config: config,
		logger: logger,
	}
}

// GetLane returns an existing lane or creates a new one with the appropriate
// concurrency limit based on the lane name prefix.
func (lm *LaneManager) GetLane(name string) *Lane {
	if v, ok := lm.lanes.Load(name); ok {
		return v.(*Lane)
	}

	maxConcurrent := lm.resolveMax(name)
	lane := NewLane(name, maxConcurrent)
	actual, _ := lm.lanes.LoadOrStore(name, lane)
	return actual.(*Lane)
}

// SessionLane returns the lane for a specific session.
func (lm *LaneManager) SessionLane(sessionID string) *Lane {
	return lm.GetLane("session:" + sessionID)
}

// GlobalLane returns the shared global lane.
func (lm *LaneManager) GlobalLane() *Lane {
	return lm.GetLane("global")
}

// CronLane returns the lane for scheduled jobs.
func (lm *LaneManager) CronLane() *Lane {
	return lm.GetLane("cron")
}

// SubagentLane returns the lane for subagent runs.
func (lm *LaneManager) SubagentLane() *Lane {
	return lm.GetLane("subagent")
}

// resolveMax determines the concurrency limit based on lane name prefix.
func (lm *LaneManager) resolveMax(name string) int {
	switch {
	case len(name) > 8 && name[:8] == "session:":
		return lm.config.SessionMax
	case name == "global":
		return lm.config.GlobalMax
	case name == "cron":
		return lm.config.CronMax
	case name == "subagent":
		return lm.config.SubagentMax
	default:
		return 1
	}
}

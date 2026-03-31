// Package copilot – coordinator.go implements a structured multi-agent
// coordination protocol with four phases:
//
//	Research     → parallel read-only workers gather information
//	Synthesis    → coordinator analyzes findings, creates specs
//	Implementation → workers execute specs (write operations)
//	Verification → workers test and validate changes
//
// Each phase restricts worker tools to prevent unintended side effects
// (e.g., research workers cannot write files). This ensures safe parallel
// execution and clear separation of concerns.
//
// Aligned with Claude Code's coordinator protocol pattern for multi-agent tasks.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// CoordinatorPhase represents a stage in the multi-agent workflow.
type CoordinatorPhase string

const (
	PhaseResearch       CoordinatorPhase = "research"
	PhaseSynthesis      CoordinatorPhase = "synthesis"
	PhaseImplementation CoordinatorPhase = "implementation"
	PhaseVerification   CoordinatorPhase = "verification"
)

// PhaseOrder returns the standard execution order of phases.
func PhaseOrder() []CoordinatorPhase {
	return []CoordinatorPhase{
		PhaseResearch,
		PhaseSynthesis,
		PhaseImplementation,
		PhaseVerification,
	}
}

// WorkerTask describes a unit of work for a subagent within a phase.
type WorkerTask struct {
	// ID is a unique identifier for this task.
	ID string

	// Label is a human-readable description.
	Label string

	// Prompt is the task instruction for the worker.
	Prompt string

	// Phase determines the tool restrictions applied.
	Phase CoordinatorPhase

	// Timeout is the maximum duration for this task. Zero uses the default.
	Timeout time.Duration
}

// WorkerResult holds the outcome of a single worker task.
type WorkerResult struct {
	TaskID   string
	Label    string
	Phase    CoordinatorPhase
	Result   string
	Error    error
	Duration time.Duration
}

// CoordinatorConfig configures the multi-agent coordinator.
type CoordinatorConfig struct {
	// MaxResearchWorkers is the max parallel workers in research phase. Default: 4.
	MaxResearchWorkers int `yaml:"max_research_workers"`

	// MaxImplWorkers is the max parallel workers in implementation phase. Default: 2.
	MaxImplWorkers int `yaml:"max_impl_workers"`

	// MaxVerifyWorkers is the max parallel workers in verification phase. Default: 2.
	MaxVerifyWorkers int `yaml:"max_verify_workers"`

	// DefaultTimeout is the default timeout per worker task. Default: 5min.
	DefaultTimeout time.Duration `yaml:"default_timeout"`

	// ResearchTools lists tools allowed during research (read-only).
	ResearchTools []string `yaml:"research_tools"`

	// ImplTools lists tools allowed during implementation.
	ImplTools []string `yaml:"impl_tools"`

	// VerifyTools lists tools allowed during verification.
	VerifyTools []string `yaml:"verify_tools"`
}

// DefaultCoordinatorConfig returns sensible defaults.
func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		MaxResearchWorkers: 4,
		MaxImplWorkers:     2,
		MaxVerifyWorkers:   2,
		DefaultTimeout:     5 * time.Minute,
		ResearchTools: []string{
			"read_file", "grep", "find", "ls", "glob",
			"git_log", "git_status", "git_diff", "git_show", "git_blame",
			"web_search", "web_fetch",
			"memory_search", "memory_list",
			"docker_ps", "docker_images", "docker_logs",
			"capabilities",
		},
		ImplTools: []string{
			"read_file", "write_file", "edit_file", "apply_patch",
			"bash", "grep", "find", "ls",
			"git_log", "git_status", "git_diff",
		},
		VerifyTools: []string{
			"read_file", "grep", "find", "ls",
			"bash", // For running tests.
			"git_status", "git_diff",
		},
	}
}

// PhaseResult holds the outcome of an entire phase.
type PhaseResult struct {
	Phase    CoordinatorPhase
	Results  []WorkerResult
	Duration time.Duration
}

// Coordinator orchestrates multi-agent workflows through structured phases.
type Coordinator struct {
	config CoordinatorConfig
	logger *slog.Logger

	// workerSpawner is called to execute a single worker task.
	// It receives the task and allowed tools, returns the result string or error.
	// This abstraction decouples the coordinator from the subagent system,
	// making it testable and flexible.
	workerSpawner WorkerSpawner
}

// WorkerSpawner is the function signature for spawning a worker.
// The coordinator calls this for each task, passing the allowed tools.
// The implementation should create a subagent with only the specified tools.
type WorkerSpawner func(ctx context.Context, task WorkerTask, allowedTools []string) (string, error)

// NewCoordinator creates a new coordinator with the given config and spawner.
func NewCoordinator(config CoordinatorConfig, spawner WorkerSpawner, logger *slog.Logger) *Coordinator {
	return &Coordinator{
		config:        config,
		workerSpawner: spawner,
		logger:        logger.With("component", "coordinator"),
	}
}

// GetPhaseTools returns the allowed tools for a given phase.
func (c *Coordinator) GetPhaseTools(phase CoordinatorPhase) []string {
	switch phase {
	case PhaseResearch:
		return c.config.ResearchTools
	case PhaseSynthesis:
		return nil // Coordinator handles synthesis itself — no worker tools.
	case PhaseImplementation:
		return c.config.ImplTools
	case PhaseVerification:
		return c.config.VerifyTools
	default:
		return nil
	}
}

// MaxWorkersForPhase returns the concurrency limit for a phase.
func (c *Coordinator) MaxWorkersForPhase(phase CoordinatorPhase) int {
	switch phase {
	case PhaseResearch:
		return c.config.MaxResearchWorkers
	case PhaseImplementation:
		return c.config.MaxImplWorkers
	case PhaseVerification:
		return c.config.MaxVerifyWorkers
	default:
		return 1
	}
}

// RunPhase executes all tasks in a phase, respecting concurrency limits.
// Tasks within a phase run in parallel up to the phase's worker limit.
// Results are returned in the same order as the input tasks.
func (c *Coordinator) RunPhase(ctx context.Context, phase CoordinatorPhase, tasks []WorkerTask) PhaseResult {
	start := time.Now()
	result := PhaseResult{
		Phase:   phase,
		Results: make([]WorkerResult, len(tasks)),
	}

	if len(tasks) == 0 {
		result.Duration = time.Since(start)
		return result
	}

	allowedTools := c.GetPhaseTools(phase)
	maxWorkers := c.MaxWorkersForPhase(phase)
	if maxWorkers <= 0 {
		maxWorkers = 1
	}

	c.logger.Info("starting phase",
		"phase", phase,
		"tasks", len(tasks),
		"max_workers", maxWorkers,
		"tools", len(allowedTools),
	)

	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for i, task := range tasks {
		task.Phase = phase // Ensure phase is set.
		if task.Timeout == 0 {
			task.Timeout = c.config.DefaultTimeout
		}

		wg.Add(1)
		go func(idx int, t WorkerTask) {
			defer wg.Done()

			// Acquire semaphore.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				result.Results[idx] = WorkerResult{
					TaskID: t.ID,
					Label:  t.Label,
					Phase:  phase,
					Error:  ctx.Err(),
				}
				return
			}

			taskStart := time.Now()
			taskCtx, cancel := context.WithTimeout(ctx, t.Timeout)
			defer cancel()

			output, err := c.workerSpawner(taskCtx, t, allowedTools)

			result.Results[idx] = WorkerResult{
				TaskID:   t.ID,
				Label:    t.Label,
				Phase:    phase,
				Result:   output,
				Error:    err,
				Duration: time.Since(taskStart),
			}

			if err != nil {
				c.logger.Warn("worker task failed",
					"phase", phase,
					"task", t.Label,
					"error", err,
					"duration_ms", time.Since(taskStart).Milliseconds(),
				)
			} else {
				c.logger.Info("worker task completed",
					"phase", phase,
					"task", t.Label,
					"result_len", len(output),
					"duration_ms", time.Since(taskStart).Milliseconds(),
				)
			}
		}(i, task)
	}

	wg.Wait()
	result.Duration = time.Since(start)

	c.logger.Info("phase completed",
		"phase", phase,
		"tasks", len(tasks),
		"duration_ms", result.Duration.Milliseconds(),
	)

	return result
}

// SynthesizeFindings combines research results into a summary for the coordinator.
func SynthesizeFindings(results []WorkerResult) string {
	var sb fmt.Stringer = &synthBuilder{}
	b := sb.(*synthBuilder)

	b.WriteString("## Research Findings\n\n")
	for i, r := range results {
		b.WriteString(fmt.Sprintf("### %d. %s\n", i+1, r.Label))
		if r.Error != nil {
			b.WriteString(fmt.Sprintf("**Error:** %v\n\n", r.Error))
		} else {
			// Truncate long results.
			content := r.Result
			if len(content) > 2000 {
				content = content[:2000] + "\n... (truncated)"
			}
			b.WriteString(content)
			b.WriteString("\n\n")
		}
	}

	return b.String()
}

// synthBuilder is a simple string builder that implements fmt.Stringer.
type synthBuilder struct {
	data []byte
}

func (b *synthBuilder) WriteString(s string) {
	b.data = append(b.data, s...)
}

func (b *synthBuilder) String() string {
	return string(b.data)
}

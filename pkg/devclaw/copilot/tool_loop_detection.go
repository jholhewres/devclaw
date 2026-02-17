// Package copilot – tool_loop_detection.go detects when the agent enters a
// tool call loop (repeating the same call with no progress) and triggers
// circuit breakers to prevent infinite loops.
//
// Three detectors:
//   - Generic repeat: same tool+args hash repeated N times
//   - Ping-pong: alternating between two tool calls
//   - Known poll: tools that poll external state without progress
package copilot

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// ToolLoopConfig configures tool loop detection thresholds.
type ToolLoopConfig struct {
	// Enabled turns loop detection on (default: true).
	Enabled bool `yaml:"enabled"`

	// HistorySize is how many recent tool calls to track (default: 30).
	HistorySize int `yaml:"history_size"`

	// WarningThreshold triggers a warning injected into the conversation (default: 8).
	WarningThreshold int `yaml:"warning_threshold"`

	// CriticalThreshold triggers a strong nudge to stop (default: 15).
	CriticalThreshold int `yaml:"critical_threshold"`

	// CircuitBreakerThreshold force-stops the agent run (default: 25).
	CircuitBreakerThreshold int `yaml:"circuit_breaker_threshold"`
}

// DefaultToolLoopConfig returns sensible defaults.
func DefaultToolLoopConfig() ToolLoopConfig {
	return ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        8,
		CriticalThreshold:       15,
		CircuitBreakerThreshold: 25,
	}
}

// LoopSeverity represents the level of loop detection.
type LoopSeverity int

const (
	LoopNone     LoopSeverity = iota
	LoopWarning               // Agent should be nudged
	LoopCritical              // Agent should be strongly nudged
	LoopBreaker               // Agent run should be terminated
)

// LoopDetectionResult is the outcome of a loop check.
type LoopDetectionResult struct {
	Severity LoopSeverity
	Message  string // Injected into the conversation as a system hint
	Streak   int    // Number of consecutive repeats detected
	Pattern  string // "repeat", "ping-pong", or ""
}

// toolCallEntry records a single tool call in the history ring buffer.
type toolCallEntry struct {
	hash string
	name string
}

// ToolLoopDetector tracks tool call history and detects loops.
type ToolLoopDetector struct {
	config  ToolLoopConfig
	history []toolCallEntry
	logger  *slog.Logger
}

// NewToolLoopDetector creates a new detector with the given config.
func NewToolLoopDetector(cfg ToolLoopConfig, logger *slog.Logger) *ToolLoopDetector {
	if cfg.HistorySize <= 0 {
		cfg.HistorySize = 30
	}
	if cfg.WarningThreshold <= 0 {
		cfg.WarningThreshold = 8
	}
	if cfg.CriticalThreshold <= 0 {
		cfg.CriticalThreshold = 15
	}
	if cfg.CircuitBreakerThreshold <= 0 {
		cfg.CircuitBreakerThreshold = 25
	}
	// Ensure thresholds are ordered.
	if cfg.CriticalThreshold <= cfg.WarningThreshold {
		cfg.CriticalThreshold = cfg.WarningThreshold + 1
	}
	if cfg.CircuitBreakerThreshold <= cfg.CriticalThreshold {
		cfg.CircuitBreakerThreshold = cfg.CriticalThreshold + 1
	}

	return &ToolLoopDetector{
		config:  cfg,
		history: make([]toolCallEntry, 0, cfg.HistorySize),
		logger:  logger,
	}
}

// RecordAndCheck records a tool call and checks for loops.
// Returns a result indicating the severity (if any).
func (d *ToolLoopDetector) RecordAndCheck(toolName string, args map[string]any) LoopDetectionResult {
	if !d.config.Enabled {
		return LoopDetectionResult{Severity: LoopNone}
	}

	hash := hashToolCall(toolName, args)
	entry := toolCallEntry{hash: hash, name: toolName}

	// Append to history (ring buffer).
	d.history = append(d.history, entry)
	if len(d.history) > d.config.HistorySize {
		d.history = d.history[len(d.history)-d.config.HistorySize:]
	}

	// Check for patterns.
	repeatStreak := d.getRepeatStreak(hash)
	pingPongStreak := d.getPingPongStreak(hash)

	// Use the worst streak.
	streak := repeatStreak
	pattern := "repeat"
	if pingPongStreak > streak {
		streak = pingPongStreak
		pattern = "ping-pong"
	}

	if streak >= d.config.CircuitBreakerThreshold {
		d.logger.Error("tool loop circuit breaker triggered",
			"tool", toolName, "streak", streak, "pattern", pattern)
		return LoopDetectionResult{
			Severity: LoopBreaker,
			Message: fmt.Sprintf(
				"CIRCUIT BREAKER: You have called '%s' %d times with the same arguments and no progress. "+
					"This run is being terminated. The approach is not working — you need a fundamentally different strategy.",
				toolName, streak),
			Streak:  streak,
			Pattern: pattern,
		}
	}

	if streak >= d.config.CriticalThreshold {
		d.logger.Warn("tool loop critical threshold reached",
			"tool", toolName, "streak", streak, "pattern", pattern)
		return LoopDetectionResult{
			Severity: LoopCritical,
			Message: fmt.Sprintf(
				"CRITICAL: You have repeated '%s' %d times with no progress. STOP this approach immediately. "+
					"Explain to the user what you tried and ask for guidance. Do NOT call this tool again with the same arguments.",
				toolName, streak),
			Streak:  streak,
			Pattern: pattern,
		}
	}

	if streak >= d.config.WarningThreshold {
		d.logger.Warn("tool loop warning threshold reached",
			"tool", toolName, "streak", streak, "pattern", pattern)
		return LoopDetectionResult{
			Severity: LoopWarning,
			Message: fmt.Sprintf(
				"WARNING: You have called '%s' %d times with similar arguments. This may indicate a loop. "+
					"Consider a different approach or ask the user for help.",
				toolName, streak),
			Streak:  streak,
			Pattern: pattern,
		}
	}

	return LoopDetectionResult{Severity: LoopNone}
}

// Reset clears the history (e.g. for a new run).
func (d *ToolLoopDetector) Reset() {
	d.history = d.history[:0]
}

// getRepeatStreak counts consecutive identical tool calls from the end.
func (d *ToolLoopDetector) getRepeatStreak(currentHash string) int {
	streak := 0
	for i := len(d.history) - 1; i >= 0; i-- {
		if d.history[i].hash == currentHash {
			streak++
		} else {
			break
		}
	}
	return streak
}

// getPingPongStreak detects alternating A-B-A-B patterns.
func (d *ToolLoopDetector) getPingPongStreak(currentHash string) int {
	if len(d.history) < 3 {
		return 0
	}

	// The current call is already appended, so check the pattern from the end.
	// Pattern: ...A, B, A, B, A (current = A)
	otherHash := ""
	streak := 1

	for i := len(d.history) - 2; i >= 0; i-- {
		h := d.history[i].hash
		if streak == 1 {
			// First step back: should be different from current.
			if h == currentHash {
				return 0
			}
			otherHash = h
			streak++
		} else if streak%2 == 0 {
			// Even positions: should match current.
			if h != currentHash {
				break
			}
			streak++
		} else {
			// Odd positions: should match other.
			if h != otherHash {
				break
			}
			streak++
		}
	}

	// Ping-pong streak is the number of pairs (each pair = 2 calls).
	return streak / 2
}

// hashToolCall creates a stable hash of tool name + args for comparison.
func hashToolCall(name string, args map[string]any) string {
	// Normalize: sort keys, marshal to JSON.
	data, err := json.Marshal(args)
	if err != nil {
		data = []byte(fmt.Sprintf("%v", args))
	}

	// For bash commands, also normalize whitespace.
	key := name + ":" + string(data)
	if name == "bash" || name == "ssh" {
		key = strings.Join(strings.Fields(key), " ")
	}

	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h[:8])
}

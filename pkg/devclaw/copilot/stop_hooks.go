// Package copilot – stop_hooks.go implements pre-stop completion verification.
// Before the agent stops, these hooks analyze the conversation to detect
// incomplete work patterns (e.g., files edited but tests not run, code written
// but not compiled). If incomplete work is detected, the hook can inject a
// continuation message to prompt the agent to finish.
//
// Aligned with Claude Code's stop hooks pattern: verify completion before
// allowing the agent to stop, preventing premature task completion.
package copilot

import (
	"fmt"
	"strings"
)

// CompletionCheck represents a single verification pattern.
type CompletionCheck struct {
	// Name identifies this check for logging.
	Name string

	// TriggerPatterns are substrings in the conversation that indicate
	// this type of work was started.
	TriggerPatterns []string

	// CompletionPatterns are substrings that indicate the work was completed.
	// If triggers match but completion doesn't, the check fails.
	CompletionPatterns []string

	// Reminder is the message to inject if the check fails.
	Reminder string
}

// StopHookVerifier checks conversation history for incomplete work.
type StopHookVerifier struct {
	checks []CompletionCheck
}

// NewStopHookVerifier creates a verifier with default completion checks.
func NewStopHookVerifier() *StopHookVerifier {
	return &StopHookVerifier{
		checks: defaultCompletionChecks(),
	}
}

// defaultCompletionChecks returns the built-in verification patterns.
func defaultCompletionChecks() []CompletionCheck {
	return []CompletionCheck{
		{
			Name: "tests_after_code_changes",
			TriggerPatterns: []string{
				"write_file", "edit_file", "apply_patch",
				"File edited successfully", "File written",
				"File created", "Patch applied",
			},
			CompletionPatterns: []string{
				"go test", "npm test", "pytest", "jest",
				"make test", "cargo test", "mvn test",
				"PASS", "passed", "tests passed",
				"test suite", "All tests",
			},
			Reminder: "You modified code but didn't run tests. Consider running the test suite to verify your changes work correctly.",
		},
		{
			Name: "build_after_code_changes",
			TriggerPatterns: []string{
				"write_file", "edit_file", "apply_patch",
				"File edited successfully", "File written",
				"File created", "Patch applied",
			},
			CompletionPatterns: []string{
				"go build", "make build", "npm run build",
				"cargo build", "mvn package", "tsc",
				"compiled", "build succeeded", "Build complete",
			},
			Reminder: "You modified source files but didn't verify the build compiles. Consider running a build check.",
		},
		{
			Name: "lint_after_code_changes",
			TriggerPatterns: []string{
				"write_file", "edit_file",
				"File edited successfully", "File written",
			},
			CompletionPatterns: []string{
				"lint", "golangci-lint", "eslint", "pylint",
				"flake8", "clippy", "make lint",
				"no issues", "0 errors",
			},
			Reminder: "Consider running the linter to catch style issues in your changes.",
		},
	}
}

// IncompleteWork represents a detected incomplete work pattern.
type IncompleteWork struct {
	// CheckName identifies which check failed.
	CheckName string

	// Reminder is the suggested action.
	Reminder string
}

// VerifyCompletion analyzes the conversation for incomplete work patterns.
// Returns a list of incomplete work items, empty if everything looks complete.
//
// The analysis works by scanning assistant and tool messages for trigger patterns
// (indicating work was done) and then checking if completion patterns also appear
// (indicating the work was verified). Only the most recent portion of the
// conversation is checked to avoid false positives from old, already-resolved work.
func (v *StopHookVerifier) VerifyCompletion(messages []chatMessage) []IncompleteWork {
	if len(messages) < 4 {
		return nil
	}

	// Only analyze the recent portion of the conversation.
	// Older work is assumed to be complete or no longer relevant.
	startIdx := len(messages) - 40
	if startIdx < 0 {
		startIdx = 0
	}
	recent := messages[startIdx:]

	// Build a combined text corpus from recent messages for pattern matching.
	var toolContent, assistantContent strings.Builder
	for _, m := range recent {
		content, ok := m.Content.(string)
		if !ok {
			continue
		}
		switch m.Role {
		case "tool":
			toolContent.WriteString(content)
			toolContent.WriteString("\n")
		case "assistant":
			assistantContent.WriteString(content)
			assistantContent.WriteString("\n")
		}
	}

	allContent := toolContent.String() + assistantContent.String()
	toolText := toolContent.String()

	var incomplete []IncompleteWork
	for _, check := range v.checks {
		triggered := false
		for _, pattern := range check.TriggerPatterns {
			if strings.Contains(allContent, pattern) {
				triggered = true
				break
			}
		}
		if !triggered {
			continue
		}

		completed := false
		for _, pattern := range check.CompletionPatterns {
			if strings.Contains(toolText, pattern) || strings.Contains(allContent, pattern) {
				completed = true
				break
			}
		}
		if !completed {
			incomplete = append(incomplete, IncompleteWork{
				CheckName: check.Name,
				Reminder:  check.Reminder,
			})
		}
	}

	return incomplete
}

// FormatIncompleteWorkMessage formats incomplete work items into a single
// message that can be injected into the conversation to prompt continuation.
func FormatIncompleteWorkMessage(items []IncompleteWork) string {
	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[Pre-stop verification] The following items may need attention:\n\n")
	for i, item := range items {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Reminder))
	}
	sb.WriteString("\nIf these are already handled or not applicable, you can complete your response.")
	return sb.String()
}

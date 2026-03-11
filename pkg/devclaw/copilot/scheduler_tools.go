// Package copilot – scheduler_tools.go implements scheduler tools for managing
// scheduled tasks and reminders: scheduler_add, scheduler_list, scheduler_remove,
// and scheduler_search.
package copilot

import (
	"context"
	"fmt"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/scheduler"
)

// RegisterSchedulerDispatcher registers individual scheduler tools:
// scheduler_add, scheduler_list, scheduler_remove, and scheduler_search.
func RegisterSchedulerDispatcher(executor *ToolExecutor, sched *scheduler.Scheduler, skillDB *SkillDB) {
	// scheduler_add — schedule a new task or reminder.
	executor.RegisterHidden(
		MakeToolDefinition("scheduler_add",
			"Schedule a new task or reminder. Supports natural language schedules ('every 5 minutes', 'daily at 9am'), cron expressions, and one-shot reminders.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "Unique job ID",
					},
					"schedule": map[string]any{
						"type":        "string",
						"description": "Natural language ('every 5 minutes', 'daily at 9am') or cron/interval format",
					},
					"type": map[string]any{
						"type":        "string",
						"description": "'at' = fires ONCE (reminders), 'every' = repeats at interval, 'cron' = cron schedule",
						"enum":        []string{"cron", "every", "at"},
					},
					"command": map[string]any{
						"type":        "string",
						"description": "Prompt/command to execute when job fires",
					},
					"channel": map[string]any{
						"type":        "string",
						"description": "Target channel for response",
					},
					"chat_id": map[string]any{
						"type":        "string",
						"description": "Target chat/group ID",
					},
					"isolate_session": map[string]any{
						"type":        "boolean",
						"description": "Run each execution in its own isolated session (prevents cron output from polluting conversation history)",
					},
					"as_subagent": map[string]any{
						"type":        "boolean",
						"description": "Run as a subagent for full isolation (own session, filtered tools, won't block user agent runs)",
					},
				},
				"required": []string{"id", "schedule", "command"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleSchedulerAdd(ctx, sched, skillDB, args)
		},
	)

	// scheduler_list — list all scheduled jobs.
	executor.RegisterHidden(
		MakeToolDefinition("scheduler_list",
			"List all currently scheduled jobs with their status, schedule, run count, and last execution details.",
			map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleSchedulerList(sched)
		},
	)

	// scheduler_remove — remove a scheduled job by ID.
	executor.RegisterHidden(
		MakeToolDefinition("scheduler_remove",
			"Remove a scheduled job by its ID. Requires explicit confirmation to prevent accidental deletion.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "Job ID to remove",
					},
					"confirm": map[string]any{
						"type":        "boolean",
						"description": "Must be true to confirm removal",
					},
				},
				"required": []string{"id", "confirm"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleSchedulerRemove(sched, skillDB, args)
		},
	)

	// scheduler_search — search reminders by keyword.
	executor.RegisterHidden(
		MakeToolDefinition("scheduler_search",
			"Search reminders and scheduled tasks by keyword. Returns matching reminders with their schedule, command, and status.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query for reminders",
					},
					"include_removed": map[string]any{
						"type":        "boolean",
						"description": "Include removed reminders in search results",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Max results (default: 20)",
					},
				},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleSchedulerSearch(skillDB, args)
		},
	)
}

func handleSchedulerAdd(ctx context.Context, sched *scheduler.Scheduler, skillDB *SkillDB, args map[string]any) (any, error) {
	id, _ := args["id"].(string)
	schedule, _ := args["schedule"].(string)
	jobType, _ := args["type"].(string)
	command, _ := args["command"].(string)
	channel, _ := args["channel"].(string)
	chatID, _ := args["chat_id"].(string)

	if id == "" || schedule == "" || command == "" {
		return nil, fmt.Errorf("id, schedule, and command are required for add action")
	}

	// Try natural language parsing first.
	if parsed, ok := scheduler.ParseNaturalLanguage(schedule); ok {
		schedule = parsed.Schedule
		if jobType == "" {
			jobType = parsed.Type
		}
	}

	if jobType == "" {
		jobType = "cron"
	}

	// Auto-fill channel/chatID from context.
	if channel == "" || chatID == "" {
		dt := DeliveryTargetFromContext(ctx)
		if dt.Channel != "" && channel == "" {
			channel = dt.Channel
		}
		if dt.ChatID != "" && chatID == "" {
			chatID = dt.ChatID
		}
	}

	isolateSession, _ := args["isolate_session"].(bool)
	asSubagent, _ := args["as_subagent"].(bool)

	job := &scheduler.Job{
		ID:             id,
		Schedule:       schedule,
		Type:           jobType,
		Command:        command,
		Channel:        channel,
		ChatID:         chatID,
		Enabled:        true,
		Announce:       true,
		IsolateSession: isolateSession,
		AsSubagent:     asSubagent,
	}

	if err := sched.Add(job); err != nil {
		return nil, fmt.Errorf("scheduling job %q: %w", id, err)
	}

	// Save reminder to tracking table (best-effort).
	if skillDB != nil {
		_ = skillDB.SaveReminder(id, jobType, schedule, command, channel, chatID)
	}

	return fmt.Sprintf("Job '%s' scheduled: %s (%s) → %s:%s", id, schedule, jobType, channel, chatID), nil
}

func handleSchedulerList(sched *scheduler.Scheduler) (any, error) {
	jobs := sched.List()
	if len(jobs) == 0 {
		return "No scheduled jobs.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Scheduled jobs (%d):\n\n", len(jobs))
	for _, j := range jobs {
		status := "enabled"
		if !j.Enabled {
			status = "disabled"
		}
		fmt.Fprintf(&sb, "- **%s** [%s] schedule=%s type=%s\n  Command: %s\n  Runs: %d",
			j.ID, status, j.Schedule, j.Type, j.Command, j.RunCount)
		if j.LastRunAt != nil {
			fmt.Fprintf(&sb, "  Last run: %s", j.LastRunAt.Format("2006-01-02 15:04"))
		}
		if j.LastError != "" {
			fmt.Fprintf(&sb, "  Last error: %s", j.LastError)
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func handleSchedulerRemove(sched *scheduler.Scheduler, skillDB *SkillDB, args map[string]any) (any, error) {
	id, _ := args["id"].(string)
	confirm, _ := args["confirm"].(bool)
	if id == "" {
		return nil, fmt.Errorf("id is required for remove action")
	}
	if !confirm {
		return nil, fmt.Errorf("removal not confirmed. Set confirm=true to remove job '%s'", id)
	}
	if err := sched.Remove(id); err != nil {
		return nil, fmt.Errorf("removing job %q: %w", id, err)
	}

	// Mark reminder as removed in tracking table (best-effort).
	if skillDB != nil {
		_ = skillDB.MarkReminderRemoved(id)
	}

	return fmt.Sprintf("Job '%s' removed.", id), nil
}

func handleSchedulerSearch(skillDB *SkillDB, args map[string]any) (any, error) {
	if skillDB == nil {
		return nil, fmt.Errorf("reminder tracking not available")
	}

	query, _ := args["query"].(string)
	includeRemoved, _ := args["include_removed"].(bool)
	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	reminders, err := skillDB.SearchReminders(query, includeRemoved, limit)
	if err != nil {
		return nil, fmt.Errorf("search reminders: %w", err)
	}

	if len(reminders) == 0 {
		if query != "" {
			return fmt.Sprintf("No reminders found matching '%s'.", query), nil
		}
		return "No reminders found.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Reminders found (%d):\n\n", len(reminders))
	for _, r := range reminders {
		status := r.Status
		fmt.Fprintf(&sb, "- **%s** [%s] type=%s\n  Schedule: %s\n  Command: %s\n  Created: %s\n",
			r.JobID, status, r.JobType, r.Schedule, r.Command, r.CreatedAt)
		if r.RemovedAt != "" {
			fmt.Fprintf(&sb, "  Removed: %s\n", r.RemovedAt)
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

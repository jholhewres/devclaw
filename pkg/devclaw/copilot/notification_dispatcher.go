// Package copilot – notification_dispatcher.go implements the notification
// dispatching system for team agents. It routes notifications to configured
// destinations such as channels, inboxes, webhooks, and activity feeds.
package copilot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

// NotificationDispatcher manages notification routing for team agents.
type NotificationDispatcher struct {
	db         *sql.DB
	teamMgr    *TeamManager
	channelMgr *channels.Manager
	hookMgr    *HookManager
	config     *NotificationConfig
	logger     *slog.Logger

	// rateLimit tracks notification counts per rule for rate limiting.
	rateLimit   map[string]*rateLimitCounter
	rateLimitMu sync.Mutex
}

type rateLimitCounter struct {
	count     int
	resetTime time.Time
}

// NewNotificationDispatcher creates a new notification dispatcher.
func NewNotificationDispatcher(
	db *sql.DB,
	teamMgr *TeamManager,
	channelMgr *channels.Manager,
	hookMgr *HookManager,
	config *NotificationConfig,
	logger *slog.Logger,
) *NotificationDispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	if config == nil {
		config = &NotificationConfig{Enabled: true}
	}

	return &NotificationDispatcher{
		db:         db,
		teamMgr:    teamMgr,
		channelMgr: channelMgr,
		hookMgr:    hookMgr,
		config:     config,
		logger:     logger.With("component", "notification_dispatcher"),
		rateLimit:  make(map[string]*rateLimitCounter),
	}
}

// Dispatch sends a notification to all applicable destinations.
func (nd *NotificationDispatcher) Dispatch(ctx context.Context, notif *TeamNotification) error {
	if !nd.config.Enabled {
		return nil
	}

	// Save notification to history
	if err := nd.saveNotification(ctx, notif); err != nil {
		nd.logger.Warn("failed to save notification", "id", notif.ID, "error", err)
		// Continue anyway to try sending
	}

	// Find applicable rules
	rules := nd.findApplicableRules(notif)
	if len(rules) == 0 {
		// No rules configured - use defaults
		if nd.config.Defaults.ActivityFeed {
			nd.sendToActivity(ctx, notif)
		}
		return nil
	}

	// Dispatch to each rule
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		if !nd.shouldFire(rule, notif) {
			continue
		}

		// Send to each destination
		for _, dest := range rule.Destinations {
			switch dest.Type {
			case DestChannel:
				if err := nd.sendToChannel(ctx, notif, dest); err != nil {
					nd.logger.Warn("failed to send to channel",
						"rule", rule.Name, "channel", dest.Channel, "error", err)
				}
			case DestInbox:
				if err := nd.sendToInbox(ctx, notif, dest); err != nil {
					nd.logger.Warn("failed to send to inbox",
						"rule", rule.Name, "target", dest.Target, "error", err)
				}
			case DestWebhook:
				if err := nd.sendToWebhook(ctx, notif, dest); err != nil {
					nd.logger.Warn("failed to send to webhook",
						"rule", rule.Name, "error", err)
				}
			case DestOwner:
				if err := nd.sendToOwner(ctx, notif, dest); err != nil {
					nd.logger.Warn("failed to send to owner",
						"rule", rule.Name, "error", err)
				}
			case DestActivity:
				nd.sendToActivity(ctx, notif)
			}
		}
	}

	// Fire hook for external observation
	if nd.hookMgr != nil {
		nd.hookMgr.DispatchAsync(HookPayload{
			Event:   HookNotification,
			Message: notif.Message,
			Extra: map[string]any{
				"notification_id":   notif.ID,
				"notification_type": notif.Type,
				"team_id":           notif.TeamID,
				"agent_id":          notif.AgentID,
				"agent_name":        notif.AgentName,
				"task_id":           notif.TaskID,
				"result":            notif.Result,
				"priority":          notif.Priority,
			},
		})
	}

	return nil
}

// DispatchTaskCompletion is a convenience method for task completion notifications.
func (nd *NotificationDispatcher) DispatchTaskCompletion(
	ctx context.Context,
	teamID, agentID, agentName, taskID, taskTitle, message string,
	success bool,
) error {
	notif := &TeamNotification{
		ID:        uuid.New().String()[:8],
		TeamID:    teamID,
		Type:      NotifTaskCompleted,
		AgentID:   agentID,
		AgentName: agentName,
		TaskID:    taskID,
		TaskTitle: taskTitle,
		Action:    "task_completed",
		Message:   message,
		Timestamp: time.Now(),
		Priority:  3,
	}

	if success {
		notif.Result = ResultSuccess
	} else {
		notif.Result = ResultFailure
		notif.Type = NotifTaskFailed
		notif.Action = "task_failed"
		notif.Priority = 2 // Higher priority for failures
	}

	return nd.Dispatch(ctx, notif)
}

// DispatchProgress sends a progress update notification.
func (nd *NotificationDispatcher) DispatchProgress(
	ctx context.Context,
	teamID, agentID, agentName, taskID, taskTitle, message string,
	progress int,
) error {
	notif := &TeamNotification{
		ID:        uuid.New().String()[:8],
		TeamID:    teamID,
		Type:      NotifTaskProgress,
		AgentID:   agentID,
		AgentName: agentName,
		TaskID:    taskID,
		TaskTitle: taskTitle,
		Action:    "task_progress",
		Result:    ResultInfo,
		Message:   message,
		Timestamp: time.Now(),
		Priority:  4,
		Details:   map[string]any{"progress": progress},
	}

	return nd.Dispatch(ctx, notif)
}

// DispatchError sends an error notification.
func (nd *NotificationDispatcher) DispatchError(
	ctx context.Context,
	teamID, agentID, agentName, message string,
	err error,
) error {
	notif := &TeamNotification{
		ID:        uuid.New().String()[:8],
		TeamID:    teamID,
		Type:      NotifAgentError,
		AgentID:   agentID,
		AgentName: agentName,
		Action:    "agent_error",
		Result:    ResultFailure,
		Message:   message,
		Timestamp: time.Now(),
		Priority:  1, // Highest priority
	}

	if err != nil {
		notif.Details = map[string]any{"error": err.Error()}
	}

	return nd.Dispatch(ctx, notif)
}

// saveNotification stores the notification in the database.
func (nd *NotificationDispatcher) saveNotification(ctx context.Context, notif *TeamNotification) error {
	if nd.db == nil {
		return nil
	}

	detailsJSON, _ := json.Marshal(notif.Details)

	_, err := nd.db.ExecContext(ctx, `
		INSERT INTO team_notifications (
			id, team_id, type, agent_id, agent_name,
			task_id, task_title, action, result, message,
			details, priority, timestamp, read
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		notif.ID, notif.TeamID, string(notif.Type), notif.AgentID, notif.AgentName,
		notif.TaskID, notif.TaskTitle, notif.Action, string(notif.Result), notif.Message,
		string(detailsJSON), notif.Priority, notif.Timestamp.Format(time.RFC3339),
	)

	return err
}

// findApplicableRules returns rules that match the notification.
func (nd *NotificationDispatcher) findApplicableRules(notif *TeamNotification) []*NotificationRule {
	var rules []*NotificationRule

	// Add configured rules
	for i := range nd.config.Rules {
		rule := &nd.config.Rules[i]
		if nd.ruleMatches(rule, notif) {
			rules = append(rules, rule)
		}
	}

	return rules
}

// ruleMatches checks if a rule applies to the notification.
func (nd *NotificationDispatcher) ruleMatches(rule *NotificationRule, notif *TeamNotification) bool {
	// Check team ID (empty = global)
	if rule.TeamID != "" && rule.TeamID != notif.TeamID {
		return false
	}

	// Check event type
	eventMatches := false
	for _, eventType := range rule.Events {
		if eventType == notif.Type {
			eventMatches = true
			break
		}
	}
	if !eventMatches {
		return false
	}

	// Check conditions
	if len(rule.Conditions.AgentIDs) > 0 {
		agentMatches := false
		for _, agentID := range rule.Conditions.AgentIDs {
			if agentID == notif.AgentID {
				agentMatches = true
				break
			}
		}
		if !agentMatches {
			return false
		}
	}

	// Check priority threshold
	if rule.Priority > 0 && notif.Priority > rule.Priority {
		return false
	}

	return true
}

// shouldFire checks rate limiting and quiet hours.
func (nd *NotificationDispatcher) shouldFire(rule *NotificationRule, notif *TeamNotification) bool {
	// Check quiet hours
	if rule.QuietHours != nil && rule.QuietHours.Enabled {
		if nd.isQuietHours(rule.QuietHours) {
			// Only allow urgent notifications during quiet hours
			if notif.Priority > 1 {
				nd.logger.Debug("notification suppressed during quiet hours",
					"rule", rule.Name, "priority", notif.Priority)
				return false
			}
		}
	}

	// Check rate limiting
	if rule.RateLimit > 0 {
		if !nd.checkRateLimit(rule.ID, rule.RateLimit) {
			nd.logger.Debug("notification rate limited",
				"rule", rule.Name, "limit", rule.RateLimit)
			return false
		}
	}

	return true
}

// isQuietHours checks if current time falls within quiet hours.
func (nd *NotificationDispatcher) isQuietHours(qh *QuietHoursConfig) bool {
	if qh == nil || !qh.Enabled {
		return false
	}

	// Get current time in configured timezone
	loc, err := time.LoadLocation(qh.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)

	// Check day of week if specified
	if len(qh.Days) > 0 {
		weekday := int(now.Weekday())
		dayMatch := false
		for _, d := range qh.Days {
			if d == weekday {
				dayMatch = true
				break
			}
		}
		if !dayMatch {
			return false
		}
	}

	// Get current time as HH:MM
	currentHM := now.Format("15:04")

	// Check if current time is within quiet hours
	startStr := qh.Start
	endStr := qh.End

	// Handle overnight quiet hours (e.g., 22:00 - 08:00)
	if startStr > endStr {
		// Overnight: quiet if current >= start OR current < end
		return currentHM >= startStr || currentHM < endStr
	}

	// Same-day: quiet if start <= current < end
	return currentHM >= startStr && currentHM < endStr
}

// checkRateLimit checks and updates rate limiting for a rule.
func (nd *NotificationDispatcher) checkRateLimit(ruleID string, limit int) bool {
	now := time.Now()

	nd.rateLimitMu.Lock()
	defer nd.rateLimitMu.Unlock()

	counter, exists := nd.rateLimit[ruleID]

	if !exists || now.After(counter.resetTime) {
		// Reset counter for new hour
		nd.rateLimit[ruleID] = &rateLimitCounter{
			count:     1,
			resetTime: now.Truncate(time.Hour).Add(time.Hour),
		}
		return true
	}

	if counter.count >= limit {
		return false
	}

	counter.count++
	return true
}

// sendToChannel sends notification to a communication channel.
func (nd *NotificationDispatcher) sendToChannel(ctx context.Context, notif *TeamNotification, dest NotificationDestination) error {
	if nd.channelMgr == nil {
		return fmt.Errorf("channel manager not available")
	}

	if dest.Channel == "" || dest.ChatID == "" {
		return fmt.Errorf("channel destination missing channel or chat_id")
	}

	msg := nd.formatMessage(notif, dest.Format)

	return nd.channelMgr.Send(ctx, dest.Channel, dest.ChatID, &channels.OutgoingMessage{
		Content: msg,
	})
}

// sendToInbox adds notification to agent's pending messages.
func (nd *NotificationDispatcher) sendToInbox(ctx context.Context, notif *TeamNotification, dest NotificationDestination) error {
	if nd.db == nil {
		return fmt.Errorf("database not available")
	}

	targetAgent := dest.Target
	if targetAgent == "" {
		// Default to the agent that created the notification
		targetAgent = notif.AgentID
	}

	msgID := uuid.New().String()[:8]
	content := nd.formatMessage(notif, "text")

	_, err := nd.db.ExecContext(ctx, `
		INSERT INTO team_pending_messages (id, to_agent, from_agent, content, thread_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		msgID, targetAgent, notif.AgentID, content, "", time.Now().Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("insert pending message: %w", err)
	}

	nd.logger.Debug("notification added to inbox",
		"id", msgID, "to", targetAgent, "from", notif.AgentID)

	return nil
}

// sendToWebhook sends notification to an external webhook.
func (nd *NotificationDispatcher) sendToWebhook(ctx context.Context, notif *TeamNotification, dest NotificationDestination) error {
	// Webhooks are handled by the hook manager
	// This is a placeholder for direct webhook calls
	// In practice, use hookMgr.Dispatch with HookNotification
	return nil
}

// sendToOwner sends notification to the team owner.
func (nd *NotificationDispatcher) sendToOwner(ctx context.Context, notif *TeamNotification, dest NotificationDestination) error {
	if nd.teamMgr == nil {
		return fmt.Errorf("team manager not available")
	}

	team, err := nd.teamMgr.GetTeam(notif.TeamID)
	if err != nil {
		return fmt.Errorf("get team: %w", err)
	}

	if team == nil || team.OwnerJID == "" {
		return fmt.Errorf("team has no owner")
	}

	// Send to owner's inbox
	ownerDest := NotificationDestination{
		Type:   DestInbox,
		Target: team.OwnerJID,
		Format: dest.Format,
	}

	return nd.sendToInbox(ctx, notif, ownerDest)
}

// sendToActivity logs notification to team activity feed.
func (nd *NotificationDispatcher) sendToActivity(ctx context.Context, notif *TeamNotification) {
	if nd.db == nil {
		return
	}

	activityID := uuid.New().String()[:8]
	activityType := ActivityNotification

	_, err := nd.db.ExecContext(ctx, `
		INSERT INTO team_activities (id, team_id, type, agent_id, message, related_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		activityID, notif.TeamID, string(activityType), notif.AgentID,
		notif.Message, notif.TaskID, time.Now().Format(time.RFC3339),
	)

	if err != nil {
		nd.logger.Warn("failed to log activity", "error", err)
	}
}

// formatMessage formats the notification message for the destination.
func (nd *NotificationDispatcher) formatMessage(notif *TeamNotification, format string) string {
	// Default format
	if format == "" {
		format = "text"
	}

	// Try custom template first (not implemented yet - would need rule context)

	// Default formatting based on result type
	var emoji string
	switch notif.Result {
	case ResultSuccess:
		emoji = "✅"
	case ResultFailure:
		emoji = "❌"
	case ResultWarning:
		emoji = "⚠️"
	default:
		emoji = "ℹ️"
	}

	var sb strings.Builder

	if format == "markdown" {
		sb.WriteString(fmt.Sprintf("%s **%s**\n\n", emoji, notif.AgentName))
		sb.WriteString(fmt.Sprintf("**Action:** %s\n", notif.Action))
		if notif.TaskTitle != "" {
			sb.WriteString(fmt.Sprintf("**Task:** %s\n", notif.TaskTitle))
		}
		sb.WriteString(fmt.Sprintf("\n%s\n", notif.Message))
	} else {
		sb.WriteString(fmt.Sprintf("%s %s\n", emoji, notif.AgentName))
		sb.WriteString(fmt.Sprintf("Action: %s\n", notif.Action))
		if notif.TaskTitle != "" {
			sb.WriteString(fmt.Sprintf("Task: %s\n", notif.TaskTitle))
		}
		sb.WriteString(fmt.Sprintf("\n%s\n", notif.Message))
	}

	return sb.String()
}

// parseTemplate parses and executes a Go template for message formatting.
func (nd *NotificationDispatcher) parseTemplate(tmplStr string, notif *TeamNotification) (string, error) {
	tmpl, err := template.New("notification").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var sb strings.Builder
	if err := tmpl.Execute(&sb, notif); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return sb.String(), nil
}

// GetNotifications retrieves notifications for a team.
func (nd *NotificationDispatcher) GetNotifications(ctx context.Context, teamID string, limit int) ([]*TeamNotification, error) {
	if nd.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	if limit <= 0 {
		limit = 50
	}

	rows, err := nd.db.QueryContext(ctx, `
		SELECT id, type, agent_id, agent_name, task_id, task_title,
		       action, result, message, details, priority, timestamp, read
		FROM team_notifications
		WHERE team_id = ?
		ORDER BY timestamp DESC
		LIMIT ?`,
		teamID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*TeamNotification
	for rows.Next() {
		notif := &TeamNotification{TeamID: teamID}
		var detailsJSON string
		var timestampStr string

		err := rows.Scan(
			&notif.ID, &notif.Type, &notif.AgentID, &notif.AgentName,
			&notif.TaskID, &notif.TaskTitle, &notif.Action, &notif.Result,
			&notif.Message, &detailsJSON, &notif.Priority, &timestampStr, &notif.Read,
		)
		if err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}

		notif.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
		if detailsJSON != "" {
			_ = json.Unmarshal([]byte(detailsJSON), &notif.Details)
		}

		notifications = append(notifications, notif)
	}

	return notifications, nil
}

// GetUnreadNotifications retrieves unread notifications for a team.
func (nd *NotificationDispatcher) GetUnreadNotifications(ctx context.Context, teamID string) ([]*TeamNotification, error) {
	if nd.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	rows, err := nd.db.QueryContext(ctx, `
		SELECT id, type, agent_id, agent_name, task_id, task_title,
		       action, result, message, details, priority, timestamp
		FROM team_notifications
		WHERE team_id = ? AND read = 0
		ORDER BY timestamp DESC`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("query unread notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*TeamNotification
	for rows.Next() {
		notif := &TeamNotification{TeamID: teamID}
		var detailsJSON string
		var timestampStr string

		err := rows.Scan(
			&notif.ID, &notif.Type, &notif.AgentID, &notif.AgentName,
			&notif.TaskID, &notif.TaskTitle, &notif.Action, &notif.Result,
			&notif.Message, &detailsJSON, &notif.Priority, &timestampStr,
		)
		if err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}

		notif.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
		if detailsJSON != "" {
			_ = json.Unmarshal([]byte(detailsJSON), &notif.Details)
		}

		notifications = append(notifications, notif)
	}

	return notifications, nil
}

// MarkNotificationRead marks a notification as read.
func (nd *NotificationDispatcher) MarkNotificationRead(ctx context.Context, notificationID string) error {
	if nd.db == nil {
		return fmt.Errorf("database not available")
	}

	now := time.Now().Format(time.RFC3339)
	_, err := nd.db.ExecContext(ctx, `
		UPDATE team_notifications
		SET read = 1, read_at = ?
		WHERE id = ?`,
		now, notificationID,
	)

	return err
}

// SetConfig updates the notification configuration.
func (nd *NotificationDispatcher) SetConfig(config *NotificationConfig) {
	if config != nil {
		nd.config = config
	}
}

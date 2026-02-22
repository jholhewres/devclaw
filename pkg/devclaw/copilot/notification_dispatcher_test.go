// Package copilot – notification_dispatcher_test.go tests the notification dispatcher.
package copilot

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// testNotificationDB creates an in-memory SQLite database for notification testing.
func testNotificationDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS teams (
		id            TEXT PRIMARY KEY,
		name          TEXT NOT NULL,
		description   TEXT DEFAULT '',
		owner_jid     TEXT NOT NULL,
		default_model TEXT DEFAULT '',
		workspace_path TEXT DEFAULT '',
		created_at    TEXT NOT NULL,
		enabled       INTEGER DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS team_notifications (
		id         TEXT PRIMARY KEY,
		team_id    TEXT NOT NULL,
		type       TEXT NOT NULL,
		agent_id   TEXT NOT NULL,
		agent_name TEXT NOT NULL,
		task_id    TEXT DEFAULT '',
		task_title TEXT DEFAULT '',
		action     TEXT NOT NULL,
		result     TEXT NOT NULL,
		message    TEXT NOT NULL,
		details    TEXT DEFAULT '{}',
		priority   INTEGER DEFAULT 3,
		timestamp  TEXT NOT NULL,
		read       INTEGER DEFAULT 0,
		read_at    TEXT DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS team_pending_messages (
		id           TEXT PRIMARY KEY,
		to_agent     TEXT NOT NULL,
		from_agent   TEXT DEFAULT '',
		from_user    TEXT DEFAULT '',
		content      TEXT NOT NULL,
		thread_id    TEXT DEFAULT '',
		created_at   TEXT NOT NULL,
		delivered    INTEGER DEFAULT 0,
		delivered_at TEXT DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS team_activities (
		id         TEXT PRIMARY KEY,
		team_id    TEXT NOT NULL,
		type       TEXT NOT NULL,
		agent_id   TEXT DEFAULT '',
		message    TEXT NOT NULL,
		related_id TEXT DEFAULT '',
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS notification_rules (
		id           TEXT PRIMARY KEY,
		team_id      TEXT DEFAULT '',
		name         TEXT NOT NULL,
		enabled      INTEGER DEFAULT 1,
		events       TEXT NOT NULL,
		conditions   TEXT DEFAULT '{}',
		destinations TEXT NOT NULL,
		template     TEXT DEFAULT '',
		priority     INTEGER DEFAULT 0,
		rate_limit   INTEGER DEFAULT 0,
		quiet_hours  TEXT DEFAULT '',
		created_at   TEXT NOT NULL,
		updated_at   TEXT NOT NULL
	);
	`

	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func TestNotificationDispatcher_Dispatch(t *testing.T) {
	db := testNotificationDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	config := &NotificationConfig{Enabled: true}

	nd := NewNotificationDispatcher(db, nil, nil, nil, config, logger)

	notif := &TeamNotification{
		ID:        "notif001",
		TeamID:    "team001",
		Type:      NotifTaskCompleted,
		AgentID:   "agent001",
		AgentName: "TestAgent",
		TaskID:    "task001",
		TaskTitle: "Test Task",
		Action:    "task_completed",
		Result:    ResultSuccess,
		Message:   "Task completed successfully",
		Priority:  3,
		Timestamp: time.Now(),
	}

	err := nd.Dispatch(context.Background(), notif)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Verify notification was saved
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM team_notifications WHERE id = ?", notif.ID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query notifications: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 notification, got %d", count)
	}
}

func TestNotificationDispatcher_DispatchTaskCompletion(t *testing.T) {
	db := testNotificationDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	config := &NotificationConfig{Enabled: true}

	nd := NewNotificationDispatcher(db, nil, nil, nil, config, logger)

	tests := []struct {
		name    string
		success bool
		want    NotificationType
		result  NotificationResult
	}{
		{"success", true, NotifTaskCompleted, ResultSuccess},
		{"failure", false, NotifTaskFailed, ResultFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentID := "agent_" + tt.name
			err := nd.DispatchTaskCompletion(
				context.Background(),
				"team001", agentID, "TestAgent",
				"task001", "Test Task", "Test message "+tt.name,
				tt.success,
			)
			if err != nil {
				t.Fatalf("DispatchTaskCompletion failed: %v", err)
			}

			// Verify the notification type and result
			var notifType, result string
			err = db.QueryRow(`
				SELECT type, result FROM team_notifications
				WHERE agent_id = ? ORDER BY timestamp DESC LIMIT 1`,
				agentID,
			).Scan(&notifType, &result)
			if err != nil {
				t.Fatalf("Failed to query notification: %v", err)
			}

			if NotificationType(notifType) != tt.want {
				t.Errorf("Expected type %s, got %s", tt.want, notifType)
			}
			if NotificationResult(result) != tt.result {
				t.Errorf("Expected result %s, got %s", tt.result, result)
			}
		})
	}
}

func TestNotificationDispatcher_DispatchDisabled(t *testing.T) {
	db := testNotificationDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	config := &NotificationConfig{Enabled: false}

	nd := NewNotificationDispatcher(db, nil, nil, nil, config, logger)

	notif := &TeamNotification{
		ID:        "notif002",
		TeamID:    "team001",
		Type:      NotifTaskCompleted,
		AgentID:   "agent001",
		AgentName: "TestAgent",
		Message:   "Test",
		Timestamp: time.Now(),
	}

	err := nd.Dispatch(context.Background(), notif)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Verify notification was NOT saved (disabled)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM team_notifications").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query notifications: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 notifications when disabled, got %d", count)
	}
}

func TestNotificationDispatcher_SendToInbox(t *testing.T) {
	db := testNotificationDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	config := &NotificationConfig{Enabled: true}

	nd := NewNotificationDispatcher(db, nil, nil, nil, config, logger)

	notif := &TeamNotification{
		ID:        "notif003",
		TeamID:    "team001",
		AgentID:   "agent001",
		AgentName: "TestAgent",
		Message:   "Test notification",
		Timestamp: time.Now(),
	}

	dest := NotificationDestination{
		Type:   DestInbox,
		Target: "agent002",
	}

	err := nd.sendToInbox(context.Background(), notif, dest)
	if err != nil {
		t.Fatalf("sendToInbox failed: %v", err)
	}

	// Verify pending message was created
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM team_pending_messages WHERE to_agent = ?", "agent002").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query pending messages: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 pending message, got %d", count)
	}
}

func TestNotificationDispatcher_SendToActivity(t *testing.T) {
	db := testNotificationDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	config := &NotificationConfig{Enabled: true}

	nd := NewNotificationDispatcher(db, nil, nil, nil, config, logger)

	notif := &TeamNotification{
		ID:        "notif004",
		TeamID:    "team001",
		AgentID:   "agent001",
		AgentName: "TestAgent",
		Message:   "Test notification",
		TaskID:    "task001",
		Timestamp: time.Now(),
	}

	// Insert team first
	_, _ = db.Exec("INSERT INTO teams (id, name, owner_jid, created_at) VALUES (?, ?, ?, ?)",
		"team001", "Test Team", "owner001", time.Now().Format(time.RFC3339))

	nd.sendToActivity(context.Background(), notif)

	// Verify activity was created
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM team_activities WHERE team_id = ? AND agent_id = ?",
		"team001", "agent001").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query activities: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 activity, got %d", count)
	}
}

func TestNotificationDispatcher_FormatMessage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	nd := NewNotificationDispatcher(nil, nil, nil, nil, nil, logger)

	tests := []struct {
		name   string
		notif  *TeamNotification
		format string
		want   string // substring to check for
	}{
		{
			name: "success text",
			notif: &TeamNotification{
				AgentName: "TestAgent",
				Action:    "task_completed",
				TaskTitle: "Test Task",
				Message:   "Done!",
				Result:    ResultSuccess,
			},
			format: "text",
			want:   "TestAgent",
		},
		{
			name: "success markdown",
			notif: &TeamNotification{
				AgentName: "TestAgent",
				Action:    "task_completed",
				TaskTitle: "Test Task",
				Message:   "Done!",
				Result:    ResultSuccess,
			},
			format: "markdown",
			want:   "**TestAgent**",
		},
		{
			name: "failure",
			notif: &TeamNotification{
				AgentName: "TestAgent",
				Action:    "task_failed",
				Message:   "Error!",
				Result:    ResultFailure,
			},
			format: "text",
			want:   "TestAgent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nd.formatMessage(tt.notif, tt.format)
			if !contains(result, tt.want) {
				t.Errorf("formatMessage() = %q, want to contain %q", result, tt.want)
			}
		})
	}
}

func TestNotificationDispatcher_FindApplicableRules(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := &NotificationConfig{
		Enabled: true,
		Rules: []NotificationRule{
			{
				ID:      "rule001",
				Name:    "All Task Completed",
				Enabled: true,
				Events:  []NotificationType{NotifTaskCompleted},
			},
			{
				ID:      "rule002",
				TeamID:  "team001",
				Name:    "Team Specific",
				Enabled: true,
				Events:  []NotificationType{NotifTaskFailed},
			},
			{
				ID:      "rule003",
				Name:    "Disabled Rule",
				Enabled: false,
				Events:  []NotificationType{NotifTaskCompleted},
			},
		},
	}

	nd := NewNotificationDispatcher(nil, nil, nil, nil, config, logger)

	tests := []struct {
		name       string
		notif      *TeamNotification
		wantCount  int
		wantRuleID string
	}{
		{
			name: "global rule matches",
			notif: &TeamNotification{
				TeamID: "team002",
				Type:   NotifTaskCompleted,
			},
			wantCount:  2, // rule001 and rule003 match, even though rule003 is disabled
			wantRuleID: "rule001",
		},
		{
			name: "team specific rule",
			notif: &TeamNotification{
				TeamID: "team001",
				Type:   NotifTaskFailed,
			},
			wantCount:  1,
			wantRuleID: "rule002",
		},
		{
			name: "no matching event",
			notif: &TeamNotification{
				TeamID: "team001",
				Type:   NotifAgentError,
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := nd.findApplicableRules(tt.notif)
			if len(rules) != tt.wantCount {
				t.Errorf("Expected %d rules, got %d", tt.wantCount, len(rules))
			}
			if tt.wantRuleID != "" && len(rules) > 0 {
				if rules[0].ID != tt.wantRuleID {
					t.Errorf("Expected rule %s, got %s", tt.wantRuleID, rules[0].ID)
				}
			}
		})
	}
}

func TestNotificationDispatcher_QuietHours(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	nd := NewNotificationDispatcher(nil, nil, nil, nil, nil, logger)

	tests := []struct {
		name     string
		qh       *QuietHoursConfig
		expected bool
	}{
		{
			name: "disabled quiet hours",
			qh: &QuietHoursConfig{
				Enabled: false,
			},
			expected: false,
		},
		{
			name: "enabled but no time range",
			qh: &QuietHoursConfig{
				Enabled: true,
				Start:   "",
				End:     "",
			},
			expected: false,
		},
		{
			name: "enabled with same range (always quiet)",
			qh: &QuietHoursConfig{
				Enabled: true,
				Start:   "00:00",
				End:     "23:59",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nd.isQuietHours(tt.qh)
			if result != tt.expected {
				t.Errorf("isQuietHours() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNotificationDispatcher_RateLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	nd := NewNotificationDispatcher(nil, nil, nil, nil, nil, logger)

	// Test rate limit of 3
	ruleID := "test-rate-limit"
	limit := 3

	// First 3 should pass
	for i := 0; i < 3; i++ {
		if !nd.checkRateLimit(ruleID, limit) {
			t.Errorf("Rate limit should pass on attempt %d", i+1)
		}
	}

	// 4th should fail
	if nd.checkRateLimit(ruleID, limit) {
		t.Errorf("Rate limit should fail on 4th attempt")
	}
}

func TestNotificationDispatcher_GetNotifications(t *testing.T) {
	db := testNotificationDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	config := &NotificationConfig{Enabled: true}

	nd := NewNotificationDispatcher(db, nil, nil, nil, config, logger)

	// Insert test notifications
	now := time.Now()
	for i := 0; i < 5; i++ {
		_, _ = db.Exec(`
			INSERT INTO team_notifications (
				id, team_id, type, agent_id, agent_name, action, result, message, priority, timestamp
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"notif00"+string(rune('1'+i)),
			"team001",
			"task_completed",
			"agent001",
			"TestAgent",
			"task_completed",
			"success",
			"Test message",
			3,
			now.Add(time.Duration(i)*time.Minute).Format(time.RFC3339),
		)
	}

	// Test GetNotifications
	notifications, err := nd.GetNotifications(context.Background(), "team001", 3)
	if err != nil {
		t.Fatalf("GetNotifications failed: %v", err)
	}

	if len(notifications) != 3 {
		t.Errorf("Expected 3 notifications, got %d", len(notifications))
	}

	// Test GetUnreadNotifications
	unread, err := nd.GetUnreadNotifications(context.Background(), "team001")
	if err != nil {
		t.Fatalf("GetUnreadNotifications failed: %v", err)
	}

	if len(unread) != 5 {
		t.Errorf("Expected 5 unread notifications, got %d", len(unread))
	}

	// Mark one as read
	err = nd.MarkNotificationRead(context.Background(), "notif001")
	if err != nil {
		t.Fatalf("MarkNotificationRead failed: %v", err)
	}

	// Check unread count decreased
	unread, err = nd.GetUnreadNotifications(context.Background(), "team001")
	if err != nil {
		t.Fatalf("GetUnreadNotifications failed: %v", err)
	}

	if len(unread) != 4 {
		t.Errorf("Expected 4 unread after marking one read, got %d", len(unread))
	}
}

func TestNotificationDispatcher_ShouldFire(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	tests := []struct {
		name     string
		rule     *NotificationRule
		notif    *TeamNotification
		expected bool
	}{
		{
			name: "no quiet hours",
			rule: &NotificationRule{
				ID:        "rule001",
				Enabled:   true,
				RateLimit: 0,
			},
			notif:    &TeamNotification{Priority: 3},
			expected: true,
		},
		{
			name: "urgent during quiet hours",
			rule: &NotificationRule{
				ID:      "rule002",
				Enabled: true,
				QuietHours: &QuietHoursConfig{
					Enabled: true,
					Start:   "00:00",
					End:     "23:59",
				},
			},
			notif:    &TeamNotification{Priority: 1},
			expected: true,
		},
		{
			name: "normal during quiet hours",
			rule: &NotificationRule{
				ID:      "rule003",
				Enabled: true,
				QuietHours: &QuietHoursConfig{
					Enabled: true,
					Start:   "00:00",
					End:     "23:59",
				},
			},
			notif:    &TeamNotification{Priority: 3},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nd := NewNotificationDispatcher(nil, nil, nil, nil, nil, logger)
			result := nd.shouldFire(tt.rule, tt.notif)
			if result != tt.expected {
				t.Errorf("shouldFire() = %v, want %v", result, tt.expected)
			}
		})
	}
}

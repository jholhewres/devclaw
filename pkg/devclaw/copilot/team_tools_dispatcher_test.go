// Package copilot – team_tools_dispatcher_test.go tests the dispatcher pattern tools.
package copilot

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// testDispatcherDB creates an in-memory SQLite database for testing dispatcher tools.
func testDispatcherDB(t *testing.T) *sql.DB {
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

	CREATE TABLE IF NOT EXISTS persistent_agents (
		id                 TEXT PRIMARY KEY,
		name               TEXT NOT NULL,
		role               TEXT NOT NULL,
		team_id            TEXT NOT NULL,
		level              TEXT DEFAULT 'specialist',
		status             TEXT DEFAULT 'idle',
		personality        TEXT DEFAULT '',
		instructions       TEXT DEFAULT '',
		model              TEXT DEFAULT '',
		skills             TEXT DEFAULT '[]',
		session_id         TEXT DEFAULT '',
		current_task_id    TEXT DEFAULT '',
		heartbeat_schedule TEXT DEFAULT '*/15 * * * *',
		created_at         TEXT NOT NULL,
		last_active_at     TEXT DEFAULT '',
		last_heartbeat_at  TEXT DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS team_tasks (
		id             TEXT PRIMARY KEY,
		team_id        TEXT NOT NULL,
		title          TEXT NOT NULL,
		description    TEXT DEFAULT '',
		status         TEXT DEFAULT 'inbox',
		assignees      TEXT DEFAULT '[]',
		priority       INTEGER DEFAULT 3,
		labels         TEXT DEFAULT '[]',
		created_by     TEXT NOT NULL,
		created_at     TEXT NOT NULL,
		updated_at     TEXT NOT NULL,
		completed_at   TEXT DEFAULT '',
		blocked_reason TEXT DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS team_messages (
		id         TEXT PRIMARY KEY,
		team_id    TEXT NOT NULL,
		thread_id  TEXT DEFAULT '',
		from_agent TEXT DEFAULT '',
		from_user  TEXT DEFAULT '',
		content    TEXT NOT NULL,
		mentions   TEXT DEFAULT '[]',
		created_at TEXT NOT NULL,
		delivered  INTEGER DEFAULT 0
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

	CREATE TABLE IF NOT EXISTS team_facts (
		id         TEXT PRIMARY KEY,
		team_id    TEXT NOT NULL,
		key        TEXT NOT NULL,
		value      TEXT NOT NULL,
		author     TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(team_id, key)
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

	CREATE TABLE IF NOT EXISTS agent_working_state (
		agent_id        TEXT PRIMARY KEY,
		team_id         TEXT NOT NULL,
		current_task_id TEXT DEFAULT '',
		status          TEXT DEFAULT 'idle',
		next_steps      TEXT DEFAULT '',
		context         TEXT DEFAULT '',
		updated_at      TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS team_documents (
		id          TEXT PRIMARY KEY,
		team_id     TEXT NOT NULL,
		task_id     TEXT DEFAULT '',
		title       TEXT NOT NULL,
		doc_type    TEXT DEFAULT 'deliverable',
		content     TEXT NOT NULL,
		format      TEXT DEFAULT 'markdown',
		file_path   TEXT DEFAULT '',
		version     INTEGER DEFAULT 1,
		author      TEXT NOT NULL,
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS team_thread_subscriptions (
		id            TEXT PRIMARY KEY,
		team_id       TEXT NOT NULL,
		thread_id     TEXT NOT NULL,
		agent_id      TEXT NOT NULL,
		subscribed_at TEXT NOT NULL,
		reason        TEXT DEFAULT 'auto',
		UNIQUE(team_id, thread_id, agent_id)
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
		read       INTEGER DEFAULT 0
	);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create test schema: %v", err)
	}

	return db
}

func testDispatcherSetup(t *testing.T) (*sql.DB, *TeamManager, *handlerContext) {
	db := testDispatcherDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tm := NewTeamManager(db, nil, logger)
	hctx := &handlerContext{
		teamMgr: tm,
		db:      db,
		logger:  logger,
	}
	return db, tm, hctx
}

// testContext creates a context with caller information for testing.
func testContext(callerJID string) context.Context {
	return ContextWithCaller(context.Background(), AccessUser, callerJID)
}

// ═══════════════════════════════════════════════════════════════════════════════
// TEAM MANAGE DISPATCHER TESTS
// ═══════════════════════════════════════════════════════════════════════════════

func TestDispatcher_TeamManage_Create(t *testing.T) {
	db, _, hctx := testDispatcherSetup(t)
	defer db.Close()

	ctx := testContext("user123")

	result, err := hctx.handleTeamCreate(ctx, map[string]any{
		"name":        "Engineering",
		"description": "Dev team",
	})
	if err != nil {
		t.Fatalf("handleTeamCreate failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Team created successfully") {
		t.Errorf("expected success message, got: %s", resultStr)
	}
	if !strings.Contains(resultStr, "Engineering") {
		t.Errorf("expected team name in output, got: %s", resultStr)
	}
}

func TestDispatcher_TeamManage_List(t *testing.T) {
	db, _, hctx := testDispatcherSetup(t)
	defer db.Close()

	ctx := testContext("user123")

	// Create two teams
	hctx.handleTeamCreate(ctx, map[string]any{"name": "Team Alpha", "description": "First team"})
	hctx.handleTeamCreate(ctx, map[string]any{"name": "Team Beta", "description": "Second team"})

	result, err := hctx.handleTeamList(map[string]any{})
	if err != nil {
		t.Fatalf("handleTeamList failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Found 2 teams") {
		t.Errorf("expected 2 teams, got: %s", resultStr)
	}
}

func TestDispatcher_TeamManage_Get(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	// Create a team
	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")

	result, err := hctx.handleTeamGet(map[string]any{"team_id": team.ID})
	if err != nil {
		t.Fatalf("handleTeamGet failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Test Team") {
		t.Errorf("expected team name in output, got: %s", resultStr)
	}
}

func TestDispatcher_TeamManage_Update(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	ctx := testContext("user123")

	// Create a team
	team, _ := tm.CreateTeam("Original Name", "Description", "user123", "")

	_, err := hctx.handleTeamUpdate(ctx, map[string]any{
		"team_id":     team.ID,
		"name":        "Updated Name",
		"description": "Updated description",
	})
	if err != nil {
		t.Fatalf("handleTeamUpdate failed: %v", err)
	}

	// Verify update
	updated, _ := tm.GetTeam(team.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got: %s", updated.Name)
	}
}

func TestDispatcher_TeamManage_Delete(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	// Create a team
	team, _ := tm.CreateTeam("Team to Delete", "Description", "user123", "")

	_, err := hctx.handleTeamDelete(map[string]any{"team_id": team.ID})
	if err != nil {
		t.Fatalf("handleTeamDelete failed: %v", err)
	}

	// Verify deletion
	deleted, _ := tm.GetTeam(team.ID)
	if deleted != nil {
		t.Error("expected team to be deleted")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TEAM AGENT DISPATCHER TESTS
// ═══════════════════════════════════════════════════════════════════════════════

func TestDispatcher_TeamAgent_Create(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	// Create a team first
	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")

	result, err := hctx.handleAgentCreate(map[string]any{
		"team_id": team.ID,
		"name":    "Jarvis",
		"role":    "Assistant",
		"level":   "senior",
	})
	if err != nil {
		t.Fatalf("handleAgentCreate failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Agent created successfully") {
		t.Errorf("expected success message, got: %s", resultStr)
	}
}

func TestDispatcher_TeamAgent_List(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	// Create a team and agents
	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	tm.CreateAgent(team.ID, "Agent1", "Role", "", "", "", nil, AgentLevelSpecialist, "")
	tm.CreateAgent(team.ID, "Agent2", "Role", "", "", "", nil, AgentLevelSpecialist, "")

	result, err := hctx.handleAgentList(map[string]any{"team_id": team.ID})
	if err != nil {
		t.Fatalf("handleAgentList failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Found 2 agents") {
		t.Errorf("expected 2 agents, got: %s", resultStr)
	}
}

func TestDispatcher_TeamAgent_Get(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	// Create a team and agent
	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	agent, _ := tm.CreateAgent(team.ID, "TestAgent", "Role", "", "", "", nil, AgentLevelSpecialist, "")

	result, err := hctx.handleAgentGet(map[string]any{"agent_id": agent.ID})
	if err != nil {
		t.Fatalf("handleAgentGet failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "TestAgent") {
		t.Errorf("expected agent name in output, got: %s", resultStr)
	}
}

func TestDispatcher_TeamAgent_Update(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	// Create a team and agent
	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	agent, _ := tm.CreateAgent(team.ID, "Original", "Role", "", "", "", nil, AgentLevelSpecialist, "")

	_, err := hctx.handleAgentUpdate(map[string]any{
		"agent_id": agent.ID,
		"name":     "Updated",
		"role":     "New Role",
	})
	if err != nil {
		t.Fatalf("handleAgentUpdate failed: %v", err)
	}

	// Verify update
	updated, _ := tm.GetAgent(agent.ID)
	if updated.Name != "Updated" {
		t.Errorf("expected name 'Updated', got: %s", updated.Name)
	}
}

func TestDispatcher_TeamAgent_WorkingState(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	// Create a team and agent
	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	agent, _ := tm.CreateAgent(team.ID, "TestAgent", "Role", "", "", "", nil, AgentLevelSpecialist, "")

	// Update working state
	_, err := hctx.handleAgentWorkingUpdate(map[string]any{
		"agent_id":        agent.ID,
		"current_task_id": "task123",
		"status":          "working",
		"next_steps":      "Continue coding",
		"context":         "Implementing feature X",
	})
	if err != nil {
		t.Fatalf("handleAgentWorkingUpdate failed: %v", err)
	}

	// Get working state
	result, err := hctx.handleAgentWorkingGet(map[string]any{"agent_id": agent.ID})
	if err != nil {
		t.Fatalf("handleAgentWorkingGet failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "task123") {
		t.Errorf("expected task_id in output, got: %s", resultStr)
	}

	// Clear working state
	_, err = hctx.handleAgentWorkingClear(map[string]any{"agent_id": agent.ID})
	if err != nil {
		t.Fatalf("handleAgentWorkingClear failed: %v", err)
	}

	// Verify cleared
	result, _ = hctx.handleAgentWorkingGet(map[string]any{"agent_id": agent.ID})
	resultStr = result.(string)
	if !strings.Contains(resultStr, "idle") {
		t.Errorf("expected idle state after clear, got: %s", resultStr)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TEAM TASK DISPATCHER TESTS
// ═══════════════════════════════════════════════════════════════════════════════

func TestDispatcher_TeamTask_Create(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	result, err := hctx.handleTaskCreate(ctx, map[string]any{
		"team_id":     team.ID,
		"title":       "Implement Feature X",
		"description": "Add new feature",
	})
	if err != nil {
		t.Fatalf("handleTaskCreate failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Task created successfully") {
		t.Errorf("expected success message, got: %s", resultStr)
	}
}

func TestDispatcher_TeamTask_List(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	// Create tasks
	hctx.handleTaskCreate(ctx, map[string]any{"team_id": team.ID, "title": "Task 1"})
	hctx.handleTaskCreate(ctx, map[string]any{"team_id": team.ID, "title": "Task 2"})

	result, err := hctx.handleTaskList(map[string]any{"team_id": team.ID})
	if err != nil {
		t.Fatalf("handleTaskList failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Found 2 tasks") {
		t.Errorf("expected 2 tasks, got: %s", resultStr)
	}
}

func TestDispatcher_TeamTask_Update(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	// Create task
	teamMem := tm.GetTeamMemory(team.ID)
	task, _ := teamMem.CreateTask("Test Task", "Description", "user123", nil)

	// Update task status
	_, err := hctx.handleTaskUpdate(ctx, map[string]any{
		"team_id": team.ID,
		"task_id": task.ID,
		"status":  "in_progress",
		"comment": "Starting work",
	})
	if err != nil {
		t.Fatalf("handleTaskUpdate failed: %v", err)
	}

	// Verify update
	updated, _ := teamMem.GetTask(task.ID)
	if updated.Status != TaskStatusProgress {
		t.Errorf("expected status 'in_progress', got: %s", updated.Status)
	}
}

func TestDispatcher_TeamTask_Assign(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	agent, _ := tm.CreateAgent(team.ID, "Agent1", "Role", "", "", "", nil, AgentLevelSpecialist, "")
	ctx := testContext("user123")

	// Create task
	teamMem := tm.GetTeamMemory(team.ID)
	task, _ := teamMem.CreateTask("Test Task", "Description", "user123", nil)

	// Assign task
	_, err := hctx.handleTaskAssign(ctx, map[string]any{
		"team_id":    team.ID,
		"task_id":    task.ID,
		"assignees":  []string{agent.ID},
	})
	if err != nil {
		t.Fatalf("handleTaskAssign failed: %v", err)
	}

	// Verify assignment
	updated, _ := teamMem.GetTask(task.ID)
	if len(updated.Assignees) != 1 || updated.Assignees[0] != agent.ID {
		t.Errorf("expected agent %s assigned, got: %v", agent.ID, updated.Assignees)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TEAM MEMORY DISPATCHER TESTS
// ═══════════════════════════════════════════════════════════════════════════════

func TestDispatcher_TeamMemory_FactSave(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	result, err := hctx.handleFactSave(ctx, map[string]any{
		"team_id": team.ID,
		"key":     "project_name",
		"value":   "DevClaw",
	})
	if err != nil {
		t.Fatalf("handleFactSave failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Fact saved") {
		t.Errorf("expected success message, got: %s", resultStr)
	}
}

func TestDispatcher_TeamMemory_FactList(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	// Save some facts
	hctx.handleFactSave(ctx, map[string]any{"team_id": team.ID, "key": "key1", "value": "value1"})
	hctx.handleFactSave(ctx, map[string]any{"team_id": team.ID, "key": "key2", "value": "value2"})

	result, err := hctx.handleFactList(map[string]any{"team_id": team.ID})
	if err != nil {
		t.Fatalf("handleFactList failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Team Facts (2)") {
		t.Errorf("expected 2 facts, got: %s", resultStr)
	}
}

func TestDispatcher_TeamMemory_DocCreate(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	result, err := hctx.handleDocCreate(ctx, map[string]any{
		"team_id":  team.ID,
		"title":    "API Documentation",
		"content":  "# API Docs\n\nThis is the API documentation.",
		"doc_type": "protocol",
	})
	if err != nil {
		t.Fatalf("handleDocCreate failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Document created") {
		t.Errorf("expected success message, got: %s", resultStr)
	}
}

func TestDispatcher_TeamMemory_DocList(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	// Create documents
	hctx.handleDocCreate(ctx, map[string]any{"team_id": team.ID, "title": "Doc 1", "content": "Content 1"})
	hctx.handleDocCreate(ctx, map[string]any{"team_id": team.ID, "title": "Doc 2", "content": "Content 2"})

	result, err := hctx.handleDocList(map[string]any{"team_id": team.ID})
	if err != nil {
		t.Fatalf("handleDocList failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Documents (2)") {
		t.Errorf("expected 2 documents, got: %s", resultStr)
	}
}

func TestDispatcher_TeamMemory_Standup(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	// Create some activity
	hctx.handleFactSave(ctx, map[string]any{"team_id": team.ID, "key": "status", "value": "active"})

	result, err := hctx.handleStandup(map[string]any{"team_id": team.ID})
	if err != nil {
		t.Fatalf("handleStandup failed: %v", err)
	}

	// Standup should return some content (even if minimal)
	resultStr := result.(string)
	if resultStr == "" {
		t.Error("expected non-empty standup output")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TEAM COMM DISPATCHER TESTS
// ═══════════════════════════════════════════════════════════════════════════════

func TestDispatcher_TeamComm_Comment(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	// Create a task first
	teamMem := tm.GetTeamMemory(team.ID)
	task, _ := teamMem.CreateTask("Test Task", "Description", "user123", nil)

	result, err := hctx.handleComment(ctx, map[string]any{
		"team_id": team.ID,
		"task_id": task.ID,
		"content": "This is a comment",
	})
	if err != nil {
		t.Fatalf("handleComment failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Comment added") {
		t.Errorf("expected success message, got: %s", resultStr)
	}
}

func TestDispatcher_TeamComm_MentionCheck(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	agent, _ := tm.CreateAgent(team.ID, "TestAgent", "Role", "", "", "", nil, AgentLevelSpecialist, "")

	// Check for mentions (should be empty initially)
	result, err := hctx.handleMentionCheck(map[string]any{"agent_id": agent.ID})
	if err != nil {
		t.Fatalf("handleMentionCheck failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "No pending mentions") {
		t.Errorf("expected no pending mentions, got: %s", resultStr)
	}
}

func TestDispatcher_TeamComm_SendMessage(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	agent1, _ := tm.CreateAgent(team.ID, "Agent1", "Role", "", "", "", nil, AgentLevelSpecialist, "")
	agent2, _ := tm.CreateAgent(team.ID, "Agent2", "Role", "", "", "", nil, AgentLevelSpecialist, "")

	ctx := ContextWithSession(testContext("user123"), "agent:"+agent1.ID)

	result, err := hctx.handleSendMessage(ctx, map[string]any{
		"team_id":   team.ID,
		"to_agent":  agent2.ID,
		"content":   "Hello from Agent1",
	})
	if err != nil {
		t.Fatalf("handleSendMessage failed: %v", err)
	}

	resultStr := result.(string)
	if !strings.Contains(resultStr, "Message sent") {
		t.Errorf("expected success message, got: %s", resultStr)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ERROR HANDLING TESTS
// ═══════════════════════════════════════════════════════════════════════════════

func TestDispatcher_ErrorHandling_MissingTeamID(t *testing.T) {
	db, _, hctx := testDispatcherSetup(t)
	defer db.Close()

	// Team create without team_id (should fail - name required)
	_, err := hctx.handleTeamCreate(context.Background(), map[string]any{})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestDispatcher_ErrorHandling_InvalidTeamID(t *testing.T) {
	db, _, hctx := testDispatcherSetup(t)
	defer db.Close()

	_, err := hctx.handleTeamGet(map[string]any{"team_id": "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent team")
	}
}

func TestDispatcher_ErrorHandling_AgentWithoutTeam(t *testing.T) {
	db, _, hctx := testDispatcherSetup(t)
	defer db.Close()

	_, err := hctx.handleAgentCreate(map[string]any{
		"name": "TestAgent",
		"role": "Role",
	})
	if err == nil {
		t.Error("expected error for missing team_id")
	}
}

func TestDispatcher_ErrorHandling_TaskWithoutTeam(t *testing.T) {
	db, _, hctx := testDispatcherSetup(t)
	defer db.Close()

	ctx := testContext("user123")

	_, err := hctx.handleTaskCreate(ctx, map[string]any{
		"title": "Test Task",
	})
	if err == nil {
		t.Error("expected error for missing team_id")
	}
}

func TestDispatcher_ErrorHandling_WorkingStateWithoutAgent(t *testing.T) {
	db, _, hctx := testDispatcherSetup(t)
	defer db.Close()

	_, err := hctx.handleAgentWorkingUpdate(map[string]any{
		"status": "working",
	})
	if err == nil {
		t.Error("expected error for missing agent_id")
	}
}

func TestDispatcher_ErrorHandling_FactWithoutKey(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	_, err := hctx.handleFactSave(ctx, map[string]any{
		"team_id": team.ID,
		"value":   "some value",
	})
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestDispatcher_ErrorHandling_DocWithoutContent(t *testing.T) {
	db, tm, hctx := testDispatcherSetup(t)
	defer db.Close()

	team, _ := tm.CreateTeam("Test Team", "Description", "user123", "")
	ctx := testContext("user123")

	_, err := hctx.handleDocCreate(ctx, map[string]any{
		"team_id": team.ID,
		"title":   "Test Doc",
	})
	if err == nil {
		t.Error("expected error for missing content")
	}
}

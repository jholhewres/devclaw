// Package copilot – team_manager_test.go tests the team manager system.
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

// testTeamManagerDB creates an in-memory SQLite database for testing.
func testTeamManagerDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create tables
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
	`

	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func TestTeamManager_CreateTeam(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	// Create a team
	team, err := tm.CreateTeam("Engineering", "Software development team", "user123", "claude-sonnet-4")
	if err != nil {
		t.Fatalf("CreateTeam failed: %v", err)
	}

	if team.ID == "" {
		t.Error("Team ID should not be empty")
	}
	if team.Name != "Engineering" {
		t.Errorf("Expected name 'Engineering', got '%s'", team.Name)
	}
	if team.OwnerJID != "user123" {
		t.Errorf("Expected owner 'user123', got '%s'", team.OwnerJID)
	}
	if !team.Enabled {
		t.Error("Team should be enabled by default")
	}
}

func TestTeamManager_GetTeam(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	// Create and retrieve
	created, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	team, err := tm.GetTeam(created.ID)
	if err != nil {
		t.Fatalf("GetTeam failed: %v", err)
	}

	if team == nil {
		t.Fatal("Team should not be nil")
	}
	if team.Name != "Test Team" {
		t.Errorf("Expected name 'Test Team', got '%s'", team.Name)
	}

	// Non-existent team
	notFound, _ := tm.GetTeam("nonexistent")
	if notFound != nil {
		t.Error("Non-existent team should be nil")
	}
}

func TestTeamManager_ListTeams(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	// Create multiple teams
	tm.CreateTeam("Team 1", "Desc 1", "user1", "")
	tm.CreateTeam("Team 2", "Desc 2", "user2", "")
	tm.CreateTeam("Team 3", "Desc 3", "user3", "")

	teams, err := tm.ListTeams()
	if err != nil {
		t.Fatalf("ListTeams failed: %v", err)
	}
	if len(teams) != 3 {
		t.Errorf("Expected 3 teams, got %d", len(teams))
	}
}

func TestTeamManager_CreateAgent(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	// Create team first
	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "claude-sonnet-4")

	// Create agent
	agent, err := tm.CreateAgent(
		team.ID,
		"Jarvis",
		"Squad Lead",
		"You are Jarvis, the coordinator.",
		"Monitor and delegate tasks.",
		"claude-opus-4",
		[]string{"coordination", "planning"},
		AgentLevelLead,
		"*/10 * * * *",
	)
	if err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}

	if agent.ID != "jarvis" {
		t.Errorf("Expected ID 'jarvis', got '%s'", agent.ID)
	}
	if agent.Name != "Jarvis" {
		t.Errorf("Expected name 'Jarvis', got '%s'", agent.Name)
	}
	if agent.Role != "Squad Lead" {
		t.Errorf("Expected role 'Squad Lead', got '%s'", agent.Role)
	}
	if agent.Level != AgentLevelLead {
		t.Errorf("Expected level 'lead', got '%s'", agent.Level)
	}
	if agent.Status != AgentStatusIdle {
		t.Errorf("Expected status 'idle', got '%s'", agent.Status)
	}
	if agent.HeartbeatSchedule != "*/10 * * * *" {
		t.Errorf("Expected custom heartbeat schedule, got '%s'", agent.HeartbeatSchedule)
	}
}

func TestTeamManager_CreateAgent_Defaults(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")

	// Create agent with minimal params
	agent, err := tm.CreateAgent(team.ID, "Loki", "Writer", "", "", "", nil, AgentLevelSpecialist, "")
	if err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}

	// Check defaults
	if agent.Level != AgentLevelSpecialist {
		t.Errorf("Expected default level 'specialist', got '%s'", agent.Level)
	}
	if agent.HeartbeatSchedule != "*/15 * * * *" {
		t.Errorf("Expected default heartbeat, got '%s'", agent.HeartbeatSchedule)
	}
}

func TestTeamManager_CreateAgent_NonexistentTeam(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	_, err := tm.CreateAgent("nonexistent", "Agent", "Role", "", "", "", nil, AgentLevelSpecialist, "")
	if err == nil {
		t.Error("Expected error for nonexistent team")
	}
}

func TestTeamManager_GetAgent(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	created, _ := tm.CreateAgent(team.ID, "TestAgent", "Test Role", "Personality", "", "", nil, AgentLevelSpecialist, "")

	agent, err := tm.GetAgent(created.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if agent == nil {
		t.Fatal("Agent should not be nil")
	}
	if agent.Name != "TestAgent" {
		t.Errorf("Expected name 'TestAgent', got '%s'", agent.Name)
	}

	// Non-existent
	notFound, _ := tm.GetAgent("nonexistent")
	if notFound != nil {
		t.Error("Non-existent agent should be nil")
	}
}

func TestTeamManager_ListAgents(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team1, _ := tm.CreateTeam("Team 1", "Desc", "user1", "")
	team2, _ := tm.CreateTeam("Team 2", "Desc", "user2", "")

	tm.CreateAgent(team1.ID, "Agent1", "Role", "", "", "", nil, AgentLevelSpecialist, "")
	tm.CreateAgent(team1.ID, "Agent2", "Role", "", "", "", nil, AgentLevelSpecialist, "")
	tm.CreateAgent(team2.ID, "Agent3", "Role", "", "", "", nil, AgentLevelSpecialist, "")

	// List all
	all, _ := tm.ListAgents("")
	if len(all) != 3 {
		t.Errorf("Expected 3 agents total, got %d", len(all))
	}

	// List by team
	team1Agents, _ := tm.ListAgents(team1.ID)
	if len(team1Agents) != 2 {
		t.Errorf("Expected 2 agents in team1, got %d", len(team1Agents))
	}

	team2Agents, _ := tm.ListAgents(team2.ID)
	if len(team2Agents) != 1 {
		t.Errorf("Expected 1 agent in team2, got %d", len(team2Agents))
	}
}

func TestTeamManager_UpdateAgentStatus(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	agent, _ := tm.CreateAgent(team.ID, "TestAgent", "Role", "", "", "", nil, AgentLevelSpecialist, "")

	// Update status
	err := tm.UpdateAgentStatus(agent.ID, AgentStatusActive)
	if err != nil {
		t.Fatalf("UpdateAgentStatus failed: %v", err)
	}

	// Verify
	updated, _ := tm.GetAgent(agent.ID)
	if updated.Status != AgentStatusActive {
		t.Errorf("Expected status 'active', got '%s'", updated.Status)
	}
}

func TestTeamManager_StopAndDeleteAgent(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	agent, _ := tm.CreateAgent(team.ID, "TestAgent", "Role", "", "", "", nil, AgentLevelSpecialist, "")

	// Stop agent
	err := tm.StopAgent(agent.ID)
	if err != nil {
		t.Fatalf("StopAgent failed: %v", err)
	}

	stopped, _ := tm.GetAgent(agent.ID)
	if stopped.Status != AgentStatusStopped {
		t.Errorf("Expected status 'stopped', got '%s'", stopped.Status)
	}

	// Delete agent
	err = tm.DeleteAgent(agent.ID)
	if err != nil {
		t.Fatalf("DeleteAgent failed: %v", err)
	}

	deleted, _ := tm.GetAgent(agent.ID)
	if deleted != nil {
		t.Error("Deleted agent should be nil")
	}
}

func TestTeamManager_FindAgentByName(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	tm.CreateAgent(team.ID, "Jarvis", "Lead", "", "", "", nil, AgentLevelLead, "")
	tm.CreateAgent(team.ID, "Loki", "Writer", "", "", "", nil, AgentLevelSpecialist, "")

	// Find by name (case-insensitive)
	agent, err := tm.FindAgentByName("JARVIS")
	if err != nil {
		t.Fatalf("FindAgentByName failed: %v", err)
	}
	if agent == nil {
		t.Fatal("Should find Jarvis")
	}
	if agent.ID != "jarvis" {
		t.Errorf("Expected ID 'jarvis', got '%s'", agent.ID)
	}

	// Find by ID
	agent2, _ := tm.FindAgentByName("loki")
	if agent2 == nil {
		t.Fatal("Should find Loki by ID")
	}

	// Not found
	notFound, _ := tm.FindAgentByName("nonexistent")
	if notFound != nil {
		t.Error("Non-existent agent should be nil")
	}
}

func TestTeamManager_ParseMentions(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	tm.CreateAgent(team.ID, "Jarvis", "Lead", "", "", "", nil, AgentLevelLead, "")
	tm.CreateAgent(team.ID, "Loki", "Writer", "", "", "", nil, AgentLevelSpecialist, "")

	text := "Hey @jarvis, can you check this? @loki needs help too. @unknown is not an agent."

	mentions := tm.ParseMentions(text)

	if len(mentions) != 2 {
		t.Errorf("Expected 2 mentions, got %d: %v", len(mentions), mentions)
	}

	// Should contain jarvis and loki
	foundJarvis := false
	foundLoki := false
	for _, m := range mentions {
		if m == "jarvis" {
			foundJarvis = true
		}
		if m == "loki" {
			foundLoki = true
		}
	}

	if !foundJarvis {
		t.Error("Should have found @jarvis")
	}
	if !foundLoki {
		t.Error("Should have found @loki")
	}
}

func TestTeamManager_BuildAgentSystemPrompt(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	agent, _ := tm.CreateAgent(
		team.ID,
		"Jarvis",
		"Squad Lead",
		"You are a helpful coordinator.",
		"Always be polite.",
		"claude-opus-4",
		[]string{"planning"},
		AgentLevelLead,
		"*/15 * * * *",
	)

	teamMem := tm.GetTeamMemory(team.ID)

	prompt := tm.BuildAgentSystemPrompt(agent, teamMem)

	// Should contain all key info
	if !strings.Contains(prompt, "Jarvis") {
		t.Error("Prompt should contain agent name")
	}
	if !strings.Contains(prompt, "Squad Lead") {
		t.Error("Prompt should contain agent role")
	}
	if !strings.Contains(prompt, "jarvis") {
		t.Error("Prompt should contain agent ID")
	}
	if !strings.Contains(prompt, "helpful coordinator") {
		t.Error("Prompt should contain personality")
	}
	if !strings.Contains(prompt, "Always be polite") {
		t.Error("Prompt should contain instructions")
	}
	if !strings.Contains(prompt, "team_check_mentions") {
		t.Error("Prompt should describe team tools")
	}
}

func TestTeamManager_AgentIDGeneration(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")

	tests := []struct {
		name     string
		expected string
	}{
		{"Jarvis", "jarvis"},
		{"Mary Jane", "mary-jane"},
		{"Agent-123", "agent-123"},
		{"Test@Agent!", "testagent"},
	}

	for _, tt := range tests {
		agent, err := tm.CreateAgent(team.ID, tt.name, "Role", "", "", "", nil, AgentLevelSpecialist, "")
		if err != nil {
			t.Errorf("CreateAgent(%s) failed: %v", tt.name, err)
			continue
		}
		if agent.ID != tt.expected {
			t.Errorf("Name '%s': expected ID '%s', got '%s'", tt.name, tt.expected, agent.ID)
		}
	}
}

func TestTeamManager_GetTeamMemory(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")

	mem := tm.GetTeamMemory(team.ID)
	if mem == nil {
		t.Fatal("GetTeamMemory should return non-nil")
	}

	// Should be able to use the memory
	mem.SaveFact("test_key", "test_value", "user")
	facts, _ := mem.GetFacts()
	if len(facts) != 1 {
		t.Errorf("Expected 1 fact, got %d", len(facts))
	}
}

func TestTeamManager_SendToAgent(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	tm.CreateAgent(team.ID, "Jarvis", "Lead", "", "", "", nil, AgentLevelLead, "")

	// Send message to agent
	err := tm.SendToAgent(context.Background(), "jarvis", "user", "Hello Jarvis!")
	if err != nil {
		t.Fatalf("SendToAgent failed: %v", err)
	}

	// Verify message is pending
	teamMem := tm.GetTeamMemory(team.ID)
	pending, _ := teamMem.GetPendingMessages("jarvis", false)
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending message, got %d", len(pending))
	}
	if pending[0].Content != "Hello Jarvis!" {
		t.Errorf("Expected content 'Hello Jarvis!', got '%s'", pending[0].Content)
	}
}

func TestTeamManager_RecordHeartbeat(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	agent, _ := tm.CreateAgent(team.ID, "Jarvis", "Lead", "", "", "", nil, AgentLevelLead, "")

	// Record heartbeat
	err := tm.RecordHeartbeat(agent.ID)
	if err != nil {
		t.Fatalf("RecordHeartbeat failed: %v", err)
	}

	// Verify
	updated, _ := tm.GetAgent(agent.ID)
	if updated.LastHeartbeatAt == nil {
		t.Error("LastHeartbeatAt should be set")
	}
}

func TestTeamManager_SetAgentCurrentTask(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	agent, _ := tm.CreateAgent(team.ID, "Jarvis", "Lead", "", "", "", nil, AgentLevelLead, "")

	// Set current task
	err := tm.SetAgentCurrentTask(agent.ID, "task123")
	if err != nil {
		t.Fatalf("SetAgentCurrentTask failed: %v", err)
	}

	// Note: current_task_id is stored in DB but not loaded in GetAgent
	// This is by design (it's for internal tracking)
}

func TestTeamManager_LoadAgentsFromDB(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Create first manager and add agents
	tm1 := NewTeamManager(db, nil, logger)
	team, _ := tm1.CreateTeam("Test Team", "Desc", "user1", "")
	tm1.CreateAgent(team.ID, "Agent1", "Role", "", "", "", nil, AgentLevelSpecialist, "")
	tm1.CreateAgent(team.ID, "Agent2", "Role", "", "", "", nil, AgentLevelSpecialist, "")

	// Stop one agent (should not be loaded)
	tm1.StopAgent("agent2")

	// Create second manager (simulates restart)
	tm2 := NewTeamManager(db, nil, logger)

	// Should have loaded agent1 (not stopped)
	agent1, _ := tm2.GetAgent("agent1")
	if agent1 == nil {
		t.Error("Agent1 should be loaded")
	}

	// Agent2 should exist in DB but be stopped
	agent2, _ := tm2.GetAgent("agent2")
	if agent2 == nil {
		t.Error("Agent2 should exist in DB")
	}
	if agent2.Status != AgentStatusStopped {
		t.Errorf("Agent2 should be stopped, got '%s'", agent2.Status)
	}
}

func TestTeamManager_WorkingState(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	// Create team and agent
	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	tm.CreateAgent(team.ID, "TestAgent", "Role", "", "", "", nil, AgentLevelSpecialist, "testagent")

	// Initially no working state
	state, err := tm.GetAgentWorkingState("testagent")
	if err != nil {
		t.Fatalf("GetAgentWorkingState failed: %v", err)
	}
	if state != nil {
		t.Error("Expected nil state initially")
	}

	// Save working state
	newState := &AgentWorkingState{
		AgentID:       "testagent",
		TeamID:        team.ID,
		CurrentTaskID: "task123",
		Status:        "working",
		NextSteps:     "1. Implement feature X\n2. Write tests",
		Context:       "Working on user authentication",
	}
	err = tm.SaveAgentWorkingState(newState)
	if err != nil {
		t.Fatalf("SaveAgentWorkingState failed: %v", err)
	}

	// Retrieve working state
	loaded, err := tm.GetAgentWorkingState("testagent")
	if err != nil {
		t.Fatalf("GetAgentWorkingState failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Expected non-nil state")
	}
	if loaded.CurrentTaskID != "task123" {
		t.Errorf("Expected task 'task123', got '%s'", loaded.CurrentTaskID)
	}
	if loaded.Status != "working" {
		t.Errorf("Expected status 'working', got '%s'", loaded.Status)
	}
	if !strings.Contains(loaded.NextSteps, "Implement feature X") {
		t.Errorf("Expected NextSteps to contain task info, got '%s'", loaded.NextSteps)
	}
}

func TestTeamManager_ClearWorkingState(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	// Create team and agent
	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	tm.CreateAgent(team.ID, "TestAgent", "Role", "", "", "", nil, AgentLevelSpecialist, "testagent")

	// Save working state
	state := &AgentWorkingState{
		AgentID:       "testagent",
		TeamID:        team.ID,
		CurrentTaskID: "task123",
		Status:        "working",
		NextSteps:     "Do something",
	}
	tm.SaveAgentWorkingState(state)

	// Clear working state
	err := tm.ClearAgentWorkingState("testagent")
	if err != nil {
		t.Fatalf("ClearAgentWorkingState failed: %v", err)
	}

	// Verify cleared
	loaded, _ := tm.GetAgentWorkingState("testagent")
	if loaded != nil && loaded.Status != "idle" {
		t.Errorf("Expected idle or nil state after clear, got '%v'", loaded)
	}
}

func TestTeamManager_HeartbeatWithWorkingState(t *testing.T) {
	db := testTeamManagerDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)

	// Create team and agent
	team, _ := tm.CreateTeam("Test Team", "Desc", "user1", "")
	tm.CreateAgent(team.ID, "Jarvis", "Squad Lead", "", "", "", nil, AgentLevelSpecialist, "jarvis")

	// Save working state
	state := &AgentWorkingState{
		AgentID:       "jarvis",
		TeamID:        team.ID,
		CurrentTaskID: "task456",
		Status:        "working",
		NextSteps:     "1. Complete API\n2. Run tests",
		Context:       "Building REST API",
	}
	tm.SaveAgentWorkingState(state)

	// Get heartbeat prompt
	agent, _ := tm.GetAgent("jarvis")
	prompt := tm.buildHeartbeatPrompt(agent)

	// Verify working state is included
	if !strings.Contains(prompt, "Current Work State") {
		t.Error("Heartbeat prompt should contain 'Current Work State' section")
	}
	if !strings.Contains(prompt, "task456") {
		t.Error("Heartbeat prompt should contain current task ID")
	}
	if !strings.Contains(prompt, "Complete API") {
		t.Error("Heartbeat prompt should contain next steps")
	}
}

// Benchmark
func BenchmarkTeamManager_CreateAgent(b *testing.B) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	// Create schema
	db.Exec(`CREATE TABLE teams (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT DEFAULT '',
		owner_jid TEXT NOT NULL, default_model TEXT DEFAULT '', workspace_path TEXT DEFAULT '',
		created_at TEXT NOT NULL, enabled INTEGER DEFAULT 1)`)
	db.Exec(`CREATE TABLE persistent_agents (id TEXT PRIMARY KEY, name TEXT NOT NULL, role TEXT NOT NULL,
		team_id TEXT NOT NULL, level TEXT DEFAULT 'specialist', status TEXT DEFAULT 'idle',
		personality TEXT DEFAULT '', instructions TEXT DEFAULT '', model TEXT DEFAULT '',
		skills TEXT DEFAULT '[]', heartbeat_schedule TEXT DEFAULT '*/15 * * * *',
		created_at TEXT NOT NULL, last_active_at TEXT DEFAULT '', last_heartbeat_at TEXT DEFAULT '')`)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamManager(db, nil, logger)
	team, _ := tm.CreateTeam("Bench Team", "Desc", "user", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.CreateAgent(team.ID, "Agent", "Role", "", "", "", nil, AgentLevelSpecialist, "")
	}
}

// Package copilot – team_memory_test.go tests the team memory system.
package copilot

import (
	"database/sql"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// testTeamMemoryDB creates an in-memory SQLite database for testing.
func testTeamMemoryDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create tables
	schema := `
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
		team_id      TEXT NOT NULL DEFAULT '',
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

	CREATE TABLE IF NOT EXISTS team_thread_subscriptions (
		id            TEXT PRIMARY KEY,
		team_id       TEXT NOT NULL,
		thread_id     TEXT NOT NULL,
		agent_id      TEXT NOT NULL,
		subscribed_at TEXT NOT NULL,
		reason        TEXT DEFAULT 'auto',
		UNIQUE(team_id, thread_id, agent_id)
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

func TestTeamMemory_CreateTask(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create a task
	task, err := tm.CreateTask("Test Task", "Test description", "user123", []string{"agent1"})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	if task.ID == "" {
		t.Error("Task ID should not be empty")
	}
	if task.Title != "Test Task" {
		t.Errorf("Expected title 'Test Task', got '%s'", task.Title)
	}
	if task.Status != TaskStatusInbox {
		t.Errorf("Expected status 'inbox', got '%s'", task.Status)
	}
	if len(task.Assignees) != 1 || task.Assignees[0] != "agent1" {
		t.Errorf("Expected assignees ['agent1'], got %v", task.Assignees)
	}
}

func TestTeamMemory_GetTask(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create a task
	created, _ := tm.CreateTask("Test Task", "Test description", "user123", nil)

	// Get the task
	task, err := tm.GetTask(created.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	if task == nil {
		t.Fatal("Task should not be nil")
	}
	if task.Title != "Test Task" {
		t.Errorf("Expected title 'Test Task', got '%s'", task.Title)
	}

	// Get non-existent task
	notFound, err := tm.GetTask("nonexistent")
	if err != nil {
		t.Fatalf("GetTask for nonexistent should not error: %v", err)
	}
	if notFound != nil {
		t.Error("Nonexistent task should be nil")
	}
}

func TestTeamMemory_UpdateTask(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create a task
	task, _ := tm.CreateTask("Test Task", "Test description", "user123", nil)

	// Update status
	err := tm.UpdateTask(task.ID, TaskStatusProgress, "Starting work", "agent1")
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	// Verify update
	updated, _ := tm.GetTask(task.ID)
	if updated.Status != TaskStatusProgress {
		t.Errorf("Expected status 'in_progress', got '%s'", updated.Status)
	}

	// Update to done
	err = tm.UpdateTask(task.ID, TaskStatusDone, "Completed", "agent1")
	if err != nil {
		t.Fatalf("UpdateTask to done failed: %v", err)
	}

	done, _ := tm.GetTask(task.ID)
	if done.Status != TaskStatusDone {
		t.Errorf("Expected status 'done', got '%s'", done.Status)
	}
	if done.CompletedAt == nil {
		t.Error("CompletedAt should be set for done tasks")
	}
}

func TestTeamMemory_ListTasks(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create multiple tasks
	tm.CreateTask("Task 1", "Desc 1", "user", []string{"agent1"})
	tm.CreateTask("Task 2", "Desc 2", "user", []string{"agent2"})
	tm.CreateTask("Task 3", "Desc 3", "user", []string{"agent1", "agent2"})

	// List all tasks
	tasks, err := tm.ListTasks("", "")
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(tasks))
	}

	// Filter by assignee
	agent1Tasks, _ := tm.ListTasks("", "agent1")
	if len(agent1Tasks) != 2 {
		t.Errorf("Expected 2 tasks for agent1, got %d", len(agent1Tasks))
	}
}

func TestTeamMemory_AssignTask(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create unassigned task
	task, _ := tm.CreateTask("Test Task", "Desc", "user", nil)

	// Assign agents
	err := tm.AssignTask(task.ID, []string{"agent1", "agent2"}, "user")
	if err != nil {
		t.Fatalf("AssignTask failed: %v", err)
	}

	// Verify assignment
	updated, _ := tm.GetTask(task.ID)
	if len(updated.Assignees) != 2 {
		t.Errorf("Expected 2 assignees, got %d", len(updated.Assignees))
	}
	if updated.Status != TaskStatusAssigned {
		t.Errorf("Expected status 'assigned', got '%s'", updated.Status)
	}
}

func TestTeamMemory_DeleteTask(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create a task
	task, _ := tm.CreateTask("Task to Delete", "Desc", "user", nil)

	// Add a message to the task thread
	tm.PostMessage(task.ID, "agent1", "Comment on task", nil)

	// Verify task exists
	tasks, _ := tm.ListTasks("", "")
	if len(tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(tasks))
	}

	// Delete task
	err := tm.DeleteTask(task.ID)
	if err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	// Verify task is deleted
	deleted, _ := tm.GetTask(task.ID)
	if deleted != nil {
		t.Error("Deleted task should be nil")
	}

	// Verify task is removed from list
	tasksAfter, _ := tm.ListTasks("", "")
	if len(tasksAfter) != 0 {
		t.Errorf("Expected 0 tasks after delete, got %d", len(tasksAfter))
	}
}

func TestTeamMemory_PostMessage(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Post a message
	msg, err := tm.PostMessage("thread123", "agent1", "Hello @agent2, can you help?", []string{"agent2"})
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}

	if msg.ID == "" {
		t.Error("Message ID should not be empty")
	}
	if msg.FromAgent != "agent1" {
		t.Errorf("Expected from 'agent1', got '%s'", msg.FromAgent)
	}
	if len(msg.Mentions) != 1 || msg.Mentions[0] != "agent2" {
		t.Errorf("Expected mentions ['agent2'], got %v", msg.Mentions)
	}

	// Get thread messages
	messages, err := tm.GetThreadMessages("thread123", 10)
	if err != nil {
		t.Fatalf("GetThreadMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}
}

func TestTeamMemory_PendingMessages(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Post message with mentions (this adds pending messages)
	tm.PostMessage("thread1", "agent1", "Hey @agent2, check this", []string{"agent2"})
	tm.PostMessage("thread2", "agent1", "@agent2 also this", []string{"agent2"})

	// Get pending messages for agent2
	pending, err := tm.GetPendingMessages("agent2", false)
	if err != nil {
		t.Fatalf("GetPendingMessages failed: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("Expected 2 pending messages, got %d", len(pending))
	}

	// Get and mark delivered
	delivered, err := tm.GetPendingMessages("agent2", true)
	if err != nil {
		t.Fatalf("GetPendingMessages with mark failed: %v", err)
	}
	if len(delivered) != 2 {
		t.Errorf("Expected 2 delivered messages, got %d", len(delivered))
	}

	// Should be no more pending
	remaining, _ := tm.GetPendingMessages("agent2", false)
	if len(remaining) != 0 {
		t.Errorf("Expected 0 remaining messages, got %d", len(remaining))
	}
}

func TestTeamMemory_SaveAndGetFacts(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Save a fact
	err := tm.SaveFact("project_name", "DevClaw Teams", "agent1")
	if err != nil {
		t.Fatalf("SaveFact failed: %v", err)
	}

	// Get all facts
	facts, err := tm.GetFacts()
	if err != nil {
		t.Fatalf("GetFacts failed: %v", err)
	}
	if len(facts) != 1 {
		t.Errorf("Expected 1 fact, got %d", len(facts))
	}
	if facts[0].Key != "project_name" {
		t.Errorf("Expected key 'project_name', got '%s'", facts[0].Key)
	}
	if facts[0].Value != "DevClaw Teams" {
		t.Errorf("Expected value 'DevClaw Teams', got '%s'", facts[0].Value)
	}

	// Update existing fact
	err = tm.SaveFact("project_name", "DevClaw Teams v2", "agent2")
	if err != nil {
		t.Fatalf("SaveFact update failed: %v", err)
	}

	updated, _ := tm.GetFacts()
	if len(updated) != 1 {
		t.Errorf("Expected 1 fact after update, got %d", len(updated))
	}
	if updated[0].Value != "DevClaw Teams v2" {
		t.Errorf("Expected updated value, got '%s'", updated[0].Value)
	}
}

func TestTeamMemory_SearchFacts(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Save multiple facts
	tm.SaveFact("api_key", "secret123", "user")
	tm.SaveFact("api_url", "https://api.example.com", "user")
	tm.SaveFact("database_host", "localhost", "user")

	// Search for api
	results, err := tm.SearchFacts("api")
	if err != nil {
		t.Fatalf("SearchFacts failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'api', got %d", len(results))
	}

	// Search for host
	hostResults, _ := tm.SearchFacts("host")
	if len(hostResults) != 1 {
		t.Errorf("Expected 1 result for 'host', got %d", len(hostResults))
	}
}

func TestTeamMemory_DeleteFact(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Save and delete
	tm.SaveFact("temp", "temporary", "user")
	facts, _ := tm.GetFacts()
	if len(facts) != 1 {
		t.Errorf("Expected 1 fact, got %d", len(facts))
	}

	err := tm.DeleteFact("temp")
	if err != nil {
		t.Fatalf("DeleteFact failed: %v", err)
	}

	after, _ := tm.GetFacts()
	if len(after) != 0 {
		t.Errorf("Expected 0 facts after delete, got %d", len(after))
	}
}

func TestTeamMemory_GetActivities(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create task (logs activity)
	tm.CreateTask("Task 1", "Desc", "user", nil)

	// Get activities
	activities, err := tm.GetActivities(10)
	if err != nil {
		t.Fatalf("GetActivities failed: %v", err)
	}

	// Should have at least task_created activity
	if len(activities) < 1 {
		t.Error("Expected at least 1 activity")
	}

	var foundCreated bool
	for _, a := range activities {
		if a.Type == ActivityTaskCreated {
			foundCreated = true
			break
		}
	}
	if !foundCreated {
		t.Error("Expected to find task_created activity")
	}
}

func TestTeamMemory_BuildTeamContext(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Setup: create facts, tasks, and pending messages
	tm.SaveFact("project", "Test Project", "user")
	tm.CreateTask("Task for agent1", "Desc", "user", []string{"agent1"})
	tm.PostMessage("thread1", "agent2", "Hey @agent1", []string{"agent1"})

	// Build context for agent1
	context, err := tm.BuildTeamContext("agent1")
	if err != nil {
		t.Fatalf("BuildTeamContext failed: %v", err)
	}

	// Should contain all relevant info
	if !containsAll(context, "project", "Test Project", "Task for agent1", "Pending Messages", "Hey @agent1") {
		t.Errorf("Context missing expected content:\n%s", context)
	}
}

func TestTeamMemory_GenerateStandup(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create tasks in various states
	task1, _ := tm.CreateTask("Completed Task", "Desc", "user", nil)
	tm.UpdateTask(task1.ID, TaskStatusDone, "Done!", "agent1")

	task2, _ := tm.CreateTask("In Progress Task", "Desc", "user", []string{"agent2"})
	tm.UpdateTask(task2.ID, TaskStatusProgress, "Working on it", "agent2")

	task3, _ := tm.CreateTask("Blocked Task", "Desc", "user", nil)
	tm.UpdateTask(task3.ID, TaskStatusBlocked, "Waiting for X", "agent1")

	// Generate standup
	standup, err := tm.GenerateStandup()
	if err != nil {
		t.Fatalf("GenerateStandup failed: %v", err)
	}

	// Should contain all sections
	if !containsAll(standup, "DAILY STANDUP", "COMPLETED", "IN PROGRESS", "BLOCKED") {
		t.Errorf("Standup missing sections:\n%s", standup)
	}
}

func TestTeamMemory_DifferentTeams(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Two different teams
	tm1 := NewTeamMemory("team1", db, logger)
	tm2 := NewTeamMemory("team2", db, logger)

	// Create tasks in each team
	tm1.CreateTask("Team 1 Task", "Desc", "user", nil)
	tm2.CreateTask("Team 2 Task", "Desc", "user", nil)

	// Team 1 should only see its task
	t1Tasks, _ := tm1.ListTasks("", "")
	if len(t1Tasks) != 1 || t1Tasks[0].Title != "Team 1 Task" {
		t.Error("Team 1 should only see its own task")
	}

	// Team 2 should only see its task
	t2Tasks, _ := tm2.ListTasks("", "")
	if len(t2Tasks) != 1 || t2Tasks[0].Title != "Team 2 Task" {
		t.Error("Team 2 should only see its own task")
	}

	// Facts should be isolated too
	tm1.SaveFact("key", "value1", "user")
	tm2.SaveFact("key", "value2", "user")

	facts1, _ := tm1.GetFacts()
	facts2, _ := tm2.GetFacts()

	if facts1[0].Value != "value1" || facts2[0].Value != "value2" {
		t.Error("Facts should be isolated between teams")
	}
}

// Helper function
func containsAll(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if !strings.Contains(s, substr) {
			return false
		}
	}
	return true
}

// Benchmark
func BenchmarkTeamMemory_CreateTask(b *testing.B) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	// Create schema
	db.Exec(`CREATE TABLE team_tasks (
		id TEXT PRIMARY KEY, team_id TEXT NOT NULL, title TEXT NOT NULL,
		description TEXT DEFAULT '', status TEXT DEFAULT 'inbox',
		assignees TEXT DEFAULT '[]', created_by TEXT NOT NULL,
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	)`)
	db.Exec(`CREATE TABLE team_activities (
		id TEXT PRIMARY KEY, team_id TEXT NOT NULL, type TEXT NOT NULL,
		agent_id TEXT DEFAULT '', message TEXT NOT NULL,
		related_id TEXT DEFAULT '', created_at TEXT NOT NULL
	)`)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("bench-team", db, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.CreateTask("Benchmark Task", "Description", "user", nil)
	}
}

// Test with time-based operations
func TestTeamMemory_TimeFormats(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create task and verify time is parseable
	task, err := tm.CreateTask("Time Test", "Desc", "user", nil)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Verify timestamps are valid
	if task.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if task.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}

	// Verify we can round-trip through DB
	loaded, _ := tm.GetTask(task.ID)
	if loaded.CreatedAt.IsZero() {
		t.Error("Loaded CreatedAt should not be zero")
	}

	// Time difference should be minimal (within 1 second)
	diff := loaded.CreatedAt.Sub(task.CreatedAt)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("Time mismatch: original=%v, loaded=%v, diff=%v", task.CreatedAt, loaded.CreatedAt, diff)
	}
}

func TestTeamMemory_CreateDocument(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create a document
	doc, err := tm.CreateDocument("API Design Doc", DocumentTypeDeliverable, "# API Design\n\nContent here...", "markdown", "", "agent1")
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}

	if doc.ID == "" {
		t.Error("Document ID should not be empty")
	}
	if doc.Title != "API Design Doc" {
		t.Errorf("Expected title 'API Design Doc', got '%s'", doc.Title)
	}
	if doc.DocType != DocumentTypeDeliverable {
		t.Errorf("Expected doc_type 'deliverable', got '%s'", doc.DocType)
	}
	if doc.Author != "agent1" {
		t.Errorf("Expected author 'agent1', got '%s'", doc.Author)
	}
}

func TestTeamMemory_GetDocument(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create a document
	created, _ := tm.CreateDocument("Test Doc", DocumentTypeNotes, "Content", "markdown", "", "user")

	// Get the document
	doc, err := tm.GetDocument(created.ID)
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}

	if doc == nil {
		t.Fatal("Document should not be nil")
	}
	if doc.Title != "Test Doc" {
		t.Errorf("Expected title 'Test Doc', got '%s'", doc.Title)
	}

	// Get non-existent document
	notFound, err := tm.GetDocument("nonexistent")
	if err != nil {
		t.Fatalf("GetDocument for nonexistent should not error: %v", err)
	}
	if notFound != nil {
		t.Error("Nonexistent document should be nil")
	}
}

func TestTeamMemory_ListDocuments(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create multiple documents
	tm.CreateDocument("Doc 1", DocumentTypeDeliverable, "Content 1", "markdown", "task1", "agent1")
	tm.CreateDocument("Doc 2", DocumentTypeResearch, "Content 2", "markdown", "task1", "agent2")
	tm.CreateDocument("Doc 3", DocumentTypeDeliverable, "Content 3", "markdown", "task2", "agent1")

	// List all documents
	docs, err := tm.ListDocuments("", "")
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("Expected 3 documents, got %d", len(docs))
	}

	// Filter by task
	taskDocs, _ := tm.ListDocuments("task1", "")
	if len(taskDocs) != 2 {
		t.Errorf("Expected 2 documents for task1, got %d", len(taskDocs))
	}

	// Filter by type
	deliverables, _ := tm.ListDocuments("", DocumentTypeDeliverable)
	if len(deliverables) != 2 {
		t.Errorf("Expected 2 deliverables, got %d", len(deliverables))
	}
}

func TestTeamMemory_UpdateDocument(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create a document
	doc, _ := tm.CreateDocument("Test Doc", DocumentTypeNotes, "Original content", "markdown", "", "user")

	// Update document
	err := tm.UpdateDocument(doc.ID, "Updated content", "agent1")
	if err != nil {
		t.Fatalf("UpdateDocument failed: %v", err)
	}

	// Verify update
	updated, _ := tm.GetDocument(doc.ID)
	if updated.Content != "Updated content" {
		t.Errorf("Expected updated content, got '%s'", updated.Content)
	}
	if updated.Version != 2 {
		t.Errorf("Expected version 2, got %d", updated.Version)
	}
}

func TestTeamMemory_DeleteDocument(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Create and delete
	doc, _ := tm.CreateDocument("Temp Doc", DocumentTypeNotes, "Content", "markdown", "", "user")
	docs, _ := tm.ListDocuments("", "")
	if len(docs) != 1 {
		t.Errorf("Expected 1 document, got %d", len(docs))
	}

	err := tm.DeleteDocument(doc.ID)
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}

	after, _ := tm.ListDocuments("", "")
	if len(after) != 0 {
		t.Errorf("Expected 0 documents after delete, got %d", len(after))
	}
}

func TestTeamMemory_SubscribeToThread(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Subscribe to thread
	err := tm.SubscribeToThread("thread123", "agent1", SubscriptionAuto)
	if err != nil {
		t.Fatalf("SubscribeToThread failed: %v", err)
	}

	// Get subscribers
	subscribers, err := tm.GetThreadSubscribers("thread123")
	if err != nil {
		t.Fatalf("GetThreadSubscribers failed: %v", err)
	}
	if len(subscribers) != 1 || subscribers[0] != "agent1" {
		t.Errorf("Expected ['agent1'], got %v", subscribers)
	}
}

func TestTeamMemory_GetSubscribedThreads(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Subscribe agent to multiple threads
	tm.SubscribeToThread("thread1", "agent1", SubscriptionAuto)
	tm.SubscribeToThread("thread2", "agent1", SubscriptionMentioned)
	tm.SubscribeToThread("thread1", "agent2", SubscriptionAuto)

	// Get threads for agent1
	threads, err := tm.GetSubscribedThreads("agent1")
	if err != nil {
		t.Fatalf("GetSubscribedThreads failed: %v", err)
	}
	if len(threads) != 2 {
		t.Errorf("Expected 2 subscribed threads, got %d", len(threads))
	}
}

func TestTeamMemory_NotifySubscribers(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Subscribe agents to thread
	tm.SubscribeToThread("thread123", "agent1", SubscriptionAuto)
	tm.SubscribeToThread("thread123", "agent2", SubscriptionMentioned)

	// Notify subscribers
	err := tm.NotifySubscribers("thread123", "user", "New message in thread", []string{"user"})
	if err != nil {
		t.Fatalf("NotifySubscribers failed: %v", err)
	}

	// Check pending messages for subscribers
	pending1, _ := tm.GetPendingMessages("agent1", false)
	if len(pending1) != 1 {
		t.Errorf("Expected 1 pending message for agent1, got %d", len(pending1))
	}

	pending2, _ := tm.GetPendingMessages("agent2", false)
	if len(pending2) != 1 {
		t.Errorf("Expected 1 pending message for agent2, got %d", len(pending2))
	}
}

func TestTeamMemory_PostMessageWithSubscriptions(t *testing.T) {
	db := testTeamMemoryDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTeamMemory("test-team", db, logger)

	// Post message - should auto-subscribe sender and mentioned agents
	msg, err := tm.PostMessage("thread456", "agent1", "Hello @agent2, can you help?", []string{"agent2"})
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}

	// Verify sender auto-subscribed
	subscribers, _ := tm.GetThreadSubscribers("thread456")
	if !containsAll(strings.Join(subscribers, ","), "agent1", "agent2") {
		t.Errorf("Expected agent1 and agent2 to be subscribed, got %v", subscribers)
	}

	// Verify mentioned agent got pending message
	pending, _ := tm.GetPendingMessages("agent2", false)
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending message for agent2, got %d", len(pending))
	}
	if pending[0].ThreadID != "thread456" {
		t.Errorf("Expected thread456, got %s", pending[0].ThreadID)
	}

	_ = msg // Use msg to avoid unused variable warning
}


// Package copilot – db.go provides the central SQLite database for DevClaw.
// A single devclaw.db file holds scheduler jobs, session history/meta/facts,
// and the audit log. The memory.db (FTS5/embeddings) and whatsapp.db
// (whatsmeow session) remain as separate databases.
package copilot

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // SQLite driver.
)

// schema is the DDL executed on every startup (idempotent via IF NOT EXISTS).
const schema = `
-- Scheduler jobs
CREATE TABLE IF NOT EXISTS jobs (
    id          TEXT PRIMARY KEY,
    schedule    TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'cron',
    command     TEXT NOT NULL,
    channel     TEXT DEFAULT '',
    chat_id     TEXT DEFAULT '',
    enabled     INTEGER DEFAULT 1,
    created_by  TEXT DEFAULT '',
    created_at  TEXT NOT NULL,
    last_run_at TEXT,
    last_error  TEXT DEFAULT '',
    run_count   INTEGER DEFAULT 0
);

-- Session conversation entries (append-only, one row per exchange).
CREATE TABLE IF NOT EXISTS session_entries (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id         TEXT NOT NULL,
    user_message       TEXT NOT NULL,
    assistant_response TEXT NOT NULL,
    created_at         TEXT NOT NULL,
    meta               TEXT DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_session_entries_sid ON session_entries(session_id);

-- Session metadata (one row per session).
CREATE TABLE IF NOT EXISTS session_meta (
    session_id    TEXT PRIMARY KEY,
    channel       TEXT DEFAULT '',
    chat_id       TEXT DEFAULT '',
    config        TEXT DEFAULT '{}',
    active_skills TEXT DEFAULT '[]',
    updated_at    TEXT NOT NULL
);

-- Session long-term facts.
CREATE TABLE IF NOT EXISTS session_facts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    fact       TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_session_facts_sid ON session_facts(session_id);

-- Active agent runs (for restart recovery).
-- When a run starts, a row is inserted; when it completes, the row is deleted.
-- On startup, any remaining rows indicate runs that were interrupted by a restart.
CREATE TABLE IF NOT EXISTS active_runs (
    session_id   TEXT PRIMARY KEY,
    channel      TEXT NOT NULL,
    chat_id      TEXT NOT NULL,
    user_message TEXT NOT NULL,
    started_at   TEXT NOT NULL
);

-- Tool execution audit log.
CREATE TABLE IF NOT EXISTS audit_log (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    tool           TEXT NOT NULL,
    caller         TEXT DEFAULT '',
    level          TEXT DEFAULT '',
    allowed        INTEGER NOT NULL,
    args_summary   TEXT DEFAULT '',
    result_summary TEXT DEFAULT '',
    created_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at);

-- Subagent runs (persisted for restart recovery and history lookup).
CREATE TABLE IF NOT EXISTS subagent_runs (
    id                TEXT PRIMARY KEY,
    label             TEXT NOT NULL,
    task              TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'running',
    result            TEXT DEFAULT '',
    error             TEXT DEFAULT '',
    model             TEXT DEFAULT '',
    parent_session_id TEXT DEFAULT '',
    tokens_used       INTEGER DEFAULT 0,
    started_at        TEXT NOT NULL,
    completed_at      TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_subagent_runs_parent ON subagent_runs(parent_session_id);
CREATE INDEX IF NOT EXISTS idx_subagent_runs_status ON subagent_runs(status);

-- System state (maintenance mode, etc.)
CREATE TABLE IF NOT EXISTS system_state (
    key       TEXT PRIMARY KEY,
    value     TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Pairing tokens (shareable invite tokens for DM access).
CREATE TABLE IF NOT EXISTS pairing_tokens (
    id           TEXT PRIMARY KEY,
    token        TEXT NOT NULL UNIQUE,
    role         TEXT NOT NULL DEFAULT 'user',
    max_uses     INTEGER DEFAULT 0,
    use_count    INTEGER DEFAULT 0,
    auto_approve INTEGER DEFAULT 0,
    workspace_id TEXT DEFAULT '',
    note         TEXT DEFAULT '',
    created_by   TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    expires_at   TEXT DEFAULT '',
    revoked      INTEGER DEFAULT 0,
    revoked_at   TEXT DEFAULT '',
    revoked_by   TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pairing_tokens_token ON pairing_tokens(token);
CREATE INDEX IF NOT EXISTS idx_pairing_tokens_created ON pairing_tokens(created_at);

-- Pairing requests (pending access requests awaiting admin approval).
CREATE TABLE IF NOT EXISTS pairing_requests (
    id           TEXT PRIMARY KEY,
    token_id     TEXT NOT NULL,
    user_jid     TEXT NOT NULL,
    user_name    TEXT DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending',
    reviewed_by  TEXT DEFAULT '',
    reviewed_at  TEXT DEFAULT '',
    created_at   TEXT NOT NULL,
    FOREIGN KEY (token_id) REFERENCES pairing_tokens(id)
);
CREATE INDEX IF NOT EXISTS idx_pairing_requests_status ON pairing_requests(status);
CREATE INDEX IF NOT EXISTS idx_pairing_requests_user ON pairing_requests(user_jid);

-- ═══════════════════════════════════════════════════════════════════
-- TEAM MANAGEMENT SYSTEM
-- ═══════════════════════════════════════════════════════════════════

-- Teams
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

-- Persistent agents (long-lived agents with specific roles)
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
    last_heartbeat_at  TEXT DEFAULT '',
    FOREIGN KEY (team_id) REFERENCES teams(id)
);
CREATE INDEX IF NOT EXISTS idx_agents_team ON persistent_agents(team_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON persistent_agents(status);

-- Team tasks
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
    blocked_reason TEXT DEFAULT '',
    FOREIGN KEY (team_id) REFERENCES teams(id)
);
CREATE INDEX IF NOT EXISTS idx_tasks_team ON team_tasks(team_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON team_tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_assignees ON team_tasks(assignees);

-- Team messages (task threads and discussions)
CREATE TABLE IF NOT EXISTS team_messages (
    id         TEXT PRIMARY KEY,
    team_id    TEXT NOT NULL,
    thread_id  TEXT DEFAULT '',
    from_agent TEXT DEFAULT '',
    from_user  TEXT DEFAULT '',
    content    TEXT NOT NULL,
    mentions   TEXT DEFAULT '[]',
    created_at TEXT NOT NULL,
    delivered  INTEGER DEFAULT 0,
    FOREIGN KEY (team_id) REFERENCES teams(id)
);
CREATE INDEX IF NOT EXISTS idx_messages_team ON team_messages(team_id);
CREATE INDEX IF NOT EXISTS idx_messages_thread ON team_messages(thread_id);

-- Team pending messages (mailbox for @mentions)
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
CREATE INDEX IF NOT EXISTS idx_pending_to ON team_pending_messages(to_agent);
CREATE INDEX IF NOT EXISTS idx_pending_delivered ON team_pending_messages(delivered);

-- Team facts (shared memory)
CREATE TABLE IF NOT EXISTS team_facts (
    id         TEXT PRIMARY KEY,
    team_id    TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    author     TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (team_id) REFERENCES teams(id),
    UNIQUE(team_id, key)
);
CREATE INDEX IF NOT EXISTS idx_facts_team ON team_facts(team_id);

-- Team activity feed
CREATE TABLE IF NOT EXISTS team_activities (
    id         TEXT PRIMARY KEY,
    team_id    TEXT NOT NULL,
    type       TEXT NOT NULL,
    agent_id   TEXT DEFAULT '',
    message    TEXT NOT NULL,
    related_id TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    FOREIGN KEY (team_id) REFERENCES teams(id)
);
CREATE INDEX IF NOT EXISTS idx_activities_team ON team_activities(team_id);
CREATE INDEX IF NOT EXISTS idx_activities_created ON team_activities(created_at);

-- Team documents (deliverables linked to tasks)
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
    updated_at  TEXT NOT NULL,
    FOREIGN KEY (team_id) REFERENCES teams(id)
);
CREATE INDEX IF NOT EXISTS idx_documents_team ON team_documents(team_id);
CREATE INDEX IF NOT EXISTS idx_documents_task ON team_documents(task_id);
CREATE INDEX IF NOT EXISTS idx_documents_type ON team_documents(doc_type);

-- Thread subscriptions (agents subscribed to task threads)
CREATE TABLE IF NOT EXISTS team_thread_subscriptions (
    id            TEXT PRIMARY KEY,
    team_id       TEXT NOT NULL,
    thread_id     TEXT NOT NULL,
    agent_id      TEXT NOT NULL,
    subscribed_at TEXT NOT NULL,
    reason        TEXT DEFAULT 'auto',
    UNIQUE(team_id, thread_id, agent_id)
);
CREATE INDEX IF NOT EXISTS idx_subscriptions_thread ON team_thread_subscriptions(thread_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_agent ON team_thread_subscriptions(agent_id);

-- Agent working state (WORKING.md pattern)
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

// OpenDatabase opens (or creates) the central devclaw.db at the given path.
// It enables WAL mode for concurrent read performance and creates all tables.
func OpenDatabase(path string) (*sql.DB, error) {
	if path == "" {
		path = "./data/devclaw.db"
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create database directory %q: %w", dir, err)
	}

	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}

	// Verify connectivity.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Create schema (idempotent).
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return db, nil
}

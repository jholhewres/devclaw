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
    announce    INTEGER DEFAULT 1,
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
-- LCM (LOSSLESS COMPACTION MODULE) — DAG-BASED MEMORY
-- ═══════════════════════════════════════════════════════════════════

-- LCM conversations (one per session)
CREATE TABLE IF NOT EXISTS lcm_conversations (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL UNIQUE,
    created_at      TEXT NOT NULL,
    next_seq        INTEGER DEFAULT 1,
    last_compact_at TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_lcm_conv_sid ON lcm_conversations(session_id);

-- LCM messages (every message persisted verbatim for lossless recall)
CREATE TABLE IF NOT EXISTS lcm_messages (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id TEXT NOT NULL,
    seq             INTEGER NOT NULL,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL,
    token_count     INTEGER DEFAULT 0,
    created_at      TEXT NOT NULL,
    FOREIGN KEY (conversation_id) REFERENCES lcm_conversations(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_lcm_msg_conv_seq ON lcm_messages(conversation_id, seq);

-- LCM summaries (DAG nodes: leaf = summarized messages, condensed = summarized summaries)
CREATE TABLE IF NOT EXISTS lcm_summaries (
    id                          TEXT PRIMARY KEY,
    conversation_id             TEXT NOT NULL,
    kind                        TEXT NOT NULL,
    depth                       INTEGER NOT NULL,
    content                     TEXT NOT NULL,
    token_count                 INTEGER DEFAULT 0,
    source_message_token_count  INTEGER DEFAULT 0,
    descendant_count            INTEGER DEFAULT 0,
    descendant_token_count      INTEGER DEFAULT 0,
    earliest_at                 TEXT NOT NULL,
    latest_at                   TEXT NOT NULL,
    created_at                  TEXT NOT NULL,
    FOREIGN KEY (conversation_id) REFERENCES lcm_conversations(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_lcm_sum_conv ON lcm_summaries(conversation_id);
CREATE INDEX IF NOT EXISTS idx_lcm_sum_depth ON lcm_summaries(conversation_id, depth);

-- LCM summary-to-message links (leaf summaries → source messages)
CREATE TABLE IF NOT EXISTS lcm_summary_messages (
    summary_id  TEXT NOT NULL,
    message_id  INTEGER NOT NULL,
    PRIMARY KEY (summary_id, message_id),
    FOREIGN KEY (summary_id) REFERENCES lcm_summaries(id) ON DELETE CASCADE,
    FOREIGN KEY (message_id) REFERENCES lcm_messages(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_lcm_sm_msg ON lcm_summary_messages(message_id);

-- LCM summary-to-summary links (condensed → children)
CREATE TABLE IF NOT EXISTS lcm_summary_parents (
    parent_id TEXT NOT NULL,
    child_id  TEXT NOT NULL,
    PRIMARY KEY (parent_id, child_id),
    FOREIGN KEY (parent_id) REFERENCES lcm_summaries(id) ON DELETE CASCADE,
    FOREIGN KEY (child_id) REFERENCES lcm_summaries(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_lcm_sp_child ON lcm_summary_parents(child_id);

-- LCM context items (ordered list of what the model sees: summaries + fresh messages)
CREATE TABLE IF NOT EXISTS lcm_context_items (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id TEXT NOT NULL,
    ordinal         INTEGER NOT NULL,
    item_type       TEXT NOT NULL,
    message_id      INTEGER,
    summary_id      TEXT,
    FOREIGN KEY (conversation_id) REFERENCES lcm_conversations(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_lcm_ci_conv ON lcm_context_items(conversation_id, ordinal);

CREATE TABLE IF NOT EXISTS lcm_files (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    original_tokens INTEGER NOT NULL,
    original_chars  INTEGER NOT NULL,
    summary         TEXT NOT NULL,
    file_path       TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    FOREIGN KEY (conversation_id) REFERENCES lcm_conversations(id) ON DELETE CASCADE
);

`

// lcmFTSSchema is created separately because FTS5 may not be available in all
// SQLite builds (e.g. test environments). The LCM degrades gracefully without it.
const lcmFTSSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS lcm_fts USING fts5(
    content,
    entity_type UNINDEXED,
    entity_id UNINDEXED,
    conversation_id UNINDEXED,
    tokenize = 'porter unicode61'
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

	// LCM table migrations: executed individually to ensure they're created on
	// existing databases where the multi-statement schema exec may not reach them.
	// Each uses CREATE TABLE/INDEX IF NOT EXISTS so they're idempotent.
	lcmMigrations := []string{
		`CREATE TABLE IF NOT EXISTS lcm_conversations (
			id              TEXT PRIMARY KEY,
			session_id      TEXT NOT NULL UNIQUE,
			created_at      TEXT NOT NULL,
			next_seq        INTEGER DEFAULT 1,
			last_compact_at TEXT DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_lcm_conv_sid ON lcm_conversations(session_id)`,
		`CREATE TABLE IF NOT EXISTS lcm_messages (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id TEXT NOT NULL,
			seq             INTEGER NOT NULL,
			role            TEXT NOT NULL,
			content         TEXT NOT NULL,
			token_count     INTEGER DEFAULT 0,
			created_at      TEXT NOT NULL,
			FOREIGN KEY (conversation_id) REFERENCES lcm_conversations(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_lcm_msg_conv_seq ON lcm_messages(conversation_id, seq)`,
		`CREATE TABLE IF NOT EXISTS lcm_summaries (
			id                          TEXT PRIMARY KEY,
			conversation_id             TEXT NOT NULL,
			kind                        TEXT NOT NULL,
			depth                       INTEGER NOT NULL,
			content                     TEXT NOT NULL,
			token_count                 INTEGER DEFAULT 0,
			source_message_token_count  INTEGER DEFAULT 0,
			descendant_count            INTEGER DEFAULT 0,
			descendant_token_count      INTEGER DEFAULT 0,
			earliest_at                 TEXT NOT NULL,
			latest_at                   TEXT NOT NULL,
			created_at                  TEXT NOT NULL,
			FOREIGN KEY (conversation_id) REFERENCES lcm_conversations(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_lcm_sum_conv ON lcm_summaries(conversation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_lcm_sum_depth ON lcm_summaries(conversation_id, depth)`,
		`CREATE TABLE IF NOT EXISTS lcm_summary_messages (
			summary_id  TEXT NOT NULL,
			message_id  INTEGER NOT NULL,
			PRIMARY KEY (summary_id, message_id),
			FOREIGN KEY (summary_id) REFERENCES lcm_summaries(id) ON DELETE CASCADE,
			FOREIGN KEY (message_id) REFERENCES lcm_messages(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_lcm_sm_msg ON lcm_summary_messages(message_id)`,
		`CREATE TABLE IF NOT EXISTS lcm_summary_parents (
			parent_id TEXT NOT NULL,
			child_id  TEXT NOT NULL,
			PRIMARY KEY (parent_id, child_id),
			FOREIGN KEY (parent_id) REFERENCES lcm_summaries(id) ON DELETE CASCADE,
			FOREIGN KEY (child_id) REFERENCES lcm_summaries(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_lcm_sp_child ON lcm_summary_parents(child_id)`,
		`CREATE TABLE IF NOT EXISTS lcm_context_items (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id TEXT NOT NULL,
			ordinal         INTEGER NOT NULL,
			item_type       TEXT NOT NULL,
			message_id      INTEGER,
			summary_id      TEXT,
			FOREIGN KEY (conversation_id) REFERENCES lcm_conversations(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_lcm_ci_conv ON lcm_context_items(conversation_id, ordinal)`,
		`CREATE TABLE IF NOT EXISTS lcm_files (
			id              TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			original_tokens INTEGER NOT NULL,
			original_chars  INTEGER NOT NULL,
			summary         TEXT NOT NULL,
			file_path       TEXT NOT NULL,
			created_at      TEXT NOT NULL,
			FOREIGN KEY (conversation_id) REFERENCES lcm_conversations(id) ON DELETE CASCADE
		)`,
	}
	for _, m := range lcmMigrations {
		if _, err := db.Exec(m); err != nil {
			db.Close()
			return nil, fmt.Errorf("lcm migration: %w", err)
		}
	}

	// LCM FTS5 table: best-effort, degrades gracefully if FTS5 is unavailable.
	_, _ = db.Exec(lcmFTSSchema)

	// Column migrations: add new columns to existing tables without dropping data.
	// Each ALTER is best-effort; errors are ignored because SQLite returns an error
	// if the column already exists (no IF NOT EXISTS support for ALTER TABLE ADD COLUMN).
	columnMigrations := []string{
		`ALTER TABLE jobs ADD COLUMN announce INTEGER DEFAULT 1`,
		`ALTER TABLE subagent_runs ADD COLUMN retry_count INTEGER DEFAULT 0`,
	}
	for _, m := range columnMigrations {
		_, _ = db.Exec(m) // ignore "duplicate column" errors
	}

	return db, nil
}

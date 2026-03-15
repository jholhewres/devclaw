// Package copilot – lcm_store.go provides SQLite CRUD operations for the
// Lossless Compaction Module (LCM). All message and summary data lives in
// the central devclaw.db database.
package copilot

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ── Structs ──────────────────────────────────────────────────────────────────

// LCMConversation is one LCM conversation tied to a session.
type LCMConversation struct {
	ID            string
	SessionID     string
	CreatedAt     time.Time
	NextSeq       int
	LastCompactAt time.Time
}

// LCMMessage is a single persisted message (verbatim).
type LCMMessage struct {
	ID             int64
	ConversationID string
	Seq            int
	Role           string
	Content        string
	TokenCount     int
	CreatedAt      time.Time
}

// LCMSummary is a DAG node — leaf (depth 0) or condensed (depth 1+).
type LCMSummary struct {
	ID                      string
	ConversationID          string
	Kind                    string // "leaf" | "condensed"
	Depth                   int
	Content                 string
	TokenCount              int
	SourceMessageTokenCount int
	DescendantCount         int
	DescendantTokenCount    int
	EarliestAt              time.Time
	LatestAt                time.Time
	CreatedAt               time.Time
}

// LCMContextItem is an ordered entry in the model's visible context.
type LCMContextItem struct {
	ID             int64
	ConversationID string
	Ordinal        int
	ItemType       string // "message" | "summary"
	MessageID      *int64
	SummaryID      *string
}

// LCMFTSResult is a full-text search hit.
type LCMFTSResult struct {
	EntityType string // "message" | "summary"
	EntityID   string
	Content    string
	Rank       float64
}

// ── Store ────────────────────────────────────────────────────────────────────

// LCMStore provides all LCM database operations.
type LCMStore struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewLCMStore creates a new LCM store.
func NewLCMStore(db *sql.DB, logger *slog.Logger) *LCMStore {
	return &LCMStore{db: db, logger: logger}
}

// ── Conversations ────────────────────────────────────────────────────────────

// GetOrCreateConversation returns the LCM conversation for a session, creating
// it if it doesn't exist (idempotent).
func (s *LCMStore) GetOrCreateConversation(sessionID string) (*LCMConversation, error) {
	return s.getOrCreateConversation(sessionID, 0)
}

func (s *LCMStore) getOrCreateConversation(sessionID string, attempt int) (*LCMConversation, error) {
	var conv LCMConversation
	var createdAt, lastCompactAt string
	err := s.db.QueryRow(
		`SELECT id, session_id, created_at, next_seq, last_compact_at FROM lcm_conversations WHERE session_id = ?`,
		sessionID,
	).Scan(&conv.ID, &conv.SessionID, &createdAt, &conv.NextSeq, &lastCompactAt)
	if err == nil {
		conv.CreatedAt = parseTimeStr(createdAt)
		conv.LastCompactAt = parseTimeStr(lastCompactAt)
		return &conv, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("lcm get conversation: %w", err)
	}

	conv = LCMConversation{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		CreatedAt: time.Now().UTC(),
		NextSeq:   1,
	}
	_, err = s.db.Exec(
		`INSERT INTO lcm_conversations (id, session_id, created_at, next_seq, last_compact_at) VALUES (?, ?, ?, ?, '')`,
		conv.ID, conv.SessionID, conv.CreatedAt.Format(time.RFC3339), conv.NextSeq,
	)
	if err != nil {
		// Race: another goroutine may have inserted first — retry read (max 1 retry).
		if strings.Contains(err.Error(), "UNIQUE") && attempt < 1 {
			return s.getOrCreateConversation(sessionID, attempt+1)
		}
		return nil, fmt.Errorf("lcm create conversation: %w", err)
	}
	return &conv, nil
}

// NextSeq atomically increments and returns the next sequence number.
func (s *LCMStore) NextSeq(convID string) (int, error) {
	var seq int
	err := s.db.QueryRow(
		`UPDATE lcm_conversations SET next_seq = next_seq + 1 WHERE id = ? RETURNING next_seq - 1`,
		convID,
	).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("lcm next seq: %w", err)
	}
	return seq, nil
}

// UpdateLastCompactAt updates the last compaction timestamp.
func (s *LCMStore) UpdateLastCompactAt(convID string) error {
	_, err := s.db.Exec(
		`UPDATE lcm_conversations SET last_compact_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), convID,
	)
	return err
}

// ── Messages ─────────────────────────────────────────────────────────────────

// IngestMessage inserts a new message, indexes it in FTS, and adds a context item.
func (s *LCMStore) IngestMessage(convID, role, content string, tokenCount int) (*LCMMessage, error) {
	seq, err := s.NextSeq(convID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO lcm_messages (conversation_id, seq, role, content, token_count, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		convID, seq, role, content, tokenCount, now,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm ingest message: %w", err)
	}
	id, _ := res.LastInsertId()

	// Index in FTS (best-effort: FTS5 may not be available).
	_, _ = s.db.Exec(
		`INSERT INTO lcm_fts (content, entity_type, entity_id, conversation_id) VALUES (?, 'message', ?, ?)`,
		content, strconv.FormatInt(id, 10), convID,
	)

	// Add context item.
	if _, ciErr := s.db.Exec(
		`INSERT INTO lcm_context_items (conversation_id, ordinal, item_type, message_id) VALUES (?, ?, 'message', ?)`,
		convID, seq, id,
	); ciErr != nil {
		s.logger.Warn("lcm: failed to insert context item", "message_id", id, "err", ciErr)
	}

	msg := &LCMMessage{
		ID:             id,
		ConversationID: convID,
		Seq:            seq,
		Role:           role,
		Content:        content,
		TokenCount:     tokenCount,
	}
	msg.CreatedAt, _ = time.Parse(time.RFC3339, now)
	return msg, nil
}

// GetMessage returns a single message by ID.
func (s *LCMStore) GetMessage(id int64) (*LCMMessage, error) {
	var m LCMMessage
	var createdAt string
	err := s.db.QueryRow(
		`SELECT id, conversation_id, seq, role, content, token_count, created_at FROM lcm_messages WHERE id = ?`, id,
	).Scan(&m.ID, &m.ConversationID, &m.Seq, &m.Role, &m.Content, &m.TokenCount, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("lcm get message: %w", err)
	}
	m.CreatedAt = parseTimeStr(createdAt)
	return &m, nil
}

// GetMessageRange returns messages within a seq range (inclusive).
func (s *LCMStore) GetMessageRange(convID string, fromSeq, toSeq int) ([]*LCMMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, conversation_id, seq, role, content, token_count, created_at
		 FROM lcm_messages WHERE conversation_id = ? AND seq >= ? AND seq <= ? ORDER BY seq`,
		convID, fromSeq, toSeq,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get message range: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// GetUnsummarizedMessages returns messages not yet linked to any summary,
// excluding the most recent excludeLastN messages (the "fresh tail").
func (s *LCMStore) GetUnsummarizedMessages(convID string, excludeLastN int) ([]*LCMMessage, error) {
	rows, err := s.db.Query(
		`SELECT m.id, m.conversation_id, m.seq, m.role, m.content, m.token_count, m.created_at
		 FROM lcm_messages m
		 WHERE m.conversation_id = ?
		   AND NOT EXISTS (SELECT 1 FROM lcm_summary_messages sm WHERE sm.message_id = m.id)
		   AND m.seq <= (SELECT MAX(seq) - ? FROM lcm_messages WHERE conversation_id = ?)
		 ORDER BY m.seq`,
		convID, excludeLastN, convID,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get unsummarized: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// CountUnsummarizedTokens returns the total token count of unsummarized messages,
// excluding the fresh tail.
func (s *LCMStore) CountUnsummarizedTokens(convID string, excludeLastN int) (int, error) {
	var total sql.NullInt64
	err := s.db.QueryRow(
		`SELECT COALESCE(SUM(m.token_count), 0)
		 FROM lcm_messages m
		 WHERE m.conversation_id = ?
		   AND NOT EXISTS (SELECT 1 FROM lcm_summary_messages sm WHERE sm.message_id = m.id)
		   AND m.seq <= (SELECT COALESCE(MAX(seq), 0) - ? FROM lcm_messages WHERE conversation_id = ?)`,
		convID, excludeLastN, convID,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("lcm count unsummarized: %w", err)
	}
	return int(total.Int64), nil
}

// GetFreshTailMessages returns the last N messages by seq (reversed to chronological order).
func (s *LCMStore) GetFreshTailMessages(convID string, count int) ([]*LCMMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, conversation_id, seq, role, content, token_count, created_at
		 FROM lcm_messages WHERE conversation_id = ?
		 ORDER BY seq DESC LIMIT ?`,
		convID, count,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get fresh tail: %w", err)
	}
	defer rows.Close()
	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	// Reverse to chronological order.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// GetRecentMessages returns the most recent N messages (by seq DESC, reversed to chronological).
// Use this instead of GetMessageRange for grep searches to prioritize recent messages.
func (s *LCMStore) GetRecentMessages(convID string, limit int) ([]*LCMMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, conversation_id, seq, role, content, token_count, created_at
		 FROM lcm_messages WHERE conversation_id = ?
		 ORDER BY seq DESC LIMIT ?`,
		convID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get recent messages: %w", err)
	}
	defer rows.Close()
	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	// Reverse to chronological order.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// MessageCount returns the total number of messages in a conversation.
func (s *LCMStore) MessageCount(convID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM lcm_messages WHERE conversation_id = ?`, convID).Scan(&count)
	return count, err
}

// ── Summaries ────────────────────────────────────────────────────────────────

// InsertSummary persists a summary node and indexes it in FTS.
func (s *LCMStore) InsertSummary(sum *LCMSummary) error {
	_, err := s.db.Exec(
		`INSERT INTO lcm_summaries (id, conversation_id, kind, depth, content, token_count,
		 source_message_token_count, descendant_count, descendant_token_count,
		 earliest_at, latest_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sum.ID, sum.ConversationID, sum.Kind, sum.Depth, sum.Content, sum.TokenCount,
		sum.SourceMessageTokenCount, sum.DescendantCount, sum.DescendantTokenCount,
		sum.EarliestAt.Format(time.RFC3339), sum.LatestAt.Format(time.RFC3339),
		sum.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("lcm insert summary: %w", err)
	}
	// FTS index.
	_, _ = s.db.Exec(
		`INSERT INTO lcm_fts (content, entity_type, entity_id, conversation_id) VALUES (?, 'summary', ?, ?)`,
		sum.Content, sum.ID, sum.ConversationID,
	)
	return nil
}

// LinkSummaryMessages links a leaf summary to its source messages.
func (s *LCMStore) LinkSummaryMessages(summaryID string, msgIDs []int64) error {
	if len(msgIDs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO lcm_summary_messages (summary_id, message_id) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, mid := range msgIDs {
		if _, err := stmt.Exec(summaryID, mid); err != nil {
			tx.Rollback()
			return fmt.Errorf("lcm link summary messages: %w", err)
		}
	}
	return tx.Commit()
}

// LinkSummaryChildren links a condensed summary to its child summaries.
func (s *LCMStore) LinkSummaryChildren(parentID string, childIDs []string) error {
	if len(childIDs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO lcm_summary_parents (parent_id, child_id) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, cid := range childIDs {
		if _, err := stmt.Exec(parentID, cid); err != nil {
			tx.Rollback()
			return fmt.Errorf("lcm link summary children: %w", err)
		}
	}
	return tx.Commit()
}

// GetSummary returns a single summary by ID.
func (s *LCMStore) GetSummary(id string) (*LCMSummary, error) {
	var sum LCMSummary
	var earliestAt, latestAt, createdAt string
	err := s.db.QueryRow(
		`SELECT id, conversation_id, kind, depth, content, token_count,
		 source_message_token_count, descendant_count, descendant_token_count,
		 earliest_at, latest_at, created_at
		 FROM lcm_summaries WHERE id = ?`, id,
	).Scan(&sum.ID, &sum.ConversationID, &sum.Kind, &sum.Depth, &sum.Content, &sum.TokenCount,
		&sum.SourceMessageTokenCount, &sum.DescendantCount, &sum.DescendantTokenCount,
		&earliestAt, &latestAt, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("lcm get summary %q: %w", id, err)
	}
	sum.EarliestAt = parseTimeStr(earliestAt)
	sum.LatestAt = parseTimeStr(latestAt)
	sum.CreatedAt = parseTimeStr(createdAt)
	return &sum, nil
}

// GetSummaryChildren returns child summaries of a condensed node.
func (s *LCMStore) GetSummaryChildren(summaryID string) ([]*LCMSummary, error) {
	rows, err := s.db.Query(
		`SELECT s.id, s.conversation_id, s.kind, s.depth, s.content, s.token_count,
		 s.source_message_token_count, s.descendant_count, s.descendant_token_count,
		 s.earliest_at, s.latest_at, s.created_at
		 FROM lcm_summaries s
		 JOIN lcm_summary_parents sp ON s.id = sp.child_id
		 WHERE sp.parent_id = ? ORDER BY s.earliest_at`, summaryID,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get summary children: %w", err)
	}
	defer rows.Close()
	return scanSummaries(rows)
}

// GetSummaryParents returns parent summaries that contain this node.
func (s *LCMStore) GetSummaryParents(summaryID string) ([]*LCMSummary, error) {
	rows, err := s.db.Query(
		`SELECT s.id, s.conversation_id, s.kind, s.depth, s.content, s.token_count,
		 s.source_message_token_count, s.descendant_count, s.descendant_token_count,
		 s.earliest_at, s.latest_at, s.created_at
		 FROM lcm_summaries s
		 JOIN lcm_summary_parents sp ON s.id = sp.parent_id
		 WHERE sp.child_id = ? ORDER BY s.earliest_at`, summaryID,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get summary parents: %w", err)
	}
	defer rows.Close()
	return scanSummaries(rows)
}

// GetSummaryMessages returns the source messages linked to a leaf summary.
func (s *LCMStore) GetSummaryMessages(summaryID string) ([]*LCMMessage, error) {
	rows, err := s.db.Query(
		`SELECT m.id, m.conversation_id, m.seq, m.role, m.content, m.token_count, m.created_at
		 FROM lcm_messages m
		 JOIN lcm_summary_messages sm ON m.id = sm.message_id
		 WHERE sm.summary_id = ? ORDER BY m.seq`, summaryID,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get summary messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// GetOrphanSummaries returns summaries at a given depth that have no parent.
func (s *LCMStore) GetOrphanSummaries(convID string, depth int) ([]*LCMSummary, error) {
	rows, err := s.db.Query(
		`SELECT s.id, s.conversation_id, s.kind, s.depth, s.content, s.token_count,
		 s.source_message_token_count, s.descendant_count, s.descendant_token_count,
		 s.earliest_at, s.latest_at, s.created_at
		 FROM lcm_summaries s
		 WHERE s.conversation_id = ? AND s.depth = ?
		   AND NOT EXISTS (SELECT 1 FROM lcm_summary_parents sp WHERE sp.child_id = s.id)
		 ORDER BY s.earliest_at`, convID, depth,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get orphan summaries: %w", err)
	}
	defer rows.Close()
	return scanSummaries(rows)
}

// GetRootSummaries returns top-level summaries (no parent), ordered by time.
func (s *LCMStore) GetRootSummaries(convID string) ([]*LCMSummary, error) {
	rows, err := s.db.Query(
		`SELECT s.id, s.conversation_id, s.kind, s.depth, s.content, s.token_count,
		 s.source_message_token_count, s.descendant_count, s.descendant_token_count,
		 s.earliest_at, s.latest_at, s.created_at
		 FROM lcm_summaries s
		 WHERE s.conversation_id = ?
		   AND NOT EXISTS (SELECT 1 FROM lcm_summary_parents sp WHERE sp.child_id = s.id)
		 ORDER BY s.earliest_at`, convID,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get root summaries: %w", err)
	}
	defer rows.Close()
	return scanSummaries(rows)
}

// GetAllSummaries returns all summaries for a conversation.
func (s *LCMStore) GetAllSummaries(convID string) ([]*LCMSummary, error) {
	rows, err := s.db.Query(
		`SELECT id, conversation_id, kind, depth, content, token_count,
		 source_message_token_count, descendant_count, descendant_token_count,
		 earliest_at, latest_at, created_at
		 FROM lcm_summaries WHERE conversation_id = ? ORDER BY earliest_at`, convID,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get all summaries: %w", err)
	}
	defer rows.Close()
	return scanSummaries(rows)
}

// GetMaxDepth returns the maximum summary depth for a conversation.
func (s *LCMStore) GetMaxDepth(convID string) (int, error) {
	var depth sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(depth) FROM lcm_summaries WHERE conversation_id = ?`, convID,
	).Scan(&depth)
	if err != nil {
		return 0, err
	}
	return int(depth.Int64), nil
}

// SummaryCount returns summary counts by kind.
func (s *LCMStore) SummaryCount(convID string) (leaf, condensed int, err error) {
	rows, err := s.db.Query(
		`SELECT kind, COUNT(*) FROM lcm_summaries WHERE conversation_id = ? GROUP BY kind`, convID,
	)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var cnt int
		if err := rows.Scan(&kind, &cnt); err != nil {
			return 0, 0, err
		}
		switch kind {
		case "leaf":
			leaf = cnt
		case "condensed":
			condensed = cnt
		}
	}
	return leaf, condensed, rows.Err()
}

// ── Context Items ────────────────────────────────────────────────────────────

// GetContextItems returns ordered context items for a conversation.
func (s *LCMStore) GetContextItems(convID string) ([]LCMContextItem, error) {
	rows, err := s.db.Query(
		`SELECT id, conversation_id, ordinal, item_type, message_id, summary_id
		 FROM lcm_context_items WHERE conversation_id = ? ORDER BY ordinal`, convID,
	)
	if err != nil {
		return nil, fmt.Errorf("lcm get context items: %w", err)
	}
	defer rows.Close()

	var items []LCMContextItem
	for rows.Next() {
		var ci LCMContextItem
		if err := rows.Scan(&ci.ID, &ci.ConversationID, &ci.Ordinal, &ci.ItemType, &ci.MessageID, &ci.SummaryID); err != nil {
			return nil, err
		}
		items = append(items, ci)
	}
	return items, rows.Err()
}

// ReplaceContextItems atomically replaces all context items for a conversation.
func (s *LCMStore) ReplaceContextItems(convID string, items []LCMContextItem) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM lcm_context_items WHERE conversation_id = ?`, convID); err != nil {
		tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare(
		`INSERT INTO lcm_context_items (conversation_id, ordinal, item_type, message_id, summary_id) VALUES (?, ?, ?, ?, ?)`,
	)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, ci := range items {
		if _, err := stmt.Exec(convID, ci.Ordinal, ci.ItemType, ci.MessageID, ci.SummaryID); err != nil {
			tx.Rollback()
			return fmt.Errorf("lcm replace context items: %w", err)
		}
	}
	return tx.Commit()
}

// ── FTS Search ───────────────────────────────────────────────────────────────

// SearchFTS performs a full-text search across messages and summaries.
// Returns nil results (not error) if FTS5 is unavailable.
func (s *LCMStore) SearchFTS(convID, query string, limit int) ([]LCMFTSResult, error) {
	if limit <= 0 {
		limit = 20
	}
	// Sanitize query for FTS5 MATCH syntax: wrap in double quotes for literal
	// matching, escaping any embedded double quotes to prevent FTS operator injection.
	safeQuery := "\"" + strings.ReplaceAll(query, "\"", "\"\"") + "\""
	rows, err := s.db.Query(
		`SELECT content, entity_type, entity_id, rank
		 FROM lcm_fts WHERE lcm_fts MATCH ? AND conversation_id = ?
		 ORDER BY rank LIMIT ?`,
		safeQuery, convID, limit,
	)
	if err != nil {
		// FTS5 table may not exist — degrade gracefully.
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such module") {
			return nil, nil
		}
		return nil, fmt.Errorf("lcm fts search: %w", err)
	}
	defer rows.Close()

	var results []LCMFTSResult
	for rows.Next() {
		var r LCMFTSResult
		if err := rows.Scan(&r.Content, &r.EntityType, &r.EntityID, &r.Rank); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// GenerateSummaryID creates a deterministic summary ID from content and timestamp.
func GenerateSummaryID(content string, ts time.Time) string {
	h := sha256.Sum256([]byte(content + ts.Format(time.RFC3339Nano)))
	return "sum_" + hex.EncodeToString(h[:8])
}

// EstimateTokens provides a rough token count estimate (len/4).
func EstimateTokens(s string) int {
	n := len(s) / 4
	if n == 0 && len(s) > 0 {
		n = 1
	}
	return n
}

func scanMessages(rows *sql.Rows) ([]*LCMMessage, error) {
	var msgs []*LCMMessage
	for rows.Next() {
		var m LCMMessage
		var createdAt string
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Seq, &m.Role, &m.Content, &m.TokenCount, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt = parseTimeStr(createdAt)
		msgs = append(msgs, &m)
	}
	return msgs, rows.Err()
}

func scanSummaries(rows *sql.Rows) ([]*LCMSummary, error) {
	var sums []*LCMSummary
	for rows.Next() {
		var sum LCMSummary
		var earliestAt, latestAt, createdAt string
		if err := rows.Scan(&sum.ID, &sum.ConversationID, &sum.Kind, &sum.Depth, &sum.Content, &sum.TokenCount,
			&sum.SourceMessageTokenCount, &sum.DescendantCount, &sum.DescendantTokenCount,
			&earliestAt, &latestAt, &createdAt); err != nil {
			return nil, err
		}
		sum.EarliestAt = parseTimeStr(earliestAt)
		sum.LatestAt = parseTimeStr(latestAt)
		sum.CreatedAt = parseTimeStr(createdAt)
		sums = append(sums, &sum)
	}
	return sums, rows.Err()
}

// parseTimeStr parses an RFC3339 time string from SQLite.
func parseTimeStr(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try without timezone.
		t, _ = time.Parse("2006-01-02 15:04:05", s)
	}
	return t
}

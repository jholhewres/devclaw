package copilot

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// setupTestLCMDB creates an in-memory SQLite database with the full schema.
func setupTestLCMDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// ── Store Tests ──────────────────────────────────────────────────────────────

func TestLCMStore_GetOrCreateConversation(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())

	// First call creates.
	conv, err := store.GetOrCreateConversation("sess-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if conv.SessionID != "sess-1" {
		t.Errorf("expected session_id=sess-1, got %s", conv.SessionID)
	}
	if conv.NextSeq != 1 {
		t.Errorf("expected next_seq=1, got %d", conv.NextSeq)
	}

	// Second call returns existing.
	conv2, err := store.GetOrCreateConversation("sess-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if conv2.ID != conv.ID {
		t.Errorf("expected same ID, got %s vs %s", conv.ID, conv2.ID)
	}
}

func TestLCMStore_IngestAndRetrieve(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-ingest")

	// Ingest 5 messages.
	for i := 0; i < 5; i++ {
		_, err := store.IngestMessage(conv.ID, "user", "message "+string(rune('A'+i)), 100)
		if err != nil {
			t.Fatalf("ingest %d: %v", i, err)
		}
	}

	// Get message count.
	count, err := store.MessageCount(conv.ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 messages, got %d", count)
	}

	// Get fresh tail (last 3).
	tail, err := store.GetFreshTailMessages(conv.ID, 3)
	if err != nil {
		t.Fatalf("fresh tail: %v", err)
	}
	if len(tail) != 3 {
		t.Errorf("expected 3 tail messages, got %d", len(tail))
	}
	// Should be in chronological order.
	if tail[0].Seq > tail[1].Seq {
		t.Errorf("tail not in chronological order: seq %d > %d", tail[0].Seq, tail[1].Seq)
	}
}

func TestLCMStore_UnsummarizedMessages(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-unsum")

	// Ingest 10 messages.
	var msgIDs []int64
	for i := 0; i < 10; i++ {
		msg, _ := store.IngestMessage(conv.ID, "user", "msg", 100)
		msgIDs = append(msgIDs, msg.ID)
	}

	// All 10 are unsummarized, excluding last 2 = 8.
	unsummarized, err := store.GetUnsummarizedMessages(conv.ID, 2)
	if err != nil {
		t.Fatalf("get unsummarized: %v", err)
	}
	if len(unsummarized) != 8 {
		t.Errorf("expected 8 unsummarized, got %d", len(unsummarized))
	}

	// Count tokens: 8 * 100 = 800.
	tokens, err := store.CountUnsummarizedTokens(conv.ID, 2)
	if err != nil {
		t.Fatalf("count unsummarized tokens: %v", err)
	}
	if tokens != 800 {
		t.Errorf("expected 800 tokens, got %d", tokens)
	}
}

func TestLCMStore_SummaryInsertAndLink(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-sum")

	// Ingest messages.
	msg1, _ := store.IngestMessage(conv.ID, "user", "hello", 10)
	msg2, _ := store.IngestMessage(conv.ID, "assistant", "hi there", 12)

	// Create leaf summary.
	now := time.Now().UTC()
	sum := &LCMSummary{
		ID:                      GenerateSummaryID("test summary", now),
		ConversationID:          conv.ID,
		Kind:                    "leaf",
		Depth:                   0,
		Content:                 "test summary",
		TokenCount:              3,
		SourceMessageTokenCount: 22,
		EarliestAt:              now.Add(-time.Hour),
		LatestAt:                now,
		CreatedAt:               now,
	}
	if err := store.InsertSummary(sum); err != nil {
		t.Fatalf("insert summary: %v", err)
	}

	// Link messages.
	if err := store.LinkSummaryMessages(sum.ID, []int64{msg1.ID, msg2.ID}); err != nil {
		t.Fatalf("link messages: %v", err)
	}

	// Retrieve summary.
	got, err := store.GetSummary(sum.ID)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if got.Kind != "leaf" || got.Depth != 0 {
		t.Errorf("unexpected kind=%s depth=%d", got.Kind, got.Depth)
	}

	// Get linked messages.
	msgs, err := store.GetSummaryMessages(sum.ID)
	if err != nil {
		t.Fatalf("get summary messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 linked messages, got %d", len(msgs))
	}

	// After linking, those messages should no longer be unsummarized.
	unsummarized, _ := store.GetUnsummarizedMessages(conv.ID, 0)
	if len(unsummarized) != 0 {
		t.Errorf("expected 0 unsummarized after linking, got %d", len(unsummarized))
	}
}

func TestLCMStore_CondensedSummaryLink(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-condensed")
	now := time.Now().UTC()

	// Create two leaf summaries.
	leaf1 := &LCMSummary{
		ID: "sum_leaf1", ConversationID: conv.ID, Kind: "leaf", Depth: 0,
		Content: "leaf 1", TokenCount: 10,
		EarliestAt: now.Add(-2 * time.Hour), LatestAt: now.Add(-time.Hour), CreatedAt: now,
	}
	leaf2 := &LCMSummary{
		ID: "sum_leaf2", ConversationID: conv.ID, Kind: "leaf", Depth: 0,
		Content: "leaf 2", TokenCount: 12,
		EarliestAt: now.Add(-time.Hour), LatestAt: now, CreatedAt: now,
	}
	store.InsertSummary(leaf1)
	store.InsertSummary(leaf2)

	// Create condensed parent.
	parent := &LCMSummary{
		ID: "sum_parent", ConversationID: conv.ID, Kind: "condensed", Depth: 1,
		Content: "condensed", TokenCount: 8, DescendantCount: 2, DescendantTokenCount: 22,
		EarliestAt: now.Add(-2 * time.Hour), LatestAt: now, CreatedAt: now,
	}
	store.InsertSummary(parent)
	store.LinkSummaryChildren("sum_parent", []string{"sum_leaf1", "sum_leaf2"})

	// Children lookup.
	children, err := store.GetSummaryChildren("sum_parent")
	if err != nil {
		t.Fatalf("get children: %v", err)
	}
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}

	// Parents lookup.
	parents, err := store.GetSummaryParents("sum_leaf1")
	if err != nil {
		t.Fatalf("get parents: %v", err)
	}
	if len(parents) != 1 || parents[0].ID != "sum_parent" {
		t.Errorf("unexpected parent: %v", parents)
	}

	// Orphan check: leaves should not be orphans (they have a parent).
	orphans, _ := store.GetOrphanSummaries(conv.ID, 0)
	if len(orphans) != 0 {
		t.Errorf("expected 0 orphan leaves, got %d", len(orphans))
	}

	// Root summaries: only the parent is a root.
	roots, _ := store.GetRootSummaries(conv.ID)
	if len(roots) != 1 || roots[0].ID != "sum_parent" {
		t.Errorf("expected 1 root (sum_parent), got %d", len(roots))
	}
}

func TestLCMStore_FTSSearch(t *testing.T) {
	db := setupTestLCMDB(t)

	// Check if FTS5 is available.
	_, ftsErr := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS _fts5_probe USING fts5(content)`)
	if ftsErr != nil {
		t.Skip("FTS5 not available in test SQLite build")
	}
	db.Exec(`DROP TABLE IF EXISTS _fts5_probe`)

	// Create FTS table manually since it's not in the main schema.
	db.Exec(lcmFTSSchema)

	store := NewLCMStore(db, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-fts")
	store.IngestMessage(conv.ID, "user", "configure PostgreSQL database", 100)
	store.IngestMessage(conv.ID, "assistant", "I will set up the MySQL connection", 100)

	results, err := store.SearchFTS(conv.ID, "PostgreSQL", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for PostgreSQL, got %d", len(results))
	}
}

func TestLCMStore_ContextItems(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-ci")

	// Ingest creates context items automatically.
	store.IngestMessage(conv.ID, "user", "hi", 5)
	store.IngestMessage(conv.ID, "assistant", "hello", 6)

	items, err := store.GetContextItems(conv.ID)
	if err != nil {
		t.Fatalf("get context items: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 context items, got %d", len(items))
	}

	// Replace with custom items.
	sumID := "sum_test"
	custom := []LCMContextItem{
		{ConversationID: conv.ID, Ordinal: 0, ItemType: "summary", SummaryID: &sumID},
		{ConversationID: conv.ID, Ordinal: 1, ItemType: "message", MessageID: items[0].MessageID},
	}
	if err := store.ReplaceContextItems(conv.ID, custom); err != nil {
		t.Fatalf("replace: %v", err)
	}

	items2, _ := store.GetContextItems(conv.ID)
	if len(items2) != 2 {
		t.Errorf("expected 2 after replace, got %d", len(items2))
	}
	if items2[0].ItemType != "summary" {
		t.Errorf("expected first item to be summary, got %s", items2[0].ItemType)
	}
}

// ── Compaction Tests ─────────────────────────────────────────────────────────

func TestLCMCompactor_ShouldCompact(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())
	cfg := resolvedLCMConfig(LCMConfig{})
	compactor := NewLCMCompactor(store, cfg, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-compact")

	// Ingest 100 messages with 1000 tokens each.
	for i := 0; i < 100; i++ {
		store.IngestMessage(conv.ID, "user", "x", 1000)
	}

	// Context window = 200000. Unsummarized (100-32)*1000 = 68000.
	// SoftThreshold = 200000 * 0.6 = 120000 → 68000 < 120000 → no.
	should, reason := compactor.ShouldCompact(conv.ID, 200000)
	if should {
		t.Errorf("expected no compaction, got %s", reason)
	}

	// With smaller context window (100000):
	// SoftThreshold = 100000 * 0.6 = 60000 → 68000 >= 60000 → yes.
	should, reason = compactor.ShouldCompact(conv.ID, 100000)
	if !should || reason != "soft_trigger" {
		t.Errorf("expected soft_trigger, got should=%v reason=%s", should, reason)
	}
}

func TestLCMCompactor_LeafPass(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())
	cfg := resolvedLCMConfig(LCMConfig{FreshTailCount: 2, LeafChunkMaxTokens: 500})
	compactor := NewLCMCompactor(store, cfg, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-leaf")

	// Ingest 10 messages.
	for i := 0; i < 10; i++ {
		store.IngestMessage(conv.ID, "user", "test message for leaf pass", 100)
	}

	// Mock summarize function.
	mockSummarize := func(ctx context.Context, text string, aggressive bool) (string, error) {
		return "## Decisions\nTest decision\n## Open TODOs\nNone\n## Constraints/Rules\nNone\n## Pending user asks\ntest\n## Exact identifiers\nNone", nil
	}

	summaries, err := compactor.LeafPass(context.Background(), conv.ID, mockSummarize)
	if err != nil {
		t.Fatalf("leaf pass: %v", err)
	}
	if len(summaries) == 0 {
		t.Error("expected at least 1 summary from leaf pass")
	}
	for _, s := range summaries {
		if s.Kind != "leaf" || s.Depth != 0 {
			t.Errorf("expected leaf/depth=0, got %s/%d", s.Kind, s.Depth)
		}
	}
}

func TestLCMCompactor_FullSweep(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())
	cfg := resolvedLCMConfig(LCMConfig{
		FreshTailCount:       2,
		LeafChunkMaxTokens:   200,
		CondensedMinChildren: 2, // Lower threshold for testing.
		CondensedMaxChildren: 4,
	})
	compactor := NewLCMCompactor(store, cfg, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-sweep")

	// Ingest 20 messages so we get multiple leaf chunks.
	for i := 0; i < 20; i++ {
		store.IngestMessage(conv.ID, "user", "sweep test message content", 50)
	}

	mockSummarize := func(ctx context.Context, text string, aggressive bool) (string, error) {
		return "## Decisions\nSweep\n## Open TODOs\n-\n## Constraints/Rules\n-\n## Pending user asks\n-\n## Exact identifiers\n-", nil
	}

	allNew, err := compactor.FullSweep(context.Background(), conv.ID, mockSummarize)
	if err != nil {
		t.Fatalf("full sweep: %v", err)
	}
	if len(allNew) == 0 {
		t.Error("expected summaries from full sweep")
	}

	// Check that we have at least some leaf summaries.
	leafCount := 0
	for _, s := range allNew {
		if s.Kind == "leaf" {
			leafCount++
		}
	}
	if leafCount == 0 {
		t.Error("expected at least 1 leaf summary")
	}
}

// ── Assembler Tests ──────────────────────────────────────────────────────────

func TestLCMAssembler_AssembleContext(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())
	cfg := resolvedLCMConfig(LCMConfig{FreshTailCount: 3})
	assembler := NewLCMAssembler(store, cfg, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-assemble")

	// Ingest messages.
	for i := 0; i < 5; i++ {
		store.IngestMessage(conv.ID, "user", "user msg", 50)
		store.IngestMessage(conv.ID, "assistant", "assistant msg", 60)
	}

	msgs, err := assembler.AssembleContext(conv.ID, "You are a helpful assistant.", "What is this?", 200000)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	// Should have: system + fresh tail (3 messages) + user message.
	if len(msgs) < 4 {
		t.Errorf("expected at least 4 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected first message to be system, got %s", msgs[0].Role)
	}
	if msgs[len(msgs)-1].Role != "user" {
		t.Errorf("expected last message to be user, got %s", msgs[len(msgs)-1].Role)
	}
}

func TestLCMAssembler_WithSummaries(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())
	cfg := resolvedLCMConfig(LCMConfig{FreshTailCount: 2})
	assembler := NewLCMAssembler(store, cfg, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-assemble-sum")
	now := time.Now().UTC()

	// Create a summary.
	sum := &LCMSummary{
		ID: "sum_test_assemble", ConversationID: conv.ID, Kind: "leaf", Depth: 0,
		Content: "## Decisions\nUsed Go\n## Open TODOs\nNone\n## Constraints/Rules\nNone\n## Pending user asks\nBuild app\n## Exact identifiers\n/main.go",
		TokenCount: 50, EarliestAt: now.Add(-time.Hour), LatestAt: now.Add(-30 * time.Minute), CreatedAt: now,
	}
	store.InsertSummary(sum)

	// Ingest recent messages.
	store.IngestMessage(conv.ID, "user", "recent question", 20)
	store.IngestMessage(conv.ID, "assistant", "recent answer", 25)

	msgs, err := assembler.AssembleContext(conv.ID, "system prompt", "new question", 200000)
	if err != nil {
		t.Fatalf("assemble with summaries: %v", err)
	}

	// Should have: system (with LCM guidance) + summary + tail messages + user.
	if len(msgs) < 4 {
		t.Errorf("expected at least 4 messages, got %d", len(msgs))
	}

	// Check system prompt includes LCM guidance.
	sysContent, ok := msgs[0].Content.(string)
	if !ok {
		t.Fatal("system content is not a string")
	}
	if !containsStr(sysContent, "LCM") {
		t.Error("expected system prompt to include LCM guidance")
	}
}

// ── Retrieval Tests ──────────────────────────────────────────────────────────

func TestLCMRetrieval_Grep(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())
	retrieval := NewLCMRetrieval(store, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-grep")
	store.IngestMessage(conv.ID, "user", "deploy to kubernetes cluster", 100)
	store.IngestMessage(conv.ID, "assistant", "running helm install", 80)

	// FTS search.
	result, err := retrieval.Grep(conv.ID, "kubernetes", false, 10)
	if err != nil {
		t.Fatalf("grep fts: %v", err)
	}
	if !containsStr(result, "kubernetes") {
		t.Errorf("expected result to contain 'kubernetes', got: %s", result)
	}

	// Regex search.
	result, err = retrieval.Grep(conv.ID, "helm.*install", true, 10)
	if err != nil {
		t.Fatalf("grep regex: %v", err)
	}
	if !containsStr(result, "helm") {
		t.Errorf("expected regex result to contain 'helm', got: %s", result)
	}
}

func TestLCMRetrieval_Describe(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())
	retrieval := NewLCMRetrieval(store, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-describe")
	now := time.Now().UTC()

	sum := &LCMSummary{
		ID: "sum_describe", ConversationID: conv.ID, Kind: "leaf", Depth: 0,
		Content: "test content", TokenCount: 5,
		EarliestAt: now.Add(-time.Hour), LatestAt: now, CreatedAt: now,
	}
	store.InsertSummary(sum)

	result, err := retrieval.Describe(conv.ID, "sum_describe")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if !containsStr(result, "sum_describe") || !containsStr(result, "leaf") {
		t.Errorf("unexpected describe result: %s", result)
	}
}

func TestLCMRetrieval_DescribeTree(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())
	retrieval := NewLCMRetrieval(store, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-tree")
	store.IngestMessage(conv.ID, "user", "hello", 10)

	result, err := retrieval.DescribeTree(conv.ID)
	if err != nil {
		t.Fatalf("describe tree: %v", err)
	}
	if !containsStr(result, "LCM DAG Overview") {
		t.Errorf("expected DAG overview, got: %s", result)
	}
	if !containsStr(result, "Total messages: 1") {
		t.Errorf("expected 1 message, got: %s", result)
	}
}

func TestLCMRetrieval_Expand(t *testing.T) {
	db := setupTestLCMDB(t)
	store := NewLCMStore(db, testLogger())
	retrieval := NewLCMRetrieval(store, testLogger())

	conv, _ := store.GetOrCreateConversation("sess-expand")

	msg1, _ := store.IngestMessage(conv.ID, "user", "original message 1", 20)
	msg2, _ := store.IngestMessage(conv.ID, "assistant", "original message 2", 25)

	now := time.Now().UTC()
	sum := &LCMSummary{
		ID: "sum_expand", ConversationID: conv.ID, Kind: "leaf", Depth: 0,
		Content: "summary of messages", TokenCount: 5,
		EarliestAt: now.Add(-time.Hour), LatestAt: now, CreatedAt: now,
	}
	store.InsertSummary(sum)
	store.LinkSummaryMessages("sum_expand", []int64{msg1.ID, msg2.ID})

	result, err := retrieval.Expand(conv.ID, "sum_expand", 0)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if !containsStr(result, "original message 1") || !containsStr(result, "original message 2") {
		t.Errorf("expected original messages in expansion, got: %s", result)
	}
}

// ── Engine Tests ─────────────────────────────────────────────────────────────

func TestLCMEngine_Bootstrap(t *testing.T) {
	db := setupTestLCMDB(t)
	cfg := resolvedLCMConfig(LCMConfig{})
	ccfg := DefaultCompactionConfig()
	engine := NewLCMEngine(db, cfg, ccfg, testLogger())

	convID, err := engine.Bootstrap("sess-boot")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if convID == "" {
		t.Error("expected non-empty conversation ID")
	}

	// Second bootstrap returns same ID.
	convID2, err := engine.Bootstrap("sess-boot")
	if err != nil {
		t.Fatalf("bootstrap 2: %v", err)
	}
	if convID2 != convID {
		t.Errorf("expected same ID, got %s vs %s", convID, convID2)
	}
}

func TestLCMEngine_IngestAndAssemble(t *testing.T) {
	db := setupTestLCMDB(t)
	cfg := resolvedLCMConfig(LCMConfig{FreshTailCount: 3})
	ccfg := DefaultCompactionConfig()
	engine := NewLCMEngine(db, cfg, ccfg, testLogger())

	convID, _ := engine.Bootstrap("sess-e2e")

	// Ingest messages via engine.
	msgs := []chatMessage{
		{Role: "system", Content: "you are helpful"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "user", Content: "how are you"},
		{Role: "assistant", Content: "I'm well"},
	}
	err := engine.Ingest(context.Background(), convID, msgs)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// System messages should be skipped.
	count, _ := engine.Store().MessageCount(convID)
	if count != 4 { // 4 non-system messages
		t.Errorf("expected 4 messages (system skipped), got %d", count)
	}

	// Assemble context.
	assembled, err := engine.Assemble(context.Background(), convID, "system prompt", "new question", 200000)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(assembled) < 3 { // system + tail + new question
		t.Errorf("expected at least 3 assembled messages, got %d", len(assembled))
	}
}

// ── Tool Dispatcher Tests ────────────────────────────────────────────────────

func TestLCMToolDispatcher_Registration(t *testing.T) {
	db := setupTestLCMDB(t)
	cfg := resolvedLCMConfig(LCMConfig{})
	ccfg := DefaultCompactionConfig()
	engine := NewLCMEngine(db, cfg, ccfg, testLogger())

	executor := NewToolExecutor(testLogger())
	RegisterLCMDispatcher(executor, engine)

	// Check tool is registered.
	tools := executor.Tools()
	found := false
	for _, tool := range tools {
		if tool.Function.Name == "lcm" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'lcm' tool to be registered")
	}
}

// ── Config Tests ─────────────────────────────────────────────────────────────

func TestLCMConfig_Defaults(t *testing.T) {
	cfg := resolvedLCMConfig(LCMConfig{})

	if cfg.FreshTailCount != 32 {
		t.Errorf("expected FreshTailCount=32, got %d", cfg.FreshTailCount)
	}
	if cfg.LeafChunkMaxTokens != 20000 {
		t.Errorf("expected LeafChunkMaxTokens=20000, got %d", cfg.LeafChunkMaxTokens)
	}
	if cfg.CondensedMinChildren != 4 {
		t.Errorf("expected CondensedMinChildren=4, got %d", cfg.CondensedMinChildren)
	}
	if cfg.SoftTriggerRatio != 0.6 {
		t.Errorf("expected SoftTriggerRatio=0.6, got %f", cfg.SoftTriggerRatio)
	}
	if cfg.HardTriggerRatio != 0.85 {
		t.Errorf("expected HardTriggerRatio=0.85, got %f", cfg.HardTriggerRatio)
	}
}

func TestLCMEnabled_Default(t *testing.T) {
	cfg := CompactionConfig{}
	if !cfg.lcmEnabled() {
		t.Error("expected LCM enabled by default (nil pointer = true)")
	}

	f := false
	cfg.LCMEnabled = &f
	if cfg.lcmEnabled() {
		t.Error("expected LCM disabled when explicitly set to false")
	}

	tr := true
	cfg.LCMEnabled = &tr
	if !cfg.lcmEnabled() {
		t.Error("expected LCM enabled when explicitly set to true")
	}
}

// ── Utility Tests ────────────────────────────────────────────────────────────

func TestGenerateSummaryID(t *testing.T) {
	now := time.Now()
	id1 := GenerateSummaryID("content", now)
	id2 := GenerateSummaryID("content", now)
	id3 := GenerateSummaryID("different", now)

	if id1 != id2 {
		t.Error("same input should produce same ID")
	}
	if id1 == id3 {
		t.Error("different input should produce different ID")
	}
	if len(id1) < 10 || id1[:4] != "sum_" {
		t.Errorf("unexpected ID format: %s", id1)
	}
}

func TestLCMEstimateTokens(t *testing.T) {
	if n := EstimateTokens("hello world"); n != 2 {
		t.Errorf("expected ~2 tokens, got %d", n)
	}
	if n := EstimateTokens("a"); n != 1 {
		t.Errorf("expected 1 token for short string, got %d", n)
	}
	if n := EstimateTokens(""); n != 0 {
		t.Errorf("expected 0 tokens for empty string, got %d", n)
	}
}

// containsStr is a test helper wrapping strings.Contains.
func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

// Package memory — sqlite_hierarchy_test.go covers the Sprint 1 palace-aware
// schema additions and CRUD operations.
//
// Retrocompat tests are the most important: we verify that the hierarchy
// schema does NOT disturb legacy files behavior.
//
// Uses the existing newTestStore helper from sqlite_store_test.go which
// provides a deterministicEmbedder and automatic cleanup.
package memory

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestInitHierarchySchemaIdempotent confirms that running the hierarchy
// schema initialization multiple times on the same database is safe.
func TestInitHierarchySchemaIdempotent(t *testing.T) {
	store := newTestStore(t)
	logger := silenceLogger()

	// Call it three times — should not fail.
	for i := 0; i < 3; i++ {
		if err := InitHierarchySchema(store.db, logger); err != nil {
			t.Fatalf("InitHierarchySchema attempt %d failed: %v", i+1, err)
		}
	}

	// Verify all hierarchy columns exist on files.
	cols, err := existingColumns(store.db, "files")
	if err != nil {
		t.Fatalf("existingColumns: %v", err)
	}
	for _, want := range hierarchyFileColumns {
		if _, ok := cols[want.Name]; !ok {
			t.Errorf("expected column %q on files, not found", want.Name)
		}
	}
}

// TestHierarchyIsRetroCompat asserts that the core schema is still fully
// usable after hierarchy init, and that legacy (wing=NULL) files work.
func TestHierarchyIsRetroCompat(t *testing.T) {
	store := newTestStore(t)

	// Insert a legacy file without wing metadata.
	_, err := store.db.Exec(`
		INSERT INTO files (file_id, hash) VALUES (?, ?)
	`, "MEMORY.md", "abc123")
	if err != nil {
		t.Fatalf("insert legacy file: %v", err)
	}

	// Read it back — the wing column must be NULL.
	var wing, hall, room interface{}
	var accessCount int
	err = store.db.QueryRow(`
		SELECT wing, hall, room, access_count
		FROM files WHERE file_id = ?
	`, "MEMORY.md").Scan(&wing, &hall, &room, &accessCount)
	if err != nil {
		t.Fatalf("read legacy file: %v", err)
	}
	if wing != nil {
		t.Errorf("expected wing=NULL for legacy file, got %v", wing)
	}
	if hall != nil {
		t.Errorf("expected hall=NULL for legacy file, got %v", hall)
	}
	if room != nil {
		t.Errorf("expected room=NULL for legacy file, got %v", room)
	}
	if accessCount != 0 {
		t.Errorf("expected access_count=0 for new file, got %d", accessCount)
	}
}

// TestLegacyFileCount verifies the TotalLegacyFiles query.
func TestLegacyFileCount(t *testing.T) {
	store := newTestStore(t)

	// No files → 0.
	n, err := store.TotalLegacyFiles()
	if err != nil {
		t.Fatalf("TotalLegacyFiles empty: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 legacy files, got %d", n)
	}

	// Insert 2 legacy + 1 classified.
	_, _ = store.db.Exec(`INSERT INTO files (file_id, hash) VALUES ('legacy1.md', 'h1')`)
	_, _ = store.db.Exec(`INSERT INTO files (file_id, hash) VALUES ('legacy2.md', 'h2')`)
	_, _ = store.db.Exec(`INSERT INTO files (file_id, hash, wing) VALUES ('work/file.md', 'h3', 'work')`)

	n, err = store.TotalLegacyFiles()
	if err != nil {
		t.Fatalf("TotalLegacyFiles populated: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 legacy files, got %d", n)
	}
}

// TestWingRegistry covers UpsertWing, ListWings, DeleteWing.
func TestWingRegistry(t *testing.T) {
	store := newTestStore(t)

	// Upsert a wing.
	if err := store.UpsertWing("work", "Work", "Job stuff"); err != nil {
		t.Fatalf("UpsertWing: %v", err)
	}

	// Upsert with invalid name.
	if err := store.UpsertWing("__system", "Bad", ""); err == nil {
		t.Error("UpsertWing should reject reserved prefix")
	}
	if err := store.UpsertWing("", "Empty", ""); err == nil {
		t.Error("UpsertWing should reject empty name")
	}

	// List includes our wing.
	wings, err := store.ListWings()
	if err != nil {
		t.Fatalf("ListWings: %v", err)
	}
	var found bool
	for _, w := range wings {
		if w.Name == "work" && w.DisplayName == "Work" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'work' wing in ListWings")
	}

	// Delete (force=false) works since no files or rooms reference it.
	if err := store.DeleteWing("work", false); err != nil {
		t.Errorf("DeleteWing: %v", err)
	}

	// Delete nonexistent — no error (DELETE is no-op).
	if err := store.DeleteWing("nonexistent", false); err != nil {
		t.Errorf("DeleteWing nonexistent: %v", err)
	}
}

// TestChannelWingMap covers the full lifecycle of a channel-wing mapping.
func TestChannelWingMap(t *testing.T) {
	store := newTestStore(t)

	// Not found initially.
	_, err := store.GetChannelWing("telegram", "12345")
	if !errors.Is(err, ErrChannelWingNotFound) {
		t.Errorf("expected ErrChannelWingNotFound, got %v", err)
	}

	// Set a manual mapping.
	if err := store.SetChannelWing("telegram", "12345", "work", "manual", 1.0); err != nil {
		t.Fatalf("SetChannelWing: %v", err)
	}

	// Get it back.
	m, err := store.GetChannelWing("telegram", "12345")
	if err != nil {
		t.Fatalf("GetChannelWing: %v", err)
	}
	if m.Wing != "work" {
		t.Errorf("expected wing=work, got %q", m.Wing)
	}
	if m.Confidence != 1.0 {
		t.Errorf("expected confidence=1.0, got %v", m.Confidence)
	}
	if m.Source != "manual" {
		t.Errorf("expected source=manual, got %q", m.Source)
	}

	// Update it (same key, different wing).
	if err := store.SetChannelWing("telegram", "12345", "personal", "manual", 1.0); err != nil {
		t.Fatalf("update mapping: %v", err)
	}
	m, _ = store.GetChannelWing("telegram", "12345")
	if m.Wing != "personal" {
		t.Errorf("update didn't stick, got wing=%q", m.Wing)
	}

	// Normalization applies on write.
	if err := store.SetChannelWing("telegram", "67890", "TRABALHO", "manual", 1.0); err != nil {
		t.Fatalf("SetChannelWing trabalho: %v", err)
	}
	m, _ = store.GetChannelWing("telegram", "67890")
	if m.Wing != "trabalho" {
		t.Errorf("expected normalized wing=trabalho, got %q", m.Wing)
	}

	// Delete the mapping.
	if err := store.DeleteChannelWing("telegram", "67890"); err != nil {
		t.Errorf("DeleteChannelWing: %v", err)
	}
	_, err = store.GetChannelWing("telegram", "67890")
	if !errors.Is(err, ErrChannelWingNotFound) {
		t.Errorf("expected not found after delete, got %v", err)
	}

	// Invalid inputs rejected.
	if err := store.SetChannelWing("", "12345", "work", "manual", 1.0); err == nil {
		t.Error("should reject empty channel")
	}
	if err := store.SetChannelWing("telegram", "", "work", "manual", 1.0); err == nil {
		t.Error("should reject empty external_id")
	}
	if err := store.SetChannelWing("telegram", "12345", "", "manual", 1.0); err == nil {
		t.Error("should reject empty wing")
	}
	if err := store.SetChannelWing("telegram", "12345", "work", "invalid-source", 1.0); err == nil {
		t.Error("should reject invalid source")
	}
	if err := store.SetChannelWing("telegram", "12345", "work", "manual", 1.5); err == nil {
		t.Error("should reject confidence > 1")
	}
}

// TestRoomRegistry covers room upsert and reuse_count increment.
func TestRoomRegistry(t *testing.T) {
	store := newTestStore(t)

	// First upsert creates.
	if err := store.UpsertRoom("work", "auth-migration", "", "manual", 1.0); err != nil {
		t.Fatalf("UpsertRoom: %v", err)
	}

	// Second upsert bumps reuse_count.
	if err := store.UpsertRoom("work", "auth-migration", "", "auto", 0.8); err != nil {
		t.Fatalf("UpsertRoom second: %v", err)
	}

	rooms, err := store.ListRooms("work")
	if err != nil {
		t.Fatalf("ListRooms: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("expected 1 room, got %d", len(rooms))
	}
	if rooms[0].ReuseCount != 1 {
		t.Errorf("expected reuse_count=1, got %d", rooms[0].ReuseCount)
	}
	// Confidence is MAXed, not overwritten.
	if rooms[0].Confidence != 1.0 {
		t.Errorf("expected confidence=1.0 (max), got %v", rooms[0].Confidence)
	}

	// Invalid source rejected.
	if err := store.UpsertRoom("work", "test", "", "bogus", 0.5); err == nil {
		t.Error("should reject invalid source")
	}
}

// TestSeedSuggestedWings confirms seed is idempotent and respects existing data.
func TestSeedSuggestedWings(t *testing.T) {
	store := newTestStore(t)

	// Seed once.
	if err := SeedSuggestedWings(store.db); err != nil {
		t.Fatalf("SeedSuggestedWings first: %v", err)
	}

	wings, _ := store.ListWings()
	initialCount := len(wings)
	if initialCount != 6 {
		t.Errorf("expected 6 suggested wings, got %d", initialCount)
	}

	// Seed again — should be no-op (count stays at 6).
	if err := SeedSuggestedWings(store.db); err != nil {
		t.Fatalf("SeedSuggestedWings idempotent: %v", err)
	}

	wings, _ = store.ListWings()
	if len(wings) != initialCount {
		t.Errorf("seed not idempotent: %d → %d", initialCount, len(wings))
	}

	// Every suggested wing should be marked is_suggested.
	for _, w := range wings {
		if !w.IsSuggested {
			t.Errorf("wing %q should be marked suggested", w.Name)
		}
	}
}

// TestGetTaxonomy smoke-tests the full tree query.
func TestGetTaxonomy(t *testing.T) {
	store := newTestStore(t)

	_ = store.UpsertWing("work", "Work", "")
	_ = store.UpsertRoom("work", "auth", "", "manual", 1.0)
	_ = store.UpsertRoom("work", "db", "", "manual", 1.0)
	_ = store.UpsertWing("personal", "Personal", "")

	tree, err := store.GetTaxonomy()
	if err != nil {
		t.Fatalf("GetTaxonomy: %v", err)
	}
	if len(tree) < 2 {
		t.Errorf("expected at least 2 wings, got %d", len(tree))
	}

	// Find work wing and verify rooms.
	var workEntry *TaxonomyEntry
	for i := range tree {
		if tree[i].Wing.Name == "work" {
			workEntry = &tree[i]
			break
		}
	}
	if workEntry == nil {
		t.Fatal("work wing not found in taxonomy")
	}
	if len(workEntry.Rooms) != 2 {
		t.Errorf("expected 2 rooms in work wing, got %d", len(workEntry.Rooms))
	}
}

// silenceLogger returns a logger that discards all output.
// Used by tests that call InitHierarchySchema directly (which needs a logger).
func silenceLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

var _ = silenceLogger

// v117CoreSchema is a copy of the v1.17.0 baseline schema that existed
// BEFORE Sprint 1 added the hierarchy columns. Do NOT modify this —
// it is the "time capsule" used by TestInitHierarchySchemaUpgradeFromLegacy
// to simulate a user upgrading from v1.17.0 to v1.18.0.
//
// This MUST stay byte-identical to the core schema as of v1.17.0
// (sqlite_store.go coreSchema constant before 2026-04-08).
const v117CoreSchema = `
	CREATE TABLE IF NOT EXISTS files (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id   TEXT UNIQUE NOT NULL,
		hash      TEXT NOT NULL,
		indexed_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS chunks (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id    TEXT NOT NULL,
		chunk_idx  INTEGER NOT NULL,
		text       TEXT NOT NULL,
		hash       TEXT NOT NULL,
		embedding  TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(file_id, chunk_idx)
	);

	CREATE TABLE IF NOT EXISTS embedding_cache (
		text_hash TEXT NOT NULL,
		provider  TEXT NOT NULL,
		model     TEXT NOT NULL,
		embedding TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (text_hash, provider, model)
	);
`

// TestInitHierarchySchemaUpgradeFromLegacy is THE critical retrocompat
// test (CR-1 from the Sprint 1 code review). It simulates the exact
// scenario that happens when a user upgrades DevClaw from v1.17.0 to
// v1.18.0:
//
//  1. A database already exists with ONLY the v1.17.0 schema (no wing
//     columns, no palace tables).
//  2. Legacy rows exist in the files table with their content in chunks.
//  3. The new v1.18.0 binary attaches to that database and calls initSchema,
//     which in turn calls InitHierarchySchema.
//  4. The upgrade must:
//     - add all new hierarchy columns to files (as NULL)
//     - create all new palace tables
//     - preserve every original row untouched
//     - leave legacy rows with wing IS NULL (first-class per ADR-006)
//
// Any break in this chain would be a retrocompat regression.
func TestInitHierarchySchemaUpgradeFromLegacy(t *testing.T) {
	// Step 1: open a raw SQLite handle and build ONLY the v1.17.0 schema.
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(v117CoreSchema); err != nil {
		t.Fatalf("apply v1.17.0 core schema: %v", err)
	}

	// Verify we do NOT yet have any hierarchy columns — this is the
	// genuinely "legacy" starting state.
	cols, err := existingColumns(db, "files")
	if err != nil {
		t.Fatalf("read legacy columns: %v", err)
	}
	for _, hc := range hierarchyFileColumns {
		if _, present := cols[hc.Name]; present {
			t.Fatalf("legacy DB should NOT have %q column yet — test fixture is wrong", hc.Name)
		}
	}

	// Step 2: insert three legacy rows and their chunks. These represent
	// memories from v1.17.0 that must survive the upgrade.
	legacyRows := []struct {
		fileID string
		hash   string
		chunks []string
	}{
		{"old-work.md", "h-work", []string{"Sprint retro from 2026-03-15"}},
		{"old-personal.md", "h-personal", []string{"Bought groceries"}},
		{"old-family.md", "h-family", []string{"Mãe ligou ontem"}},
	}
	for _, r := range legacyRows {
		if _, err := db.Exec(`INSERT INTO files (file_id, hash) VALUES (?, ?)`, r.fileID, r.hash); err != nil {
			t.Fatalf("insert legacy file %s: %v", r.fileID, err)
		}
		for i, c := range r.chunks {
			if _, err := db.Exec(
				`INSERT INTO chunks (file_id, chunk_idx, text, hash) VALUES (?, ?, ?, ?)`,
				r.fileID, i, c, "chunk-h"); err != nil {
				t.Fatalf("insert chunk: %v", err)
			}
		}
	}

	// Snapshot pre-upgrade row count and a specific row for byte comparison.
	var preCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&preCount); err != nil {
		t.Fatalf("pre-count: %v", err)
	}
	if preCount != 3 {
		t.Fatalf("expected 3 legacy rows, got %d", preCount)
	}
	var preIndexedAt string
	if err := db.QueryRow(`SELECT indexed_at FROM files WHERE file_id = ?`, "old-work.md").Scan(&preIndexedAt); err != nil {
		t.Fatalf("pre indexed_at: %v", err)
	}

	// Step 3: run the v1.18.0 hierarchy initialization. This is the exact
	// call site exercised in production via sqlite_store.go's initSchema.
	if err := InitHierarchySchema(db, silenceLogger()); err != nil {
		t.Fatalf("InitHierarchySchema on legacy DB: %v", err)
	}

	// Step 4a: every new column must now be present on files.
	cols, err = existingColumns(db, "files")
	if err != nil {
		t.Fatalf("read post-upgrade columns: %v", err)
	}
	for _, hc := range hierarchyFileColumns {
		if _, present := cols[hc.Name]; !present {
			t.Errorf("post-upgrade: column %q missing from files", hc.Name)
		}
	}

	// Step 4b: every new palace table must now exist. Use the query the
	// CRUD functions actually issue — if that works, the table is there.
	paceTables := []string{"wings", "rooms", "rooms_archive", "channel_wing_map"}
	for _, tbl := range paceTables {
		var dummy int
		row := db.QueryRow(`SELECT count(*) FROM ` + tbl)
		if err := row.Scan(&dummy); err != nil {
			t.Errorf("post-upgrade: table %q not queryable: %v", tbl, err)
		}
	}

	// Step 4c: legacy rows must be PRESERVED and carry wing IS NULL.
	// Count preserved:
	var postCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&postCount); err != nil {
		t.Fatalf("post-count: %v", err)
	}
	if postCount != preCount {
		t.Errorf("legacy rows lost: pre=%d post=%d", preCount, postCount)
	}

	// Same file still has its original indexed_at timestamp (byte-level):
	var postIndexedAt string
	if err := db.QueryRow(`SELECT indexed_at FROM files WHERE file_id = ?`, "old-work.md").Scan(&postIndexedAt); err != nil {
		t.Fatalf("post indexed_at: %v", err)
	}
	if postIndexedAt != preIndexedAt {
		t.Errorf("legacy indexed_at mutated: pre=%q post=%q", preIndexedAt, postIndexedAt)
	}

	// All legacy rows MUST have wing IS NULL (first-class per ADR-006).
	for _, r := range legacyRows {
		var wing sql.NullString
		var hall sql.NullString
		var room sql.NullString
		err := db.QueryRow(
			`SELECT wing, hall, room FROM files WHERE file_id = ?`, r.fileID,
		).Scan(&wing, &hall, &room)
		if err != nil {
			t.Errorf("read upgraded legacy row %s: %v", r.fileID, err)
			continue
		}
		if wing.Valid {
			t.Errorf("legacy row %s got wing=%q, expected NULL", r.fileID, wing.String)
		}
		if hall.Valid {
			t.Errorf("legacy row %s got hall=%q, expected NULL", r.fileID, hall.String)
		}
		if room.Valid {
			t.Errorf("legacy row %s got room=%q, expected NULL", r.fileID, room.String)
		}
	}

	// Step 4d: chunks table must be untouched (chunks have no hierarchy yet).
	var chunkCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&chunkCount); err != nil {
		t.Fatalf("post-chunk count: %v", err)
	}
	if chunkCount != 3 {
		t.Errorf("chunks mutated: expected 3, got %d", chunkCount)
	}

	// Step 4e: running InitHierarchySchema AGAIN must be a no-op (idempotent
	// on already-upgraded DBs, which is what dream cycles and reconnects
	// will cause in practice).
	if err := InitHierarchySchema(db, silenceLogger()); err != nil {
		t.Errorf("second InitHierarchySchema call should be idempotent: %v", err)
	}
	var postPostCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&postPostCount)
	if postPostCount != preCount {
		t.Errorf("second init changed row count: %d → %d", preCount, postPostCount)
	}
}

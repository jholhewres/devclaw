package copilot

import (
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMaintenanceManager_IsEnabled(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS system_state (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	mgr := NewMaintenanceManager(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// Initially disabled
	if mgr.IsEnabled() {
		t.Error("expected maintenance mode to be disabled initially")
	}

	// Enable
	if err := mgr.Set(true, "Test message", "test-user"); err != nil {
		t.Fatalf("failed to enable maintenance: %v", err)
	}

	if !mgr.IsEnabled() {
		t.Error("expected maintenance mode to be enabled")
	}

	// Disable
	if err := mgr.Set(false, "", "test-user"); err != nil {
		t.Fatalf("failed to disable maintenance: %v", err)
	}

	if mgr.IsEnabled() {
		t.Error("expected maintenance mode to be disabled")
	}
}

func TestMaintenanceManager_Get(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS system_state (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	mgr := NewMaintenanceManager(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// Get when nil
	m := mgr.Get()
	if m != nil {
		t.Error("expected nil when maintenance not set")
	}

	// Set and get
	if err := mgr.Set(true, "Under maintenance", "admin"); err != nil {
		t.Fatalf("failed to set maintenance: %v", err)
	}

	m = mgr.Get()
	if m == nil {
		t.Fatal("expected maintenance mode to be returned")
	}
	if !m.Enabled {
		t.Error("expected Enabled to be true")
	}
	if m.Message != "Under maintenance" {
		t.Errorf("expected Message to be 'Under maintenance', got %q", m.Message)
	}
	if m.SetBy != "admin" {
		t.Errorf("expected SetBy to be 'admin', got %q", m.SetBy)
	}
}

func TestMaintenanceManager_Load(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS system_state (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Insert existing state
	existingValue := `{"enabled":true,"message":"Existing maintenance","set_by":"previous-admin","set_at":"2024-01-01T00:00:00Z"}`
	_, err = db.Exec(
		"INSERT INTO system_state (key, value, updated_at) VALUES (?, ?, ?)",
		"maintenance_mode", existingValue, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("failed to insert existing state: %v", err)
	}

	mgr := NewMaintenanceManager(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// Load should restore the state
	if err := mgr.Load(); err != nil {
		t.Fatalf("failed to load maintenance state: %v", err)
	}

	if !mgr.IsEnabled() {
		t.Error("expected maintenance mode to be enabled after load")
	}

	m := mgr.Get()
	if m.Message != "Existing maintenance" {
		t.Errorf("expected Message to be 'Existing maintenance', got %q", m.Message)
	}
}

func TestMaintenanceManager_NilDatabase(t *testing.T) {
	mgr := NewMaintenanceManager(nil, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// Operations should not panic with nil database
	if mgr.IsEnabled() {
		t.Error("expected disabled with nil database")
	}

	// Set should not error (but won't persist)
	if err := mgr.Set(true, "test", "user"); err != nil {
		t.Errorf("unexpected error with nil database: %v", err)
	}

	// Load should not error
	if err := mgr.Load(); err != nil {
		t.Errorf("unexpected error with nil database: %v", err)
	}
}

func TestMaintenanceManager_Persistence(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create schema
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS system_state (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create first manager and set maintenance
	mgr1 := NewMaintenanceManager(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := mgr1.Set(true, "Persistent message", "user1"); err != nil {
		t.Fatalf("failed to set maintenance: %v", err)
	}

	db.Close()

	// Reopen database and create new manager
	db2, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer db2.Close()

	mgr2 := NewMaintenanceManager(db2, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := mgr2.Load(); err != nil {
		t.Fatalf("failed to load maintenance state: %v", err)
	}

	m := mgr2.Get()
	if m == nil {
		t.Fatal("expected maintenance mode to be restored")
	}
	if m.Message != "Persistent message" {
		t.Errorf("expected 'Persistent message', got %q", m.Message)
	}
	if m.SetBy != "user1" {
		t.Errorf("expected SetBy to be 'user1', got %q", m.SetBy)
	}
}

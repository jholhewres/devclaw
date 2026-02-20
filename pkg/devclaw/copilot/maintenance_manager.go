// Package copilot – maintenance_manager.go manages maintenance mode state.
package copilot

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// MaintenanceManager manages maintenance mode state with database persistence.
type MaintenanceManager struct {
	mu     sync.RWMutex
	current *MaintenanceMode
	db      *sql.DB
	logger  *slog.Logger
}

// NewMaintenanceManager creates a new maintenance manager.
func NewMaintenanceManager(db *sql.DB, logger *slog.Logger) *MaintenanceManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &MaintenanceManager{
		db:     db,
		logger: logger.With("component", "maintenance"),
	}
}

// IsEnabled returns true if maintenance mode is active.
func (m *MaintenanceManager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current != nil && m.current.Enabled
}

// Get returns the current maintenance mode state.
func (m *MaintenanceManager) Get() *MaintenanceMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == nil {
		return nil
	}
	// Return a copy
	copy := *m.current
	return &copy
}

// Set enables or disables maintenance mode.
func (m *MaintenanceManager) Set(enabled bool, message, setBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if enabled {
		m.current = &MaintenanceMode{
			Enabled: true,
			Message: message,
			SetBy:   setBy,
			SetAt:   time.Now(),
		}
	} else {
		m.current = &MaintenanceMode{
			Enabled: false,
			SetBy:   setBy,
			SetAt:   time.Now(),
		}
	}

	if err := m.save(); err != nil {
		m.logger.Error("failed to save maintenance state", "error", err)
		return err
	}

	m.logger.Info("maintenance mode changed",
		"enabled", enabled,
		"message", message,
		"set_by", setBy,
	)
	return nil
}

// Load restores maintenance mode from database on startup.
func (m *MaintenanceManager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db == nil {
		return nil
	}

	var value string
	var updatedAt string
	err := m.db.QueryRow(
		"SELECT value, updated_at FROM system_state WHERE key = ?",
		"maintenance_mode",
	).Scan(&value, &updatedAt)

	if err == sql.ErrNoRows {
		// No previous state, start with maintenance disabled
		m.current = nil
		return nil
	}
	if err != nil {
		return err
	}

	var mode MaintenanceMode
	if err := json.Unmarshal([]byte(value), &mode); err != nil {
		m.logger.Warn("failed to unmarshal maintenance state", "error", err)
		return err
	}

	m.current = &mode
	m.logger.Info("loaded maintenance state from database",
		"enabled", mode.Enabled,
		"message", mode.Message,
	)
	return nil
}

// save persists the current state to the database.
func (m *MaintenanceManager) save() error {
	if m.db == nil {
		return nil
	}

	value, err := json.Marshal(m.current)
	if err != nil {
		return err
	}

	_, err = m.db.Exec(
		`INSERT INTO system_state (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		"maintenance_mode",
		string(value),
		time.Now().Format(time.RFC3339),
	)
	return err
}

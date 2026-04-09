// Package memory — sqlite_palace_ops.go provides CRUD operations for the
// palace-aware tables introduced in Sprint 1: wings, rooms, channel_wing_map.
//
// All functions here are SAFE TO CALL even when the palace-aware feature
// flag is off — they operate on the registry tables which exist
// unconditionally (though may be empty). Callers should gate invocation
// at a higher level.
//
// Sprint 2 Room 2.0b adds AssignWingToFile which touches the `files` table.
// All other functions deal exclusively with the registry tables.
package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Wing registry operations
// ─────────────────────────────────────────────────────────────────────────────

// WingInfo describes a registered wing.
type WingInfo struct {
	Name         string
	DisplayName  string
	Description  string
	Color        string
	IsDefault    bool
	IsSuggested  bool
	MemoryCount  int // computed from files table at query time
	RoomCount    int // computed from rooms table at query time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UpsertWing creates a wing entry if it doesn't exist, or updates its
// display metadata if it does. Normalizes the name first; returns an error
// if normalization yields an empty string.
//
// Does not touch rooms or memories — purely a registry operation.
func (s *SQLiteStore) UpsertWing(name, displayName, description string) error {
	norm := NormalizeWing(name)
	if norm == "" {
		return fmt.Errorf("invalid wing name: %q (must normalize to non-empty identifier)", name)
	}
	_, err := s.db.Exec(`
		INSERT INTO wings (name, display_name, description, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			display_name = COALESCE(NULLIF(excluded.display_name, ''), wings.display_name),
			description  = COALESCE(NULLIF(excluded.description, ''),  wings.description),
			updated_at   = CURRENT_TIMESTAMP
	`, norm, displayName, description)
	return err
}

// ListWings returns all registered wings with their memory and room counts.
// Counts are computed via JOIN at query time rather than maintained as a
// cached column, so they always reflect current state.
//
// The `files` table may not yet have a `wing` column if the hierarchy
// schema failed to initialize — in that case, memory_count is reported as 0
// rather than returning an error.
func (s *SQLiteStore) ListWings() ([]WingInfo, error) {
	rows, err := s.db.Query(`
		SELECT
			w.name,
			COALESCE(w.display_name, ''),
			COALESCE(w.description, ''),
			COALESCE(w.color, ''),
			w.is_default,
			w.is_suggested,
			w.created_at,
			w.updated_at
		FROM wings w
		ORDER BY w.name
	`)
	if err != nil {
		return nil, fmt.Errorf("list wings: %w", err)
	}
	defer rows.Close()

	var out []WingInfo
	for rows.Next() {
		var wi WingInfo
		var isDefault, isSuggested int
		if err := rows.Scan(
			&wi.Name,
			&wi.DisplayName,
			&wi.Description,
			&wi.Color,
			&isDefault,
			&isSuggested,
			&wi.CreatedAt,
			&wi.UpdatedAt,
		); err != nil {
			return nil, err
		}
		wi.IsDefault = isDefault != 0
		wi.IsSuggested = isSuggested != 0
		out = append(out, wi)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Enrich with counts (best-effort — tolerate missing columns).
	for i := range out {
		_ = s.fillWingCounts(&out[i])
	}
	return out, nil
}

// fillWingCounts populates MemoryCount and RoomCount for a wing. Any query
// failure is swallowed so that listing still returns the registry even if
// the hierarchy schema is partially applied.
func (s *SQLiteStore) fillWingCounts(wi *WingInfo) error {
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE wing = ?`, wi.Name).Scan(&wi.MemoryCount)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM rooms WHERE wing = ?`, wi.Name).Scan(&wi.RoomCount)
	return nil
}

// DeleteWing removes a wing from the registry. Does NOT cascade to files
// or rooms — those are left intact but orphaned. Intended for cleanup of
// suggested wings the user never used.
//
// Returns an error if the wing still has any associated files or rooms,
// unless force is true.
func (s *SQLiteStore) DeleteWing(name string, force bool) error {
	norm := NormalizeWing(name)
	if norm == "" {
		return fmt.Errorf("invalid wing name: %q", name)
	}

	if !force {
		var fileCount, roomCount int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE wing = ?`, norm).Scan(&fileCount)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM rooms WHERE wing = ?`, norm).Scan(&roomCount)
		if fileCount > 0 || roomCount > 0 {
			return fmt.Errorf("wing %q has %d files and %d rooms; pass force=true to delete anyway", norm, fileCount, roomCount)
		}
	}

	_, err := s.db.Exec(`DELETE FROM wings WHERE name = ?`, norm)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Room registry operations
// ─────────────────────────────────────────────────────────────────────────────

// RoomInfo describes a registered room inside a wing.
type RoomInfo struct {
	Wing         string
	Name         string
	Hall         string
	Source       string // 'manual' | 'auto' | 'inferred' | 'legacy'
	Confidence   float64
	ReuseCount   int
	MemoryCount  int
	DisplayName  string
	Description  string
	LastActivity time.Time
	CreatedAt    time.Time
}

// ListRooms returns rooms belonging to a wing. Pass an empty wing to list
// all rooms across all wings (useful for maintenance views).
//
// Results are ordered by memory_count DESC so the most-used rooms surface first.
func (s *SQLiteStore) ListRooms(wing string) ([]RoomInfo, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if wing == "" {
		rows, err = s.db.Query(`
			SELECT wing, name, COALESCE(hall, ''), source, confidence,
				reuse_count, memory_count,
				COALESCE(display_name, ''), COALESCE(description, ''),
				last_activity, created_at
			FROM rooms
			ORDER BY wing, memory_count DESC
		`)
	} else {
		norm := NormalizeWing(wing)
		if norm == "" {
			return nil, fmt.Errorf("invalid wing name: %q", wing)
		}
		rows, err = s.db.Query(`
			SELECT wing, name, COALESCE(hall, ''), source, confidence,
				reuse_count, memory_count,
				COALESCE(display_name, ''), COALESCE(description, ''),
				last_activity, created_at
			FROM rooms
			WHERE wing = ?
			ORDER BY memory_count DESC
		`, norm)
	}
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}
	defer rows.Close()

	var out []RoomInfo
	for rows.Next() {
		var ri RoomInfo
		// last_activity is DATETIME but may be NULL for newly created rooms
		// that have not yet been touched. sql.NullTime handles both cases.
		var lastActivity sql.NullTime
		if err := rows.Scan(
			&ri.Wing, &ri.Name, &ri.Hall,
			&ri.Source, &ri.Confidence,
			&ri.ReuseCount, &ri.MemoryCount,
			&ri.DisplayName, &ri.Description,
			&lastActivity, &ri.CreatedAt,
		); err != nil {
			return nil, err
		}
		if lastActivity.Valid {
			ri.LastActivity = lastActivity.Time
		}
		// If NULL, LastActivity remains the zero value — callers should
		// check for .IsZero() before displaying.
		out = append(out, ri)
	}
	return out, rows.Err()
}

// UpsertRoom creates or updates a room entry. The source parameter controls
// how the room was derived; confidence should be 1.0 for manual entries.
//
// If the room already exists, this function increments its reuse_count and
// bumps last_activity — useful for the ADR-002 Addendum A confidence
// promotion rule.
func (s *SQLiteStore) UpsertRoom(wing, name, hall, source string, confidence float64) error {
	wNorm := NormalizeWing(wing)
	rNorm := NormalizeRoom(name)
	hNorm := NormalizeHall(hall) // may be empty
	if wNorm == "" || rNorm == "" {
		return fmt.Errorf("invalid wing/room: wing=%q room=%q", wing, name)
	}
	validSources := map[string]bool{"manual": true, "auto": true, "inferred": true, "legacy": true}
	if !validSources[source] {
		return fmt.Errorf("invalid room source: %q (valid: manual, auto, inferred, legacy)", source)
	}
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence out of range [0,1]: %v", confidence)
	}

	_, err := s.db.Exec(`
		INSERT INTO rooms (wing, name, hall, source, confidence, last_activity)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(wing, name) DO UPDATE SET
			reuse_count   = rooms.reuse_count + 1,
			confidence    = MAX(rooms.confidence, excluded.confidence),
			last_activity = CURRENT_TIMESTAMP
	`, wNorm, rNorm, hNorm, source, confidence)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Channel → wing mapping operations
// ─────────────────────────────────────────────────────────────────────────────

// ChannelWingMapping describes how the context router should treat messages
// arriving from a specific channel + external ID.
type ChannelWingMapping struct {
	Channel    string
	ExternalID string
	Wing       string
	Confidence float64
	Source     string // 'manual' | 'heuristic' | 'llm' | 'inherited'
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ErrChannelWingNotFound is returned by GetChannelWing when no mapping
// exists for the given (channel, external_id) pair.
var ErrChannelWingNotFound = errors.New("channel wing mapping not found")

// GetChannelWing looks up the wing assigned to a specific channel + external ID.
// Returns ErrChannelWingNotFound if no mapping exists — callers should treat
// this as "unmapped" and fall back to heuristics or defaults.
func (s *SQLiteStore) GetChannelWing(channel, externalID string) (ChannelWingMapping, error) {
	var m ChannelWingMapping
	err := s.db.QueryRow(`
		SELECT channel, external_id, wing, confidence, source, created_at, updated_at
		FROM channel_wing_map
		WHERE channel = ? AND external_id = ?
	`, channel, externalID).Scan(
		&m.Channel, &m.ExternalID, &m.Wing,
		&m.Confidence, &m.Source, &m.CreatedAt, &m.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ChannelWingMapping{}, ErrChannelWingNotFound
	}
	if err != nil {
		return ChannelWingMapping{}, fmt.Errorf("get channel wing: %w", err)
	}
	return m, nil
}

// SetChannelWing creates or updates a channel → wing mapping. The wing name
// is normalized before storage. Source should be one of:
//   - "manual"    — user explicitly set it (confidence 1.0)
//   - "heuristic" — derived from channel pattern (confidence 0.5-0.8)
//   - "llm"       — classified by LLM (confidence varies)
//   - "inherited" — copied from another mapping (confidence inherited)
//
// Passing an empty wing is an error — use DeleteChannelWing to remove a mapping.
func (s *SQLiteStore) SetChannelWing(channel, externalID, wing, source string, confidence float64) error {
	wNorm := NormalizeWing(wing)
	if wNorm == "" {
		return fmt.Errorf("invalid wing name: %q", wing)
	}
	if channel == "" || externalID == "" {
		return fmt.Errorf("channel and external_id must be non-empty")
	}
	validSources := map[string]bool{"manual": true, "heuristic": true, "llm": true, "inherited": true}
	if !validSources[source] {
		return fmt.Errorf("invalid source: %q", source)
	}
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence out of range [0,1]: %v", confidence)
	}

	// Ensure the wing exists in the registry (lazy creation).
	// Silently ignore registry errors — the mapping itself is the source of truth.
	_ = s.UpsertWing(wNorm, "", "")

	_, err := s.db.Exec(`
		INSERT INTO channel_wing_map (channel, external_id, wing, confidence, source, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(channel, external_id) DO UPDATE SET
			wing       = excluded.wing,
			confidence = excluded.confidence,
			source     = excluded.source,
			updated_at = CURRENT_TIMESTAMP
	`, channel, externalID, wNorm, confidence, source)
	return err
}

// DeleteChannelWing removes a channel → wing mapping. Subsequent lookups
// for the same (channel, external_id) will return ErrChannelWingNotFound.
func (s *SQLiteStore) DeleteChannelWing(channel, externalID string) error {
	_, err := s.db.Exec(`
		DELETE FROM channel_wing_map
		WHERE channel = ? AND external_id = ?
	`, channel, externalID)
	return err
}

// ListChannelWings returns all channel → wing mappings, optionally filtered
// by wing. Useful for admin/audit views showing "what maps to wing=work".
func (s *SQLiteStore) ListChannelWings(wingFilter string) ([]ChannelWingMapping, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if wingFilter == "" {
		rows, err = s.db.Query(`
			SELECT channel, external_id, wing, confidence, source, created_at, updated_at
			FROM channel_wing_map
			ORDER BY channel, external_id
		`)
	} else {
		norm := NormalizeWing(wingFilter)
		if norm == "" {
			return nil, fmt.Errorf("invalid wing filter: %q", wingFilter)
		}
		rows, err = s.db.Query(`
			SELECT channel, external_id, wing, confidence, source, created_at, updated_at
			FROM channel_wing_map
			WHERE wing = ?
			ORDER BY channel, external_id
		`, norm)
	}
	if err != nil {
		return nil, fmt.Errorf("list channel wings: %w", err)
	}
	defer rows.Close()

	var out []ChannelWingMapping
	for rows.Next() {
		var m ChannelWingMapping
		if err := rows.Scan(
			&m.Channel, &m.ExternalID, &m.Wing,
			&m.Confidence, &m.Source, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Taxonomy query (for CLI `devclaw wing list --tree` and WebUI)
// ─────────────────────────────────────────────────────────────────────────────

// TaxonomyEntry describes one wing and its rooms for tree rendering.
type TaxonomyEntry struct {
	Wing        WingInfo
	Rooms       []RoomInfo
	LegacyCount int // files with wing=NULL that would be "sibling" to this tree
}

// GetTaxonomy returns the full wing → rooms tree with counts. The result is
// suitable for CLI/WebUI rendering of the palace structure.
//
// LegacyCount on each entry is always 0 — it is computed once at the level
// of the caller by summing across all wings (see the first return value of
// TotalLegacyFiles).
func (s *SQLiteStore) GetTaxonomy() ([]TaxonomyEntry, error) {
	wings, err := s.ListWings()
	if err != nil {
		return nil, err
	}

	out := make([]TaxonomyEntry, 0, len(wings))
	for _, w := range wings {
		rooms, err := s.ListRooms(w.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, TaxonomyEntry{Wing: w, Rooms: rooms})
	}
	return out, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// File wing assignment (Sprint 2 Room 2.0b)
// ─────────────────────────────────────────────────────────────────────────────

// AssignWingToFile sets the wing column on a file row, but ONLY when the
// current wing is NULL. This conditional UPDATE is the race barrier that
// makes concurrent saves and dream classifier passes safe:
//
//   - If memory_save runs first → sets wing=X; classifier's UPDATE becomes a
//     no-op (WHERE wing IS NULL no longer matches).
//   - If classifier runs first → sets wing=Y; memory_save's UPDATE becomes a
//     no-op. Both end states are valid; no cross-contamination.
//
// The fileID is the key used by IndexDirectory/IndexChunks — for files
// written by FileStore.Save this is always "MEMORY.md".
//
// An empty wing is rejected at the caller level (NormalizeWing returns "").
// The query uses a parameterized placeholder to prevent injection.
func (s *SQLiteStore) AssignWingToFile(ctx context.Context, fileID string, wing string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE files SET wing = ? WHERE file_id = ? AND wing IS NULL`,
		wing, fileID,
	)
	return err
}

// TotalLegacyFiles returns the count of files with wing IS NULL — memories
// that predate the palace hierarchy and have not been classified. This is
// always >= 0 and is a first-class value per ADR-006 (legacy cidadão).
func (s *SQLiteStore) TotalLegacyFiles() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE wing IS NULL`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count legacy files: %w", err)
	}
	return count, nil
}

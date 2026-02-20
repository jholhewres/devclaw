// Package copilot – pairing.go implements the DM pairing system for secure access onboarding.
//
// The pairing system allows admins to generate shareable tokens that new users
// can send to the bot to request access. Tokens can be configured for:
//   - Auto-approval (immediate access) or manual approval
//   - Expiration time
//   - Maximum number of uses
//   - Role to grant (user or admin)
//   - Workspace assignment
package copilot

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TokenRole defines the access level granted by a pairing token.
type TokenRole string

const (
	TokenRoleUser  TokenRole = "user"
	TokenRoleAdmin TokenRole = "admin"
)

// PairingToken represents a shareable invite token.
type PairingToken struct {
	ID          string     `json:"id"`
	Token       string     `json:"token"`
	Role        TokenRole  `json:"role"`
	MaxUses     int        `json:"max_uses"`     // 0 = unlimited
	UseCount    int        `json:"use_count"`
	AutoApprove bool       `json:"auto_approve"` // If true, grants access immediately
	WorkspaceID string     `json:"workspace_id"`
	Note        string     `json:"note"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Revoked     bool       `json:"revoked"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	RevokedBy   string     `json:"revoked_by"`
}

// IsExpired returns true if the token has expired.
func (t *PairingToken) IsExpired() bool {
	if t.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*t.ExpiresAt)
}

// CanUse returns true if the token can still be used.
func (t *PairingToken) CanUse() bool {
	if t.Revoked || t.IsExpired() {
		return false
	}
	if t.MaxUses > 0 && t.UseCount >= t.MaxUses {
		return false
	}
	return true
}

// PairingRequest represents a pending access request.
type PairingRequest struct {
	ID         string     `json:"id"`
	TokenID    string     `json:"token_id"`
	UserJID    string     `json:"user_jid"`
	UserName   string     `json:"user_name"`
	Status     string     `json:"status"` // pending, approved, denied
	ReviewedBy string     `json:"reviewed_by"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`

	// Loaded via join for display
	TokenNote string    `json:"token_note,omitempty"`
	TokenRole TokenRole `json:"token_role,omitempty"`
}

// TokenOptions configures token generation.
type TokenOptions struct {
	Role        TokenRole
	MaxUses     int           // 0 = unlimited
	ExpiresIn   time.Duration // 0 = never expires
	AutoApprove bool
	WorkspaceID string
	Note        string
}

// PairingManager handles pairing tokens and requests.
type PairingManager struct {
	db        *sql.DB
	accessMgr *AccessManager
	wsMgr     *WorkspaceManager
	logger    *slog.Logger
	mu        sync.RWMutex

	// In-memory cache of valid tokens for fast lookup
	tokenCache map[string]*PairingToken
}

// NewPairingManager creates a new pairing manager.
func NewPairingManager(db *sql.DB, accessMgr *AccessManager, wsMgr *WorkspaceManager, logger *slog.Logger) *PairingManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &PairingManager{
		db:         db,
		accessMgr:  accessMgr,
		wsMgr:      wsMgr,
		logger:     logger.With("component", "pairing"),
		tokenCache: make(map[string]*PairingToken),
	}
}

// Load restores token cache from database on startup.
func (pm *PairingManager) Load() error {
	if pm.db == nil {
		return nil
	}

	rows, err := pm.db.Query(`
		SELECT id, token, role, max_uses, use_count, auto_approve,
		       workspace_id, note, created_by, created_at, expires_at,
		       revoked, revoked_at, revoked_by
		FROM pairing_tokens
		WHERE revoked = 0
	`)
	if err != nil {
		return fmt.Errorf("query pairing tokens: %w", err)
	}
	defer rows.Close()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.tokenCache = make(map[string]*PairingToken)
	count := 0

	for rows.Next() {
		t := &PairingToken{}
		var expiresAt, revokedAt sql.NullString
		var maxUses, useCount int
		var autoApprove, revoked int

		err := rows.Scan(
			&t.ID, &t.Token, &t.Role, &maxUses, &useCount, &autoApprove,
			&t.WorkspaceID, &t.Note, &t.CreatedBy, &t.CreatedAt, &expiresAt,
			&revoked, &revokedAt, &t.RevokedBy,
		)
		if err != nil {
			pm.logger.Warn("failed to scan token", "error", err)
			continue
		}

		t.MaxUses = maxUses
		t.UseCount = useCount
		t.AutoApprove = autoApprove == 1
		t.Revoked = revoked == 1

		if expiresAt.Valid && expiresAt.String != "" {
			et, err := time.Parse(time.RFC3339, expiresAt.String)
			if err == nil {
				t.ExpiresAt = &et
			}
		}

		if revokedAt.Valid && revokedAt.String != "" {
			rt, err := time.Parse(time.RFC3339, revokedAt.String)
			if err == nil {
				t.RevokedAt = &rt
			}
		}

		pm.tokenCache[t.Token] = t
		count++
	}

	pm.logger.Info("loaded pairing tokens", "count", count)
	return nil
}

// GenerateToken creates a new pairing token.
func (pm *PairingManager) GenerateToken(createdBy string, opts TokenOptions) (*PairingToken, error) {
	// Generate secure random token (24 bytes = 48 hex chars).
	tokenStr, err := generateSecureToken(24)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	if opts.Role == "" {
		opts.Role = TokenRoleUser
	}

	now := time.Now()
	t := &PairingToken{
		ID:          uuid.New().String(),
		Token:       tokenStr,
		Role:        opts.Role,
		MaxUses:     opts.MaxUses,
		UseCount:    0,
		AutoApprove: opts.AutoApprove,
		WorkspaceID: opts.WorkspaceID,
		Note:        opts.Note,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		Revoked:     false,
	}

	if opts.ExpiresIn > 0 {
		et := now.Add(opts.ExpiresIn)
		t.ExpiresAt = &et
	}

	// Store in database.
	if pm.db != nil {
		var expiresAt string
		if t.ExpiresAt != nil {
			expiresAt = t.ExpiresAt.Format(time.RFC3339)
		}

		autoApprove := 0
		if t.AutoApprove {
			autoApprove = 1
		}

		_, err := pm.db.Exec(`
			INSERT INTO pairing_tokens
			(id, token, role, max_uses, use_count, auto_approve,
			 workspace_id, note, created_by, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, t.ID, t.Token, t.Role, t.MaxUses, t.UseCount, autoApprove,
			t.WorkspaceID, t.Note, t.CreatedBy, t.CreatedAt.Format(time.RFC3339), expiresAt)
		if err != nil {
			return nil, fmt.Errorf("insert token: %w", err)
		}
	}

	// Add to cache.
	pm.mu.Lock()
	pm.tokenCache[t.Token] = t
	pm.mu.Unlock()

	pm.logger.Info("generated pairing token",
		"id", t.ID,
		"role", t.Role,
		"auto_approve", t.AutoApprove,
		"max_uses", t.MaxUses,
		"created_by", createdBy,
	)

	return t, nil
}

// ValidateToken checks if a token is valid and returns it.
func (pm *PairingManager) ValidateToken(token string) (*PairingToken, error) {
	pm.mu.RLock()
	t, ok := pm.tokenCache[token]
	pm.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("token not found")
	}

	if t.Revoked {
		return nil, fmt.Errorf("token has been revoked")
	}

	if t.IsExpired() {
		return nil, fmt.Errorf("token has expired")
	}

	if t.MaxUses > 0 && t.UseCount >= t.MaxUses {
		return nil, fmt.Errorf("token has reached maximum uses")
	}

	return t, nil
}

// ProcessTokenRedemption handles a user sending a token.
// Returns: (approved, message, error)
func (pm *PairingManager) ProcessTokenRedemption(tokenStr, userJID, userName string) (bool, string, error) {
	token, err := pm.ValidateToken(tokenStr)
	if err != nil {
		return false, fmt.Sprintf("Invalid token: %v", err), nil
	}

	// Check if user already has access.
	if level := pm.accessMgr.GetLevel(userJID); level >= AccessUser {
		return false, "You already have access to this bot.", nil
	}

	// Auto-approve: grant access immediately.
	if token.AutoApprove {
		level := AccessUser
		if token.Role == TokenRoleAdmin {
			level = AccessAdmin
		}

		if err := pm.accessMgr.Grant(userJID, level, "pairing:"+token.ID); err != nil {
			return false, "", fmt.Errorf("grant access: %w", err)
		}

		// Assign to workspace if specified.
		if token.WorkspaceID != "" && pm.wsMgr != nil {
			_ = pm.wsMgr.AssignUser(userJID, token.WorkspaceID, "pairing:"+token.ID)
		}

		// Increment use count.
		pm.incrementUseCount(token.ID)

		pm.logger.Info("auto-approved via pairing token",
			"token_id", token.ID,
			"user_jid", userJID,
			"role", token.Role,
		)

		return true, fmt.Sprintf("Access granted! You have been approved as %s. Welcome!", token.Role), nil
	}

	// Not auto-approve: create pending request.
	request, err := pm.CreateRequest(token.ID, userJID, userName)
	if err != nil {
		return false, "", fmt.Errorf("create request: %w", err)
	}

	pm.logger.Info("created pairing request",
		"request_id", request.ID,
		"token_id", token.ID,
		"user_jid", userJID,
	)

	return false, fmt.Sprintf("Access request submitted! An administrator will review your request. Request ID: %s", request.ID[:8]), nil
}

// CreateRequest creates a pending pairing request.
func (pm *PairingManager) CreateRequest(tokenID, userJID, userName string) (*PairingRequest, error) {
	now := time.Now()
	r := &PairingRequest{
		ID:        uuid.New().String(),
		TokenID:   tokenID,
		UserJID:   userJID,
		UserName:  userName,
		Status:    "pending",
		CreatedAt: now,
	}

	if pm.db != nil {
		_, err := pm.db.Exec(`
			INSERT INTO pairing_requests
			(id, token_id, user_jid, user_name, status, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, r.ID, r.TokenID, r.UserJID, r.UserName, r.Status, r.CreatedAt.Format(time.RFC3339))
		if err != nil {
			return nil, fmt.Errorf("insert request: %w", err)
		}
	}

	return r, nil
}

// ApproveRequest approves a pending request and grants access.
func (pm *PairingManager) ApproveRequest(requestID, approvedBy string) error {
	if pm.db == nil {
		return fmt.Errorf("database not available")
	}

	// Get request details.
	var tokenID, userJID string
	var status string
	err := pm.db.QueryRow(`
		SELECT token_id, user_jid, status
		FROM pairing_requests
		WHERE id = ?
	`, requestID).Scan(&tokenID, &userJID, &status)
	if err == sql.ErrNoRows {
		return fmt.Errorf("request not found")
	}
	if err != nil {
		return fmt.Errorf("query request: %w", err)
	}

	if status != "pending" {
		return fmt.Errorf("request already %s", status)
	}

	// Get token to determine role.
	pm.mu.RLock()
	token, ok := pm.tokenCache[tokenID]
	pm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("token not found")
	}

	// Grant access.
	level := AccessUser
	if token.Role == TokenRoleAdmin {
		level = AccessAdmin
	}

	if err := pm.accessMgr.Grant(userJID, level, approvedBy); err != nil {
		return fmt.Errorf("grant access: %w", err)
	}

	// Assign to workspace if specified.
	if token.WorkspaceID != "" && pm.wsMgr != nil {
		_ = pm.wsMgr.AssignUser(userJID, token.WorkspaceID, approvedBy)
	}

	// Update request status.
	now := time.Now()
	_, err = pm.db.Exec(`
		UPDATE pairing_requests
		SET status = 'approved', reviewed_by = ?, reviewed_at = ?
		WHERE id = ?
	`, approvedBy, now.Format(time.RFC3339), requestID)
	if err != nil {
		return fmt.Errorf("update request: %w", err)
	}

	// Increment token use count.
	pm.incrementUseCount(tokenID)

	pm.logger.Info("approved pairing request",
		"request_id", requestID,
		"user_jid", userJID,
		"approved_by", approvedBy,
	)

	return nil
}

// DenyRequest denies a pending request.
func (pm *PairingManager) DenyRequest(requestID, deniedBy, reason string) error {
	if pm.db == nil {
		return fmt.Errorf("database not available")
	}

	// Check request exists and is pending.
	var status string
	err := pm.db.QueryRow(`
		SELECT status FROM pairing_requests WHERE id = ?
	`, requestID).Scan(&status)
	if err == sql.ErrNoRows {
		return fmt.Errorf("request not found")
	}
	if err != nil {
		return fmt.Errorf("query request: %w", err)
	}

	if status != "pending" {
		return fmt.Errorf("request already %s", status)
	}

	// Update request status.
	now := time.Now()
	_, err = pm.db.Exec(`
		UPDATE pairing_requests
		SET status = 'denied', reviewed_by = ?, reviewed_at = ?
		WHERE id = ?
	`, deniedBy, now.Format(time.RFC3339), requestID)
	if err != nil {
		return fmt.Errorf("update request: %w", err)
	}

	pm.logger.Info("denied pairing request",
		"request_id", requestID,
		"denied_by", deniedBy,
		"reason", reason,
	)

	return nil
}

// ListTokens returns all tokens (with optional filter).
func (pm *PairingManager) ListTokens(includeRevoked bool) ([]*PairingToken, error) {
	if pm.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	query := `
		SELECT id, token, role, max_uses, use_count, auto_approve,
		       workspace_id, note, created_by, created_at, expires_at,
		       revoked, revoked_at, revoked_by
		FROM pairing_tokens
	`
	if !includeRevoked {
		query += " WHERE revoked = 0"
	}
	query += " ORDER BY created_at DESC"

	rows, err := pm.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*PairingToken
	for rows.Next() {
		t := &PairingToken{}
		var expiresAt, revokedAt sql.NullString
		var maxUses, useCount int
		var autoApprove, revoked int

		err := rows.Scan(
			&t.ID, &t.Token, &t.Role, &maxUses, &useCount, &autoApprove,
			&t.WorkspaceID, &t.Note, &t.CreatedBy, &t.CreatedAt, &expiresAt,
			&revoked, &revokedAt, &t.RevokedBy,
		)
		if err != nil {
			continue
		}

		t.MaxUses = maxUses
		t.UseCount = useCount
		t.AutoApprove = autoApprove == 1
		t.Revoked = revoked == 1

		if expiresAt.Valid && expiresAt.String != "" {
			et, err := time.Parse(time.RFC3339, expiresAt.String)
			if err == nil {
				t.ExpiresAt = &et
			}
		}

		if revokedAt.Valid && revokedAt.String != "" {
			rt, err := time.Parse(time.RFC3339, revokedAt.String)
			if err == nil {
				t.RevokedAt = &rt
			}
		}

		tokens = append(tokens, t)
	}

	return tokens, nil
}

// ListPendingRequests returns all pending requests.
func (pm *PairingManager) ListPendingRequests() ([]*PairingRequest, error) {
	if pm.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	rows, err := pm.db.Query(`
		SELECT r.id, r.token_id, r.user_jid, r.user_name, r.status,
		       r.reviewed_by, r.reviewed_at, r.created_at,
		       t.note, t.role
		FROM pairing_requests r
		JOIN pairing_tokens t ON r.token_id = t.id
		WHERE r.status = 'pending'
		ORDER BY r.created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query requests: %w", err)
	}
	defer rows.Close()

	var requests []*PairingRequest
	for rows.Next() {
		r := &PairingRequest{}
		var reviewedAt sql.NullString

		err := rows.Scan(
			&r.ID, &r.TokenID, &r.UserJID, &r.UserName, &r.Status,
			&r.ReviewedBy, &reviewedAt, &r.CreatedAt,
			&r.TokenNote, &r.TokenRole,
		)
		if err != nil {
			continue
		}

		if reviewedAt.Valid && reviewedAt.String != "" {
			rt, err := time.Parse(time.RFC3339, reviewedAt.String)
			if err == nil {
				r.ReviewedAt = &rt
			}
		}

		requests = append(requests, r)
	}

	return requests, nil
}

// RevokeToken revokes a token.
func (pm *PairingManager) RevokeToken(tokenID, revokedBy string) error {
	if pm.db == nil {
		return fmt.Errorf("database not available")
	}

	now := time.Now()
	_, err := pm.db.Exec(`
		UPDATE pairing_tokens
		SET revoked = 1, revoked_at = ?, revoked_by = ?
		WHERE id = ?
	`, now.Format(time.RFC3339), revokedBy, tokenID)
	if err != nil {
		return fmt.Errorf("update token: %w", err)
	}

	// Remove from cache.
	pm.mu.Lock()
	for token, t := range pm.tokenCache {
		if t.ID == tokenID {
			delete(pm.tokenCache, token)
			break
		}
	}
	pm.mu.Unlock()

	pm.logger.Info("revoked pairing token",
		"token_id", tokenID,
		"revoked_by", revokedBy,
	)

	return nil
}

// GetTokenByIDOrPrefix finds a token by ID or token prefix.
func (pm *PairingManager) GetTokenByIDOrPrefix(idOrPrefix string) (*PairingToken, error) {
	// Try exact ID match first.
	if pm.db != nil {
		t := &PairingToken{}
		var expiresAt, revokedAt sql.NullString
		var maxUses, useCount int
		var autoApprove, revoked int

		err := pm.db.QueryRow(`
			SELECT id, token, role, max_uses, use_count, auto_approve,
			       workspace_id, note, created_by, created_at, expires_at,
			       revoked, revoked_at, revoked_by
			FROM pairing_tokens
			WHERE id = ? OR token LIKE ?
		`, idOrPrefix, idOrPrefix+"%").Scan(
			&t.ID, &t.Token, &t.Role, &maxUses, &useCount, &autoApprove,
			&t.WorkspaceID, &t.Note, &t.CreatedBy, &t.CreatedAt, &expiresAt,
			&revoked, &revokedAt, &t.RevokedBy,
		)
		if err == nil {
			t.MaxUses = maxUses
			t.UseCount = useCount
			t.AutoApprove = autoApprove == 1
			t.Revoked = revoked == 1

			if expiresAt.Valid && expiresAt.String != "" {
				et, _ := time.Parse(time.RFC3339, expiresAt.String)
				t.ExpiresAt = &et
			}

			return t, nil
		}
	}

	return nil, fmt.Errorf("token not found")
}

// incrementUseCount increments the use count for a token.
func (pm *PairingManager) incrementUseCount(tokenID string) {
	if pm.db == nil {
		return
	}

	_, _ = pm.db.Exec(`UPDATE pairing_tokens SET use_count = use_count + 1 WHERE id = ?`, tokenID)

	// Update cache.
	pm.mu.Lock()
	for _, t := range pm.tokenCache {
		if t.ID == tokenID {
			t.UseCount++
			break
		}
	}
	pm.mu.Unlock()
}

// generateSecureToken creates a cryptographically secure random token.
func generateSecureToken(byteLength int) (string, error) {
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ExtractTokenFromMessage attempts to extract a pairing token from message content.
// Tokens are 48+ hex characters. Returns empty string if not a token attempt.
func ExtractTokenFromMessage(content string) string {
	content = strings.TrimSpace(content)
	content = strings.ToLower(content)

	// Direct token: 48+ hex characters
	if len(content) >= 48 && isHexString(content) {
		return content
	}

	// "token: <hex>" format
	if strings.HasPrefix(content, "token:") {
		token := strings.TrimSpace(strings.TrimPrefix(content, "token:"))
		if len(token) >= 48 && isHexString(token) {
			return token
		}
	}

	return ""
}

// isHexString checks if a string is all lowercase hex characters.
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

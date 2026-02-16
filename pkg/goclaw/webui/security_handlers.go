// Package webui – security_handlers.go implements the security dashboard
// API endpoints: audit log, tool guard config, vault status, and API keys.
package webui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// ── Types for Security API ──

// AuditEntry represents a single audit log record for the UI.
type AuditEntry struct {
	ID            int64  `json:"id"`
	Tool          string `json:"tool"`
	Caller        string `json:"caller"`
	Level         string `json:"level"`
	Allowed       bool   `json:"allowed"`
	ArgsSummary   string `json:"args_summary"`
	ResultSummary string `json:"result_summary"`
	CreatedAt     string `json:"created_at"`
}

// ToolGuardStatus represents the tool guard configuration for the UI.
type ToolGuardStatus struct {
	Enabled             bool              `json:"enabled"`
	AllowDestructive    bool              `json:"allow_destructive"`
	AllowSudo           bool              `json:"allow_sudo"`
	AllowReboot         bool              `json:"allow_reboot"`
	AutoApprove         []string          `json:"auto_approve"`
	RequireConfirmation []string          `json:"require_confirmation"`
	ProtectedPaths      []string          `json:"protected_paths"`
	SSHAllowedHosts     []string          `json:"ssh_allowed_hosts"`
	DangerousCommands   []string          `json:"dangerous_commands"`
	ToolPermissions     map[string]string `json:"tool_permissions"`
}

// VaultStatus represents the vault state for the UI (no secret values).
type VaultStatus struct {
	Exists   bool     `json:"exists"`
	Unlocked bool     `json:"unlocked"`
	Keys     []string `json:"keys"`
}

// SecurityStatus is an overview returned at /api/security.
type SecurityStatus struct {
	GatewayAuthConfigured bool `json:"gateway_auth_configured"`
	WebUIAuthConfigured   bool `json:"webui_auth_configured"`
	ToolGuardEnabled      bool `json:"tool_guard_enabled"`
	VaultExists           bool `json:"vault_exists"`
	VaultUnlocked         bool `json:"vault_unlocked"`
	AuditEntryCount       int  `json:"audit_entry_count"`
}

// ── Handlers ──

// handleAPISecurity dispatches /api/security/* requests.
func (s *Server) handleAPISecurity(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/security")
	path = strings.TrimPrefix(path, "/")

	switch {
	case path == "" || path == "/":
		s.handleSecurityOverview(w, r)
	case path == "audit" || strings.HasPrefix(path, "audit"):
		s.handleSecurityAudit(w, r)
	case path == "tool-guard" || strings.HasPrefix(path, "tool-guard"):
		s.handleSecurityToolGuard(w, r)
	case path == "vault" || strings.HasPrefix(path, "vault"):
		s.handleSecurityVault(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleSecurityOverview returns a high-level security summary.
func (s *Server) handleSecurityOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	status := s.api.GetSecurityStatus()
	writeJSON(w, http.StatusOK, status)
}

// handleSecurityAudit returns audit log entries.
func (s *Server) handleSecurityAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	entries := s.api.GetAuditLog(limit)
	if entries == nil {
		entries = []AuditEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"total":   s.api.GetAuditCount(),
	})
}

// handleSecurityToolGuard returns or updates the tool guard config.
func (s *Server) handleSecurityToolGuard(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := s.api.GetToolGuardStatus()
		writeJSON(w, http.StatusOK, status)

	case http.MethodPut:
		var update ToolGuardStatus
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if err := s.api.UpdateToolGuard(update); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleSecurityVault returns vault status (no secret values).
func (s *Server) handleSecurityVault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	status := s.api.GetVaultStatus()
	writeJSON(w, http.StatusOK, status)
}

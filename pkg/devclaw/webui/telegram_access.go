package webui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleAPITelegramAccess handles GET /api/channels/telegram/access and PATCH for default policy.
func (s *Server) handleAPITelegramAccess(w http.ResponseWriter, r *http.Request) {
	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg := adapter.GetTelegramAccessConfig()
		writeJSON(w, http.StatusOK, cfg)

	case http.MethodPatch:
		var req struct {
			DefaultPolicy *string `json:"default_policy"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if req.DefaultPolicy != nil {
			if err := adapter.UpdateTelegramAccessDefaultPolicy(*req.DefaultPolicy); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPITelegramAccessUser handles POST/DELETE /api/channels/telegram/access/users/:id
func (s *Server) handleAPITelegramAccessUser(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/channels/telegram/access/users/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user ID required"})
		return
	}

	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Level string `json:"level"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if err := adapter.GrantTelegramUserAccess(path, req.Level); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"status": "granted", "id": path, "level": req.Level})

	case http.MethodDelete:
		if err := adapter.RevokeTelegramUserAccess(path); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"status": "revoked", "id": path})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPITelegramBlockedUser handles POST/DELETE /api/channels/telegram/access/blocked/:id
func (s *Server) handleAPITelegramBlockedUser(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/channels/telegram/access/blocked/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user ID required"})
		return
	}

	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	switch r.Method {
	case http.MethodPost:
		if err := adapter.BlockTelegramUser(path); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"status": "blocked", "id": path})

	case http.MethodDelete:
		if err := adapter.UnblockTelegramUser(path); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"status": "unblocked", "id": path})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

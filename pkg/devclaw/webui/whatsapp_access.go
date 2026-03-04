package webui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleAPIWhatsAppAccess handles GET /api/channels/whatsapp/access and PATCH for default policy
func (s *Server) handleAPIWhatsAppAccess(w http.ResponseWriter, r *http.Request) {
	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg := adapter.GetWhatsAppAccessConfig()
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
			if err := adapter.UpdateWhatsAppAccessDefaultPolicy(*req.DefaultPolicy); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPIWhatsAppAccessUser handles POST/DELETE /api/channels/whatsapp/access/users/:jid
func (s *Server) handleAPIWhatsAppAccessUser(w http.ResponseWriter, r *http.Request) {
	// Extract JID from path: /api/channels/whatsapp/access/users/:jid
	path := strings.TrimPrefix(r.URL.Path, "/api/channels/whatsapp/access/users/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user JID required"})
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

		if err := adapter.GrantWhatsAppUserAccess(path, req.Level); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"status": "granted", "jid": path, "level": req.Level})

	case http.MethodDelete:
		if err := adapter.RevokeWhatsAppUserAccess(path); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"status": "revoked", "jid": path})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPIWhatsAppBlockedUser handles POST/DELETE /api/channels/whatsapp/access/blocked/:jid
func (s *Server) handleAPIWhatsAppBlockedUser(w http.ResponseWriter, r *http.Request) {
	// Extract JID from path: /api/channels/whatsapp/access/blocked/:jid
	path := strings.TrimPrefix(r.URL.Path, "/api/channels/whatsapp/access/blocked/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user JID required"})
		return
	}

	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	switch r.Method {
	case http.MethodPost:
		if err := adapter.BlockWhatsAppUser(path); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"status": "blocked", "jid": path})

	case http.MethodDelete:
		if err := adapter.UnblockWhatsAppUser(path); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"status": "unblocked", "jid": path})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPIWhatsAppJoinedGroups handles GET /api/channels/whatsapp/groups/joined
func (s *Server) handleAPIWhatsAppJoinedGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	groups, err := adapter.GetWhatsAppJoinedGroups()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, groups)
}

// handleAPIWhatsAppGroups handles GET/PATCH /api/channels/whatsapp/groups and PUT /api/channels/whatsapp/groups/:jid
func (s *Server) handleAPIWhatsAppGroups(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/channels/whatsapp/groups")

	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	// GET /api/channels/whatsapp/groups - list all group policies
	if r.Method == http.MethodGet && path == "" {
		policies := adapter.GetWhatsAppGroupPolicies()
		writeJSON(w, http.StatusOK, policies)
		return
	}

	// PATCH /api/channels/whatsapp/groups - update default policy
	if r.Method == http.MethodPatch && path == "" {
		var req struct {
			DefaultPolicy *string `json:"default_policy"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if req.DefaultPolicy != nil {
			if err := adapter.UpdateWhatsAppGroupDefaultPolicy(*req.DefaultPolicy); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
		return
	}

	// PUT /api/channels/whatsapp/groups/:jid - set group policy
	if r.Method == http.MethodPut && path != "" {
		groupJID := strings.TrimPrefix(path, "/")

		var req struct {
			Name         string   `json:"name"`
			Policy       string   `json:"policy"`
			Policies     []string `json:"policies,omitempty"`
			Keywords     []string `json:"keywords,omitempty"`
			AllowedUsers []string `json:"allowed_users,omitempty"`
			Workspace    string   `json:"workspace,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		policy := map[string]any{
			"name":          req.Name,
			"policy":        req.Policy,
			"policies":      req.Policies,
			"keywords":      req.Keywords,
			"allowed_users": req.AllowedUsers,
			"workspace":     req.Workspace,
		}

		if err := adapter.SetWhatsAppGroupPolicy(groupJID, policy); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "group": groupJID})
		return
	}

	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

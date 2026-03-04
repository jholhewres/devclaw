package webui

import (
	"encoding/json"
	"net/http"
)

// handleAPIWhatsAppConfig handles GET/PATCH /api/channels/whatsapp/config
func (s *Server) handleAPIWhatsAppConfig(w http.ResponseWriter, r *http.Request) {
	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Return current WhatsApp settings
		config := adapter.GetWhatsAppConfig()
		writeJSON(w, http.StatusOK, config)

	case http.MethodPatch:
		var config map[string]any
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if err := adapter.UpdateWhatsAppConfig(config); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

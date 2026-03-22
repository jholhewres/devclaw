package webui

import (
	"encoding/json"
	"net/http"
)

// TelegramConfig holds Telegram settings exposed to the UI.
type TelegramConfig struct {
	Connected           bool    `json:"connected"`
	BotUsername         string  `json:"bot_username,omitempty"`
	BotID               int64  `json:"bot_id,omitempty"`
	RespondToGroups     bool    `json:"respond_to_groups"`
	RespondToDMs        bool    `json:"respond_to_dms"`
	SendTyping          bool    `json:"send_typing"`
	AllowedChats        []int64 `json:"allowed_chats"`
	ReactionNotifications string `json:"reaction_notifications"`
}

// handleAPITelegramConfig handles GET/PATCH /api/channels/telegram/config
func (s *Server) handleAPITelegramConfig(w http.ResponseWriter, r *http.Request) {
	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		config := adapter.GetTelegramConfig()
		writeJSON(w, http.StatusOK, config)

	case http.MethodPatch:
		var config map[string]any
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if err := adapter.UpdateTelegramConfig(config); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPITelegramDisconnect handles POST /api/channels/telegram/disconnect
func (s *Server) handleAPITelegramDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	if err := adapter.DisconnectTelegram(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

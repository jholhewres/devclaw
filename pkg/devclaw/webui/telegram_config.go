package webui

import (
	"encoding/json"
	"net/http"
)

// TelegramAccessConfig holds the Telegram access control configuration for the UI.
type TelegramAccessConfig struct {
	DefaultPolicy string   `json:"default_policy"`
	Owners        []string `json:"owners"`
	Admins        []string `json:"admins"`
	AllowedUsers  []string `json:"allowed_users"`
	BlockedUsers  []string `json:"blocked_users"`
}

// TelegramConfig holds Telegram settings exposed to the UI.
type TelegramConfig struct {
	Connected             bool    `json:"connected"`
	Configured            bool    `json:"configured"`
	BotUsername           string  `json:"bot_username,omitempty"`
	BotID                 int64   `json:"bot_id,omitempty"`
	RespondToGroups       bool    `json:"respond_to_groups"`
	RespondToDMs          bool    `json:"respond_to_dms"`
	SendTyping            bool    `json:"send_typing"`
	AllowedChats          []int64 `json:"allowed_chats"`
	ReactionNotifications string  `json:"reaction_notifications"`
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

// handleAPITelegramConnect handles POST /api/channels/telegram/connect
// Accepts {"token": "..."} to set the bot token and start the channel.
func (s *Server) handleAPITelegramConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	adapter, ok := s.api.(*AssistantAdapter)
	if !ok || adapter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant not available"})
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}

	if err := adapter.ConnectTelegram(body.Token); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

// handleAPITelegramDisconnect handles POST /api/channels/telegram/disconnect
// Disconnects and removes the token from config.
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

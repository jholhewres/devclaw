package webui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ── Dashboard ──

func (s *Server) handleAPIDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	data := map[string]any{
		"sessions": s.api.ListSessions(),
		"usage":    s.api.GetUsageGlobal(),
		"channels": s.api.GetChannelHealth(),
		"jobs":     s.api.GetSchedulerJobs(),
	}
	writeJSON(w, http.StatusOK, data)
}

// ── Sessions ──

func (s *Server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.api.ListSessions())
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleAPISessionDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(path, "/", 2)
	sessionID := parts[0]

	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session ID"})
		return
	}

	// GET /api/sessions/{id}/messages
	if len(parts) > 1 && parts[1] == "messages" {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, s.api.GetSessionMessages(sessionID))
			return
		}
	}

	// DELETE /api/sessions/{id}
	if r.Method == http.MethodDelete {
		if err := s.api.DeleteSession(sessionID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

// ── Chat ──

func (s *Server) handleAPIChat(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/chat/{sessionId}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/chat/")
	parts := strings.SplitN(path, "/", 2)
	sessionID := parts[0]

	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session ID"})
		return
	}

	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch action {
	case "send":
		s.handleChatSend(w, r, sessionID)
	case "history":
		s.handleChatHistory(w, r, sessionID)
	case "abort":
		s.handleChatAbort(w, r, sessionID)
	case "stream":
		s.handleChatStream(w, r, sessionID)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
	}
}

// handleChatSend starts an agent run and returns a run_id for SSE streaming.
func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing content"})
		return
	}

	// Start a streaming agent run.
	handle, err := s.api.StartChatStream(r.Context(), sessionID, body.Content)
	if err != nil {
		s.logger.Error("chat send failed", "session", sessionID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Register the run so the /stream endpoint can find it.
	s.registerRun(handle)

	writeJSON(w, http.StatusOK, map[string]string{"run_id": handle.RunID})
}

func (s *Server) handleChatHistory(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, s.api.GetSessionMessages(sessionID))
}

func (s *Server) handleChatAbort(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Cancel via the webui's active stream registry (primary path for web UI runs).
	stopped := false
	s.activeStreamMu.Lock()
	for runID, handle := range s.activeStreams {
		if handle.SessionID == sessionID {
			handle.Cancel()
			delete(s.activeStreams, runID)
			stopped = true
			break
		}
	}
	s.activeStreamMu.Unlock()

	// Fallback: try via the assistant's active runs (for channel-driven runs).
	if !stopped {
		stopped = s.api.AbortRun(sessionID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"stopped": stopped})
}

// handleChatStream serves SSE events for an active agent run.
// The frontend connects here after receiving a run_id from /send.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request, sessionID string) {
	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing run_id"})
		return
	}

	// Look up the active run.
	handle := s.getRun(runID)
	if handle == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found or already completed"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Stream events from the run handle until the channel is closed.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected — cancel the agent run and clean up.
			s.logger.Debug("SSE client disconnected", "run_id", runID)
			handle.Cancel()
			s.unregisterRun(runID)
			return

		case event, ok := <-handle.Events:
			if !ok {
				// Channel closed — run completed. Clean up.
				handle.Cancel() // Ensure context resources are released.
				s.unregisterRun(runID)
				return
			}
			writeSSE(w, flusher, event.Type, event.Data)
		}
	}
}

// ── Skills ──

func (s *Server) handleAPISkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.api.ListSkills())
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ── Channels ──

func (s *Server) handleAPIChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.api.GetChannelHealth())
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ── Config ──

func (s *Server) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.api.GetConfigMap())
	case http.MethodPut:
		// TODO: implement config update
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ── Usage ──

func (s *Server) handleAPIUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, s.api.GetUsageGlobal())
}

// ── Jobs ──

func (s *Server) handleAPIJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.api.GetSchedulerJobs())
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}


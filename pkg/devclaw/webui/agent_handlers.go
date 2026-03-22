package webui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleAPIAgents handles /api/agents:
//   - GET  — list all agents
//   - POST — create a new agent
func (s *Server) handleAPIAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.api.ListAgents()
		if list == nil {
			list = []AgentInfoAPI{}
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
		var req CreateAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		id, err := s.api.CreateAgent(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "id": id})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPIAgentAction handles /api/agents/{id}/* actions:
//   - GET    /api/agents/{id}          — agent detail
//   - PUT    /api/agents/{id}          — update agent
//   - DELETE /api/agents/{id}          — delete agent
//   - POST   /api/agents/{id}/toggle   — enable/disable
//   - POST   /api/agents/{id}/default  — set as default
func (s *Server) handleAPIAgentAction(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/agents/{id} or /api/agents/{id}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent id required"})
		return
	}

	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			s.handleAgentDetail(w, r, id)
		case http.MethodPut:
			s.handleAgentUpdate(w, r, id)
		case http.MethodDelete:
			s.handleAgentDelete(w, r, id)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	case "toggle":
		s.handleAgentToggle(w, r, id)
	case "default":
		s.handleAgentSetDefault(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
	}
}

// agentErrorStatus returns the appropriate HTTP status for an agent error.
func agentErrorStatus(err error) int {
	msg := err.Error()
	if strings.Contains(msg, "not found") {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

// handleAgentDetail returns full agent info.
func (s *Server) handleAgentDetail(w http.ResponseWriter, _ *http.Request, id string) {
	info, err := s.api.GetAgent(id)
	if err != nil {
		writeJSON(w, agentErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleAgentUpdate updates an agent's configuration.
func (s *Server) handleAgentUpdate(w http.ResponseWriter, r *http.Request, id string) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	var req UpdateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if err := s.api.UpdateAgent(id, req); err != nil {
		writeJSON(w, agentErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAgentDelete removes an agent.
func (s *Server) handleAgentDelete(w http.ResponseWriter, _ *http.Request, id string) {
	if err := s.api.DeleteAgent(id); err != nil {
		writeJSON(w, agentErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAgentToggle enables/disables an agent.
func (s *Server) handleAgentToggle(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if err := s.api.ToggleAgent(id, body.Active); err != nil {
		writeJSON(w, agentErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAgentSetDefault sets an agent as the default.
func (s *Server) handleAgentSetDefault(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if err := s.api.SetDefaultAgent(id); err != nil {
		writeJSON(w, agentErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

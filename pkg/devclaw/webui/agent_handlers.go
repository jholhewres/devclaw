package webui

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// ErrAgentNotFound is returned when an agent/workspace is not found.
var ErrAgentNotFound = errors.New("agent not found")

// handleAPIModels handles GET /api/models — returns available LLM models.
func (s *Server) handleAPIModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	models := s.api.ListModels()
	if models == nil {
		models = []ModelInfo{}
	}
	writeJSON(w, http.StatusOK, models)
}

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
	case "files":
		s.handleAgentFilesAction(w, r, id)
	default:
		// Check for files/{filename} pattern
		if strings.HasPrefix(action, "files/") {
			filename := strings.TrimPrefix(action, "files/")
			s.handleAgentFileUpdate(w, r, id, filename)
		} else {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
		}
	}
}

// agentErrorStatus returns the appropriate HTTP status for an agent error.
func agentErrorStatus(err error) int {
	if errors.Is(err, ErrAgentNotFound) {
		return http.StatusNotFound
	}
	// Fallback: check message for backwards compatibility with errors not yet
	// wrapped with ErrAgentNotFound.
	if strings.Contains(err.Error(), "not found") {
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

// handleAgentFilesAction handles GET /api/agents/{id}/files — list workspace files.
func (s *Server) handleAgentFilesAction(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	result, err := s.api.ListAgentFiles(id)
	if err != nil {
		writeJSON(w, agentErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleAgentFileUpdate handles PUT /api/agents/{id}/files/{filename} — update a workspace file.
func (s *Server) handleAgentFileUpdate(w http.ResponseWriter, r *http.Request, id, filename string) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Validate filename (prevent path traversal)
	allowed := map[string]bool{
		"SOUL.md": true, "IDENTITY.md": true, "TOOLS.md": true,
		"MEMORY.md": true, "AGENTS.md": true, "HEARTBEAT.md": true,
	}
	if !allowed[filename] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if err := s.api.UpdateAgentFile(id, filename, req.Content); err != nil {
		writeJSON(w, agentErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

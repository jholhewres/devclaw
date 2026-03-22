package webui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleAPIPlugins handles /api/plugins:
//   - GET  — list all plugins
//   - POST — install a plugin from source
func (s *Server) handleAPIPlugins(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.api.ListPlugins()
		if list == nil {
			list = []PluginInfoAPI{}
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		s.handlePluginInstall(w, r)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPIPluginAction handles /api/plugins/{id}/* actions:
//   - GET    /api/plugins/{id}          — plugin detail
//   - PUT    /api/plugins/{id}/config   — update config
//   - POST   /api/plugins/{id}/toggle   — enable/disable
//   - DELETE /api/plugins/{id}          — remove plugin
func (s *Server) handleAPIPluginAction(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/plugins/{id} or /api/plugins/{id}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/plugins/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plugin id required"})
		return
	}

	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch action {
	case "":
		if r.Method == http.MethodDelete {
			s.handlePluginRemove(w, r, id)
			return
		}
		s.handlePluginDetail(w, r, id)
	case "config":
		s.handlePluginConfig(w, r, id)
	case "toggle":
		s.handlePluginToggle(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
	}
}

// handlePluginDetail returns full plugin info including config schema and UI.
func (s *Server) handlePluginDetail(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	info := s.api.GetPluginInfo(id)
	if info == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "plugin not found"})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handlePluginConfig updates plugin configuration.
func (s *Server) handlePluginConfig(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if err := s.api.ConfigurePlugin(id, updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handlePluginToggle enables/disables a plugin.
func (s *Server) handlePluginToggle(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if err := s.api.TogglePlugin(id, body.Enabled); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handlePluginInstall installs a plugin from a source (GitHub repo or local path).
func (s *Server) handlePluginInstall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Source == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source is required"})
		return
	}

	result, err := s.api.InstallPlugin(body.Source)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handlePluginRemove removes an installed plugin.
func (s *Server) handlePluginRemove(w http.ResponseWriter, _ *http.Request, name string) {
	if err := s.api.RemovePlugin(name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

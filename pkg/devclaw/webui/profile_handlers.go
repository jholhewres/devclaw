package webui

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/auth/profiles"
)

// ── Auth Profiles ──

// handleAPIProfiles handles listing and creating profiles
// GET /api/profiles - list all profiles
// POST /api/profiles - create a new profile
func (s *Server) handleAPIProfiles(w http.ResponseWriter, r *http.Request) {
	pm := s.api.GetProfileManager()
	if pm == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Profile manager not available. Ensure vault is unlocked.",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// List all profiles
		allProfiles := pm.List()
		result := make([]map[string]any, 0, len(allProfiles))
		for _, p := range allProfiles {
			profileData := map[string]any{
				"id":         string(p.ID),
				"provider":   p.Provider,
				"name":       p.Name,
				"mode":       string(p.Mode),
				"enabled":    p.Enabled,
				"priority":   p.Priority,
				"valid":      p.IsValid(),
				"expired":    p.IsExpired(),
				"created_at": p.CreatedAt,
				"updated_at": p.UpdatedAt,
			}
			if p.LastUsedAt != nil {
				profileData["last_used_at"] = *p.LastUsedAt
			}
			if p.OAuth != nil && p.OAuth.Email != "" {
				profileData["email"] = p.OAuth.Email
			}
			if p.LastError != "" {
				profileData["last_error"] = p.LastError
			}
			result = append(result, profileData)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"profiles": result,
			"count":    len(result),
		})

	case http.MethodPost:
		// Create a new profile
		var req struct {
			Provider string `json:"provider"`
			Name     string `json:"name"`
			Mode     string `json:"mode"`
			APIKey   string `json:"api_key,omitempty"`
			Token    string `json:"token,omitempty"`
			Priority int    `json:"priority,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		if req.Provider == "" || req.Name == "" || req.Mode == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider, name, and mode are required"})
			return
		}

		// Validate provider
		if _, ok := profiles.GetProvider(req.Provider); !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown provider: " + req.Provider})
			return
		}

		// Create profile
		profile := &profiles.AuthProfile{
			ID:        profiles.NewProfileID(req.Provider, req.Name),
			Provider:  req.Provider,
			Name:      req.Name,
			Mode:      profiles.AuthMode(req.Mode),
			Enabled:   true,
			Priority:  req.Priority,
		}

		switch profile.Mode {
		case profiles.ModeAPIKey:
			if req.APIKey == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "api_key is required for api_key mode"})
				return
			}
			profile.APIKey = &profiles.APIKeyCredential{Key: req.APIKey}
		case profiles.ModeToken:
			if req.Token == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required for token mode"})
				return
			}
			profile.Token = &profiles.TokenCredential{Token: req.Token}
		case profiles.ModeOAuth:
			// OAuth profiles are created without credentials initially
			// Credentials are added after OAuth flow completes
			profile.OAuth = &profiles.OAuthCredential{
				Provider: req.Provider,
			}
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid mode: " + string(profile.Mode)})
			return
		}

		if err := pm.Save(profile); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save profile: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":      string(profile.ID),
			"success": true,
			"message": "Profile created successfully. For OAuth profiles, use the OAuth flow to authorize.",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPIProfileDetail handles individual profile operations
// GET /api/profiles/{id} - get profile details
// PUT /api/profiles/{id} - update profile
// DELETE /api/profiles/{id} - delete profile
// POST /api/profiles/{id}/test - test profile
func (s *Server) handleAPIProfileDetail(w http.ResponseWriter, r *http.Request) {
	pm := s.api.GetProfileManager()
	if pm == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Profile manager not available. Ensure vault is unlocked.",
		})
		return
	}

	// Extract profile ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/profiles/")

	// Check if this is a test request (POST /api/profiles/{id}/test)
	if r.Method == http.MethodPost && strings.HasSuffix(path, "/test") {
		s.handleAPIProfileTest(w, r)
		return
	}

	profileID := profiles.ProfileID(path)

	if profileID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing profile ID"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		profile, ok := pm.Get(profileID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
			return
		}

		result := map[string]any{
			"id":         string(profile.ID),
			"provider":   profile.Provider,
			"name":       profile.Name,
			"mode":       string(profile.Mode),
			"enabled":    profile.Enabled,
			"priority":   profile.Priority,
			"valid":      profile.IsValid(),
			"expired":    profile.IsExpired(),
			"created_at": profile.CreatedAt,
			"updated_at": profile.UpdatedAt,
		}
		if profile.LastUsedAt != nil {
			result["last_used_at"] = *profile.LastUsedAt
		}
		if profile.OAuth != nil {
			result["oauth"] = map[string]any{
				"email":     profile.OAuth.Email,
				"expires_at": profile.OAuth.ExpiresAt,
				"scopes":    profile.OAuth.Scopes,
			}
		}
		if profile.LastError != "" {
			result["last_error"] = profile.LastError
		}

		writeJSON(w, http.StatusOK, result)

	case http.MethodPut:
		profile, ok := pm.Get(profileID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
			return
		}

		var req struct {
			Enabled  *bool   `json:"enabled,omitempty"`
			Priority *int    `json:"priority,omitempty"`
			APIKey   *string `json:"api_key,omitempty"`
			Token    *string `json:"token,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		if req.Enabled != nil {
			profile.Enabled = *req.Enabled
		}
		if req.Priority != nil {
			profile.Priority = *req.Priority
		}
		if req.APIKey != nil && profile.Mode == profiles.ModeAPIKey {
			profile.APIKey = &profiles.APIKeyCredential{Key: *req.APIKey}
		}
		if req.Token != nil && profile.Mode == profiles.ModeToken {
			profile.Token = &profiles.TokenCredential{Token: *req.Token}
		}

		if err := pm.Save(profile); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update profile: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":      string(profile.ID),
			"success": true,
		})

	case http.MethodDelete:
		if err := pm.Delete(profileID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":      string(profileID),
			"success": true,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPIProfileTest tests a profile's credentials
// POST /api/profiles/{id}/test
func (s *Server) handleAPIProfileTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	pm := s.api.GetProfileManager()
	if pm == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Profile manager not available. Ensure vault is unlocked.",
		})
		return
	}

	// Extract profile ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/profiles/")
	path = strings.TrimSuffix(path, "/test")
	profileID := profiles.ProfileID(path)

	if profileID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing profile ID"})
		return
	}

	profile, ok := pm.Get(profileID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
		return
	}

	// Test the profile
	result := map[string]any{
		"id":      string(profileID),
		"valid":   profile.IsValid(),
		"expired": profile.IsExpired(),
	}

	if profile.OAuth != nil {
		result["email"] = profile.OAuth.Email
		result["expires_at"] = profile.OAuth.ExpiresAt
	}
	if profile.LastError != "" {
		result["error"] = profile.LastError
	}

	pm.MarkUsed(profileID)

	writeJSON(w, http.StatusOK, result)
}

// handleAPIProviders lists available auth providers
// GET /api/auth/providers
func (s *Server) handleAPIProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	allProviders := profiles.ListProviders()
	result := make([]map[string]any, 0, len(allProviders))
	for _, p := range allProviders {
		modes := make([]string, len(p.Modes))
		for i, m := range p.Modes {
			modes[i] = string(m)
		}

		providerData := map[string]any{
			"name":        p.Name,
			"label":       p.Label,
			"description": p.Description,
			"modes":       modes,
			"website":     p.Website,
		}
		if p.EnvKey != "" {
			providerData["env_key"] = p.EnvKey
		}
		if p.ParentProvider != "" {
			providerData["parent_provider"] = p.ParentProvider
		}

		result = append(result, providerData)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"providers": result,
		"count":     len(result),
	})
}

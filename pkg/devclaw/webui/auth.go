package webui

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// handleAuthLogin validates the password and returns the auth token.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// If no auth configured, login is not needed.
	if s.cfg.AuthToken == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"token":   "",
			"message": "authentication not required",
		})
		return
	}

	// Constant-time comparison to prevent timing attacks.
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.cfg.AuthToken)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "senha incorreta"})
		return
	}

	// Set HttpOnly cookie for browser sessions.
	http.SetCookie(w, &http.Cookie{
		Name:     "devclaw_token",
		Value:    s.cfg.AuthToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 3600, // 7 days
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"token": s.cfg.AuthToken,
	})
}

// handleAuthStatus reports whether auth is required and whether the current
// request is already authenticated (via header, cookie, or query param).
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	authRequired := s.cfg.AuthToken != ""
	authenticated := !authRequired // no auth = always authenticated

	if authRequired {
		token := extractToken(r)
		if token != "" {
			authenticated = subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.AuthToken)) == 1
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"auth_required":  authRequired,
		"authenticated":  authenticated,
		"setup_complete": configFileExists(),
	})
}

// handleAuthLogout clears the auth cookie.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "devclaw_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1, // delete
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// extractToken extracts the auth token from a request.
// Checks: Authorization header → query param → cookie.
func extractToken(r *http.Request) string {
	// Bearer token in Authorization header.
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Query parameter (for SSE connections).
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}

	// HttpOnly cookie.
	if cookie, err := r.Cookie("devclaw_token"); err == nil {
		return cookie.Value
	}

	return ""
}

package webui

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// deriveToken produces a hex-encoded SHA-256 hash of the raw password so that
// the plain-text secret is never stored in cookies or returned to the client.
func deriveToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// compareTokens performs timing-safe comparison by hashing both inputs with
// SHA-256 before calling ConstantTimeCompare to prevent length-based leakage.
func compareTokens(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

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
	raw, derived := s.getAuth()
	if raw == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"token":   "",
			"message": "authentication not required",
		})
		return
	}

	// Constant-time comparison to prevent timing attacks.
	if !compareTokens(body.Password, raw) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "senha incorreta"})
		return
	}

	token := derived

	// Set HttpOnly cookie for browser sessions.
	http.SetCookie(w, &http.Cookie{
		Name:     "devclaw_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 3600, // 7 days
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
	})
}

// handleAuthStatus reports whether auth is required and whether the current
// request is already authenticated (via header, cookie, or query param).
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	raw, derived := s.getAuth()
	authRequired := raw != ""
	authenticated := !authRequired // no auth = always authenticated

	if authRequired {
		token := extractToken(r)
		if token != "" {
			authenticated = compareTokens(token, derived)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"auth_required":  authRequired,
		"authenticated":  authenticated,
		"setup_complete": configFileExists(),
	})
}

// handleAuthLogout clears the auth cookie.
// Accepts POST (API clients) and GET (browser redirect for reliable cookie clearing).
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
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

	// GET requests come from direct browser navigation — redirect to login.
	if r.Method == http.MethodGet {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

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

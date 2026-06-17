package copilot

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

func newTestVault(t *testing.T) *Vault {
	t.Helper()
	v := NewVault(filepath.Join(t.TempDir(), "test.vault"))
	if err := v.Create("pw-123456"); err != nil {
		t.Fatalf("create vault: %v", err)
	}
	return v
}

// fakeOAuthProvider simulates discovery (RFC 8414), DCR (RFC 7591), and the
// token endpoint (authorization_code + refresh_token).
func fakeOAuthProvider(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var base string
	srv := httptest.NewServer(mux)
	base = srv.URL

	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"authorization_endpoint":%q,"token_endpoint":%q,"registration_endpoint":%q}`,
			base+"/authorize", base+"/token", base+"/register")
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"client_id":"client-xyz"}`)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code_verifier") == "" {
				http.Error(w, "missing pkce", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"access_token":"access-1","refresh_token":"refresh-1","token_type":"Bearer","expires_in":3600}`)
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh-1" {
				http.Error(w, "bad refresh", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"access_token":"access-2","token_type":"Bearer","expires_in":3600}`)
		default:
			http.Error(w, "unsupported grant", http.StatusBadRequest)
		}
	})
	return srv
}

func TestMCPOAuth_FullFlow(t *testing.T) {
	provider := fakeOAuthProvider(t)
	defer provider.Close()

	vault := newTestVault(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	redirect := "http://localhost:8085/oauth/mcp/callback"
	mgr := NewMCPOAuthManager(vault, redirect, logger)

	srv := ManagedMCPServerConfig{Name: "remote", Type: MCPTypeHTTP, URL: provider.URL, OAuth: true}

	// Begin: discovery + DCR + PKCE → consent URL.
	authURL, err := mgr.BeginAuthorization(context.Background(), srv)
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authURL: %v", err)
	}
	q := u.Query()
	if q.Get("client_id") != "client-xyz" {
		t.Errorf("client_id = %q, want client-xyz", q.Get("client_id"))
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Error("missing PKCE challenge")
	}
	if q.Get("redirect_uri") != redirect {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	state := q.Get("state")
	if state == "" {
		t.Fatal("missing state")
	}

	// Callback: exchange code → token stored in vault.
	server, err := mgr.HandleCallback(context.Background(), state, "auth-code-123")
	if err != nil {
		t.Fatalf("HandleCallback: %v", err)
	}
	if server != "remote" {
		t.Errorf("server = %q, want remote", server)
	}
	if !mgr.HasToken("remote") {
		t.Error("token should be stored after callback")
	}

	// Provider returns a valid Bearer header.
	hdr, err := mgr.provider("remote").AuthHeader(context.Background())
	if err != nil {
		t.Fatalf("AuthHeader: %v", err)
	}
	if hdr != "Bearer access-1" {
		t.Errorf("AuthHeader = %q, want Bearer access-1", hdr)
	}

	// Force expiry → refresh path yields the new access token.
	tok, _ := mgr.loadToken("remote")
	tok.Expiry = 1 // far in the past
	_ = mgr.storeToken("remote", tok)
	access, err := mgr.validAccessToken(context.Background(), "remote")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if access != "access-2" {
		t.Errorf("refreshed access = %q, want access-2", access)
	}
}

func TestMCPOAuth_UnknownStateRejected(t *testing.T) {
	mgr := NewMCPOAuthManager(newTestVault(t), "http://localhost/cb", nil)
	if _, err := mgr.HandleCallback(context.Background(), "nope", "code"); err == nil {
		t.Error("callback with unknown state should fail")
	}
}

func TestPKCEPair(t *testing.T) {
	v, c, err := pkcePair()
	if err != nil {
		t.Fatalf("pkcePair: %v", err)
	}
	sum := sha256.Sum256([]byte(v))
	if c != base64.RawURLEncoding.EncodeToString(sum[:]) {
		t.Error("challenge is not S256(verifier)")
	}
}

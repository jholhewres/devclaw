package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CallbackServer handles OAuth callbacks on a local HTTP server.
type CallbackServer struct {
	port       int
	state      string
	timeout    time.Duration
	resultChan chan CallbackResult
	server     *http.Server
	wg         sync.WaitGroup
	logger     *slog.Logger

	mu     sync.Mutex
	done   bool
	result *CallbackResult
}

// NewCallbackServer creates a new callback server.
func NewCallbackServer(port int, state string, timeout time.Duration, logger *slog.Logger) *CallbackServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &CallbackServer{
		port:       port,
		state:      state,
		timeout:    timeout,
		resultChan: make(chan CallbackResult, 1),
		logger:     logger.With("component", "oauth-callback"),
	}
}

// Start starts the callback server.
func (s *CallbackServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", s.handleCallback)
	mux.HandleFunc("/oauth/callback/", s.handleCallback)

	// Handle any path for flexibility
	mux.HandleFunc("/", s.handleCallback)

	addr := "127.0.0.1:" + strconv.Itoa(s.port)
	s.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start listener
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start callback server on port %d: %w", s.port, err)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Debug("callback server stopped", "error", err)
		}
	}()

	s.logger.Info("OAuth callback server started", "port", s.port, "callback_url", s.CallbackURL())
	return nil
}

// CallbackURL returns the callback URL for this server.
func (s *CallbackServer) CallbackURL() string {
	return fmt.Sprintf("http://localhost:%d/oauth/callback", s.port)
}

// handleCallback handles the OAuth callback HTTP request.
func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Check for error response
	if err := query.Get("error"); err != "" {
		s.sendResult(CallbackResult{
			Error:            err,
			ErrorDescription: query.Get("error_description"),
		})
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "<html><body><h1>Authentication Failed</h1><p>%s: %s</p></body></html>",
			err, query.Get("error_description"))
		return
	}

	// Get code and state
	code := query.Get("code")
	state := query.Get("state")

	if code == "" {
		s.sendResult(CallbackResult{
			Error: "missing_code",
			ErrorDescription: "No authorization code received",
		})
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "<html><body><h1>Error</h1><p>No authorization code received</p></body></html>")
		return
	}

	// Verify state (CSRF protection)
	if state != s.state {
		s.sendResult(CallbackResult{
			Error: "state_mismatch",
			ErrorDescription: fmt.Sprintf("State mismatch: expected %s, got %s", s.state, state),
		})
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "<html><body><h1>Error</h1><p>State mismatch - possible CSRF attack</p></body></html>")
		return
	}

	// Success
	s.sendResult(CallbackResult{
		Code:  code,
		State: state,
	})

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<html>
<head><title>Authentication Successful</title></head>
<body style="font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f5f5f5;">
<div style="text-align: center; padding: 2rem; background: white; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1);">
<h1 style="color: #22c55e; margin-bottom: 0.5rem;">✓ Authentication Successful</h1>
<p style="color: #666;">You can close this window and return to DevClaw.</p>
</div>
</body>
</html>`)
}

// sendResult sends the result to the channel (non-blocking, only first result is kept).
func (s *CallbackServer) sendResult(result CallbackResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done {
		return
	}
	s.done = true
	s.result = &result

	select {
	case s.resultChan <- result:
	default:
	}
}

// WaitForCallback waits for the OAuth callback or timeout.
func (s *CallbackServer) WaitForCallback() (*CallbackResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	select {
	case result := <-s.resultChan:
		return &result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("OAuth callback timeout after %v", s.timeout)
	}
}

// WaitForCallbackWithContext waits for the OAuth callback with context support.
func (s *CallbackServer) WaitForCallbackWithContext(ctx context.Context) (*CallbackResult, error) {
	select {
	case result := <-s.resultChan:
		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close stops the callback server.
func (s *CallbackServer) Close() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(ctx)
	s.wg.Wait()
	return err
}

// Port returns the port the server is listening on.
func (s *CallbackServer) Port() int {
	return s.port
}

// FindAvailablePort finds an available port on localhost.
func FindAvailablePort(startPort int) (int, error) {
	for port := startPort; port < startPort+100; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return port, nil
		}
		// Check if error is "address already in use"
		if !strings.Contains(err.Error(), "address already in use") &&
			!strings.Contains(err.Error(), "bind: address already in use") {
			return 0, fmt.Errorf("error checking port %d: %w", port, err)
		}
	}
	return 0, fmt.Errorf("no available port found in range %d-%d", startPort, startPort+99)
}

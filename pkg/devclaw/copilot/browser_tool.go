// Package copilot – browser_tool.go implements a browser automation tool using
// Chrome DevTools Protocol (CDP). This allows the agent to navigate web pages,
// take screenshots, extract content,
// click elements, and fill forms.
//
// Architecture:
//
//	Agent ──browser_navigate──▶ BrowserManager ──CDP──▶ Chrome/Chromium
//	Agent ──browser_screenshot──▶ BrowserManager ──CDP──▶ Screenshot → base64
//	Agent ──browser_content──▶ BrowserManager ──CDP──▶ DOM → text
//	Agent ──browser_click──▶ BrowserManager ──CDP──▶ Click element
//
// The browser is launched lazily on first use and kept alive for the session.
// A configurable timeout prevents runaway browser sessions.
package copilot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/security"
)

// BrowserConfig configures the browser tool.
type BrowserConfig struct {
	// Enabled turns the browser tool on/off (default: true if Chrome is found).
	Enabled bool `yaml:"enabled"`

	// ChromePath is the path to the Chrome/Chromium binary.
	// Auto-detected if empty.
	ChromePath string `yaml:"chrome_path"`

	// Headless runs the browser without a visible window (default: true).
	Headless bool `yaml:"headless"`

	// TimeoutSeconds is the max time for a single browser operation (default: 30).
	TimeoutSeconds int `yaml:"timeout_seconds"`

	// MaxPages is the max number of simultaneous pages/tabs (default: 3).
	MaxPages int `yaml:"max_pages"`

	// ViewportWidth is the browser viewport width (default: 1280).
	ViewportWidth int `yaml:"viewport_width"`

	// ViewportHeight is the browser viewport height (default: 720).
	ViewportHeight int `yaml:"viewport_height"`

	// DefaultProfile is the default browser profile to use.
	DefaultProfile string `yaml:"default_profile"`

	// Profiles maps profile names to their configurations.
	Profiles map[string]BrowserProfile `yaml:"profiles"`

	// SSRFPolicy configures SSRF protection for browser navigation.
	SSRFPolicy BrowserSSRFPolicy `yaml:"ssrf_policy"`

	// AttachOnly means never launch a browser; only attach if already running.
	AttachOnly bool `yaml:"attach_only"`

	// ExtraArgs are additional command-line arguments for Chrome.
	ExtraArgs []string `yaml:"extra_args"`
}

// BrowserProfile configures a browser profile.
type BrowserProfile struct {
	// Name is the profile name.
	Name string `yaml:"name"`

	// CDPUrl is the remote CDP endpoint (e.g., "http://10.0.0.42:9222").
	CDPUrl string `yaml:"cdp_url"`

	// CDPPort is the local CDP port for this profile.
	CDPPort int `yaml:"cdp_port"`

	// Color is the UI tint color for this profile.
	Color string `yaml:"color"`

	// Driver is "devclaw" (managed) or "extension" (relay).
	Driver string `yaml:"driver"`
}

// BrowserSSRFPolicy configures SSRF protection.
type BrowserSSRFPolicy struct {
	// AllowPrivateNetwork allows navigation to private network addresses (default: true).
	AllowPrivateNetwork bool `yaml:"allow_private_network"`

	// AllowedHostnames is a whitelist of allowed hostnames.
	AllowedHostnames []string `yaml:"allowed_hostnames"`
}

// DefaultBrowserConfig returns sensible defaults.
func DefaultBrowserConfig() BrowserConfig {
	return BrowserConfig{
		Enabled:        true,
		Headless:       true,
		TimeoutSeconds: 30,
		MaxPages:       3,
		ViewportWidth:  1280,
		ViewportHeight: 720,
		DefaultProfile: "default",
		Profiles:       make(map[string]BrowserProfile),
		SSRFPolicy: BrowserSSRFPolicy{
			AllowPrivateNetwork: true,
		},
	}
}

// BrowserManager manages a Chrome/Chromium process and CDP connections.
type BrowserManager struct {
	cfg       BrowserConfig
	logger    *slog.Logger
	ssrfGuard *security.SSRFGuard

	mu         sync.Mutex
	cmd        *exec.Cmd
	browserURL string           // Browser WebSocket URL
	browserConn *websocket.Conn // Connection to browser (for target management)
	pageConn   *websocket.Conn  // Connection to current page (for page commands)
	pageTarget *CDPTarget       // Current page target
	msgID      int
	started    bool

	// Role references per targetId (for element resolution)
	roleRefsMu sync.RWMutex
	roleRefs   map[string]map[string]Ref // targetId -> ref -> Ref

	// Page state tracking
	pageStateMu sync.RWMutex
	pageState   map[string]*PageState // targetId -> state
}

// CDPTarget represents a CDP target (page, worker, etc.)
type CDPTarget struct {
	TargetID         string `json:"targetId"`
	Type             string `json:"type"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	Attached         bool   `json:"attached"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// WithSSRFGuard attaches an SSRF guard to the browser manager.
// When set, Navigate() will validate URLs before loading them.
func (bm *BrowserManager) WithSSRFGuard(guard *security.SSRFGuard) *BrowserManager {
	bm.ssrfGuard = guard
	return bm
}

// NewBrowserManager creates a new browser manager.
func NewBrowserManager(cfg BrowserConfig, logger *slog.Logger) *BrowserManager {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 30
	}
	if cfg.MaxPages <= 0 {
		cfg.MaxPages = 3
	}
	if cfg.ViewportWidth <= 0 {
		cfg.ViewportWidth = 1280
	}
	if cfg.ViewportHeight <= 0 {
		cfg.ViewportHeight = 720
	}
	return &BrowserManager{
		cfg:    cfg,
		logger: logger.With("component", "browser"),
	}
}

// findChrome locates the Chrome/Chromium binary.
func (bm *BrowserManager) findChrome() string {
	if bm.cfg.ChromePath != "" {
		return bm.cfg.ChromePath
	}
	candidates := []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium-browser",
		"chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium-browser",
		"/snap/bin/chromium",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
	}
	return ""
}

// allocatePort finds a free TCP port.
func allocatePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// Start launches Chrome with CDP enabled. Called lazily on first tool use.
func (bm *BrowserManager) Start(ctx context.Context) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Check if already started and still alive
	if bm.started {
		if bm.isAlive() && bm.pageConn != nil {
			return nil
		}
		// Chrome died or connection lost, clean up and restart
		bm.logger.Warn("chrome process died or connection lost, restarting")
		bm.cleanup()
	}

	return bm.startChrome(ctx)
}

// ensurePageTarget creates or attaches to a page target and connects to it.
// MUST be called with bm.mu already held.
func (bm *BrowserManager) ensurePageTarget() error {
	// Use HTTP /json/list to get targets with WebSocket URLs (more reliable than CDP)
	port := 0
	if bm.cmd != nil && bm.cmd.Process != nil {
		// Parse port from browserURL
		parts := strings.Split(bm.browserURL, ":")
		if len(parts) >= 3 {
			fmt.Sscanf(parts[2], "%d/", &port)
		}
	}
	if port == 0 {
		// Fallback: extract from URL
		for i, c := range bm.browserURL {
			if c == ':' && i > 0 {
				var p int
				if _, err := fmt.Sscanf(bm.browserURL[i+1:], "%d", &p); err == nil && p > 0 {
					port = p
					break
				}
			}
		}
	}

	// Get targets via HTTP /json/list
	targets, err := bm.getTargetsHTTP(port)
	if err != nil {
		bm.logger.Warn("failed to get targets via HTTP, falling back to CDP", "error", err)
		targets, err = bm.getTargets()
		if err != nil {
			return fmt.Errorf("failed to get targets: %w", err)
		}
	}

	// Find an existing page target
	var pageTarget *CDPTarget
	for i := range targets {
		if targets[i].Type == "page" && targets[i].WebSocketDebuggerURL != "" {
			pageTarget = &targets[i]
			break
		}
	}

	// Create a new page if none exists
	if pageTarget == nil {
		bm.logger.Info("creating new page target")
		targetID, err := bm.createTarget("about:blank")
		if err != nil {
			return fmt.Errorf("failed to create target: %w", err)
		}
		// Get the new target info via HTTP
		time.Sleep(100 * time.Millisecond) // Wait for target to be ready
		targets, err = bm.getTargetsHTTP(port)
		if err != nil {
			return fmt.Errorf("failed to get targets after creation: %w", err)
		}
		for i := range targets {
			if targets[i].TargetID == targetID && targets[i].WebSocketDebuggerURL != "" {
				pageTarget = &targets[i]
				break
			}
		}
	}

	if pageTarget == nil {
		return fmt.Errorf("no page target available")
	}

	bm.pageTarget = pageTarget
	bm.logger.Info("using page target", "targetId", pageTarget.TargetID, "url", pageTarget.URL, "wsURL", pageTarget.WebSocketDebuggerURL)

	// Connect to the page's WebSocket
	pageConn, _, err := websocket.DefaultDialer.Dial(pageTarget.WebSocketDebuggerURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Page WebSocket: %w", err)
	}
	bm.pageConn = pageConn
	bm.logger.Info("Page WebSocket connected")

	// Enable CDP domains on the page connection
	if err := bm.enablePageDomains(); err != nil {
		bm.logger.Warn("failed to enable some CDP domains", "error", err)
	}

	return nil
}

// getTargetsHTTP gets targets via HTTP /json/list endpoint (more reliable)
func (bm *BrowserManager) getTargetsHTTP(port int) ([]CDPTarget, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/list", port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	var targets []CDPTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("failed to parse targets: %w", err)
	}
	return targets, nil
}

// getTargets returns all current targets.
func (bm *BrowserManager) getTargets() ([]CDPTarget, error) {
	result, err := bm.sendBrowserCDP("Target.getTargets", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		TargetInfos []CDPTarget `json:"targetInfos"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse targets: %w", err)
	}
	return resp.TargetInfos, nil
}

// createTarget creates a new page target.
func (bm *BrowserManager) createTarget(url string) (string, error) {
	result, err := bm.sendBrowserCDP("Target.createTarget", map[string]any{
		"url": url,
	})
	if err != nil {
		return "", err
	}

	var resp struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return "", fmt.Errorf("failed to parse createTarget response: %w", err)
	}
	return resp.TargetID, nil
}

// attachToTarget attaches to a target and returns its WebSocket URL.
func (bm *BrowserManager) attachToTarget(targetID string) (string, error) {
	result, err := bm.sendBrowserCDP("Target.attachToTarget", map[string]any{
		"targetId":    targetID,
		"flatten":     true,
	})
	if err != nil {
		return "", err
	}

	var resp struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return "", fmt.Errorf("failed to parse attachToTarget response: %w", err)
	}
	return resp.WebSocketDebuggerURL, nil
}

// sendBrowserCDP sends a CDP command to the browser connection.
func (bm *BrowserManager) sendBrowserCDP(method string, params map[string]any) (json.RawMessage, error) {
	if bm.browserConn == nil {
		return nil, fmt.Errorf("browser connection not established")
	}

	bm.msgID++
	msg := map[string]any{
		"id":     bm.msgID,
		"method": method,
	}
	if params != nil {
		msg["params"] = params
	}

	if err := bm.browserConn.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("CDP write error: %w", err)
	}

	// Read response with timeout
	deadline := time.Now().Add(time.Duration(bm.cfg.TimeoutSeconds) * time.Second)
	bm.browserConn.SetReadDeadline(deadline)

	for {
		var resp struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := bm.browserConn.ReadJSON(&resp); err != nil {
			return nil, fmt.Errorf("CDP read error: %w", err)
		}
		if resp.ID == bm.msgID {
			if resp.Error != nil {
				return nil, fmt.Errorf("CDP error: %s", resp.Error.Message)
			}
			return resp.Result, nil
		}
		// Ignore events or other responses
	}
}

// enablePageDomains enables CDP domains on the page connection.
func (bm *BrowserManager) enablePageDomains() error {
	domains := []string{
		"Page.enable",
		"DOM.enable",
		"Network.enable",
		"Runtime.enable",
	}

	for _, domain := range domains {
		_, err := bm.sendPageCDP(domain, nil)
		if err != nil {
			bm.logger.Warn("CDP domain enable failed", "domain", domain, "error", err)
		} else {
			bm.logger.Info("CDP domain enabled", "domain", domain)
		}
	}

	return nil
}

// sendPageCDP sends a CDP command to the page connection.
func (bm *BrowserManager) sendPageCDP(method string, params map[string]any) (json.RawMessage, error) {
	if bm.pageConn == nil {
		return nil, fmt.Errorf("page connection not established")
	}

	bm.msgID++
	msg := map[string]any{
		"id":     bm.msgID,
		"method": method,
	}
	if params != nil {
		msg["params"] = params
	}

	if err := bm.pageConn.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("CDP write error: %w", err)
	}

	// Read response with timeout
	deadline := time.Now().Add(time.Duration(bm.cfg.TimeoutSeconds) * time.Second)
	bm.pageConn.SetReadDeadline(deadline)

	for {
		var resp struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := bm.pageConn.ReadJSON(&resp); err != nil {
			return nil, fmt.Errorf("CDP read error: %w", err)
		}
		if resp.ID == bm.msgID {
			if resp.Error != nil {
				return nil, fmt.Errorf("CDP error: %s", resp.Error.Message)
			}
			return resp.Result, nil
		}
		// Ignore events or other responses
	}
}

// sendCDP sends a CDP command to the page connection (alias for sendPageCDP).
// This is the main method used by browser tools.
// Automatically reconnects or restarts Chrome if the connection is lost.
func (bm *BrowserManager) sendCDP(method string, params map[string]any) (json.RawMessage, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Try to send the command
	result, err := bm.sendPageCDP(method, params)
	if err == nil {
		return result, nil
	}

	// Check if it's a connection error
	if isConnectionError(err) {
		bm.logger.Warn("connection lost, attempting reconnect", "error", err)

		// Check if Chrome is still alive
		if !bm.isAlive() {
			bm.logger.Warn("Chrome process died, full restart required")
			bm.cleanup()
			// Restart Chrome completely
			if err := bm.startChrome(context.Background()); err != nil {
				return nil, fmt.Errorf("Chrome restart failed: %w", err)
			}
		} else {
			// Chrome is alive, just reconnect to page
			if bm.pageConn != nil {
				bm.pageConn.Close()
				bm.pageConn = nil
			}
			if err := bm.ensurePageTarget(); err != nil {
				// If reconnect fails, try full restart
				bm.logger.Warn("page reconnect failed, trying full restart", "error", err)
				bm.cleanup()
				if err := bm.startChrome(context.Background()); err != nil {
					return nil, fmt.Errorf("Chrome restart failed: %w", err)
				}
			}
		}

		// Retry the command
		return bm.sendPageCDP(method, params)
	}

	return nil, err
}

// startChrome starts the Chrome process without acquiring the lock.
// MUST be called with bm.mu already held.
func (bm *BrowserManager) startChrome(ctx context.Context) error {
	chromePath := bm.findChrome()
	if chromePath == "" {
		return fmt.Errorf("chrome/chromium not found; install Chrome or set browser.chrome_path in config")
	}

	port, err := allocatePort()
	if err != nil {
		return fmt.Errorf("failed to allocate CDP port: %w", err)
	}

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-popup-blocking",
		"--disable-translate",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-default-apps",
		"--disable-dev-shm-usage",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-software-rasterizer",
		"--disable-dbus",
		"--disable-crash-reporter",
		"--disable-in-process-stack-traces",
		"--disable-logging",
		"--log-level=3",
		"--silent-debugger-extension-api",
		fmt.Sprintf("--window-size=%d,%d", bm.cfg.ViewportWidth, bm.cfg.ViewportHeight),
	}
	if bm.cfg.Headless {
		args = append(args, "--headless=new")
	}
	args = append(args, "about:blank")

	bm.cmd = exec.CommandContext(ctx, chromePath, args...)
	bm.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := bm.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Chrome: %w", err)
	}

	bm.logger.Info("chrome started", "pid", bm.cmd.Process.Pid, "port", port)

	// Wait for CDP to be ready
	wsURL, err := bm.waitForCDP(port, 30*time.Second)
	if err != nil {
		bm.killProcessGroup()
		return fmt.Errorf("CDP not ready: %w", err)
	}

	bm.logger.Info("CDP ready", "wsURL", wsURL)
	bm.browserURL = wsURL

	// Connect to Browser WebSocket
	browserConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		bm.killProcessGroup()
		return fmt.Errorf("Browser WebSocket connection failed: %w", err)
	}
	bm.browserConn = browserConn
	bm.logger.Info("Browser WebSocket connected")

	// Connect to page target
	if err := bm.ensurePageTarget(); err != nil {
		bm.killProcessGroup()
		return fmt.Errorf("failed to create page target: %w", err)
	}

	bm.started = true
	return nil
}

// isConnectionError checks if an error is a connection-related error
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "closed") ||
		strings.Contains(errStr, "EOF")
}

// sendBrowserCDPLocked sends a CDP command to the browser connection without acquiring lock.
func (bm *BrowserManager) sendBrowserCDPLocked(method string, params map[string]any) (json.RawMessage, error) {
	return bm.sendBrowserCDP(method, params)
}

// waitForCDP polls the CDP /json/version endpoint until it responds.
func (bm *BrowserManager) waitForCDP(port int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)

	for time.Now().Before(deadline) {
		conn, _, err := websocket.DefaultDialer.Dial(
			fmt.Sprintf("ws://127.0.0.1:%d", port), nil)
		if err == nil {
			conn.Close()
		}

		// Try HTTP to get the WS URL.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil {
			var info struct {
				WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
			}
			if json.NewDecoder(resp.Body).Decode(&info) == nil && info.WebSocketDebuggerUrl != "" {
				resp.Body.Close()
				return info.WebSocketDebuggerUrl, nil
			}
			resp.Body.Close()
		}

		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("timeout waiting for CDP on port %d", port)
}

// Navigate opens a URL in the browser.
func (bm *BrowserManager) Navigate(ctx context.Context, url string) error {
	if bm.ssrfGuard != nil {
		if err := bm.ssrfGuard.IsAllowed(url); err != nil {
			return fmt.Errorf("browser navigation blocked: %w", err)
		}
	}
	if err := bm.Start(ctx); err != nil {
		return err
	}
	_, err := bm.sendCDP("Page.navigate", map[string]any{"url": url})
	if err != nil {
		return err
	}
	// Wait for load.
	time.Sleep(500 * time.Millisecond)
	return nil
}

// Screenshot captures the current page as a PNG and returns base64-encoded data.
func (bm *BrowserManager) Screenshot(ctx context.Context) (string, error) {
	if err := bm.Start(ctx); err != nil {
		return "", err
	}
	result, err := bm.sendCDP("Page.captureScreenshot", map[string]any{
		"format": "png",
	})
	if err != nil {
		return "", err
	}
	var screenshotResult struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &screenshotResult); err != nil {
		return "", err
	}
	return screenshotResult.Data, nil
}

// GetContent returns the text content of the current page.
func (bm *BrowserManager) GetContent(ctx context.Context) (string, error) {
	if err := bm.Start(ctx); err != nil {
		return "", err
	}

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": "document.body ? document.body.innerText : document.documentElement.innerText",
	})
	if err != nil {
		return "", err
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return "", err
	}

	// Truncate to avoid context overflow.
	text := evalResult.Result.Value
	if len(text) > 10000 {
		text = text[:10000] + "\n... (truncated)"
	}
	return text, nil
}

// ClickElement clicks an element matched by CSS selector.
func (bm *BrowserManager) ClickElement(ctx context.Context, selector string) error {
	if err := bm.Start(ctx); err != nil {
		return err
	}

	js := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return 'not_found';
			el.click();
			return 'ok';
		})()
	`, selector)

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": js,
	})
	if err != nil {
		return err
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return err
	}
	if evalResult.Result.Value == "not_found" {
		return fmt.Errorf("element not found: %s", selector)
	}
	return nil
}

// FillInput fills a text input matched by CSS selector.
func (bm *BrowserManager) FillInput(ctx context.Context, selector, value string) error {
	if err := bm.Start(ctx); err != nil {
		return err
	}

	js := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return 'not_found';
			el.value = %q;
			el.dispatchEvent(new Event('input', { bubbles: true }));
			el.dispatchEvent(new Event('change', { bubbles: true }));
			return 'ok';
		})()
	`, selector, value)

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": js,
	})
	if err != nil {
		return err
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return err
	}
	if evalResult.Result.Value == "not_found" {
		return fmt.Errorf("input not found: %s", selector)
	}
	return nil
}

// Stop kills the Chrome process and closes connections.
func (bm *BrowserManager) Stop() {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.cleanup()
	bm.logger.Info("chrome stopped")
}

// cleanup closes connections and kills Chrome process.
func (bm *BrowserManager) cleanup() {
	if bm.pageConn != nil {
		bm.pageConn.Close()
		bm.pageConn = nil
	}
	if bm.browserConn != nil {
		bm.browserConn.Close()
		bm.browserConn = nil
	}
	bm.pageTarget = nil
	bm.killProcessGroup()
	bm.started = false
}

// killProcessGroup kills Chrome and all its child processes.
func (bm *BrowserManager) killProcessGroup() {
	if bm.cmd == nil || bm.cmd.Process == nil {
		return
	}
	// Kill the entire process group (Chrome + children)
	pgid, err := syscall.Getpgid(bm.cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		// Fallback to killing just the process
		bm.cmd.Process.Kill()
	}
	bm.cmd.Wait()
	bm.cmd = nil
}

// isAlive checks if Chrome process is still running.
func (bm *BrowserManager) isAlive() bool {
	if bm.cmd == nil || bm.cmd.Process == nil {
		return false
	}
	// Send signal 0 to check if process exists
	err := bm.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

// ─── Tool Registration ───

// RegisterBrowserTools registers browser automation tools in the executor.
func RegisterBrowserTools(executor *ToolExecutor, browserMgr *BrowserManager, logger *slog.Logger) {
	if browserMgr == nil {
		return
	}

	// browser_navigate
	executor.Register(
		MakeToolDefinition("browser_navigate",
			"Navigate the browser to a URL. Opens the page and waits for it to load.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "The URL to navigate to.",
					},
				},
				"required": []string{"url"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			url, _ := args["url"].(string)
			if url == "" {
				return nil, fmt.Errorf("url is required")
			}
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				url = "https://" + url
			}
			if err := browserMgr.Navigate(ctx, url); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Navigated to %s", url), nil
		},
	)

	// browser_screenshot
	executor.Register(
		MakeToolDefinition("browser_screenshot",
			"Take a screenshot of the current browser page. Returns the base64-encoded PNG image data that can be used with describe_image for vision analysis.",
			map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			data, err := browserMgr.Screenshot(ctx)
			if err != nil {
				return nil, err
			}
			// Return the actual base64 data so it can be used with describe_image
			return data, nil
		},
	)

	// browser_content
	executor.Register(
		MakeToolDefinition("browser_content",
			"Get the text content of the current browser page. Useful for reading web pages "+
				"without rendering. Returns the visible text, truncated if too long.",
			map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			return browserMgr.GetContent(ctx)
		},
	)

	// browser_click
	executor.Register(
		MakeToolDefinition("browser_click",
			"Click an element on the current page by CSS selector.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector for the element to click (e.g. 'button.submit', '#login-btn').",
					},
				},
				"required": []string{"selector"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			selector, _ := args["selector"].(string)
			if selector == "" {
				return nil, fmt.Errorf("selector is required")
			}
			if err := browserMgr.ClickElement(ctx, selector); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Clicked element: %s", selector), nil
		},
	)

	// browser_fill
	executor.Register(
		MakeToolDefinition("browser_fill",
			"Fill a text input on the current page by CSS selector.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector for the input element.",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "The value to enter into the input.",
					},
				},
				"required": []string{"selector", "value"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			selector, _ := args["selector"].(string)
			value, _ := args["value"].(string)
			if selector == "" {
				return nil, fmt.Errorf("selector is required")
			}
			if err := browserMgr.FillInput(ctx, selector, value); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Filled input %s with value", selector), nil
		},
	)

	// browser_snapshot
	executor.Register(
		MakeToolDefinition("browser_snapshot",
			"Capture an accessibility snapshot of the current page. Returns structured tree with element refs (e1, e2, ...) for subsequent actions. Use this to understand page structure before interacting.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"interactive": map[string]any{
						"type":        "boolean",
						"description": "Only include interactive elements (buttons, links, inputs). Default: true.",
					},
					"compact": map[string]any{
						"type":        "boolean",
						"description": "Remove structural noise. Default: true.",
					},
				},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			interactive := true
			compact := true
			if v, ok := args["interactive"].(bool); ok {
				interactive = v
			}
			if v, ok := args["compact"].(bool); ok {
				compact = v
			}
			return browserMgr.Snapshot(ctx, SnapshotOptions{
				InteractiveOnly: interactive,
				Compact:         compact,
			})
		},
	)

	// browser_tabs
	executor.Register(
		MakeToolDefinition("browser_tabs",
			"List all open browser tabs. Returns tab IDs, URLs, and titles.",
			map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			return browserMgr.ListTabs(ctx)
		},
	)

	// browser_open_tab
	executor.Register(
		MakeToolDefinition("browser_open_tab",
			"Open a new browser tab and optionally navigate to a URL.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "The URL to navigate to (optional, defaults to about:blank).",
					},
				},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			url, _ := args["url"].(string)
			if url == "" {
				url = "about:blank"
			}
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && url != "about:blank" {
				url = "https://" + url
			}
			return browserMgr.OpenTab(ctx, url)
		},
	)

	// browser_focus_tab
	executor.Register(
		MakeToolDefinition("browser_focus_tab",
			"Focus a browser tab by its target ID.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target_id": map[string]any{
						"type":        "string",
						"description": "The target ID of the tab to focus.",
					},
				},
				"required": []string{"target_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			targetID, _ := args["target_id"].(string)
			if targetID == "" {
				return nil, fmt.Errorf("target_id is required")
			}
			if err := browserMgr.FocusTab(ctx, targetID); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Focused tab: %s", targetID), nil
		},
	)

	// browser_close_tab
	executor.Register(
		MakeToolDefinition("browser_close_tab",
			"Close a browser tab by its target ID.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target_id": map[string]any{
						"type":        "string",
						"description": "The target ID of the tab to close.",
					},
				},
				"required": []string{"target_id"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			targetID, _ := args["target_id"].(string)
			if targetID == "" {
				return nil, fmt.Errorf("target_id is required")
			}
			if err := browserMgr.CloseTab(ctx, targetID); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Closed tab: %s", targetID), nil
		},
	)

	// browser_act - Unified browser actions
	executor.Register(
		MakeToolDefinition("browser_act",
			"Perform a browser action. Use after browser_snapshot to get element refs (e1, e2, ...). Kinds: click, type, press, hover, drag, select, fill, resize, wait, evaluate.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{
						"type":        "string",
						"enum":        []string{"click", "type", "press", "hover", "drag", "select", "fill", "resize", "wait", "evaluate"},
						"description": "Action kind to perform.",
					},
					"ref": map[string]any{
						"type":        "string",
						"description": "Element reference from snapshot (e1, e2, ...).",
					},
					"text": map[string]any{
						"type":        "string",
						"description": "Text to type (for type action).",
					},
					"key": map[string]any{
						"type":        "string",
						"description": "Key to press (for press action).",
					},
					"start_ref": map[string]any{
						"type":        "string",
						"description": "Start element ref (for drag action).",
					},
					"end_ref": map[string]any{
						"type":        "string",
						"description": "End element ref (for drag action).",
					},
					"values": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Values to select (for select action).",
					},
					"fields": map[string]any{
						"type":        "array",
						"description": "Form fields to fill (for fill action).",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"ref":   map[string]any{"type": "string"},
								"type":  map[string]any{"type": "string"},
								"value": map[string]any{"type": "string"},
							},
						},
					},
					"width": map[string]any{
						"type":        "integer",
						"description": "Window width (for resize action).",
					},
					"height": map[string]any{
						"type":        "integer",
						"description": "Window height (for resize action).",
					},
					"time_ms": map[string]any{
						"type":        "integer",
						"description": "Time to wait in ms (for wait action).",
					},
					"fn": map[string]any{
						"type":        "string",
						"description": "JavaScript function to evaluate (for evaluate action).",
					},
					"submit": map[string]any{
						"type":        "boolean",
						"description": "Press Enter after typing (for type action).",
					},
					"double_click": map[string]any{
						"type":        "boolean",
						"description": "Double click instead of single (for click action).",
					},
				},
				"required": []string{"kind"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			kind, _ := args["kind"].(string)
			if kind == "" {
				return nil, fmt.Errorf("kind is required")
			}

			req := ActRequest{Kind: kind}

			// Extract all optional parameters
			if v, ok := args["ref"].(string); ok {
				req.Ref = v
			}
			if v, ok := args["text"].(string); ok {
				req.Text = v
			}
			if v, ok := args["key"].(string); ok {
				req.Key = v
			}
			if v, ok := args["start_ref"].(string); ok {
				req.StartRef = v
			}
			if v, ok := args["end_ref"].(string); ok {
				req.EndRef = v
			}
			if v, ok := args["fn"].(string); ok {
				req.Function = v
			}
			if v, ok := args["submit"].(bool); ok {
				req.Submit = v
			}
			if v, ok := args["double_click"].(bool); ok {
				req.DoubleClick = v
			}
			if v, ok := args["width"].(float64); ok {
				req.Width = int(v)
			}
			if v, ok := args["height"].(float64); ok {
				req.Height = int(v)
			}
			if v, ok := args["time_ms"].(float64); ok {
				req.TimeMs = int(v)
			}
			if values, ok := args["values"].([]any); ok {
				for _, v := range values {
					if s, ok := v.(string); ok {
						req.Values = append(req.Values, s)
					}
				}
			}
			if fields, ok := args["fields"].([]any); ok {
				for _, f := range fields {
					if field, ok := f.(map[string]any); ok {
						ff := FormField{}
						if v, ok := field["ref"].(string); ok {
							ff.Ref = v
						}
						if v, ok := field["type"].(string); ok {
							ff.Type = v
						}
						if v, ok := field["value"].(string); ok {
							ff.Value = v
						}
						req.Fields = append(req.Fields, ff)
					}
				}
			}

			return browserMgr.Act(ctx, req)
		},
	)

	logger.Info("browser tools registered",
		"tools", []string{"browser_navigate", "browser_screenshot", "browser_content", "browser_click", "browser_fill", "browser_snapshot", "browser_tabs", "browser_open_tab", "browser_focus_tab", "browser_close_tab", "browser_act"},
	)
}

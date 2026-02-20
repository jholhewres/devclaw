// Package copilot – webhooks.go implements external webhook support for hooks.
// Webhooks allow sending hook events to external HTTP endpoints.
package copilot

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// WebhookConfig configures an external webhook endpoint.
type WebhookConfig struct {
	// Name identifies this webhook for logging.
	Name string `yaml:"name"`

	// URL is the webhook endpoint URL.
	URL string `yaml:"url"`

	// Events lists which events to send to this webhook.
	Events []string `yaml:"events"`

	// Secret is used to sign payloads (HMAC-SHA256).
	Secret string `yaml:"secret"`

	// Headers are additional HTTP headers to send.
	Headers map[string]string `yaml:"headers"`

	// Timeout is the request timeout in seconds (default: 10).
	Timeout int `yaml:"timeout"`

	// Enabled controls whether this webhook is active.
	Enabled bool `yaml:"enabled"`

	// RetryCount is the number of retry attempts on failure (default: 3).
	RetryCount int `yaml:"retry_count"`

	// RetryDelayMs is the delay between retries in milliseconds (default: 1000).
	RetryDelayMs int `yaml:"retry_delay_ms"`
}

// WebhooksConfig holds all webhook configurations.
type WebhooksConfig struct {
	// Enabled turns the webhook system on/off.
	Enabled bool `yaml:"enabled"`

	// Webhooks is the list of configured webhooks.
	Webhooks []WebhookConfig `yaml:"webhooks"`
}

// WebhookPayload is the JSON payload sent to webhook endpoints.
type WebhookPayload struct {
	// Event is the hook event name.
	Event string `json:"event"`

	// Timestamp is when the event occurred (ISO 8601).
	Timestamp string `json:"timestamp"`

	// SessionID is the session this event relates to (if applicable).
	SessionID string `json:"session_id,omitempty"`

	// Channel is the originating channel (if applicable).
	Channel string `json:"channel,omitempty"`

	// ToolName is the tool being called (for tool events).
	ToolName string `json:"tool_name,omitempty"`

	// Message is a human-readable description.
	Message string `json:"message,omitempty"`

	// Error is the error message (for error events).
	Error string `json:"error,omitempty"`

	// Extra holds arbitrary key-value data.
	Extra map[string]any `json:"extra,omitempty"`
}

// WebhookManager manages webhook delivery.
type WebhookManager struct {
	config    WebhooksConfig
	client    *http.Client
	hookMgr   *HookManager
	logger    *slog.Logger
	eventMap  map[string]bool // events to send
	mu        sync.RWMutex
}

// NewWebhookManager creates a new webhook manager.
func NewWebhookManager(cfg WebhooksConfig, hookMgr *HookManager, logger *slog.Logger) *WebhookManager {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}

	timeout := 10 * time.Second
	for _, wh := range cfg.Webhooks {
		if wh.Timeout > 0 {
			timeout = time.Duration(wh.Timeout) * time.Second
			break
		}
	}

	// Build event map for quick lookup.
	eventMap := make(map[string]bool)
	for _, wh := range cfg.Webhooks {
		for _, ev := range wh.Events {
			eventMap[strings.ToLower(ev)] = true
		}
	}

	wm := &WebhookManager{
		config:   cfg,
		hookMgr:  hookMgr,
		eventMap: eventMap,
		logger:   logger.With("component", "webhooks"),
		client: &http.Client{
			Timeout: timeout,
		},
	}

	// Register hook handler if enabled.
	if cfg.Enabled && hookMgr != nil {
		wm.registerHookHandler()
	}

	return wm
}

// registerHookHandler registers a hook handler that forwards events to webhooks.
func (wm *WebhookManager) registerHookHandler() {
	// Collect all events we need to listen to.
	events := make([]HookEvent, 0, len(wm.eventMap))
	for ev := range wm.eventMap {
		events = append(events, HookEvent(ev))
	}

	if len(events) == 0 {
		return
	}

	wm.hookMgr.Register(&RegisteredHook{
		Name:        "webhook-forwarder",
		Description: "Forwards hook events to configured webhooks",
		Source:      "system",
		Events:      events,
		Priority:    1000, // Run late
		Enabled:     true,
		Handler:     wm.handleHookEvent,
	})

	wm.logger.Info("webhook handler registered", "events", len(events))
}

// handleHookEvent is the hook handler that forwards events to webhooks.
func (wm *WebhookManager) handleHookEvent(ctx context.Context, payload HookPayload) HookAction {
	// Only forward, don't block.
	go wm.ForwardEvent(payload)
	return HookAction{}
}

// ForwardEvent sends an event to all matching webhooks.
func (wm *WebhookManager) ForwardEvent(payload HookPayload) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	for i := range wm.config.Webhooks {
		wh := &wm.config.Webhooks[i]
		if !wh.Enabled {
			continue
		}

		// Check if this webhook wants this event.
		matches := false
		for _, ev := range wh.Events {
			if strings.EqualFold(ev, string(payload.Event)) {
				matches = true
				break
			}
		}
		if !matches {
			continue
		}

		// Send async.
		go wm.sendWebhook(wh, payload)
	}
}

// sendWebhook delivers a payload to a webhook endpoint with retries.
func (wm *WebhookManager) sendWebhook(wh *WebhookConfig, payload HookPayload) {
	webhookPayload := WebhookPayload{
		Event:     string(payload.Event),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		SessionID: payload.SessionID,
		Channel:   payload.Channel,
		ToolName:  payload.ToolName,
		Message:   payload.Message,
		Extra:     payload.Extra,
	}

	if payload.Error != nil {
		webhookPayload.Error = payload.Error.Error()
	}

	body, err := json.Marshal(webhookPayload)
	if err != nil {
		wm.logger.Error("failed to marshal webhook payload",
			"webhook", wh.Name,
			"error", err)
		return
	}

	retryCount := wh.RetryCount
	if retryCount == 0 {
		retryCount = 3
	}
	retryDelay := wh.RetryDelayMs
	if retryDelay == 0 {
		retryDelay = 1000
	}

	var lastErr error
	for attempt := 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(retryDelay) * time.Millisecond)
			wm.logger.Debug("webhook retry",
				"webhook", wh.Name,
				"attempt", attempt)
		}

		lastErr = wm.doRequest(wh, body)
		if lastErr == nil {
			wm.logger.Debug("webhook sent",
				"webhook", wh.Name,
				"event", payload.Event)
			return
		}
	}

	wm.logger.Error("webhook failed after retries",
		"webhook", wh.Name,
		"event", payload.Event,
		"error", lastErr)
}

// doRequest performs the HTTP request to the webhook endpoint.
func (wm *WebhookManager) doRequest(wh *WebhookConfig, body []byte) error {
	req, err := http.NewRequest("POST", wh.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add custom headers.
	for k, v := range wh.Headers {
		req.Header.Set(k, v)
	}

	// Add HMAC signature if secret is configured.
	if wh.Secret != "" {
		signature := wm.signPayload(body, wh.Secret)
		req.Header.Set("X-Webhook-Signature", signature)
	}

	req.Header.Set("X-Webhook-Event", wh.Name)

	resp, err := wm.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// signPayload generates an HMAC-SHA256 signature for the payload.
func (wm *WebhookManager) signPayload(body []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// ListWebhooks returns all configured webhooks.
func (wm *WebhookManager) ListWebhooks() []WebhookConfig {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	result := make([]WebhookConfig, len(wm.config.Webhooks))
	copy(result, wm.config.Webhooks)
	return result
}

// SetWebhookEnabled enables or disables a webhook by name.
func (wm *WebhookManager) SetWebhookEnabled(name string, enabled bool) bool {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	for i := range wm.config.Webhooks {
		if wm.config.Webhooks[i].Name == name {
			wm.config.Webhooks[i].Enabled = enabled
			wm.logger.Info("webhook toggled", "name", name, "enabled", enabled)
			return true
		}
	}
	return false
}

// Reload updates the webhook configuration.
func (wm *WebhookManager) Reload(cfg WebhooksConfig) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.config = cfg

	// Rebuild event map.
	wm.eventMap = make(map[string]bool)
	for _, wh := range cfg.Webhooks {
		for _, ev := range wh.Events {
			wm.eventMap[strings.ToLower(ev)] = true
		}
	}

	wm.logger.Info("webhooks reloaded", "count", len(cfg.Webhooks))
}

// Package whatsapp implements the WhatsApp channel for DevClaw using
// whatsmeow — a native Go WhatsApp Web API library. No Node.js, no Baileys.
//
// Features:
//   - QR code login with persistent session
//   - Send/receive text, images, audio, video, documents, stickers
//   - Group message support
//   - Reply and quoting
//   - Reactions (emoji)
//   - Typing indicators and read receipts
//   - Media upload/download with encryption
//   - Automatic reconnection with backoff
//   - Connection state management and events
//
// This is a core channel (compiled into the binary, not a plugin).
package whatsapp

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "github.com/mattn/go-sqlite3" // SQLite driver for session store.
)

// Config holds WhatsApp channel configuration.
type Config struct {
	// SessionDir is the directory for session persistence (SQLite).
	// Ignored if DatabasePath is set.
	SessionDir string `yaml:"session_dir"`

	// DatabasePath is the path to the SQLite database file for session storage.
	// If set, the WhatsApp session tables (prefixed with whatsmeow_) will be
	// stored in this database alongside other devclaw data.
	// If empty, defaults to {SessionDir}/whatsapp.db.
	DatabasePath string `yaml:"database_path"`

	// Trigger is the keyword that activates the bot (e.g. "@devclaw").
	Trigger string `yaml:"trigger"`

	// RespondToGroups enables responding in group chats.
	RespondToGroups bool `yaml:"respond_to_groups"`

	// RespondToDMs enables responding in direct messages.
	RespondToDMs bool `yaml:"respond_to_dms"`

	// AutoRead marks incoming messages as read.
	AutoRead bool `yaml:"auto_read"`

	// SendTyping sends typing indicators while processing.
	SendTyping bool `yaml:"send_typing"`

	// MediaDir is the directory for downloaded media files.
	MediaDir string `yaml:"media_dir"`

	// MaxMediaSizeMB is the maximum media file size to process.
	MaxMediaSizeMB int `yaml:"max_media_size_mb"`

	// ReconnectBackoff is the initial backoff duration for reconnection.
	ReconnectBackoff time.Duration `yaml:"reconnect_backoff"`

	// MaxReconnectAttempts is the maximum number of reconnection attempts (0 = unlimited).
	MaxReconnectAttempts int `yaml:"max_reconnect_attempts"`

	// HealthMonitor configures proactive connection health monitoring.
	HealthMonitor HealthMonitorConfig `yaml:"health_monitor"`

	// Access control configuration.
	Access AccessControlConfig `yaml:"access"`

	// Group policies configuration.
	GroupPolicies GroupPoliciesConfig `yaml:"group_policies"`
}

// AccessControlConfig defines who can use the bot.
type AccessControlConfig struct {
	// DefaultPolicy: "allow" (anyone), "deny" (only authorized), or "ask" (ask once).
	DefaultPolicy string `yaml:"default_policy"`

	// Owners have full control.
	Owners []string `yaml:"owners"`

	// Admins can manage users.
	Admins []string `yaml:"admins"`

	// Allowed users can interact.
	AllowedUsers []string `yaml:"allowed_users"`

	// Blocked users cannot interact.
	BlockedUsers []string `yaml:"blocked_users"`

	// Allowed groups can interact.
	AllowedGroups []string `yaml:"allowed_groups"`

	// Blocked groups cannot interact.
	BlockedGroups []string `yaml:"blocked_groups"`

	// Pending message sent to unauthorized users.
	PendingMessage string `yaml:"pending_message"`
}

// GroupPoliciesConfig defines group-specific activation policies.
type GroupPoliciesConfig struct {
	// Default policies for unconfigured groups.
	// Multiple policies are combined with OR logic.
	DefaultPolicies []string `yaml:"default_policies"`

	// Legacy DefaultPolicy for backwards compatibility.
	// Deprecated: Use DefaultPolicies instead.
	DefaultPolicy string `yaml:"default_policy"`

	// Specific group policies.
	Groups []GroupPolicyConfig `yaml:"groups"`
}

// GroupPolicyConfig defines policy for a specific group.
type GroupPolicyConfig struct {
	// ID is the group JID.
	ID string `yaml:"id"`

	// Name is optional display name.
	Name string `yaml:"name"`

	// Policies: list of "always", "mention", "reply", "keyword", "disabled", "allowlist".
	// Multiple policies are combined with OR logic (respond if ANY policy matches).
	Policies []string `yaml:"policies"`

	// Legacy Policy field for backwards compatibility.
	// Deprecated: Use Policies instead.
	Policy string `yaml:"policy"`

	// Keywords that trigger the bot (for policy="keyword").
	Keywords []string `yaml:"keywords"`

	// Allowed users (for policy="allowlist").
	AllowedUsers []string `yaml:"allowed_users"`

	// Workspace override for this group.
	Workspace string `yaml:"workspace"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		SessionDir:           "./sessions/whatsapp",
		Trigger:              "@devclaw",
		RespondToGroups:      true,
		RespondToDMs:         true,
		AutoRead:             true,
		SendTyping:           true,
		MediaDir:             "./data/media",
		MaxMediaSizeMB:       16,
		ReconnectBackoff:     5 * time.Second,
		MaxReconnectAttempts: 10,
		HealthMonitor:        DefaultHealthMonitorConfig(),
	}
}

// QREvent represents a QR code event sent to observers.
type QREvent struct {
	// Type is "code", "success", "timeout", "error", or "refresh".
	Type string `json:"type"`
	// Code is the raw QR code string (only for Type == "code").
	Code string `json:"code,omitempty"`
	// Message is a human-readable description.
	Message string `json:"message,omitempty"`
	// ExpiresAt is when the QR code expires.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// SecondsLeft is seconds until expiration.
	SecondsLeft int `json:"seconds_left,omitempty"`
}

// WhatsApp implements the channels.Channel, channels.MediaChannel,
// channels.PresenceChannel, and channels.ReactionChannel interfaces.
type WhatsApp struct {
	cfg    Config
	client *whatsmeow.Client
	logger *slog.Logger

	// container holds the session store for device management.
	container *sqlstore.Container

	// dbPath stores the database path for cleanup operations.
	dbPath string

	// messages is the channel for incoming messages.
	messages chan *channels.IncomingMessage

	// connected tracks connection state.
	connected atomic.Bool

	// state tracks detailed connection state.
	state atomic.Value // ConnectionState

	// lastMsg tracks the last message timestamp for health.
	lastMsg atomic.Value // time.Time

	// errorCount tracks consecutive errors.
	errorCount atomic.Int64

	// reconnectAttempts tracks reconnection tries (thread-safe).
	reconnectAttempts atomic.Int32

	// qrObservers receives QR events (for web UI).
	qrObservers   []chan QREvent
	qrObserversMu sync.Mutex
	// lastQR caches the most recent QR code so late-joining observers get it.
	lastQR *QREvent
	// qrGeneratedAt tracks when QR was generated for expiration.
	qrGeneratedAt time.Time

	// connObservers receives connection state changes.
	connObservers   []ConnectionObserver
	connObserversMu sync.Mutex

	// ctx and cancel for lifecycle management.
	ctx    context.Context
	cancel context.CancelFunc

	// Access control runtime state (can be modified via API).
	accessUsers   map[string]string    // JID -> level ("owner", "admin", "user")
	accessGroups  map[string]string    // group JID -> level
	blockedUsers  map[string]struct{}  // blocked users
	blockedGroups map[string]struct{}  // blocked groups
	askedOnce     map[string]time.Time // users who received pending message

	// Group policies runtime state.
	groupPolicies map[string]*GroupPolicyConfig // group JID -> policy

	mu sync.RWMutex

	// reconnectGuard prevents multiple concurrent reconnection attempts.
	reconnectGuard atomic.Bool

	// messagesClosed tracks if the messages channel has been closed.
	// This prevents sending to a closed channel which would cause a panic.
	messagesClosed atomic.Bool
}

// New creates a new WhatsApp channel instance.
func New(cfg Config, logger *slog.Logger) *WhatsApp {
	if logger == nil {
		logger = slog.Default()
	}

	// Apply defaults.
	if cfg.ReconnectBackoff == 0 {
		cfg.ReconnectBackoff = 5 * time.Second
	}

	w := &WhatsApp{
		cfg:           cfg,
		logger:        logger.With("component", "whatsapp"),
		messages:      make(chan *channels.IncomingMessage, 256),
		accessUsers:   make(map[string]string),
		accessGroups:  make(map[string]string),
		blockedUsers:  make(map[string]struct{}),
		blockedGroups: make(map[string]struct{}),
		askedOnce:     make(map[string]time.Time),
		groupPolicies: make(map[string]*GroupPolicyConfig),
	}
	w.setState(StateDisconnected)

	// Seed access control from config.
	w.seedAccessFromConfig()

	// Seed group policies from config.
	w.seedGroupPoliciesFromConfig()

	return w
}

// ---------- State Management ----------

// getState returns the current connection state.
func (w *WhatsApp) getState() ConnectionState {
	if v := w.state.Load(); v != nil {
		return v.(ConnectionState)
	}
	return StateDisconnected
}

// setState updates the connection state.
func (w *WhatsApp) setState(state ConnectionState) {
	w.state.Store(state)
}

// GetState returns the current connection state (public API).
func (w *WhatsApp) GetState() ConnectionState {
	return w.getState()
}

// getClientJID returns the current client JID if connected.
func (w *WhatsApp) getClientJID() string {
	if w.client != nil && w.client.Store.ID != nil {
		return w.client.Store.ID.String()
	}
	return ""
}

// getClientPlatform returns the current platform.
func (w *WhatsApp) getClientPlatform() string {
	if w.client != nil && w.client.Store.Platform != "" {
		return w.client.Store.Platform
	}
	return ""
}

// ---------- QR Code Subscription ----------

// SubscribeQR registers a channel to receive QR code events.
// Returns an unsubscribe function.
func (w *WhatsApp) SubscribeQR() (chan QREvent, func()) {
	ch := make(chan QREvent, 8)
	w.qrObserversMu.Lock()
	w.qrObservers = append(w.qrObservers, ch)
	// Replay the last QR code to the new observer so it doesn't miss it.
	if w.lastQR != nil {
		// Calculate remaining time.
		evt := *w.lastQR
		if !w.qrGeneratedAt.IsZero() {
			elapsed := time.Since(w.qrGeneratedAt)
			evt.SecondsLeft = max(0, 60-int(elapsed.Seconds()))
		}
		select {
		case ch <- evt:
		default:
		}
	}
	w.qrObserversMu.Unlock()

	return ch, func() {
		w.qrObserversMu.Lock()
		defer w.qrObserversMu.Unlock()
		for i, obs := range w.qrObservers {
			if obs == ch {
				w.qrObservers = append(w.qrObservers[:i], w.qrObservers[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

// notifyQR sends a QR event to all observers.
func (w *WhatsApp) notifyQR(evt QREvent) {
	w.qrObserversMu.Lock()
	defer w.qrObserversMu.Unlock()

	// Cache the latest QR code for late-joining observers.
	if evt.Type == "code" {
		w.lastQR = &evt
		w.qrGeneratedAt = time.Now()
	} else {
		// Clear cache on success/timeout/error — QR is no longer valid.
		w.lastQR = nil
		w.qrGeneratedAt = time.Time{}
	}

	for _, ch := range w.qrObservers {
		select {
		case ch <- evt:
		default:
			// Observer too slow, skip.
		}
	}
}

// ---------- Connection Observer ----------

// AddConnectionObserver registers a connection observer.
func (w *WhatsApp) AddConnectionObserver(obs ConnectionObserver) {
	w.connObserversMu.Lock()
	defer w.connObserversMu.Unlock()
	w.connObservers = append(w.connObservers, obs)
}

// notifyConnectionChange notifies all connection observers.
func (w *WhatsApp) notifyConnectionChange(evt ConnectionEvent) {
	w.connObserversMu.Lock()
	observers := make([]ConnectionObserver, len(w.connObservers))
	copy(observers, w.connObservers)
	w.connObserversMu.Unlock()

	for _, obs := range observers {
		go func(o ConnectionObserver) {
			defer func() {
				if r := recover(); r != nil {
					w.logger.Warn("whatsapp: connection observer panic", "error", r)
				}
			}()
			o.OnConnectionChange(evt)
		}(obs)
	}
}

// ---------- Channel Interface ----------

// Name returns "whatsapp".
func (w *WhatsApp) Name() string { return "whatsapp" }

// Connect establishes the WhatsApp Web connection via whatsmeow.
// If no existing session is found, the QR login process runs in the
// background (non-blocking) so the server can start immediately.
// The QR code is streamed to web UI observers for scanning via browser.
func (w *WhatsApp) Connect(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	w.setState(StateConnecting)
	w.logger.Info("whatsapp: initializing connection...")

	// Initialize session store (SQLite).
	// Use DatabasePath if provided, otherwise fall back to SessionDir/whatsapp.db.
	dbPath := w.cfg.DatabasePath
	if dbPath == "" {
		dbPath = w.cfg.SessionDir + "/whatsapp.db"
		w.logger.Info("whatsapp: using standalone session database", "path", dbPath)
	} else {
		w.logger.Info("whatsapp: using shared devclaw database for sessions", "path", dbPath)
	}
	container, err := sqlstore.New(w.ctx, "sqlite3",
		fmt.Sprintf("file:%s?_foreign_keys=1&_journal_mode=WAL", dbPath),
		waLog.Noop)
	if err != nil {
		w.setState(StateDisconnected)
		return fmt.Errorf("creating session store: %w", err)
	}
	w.container = container
	w.dbPath = dbPath

	// Get or create device.
	device, err := w.getDevice(w.ctx, container)
	if err != nil {
		w.setState(StateDisconnected)
		return fmt.Errorf("getting device: %w", err)
	}

	// Set device name shown in WhatsApp linked devices list.
	store.SetOSInfo("DevClaw", [3]uint32{1, 0, 0})

	// Create client.
	w.client = whatsmeow.NewClient(device, waLog.Noop)
	w.client.AddEventHandler(w.handleEvent)

	// Enable whatsmeow's built-in auto-reconnect for resilient connections.
	// This handles network hiccups, server-initiated disconnects, and
	// keepalive failures automatically.
	w.client.EnableAutoReconnect = true
	w.client.InitialAutoReconnect = true

	// Connect.
	if w.client.Store.ID == nil {
		// First login — start QR process in background (non-blocking).
		w.setState(StateWaitingQR)
		w.logger.Info("whatsapp: no existing session, QR code required — scan via web UI")
		go func() {
			if err := w.loginWithQR(w.ctx); err != nil {
				w.logger.Warn("whatsapp: QR login pending", "error", err)
			}
		}()
		return nil
	}

	// Existing session — reconnect.
	err = w.client.Connect()
	if err != nil {
		w.setState(StateDisconnected)
		return fmt.Errorf("connecting: %w", err)
	}

	w.connected.Store(true)
	w.logger.Info("whatsapp: connected (existing session)",
		"jid", w.getClientJID())

	// Start health monitoring.
	w.StartHealthMonitor(w.ctx, w.cfg.HealthMonitor)

	return nil
}

// Disconnect gracefully closes the WhatsApp connection.
func (w *WhatsApp) Disconnect() error {
	previous := w.getState()
	w.setState(StateDisconnected)
	w.connected.Store(false)

	if w.cancel != nil {
		w.cancel()
	}
	if w.client != nil {
		w.client.Disconnect()
	}

	// Mark channel as closed before actually closing to prevent
	// race condition with emitMessage trying to send to closed channel.
	if w.messagesClosed.CompareAndSwap(false, true) {
		close(w.messages)
	}

	w.logger.Info("whatsapp: disconnected")

	// Notify observers.
	w.notifyConnectionChange(ConnectionEvent{
		State:     StateDisconnected,
		Previous:  previous,
		Timestamp: time.Now(),
		Reason:    "user_request",
	})

	return nil
}

// Logout logs out and clears the session.
func (w *WhatsApp) Logout() error {
	if w.client == nil {
		return nil
	}

	previous := w.getState()
	w.setState(StateLoggingOut)
	w.connected.Store(false)

	// Logout from WhatsApp (this also disconnects and deletes store).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := w.client.Logout(ctx)
	if err != nil {
		w.logger.Warn("whatsapp: logout error, forcing cleanup", "error", err)
		w.client.Disconnect()
		if w.client.Store != nil {
			if delErr := w.client.Store.Delete(ctx); delErr != nil {
				w.logger.Warn("whatsapp: failed to delete store", "error", delErr)
			}
		}
	}

	// Clean orphan tables that don't have FK CASCADE constraints.
	w.cleanOrphanTables()

	w.setState(StateDisconnected)
	w.lastQR = nil

	w.logger.Info("whatsapp: logged out, session cleared")

	// Notify observers.
	w.notifyConnectionChange(ConnectionEvent{
		State:     StateDisconnected,
		Previous:  previous,
		Timestamp: time.Now(),
		Reason:    "logout",
		Details: map[string]any{
			"session_cleared": true,
			"needs_qr":        true,
		},
	})

	return nil
}

// attemptReconnect tries to reconnect with exponential backoff.
// Uses a guard pattern to prevent multiple concurrent reconnection attempts.
// This runs in a continuous loop until reconnection succeeds or max attempts reached.
func (w *WhatsApp) attemptReconnect() {
	// Guard: prevent multiple concurrent reconnection attempts.
	if !w.reconnectGuard.CompareAndSwap(false, true) {
		w.logger.Debug("whatsapp: reconnect already in progress, skipping")
		return
	}
	defer w.reconnectGuard.Store(false)

	previous := w.getState()
	w.setState(StateReconnecting)

	for {
		if w.ctx.Err() != nil {
			w.logger.Debug("whatsapp: reconnect cancelled, context done")
			return
		}

		attempts := w.reconnectAttempts.Add(1)
		if w.cfg.MaxReconnectAttempts > 0 && attempts > int32(w.cfg.MaxReconnectAttempts) {
			w.logger.Error("whatsapp: max reconnect attempts reached",
				"attempts", attempts)
			w.setState(StateDisconnected)
			w.notifyConnectionChange(ConnectionEvent{
				State:     StateDisconnected,
				Timestamp: time.Now(),
				Reason:    "max_reconnect_attempts",
				Details: map[string]any{
					"attempts": attempts,
				},
			})
			return
		}

		backoff := min(w.cfg.ReconnectBackoff*time.Duration(attempts), 5*time.Minute)

		w.logger.Info("whatsapp: attempting reconnect",
			"attempt", attempts,
			"backoff", backoff)

		// Notify observers.
		w.notifyConnectionChange(ConnectionEvent{
			State:     StateReconnecting,
			Previous:  previous,
			Timestamp: time.Now(),
			Reason:    "connection_lost",
			Details: map[string]any{
				"attempt":     attempts,
				"backoff_sec": backoff.Seconds(),
			},
		})

		// Wait for backoff period.
		select {
		case <-time.After(backoff):
		case <-w.ctx.Done():
			w.logger.Debug("whatsapp: reconnect cancelled during backoff")
			return
		}

		if w.client == nil {
			w.logger.Warn("whatsapp: client is nil, cannot reconnect")
			return
		}

		// Disconnect first to clear any stale websocket state.
		// This fixes "websocket is already connected" error on reconnect.
		if w.client.IsConnected() {
			w.client.Disconnect()
			time.Sleep(100 * time.Millisecond) // Brief pause to allow cleanup.
		}

		err := w.client.Connect()
		if err != nil {
			w.logger.Warn("whatsapp: reconnect attempt failed, will retry",
				"attempt", attempts,
				"error", err)
			// Continue loop to retry
			continue
		}

		// Connection succeeded - the Connected event will update state.
		w.logger.Info("whatsapp: reconnect connection initiated, waiting for confirmation")
		return
	}
}

// Send sends a text message to the specified JID.
func (w *WhatsApp) Send(ctx context.Context, to string, msg *channels.OutgoingMessage) error {
	if !w.connected.Load() {
		return channels.ErrChannelDisconnected
	}

	// Suppress reasoning/thinking messages to prevent leaking internal thoughts.
	// This handles both explicit IsReasoning flag and content starting with "Reasoning:".
	if msg.IsReasoning || isReasoningContent(msg.Content) {
		w.logger.Debug("whatsapp: suppressing reasoning message", "content_preview", truncateString(msg.Content, 50))
		return nil // Successfully suppressed, not an error
	}

	jid, err := parseJID(to)
	if err != nil {
		return fmt.Errorf("invalid JID %q: %w", to, err)
	}

	waMsg := buildTextMessage(msg.Content, msg.ReplyTo)

	_, err = w.client.SendMessage(ctx, jid, waMsg)
	if err != nil {
		w.errorCount.Add(1)
		return fmt.Errorf("sending message: %w", err)
	}

	return nil
}

// isReasoningContent checks if content appears to be reasoning/thinking output
// that should be suppressed from end-user delivery.
func isReasoningContent(content string) bool {
	if content == "" {
		return false
	}
	// Check for common reasoning prefixes
	trimmed := strings.TrimSpace(content)
	return strings.HasPrefix(trimmed, "Reasoning:") ||
		strings.HasPrefix(trimmed, "Thinking:") ||
		strings.HasPrefix(trimmed, "<thinking>") ||
		strings.HasPrefix(trimmed, "<reasoning>")
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Receive returns the incoming messages channel.
func (w *WhatsApp) Receive() <-chan *channels.IncomingMessage {
	return w.messages
}

// IsConnected returns true if WhatsApp is connected.
func (w *WhatsApp) IsConnected() bool {
	return w.connected.Load()
}

// NeedsQR returns true if the WhatsApp session is not linked (needs QR scan).
func (w *WhatsApp) NeedsQR() bool {
	return w.client != nil && w.client.Store.ID == nil && !w.connected.Load()
}

// Health returns the WhatsApp channel health status.
func (w *WhatsApp) Health() channels.HealthStatus {
	h := channels.HealthStatus{
		Connected:  w.connected.Load(),
		ErrorCount: int(w.errorCount.Load()),
		Details:    make(map[string]any),
	}
	if t, ok := w.lastMsg.Load().(time.Time); ok {
		h.LastMessageAt = t
	}
	h.Details["state"] = string(w.getState())
	if w.client != nil && w.client.Store.ID != nil {
		h.Details["jid"] = w.client.Store.ID.String()
		h.Details["platform"] = w.client.Store.Platform
	}
	h.Details["reconnect_attempts"] = w.reconnectAttempts.Load()
	return h
}

// ---------- MediaChannel Interface ----------

// SendMedia sends a media message (image, audio, video, document, sticker).
func (w *WhatsApp) SendMedia(ctx context.Context, to string, media *channels.MediaMessage) error {
	if !w.connected.Load() {
		return channels.ErrChannelDisconnected
	}

	jid, err := parseJID(to)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	waMsg, err := w.buildMediaMessage(ctx, media)
	if err != nil {
		return fmt.Errorf("building media message: %w", err)
	}

	_, err = w.client.SendMessage(ctx, jid, waMsg)
	if err != nil {
		w.errorCount.Add(1)
		return fmt.Errorf("sending media: %w", err)
	}

	return nil
}

// DownloadMedia downloads media from an incoming message.
func (w *WhatsApp) DownloadMedia(ctx context.Context, msg *channels.IncomingMessage) ([]byte, string, error) {
	if msg.Media == nil {
		return nil, "", fmt.Errorf("message has no media")
	}
	return w.downloadMediaFromInfo(ctx, msg.Media)
}

// ---------- PresenceChannel Interface ----------

// SendTyping sends a typing indicator.
func (w *WhatsApp) SendTyping(ctx context.Context, to string) error {
	if !w.connected.Load() {
		return nil
	}
	jid, err := parseJID(to)
	if err != nil {
		return err
	}
	return w.client.SendChatPresence(ctx, jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)
}

// SendPresence updates the bot's online/offline status.
func (w *WhatsApp) SendPresence(ctx context.Context, available bool) error {
	if !w.connected.Load() {
		return nil
	}
	if available {
		return w.client.SendPresence(ctx, types.PresenceAvailable)
	}
	return w.client.SendPresence(ctx, types.PresenceUnavailable)
}

// MarkRead marks messages as read.
func (w *WhatsApp) MarkRead(ctx context.Context, chatID string, messageIDs []string) error {
	if !w.connected.Load() {
		return nil
	}
	jid, err := parseJID(chatID)
	if err != nil {
		return err
	}

	ids := make([]types.MessageID, len(messageIDs))
	for i, id := range messageIDs {
		ids[i] = types.MessageID(id)
	}

	return w.client.MarkRead(ctx, ids, time.Now(), jid, jid)
}

// ---------- ReactionChannel Interface ----------

// SendReaction sends an emoji reaction to a message.
func (w *WhatsApp) SendReaction(ctx context.Context, chatID, messageID, emoji string) error {
	if !w.connected.Load() {
		return channels.ErrChannelDisconnected
	}

	jid, err := parseJID(chatID)
	if err != nil {
		return err
	}

	waMsg := buildReactionMessage(messageID, jid, emoji)
	_, err = w.client.SendMessage(ctx, jid, waMsg)
	return err
}

// ---------- Internal ----------

// getDevice retrieves an existing device or creates a new one.
func (w *WhatsApp) getDevice(ctx context.Context, container *sqlstore.Container) (*store.Device, error) {
	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}
	if len(devices) > 0 {
		return devices[0], nil
	}
	return container.NewDevice(), nil
}

// cleanOrphanTables cleans up orphan data from whatsmeow tables that don't have
// FK CASCADE constraints. This is needed because DeleteDevice doesn't clean
// whatsmeow_privacy_tokens and whatsmeow_lid_map tables.
func (w *WhatsApp) cleanOrphanTables() {
	if w.dbPath == "" {
		return
	}

	// Open database connection for cleanup.
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=1", w.dbPath))
	if err != nil {
		w.logger.Warn("whatsapp: failed to open database for cleanup", "error", err)
		return
	}
	defer db.Close()

	// Clean orphan tables that don't have FK CASCADE.
	// SAFE: Table names are hardcoded constants from the whatsmeow library,
	// not user input. This pattern should NOT be used with dynamic table names.
	tables := []string{"whatsmeow_privacy_tokens", "whatsmeow_lid_map"}
	for _, table := range tables {
		result, err := db.Exec(fmt.Sprintf("DELETE FROM %s", table))
		if err != nil {
			w.logger.Warn("whatsapp: failed to clean orphan table", "table", table, "error", err)
			continue
		}
		if rows, _ := result.RowsAffected(); rows > 0 {
			w.logger.Info("whatsapp: cleaned orphan table", "table", table, "rows", rows)
		}
	}
}

// loginWithQR handles the QR code login flow.
// QR codes are delivered exclusively to web UI observers (no terminal output).
// This is designed for headless/server deployments managed via the web dashboard.
func (w *WhatsApp) loginWithQR(ctx context.Context) error {
	qrChan, err := w.client.GetQRChannel(ctx)
	if err != nil {
		return fmt.Errorf("getting QR channel: %w", err)
	}

	err = w.client.Connect()
	if err != nil {
		return fmt.Errorf("connecting for QR: %w", err)
	}

	w.setState(StateWaitingQR)
	w.logger.Info("whatsapp: waiting for QR code scan via web UI")

	qrAttempts := 0

	for {
		select {
		case <-ctx.Done():
			w.setState(StateDisconnected)
			return ctx.Err()
		case evt, ok := <-qrChan:
			if !ok {
				return fmt.Errorf("QR channel closed unexpectedly")
			}

			switch evt.Event {
			case "code":
				qrAttempts++
				w.setState(StateWaitingQR)
				w.logger.Info("whatsapp: QR code ready",
					"attempt", qrAttempts,
					"url", "/channels/whatsapp")

				w.notifyQR(QREvent{
					Type:    "code",
					Code:    evt.Code,
					Message: "Scan the QR code with WhatsApp to link your device",
				})

			case "success":
				w.connected.Store(true)
				w.reconnectAttempts.Store(0)
				w.setState(StateConnected)
				w.logger.Info("whatsapp: login successful!")
				w.notifyQR(QREvent{
					Type:    "success",
					Message: "WhatsApp linked successfully!",
				})
				return nil

			case "timeout":
				w.setState(StateDisconnected)
				w.logger.Warn("whatsapp: QR code expired")
				w.notifyQR(QREvent{
					Type:    "timeout",
					Message: "QR code expired — click refresh to try again",
				})
				return fmt.Errorf("QR code timeout")

			default:
				if evt.Error != nil {
					w.setState(StateDisconnected)
					w.logger.Error("whatsapp: QR login error", "error", evt.Error)
					w.notifyQR(QREvent{
						Type:    "error",
						Message: fmt.Sprintf("Error: %s", evt.Error.Error()),
					})
					return fmt.Errorf("QR login error: %v", evt.Error)
				}
			}
		}
	}
}

// resetClientForQR tears down the current (invalidated) session, creates a
// fresh device, and starts the QR login flow. Used after server-side logout
// where the old device store has stale foreign key references.
func (w *WhatsApp) resetClientForQR(ctx context.Context) error {
	// 1. Disconnect the transport — loginWithQR needs a disconnected client
	//    to call GetQRChannel before Connect.
	if w.client != nil {
		w.client.Disconnect()

		// 2. Delete the invalidated device store to avoid FK constraint
		//    failures when whatsmeow creates a new device identity.
		if w.client.Store != nil {
			if err := w.client.Store.Delete(ctx); err != nil {
				w.logger.Warn("whatsapp: failed to delete stale device store", "error", err)
			}
		}
	}

	// 3. Create a fresh device and client.
	if w.container == nil {
		return fmt.Errorf("session container not initialized")
	}
	newDevice := w.container.NewDevice()
	w.client = whatsmeow.NewClient(newDevice, waLog.Noop)
	w.client.AddEventHandler(w.handleEvent)
	w.client.EnableAutoReconnect = true
	w.client.InitialAutoReconnect = true

	// 4. Start QR login flow.
	return w.loginWithQR(ctx)
}

// RequestNewQR disconnects and reconnects to generate a fresh QR code.
// This is used when the web UI needs a new QR after timeout.
// A default timeout of 2 minutes is applied if the context has no deadline.
func (w *WhatsApp) RequestNewQR(ctx context.Context) error {
	if w.connected.Load() {
		return fmt.Errorf("already connected")
	}
	if w.client == nil {
		return fmt.Errorf("client not initialized")
	}

	// Disconnect current attempt if any.
	w.client.Disconnect()
	w.lastQR = nil

	// Delete the old device/session if it exists to allow fresh QR login.
	// This is necessary because GetQRChannel can only be called when there's no userID in Store.
	if w.container != nil && w.client.Store.ID != nil {
		w.logger.Info("whatsapp: deleting old session to allow fresh QR login")
		oldDevice := w.client.Store
		if err := w.container.DeleteDevice(w.ctx, oldDevice); err != nil {
			w.logger.Warn("whatsapp: failed to delete old device", "error", err)
		}
		// Clean orphan tables that don't have FK CASCADE constraints.
		// whatsmeow's DeleteDevice doesn't clean privacy_tokens and lid_map tables.
		w.cleanOrphanTables()
		// Create a new device for fresh login.
		newDevice := w.container.NewDevice()
		w.client = whatsmeow.NewClient(newDevice, waLog.Noop)
		w.client.AddEventHandler(w.handleEvent)
	}

	// Notify that we're refreshing.
	w.notifyQR(QREvent{
		Type:    "refresh",
		Message: "Generating new QR code...",
	})

	// Re-login with QR in a goroutine (non-blocking for the web handler).
	// Use resetClientForQR if the device store was invalidated (ID is nil
	// after logout) to avoid FK constraint failures.
	go func() {
		// Apply default timeout if context has no deadline.
		qrCtx := ctx
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			qrCtx, cancel = context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
		}

		var err error
		if w.client.Store == nil || w.client.Store.ID == nil {
			err = w.resetClientForQR(qrCtx)
		} else {
			err = w.loginWithQR(qrCtx)
		}
		if err != nil {
			w.logger.Error("whatsapp: QR re-login failed", "error", err)
		}
	}()

	return nil
}

// emitMessage sends a message to the incoming messages channel.
func (w *WhatsApp) emitMessage(msg *channels.IncomingMessage) {
	// Check if channel is already closed to prevent panic.
	if w.messagesClosed.Load() {
		return
	}

	select {
	case w.messages <- msg:
		w.lastMsg.Store(time.Now())
	case <-w.ctx.Done():
	default:
		w.logger.Warn("whatsapp: message channel full, dropping message",
			"from", msg.From, "type", msg.Type)
	}
}

// getClientLID returns the current client LID (Linked Identity) if available.
// WhatsApp uses LID for mentions in groups, which is different from the regular JID.
func (w *WhatsApp) getClientLID() string {
	if w.client != nil && w.client.Store != nil {
		// Try to get the LID from the store
		// The LID is stored in the device's LID field if available
		lid := w.client.Store.LID
		if !lid.IsEmpty() {
			return lid.String()
		}
	}
	return ""
}

// getAllClientIDs returns both JID and LID for comparison purposes.
func (w *WhatsApp) getAllClientIDs() []string {
	var ids []string
	if jid := w.getClientJID(); jid != "" {
		ids = append(ids, jid)
	}
	if lid := w.getClientLID(); lid != "" {
		ids = append(ids, lid)
	}
	return ids
}

// AutoReadEnabled returns true if AutoRead is configured.
func (w *WhatsApp) AutoReadEnabled() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cfg.AutoRead
}

// SetAutoRead enables or disables auto-read behavior.
func (w *WhatsApp) SetAutoRead(enabled bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cfg.AutoRead = enabled
	w.logger.Info("whatsapp: auto_read updated", "enabled", enabled)
}

// SendTypingEnabled returns true if SendTyping is configured.
func (w *WhatsApp) SendTypingEnabled() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cfg.SendTyping
}

// SetSendTyping enables or disables send-typing behavior.
func (w *WhatsApp) SetSendTyping(enabled bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cfg.SendTyping = enabled
	w.logger.Info("whatsapp: send_typing updated", "enabled", enabled)
}

// TriggerValue returns the configured trigger word.
func (w *WhatsApp) TriggerValue() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cfg.Trigger
}

// SetTrigger sets the trigger word for the bot.
func (w *WhatsApp) SetTrigger(trigger string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cfg.Trigger = trigger
	w.logger.Info("whatsapp: trigger updated", "trigger", trigger)
}

// ---------- AccessFilter Interface ----------

// CanResponse checks if the sender is allowed to interact with the bot.
func (w *WhatsApp) CanResponse(msg *channels.IncomingMessage) (bool, string) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	senderJID := normalizeJID(msg.From)
	chatJID := normalizeJID(msg.ChatID)

	// Check if user is explicitly blocked.
	if _, blocked := w.blockedUsers[senderJID]; blocked {
		return false, "user blocked"
	}

	// Check if group is explicitly blocked.
	if msg.IsGroup {
		if _, blocked := w.blockedGroups[chatJID]; blocked {
			return false, "group blocked"
		}
	}

	// Check if user has explicit access level.
	if level, exists := w.accessUsers[senderJID]; exists {
		return true, level
	}

	// Check if group has explicit access level.
	if msg.IsGroup {
		if level, exists := w.accessGroups[chatJID]; exists {
			return true, level
		}
	}

	// Apply default policy.
	switch w.cfg.Access.DefaultPolicy {
	case "allow":
		return true, "default allow"
	case "deny":
		return false, "default deny"
	default:
		// Unknown policy (including legacy "ask") - log warning and default to deny
		w.logger.Warn("unknown access policy, defaulting to deny", "policy", w.cfg.Access.DefaultPolicy)
		return false, "default deny"
	}
}

// MarkAsked marks a user as having received the pending message.
func (w *WhatsApp) MarkAsked(jid string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.askedOnce[normalizeJID(jid)] = time.Now()
}

// GrantAccess grants access to a user.
func (w *WhatsApp) GrantAccess(jid string, level string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.accessUsers[normalizeJID(jid)] = level
	w.logger.Info("whatsapp: access granted", "jid", jid, "level", level)
}

// RevokeAccess revokes access from a user.
func (w *WhatsApp) RevokeAccess(jid string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.accessUsers, normalizeJID(jid))
	w.logger.Info("whatsapp: access revoked", "jid", jid)
}

// BlockUser blocks a user.
func (w *WhatsApp) BlockUser(jid string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.blockedUsers[normalizeJID(jid)] = struct{}{}
	w.logger.Info("whatsapp: user blocked", "jid", jid)
}

// UnblockUser unblocks a user.
func (w *WhatsApp) UnblockUser(jid string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.blockedUsers, normalizeJID(jid))
	w.logger.Info("whatsapp: user unblocked", "jid", jid)
}

// ---------- GroupFilter Interface ----------

// ShouldRespond checks if the bot should respond to a group message.
// Supports multiple policies combined with OR logic.
func (w *WhatsApp) ShouldRespond(msg *channels.IncomingMessage, trigger string) bool {
	if !msg.IsGroup {
		// Non-group messages (DMs) always respond after trigger check.
		return true
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	chatJID := normalizeJID(msg.ChatID)
	botIDs := w.getAllClientIDs() // Get both JID and LID for comparison

	// Helper function to check a single policy
	checkPolicy := func(policy string, keywords []string, allowedUsers []string) bool {
		switch policy {
		case "always", "":
			return true
		case "disabled":
			return false
		case "mention":
			// Check if trigger word is in content
			if containsMention(msg.Content, w.cfg.Trigger) {
				return true
			}
			// Check if bot JID or LID is in the mentions list (WhatsApp native @mention)
			if isBotMentioned(msg.Mentions, botIDs) {
				return true
			}
			return false
		case "reply":
			return msg.ReplyTo != ""
		case "keyword":
			if len(keywords) > 0 {
				return containsKeywordInList(msg.Content, keywords)
			}
			return containsKeyword(msg.Content, w.cfg.GroupPolicies.Groups)
		case "allowlist":
			if len(allowedUsers) > 0 {
				return containsUserInList(msg.From, allowedUsers)
			}
			return false
		default:
			return true
		}
	}

	// Helper function to check multiple policies (OR logic)
	checkPolicies := func(policies []string, keywords []string, allowedUsers []string) bool {
		if len(policies) == 0 {
			return true // No policies = always respond
		}
		for _, p := range policies {
			if checkPolicy(p, keywords, allowedUsers) {
				return true
			}
		}
		return false
	}

	// Find group policy.
	policy := w.groupPolicies[chatJID]
	if policy == nil {
		// Use default policy.
		// First check if we have multiple default policies
		if len(w.cfg.GroupPolicies.DefaultPolicies) > 0 {
			return checkPolicies(w.cfg.GroupPolicies.DefaultPolicies, nil, nil)
		}
		// Fall back to single default policy for backwards compatibility
		return checkPolicy(w.cfg.GroupPolicies.DefaultPolicy, nil, nil)
	}

	// Apply specific group policy.
	// First check if we have multiple policies (new format)
	if len(policy.Policies) > 0 {
		return checkPolicies(policy.Policies, policy.Keywords, policy.AllowedUsers)
	}
	// Fall back to single policy (legacy format)
	return checkPolicy(policy.Policy, policy.Keywords, policy.AllowedUsers)
}

// SetGroupPolicy sets a policy for a group.
// Accepts either *GroupPolicyConfig or map[string]any for API compatibility.
func (w *WhatsApp) SetGroupPolicy(groupJID string, policy any) {
	var cfg *GroupPolicyConfig

	switch p := policy.(type) {
	case *GroupPolicyConfig:
		cfg = p
	case map[string]any:
		// Convert from API request
		cfg = &GroupPolicyConfig{}
		if name, ok := p["name"].(string); ok {
			cfg.Name = name
		}
		if pol, ok := p["policy"].(string); ok {
			cfg.Policy = pol
		}
		// Handle multiple policies (new format)
		if policies, ok := p["policies"].([]string); ok {
			cfg.Policies = policies
		}
		if policies, ok := p["policies"].([]any); ok {
			cfg.Policies = make([]string, len(policies))
			for i, pol := range policies {
				if s, ok := pol.(string); ok {
					cfg.Policies[i] = s
				}
			}
		}
		if keywords, ok := p["keywords"].([]string); ok {
			cfg.Keywords = keywords
		}
		if keywords, ok := p["keywords"].([]any); ok {
			cfg.Keywords = make([]string, len(keywords))
			for i, k := range keywords {
				if s, ok := k.(string); ok {
					cfg.Keywords[i] = s
				}
			}
		}
		if users, ok := p["allowed_users"].([]string); ok {
			cfg.AllowedUsers = users
		}
		if users, ok := p["allowed_users"].([]any); ok {
			cfg.AllowedUsers = make([]string, len(users))
			for i, u := range users {
				if s, ok := u.(string); ok {
					cfg.AllowedUsers[i] = s
				}
			}
		}
		if ws, ok := p["workspace"].(string); ok {
			cfg.Workspace = ws
		}
	}

	if cfg != nil {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.groupPolicies[normalizeJID(groupJID)] = cfg
		if len(cfg.Policies) > 0 {
			w.logger.Info("whatsapp: group policy set", "group", groupJID, "policies", cfg.Policies)
		} else {
			w.logger.Info("whatsapp: group policy set", "group", groupJID, "policy", cfg.Policy)
		}
	}
}

// GetGroupPolicy returns the policy for a group.
func (w *WhatsApp) GetGroupPolicy(groupJID string) *GroupPolicyConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.groupPolicies[normalizeJID(groupJID)]
}

// ListGroupPolicies returns all configured group policies.
func (w *WhatsApp) ListGroupPolicies() map[string]*GroupPolicyConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make(map[string]*GroupPolicyConfig, len(w.groupPolicies))
	for k, v := range w.groupPolicies {
		result[k] = v
	}
	return result
}

// ListGroupPoliciesForConfig returns all group policies as a slice for config persistence.
// This implements WhatsAppAccessManager interface.
func (w *WhatsApp) ListGroupPoliciesForConfig() []channels.GroupPolicyConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make([]channels.GroupPolicyConfig, 0, len(w.groupPolicies))
	for jid, cfg := range w.groupPolicies {
		result = append(result, channels.GroupPolicyConfig{
			ID:           jid,
			Name:         cfg.Name,
			Policy:       cfg.Policy,
			Policies:     cfg.Policies,
			Keywords:     cfg.Keywords,
			AllowedUsers: cfg.AllowedUsers,
			Workspace:    cfg.Workspace,
		})
	}
	return result
}

// SetGroupDefaultPolicy updates the default group policy.
func (w *WhatsApp) SetGroupDefaultPolicy(policy string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cfg.GroupPolicies.DefaultPolicy = policy
	w.logger.Info("whatsapp: group default_policy updated", "policy", policy)
}

// GetJoinedGroups returns all groups the bot is a member of.
func (w *WhatsApp) GetJoinedGroups() ([]channels.WhatsAppJoinedGroup, error) {
	w.mu.RLock()
	client := w.client
	w.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("whatsapp not connected")
	}

	groups, err := client.GetJoinedGroups(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get groups: %w", err)
	}

	result := make([]channels.WhatsAppJoinedGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, channels.WhatsAppJoinedGroup{
			JID:  g.JID.String(),
			Name: g.Name,
		})
	}

	return result, nil
}

// ---------- Access & Group Config Exporters for API ----------

// GetAccessConfig returns the current access control configuration.
func (w *WhatsApp) GetAccessConfig() any {
	w.mu.RLock()
	defer w.mu.RUnlock()

	owners := make([]string, 0, len(w.accessUsers))
	admins := make([]string, 0, len(w.accessUsers))
	allowed := make([]string, 0, len(w.accessUsers))
	blocked := make([]string, 0, len(w.blockedUsers))
	blockedGroups := make([]string, 0, len(w.blockedGroups))

	for jid, level := range w.accessUsers {
		switch level {
		case "owner":
			owners = append(owners, jid)
		case "admin":
			admins = append(admins, jid)
		case "user":
			allowed = append(allowed, jid)
		}
	}

	for jid := range w.blockedUsers {
		blocked = append(blocked, jid)
	}

	for jid := range w.blockedGroups {
		blockedGroups = append(blockedGroups, jid)
	}

	return map[string]any{
		"default_policy":  w.cfg.Access.DefaultPolicy,
		"owners":          owners,
		"admins":          admins,
		"allowed_users":   allowed,
		"blocked_users":   blocked,
		"allowed_groups":  w.cfg.Access.AllowedGroups,
		"blocked_groups":  blockedGroups,
		"pending_message": w.cfg.Access.PendingMessage,
	}
}

// SetDefaultPolicy updates the default access policy.
func (w *WhatsApp) SetDefaultPolicy(policy string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cfg.Access.DefaultPolicy = policy
	w.logger.Info("whatsapp: default_policy updated", "policy", policy)
}

// GetGroupPolicies returns the current group policies configuration.
func (w *WhatsApp) GetGroupPolicies() any {
	w.mu.RLock()
	defer w.mu.RUnlock()

	groups := make([]any, 0, len(w.groupPolicies))
	for jid, policy := range w.groupPolicies {
		groups = append(groups, map[string]any{
			"id":            jid,
			"name":          policy.Name,
			"policy":        policy.Policy,
			"policies":      policy.Policies,
			"keywords":      policy.Keywords,
			"allowed_users": policy.AllowedUsers,
			"workspace":     policy.Workspace,
		})
	}

	return map[string]any{
		"default_policy": w.cfg.GroupPolicies.DefaultPolicy,
		"groups":         groups,
	}
}

// ---------- Helpers ----------

// seedAccessFromConfig initializes access control from config.
func (w *WhatsApp) seedAccessFromConfig() {
	for _, jid := range w.cfg.Access.Owners {
		w.accessUsers[normalizeJID(jid)] = "owner"
	}
	for _, jid := range w.cfg.Access.Admins {
		w.accessUsers[normalizeJID(jid)] = "admin"
	}
	for _, jid := range w.cfg.Access.AllowedUsers {
		w.accessUsers[normalizeJID(jid)] = "user"
	}
	for _, jid := range w.cfg.Access.BlockedUsers {
		w.blockedUsers[normalizeJID(jid)] = struct{}{}
	}
	for _, jid := range w.cfg.Access.BlockedGroups {
		w.blockedGroups[normalizeJID(jid)] = struct{}{}
	}
}

// seedGroupPoliciesFromConfig initializes group policies from config.
func (w *WhatsApp) seedGroupPoliciesFromConfig() {
	for i := range w.cfg.GroupPolicies.Groups {
		policy := &w.cfg.GroupPolicies.Groups[i]
		jid := normalizeJID(policy.ID)
		w.groupPolicies[jid] = policy
	}
}

// normalizeJID normalizes a JID for consistent lookups.
// Strips companion device suffix (:XX) so that messages from linked devices
// are treated as coming from the same user.
// Example: 551199999999:95@s.whatsapp.net -> 551199999999@s.whatsapp.net
// Also handles LID format: 123456:1@lid -> 123456@lid
func normalizeJID(jid string) string {
	jid = strings.TrimSpace(jid)

	// Handle companion/linked devices: strip :XX suffix before @s.whatsapp.net
	// Format: phone:deviceID@s.whatsapp.net -> phone@s.whatsapp.net
	if strings.HasSuffix(jid, "@s.whatsapp.net") {
		base := strings.TrimSuffix(jid, "@s.whatsapp.net")
		// Remove device suffix if present (e.g., :95, :0)
		if idx := strings.LastIndex(base, ":"); idx > 0 {
			// Check if it's a device suffix (numeric after colon)
			devicePart := base[idx+1:]
			if _, err := strconv.Atoi(devicePart); err == nil {
				base = base[:idx]
			}
		}
		return base + "@s.whatsapp.net"
	}

	// Handle LID format: strip :XX suffix before @lid
	// Format: 123456:1@lid -> 123456@lid
	if strings.HasSuffix(jid, "@lid") {
		base := strings.TrimSuffix(jid, "@lid")
		// Remove device suffix if present (e.g., :1)
		if idx := strings.LastIndex(base, ":"); idx > 0 {
			devicePart := base[idx+1:]
			if _, err := strconv.Atoi(devicePart); err == nil {
				base = base[:idx]
			}
		}
		return base + "@lid"
	}

	if strings.Contains(jid, "@") {
		return jid
	}
	// Assume bare phone number.
	return jid + "@s.whatsapp.net"
}

// IsValidJID checks if a JID looks valid (basic validation).
// This is exported for use by API handlers.
func IsValidJID(jid string) bool {
	return isValidJID(jid)
}

// isValidJID checks if a JID looks valid (basic validation).
func isValidJID(jid string) bool {
	jid = strings.TrimSpace(jid)
	if jid == "" {
		return false
	}
	// Check for valid WhatsApp JID formats:
	// - phone@s.whatsapp.net (user)
	// - xxx@g.us (group)
	// - bare phone number (will be normalized)
	if strings.HasSuffix(jid, "@s.whatsapp.net") {
		local := strings.TrimSuffix(jid, "@s.whatsapp.net")
		return isValidPhone(local)
	}
	if strings.HasSuffix(jid, "@g.us") {
		return len(jid) > 10 // Basic check for group JID
	}
	if strings.Contains(jid, "@") {
		return false // Unknown JID format
	}
	// Bare phone number
	return isValidPhone(jid)
}

// isValidPhone checks if a string looks like a valid phone number.
func isValidPhone(phone string) bool {
	if len(phone) < 8 || len(phone) > 16 {
		return false
	}
	for _, r := range phone {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// containsMention checks if content contains the trigger/mention.
func containsMention(content, trigger string) bool {
	if trigger == "" {
		return true
	}
	content = strings.ToLower(content)
	trigger = strings.ToLower(trigger)
	return strings.Contains(content, trigger)
}

// isBotMentioned checks if any of the bot's IDs (JID or LID) is in the mentions list.
// This handles WhatsApp's native @mention feature where the mention appears
// as @<phone_number> in the text and in the MentionedJID list.
// WhatsApp may use LID (Linked Identity) format for mentions instead of regular JID.
func isBotMentioned(mentions []string, botIDs []string) bool {
	if len(mentions) == 0 || len(botIDs) == 0 {
		return false
	}
	// Normalize all bot IDs for comparison
	normalizedBotIDs := make([]string, len(botIDs))
	for i, id := range botIDs {
		normalizedBotIDs[i] = normalizeJID(id)
	}

	for _, mentioned := range mentions {
		normalizedMention := normalizeJID(mentioned)
		for _, botID := range normalizedBotIDs {
			if normalizedMention == botID {
				return true
			}
		}
	}
	return false
}

// containsKeyword checks if content matches any keyword from policies.
func containsKeyword(content string, policies []GroupPolicyConfig) bool {
	contentLower := strings.ToLower(content)
	for _, p := range policies {
		for _, kw := range p.Keywords {
			if strings.Contains(contentLower, strings.ToLower(kw)) {
				return true
			}
		}
	}
	return false
}

// containsKeywordInList checks if content contains any keyword from the list.
func containsKeywordInList(content string, keywords []string) bool {
	contentLower := strings.ToLower(content)
	for _, kw := range keywords {
		if strings.Contains(contentLower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// containsUserInList checks if user JID is in the allowed list.
func containsUserInList(userJID string, allowedList []string) bool {
	normalized := normalizeJID(userJID)
	for _, allowed := range allowedList {
		if normalizeJID(allowed) == normalized {
			return true
		}
	}
	return false
}

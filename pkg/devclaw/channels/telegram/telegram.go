// Package telegram implements the Telegram channel for DevClaw using the
// Telegram Bot API directly via HTTP — no external dependencies.
//
// Features:
//   - Long polling for updates (getUpdates)
//   - Send/receive text, images, audio, video, documents, voice notes
//   - Typing indicators (sendChatAction)
//   - Reactions (setMessageReaction, Bot API 7.0+)
//   - Media download via getFile
//   - HTML formatting for rich messages
//   - Group and DM support
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

// Config holds Telegram channel configuration.
type Config struct {
	// InstanceID identifies this instance ("" for default, e.g. "alerts" for named).
	// Set automatically from the config key in telegram_instances.
	InstanceID string `yaml:"instance_id,omitempty"`

	// Token is the Telegram Bot API token (from @BotFather).
	Token string `yaml:"token"`

	// AllowedChats restricts which chat IDs the bot responds to.
	// Empty means respond to all chats.
	AllowedChats []int64 `yaml:"allowed_chats"`

	// RespondToGroups enables responding in group chats.
	RespondToGroups bool `yaml:"respond_to_groups"`

	// RespondToDMs enables responding in direct messages.
	RespondToDMs bool `yaml:"respond_to_dms"`

	// SendTyping sends "typing..." indicators while processing.
	SendTyping bool `yaml:"send_typing"`

	// ParseMode sets the default parse mode for outgoing messages ("HTML" or "Markdown").
	ParseMode string `yaml:"parse_mode"`

	// ReactionNotifications controls when user reactions are surfaced as system events.
	// "off" (default): ignore reactions
	// "own": only reactions to bot messages
	// "all": all reactions in allowed chats
	ReactionNotifications string `yaml:"reaction_notifications"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		RespondToGroups:       true,
		RespondToDMs:         true,
		SendTyping:           true,
		ParseMode:            "HTML",
		ReactionNotifications: "off",
	}
}

// ButtonStyle is the visual style of an inline keyboard button.
// Telegram Bot API 9.4+ supports native styles; older clients may fall back to emoji prefixes.
const (
	ButtonStyleDefault  = ""
	ButtonStylePrimary  = "primary"  // blue
	ButtonStyleSuccess  = "success"  // green
	ButtonStyleDanger   = "danger"   // red
)

// InlineButton represents an inline keyboard button.
// Use via OutgoingMessage.Metadata["telegram_buttons"] as []InlineButton.
// Each button needs either CallbackData or URL; Style is optional.
type InlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
	Style        string `json:"style,omitempty"` // "primary", "success", "danger", or ""
}

// Telegram implements channels.Channel, channels.MediaChannel,
// channels.PresenceChannel, and channels.ReactionChannel.
type Telegram struct {
	cfg    Config
	logger *slog.Logger
	client *http.Client

	// baseURL is the Telegram Bot API base URL (https://api.telegram.org/bot<token>).
	baseURL string

	// messages is the channel for incoming messages forwarded to the assistant.
	messages chan *channels.IncomingMessage

	// connected tracks connection state.
	connected atomic.Bool

	// lastMsg tracks the last message timestamp for health.
	lastMsg atomic.Value // time.Time

	// errorCount tracks consecutive errors.
	errorCount atomic.Int64

	// offset is the last processed update ID + 1.
	offset int64

	// sentMessageIDs tracks (chatID, messageID) of messages sent by the bot,
	// used for ReactionNotifications "own" scope.
	sentMessageIDs map[string]bool
	sentMu         sync.RWMutex

	// typingConsecutive401 tracks consecutive auth errors for sendChatAction.
	typingConsecutive401 atomic.Int32
	// typingSuspended is set to true when sendChatAction is suspended due to repeated auth errors.
	typingSuspended atomic.Bool
	// typingSuspendedAt tracks when typing was suspended for time-based recovery.
	typingSuspendedAt atomic.Value // time.Time

	// lastPollAt tracks when the last successful getUpdates completed (for stall detection).
	lastPollAt atomic.Value // time.Time
	// pollRestartCh signals the pollLoop to restart (used by pollWatchdog).
	pollRestartCh chan struct{}

	// lastLatencyMs tracks the last measured API latency in milliseconds.
	lastLatencyMs atomic.Int64

	// messagesClosed prevents double-close on the messages channel.
	messagesClosed atomic.Bool

	// botUsername is the bot's @username, populated after successful Connect via getMe.
	botUsername string
	// botID is the bot's numeric user ID, populated after successful Connect via getMe.
	botID int64

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// New creates a new Telegram channel instance.
func New(cfg Config, logger *slog.Logger) *Telegram {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.ParseMode == "" {
		cfg.ParseMode = "HTML"
	}
	return &Telegram{
		cfg:             cfg,
		logger:          logger.With("component", "telegram"),
		client:          &http.Client{Timeout: 60 * time.Second},
		baseURL:         "https://api.telegram.org/bot" + cfg.Token,
		messages:        make(chan *channels.IncomingMessage, 256),
		sentMessageIDs:  make(map[string]bool),
		pollRestartCh:   make(chan struct{}, 1),
	}
}

// ---------- Channel Interface ----------

// Name returns the channel name. For the default instance this is "telegram";
// for named instances it returns "telegram:<instance_id>".
func (t *Telegram) Name() string {
	if t.cfg.InstanceID != "" {
		return "telegram:" + t.cfg.InstanceID
	}
	return "telegram"
}

// InstanceID returns the instance identifier ("" for default).
func (t *Telegram) InstanceID() string { return t.cfg.InstanceID }

// BaseType returns "telegram".
func (t *Telegram) BaseType() string { return "telegram" }

// Connect starts the long-polling loop for receiving updates.
func (t *Telegram) Connect(ctx context.Context) error {
	if t.cfg.Token == "" {
		return fmt.Errorf("telegram: bot token is required")
	}

	// Prevent double-connect goroutine leak.
	if t.connected.Load() {
		return nil
	}

	t.ctx, t.cancel = context.WithCancel(ctx)

	// Verify token by calling getMe.
	me, err := t.getMe()
	if err != nil {
		return fmt.Errorf("telegram: failed to verify token: %w", err)
	}
	t.logger.Info("telegram: connected", "bot", me.Username, "id", me.ID)
	t.mu.Lock()
	t.botUsername = me.Username
	t.botID = me.ID
	t.mu.Unlock()
	t.connected.Store(true)

	// Start polling loop.
	go t.pollLoop()

	// Start poll watchdog to detect stalled polling.
	go t.pollWatchdog()

	return nil
}

// Disconnect stops the polling loop and closes the messages channel.
func (t *Telegram) Disconnect() error {
	if t.cancel != nil {
		t.cancel()
	}
	t.connected.Store(false)
	// Close the messages channel so the manager's listener goroutine exits.
	// The atomic guard prevents double-close panics on repeated Disconnect calls.
	if t.messagesClosed.CompareAndSwap(false, true) {
		close(t.messages)
	}
	t.logger.Info("telegram: disconnected")
	return nil
}

// BotUsername returns the bot's @username (available after Connect).
func (t *Telegram) BotUsername() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.botUsername
}

// BotID returns the bot's numeric user ID (available after Connect).
func (t *Telegram) BotID() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.botID
}

// trySend attempts to send a message to the incoming messages channel.
// Returns false if the channel is closed (Disconnect was called) or full.
func (t *Telegram) trySend(msg *channels.IncomingMessage) bool {
	if t.messagesClosed.Load() {
		return false
	}
	select {
	case t.messages <- msg:
		return true
	default:
		return false
	}
}

// SetRespondToGroups updates the respond-to-groups setting on the live instance.
func (t *Telegram) SetRespondToGroups(v bool) {
	t.mu.Lock()
	t.cfg.RespondToGroups = v
	t.mu.Unlock()
}

// SetRespondToDMs updates the respond-to-DMs setting on the live instance.
func (t *Telegram) SetRespondToDMs(v bool) {
	t.mu.Lock()
	t.cfg.RespondToDMs = v
	t.mu.Unlock()
}

// SetSendTyping updates the typing indicator setting on the live instance.
func (t *Telegram) SetSendTyping(v bool) {
	t.mu.Lock()
	t.cfg.SendTyping = v
	t.mu.Unlock()
}

// SetReactionNotifications updates the reaction notifications setting on the live instance.
func (t *Telegram) SetReactionNotifications(v string) {
	t.mu.Lock()
	t.cfg.ReactionNotifications = v
	t.mu.Unlock()
}

// Send sends a text message to the specified chat.
// If message.EditMessageID is set, edits the existing message instead.
func (t *Telegram) Send(ctx context.Context, to string, message *channels.OutgoingMessage) error {
	if !t.connected.Load() {
		return channels.ErrChannelDisconnected
	}
	chatID, threadID, err := parseChatIDAndThread(to)
	if err != nil {
		return err
	}

	// Edit existing message if EditMessageID is set.
	if message.EditMessageID != "" {
		return t.editMessage(chatID, message)
	}

	payload := map[string]any{
		"chat_id":    chatID,
		"text":       message.Content,
		"parse_mode": t.cfg.ParseMode,
	}
	if threadID != 0 {
		payload["message_thread_id"] = threadID
	}
	if message.ReplyTo != "" {
		if msgID, e := strconv.ParseInt(message.ReplyTo, 10, 64); e == nil {
			payload["reply_parameters"] = map[string]any{"message_id": msgID}
		}
	}

	// Add inline keyboard if buttons are provided via Metadata.
	if replyMarkup := t.buildReplyMarkup(message); replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}

	result, err := t.sendWithThreadFallback("sendMessage", payload)
	if err != nil {
		return err
	}

	// Record sent message ID for reply-to-bot detection and reaction notifications.
	if result != nil {
		t.recordSentMessage(chatID, result)
	}
	return nil
}

// editMessage edits an existing message via editMessageText.
func (t *Telegram) editMessage(chatID int64, message *channels.OutgoingMessage) error {
	msgID, err := strconv.ParseInt(message.EditMessageID, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram: invalid edit message ID %q: %w", message.EditMessageID, err)
	}

	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": msgID,
		"text":       message.Content,
		"parse_mode": t.cfg.ParseMode,
	}

	// Preserve inline keyboard if present.
	if replyMarkup := t.buildReplyMarkup(message); replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}

	_, err = t.apiCall("editMessageText", payload)
	if err != nil {
		// Treat "message is not modified" as success — content already matches.
		if tgErr := asTelegramAPIError(err); tgErr != nil && tgErr.isMessageNotModified() {
			return nil
		}
		return err
	}
	return nil
}

// Receive returns the incoming messages channel.
func (t *Telegram) Receive() <-chan *channels.IncomingMessage {
	return t.messages
}

// IsConnected returns true if the bot is connected.
func (t *Telegram) IsConnected() bool { return t.connected.Load() }

// Health returns the channel health status.
func (t *Telegram) Health() channels.HealthStatus {
	var lastAt time.Time
	if v := t.lastMsg.Load(); v != nil {
		lastAt = v.(time.Time)
	}
	return channels.HealthStatus{
		Connected:     t.connected.Load(),
		LastMessageAt: lastAt,
		ErrorCount:    int(t.errorCount.Load()),
		LatencyMs:     t.lastLatencyMs.Load(),
	}
}

// ---------- MediaChannel Interface ----------

// SendMedia sends a media message to the specified chat.
func (t *Telegram) SendMedia(ctx context.Context, to string, media *channels.MediaMessage) error {
	if !t.connected.Load() {
		return channels.ErrChannelDisconnected
	}
	chatID, threadID, err := parseChatIDAndThread(to)
	if err != nil {
		return err
	}

	var method string
	var fieldName string
	switch media.Type {
	case channels.MessageImage:
		method = "sendPhoto"
		fieldName = "photo"
	case channels.MessageAudio:
		method = "sendAudio"
		fieldName = "audio"
	case channels.MessageVideo:
		method = "sendVideo"
		fieldName = "video"
	case channels.MessageVideoNote:
		method = "sendVideoNote"
		fieldName = "video_note"
	case channels.MessageDocument:
		method = "sendDocument"
		fieldName = "document"
	default:
		method = "sendDocument"
		fieldName = "document"
	}

	// If we have a URL, send it directly.
	if media.URL != "" {
		payload := map[string]any{
			"chat_id":  chatID,
			fieldName:  media.URL,
		}
		if threadID != 0 {
			payload["message_thread_id"] = threadID
		}
		if media.Caption != "" {
			payload["caption"] = media.Caption
			payload["parse_mode"] = t.cfg.ParseMode
		}
		_, err = t.sendWithThreadFallback(method, payload)
		return err
	}

	// Otherwise, upload the file.
	return t.uploadFile(method, chatID, threadID, fieldName, media)
}

// DownloadMedia downloads media from an incoming message.
func (t *Telegram) DownloadMedia(ctx context.Context, msg *channels.IncomingMessage) ([]byte, string, error) {
	if msg.Media == nil || msg.Media.URL == "" {
		return nil, "", channels.ErrMediaDownloadFailed
	}

	// msg.Media.URL contains the file_id; we need to call getFile first.
	fileInfo, err := t.getFile(msg.Media.URL)
	if err != nil {
		return nil, "", fmt.Errorf("telegram: getFile failed: %w", err)
	}

	// Download from https://api.telegram.org/file/bot<token>/<file_path>
	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", t.cfg.Token, fileInfo.FilePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("telegram: creating download request: %w", err)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("telegram: download failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("telegram: reading media: %w", err)
	}

	return data, msg.Media.MimeType, nil
}

// ---------- PresenceChannel Interface ----------

// SendTyping sends a "typing..." chat action.
// Implements a circuit breaker: after 10 consecutive auth errors,
// typing indicators are silently suspended to prevent hammering
// the API with an invalid token and triggering bot exclusion.
func (t *Telegram) SendTyping(ctx context.Context, to string) error {
	if !t.connected.Load() {
		return nil
	}

	// Circuit breaker: if typing is suspended, check for time-based recovery.
	if t.typingSuspended.Load() {
		if v := t.typingSuspendedAt.Load(); v != nil {
			if suspendedAt, ok := v.(time.Time); ok && time.Since(suspendedAt) > 5*time.Minute {
				// Recovery: allow a single probe request.
				t.typingSuspended.Store(false)
				t.typingConsecutive401.Store(0)
				t.logger.Info("telegram: typing circuit breaker recovered after cooldown")
			} else {
				return nil
			}
		} else {
			return nil
		}
	}

	chatID, threadID, err := parseChatIDAndThread(to)
	if err != nil {
		return nil // ignore invalid chat IDs
	}
	payload := map[string]any{
		"chat_id": chatID,
		"action":  "typing",
	}
	if threadID != 0 {
		payload["message_thread_id"] = threadID
	}
	_, err = t.sendWithThreadFallback("sendChatAction", payload)
	if err != nil {
		if tgErr := asTelegramAPIError(err); tgErr != nil && tgErr.isAuthError() {
			count := t.typingConsecutive401.Add(1)
			if count >= 10 {
				t.typingSuspended.Store(true)
				t.typingSuspendedAt.Store(time.Now())
				t.logger.Warn("telegram: typing suspended after repeated auth errors",
					"consecutive_errors", count)
			}
			return nil // Suppress auth errors for typing
		}
		return err
	}

	// Reset on success.
	t.typingConsecutive401.Store(0)
	return nil
}

// SendPresence is a no-op for Telegram.
func (t *Telegram) SendPresence(ctx context.Context, available bool) error { return nil }

// MarkRead is a no-op for Telegram (bots can't mark messages as read).
func (t *Telegram) MarkRead(ctx context.Context, chatID string, messageIDs []string) error {
	return nil
}

// ---------- ReactionChannel Interface ----------

// SendReaction sends a reaction emoji to a specific message (Bot API 7.0+).
func (t *Telegram) SendReaction(ctx context.Context, chatID, messageID, emoji string) error {
	if !t.connected.Load() {
		return nil
	}
	cid, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil
	}
	mid, err := strconv.ParseInt(messageID, 10, 64)
	if err != nil {
		return nil
	}
	_, err = t.apiCall("setMessageReaction", map[string]any{
		"chat_id":    cid,
		"message_id": mid,
		"reaction":   []map[string]string{{"type": "emoji", "emoji": emoji}},
	})
	return err
}

// ---------- Internal Helpers ----------

// parseChatIDAndThread splits a "to" string that may contain a ":topic:NNN"
// suffix (e.g. "12345678:topic:42") into the numeric chat ID and an optional
// message_thread_id for supergroup forum topics. If there is no suffix the
// returned threadID is 0.
func parseChatIDAndThread(to string) (chatID int64, threadID int64, err error) {
	// Look for the ":topic:" marker.
	if idx := strings.Index(to, ":topic:"); idx != -1 {
		threadStr := to[idx+len(":topic:"):]
		chatStr := to[:idx]
		chatID, err = strconv.ParseInt(chatStr, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("telegram: invalid chat ID %q: %w", chatStr, err)
		}
		threadID, err = strconv.ParseInt(threadStr, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("telegram: invalid thread ID %q: %w", threadStr, err)
		}
		return chatID, threadID, nil
	}
	chatID, err = strconv.ParseInt(to, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("telegram: invalid chat ID %q: %w", to, err)
	}
	return chatID, 0, nil
}

// ---------- Internal Methods ----------

// buildReplyMarkup builds an InlineKeyboardMarkup from OutgoingMessage.Metadata["telegram_buttons"].
// Each button can have text, callback_data or url, and optional style (primary/success/danger).
func (t *Telegram) buildReplyMarkup(msg *channels.OutgoingMessage) map[string]any {
	if msg == nil || msg.Metadata == nil {
		return nil
	}
	raw, ok := msg.Metadata["telegram_buttons"]
	if !ok {
		return nil
	}
	// Accept []InlineButton or []map[string]any for flexibility.
	var buttons []InlineButton
	switch v := raw.(type) {
	case []InlineButton:
		for _, b := range v {
			if b.Text != "" {
				if b.CallbackData == "" && b.URL == "" {
					b.CallbackData = "1"
				}
				buttons = append(buttons, b)
			}
		}
	case []map[string]any:
		for _, m := range v {
			var b InlineButton
			if text, ok := m["text"].(string); ok {
				b.Text = text
			}
			if cb, ok := m["callback_data"].(string); ok {
				b.CallbackData = cb
			}
			if url, ok := m["url"].(string); ok {
				b.URL = url
			}
			if style, ok := m["style"].(string); ok {
				b.Style = style
			}
			if b.Text != "" {
				if b.CallbackData == "" && b.URL == "" {
					b.CallbackData = "1" // Telegram requires callback_data or url; use minimal placeholder
				}
				buttons = append(buttons, b)
			}
		}
	case []any:
		for _, a := range v {
			if m, ok := a.(map[string]any); ok {
				var b InlineButton
				if text, ok := m["text"].(string); ok {
					b.Text = text
				}
				if cb, ok := m["callback_data"].(string); ok {
					b.CallbackData = cb
				}
				if url, ok := m["url"].(string); ok {
					b.URL = url
				}
				if style, ok := m["style"].(string); ok {
					b.Style = style
				}
				if b.Text != "" {
					if b.CallbackData == "" && b.URL == "" {
						b.CallbackData = "1"
					}
					buttons = append(buttons, b)
				}
			}
		}
	default:
		return nil
	}
	if len(buttons) == 0 {
		return nil
	}
	// Build rows: one row per button (each button on its own row).
	// For multiple buttons per row, caller could pass rows as Metadata["telegram_button_rows"].
	rows := make([][]map[string]any, 0, len(buttons))
	for _, b := range buttons {
		btn := map[string]any{"text": t.applyButtonStyle(b)}
		if b.URL != "" {
			btn["url"] = b.URL
		} else if b.CallbackData != "" {
			if len(b.CallbackData) > 64 {
				b.CallbackData = b.CallbackData[:64]
			}
			btn["callback_data"] = b.CallbackData
		}
		// Telegram Bot API 9.4+ supports native style.
		if b.Style == ButtonStylePrimary || b.Style == ButtonStyleSuccess || b.Style == ButtonStyleDanger {
			btn["style"] = b.Style
		}
		rows = append(rows, []map[string]any{btn})
	}
	return map[string]any{"inline_keyboard": rows}
}

// applyButtonStyle prefixes button text with emoji for older clients that don't support native style.
// Returns the display text. Native style is sent separately in buildReplyMarkup.
func (t *Telegram) applyButtonStyle(b InlineButton) string {
	text := b.Text
	// Emoji prefix as fallback for clients without native style support.
	switch b.Style {
	case ButtonStyleSuccess:
		text = "✅ " + text
	case ButtonStyleDanger:
		text = "❌ " + text
	case ButtonStylePrimary:
		text = "🔵 " + text
	}
	return text
}

// processMessageReaction handles message_reaction updates and surfaces them as system events.
func (t *Telegram) processMessageReaction(r *tgMessageReaction) {
	t.mu.RLock()
	reactionMode := t.cfg.ReactionNotifications
	allowedChats := t.cfg.AllowedChats
	t.mu.RUnlock()
	mode := strings.ToLower(strings.TrimSpace(reactionMode))
	if mode == "" {
		mode = "off"
	}
	if mode == "off" {
		return
	}

	// "own" = only reactions to bot messages.
	if mode == "own" && !t.isBotMessage(r.Chat.ID, r.MessageID) {
		return
	}

	// Apply AllowedChats filter.
	if len(allowedChats) > 0 {
		allowed := false
		for _, id := range allowedChats {
			if id == r.Chat.ID {
				allowed = true
				break
			}
		}
		if !allowed {
			return
		}
	}

	emoji := t.extractReactionEmoji(r.NewReaction)
	if emoji == "" {
		emoji = "👤" // fallback for custom emoji or empty
	}

	fromID := "0"
	fromName := "Unknown"
	if r.User != nil {
		fromID = strconv.FormatInt(r.User.ID, 10)
		if n := strings.TrimSpace(r.User.FirstName + " " + r.User.LastName); n != "" {
			fromName = n
		} else if r.User.Username != "" {
			fromName = r.User.Username
		} else {
			fromName = fromID
		}
	}
	if r.ActorChat != nil && r.ActorChat.Title != "" {
		fromName = r.ActorChat.Title // anonymous reaction from group
		fromID = strconv.FormatInt(r.ActorChat.ID, 10)
	}

	content := fmt.Sprintf("Telegram reaction: %s by %s on message #%d", emoji, fromName, r.MessageID)
	chatIDStr := strconv.FormatInt(r.Chat.ID, 10)
	isGroup := r.Chat.Type == "group" || r.Chat.Type == "supergroup"

	incoming := &channels.IncomingMessage{
		ID:        fmt.Sprintf("reaction-%d-%d", r.Chat.ID, r.MessageID),
		Channel:   t.Name(),
		From:      fromID,
		FromName:  fromName,
		ChatID:    chatIDStr,
		IsGroup:   isGroup,
		Type:      channels.MessageReaction,
		Content:   content,
		Timestamp: time.Unix(int64(r.Date), 0),
		ReplyTo:   strconv.Itoa(r.MessageID),
		Reaction: &channels.ReactionInfo{
			Emoji:     emoji,
			MessageID: strconv.Itoa(r.MessageID),
			From:      fromID,
			Remove:    len(r.NewReaction) == 0,
		},
	}

	t.lastMsg.Store(time.Now())
	if !t.trySend(incoming) {
		t.logger.Warn("telegram: message buffer full or closed, dropping reaction", "msg_id", r.MessageID)
	}
}

// processCallbackQuery handles inline keyboard button presses.
// It ACKs the callback via answerCallbackQuery and emits the data as an IncomingMessage.
func (t *Telegram) processCallbackQuery(cq *tgCallbackQuery) {
	// Always ACK the callback query to dismiss the loading indicator.
	_, _ = t.apiCall("answerCallbackQuery", map[string]any{
		"callback_query_id": cq.ID,
	})

	// Build sender info.
	from := strconv.FormatInt(cq.From.ID, 10)
	fromName := strings.TrimSpace(cq.From.FirstName + " " + cq.From.LastName)
	if fromName == "" {
		fromName = cq.From.Username
	}

	// Determine chat info from the associated message.
	chatIDStr := from // fallback to sender for DMs
	isGroup := false
	if cq.Message != nil {
		chatIDStr = strconv.FormatInt(cq.Message.Chat.ID, 10)
		isGroup = cq.Message.Chat.Type == "group" || cq.Message.Chat.Type == "supergroup"
	}

	incoming := &channels.IncomingMessage{
		ID:        "cb-" + cq.ID,
		Channel:   t.Name(),
		From:      from,
		FromName:  fromName,
		ChatID:    chatIDStr,
		IsGroup:   isGroup,
		Type:      channels.MessageText,
		Content:   cq.Data,
		Timestamp: time.Now(),
		Metadata: map[string]any{
			"callback_query": true,
			"callback_id":    cq.ID,
		},
	}

	// If there's an associated message, set it as ReplyTo.
	if cq.Message != nil {
		incoming.ReplyTo = strconv.FormatInt(int64(cq.Message.MessageID), 10)
	}

	t.lastMsg.Store(time.Now())
	if !t.trySend(incoming) {
		t.logger.Warn("telegram: message buffer full or closed, dropping callback query", "cb_id", cq.ID)
	}
}

// extractReactionEmoji returns the emoji string from the first emoji-type reaction.
func (t *Telegram) extractReactionEmoji(reactions []tgReaction) string {
	for _, r := range reactions {
		if r.Type == "emoji" && r.Emoji != "" {
			return r.Emoji
		}
	}
	return ""
}

// recordSentMessage parses the sendMessage result and stores the message ID.
func (t *Telegram) recordSentMessage(chatID int64, result json.RawMessage) {
	var msg struct {
		MessageID int `json:"message_id"`
	}
	if err := json.Unmarshal(result, &msg); err != nil {
		return
	}
	key := fmt.Sprintf("%d:%d", chatID, msg.MessageID)
	t.sentMu.Lock()
	if len(t.sentMessageIDs) >= 5000 {
		// Simple eviction: clear half when full.
		for k := range t.sentMessageIDs {
			delete(t.sentMessageIDs, k)
			if len(t.sentMessageIDs) < 2500 {
				break
			}
		}
	}
	t.sentMessageIDs[key] = true
	t.sentMu.Unlock()
}

// IsBotMessage returns true if the given message was sent by the bot.
// Implements channels.SentMessageTracker.
func (t *Telegram) IsBotMessage(chatID, messageID string) bool {
	cid, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return false
	}
	mid, err := strconv.Atoi(messageID)
	if err != nil {
		return false
	}
	return t.isBotMessage(cid, mid)
}

// isBotMessage returns true if the given chatID:messageID was sent by the bot.
func (t *Telegram) isBotMessage(chatID int64, messageID int) bool {
	key := fmt.Sprintf("%d:%d", chatID, messageID)
	t.sentMu.RLock()
	ok := t.sentMessageIDs[key]
	t.sentMu.RUnlock()
	return ok
}

// pollLoop runs the getUpdates long-polling loop.
// It listens for restart signals from pollWatchdog via pollRestartCh.
func (t *Telegram) pollLoop() {
	t.logger.Info("telegram: polling started")
	backoff := time.Second

	for {
		select {
		case <-t.ctx.Done():
			t.logger.Info("telegram: polling stopped")
			return
		case <-t.pollRestartCh:
			// Watchdog detected stall — reset backoff and continue polling.
			t.logger.Info("telegram: poll loop restarted by watchdog")
			backoff = time.Second
			continue
		default:
		}

		updates, err := t.getUpdates(t.offset, 100, 30)
		if err != nil {
			t.errorCount.Add(1)
			t.logger.Warn("telegram: getUpdates error", "error", err, "backoff", backoff)
			select {
			case <-t.ctx.Done():
				return
			case <-t.pollRestartCh:
				backoff = time.Second
				continue
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}

		backoff = time.Second
		t.errorCount.Store(0)
		t.lastPollAt.Store(time.Now())

		for _, u := range updates {
			if u.UpdateID >= t.offset {
				t.offset = u.UpdateID + 1
			}
			t.processUpdate(u)
		}
	}
}

// pollWatchdog monitors the polling loop and signals a restart if stalled.
// If no successful getUpdates completes within 90 seconds, the watchdog
// sends a restart signal to the poll loop via pollRestartCh.
func (t *Telegram) pollWatchdog() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			v := t.lastPollAt.Load()
			if v == nil {
				continue // No poll yet, skip.
			}
			lastPoll := v.(time.Time)
			if time.Since(lastPoll) > 90*time.Second {
				t.logger.Warn("telegram: poll stall detected, signaling restart",
					"last_poll", lastPoll, "stalled_for", time.Since(lastPoll))
				t.lastPollAt.Store(time.Now())
				// Non-blocking signal to pollLoop.
				select {
				case t.pollRestartCh <- struct{}{}:
				default:
				}
			}
		}
	}
}

// processUpdate converts a Telegram update into an IncomingMessage.
func (t *Telegram) processUpdate(u tgUpdate) {
	// Handle callback queries from inline keyboard buttons.
	if u.CallbackQuery != nil {
		t.processCallbackQuery(u.CallbackQuery)
		return
	}

	// Handle message_reaction updates (user reactions to messages).
	if u.MessageReaction != nil {
		t.processMessageReaction(u.MessageReaction)
		return
	}

	msg := u.Message
	if msg == nil {
		if u.EditedMessage != nil {
			msg = u.EditedMessage // treat edits as new messages
		} else if u.ChannelPost != nil {
			msg = u.ChannelPost // treat channel posts as messages
		} else {
			return
		}
	}

	chatIDStr := strconv.FormatInt(msg.Chat.ID, 10)
	isGroup := msg.Chat.Type == "group" || msg.Chat.Type == "supergroup"

	// Read config under lock since settings can be hot-reloaded.
	t.mu.RLock()
	allowedChats := t.cfg.AllowedChats
	respondToGroups := t.cfg.RespondToGroups
	respondToDMs := t.cfg.RespondToDMs
	t.mu.RUnlock()

	// Apply chat filter.
	if len(allowedChats) > 0 {
		allowed := false
		for _, id := range allowedChats {
			if id == msg.Chat.ID {
				allowed = true
				break
			}
		}
		if !allowed {
			return
		}
	}

	if isGroup && !respondToGroups {
		return
	}
	if !isGroup && !respondToDMs {
		return
	}

	// Build sender info.
	from := ""
	fromName := ""
	if msg.From != nil {
		from = strconv.FormatInt(msg.From.ID, 10)
		fromName = strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
		if fromName == "" {
			fromName = msg.From.Username
		}
	}

	// Build the incoming message.
	incoming := &channels.IncomingMessage{
		ID:        strconv.FormatInt(int64(msg.MessageID), 10),
		Channel:   t.Name(),
		From:      from,
		FromName:  fromName,
		ChatID:    chatIDStr,
		IsGroup:   isGroup,
		Type:      channels.MessageText,
		Content:   msg.Text,
		Timestamp: time.Unix(int64(msg.Date), 0),
	}

	// Handle caption (media messages have caption instead of text).
	if msg.Caption != "" && incoming.Content == "" {
		incoming.Content = msg.Caption
	}

	// Handle reply.
	if msg.ReplyToMessage != nil {
		incoming.ReplyTo = strconv.FormatInt(int64(msg.ReplyToMessage.MessageID), 10)
		if msg.ReplyToMessage.Text != "" {
			incoming.QuotedContent = msg.ReplyToMessage.Text
		}
	}

	// Handle media.
	if len(msg.Photo) > 0 {
		// Use the largest photo (last in array).
		photo := msg.Photo[len(msg.Photo)-1]
		incoming.Type = channels.MessageImage
		incoming.Media = &channels.MediaInfo{
			Type:     channels.MessageImage,
			URL:      photo.FileID,
			FileSize: uint64(photo.FileSize),
			Width:    uint32(photo.Width),
			Height:   uint32(photo.Height),
		}
	} else if msg.Audio != nil {
		incoming.Type = channels.MessageAudio
		incoming.Media = &channels.MediaInfo{
			Type:     channels.MessageAudio,
			URL:      msg.Audio.FileID,
			MimeType: msg.Audio.MimeType,
			FileSize: uint64(msg.Audio.FileSize),
			Duration: uint32(msg.Audio.Duration),
		}
	} else if msg.Voice != nil {
		incoming.Type = channels.MessageAudio
		incoming.Media = &channels.MediaInfo{
			Type:     channels.MessageAudio,
			URL:      msg.Voice.FileID,
			MimeType: msg.Voice.MimeType,
			FileSize: uint64(msg.Voice.FileSize),
			Duration: uint32(msg.Voice.Duration),
		}
	} else if msg.Video != nil {
		incoming.Type = channels.MessageVideo
		incoming.Media = &channels.MediaInfo{
			Type:     channels.MessageVideo,
			URL:      msg.Video.FileID,
			MimeType: msg.Video.MimeType,
			FileSize: uint64(msg.Video.FileSize),
			Duration: uint32(msg.Video.Duration),
			Width:    uint32(msg.Video.Width),
			Height:   uint32(msg.Video.Height),
		}
	} else if msg.Document != nil {
		incoming.Type = channels.MessageDocument
		incoming.Media = &channels.MediaInfo{
			Type:     channels.MessageDocument,
			URL:      msg.Document.FileID,
			MimeType: msg.Document.MimeType,
			FileSize: uint64(msg.Document.FileSize),
			Filename: msg.Document.FileName,
		}
	} else if msg.VideoNote != nil {
		incoming.Type = channels.MessageVideoNote
		incoming.Media = &channels.MediaInfo{
			Type:     channels.MessageVideoNote,
			URL:      msg.VideoNote.FileID,
			FileSize: uint64(msg.VideoNote.FileSize),
			Duration: uint32(msg.VideoNote.Duration),
		}
	} else if msg.Sticker != nil {
		incoming.Type = channels.MessageSticker
		incoming.Media = &channels.MediaInfo{
			Type: channels.MessageSticker,
			URL:  msg.Sticker.FileID,
		}
	}

	t.lastMsg.Store(time.Now())
	if !t.trySend(incoming) {
		t.logger.Warn("telegram: message buffer full or closed, dropping message", "msg_id", incoming.ID)
	}
}

// ---------- Telegram Bot API Types ----------

type tgUpdate struct {
	UpdateID        int64                `json:"update_id"`
	Message         *tgMessage           `json:"message"`
	EditedMessage   *tgMessage           `json:"edited_message"`
	ChannelPost     *tgMessage           `json:"channel_post"`
	MessageReaction *tgMessageReaction   `json:"message_reaction"`
	CallbackQuery   *tgCallbackQuery     `json:"callback_query"`
}

// tgCallbackQuery represents a callback query from an inline keyboard button.
type tgCallbackQuery struct {
	ID      string     `json:"id"`
	From    tgUser     `json:"from"`
	Message *tgMessage `json:"message"`
	Data    string     `json:"data"`
}

// tgMessageReaction is the MessageReactionUpdated object from the Bot API.
type tgMessageReaction struct {
	Chat        tgChat         `json:"chat"`
	MessageID   int            `json:"message_id"`
	User        *tgUser        `json:"user"`
	ActorChat   *tgChat        `json:"actor_chat"`
	Date        int            `json:"date"`
	OldReaction []tgReaction   `json:"old_reaction"`
	NewReaction []tgReaction   `json:"new_reaction"`
}

// tgReaction represents a ReactionType (emoji or custom_emoji).
type tgReaction struct {
	Type  string `json:"type"`  // "emoji" or "custom_emoji"
	Emoji string `json:"emoji"` // for type "emoji"
	CustomEmojiID string `json:"custom_emoji_id"` // for type "custom_emoji"
}

type tgMessage struct {
	MessageID      int          `json:"message_id"`
	From           *tgUser      `json:"from"`
	Chat           tgChat       `json:"chat"`
	Date           int          `json:"date"`
	Text           string       `json:"text"`
	Caption        string       `json:"caption"`
	ReplyToMessage *tgMessage   `json:"reply_to_message"`
	Photo          []tgPhoto    `json:"photo"`
	Audio          *tgAudio     `json:"audio"`
	Voice          *tgVoice     `json:"voice"`
	Video          *tgVideo     `json:"video"`
	VideoNote      *tgVideoNote `json:"video_note"`
	Document       *tgDocument  `json:"document"`
	Sticker        *tgSticker   `json:"sticker"`
}

type tgVideoNote struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	Length   int    `json:"length"`
	FileSize int    `json:"file_size"`
}

type tgUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	IsBot     bool   `json:"is_bot"`
}

type tgChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"` // "private", "group", "supergroup", "channel"
	Title string `json:"title"`
}

type tgPhoto struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int    `json:"file_size"`
}

type tgAudio struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	MimeType string `json:"mime_type"`
	FileSize int    `json:"file_size"`
}

type tgVoice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	MimeType string `json:"mime_type"`
	FileSize int    `json:"file_size"`
}

type tgVideo struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Duration int    `json:"duration"`
	MimeType string `json:"mime_type"`
	FileSize int    `json:"file_size"`
}

type tgDocument struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int    `json:"file_size"`
}

type tgSticker struct {
	FileID string `json:"file_id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Emoji  string `json:"emoji"`
}

type tgFile struct {
	FileID   string `json:"file_id"`
	FilePath string `json:"file_path"`
	FileSize int    `json:"file_size"`
}

type tgBotUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// sendWithThreadFallback calls apiCall and, if the error indicates a missing
// forum topic/thread, removes message_thread_id from the payload and retries.
// This handles cases where a supergroup topic has been closed or deleted.
// The original payload is not mutated; a shallow copy is used for the retry.
func (t *Telegram) sendWithThreadFallback(method string, payload map[string]any) (json.RawMessage, error) {
	result, err := t.apiCall(method, payload)
	if err != nil {
		if tgErr := asTelegramAPIError(err); tgErr != nil && tgErr.isThreadNotFound() {
			if _, hasThread := payload["message_thread_id"]; hasThread {
				t.logger.Warn("telegram: thread not found, retrying without thread ID",
					"method", method, "thread_id", payload["message_thread_id"])
				// Clone payload to avoid mutating the caller's map.
				retry := make(map[string]any, len(payload)-1)
				for k, v := range payload {
					if k != "message_thread_id" {
						retry[k] = v
					}
				}
				return t.apiCall(method, retry)
			}
		}
		return nil, err
	}
	return result, nil
}

// ---------- API Helpers ----------

// apiCall makes a POST request to the Telegram Bot API.
// On API errors it returns a *TelegramAPIError with structured fields
// for classification (retryable, auth, rate-limited, etc.).
func (t *Telegram) apiCall(method string, payload map[string]any) (json.RawMessage, error) {
	url := t.baseURL + "/" + method
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("telegram: marshal %s: %w", method, err)
	}

	req, err := http.NewRequestWithContext(t.ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("telegram: creating request for %s: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: %s request failed: %w", method, err)
	}
	defer resp.Body.Close()

	latency := time.Since(start).Milliseconds()

	var result struct {
		OK          bool            `json:"ok"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("telegram: decoding %s response: %w", method, err)
	}
	if !result.OK {
		return nil, &TelegramAPIError{
			Method:      method,
			HTTPStatus:  resp.StatusCode,
			ErrorCode:   result.ErrorCode,
			Description: result.Description,
		}
	}

	// Track latency only from successful API calls.
	t.lastLatencyMs.Store(latency)

	return result.Result, nil
}

// getMe verifies the bot token and returns bot info.
func (t *Telegram) getMe() (*tgBotUser, error) {
	data, err := t.apiCall("getMe", nil)
	if err != nil {
		return nil, err
	}
	var user tgBotUser
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("telegram: parsing getMe: %w", err)
	}
	return &user, nil
}

// getUpdates fetches new updates using long polling.
func (t *Telegram) getUpdates(offset int64, limit, timeoutSecs int) ([]tgUpdate, error) {
	payload := map[string]any{
		"offset":  offset,
		"limit":   limit,
		"timeout": timeoutSecs,
		"allowed_updates": []string{
			"message", "edited_message", "channel_post", "message_reaction", "callback_query",
		},
	}
	data, err := t.apiCall("getUpdates", payload)
	if err != nil {
		return nil, err
	}
	var updates []tgUpdate
	if err := json.Unmarshal(data, &updates); err != nil {
		return nil, fmt.Errorf("telegram: parsing updates: %w", err)
	}
	return updates, nil
}

// getFile retrieves file info for downloading.
func (t *Telegram) getFile(fileID string) (*tgFile, error) {
	data, err := t.apiCall("getFile", map[string]any{"file_id": fileID})
	if err != nil {
		return nil, err
	}
	var file tgFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("telegram: parsing getFile: %w", err)
	}
	return &file, nil
}

// uploadFile uploads a file to Telegram using multipart form data.
func (t *Telegram) uploadFile(method string, chatID int64, threadID int64, fieldName string, media *channels.MediaMessage) error {
	if len(media.Data) == 0 {
		return fmt.Errorf("telegram: media data is required for upload")
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	_ = w.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	if threadID != 0 {
		_ = w.WriteField("message_thread_id", strconv.FormatInt(threadID, 10))
	}
	if media.Caption != "" {
		_ = w.WriteField("caption", media.Caption)
		_ = w.WriteField("parse_mode", t.cfg.ParseMode)
	}

	filename := media.Filename
	if filename == "" {
		filename = "file"
	}
	part, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		return fmt.Errorf("telegram: creating form file: %w", err)
	}
	if _, err := part.Write(media.Data); err != nil {
		return fmt.Errorf("telegram: writing file data: %w", err)
	}
	w.Close()

	url := t.baseURL + "/" + method
	req, err := http.NewRequestWithContext(t.ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("telegram: creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: upload failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("telegram: decoding %s upload response: %w", method, err)
	}
	if !result.OK {
		return fmt.Errorf("telegram: %s upload: %s", method, result.Description)
	}

	// Record sent message ID for reply-to-bot detection.
	if result.Result != nil {
		t.recordSentMessage(chatID, result.Result)
	}
	return nil
}

// Compile-time interface verification.
var (
	_ channels.Channel            = (*Telegram)(nil)
	_ channels.MediaChannel       = (*Telegram)(nil)
	_ channels.PresenceChannel    = (*Telegram)(nil)
	_ channels.ReactionChannel    = (*Telegram)(nil)
	_ channels.SentMessageTracker = (*Telegram)(nil)
)

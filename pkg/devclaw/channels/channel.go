// Package channels defines the interfaces and types for DevClaw communication
// channels. Each channel (WhatsApp, Discord, Telegram) implements the Channel
// interface to receive and send messages in a unified way.
package channels

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// MessageType identifies the kind of message content.
type MessageType string

const (
	MessageText     MessageType = "text"
	MessageImage    MessageType = "image"
	MessageAudio    MessageType = "audio"
	MessageVideo    MessageType = "video"
	MessageDocument MessageType = "document"
	MessageSticker  MessageType = "sticker"
	MessageLocation MessageType = "location"
	MessageContact  MessageType = "contact"
	MessageReaction  MessageType = "reaction"
	MessageVideoNote MessageType = "video_note"
)

// Channel defines the interface that every communication channel must implement.
type Channel interface {
	// Name returns the channel identifier (e.g. "whatsapp", "discord").
	Name() string

	// Connect establishes the connection to the messaging platform.
	Connect(ctx context.Context) error

	// Disconnect gracefully closes the connection.
	Disconnect() error

	// Send sends a message to the specified recipient.
	Send(ctx context.Context, to string, message *OutgoingMessage) error

	// Receive returns a Go channel that emits incoming messages.
	Receive() <-chan *IncomingMessage

	// IsConnected returns true if the channel is connected.
	IsConnected() bool

	// Health returns the channel health status.
	Health() HealthStatus
}

// MediaChannel extends Channel with media capabilities.
// Channels that support media (images, audio, video, documents) should
// implement this interface.
type MediaChannel interface {
	Channel

	// SendMedia sends a media message (image, audio, video, document).
	SendMedia(ctx context.Context, to string, media *MediaMessage) error

	// DownloadMedia downloads media from an incoming message.
	// Returns the raw bytes and MIME type.
	DownloadMedia(ctx context.Context, msg *IncomingMessage) ([]byte, string, error)
}

// PresenceChannel extends Channel with typing/presence indicators.
type PresenceChannel interface {
	Channel

	// SendTyping sends a "typing..." indicator to the recipient.
	SendTyping(ctx context.Context, to string) error

	// SendPresence updates the bot's presence status.
	SendPresence(ctx context.Context, available bool) error

	// MarkRead marks a message as read.
	MarkRead(ctx context.Context, chatID string, messageIDs []string) error
}

// ReactionChannel extends Channel with message reaction support.
type ReactionChannel interface {
	Channel

	// SendReaction sends a reaction emoji to a specific message.
	SendReaction(ctx context.Context, chatID, messageID, emoji string) error
}

// SentMessageTracker tracks messages sent by the bot for reply detection.
// Channels that implement this can identify whether a given message was
// sent by the bot, enabling accurate reply-to-bot detection.
type SentMessageTracker interface {
	IsBotMessage(chatID, messageID string) bool
}

// InstanceAware is an optional interface for channels that support multiple
// instances of the same type (e.g., two WhatsApp accounts). Channels that
// implement this return a non-empty InstanceID for named instances, while
// the default instance returns "".
type InstanceAware interface {
	// InstanceID returns the instance identifier ("" for default, e.g. "business" for named).
	InstanceID() string

	// BaseType returns the channel type without instance suffix (e.g. "whatsapp").
	BaseType() string
}

// AutoReadConfigurable extends Channel with AutoRead configuration access.
// Channels that support configurable auto-read behavior should implement this.
type AutoReadConfigurable interface {
	Channel

	// AutoReadEnabled returns true if the channel should automatically mark
	// messages as read after passing access control and policy checks.
	AutoReadEnabled() bool

	// SetAutoRead enables or disables auto-read behavior.
	SetAutoRead(enabled bool)
}

// AccessFilter extends Channel with access control capabilities.
// Channels that implement this filter their own messages before sending to the assistant.
type AccessFilter interface {
	Channel

	// CanResponse checks if the sender is allowed to interact with the bot.
	// Returns (allowed, reason). If allowed is false, the message is ignored.
	CanResponse(msg *IncomingMessage) (allowed bool, reason string)
}

// GroupFilter extends Channel with group-specific policies.
// Channels that implement this filter group messages based on activation modes.
type GroupFilter interface {
	Channel

	// ShouldRespond checks if the bot should respond to a group message.
	// Returns true if the message matches the group's activation policy.
	ShouldRespond(msg *IncomingMessage, trigger string) bool
}

// WhatsAppAccessManager provides access control and group policy management
// for WhatsApp channels. This interface allows the API layer to manage
// access control without using reflection.
type WhatsAppAccessManager interface {
	// Access control
	GetAccessConfig() any
	GrantAccess(jid, level string)
	RevokeAccess(jid string)
	ReplaceAccessList(owners, admins, allowedUsers, blockedUsers []string, defaultPolicy, pendingMessage string)
	BlockUser(jid string)
	UnblockUser(jid string)
	SetDefaultPolicy(policy string)

	// Group policies
	GetGroupPolicies() any
	SetGroupPolicy(groupJID string, policy any)
	SetGroupDefaultPolicy(policy string)
	GetJoinedGroups() ([]WhatsAppJoinedGroup, error)
	ListGroupPoliciesForConfig() []GroupPolicyConfig

	// Configuration
	SetAutoRead(enabled bool)
	SetSendTyping(enabled bool)
	SetTrigger(trigger string)
}

// GroupPolicyConfig defines policy for a specific group (for interface/cross-package use).
type GroupPolicyConfig struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Policy       string   `yaml:"policy"`
	Policies     []string `yaml:"policies,omitempty"`
	Keywords     []string `yaml:"keywords,omitempty"`
	AllowedUsers []string `yaml:"allowed_users,omitempty"`
	Workspace    string   `yaml:"workspace,omitempty"`
}

// WhatsAppJoinedGroup represents a WhatsApp group the bot is part of.
type WhatsAppJoinedGroup struct {
	JID  string
	Name string
}

// IncomingMessage represents a message received from any channel.
type IncomingMessage struct {
	// ID is the unique message identifier in the source channel.
	ID string

	// Channel identifies the source channel (e.g. "whatsapp").
	Channel string

	// From is the sender identifier on the platform.
	From string

	// FromName is the sender display name (if available).
	FromName string

	// ChatID is the group or DM identifier.
	ChatID string

	// IsGroup indicates whether the message is from a group chat.
	IsGroup bool

	// Type is the message content type.
	Type MessageType

	// Content is the text content of the message.
	Content string

	// Timestamp is when the message was sent.
	Timestamp time.Time

	// ReplyTo contains the ID of the message being replied to.
	ReplyTo string

	// QuotedContent is the text of the quoted message (if replying).
	QuotedContent string

	// Mentions contains JIDs of users mentioned in the message (for groups).
	Mentions []string

	// Media contains media attachment details (if any).
	Media *MediaInfo

	// Location contains location data (if MessageLocation).
	Location *LocationInfo

	// Contact contains contact data (if MessageContact).
	Contact *ContactInfo

	// Reaction contains reaction data (if MessageReaction).
	Reaction *ReactionInfo

	// Metadata contains additional channel-specific data.
	Metadata map[string]any
}

// OutgoingMessage represents a message to be sent through a channel.
type OutgoingMessage struct {
	// Content is the text content of the message.
	Content string

	// ReplyTo contains the ID of the message to reply to.
	ReplyTo string

	// Attachments contains media attachments to send with the message.
	Attachments []*MediaAttachment

	// EditMessageID, if set, indicates this is an edit of an existing message
	// rather than a new message. The channel should update the original message.
	EditMessageID string

	// IsReasoning indicates this message contains internal reasoning/thinking
	// that should not be sent to end users on certain channels.
	IsReasoning bool

	// Metadata contains additional channel-specific data.
	Metadata map[string]any
}

// MediaAttachment references stored media for sending.
type MediaAttachment struct {
	// MediaID is the stored media identifier.
	MediaID string

	// Type is the media type (for convenience).
	Type MessageType

	// Caption is the text accompanying the media.
	Caption string
}

// MediaMessage represents a media file to be sent.
type MediaMessage struct {
	// Type is the media type (image, audio, video, document, sticker).
	Type MessageType

	// Data is the raw media bytes. Either Data or URL must be set.
	Data []byte

	// URL is a URL to the media file. Either Data or URL must be set.
	URL string

	// MimeType is the MIME type (e.g. "image/jpeg", "audio/ogg").
	MimeType string

	// Filename is the original filename (for documents).
	Filename string

	// Caption is the text caption accompanying the media.
	Caption string

	// Duration is the duration in seconds (for audio/video).
	Duration uint32

	// Width is the media width in pixels (for images/video).
	Width uint32

	// Height is the media height in pixels (for images/video).
	Height uint32

	// ReplyTo contains the ID of the message to reply to.
	ReplyTo string
}

// MediaInfo describes media attached to an incoming message.
type MediaInfo struct {
	// Type is the media type.
	Type MessageType

	// MimeType is the MIME type of the media.
	MimeType string

	// Filename is the original filename (for documents).
	Filename string

	// FileSize is the size in bytes.
	FileSize uint64

	// Caption is the media caption text.
	Caption string

	// Duration is the duration in seconds (audio/video).
	Duration uint32

	// Width is the width in pixels (images/video).
	Width uint32

	// Height is the height in pixels (images/video).
	Height uint32

	// URL is a direct download URL (if available).
	URL string

	// DirectPath is the platform-specific media path (for whatsapp).
	DirectPath string

	// MediaKey is the encryption key for the media (for whatsapp).
	MediaKey []byte

	// FileSHA256 is the SHA256 hash of the file.
	FileSHA256 []byte

	// FileEncSHA256 is the SHA256 hash of the encrypted file.
	FileEncSHA256 []byte
}

// LocationInfo contains location coordinates.
type LocationInfo struct {
	Latitude  float64
	Longitude float64
	Name      string
	Address   string
	URL       string
	AccuracyM uint32
}

// ContactInfo contains shared contact data.
type ContactInfo struct {
	DisplayName string
	VCard       string
	Phone       string
}

// ReactionInfo contains reaction emoji data.
type ReactionInfo struct {
	Emoji     string
	MessageID string // The message being reacted to.
	From      string
	Remove    bool // True if the reaction is being removed.
}

// HealthStatus represents the health state of a channel.
type HealthStatus struct {
	Connected     bool
	LastMessageAt time.Time
	ErrorCount    int
	LatencyMs     int64
	Details       map[string]any
}

// ChannelConfig contains common configuration for all channels.
type ChannelConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Trigger        string `yaml:"trigger"`
	MaxRetries     int    `yaml:"max_retries"`
	RetryBackoffMs int    `yaml:"retry_backoff_ms"`
}

// validInstanceID matches alphanumeric, underscore, and hyphen; 1–64 chars.
var validInstanceID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// ValidateInstanceID checks that id is safe to use as a channel instance identifier.
// An empty string is always valid (it represents the default instance).
// Named instances must be 1–64 characters of [a-zA-Z0-9_-].
func ValidateInstanceID(id string) error {
	if id == "" {
		return nil
	}
	if !validInstanceID.MatchString(id) {
		return fmt.Errorf("invalid instance ID %q: must be 1-64 alphanumeric, underscore, or hyphen characters", id)
	}
	return nil
}

// ValidChannelTypes lists the known channel type slugs.
var ValidChannelTypes = map[string]bool{
	"whatsapp": true,
	"telegram": true,
	"discord":  true,
	"slack":    true,
}

// Errors.
var (
	ErrChannelDisconnected = fmt.Errorf("channel is not connected")
	ErrSendFailed          = fmt.Errorf("failed to send message")
	ErrConnectionFailed    = fmt.Errorf("failed to connect to channel")
	ErrMediaNotSupported   = fmt.Errorf("media not supported by this channel")
	ErrMediaDownloadFailed = fmt.Errorf("failed to download media")
	ErrInvalidInstanceID   = fmt.Errorf("invalid instance ID")
)

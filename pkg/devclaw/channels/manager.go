// Package channels – manager.go orchestrates multiple communication channels,
// providing a single entry point to receive messages from all platforms
// and route responses to the correct channel.
package channels

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Manager orchestrates multiple communication channels, aggregating
// incoming messages into a single stream and routing responses.
type Manager struct {
	channels        map[string]Channel
	messages        chan *IncomingMessage
	logger          *slog.Logger
	listenWg        sync.WaitGroup
	activeListeners map[string]bool // tracks channels with running listener goroutines

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager creates a new channel manager.
func NewManager(logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}

	return &Manager{
		channels:        make(map[string]Channel),
		messages:        make(chan *IncomingMessage, 256),
		logger:          logger,
		activeListeners: make(map[string]bool),
	}
}

// Register adds a channel. Must be called before Start.
func (m *Manager) Register(ch Channel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := ch.Name()
	if _, exists := m.channels[name]; exists {
		return fmt.Errorf("channel %q already registered", name)
	}

	m.channels[name] = ch
	m.logger.Info("channel registered", "channel", name)
	return nil
}

// Start connects all registered channels and begins listening for messages.
// Channels that fail to connect are logged but don't block others.
// The listen goroutine is started for ALL channels (even failed ones),
// so reconnections via web UI or background retries deliver messages.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	m.mu.RLock()
	snapshot := make(map[string]Channel, len(m.channels))
	for k, v := range m.channels {
		snapshot[k] = v
	}
	m.mu.RUnlock()

	if len(snapshot) == 0 {
		m.logger.Warn("no channels registered, running without messaging")
		return nil
	}

	var connected int
	for name, ch := range snapshot {
		if err := ch.Connect(m.ctx); err != nil {
			m.logger.Error("failed to connect channel",
				"channel", name, "error", err)
		} else {
			connected++
			m.logger.Info("channel connected", "channel", name)
			// Start listening only for successfully connected channels.
			// Channels that fail here will get a listener via ConnectChannel on reconnect.
			m.mu.Lock()
			m.startListener(ch)
			m.mu.Unlock()
		}
	}

	if connected == 0 {
		return fmt.Errorf("no channel connected successfully")
	}

	m.logger.Info("manager started", "channels_connected", connected)
	return nil
}

// Stop gracefully disconnects all channels.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}

	// Disconnect channels first — this closes their Receive() channels,
	// which unblocks listenChannel goroutines.
	m.mu.RLock()
	for name, ch := range m.channels {
		if err := ch.Disconnect(); err != nil {
			m.logger.Error("error disconnecting channel",
				"channel", name, "error", err)
		}
	}
	m.mu.RUnlock()

	// Now wait for listener goroutines to finish.
	m.listenWg.Wait()

	close(m.messages)
	m.logger.Info("manager stopped")
}

// Messages returns the aggregated message stream from all channels.
func (m *Manager) Messages() <-chan *IncomingMessage {
	return m.messages
}

// Send sends a message through the specified channel.
func (m *Manager) Send(ctx context.Context, channelName, to string, msg *OutgoingMessage) error {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel %q not found", channelName)
	}

	if !ch.IsConnected() {
		return fmt.Errorf("channel %q disconnected", channelName)
	}

	return ch.Send(ctx, to, msg)
}

// SendMedia sends a media message through the specified channel.
// Returns ErrMediaNotSupported if the channel doesn't support media.
func (m *Manager) SendMedia(ctx context.Context, channelName, to string, media *MediaMessage) error {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel %q not found", channelName)
	}

	mc, ok := ch.(MediaChannel)
	if !ok {
		return ErrMediaNotSupported
	}

	return mc.SendMedia(ctx, to, media)
}

// SendTyping sends a typing indicator on the specified channel.
// Silently does nothing if the channel doesn't support presence.
func (m *Manager) SendTyping(ctx context.Context, channelName, to string) {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return
	}

	if pc, ok := ch.(PresenceChannel); ok {
		_ = pc.SendTyping(ctx, to)
	}
}

// MarkRead marks messages as read on the specified channel.
// Silently does nothing if the channel doesn't support presence.
func (m *Manager) MarkRead(ctx context.Context, channelName, chatID string, messageIDs []string) {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return
	}

	if pc, ok := ch.(PresenceChannel); ok {
		_ = pc.MarkRead(ctx, chatID, messageIDs)
	}
}

// SendReaction sends an emoji reaction to a message on the specified channel.
// Silently does nothing if the channel doesn't support reactions.
func (m *Manager) SendReaction(ctx context.Context, channelName, chatID, messageID, emoji string) {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return
	}

	if rc, ok := ch.(ReactionChannel); ok {
		_ = rc.SendReaction(ctx, chatID, messageID, emoji)
	}
}

// IsBotMessage checks if the given message was sent by the bot on the specified channel.
// Returns false if the channel doesn't implement SentMessageTracker.
func (m *Manager) IsBotMessage(channelName, chatID, messageID string) bool {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()
	if !exists {
		return false
	}
	if tracker, ok := ch.(SentMessageTracker); ok {
		return tracker.IsBotMessage(chatID, messageID)
	}
	return false
}

// Channel returns a specific channel by name.
func (m *Manager) Channel(name string) (Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[name]
	return ch, ok
}

// HealthAll returns health status for all registered channels.
func (m *Manager) HealthAll() map[string]HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make(map[string]HealthStatus, len(m.channels))
	for name, ch := range m.channels {
		statuses[name] = ch.Health()
	}
	return statuses
}

// HasChannels returns true if at least one channel is registered.
func (m *Manager) HasChannels() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.channels) > 0
}

// RegisterAndConnect registers a new channel and immediately connects it.
// Use this to add channels at runtime after the manager has started
// (e.g., when a user configures a token via the Web UI).
// Returns error if the channel is already registered, manager is not started,
// or connection fails (the channel is still registered on connect failure).
func (m *Manager) RegisterAndConnect(ch Channel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := ch.Name()
	if _, exists := m.channels[name]; exists {
		return fmt.Errorf("channel %q already registered", name)
	}
	if m.ctx == nil {
		return fmt.Errorf("manager not started")
	}

	m.channels[name] = ch
	m.logger.Info("channel registered (hot-reload)", "channel", name)

	if err := ch.Connect(m.ctx); err != nil {
		m.logger.Error("failed to connect channel", "channel", name, "error", err)
		return err
	}

	m.logger.Info("channel connected (hot-reload)", "channel", name)
	m.startListener(ch)
	return nil
}

// ConnectChannel connects a specific channel by name.
// Returns error if channel not found or connection fails.
func (m *Manager) ConnectChannel(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch, exists := m.channels[name]
	if !exists {
		return fmt.Errorf("channel %q not found", name)
	}

	if ch.IsConnected() {
		return fmt.Errorf("channel %q already connected", name)
	}

	if m.ctx == nil {
		return fmt.Errorf("manager not started")
	}

	if err := ch.Connect(m.ctx); err != nil {
		m.logger.Error("failed to connect channel", "channel", name, "error", err)
		return err
	}

	m.logger.Info("channel connected", "channel", name)

	// Start listening if not already running.
	m.startListener(ch)

	return nil
}

// DisconnectChannel disconnects a specific channel by name.
func (m *Manager) DisconnectChannel(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch, exists := m.channels[name]
	if !exists {
		return fmt.Errorf("channel %q not found", name)
	}

	if !ch.IsConnected() {
		return fmt.Errorf("channel %q already disconnected", name)
	}

	if err := ch.Disconnect(); err != nil {
		m.logger.Error("failed to disconnect channel", "channel", name, "error", err)
		return err
	}

	m.logger.Info("channel disconnected", "channel", name)
	return nil
}

// ChannelStatus returns health status for a specific channel.
func (m *Manager) ChannelStatus(name string) (HealthStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ch, exists := m.channels[name]
	if !exists {
		return HealthStatus{}, fmt.Errorf("channel %q not found", name)
	}

	return ch.Health(), nil
}

// ListChannels returns the names of all registered channels.
func (m *Manager) ListChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// UnregisterChannel removes a channel by name. If the channel is connected,
// it is disconnected first. Use this for runtime instance deletion.
func (m *Manager) UnregisterChannel(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch, exists := m.channels[name]
	if !exists {
		return fmt.Errorf("channel %q not found", name)
	}

	if ch.IsConnected() {
		if err := ch.Disconnect(); err != nil {
			m.logger.Error("error disconnecting channel during unregister",
				"channel", name, "error", err)
		}
	}

	delete(m.channels, name)
	delete(m.activeListeners, name)
	m.logger.Info("channel unregistered", "channel", name)
	return nil
}

// ChannelsByType returns all channels whose base type matches. For instance-aware
// channels, this matches on BaseType(); for others, it matches on Name() equality
// or Name() having the given prefix followed by ":".
func (m *Manager) ChannelsByType(baseType string) []Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Channel
	for _, ch := range m.channels {
		if ia, ok := ch.(InstanceAware); ok {
			if ia.BaseType() == baseType {
				result = append(result, ch)
			}
		} else if ch.Name() == baseType || strings.HasPrefix(ch.Name(), baseType+":") {
			result = append(result, ch)
		}
	}
	return result
}

// startListener starts a listener goroutine for a channel if one isn't already running.
// Must be called with m.mu held.
func (m *Manager) startListener(ch Channel) {
	name := ch.Name()
	if m.activeListeners[name] {
		m.logger.Debug("listener already active, skipping", "channel", name)
		return
	}
	m.activeListeners[name] = true
	m.listenWg.Add(1)
	go func(c Channel, n string) {
		defer m.listenWg.Done()
		defer func() {
			m.mu.Lock()
			delete(m.activeListeners, n)
			m.mu.Unlock()
		}()
		m.listenChannel(c)
	}(ch, name)
}

// listenChannel listens for messages from a channel and forwards them
// to the aggregated stream. Exits when the channel closes or context is cancelled.
func (m *Manager) listenChannel(ch Channel) {
	incoming := ch.Receive()
	for {
		select {
		case msg, ok := <-incoming:
			if !ok {
				return // Channel closed.
			}
			select {
			case m.messages <- msg:
			case <-m.ctx.Done():
				return
			}
		case <-m.ctx.Done():
			return
		}
	}
}

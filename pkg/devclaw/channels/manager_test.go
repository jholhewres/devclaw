package channels_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

// fakeChannel is a minimal Channel implementation for testing.
type fakeChannel struct {
	name      string
	connected bool
	messages  chan *channels.IncomingMessage
}

func newFake(name string) *fakeChannel {
	return &fakeChannel{
		name:     name,
		messages: make(chan *channels.IncomingMessage, 8),
	}
}

func (f *fakeChannel) Name() string                                                       { return f.name }
func (f *fakeChannel) Connect(_ context.Context) error                                    { f.connected = true; return nil }
func (f *fakeChannel) Disconnect() error                                                  { f.connected = false; return nil }
func (f *fakeChannel) Send(_ context.Context, _ string, _ *channels.OutgoingMessage) error { return nil }
func (f *fakeChannel) Receive() <-chan *channels.IncomingMessage                           { return f.messages }
func (f *fakeChannel) IsConnected() bool                                                  { return f.connected }
func (f *fakeChannel) Health() channels.HealthStatus                                      { return channels.HealthStatus{Connected: f.connected} }

// fakeInstanceChannel extends fakeChannel with InstanceAware.
type fakeInstanceChannel struct {
	fakeChannel
	instanceID string
	baseType   string
}

func newFakeInstance(baseType, instanceID string) *fakeInstanceChannel {
	name := baseType
	if instanceID != "" {
		name = baseType + ":" + instanceID
	}
	return &fakeInstanceChannel{
		fakeChannel: fakeChannel{
			name:     name,
			messages: make(chan *channels.IncomingMessage, 8),
		},
		instanceID: instanceID,
		baseType:   baseType,
	}
}

func (f *fakeInstanceChannel) InstanceID() string { return f.instanceID }
func (f *fakeInstanceChannel) BaseType() string   { return f.baseType }

func TestManagerRegisterMultiInstance(t *testing.T) {
	mgr := channels.NewManager(slog.Default())

	wa := newFakeInstance("whatsapp", "")
	waBiz := newFakeInstance("whatsapp", "business")

	if err := mgr.Register(wa); err != nil {
		t.Fatalf("register default: %v", err)
	}
	if err := mgr.Register(waBiz); err != nil {
		t.Fatalf("register business: %v", err)
	}

	// Both should be listed.
	names := mgr.ListChannels()
	if len(names) != 2 {
		t.Fatalf("expected 2 channels, got %d: %v", len(names), names)
	}

	// Each should be retrievable by its full name.
	if _, ok := mgr.Channel("whatsapp"); !ok {
		t.Error("expected to find 'whatsapp'")
	}
	if _, ok := mgr.Channel("whatsapp:business"); !ok {
		t.Error("expected to find 'whatsapp:business'")
	}
}

func TestManagerChannelsByType(t *testing.T) {
	mgr := channels.NewManager(slog.Default())

	mgr.Register(newFakeInstance("whatsapp", ""))
	mgr.Register(newFakeInstance("whatsapp", "business"))
	mgr.Register(newFakeInstance("whatsapp", "alerts"))
	mgr.Register(newFakeInstance("telegram", ""))

	waChs := mgr.ChannelsByType("whatsapp")
	if len(waChs) != 3 {
		t.Fatalf("expected 3 whatsapp channels, got %d", len(waChs))
	}

	tgChs := mgr.ChannelsByType("telegram")
	if len(tgChs) != 1 {
		t.Fatalf("expected 1 telegram channel, got %d", len(tgChs))
	}

	dcChs := mgr.ChannelsByType("discord")
	if len(dcChs) != 0 {
		t.Fatalf("expected 0 discord channels, got %d", len(dcChs))
	}
}

func TestManagerUnregisterChannel(t *testing.T) {
	mgr := channels.NewManager(slog.Default())

	ch := newFakeInstance("whatsapp", "business")
	mgr.Register(ch)
	ch.connected = true

	if err := mgr.UnregisterChannel("whatsapp:business"); err != nil {
		t.Fatalf("unregister: %v", err)
	}

	// Should no longer be found.
	if _, ok := mgr.Channel("whatsapp:business"); ok {
		t.Error("channel should be removed after unregister")
	}

	// Should have been disconnected.
	if ch.connected {
		t.Error("channel should have been disconnected during unregister")
	}

	// Unregistering again should fail.
	if err := mgr.UnregisterChannel("whatsapp:business"); err == nil {
		t.Error("expected error unregistering non-existent channel")
	}
}

func TestManagerDuplicateRegister(t *testing.T) {
	mgr := channels.NewManager(slog.Default())

	mgr.Register(newFakeInstance("whatsapp", ""))
	err := mgr.Register(newFakeInstance("whatsapp", ""))
	if err == nil {
		t.Error("expected error registering duplicate channel name")
	}
}

func TestManagerChannelsByTypeWithoutInterface(t *testing.T) {
	mgr := channels.NewManager(slog.Default())

	// A plain channel (no InstanceAware) should match by name prefix.
	mgr.Register(newFake("whatsapp"))
	mgr.Register(newFake("whatsapp:custom"))

	waChs := mgr.ChannelsByType("whatsapp")
	if len(waChs) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(waChs))
	}
}

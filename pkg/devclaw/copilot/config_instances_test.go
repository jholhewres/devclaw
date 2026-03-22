package copilot

import (
	"testing"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels/discord"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/slack"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/telegram"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/whatsapp"
)

func TestConfigWhatsAppAll(t *testing.T) {
	cfg := ChannelsConfig{
		WhatsApp: whatsapp.Config{Trigger: "@default"},
		WhatsAppInstances: map[string]whatsapp.Config{
			"business": {Trigger: "@support"},
			"alerts":   {Trigger: "@alerts"},
		},
	}

	all := cfg.WhatsAppAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 instances, got %d", len(all))
	}
	if all[""].Trigger != "@default" {
		t.Errorf("default trigger = %q, want @default", all[""].Trigger)
	}
	if all["business"].Trigger != "@support" {
		t.Errorf("business trigger = %q, want @support", all["business"].Trigger)
	}
	if all["alerts"].Trigger != "@alerts" {
		t.Errorf("alerts trigger = %q, want @alerts", all["alerts"].Trigger)
	}
}

func TestConfigWhatsAppAllEmpty(t *testing.T) {
	cfg := ChannelsConfig{
		WhatsApp: whatsapp.Config{Trigger: "@default"},
	}

	all := cfg.WhatsAppAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 instance (default only), got %d", len(all))
	}
	if _, ok := all[""]; !ok {
		t.Error("expected default instance with empty key")
	}
}

func TestConfigTelegramAll(t *testing.T) {
	cfg := ChannelsConfig{
		Telegram: telegram.Config{Token: "default-token"},
		TelegramInstances: map[string]telegram.Config{
			"alerts": {Token: "alerts-token"},
		},
	}

	all := cfg.TelegramAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(all))
	}
	if all[""].Token != "default-token" {
		t.Error("default token mismatch")
	}
	if all["alerts"].Token != "alerts-token" {
		t.Error("alerts token mismatch")
	}
}

func TestConfigDiscordAll(t *testing.T) {
	cfg := ChannelsConfig{
		Discord: discord.Config{Token: "default-token"},
		DiscordInstances: map[string]discord.Config{
			"gaming": {Token: "gaming-token"},
		},
	}

	all := cfg.DiscordAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(all))
	}
}

func TestConfigSlackAll(t *testing.T) {
	cfg := ChannelsConfig{
		Slack: slack.Config{BotToken: "default-token"},
		SlackInstances: map[string]slack.Config{
			"support": {BotToken: "support-token"},
		},
	}

	all := cfg.SlackAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(all))
	}
}

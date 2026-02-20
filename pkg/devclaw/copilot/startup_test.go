package copilot

import (
	"log/slog"
	"os"
	"testing"
)

func TestStartupVerifier_CheckConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("valid config", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Name = "test-bot"
		cfg.Model = "claude-3-sonnet"

		sv := NewStartupVerifier(cfg, nil, logger)
		result := sv.checkConfig()

		if result.Name != "config" {
			t.Errorf("expected name 'config', got %s", result.Name)
		}
		if result.Status != "ok" {
			t.Errorf("expected status 'ok', got %s: %s", result.Status, result.Message)
		}
		if !result.Required {
			t.Error("config check should be required")
		}
	})

	t.Run("config without name", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Name = ""

		sv := NewStartupVerifier(cfg, nil, logger)
		result := sv.checkConfig()

		if result.Status != "warning" {
			t.Errorf("expected status 'warning', got %s", result.Status)
		}
	})

	t.Run("nil config", func(t *testing.T) {
		sv := NewStartupVerifier(nil, nil, logger)
		result := sv.checkConfig()

		if result.Status != "error" {
			t.Errorf("expected status 'error', got %s", result.Status)
		}
	})
}

func TestStartupVerifier_CheckVault(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("no vault", func(t *testing.T) {
		cfg := DefaultConfig()
		sv := NewStartupVerifier(cfg, nil, logger)
		result := sv.checkVault()

		if result.Name != "vault" {
			t.Errorf("expected name 'vault', got %s", result.Name)
		}
		// No vault is fine (skipped status)
		if result.Required {
			t.Error("vault check should not be required")
		}
	})

	t.Run("vault locked", func(t *testing.T) {
		cfg := DefaultConfig()
		vault := NewVault(VaultFile)
		// Vault exists but locked (not unlocked)
		sv := NewStartupVerifier(cfg, vault, logger)
		result := sv.checkVault()

		// Should be warning or skipped depending on vault state
		if result.Required {
			t.Error("vault check should not be required")
		}
	})
}

func TestStartupVerifier_CheckAPIKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("api key in config", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.API.APIKey = "sk-test-key"

		sv := NewStartupVerifier(cfg, nil, logger)
		result := sv.checkAPIKey()

		if result.Status != "ok" {
			t.Errorf("expected status 'ok', got %s: %s", result.Status, result.Message)
		}
		if !result.Required {
			t.Error("api_key check should be required")
		}
	})

	t.Run("api key in env", func(t *testing.T) {
		cfg := DefaultConfig()
		// Clear config key
		cfg.API.APIKey = ""

		// Set env variable
		os.Setenv("DEVCLAW_API_KEY", "sk-env-key")
		defer os.Unsetenv("DEVCLAW_API_KEY")

		sv := NewStartupVerifier(cfg, nil, logger)
		result := sv.checkAPIKey()

		if result.Status != "ok" {
			t.Errorf("expected status 'ok', got %s: %s", result.Status, result.Message)
		}
	})

	t.Run("no api key", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.API.APIKey = ""

		// Clear env variables
		os.Unsetenv("DEVCLAW_API_KEY")
		os.Unsetenv("OPENAI_API_KEY")

		sv := NewStartupVerifier(cfg, nil, logger)
		result := sv.checkAPIKey()

		if result.Status != "error" {
			t.Errorf("expected status 'error', got %s", result.Status)
		}
	})
}

func TestStartupVerifier_CheckChannels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("no channels configured", func(t *testing.T) {
		cfg := DefaultConfig()
		// No channel tokens set

		sv := NewStartupVerifier(cfg, nil, logger)
		result := sv.checkChannels()

		// WhatsApp is always available (QR pairing)
		if result.Status == "error" {
			t.Errorf("expected non-error status, got %s: %s", result.Status, result.Message)
		}
	})

	t.Run("channels configured", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Channels.Telegram.Token = "test-token"
		cfg.Channels.Discord.Token = "test-token"

		sv := NewStartupVerifier(cfg, nil, logger)
		result := sv.checkChannels()

		if result.Status != "ok" && result.Status != "warning" {
			t.Errorf("expected status 'ok' or 'warning', got %s", result.Status)
		}
	})
}

func TestStartupVerifier_RunAll(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("complete verification", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Name = "test-bot"
		cfg.API.APIKey = "sk-test-key"

		// Set env variable to ensure API key is found
		os.Setenv("DEVCLAW_API_KEY", "sk-test-key")
		defer os.Unsetenv("DEVCLAW_API_KEY")

		sv := NewStartupVerifier(cfg, nil, logger)
		report := sv.RunAll()

		if report == nil {
			t.Fatal("expected non-nil report")
		}
		if len(report.Results) == 0 {
			t.Error("expected some check results")
		}

		// Should have checks for: config, vault, api_key, database, data_dirs, channels, media
		expectedChecks := []string{"config", "vault", "api_key", "database", "data_dirs", "channels", "media"}
		foundChecks := make(map[string]bool)
		for _, r := range report.Results {
			foundChecks[r.Name] = true
		}
		for _, expected := range expectedChecks {
			if !foundChecks[expected] {
				t.Errorf("missing check: %s", expected)
			}
		}
	})

	t.Run("healthy report with api key", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Name = "test-bot"
		cfg.API.APIKey = "sk-test-key"

		os.Setenv("DEVCLAW_API_KEY", "sk-test-key")
		defer os.Unsetenv("DEVCLAW_API_KEY")

		sv := NewStartupVerifier(cfg, nil, logger)
		report := sv.RunAll()

		if !report.Healthy {
			t.Error("expected healthy report when api key is available")
		}
	})

	t.Run("unhealthy report without api key", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Name = "test-bot"
		cfg.API.APIKey = ""

		os.Unsetenv("DEVCLAW_API_KEY")
		os.Unsetenv("OPENAI_API_KEY")

		sv := NewStartupVerifier(cfg, nil, logger)
		report := sv.RunAll()

		if report.Healthy {
			t.Error("expected unhealthy report when api key is missing")
		}
	})
}

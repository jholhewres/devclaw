package copilot

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestInjectVaultSecrets_PopulatesConfig(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "test.vault")
	v := NewVault(vaultPath)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}
	v.Set("api_key", "sk-test-12345")
	v.Set("webui_token", "my-webui-password")
	v.Set("custom_key", "custom-value") // no env mapping for this

	cfg := DefaultConfig()
	logger := slog.Default()

	injectVaultSecrets(v, cfg, logger)

	if cfg.API.APIKey != "sk-test-12345" {
		t.Errorf("API.APIKey = %q, want %q", cfg.API.APIKey, "sk-test-12345")
	}
	if cfg.WebUI.AuthToken != "my-webui-password" {
		t.Errorf("WebUI.AuthToken = %q, want %q", cfg.WebUI.AuthToken, "my-webui-password")
	}

	// Check env vars were injected.
	if got := os.Getenv("DEVCLAW_API_KEY"); got != "sk-test-12345" {
		t.Errorf("env DEVCLAW_API_KEY = %q, want %q", got, "sk-test-12345")
	}
	if got := os.Getenv("DEVCLAW_WEBUI_TOKEN"); got != "my-webui-password" {
		t.Errorf("env DEVCLAW_WEBUI_TOKEN = %q, want %q", got, "my-webui-password")
	}
}

func TestInjectVaultSecrets_UnknownKeysNotInjected(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "test.vault")
	v := NewVault(vaultPath)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}
	v.Set("unknown_key", "secret")

	cfg := DefaultConfig()
	logger := slog.Default()

	// Clean up env before test.
	os.Unsetenv("DEVCLAW_UNKNOWN_KEY")

	injectVaultSecrets(v, cfg, logger)

	// Unknown keys should NOT be injected as env vars.
	if got := os.Getenv("DEVCLAW_UNKNOWN_KEY"); got != "" {
		t.Errorf("unknown key should not be injected as env var, got %q", got)
	}
}

func TestInjectVaultSecrets_EmptyVault(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "test.vault")
	v := NewVault(vaultPath)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	logger := slog.Default()

	// Should not panic or error with empty vault.
	injectVaultSecrets(v, cfg, logger)

	if cfg.API.APIKey != "" {
		t.Errorf("expected empty API key, got %q", cfg.API.APIKey)
	}
}


// Package copilot – keyring.go provides secure credential storage using the
// operating system's native keyring (Linux: Secret Service/GNOME Keyring,
// macOS: Keychain, Windows: Credential Manager).
//
// Priority for resolving secrets:
//  1. Encrypted vault (.goclaw.vault — AES-256-GCM + Argon2, requires master password)
//  2. OS keyring (encrypted by the OS, requires user session)
//  3. Environment variable (GOCLAW_API_KEY, OPENAI_API_KEY, etc.)
//  4. .env file (loaded by godotenv)
//  5. config.yaml value (least secure — plaintext on disk)
package copilot

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	// keyringService is the service name used in the OS keyring.
	keyringService = "goclaw"

	// keyringAPIKey is the key name for the LLM API key.
	keyringAPIKey = "api_key"
)

// StoreKeyring saves a secret to the OS keyring.
func StoreKeyring(key, value string) error {
	return keyring.Set(keyringService, key, value)
}

// GetKeyring retrieves a secret from the OS keyring.
// Returns empty string if not found.
func GetKeyring(key string) string {
	val, err := keyring.Get(keyringService, key)
	if err != nil {
		return ""
	}
	return val
}

// DeleteKeyring removes a secret from the OS keyring.
func DeleteKeyring(key string) error {
	return keyring.Delete(keyringService, key)
}

// KeyringAvailable checks if the OS keyring is accessible.
func KeyringAvailable() bool {
	// Try a write+delete cycle with a test key.
	testKey := "__goclaw_test__"
	if err := keyring.Set(keyringService, testKey, "test"); err != nil {
		return false
	}
	_ = keyring.Delete(keyringService, testKey)
	return true
}

// vaultEnvMapping maps vault key names to the environment variables they
// should be injected as. This ensures ${GOCLAW_*} references in config.yaml
// resolve correctly even when the only on-disk secret is the vault password.
var vaultEnvMapping = map[string]string{
	"api_key":     "GOCLAW_API_KEY",
	"webui_token": "GOCLAW_WEBUI_TOKEN",
}

// ResolveAPIKey resolves the API key using the priority chain:
// vault → keyring → env var → config value.
// Also updates the config in-place with the resolved value.
// If a vault exists but is locked, it prompts for the master password
// (or uses GOCLAW_VAULT_PASSWORD env var for non-interactive mode).
// Returns the unlocked vault (or nil if unavailable) so it can be reused
// by the assistant for agent vault tools.
func ResolveAPIKey(cfg *Config, logger *slog.Logger) *Vault {
	// 1. Try encrypted vault first (most secure — password-protected).
	vault := NewVault(VaultFile)
	if vault.Exists() {
		if !vault.IsUnlocked() {
			// Try GOCLAW_VAULT_PASSWORD env var first (for PM2, systemd, Docker).
			if envPass := os.Getenv("GOCLAW_VAULT_PASSWORD"); envPass != "" {
				if err := vault.Unlock(envPass); err != nil {
					logger.Warn("failed to unlock vault with GOCLAW_VAULT_PASSWORD", "error", err)
				} else {
					logger.Info("vault unlocked via GOCLAW_VAULT_PASSWORD")
				}
			}
		}

		if !vault.IsUnlocked() {
			// Fall back to interactive prompt if stdin is a terminal.
			if term.IsTerminal(int(os.Stdin.Fd())) {
				password, err := ReadPassword("Vault password: ")
				if err != nil {
					logger.Warn("failed to read vault password", "error", err)
				} else if err := vault.Unlock(password); err != nil {
					logger.Warn("failed to unlock vault", "error", err)
				}
			} else {
				logger.Info("vault exists but skipping (non-interactive mode, no GOCLAW_VAULT_PASSWORD), using env/config")
			}
		}

		if vault.IsUnlocked() {
			// Inject all vault secrets into the process environment so that
			// ${GOCLAW_*} references in config.yaml resolve correctly.
			// This is the key design: .env only holds the vault password;
			// all other secrets live encrypted in the vault and are injected
			// at runtime.
			injectVaultSecrets(vault, cfg, logger)

			return vault
		}
	}

	// 2. Try OS keyring (encrypted by the OS).
	if val := GetKeyring(keyringAPIKey); val != "" {
		cfg.API.APIKey = val
		logger.Debug("API key loaded from OS keyring")
		return nil
	}

	// 3. If config already has a resolved value (from env expansion), keep it.
	if cfg.API.APIKey != "" && !IsEnvReference(cfg.API.APIKey) {
		logger.Debug("API key loaded from config/env")
		return nil
	}

	logger.Warn("no API key found. Set one with: copilot config set-key or copilot config vault-set")
	return nil
}

// injectVaultSecrets reads all secrets from the unlocked vault, sets them as
// environment variables (GOCLAW_<KEY>), and resolves known config fields.
// This allows config.yaml to use ${GOCLAW_API_KEY}, ${GOCLAW_WEBUI_TOKEN},
// etc. without those values ever touching .env or config files in plain text.
func injectVaultSecrets(vault *Vault, cfg *Config, logger *slog.Logger) {
	keys := vault.List()
	injected := 0

	for _, key := range keys {
		val, err := vault.Get(key)
		if err != nil || val == "" {
			continue
		}

		// Inject as env var if there's a known mapping.
		if envName, ok := vaultEnvMapping[key]; ok {
			os.Setenv(envName, val)
			injected++
			logger.Debug("vault secret injected as env var", "key", key, "env", envName)
		}
	}

	// Resolve known config fields from the injected env vars.
	if val, err := vault.Get("api_key"); err == nil && val != "" {
		cfg.API.APIKey = val
		logger.Debug("API key loaded from encrypted vault")
	}
	if val, err := vault.Get("webui_token"); err == nil && val != "" {
		cfg.WebUI.AuthToken = val
		logger.Debug("WebUI auth token loaded from encrypted vault")
	}

	if injected > 0 {
		logger.Info("vault secrets injected into process environment",
			"count", injected)
	}
}

// MigrateKeyToKeyring moves an API key from config/env to the OS keyring
// and clears it from the original location.
func MigrateKeyToKeyring(apiKey string, logger *slog.Logger) error {
	if err := StoreKeyring(keyringAPIKey, apiKey); err != nil {
		return fmt.Errorf("storing in keyring: %w", err)
	}
	logger.Info("API key stored in OS keyring",
		"service", keyringService,
		"hint", "You can now remove it from .env and config.yaml")
	return nil
}

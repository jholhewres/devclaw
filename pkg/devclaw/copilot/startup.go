// Package copilot provides startup verification for DevClaw.
// Checks vault, database, channels, and system dependencies.
package copilot

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// StartupCheckResult represents the result of a single startup check.
type StartupCheckResult struct {
	Name     string // Check name (e.g., "vault", "database", "channels")
	Status   string // "ok", "warning", "error", "skipped"
	Message  string // Human-readable message
	Required bool   // If true, failure blocks startup
}

// StartupReport contains all startup check results.
type StartupReport struct {
	Results []StartupCheckResult
	Healthy bool // true if all required checks pass
}

// StartupVerifier performs system checks at initialization.
type StartupVerifier struct {
	config *Config
	vault  *Vault
	logger *slog.Logger
}

// NewStartupVerifier creates a new startup verifier.
func NewStartupVerifier(cfg *Config, vault *Vault, logger *slog.Logger) *StartupVerifier {
	return &StartupVerifier{
		config: cfg,
		vault:  vault,
		logger: logger.With("component", "startup"),
	}
}

// RunAll executes all startup checks and returns a report.
func (sv *StartupVerifier) RunAll() *StartupReport {
	report := &StartupReport{
		Results: make([]StartupCheckResult, 0, 10),
		Healthy: true,
	}

	// Run all checks in order.
	sv.runCheck(report, sv.checkConfig)
	sv.runCheck(report, sv.checkVault)
	sv.runCheck(report, sv.checkAPIKey)
	sv.runCheck(report, sv.checkDatabase)
	sv.runCheck(report, sv.checkDataDirs)
	sv.runCheck(report, sv.checkChannels)
	sv.runCheck(report, sv.checkMedia)

	// Determine overall health.
	for _, r := range report.Results {
		if r.Required && r.Status == "error" {
			report.Healthy = false
			break
		}
	}

	return report
}

// runCheck executes a single check and appends the result.
func (sv *StartupVerifier) runCheck(report *StartupReport, check func() StartupCheckResult) {
	result := check()
	report.Results = append(report.Results, result)

	// Log the result.
	switch result.Status {
	case "ok":
		sv.logger.Info(result.Name+": "+result.Message, "status", "ok")
	case "warning":
		sv.logger.Warn(result.Name+": "+result.Message, "status", "warning")
	case "error":
		if result.Required {
			sv.logger.Error(result.Name+": "+result.Message, "status", "error", "required", true)
		} else {
			sv.logger.Warn(result.Name+": "+result.Message, "status", "error", "required", false)
		}
	case "skipped":
		sv.logger.Debug(result.Name+": "+result.Message, "status", "skipped")
	}
}

// checkConfig validates the configuration was loaded.
func (sv *StartupVerifier) checkConfig() StartupCheckResult {
	if sv.config == nil {
		return StartupCheckResult{
			Name:     "config",
			Status:   "error",
			Message:  "no configuration loaded",
			Required: true,
		}
	}
	if sv.config.Name == "" {
		return StartupCheckResult{
			Name:     "config",
			Status:   "warning",
			Message:  "assistant name not set",
			Required: false,
		}
	}
	return StartupCheckResult{
		Name:     "config",
		Status:   "ok",
		Message:  fmt.Sprintf("loaded (name: %s, model: %s)", sv.config.Name, sv.config.Model),
		Required: true,
	}
}

// checkVault verifies vault status.
func (sv *StartupVerifier) checkVault() StartupCheckResult {
	if sv.vault == nil {
		// Vault is optional - check if file exists.
		v := NewVault(VaultFile)
		if v.Exists() {
			return StartupCheckResult{
				Name:     "vault",
				Status:   "warning",
				Message:  "vault exists but is locked (set password via VAULT_PASSWORD env or web UI)",
				Required: false,
			}
		}
		return StartupCheckResult{
			Name:     "vault",
			Status:   "skipped",
			Message:  "no vault configured (secrets from env/keyring/config)",
			Required: false,
		}
	}

	if !sv.vault.Exists() {
		return StartupCheckResult{
			Name:     "vault",
			Status:   "skipped",
			Message:  "vault file not created",
			Required: false,
		}
	}

	if !sv.vault.IsUnlocked() {
		return StartupCheckResult{
			Name:     "vault",
			Status:   "warning",
			Message:  "vault exists but locked",
			Required: false,
		}
	}

	keys := sv.vault.List()
	return StartupCheckResult{
		Name:     "vault",
		Status:   "ok",
		Message:  fmt.Sprintf("unlocked (%d secrets)", len(keys)),
		Required: false,
	}
}

// checkAPIKey verifies API key availability.
func (sv *StartupVerifier) checkAPIKey() StartupCheckResult {
	// Check if we have any API key source.
	hasKey := false
	source := ""

	if sv.config.API.APIKey != "" {
		hasKey = true
		source = "config"
	}
	if sv.vault != nil && sv.vault.IsUnlocked() {
		if key, _ := sv.vault.Get("api_key"); key != "" {
			hasKey = true
			source = "vault"
		}
	}
	if GetKeyring("api_key") != "" {
		hasKey = true
		source = "keyring"
	}
	if os.Getenv("DEVCLAW_API_KEY") != "" {
		hasKey = true
		source = "env:DEVCLAW_API_KEY"
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		hasKey = true
		source = "env:OPENAI_API_KEY"
	}

	if !hasKey {
		return StartupCheckResult{
			Name:     "api_key",
			Status:   "error",
			Message:  "no API key found (set via vault, keyring, env, or config)",
			Required: true,
		}
	}

	return StartupCheckResult{
		Name:     "api_key",
		Status:   "ok",
		Message:  fmt.Sprintf("available (source: %s)", source),
		Required: true,
	}
}

// checkDatabase verifies database connectivity.
func (sv *StartupVerifier) checkDatabase() StartupCheckResult {
	dbPath := sv.config.Database.Path
	if dbPath == "" {
		dbPath = "./data/devclaw.db"
	}

	// Check if database directory exists.
	dbDir := filepath.Dir(dbPath)
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		return StartupCheckResult{
			Name:     "database",
			Status:   "warning",
			Message:  fmt.Sprintf("database directory %s does not exist (will be created)", dbDir),
			Required: false,
		}
	}

	// Check if database file exists.
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return StartupCheckResult{
			Name:     "database",
			Status:   "warning",
			Message:  fmt.Sprintf("database file %s does not exist (will be created)", dbPath),
			Required: false,
		}
	}

	return StartupCheckResult{
		Name:     "database",
		Status:   "ok",
		Message:  fmt.Sprintf("ready (%s)", dbPath),
		Required: false,
	}
}

// checkDataDirs verifies required data directories.
func (sv *StartupVerifier) checkDataDirs() StartupCheckResult {
	dirs := []string{
		"./data",
		"./data/memory",
		"./data/sessions",
		"./sessions",
	}

	missing := []string{}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			// Try to create it.
			if err := os.MkdirAll(dir, 0755); err != nil {
				missing = append(missing, dir)
			}
		}
	}

	if len(missing) > 0 {
		return StartupCheckResult{
			Name:     "data_dirs",
			Status:   "warning",
			Message:  fmt.Sprintf("could not create directories: %v", missing),
			Required: false,
		}
	}

	return StartupCheckResult{
		Name:     "data_dirs",
		Status:   "ok",
		Message:  "all data directories ready",
		Required: false,
	}
}

// checkChannels verifies channel configurations.
func (sv *StartupVerifier) checkChannels() StartupCheckResult {
	channels := []struct {
		name    string
		enabled bool
		hasCred bool
	}{
		{"whatsapp", true, true}, // WhatsApp uses QR pairing, no pre-configured token
		{"telegram", sv.config.Channels.Telegram.Token != "", sv.config.Channels.Telegram.Token != ""},
		{"discord", sv.config.Channels.Discord.Token != "", sv.config.Channels.Discord.Token != ""},
		{"slack", sv.config.Channels.Slack.BotToken != "", sv.config.Channels.Slack.BotToken != ""},
	}

	enabled := []string{}
	missing := []string{}

	for _, ch := range channels {
		if ch.name == "whatsapp" {
			// WhatsApp is always potentially enabled (QR pairing).
			enabled = append(enabled, "whatsapp")
		} else if ch.enabled {
			if ch.hasCred {
				enabled = append(enabled, ch.name)
			} else {
				missing = append(missing, ch.name)
			}
		}
	}

	if len(enabled) == 0 && len(missing) == 0 {
		return StartupCheckResult{
			Name:     "channels",
			Status:   "warning",
			Message:  "no channels configured",
			Required: false,
		}
	}

	msg := ""
	if len(enabled) > 0 {
		msg = fmt.Sprintf("enabled: %v", enabled)
	}
	if len(missing) > 0 {
		if msg != "" {
			msg += ", "
		}
		msg += fmt.Sprintf("missing credentials: %v", missing)
	}

	status := "ok"
	if len(enabled) == 0 {
		status = "warning"
	}

	return StartupCheckResult{
		Name:     "channels",
		Status:   status,
		Message:  msg,
		Required: false,
	}
}

// checkMedia verifies media processing configuration.
func (sv *StartupVerifier) checkMedia() StartupCheckResult {
	media := sv.config.Media.Effective()

	features := []string{}
	if media.VisionEnabled {
		features = append(features, "vision")
	}
	if media.TranscriptionEnabled {
		features = append(features, "transcription")
	}

	if len(features) == 0 {
		return StartupCheckResult{
			Name:     "media",
			Status:   "skipped",
			Message:  "media processing disabled",
			Required: false,
		}
	}

	return StartupCheckResult{
		Name:     "media",
		Status:   "ok",
		Message:  fmt.Sprintf("enabled: %v", features),
		Required: false,
	}
}

// PrintReport logs a formatted startup report.
func (sv *StartupVerifier) PrintReport(report *StartupReport) {
	sv.logger.Info("═══════════════════════════════════════════════════════════")
	sv.logger.Info("                 DevClaw Startup Verification              ")
	sv.logger.Info("═══════════════════════════════════════════════════════════")

	for _, r := range report.Results {
		icon := "✓"
		switch r.Status {
		case "ok":
			icon = "✓"
		case "warning":
			icon = "⚠"
		case "error":
			icon = "✗"
		case "skipped":
			icon = "○"
		}

		req := ""
		if r.Required {
			req = " [required]"
		}

		sv.logger.Info(fmt.Sprintf("  %s %-12s %s%s", icon, r.Name+":", r.Message, req))
	}

	sv.logger.Info("───────────────────────────────────────────────────────────")
	if report.Healthy {
		sv.logger.Info("  Status: All required checks passed ✓")
	} else {
		sv.logger.Warn("  Status: Some required checks failed ✗")
	}
	sv.logger.Info("═══════════════════════════════════════════════════════════")
}

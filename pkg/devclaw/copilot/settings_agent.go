package copilot

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// registerSettingsTool registers the `settings` tool, letting the main agent
// inspect and change its own whitelisted runtime settings (media/model) with
// immediate hot-reload. Owner/admin only. Backed by the executor's settings
// handlers, wired by the Assistant.
func registerSettingsTool(executor *ToolExecutor) {
	executor.Register(
		MakeToolDefinition("settings",
			"Read or change the agent's own runtime settings with immediate hot-reload (no restart). "+
				"Whitelisted keys only: media.vision_enabled, media.vision_model (empty = use main model), "+
				"media.vision_detail (auto/low/high), media.transcription_enabled, media.transcription_model, "+
				"media.transcription_base_url, media.transcription_language, model. Owner/admin only.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"get", "set"},
						"description": "get the current settings, or set one key",
					},
					"key": map[string]any{
						"type":        "string",
						"description": "setting key to change (required for set)",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "new value (required for set; booleans as true/false)",
					},
				},
				"required": []string{"action"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			if lvl := CallerLevelFromContext(ctx); lvl != AccessOwner && lvl != AccessAdmin {
				return nil, fmt.Errorf("permission denied: settings requires owner or admin")
			}
			get, set := executor.settingsHandlers()
			if get == nil || set == nil {
				return nil, fmt.Errorf("settings management is not available")
			}
			action, _ := args["action"].(string)
			switch action {
			case "get", "":
				return get()
			case "set":
				key, _ := args["key"].(string)
				value, _ := args["value"].(string)
				if strings.TrimSpace(key) == "" {
					return nil, fmt.Errorf("key is required for action=set")
				}
				return set(key, value)
			default:
				return nil, fmt.Errorf("unknown action %q (use get or set)", action)
			}
		},
	)
}

// SetConfigPath records the on-disk config path so the `settings` tool can
// persist whitelisted runtime changes. Called once at startup by the server.
func (a *Assistant) SetConfigPath(path string) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.configPath = path
}

// allowedSettings is the whitelist of keys the main agent may change via the
// `settings` tool. Deliberately limited to media (vision/transcription) and the
// main model — never security, access, vault, channels, or API keys.
var allowedSettings = []string{
	"media.vision_enabled",
	"media.vision_model",
	"media.vision_detail",
	"media.transcription_enabled",
	"media.transcription_model",
	"media.transcription_base_url",
	"media.transcription_language",
	"model",
}

// applySettingToConfig validates and applies a single whitelisted setting onto
// cfg, returning whether the main model changed. Pure (no I/O) for testability;
// the whitelist here is the security boundary for agent self-configuration.
func applySettingToConfig(cfg *Config, key, value string) (modelChanged bool, err error) {
	parseBool := func() (bool, error) {
		b, perr := strconv.ParseBool(value)
		if perr != nil {
			return false, fmt.Errorf("%q expects a boolean (true/false), got %q", key, value)
		}
		return b, nil
	}
	switch key {
	case "media.vision_enabled":
		b, err := parseBool()
		if err != nil {
			return false, err
		}
		cfg.Media.VisionEnabled = b
	case "media.vision_model":
		cfg.Media.VisionModel = value // empty = use main model
	case "media.vision_detail":
		switch value {
		case "auto", "low", "high":
			cfg.Media.VisionDetail = value
		default:
			return false, fmt.Errorf("media.vision_detail must be auto, low, or high (got %q)", value)
		}
	case "media.transcription_enabled":
		b, err := parseBool()
		if err != nil {
			return false, err
		}
		cfg.Media.TranscriptionEnabled = b
	case "media.transcription_model":
		cfg.Media.TranscriptionModel = value
	case "media.transcription_base_url":
		cfg.Media.TranscriptionBaseURL = value
	case "media.transcription_language":
		cfg.Media.TranscriptionLanguage = value
	case "model":
		if value == "" {
			return false, fmt.Errorf("model cannot be empty")
		}
		cfg.Model = value
		return true, nil
	default:
		sorted := append([]string(nil), allowedSettings...)
		sort.Strings(sorted)
		return false, fmt.Errorf("setting %q is not allowed; changeable keys: %s", key, strings.Join(sorted, ", "))
	}
	return false, nil
}

// getAgentSettings returns a readable summary of the current effective media and
// model settings, plus the list of keys that can be changed.
func (a *Assistant) getAgentSettings() (string, error) {
	a.configMu.RLock()
	m := a.config.Media.Effective()
	model := a.config.Model
	provider := a.config.API.Provider
	a.configMu.RUnlock()

	visionModel := m.VisionModel
	if visionModel == "" {
		visionModel = "(main model: " + model + ")"
	}

	var b strings.Builder
	b.WriteString("Current settings:\n")
	fmt.Fprintf(&b, "- model: %s (provider: %s)\n", model, provider)
	fmt.Fprintf(&b, "- media.vision_enabled: %t\n", m.VisionEnabled)
	fmt.Fprintf(&b, "- media.vision_model: %s\n", visionModel)
	fmt.Fprintf(&b, "- media.vision_detail: %s\n", m.VisionDetail)
	fmt.Fprintf(&b, "- media.transcription_enabled: %t\n", m.TranscriptionEnabled)
	fmt.Fprintf(&b, "- media.transcription_model: %s\n", m.TranscriptionModel)
	fmt.Fprintf(&b, "- media.transcription_base_url: %s\n", m.TranscriptionBaseURL)
	fmt.Fprintf(&b, "- media.transcription_language: %s\n", m.TranscriptionLanguage)
	b.WriteString("\nChangeable keys: ")
	b.WriteString(strings.Join(allowedSettings, ", "))
	b.WriteString("\nTip: leave media.vision_model empty to use the main model.")
	return b.String(), nil
}

// setAgentSetting validates and applies a single whitelisted setting, persists
// it to config.yaml, and hot-reloads it so it takes effect without a restart.
func (a *Assistant) setAgentSetting(key, value string) (string, error) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	a.configMu.RLock()
	path := a.configPath
	a.configMu.RUnlock()
	if path == "" {
		return "", fmt.Errorf("no config file is in use, cannot persist settings")
	}

	// Load the on-disk config, mutate the single whitelisted field, persist.
	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}

	modelChanged, err := applySettingToConfig(cfg, key, value)
	if err != nil {
		return "", err
	}

	if err := SaveConfigToFile(cfg, path); err != nil {
		return "", fmt.Errorf("persisting config: %w", err)
	}

	// Hot-reload: media changes apply via UpdateMediaConfig (the inbound
	// enrichment path reads the live config); model changes recreate the LLM client.
	a.UpdateMediaConfig(cfg.Media)
	if modelChanged {
		a.UpdateLLMClient(cfg)
	}

	a.logger.Info("settings changed by agent", "key", key, "model_changed", modelChanged)
	return fmt.Sprintf("OK — %s set to %q, persisted and hot-reloaded (no restart needed).", key, value), nil
}

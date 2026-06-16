package copilot

import "testing"

func TestApplySettingToConfig_Whitelist(t *testing.T) {
	// Non-whitelisted keys must be rejected (security boundary).
	for _, key := range []string{
		"security.max_input_length",
		"api.api_key",
		"channels.whatsapp.enabled",
		"vault.something",
		"media.transcription_api_key", // secret — intentionally NOT settable
		"",
	} {
		cfg := &Config{}
		if _, err := applySettingToConfig(cfg, key, "x"); err == nil {
			t.Errorf("key %q should be rejected, but was accepted", key)
		}
	}
}

func TestApplySettingToConfig_MediaKeys(t *testing.T) {
	cfg := &Config{}

	if _, err := applySettingToConfig(cfg, "media.vision_model", "glm-4.6v"); err != nil {
		t.Fatalf("vision_model: %v", err)
	}
	if cfg.Media.VisionModel != "glm-4.6v" {
		t.Errorf("VisionModel = %q, want glm-4.6v", cfg.Media.VisionModel)
	}

	// Empty vision_model is allowed (means "use main model").
	if _, err := applySettingToConfig(cfg, "media.vision_model", ""); err != nil {
		t.Fatalf("empty vision_model should be allowed: %v", err)
	}
	if cfg.Media.VisionModel != "" {
		t.Errorf("VisionModel should be cleared, got %q", cfg.Media.VisionModel)
	}

	if _, err := applySettingToConfig(cfg, "media.vision_enabled", "false"); err != nil {
		t.Fatalf("vision_enabled: %v", err)
	}
	if cfg.Media.VisionEnabled {
		t.Error("VisionEnabled should be false")
	}

	if _, err := applySettingToConfig(cfg, "media.transcription_model", "glm-asr-2512"); err != nil {
		t.Fatalf("transcription_model: %v", err)
	}
	if cfg.Media.TranscriptionModel != "glm-asr-2512" {
		t.Errorf("TranscriptionModel = %q", cfg.Media.TranscriptionModel)
	}

	if _, err := applySettingToConfig(cfg, "media.transcription_base_url", "https://api.z.ai/api/paas/v4"); err != nil {
		t.Fatalf("transcription_base_url: %v", err)
	}
	if cfg.Media.TranscriptionBaseURL != "https://api.z.ai/api/paas/v4" {
		t.Errorf("TranscriptionBaseURL = %q", cfg.Media.TranscriptionBaseURL)
	}
}

func TestApplySettingToConfig_Validation(t *testing.T) {
	cfg := &Config{}

	if _, err := applySettingToConfig(cfg, "media.vision_detail", "ultra"); err == nil {
		t.Error("invalid vision_detail should be rejected")
	}
	if _, err := applySettingToConfig(cfg, "media.vision_detail", "high"); err != nil {
		t.Errorf("valid vision_detail rejected: %v", err)
	}
	if _, err := applySettingToConfig(cfg, "media.vision_enabled", "notabool"); err == nil {
		t.Error("non-boolean for vision_enabled should be rejected")
	}
	if _, err := applySettingToConfig(cfg, "model", ""); err == nil {
		t.Error("empty model should be rejected")
	}
}

func TestApplySettingToConfig_ModelChangedFlag(t *testing.T) {
	cfg := &Config{}

	changed, err := applySettingToConfig(cfg, "model", "glm-5.2")
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	if !changed {
		t.Error("changing model must report modelChanged=true (to trigger LLM client reload)")
	}
	if cfg.Model != "glm-5.2" {
		t.Errorf("Model = %q, want glm-5.2", cfg.Model)
	}

	changed, err = applySettingToConfig(cfg, "media.vision_model", "x")
	if err != nil {
		t.Fatalf("vision_model: %v", err)
	}
	if changed {
		t.Error("a media change must NOT report modelChanged")
	}
}

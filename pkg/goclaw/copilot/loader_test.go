package copilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandEnvVars(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel, so no parallel here.

	t.Run("dollar brace", func(t *testing.T) {
		t.Setenv("TEST_EXPAND_A", "value_a")
		got := expandEnvVars("key: ${TEST_EXPAND_A}")
		if got != "key: value_a" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("dollar bare", func(t *testing.T) {
		t.Setenv("TEST_EXPAND_B", "value_b")
		got := expandEnvVars("key: $TEST_EXPAND_B")
		if got != "key: value_b" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("unset var stays", func(t *testing.T) {
		got := expandEnvVars("key: ${UNSET_VAR_XYZ_12345}")
		if got != "key: ${UNSET_VAR_XYZ_12345}" {
			t.Errorf("expected placeholder to remain, got %q", got)
		}
	})

	t.Run("no vars", func(t *testing.T) {
		got := expandEnvVars("plain text")
		if got != "plain text" {
			t.Errorf("got %q", got)
		}
	})
}

func TestIsEnvReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want bool
	}{
		{"${GOCLAW_API_KEY}", true},
		{"$GOCLAW_API_KEY", true},
		{"sk-1234567890", false},
		{"", false},
		{"hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := IsEnvReference(tt.in); got != tt.want {
				t.Errorf("IsEnvReference(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeSecret(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel.

	t.Run("empty value", func(t *testing.T) {
		got := sanitizeSecret("", "GOCLAW_API_KEY")
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("env reference passthrough", func(t *testing.T) {
		got := sanitizeSecret("${GOCLAW_API_KEY}", "GOCLAW_API_KEY")
		if got != "${GOCLAW_API_KEY}" {
			t.Errorf("expected passthrough, got %q", got)
		}
	})

	t.Run("env match returns reference", func(t *testing.T) {
		t.Setenv("TEST_SANITIZE_KEY", "real-key-value")
		got := sanitizeSecret("real-key-value", "TEST_SANITIZE_KEY")
		if got != "${TEST_SANITIZE_KEY}" {
			t.Errorf("expected env reference, got %q", got)
		}
	})

	t.Run("no match returns as-is", func(t *testing.T) {
		got := sanitizeSecret("some-value", "NONEXISTENT_VAR_XYZ")
		if got != "some-value" {
			t.Errorf("expected original, got %q", got)
		}
	})
}

func TestLooksLikeRealKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want bool
	}{
		{"sk-abcdef1234567890", true},
		{"sk-ant-abcdef1234567890", true},
		{"a-very-long-string-that-is-over-twenty-characters", true},
		{"${GOCLAW_API_KEY}", false},
		{"$VAR", false},
		{"short", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := looksLikeRealKey(tt.in); got != tt.want {
				t.Errorf("looksLikeRealKey(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSaveAndLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.Name = "TestBot"
	cfg.Model = "test-model"

	if err := SaveConfigToFile(cfg, cfgPath); err != nil {
		t.Fatalf("SaveConfigToFile: %v", err)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestSaveConfigToFile_CreatesBackup(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	// Write initial config.
	cfg := DefaultConfig()
	cfg.Name = "v1"
	if err := SaveConfigToFile(cfg, cfgPath); err != nil {
		t.Fatalf("first write: %v", err)
	}

	// Write again — should create .bak.
	cfg.Name = "v2"
	if err := SaveConfigToFile(cfg, cfgPath); err != nil {
		t.Fatalf("second write: %v", err)
	}

	bakPath := cfgPath + ".bak"
	if _, err := os.Stat(bakPath); err != nil {
		t.Fatalf("backup not created: %v", err)
	}

	bakData, _ := os.ReadFile(bakPath)
	if !strings.Contains(string(bakData), "v1") {
		t.Error("backup should contain original config")
	}
}

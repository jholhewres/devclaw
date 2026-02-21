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

	t.Run("default value when unset", func(t *testing.T) {
		got := expandEnvVars("key: ${UNSET_VAR_DEFAULT_123:-fallback}")
		if got != "key: fallback" {
			t.Errorf("expected default value, got %q", got)
		}
	})

	t.Run("default value ignored when set", func(t *testing.T) {
		t.Setenv("TEST_DEFAULT_SET", "actual")
		got := expandEnvVars("key: ${TEST_DEFAULT_SET:-ignored}")
		if got != "key: actual" {
			t.Errorf("expected actual value, got %q", got)
		}
	})

	t.Run("empty default", func(t *testing.T) {
		got := expandEnvVars("key: ${UNSET_EMPTY_DEFAULT:-}")
		if got != "key: " {
			t.Errorf("expected empty value, got %q", got)
		}
	})

	t.Run("default with special chars", func(t *testing.T) {
		got := expandEnvVars("url: ${UNSET_URL:-https://api.example.com/v1}")
		if got != "url: https://api.example.com/v1" {
			t.Errorf("expected URL default, got %q", got)
		}
	})

	t.Run("error marker when unset", func(t *testing.T) {
		got := expandEnvVars("key: ${UNSET_REQUIRED_VAR_123:?API key is required}")
		if !strings.Contains(got, "ERROR:") {
			t.Errorf("expected ERROR marker, got %q", got)
		}
		if !strings.Contains(got, "API key is required") {
			t.Errorf("expected error message, got %q", got)
		}
	})

	t.Run("error marker empty message", func(t *testing.T) {
		got := expandEnvVars("key: ${UNSET_REQUIRED_EMPTY:?}")
		if !strings.Contains(got, "ERROR:") {
			t.Errorf("expected ERROR marker, got %q", got)
		}
		if !strings.Contains(got, "required environment variable not set") {
			t.Errorf("expected default error message, got %q", got)
		}
	})

	t.Run("error ignored when set", func(t *testing.T) {
		t.Setenv("TEST_REQUIRED_SET", "value")
		got := expandEnvVars("key: ${TEST_REQUIRED_SET:?should not appear}")
		if got != "key: value" {
			t.Errorf("expected actual value, got %q", got)
		}
	})

	t.Run("mixed patterns", func(t *testing.T) {
		t.Setenv("MIXED_SET", "yes")
		got := expandEnvVars("a: ${MIXED_SET}, b: ${UNSET_MIXED:-no}, c: ${UNSET_KEEP}")
		expected := "a: yes, b: no, c: ${UNSET_KEEP}"
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})
}

func TestExpandEnvVarsWithValidation(t *testing.T) {
	t.Run("no errors returns result", func(t *testing.T) {
		t.Setenv("TEST_VALIDATE_A", "value")
		got, err := expandEnvVarsWithValidation("key: ${TEST_VALIDATE_A}")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if got != "key: value" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("default value no error", func(t *testing.T) {
		got, err := expandEnvVarsWithValidation("key: ${UNSET_VALIDATE_DEFAULT:-fallback}")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if got != "key: fallback" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("required var missing returns error", func(t *testing.T) {
		_, err := expandEnvVarsWithValidation("key: ${UNSET_VALIDATE_REQUIRED:?missing var}")
		if err == nil {
			t.Error("expected error for missing required var")
		}
		if !strings.Contains(err.Error(), "UNSET_VALIDATE_REQUIRED") {
			t.Errorf("error should mention var name: %v", err)
		}
		if !strings.Contains(err.Error(), "missing var") {
			t.Errorf("error should contain message: %v", err)
		}
	})

	t.Run("required var set succeeds", func(t *testing.T) {
		t.Setenv("TEST_VALIDATE_REQUIRED_SET", "value")
		got, err := expandEnvVarsWithValidation("key: ${TEST_VALIDATE_REQUIRED_SET:?should not error}")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if got != "key: value" {
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
		{"${DEVCLAW_API_KEY}", true},
		{"$DEVCLAW_API_KEY", true},
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
		got := sanitizeSecret("", "DEVCLAW_API_KEY")
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("env reference passthrough", func(t *testing.T) {
		got := sanitizeSecret("${DEVCLAW_API_KEY}", "DEVCLAW_API_KEY")
		if got != "${DEVCLAW_API_KEY}" {
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
		{"${DEVCLAW_API_KEY}", false},
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

func TestResolvePathFromConfig(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		configDir string
		want      string
	}{
		{
			name:      "relative path",
			path:      "./skills",
			configDir: "/home/user/devclaw",
			want:      "/home/user/devclaw/skills",
		},
		{
			name:      "relative path without dot",
			path:      "skills",
			configDir: "/home/user/devclaw",
			want:      "/home/user/devclaw/skills",
		},
		{
			name:      "absolute path unchanged",
			path:      "/absolute/skills",
			configDir: "/home/user/devclaw",
			want:      "/absolute/skills",
		},
		{
			name:      "empty path unchanged",
			path:      "",
			configDir: "/home/user/devclaw",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePathFromConfig(tt.path, tt.configDir)
			if got != tt.want {
				t.Errorf("resolvePathFromConfig(%q, %q) = %q, want %q", tt.path, tt.configDir, got, tt.want)
			}
		})
	}
}

func TestLoadConfigFromFile_ResolvesRelativePaths(t *testing.T) {
	// Create a temp directory structure.
	dir := t.TempDir()
	configDir := filepath.Join(dir, "devclaw")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	// Create a skills directory to verify path resolution.
	skillsDir := filepath.Join(configDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("creating skills dir: %v", err)
	}

	// Create a subdirectory for a skill.
	if err := os.Mkdir(filepath.Join(skillsDir, "test-skill"), 0755); err != nil {
		t.Fatalf("creating test skill dir: %v", err)
	}

	// Write a config with relative paths.
	cfgPath := filepath.Join(configDir, "config.yaml")
	cfgContent := `
name: "TestBot"
model: "test-model"
skills:
  clawdhub_dirs: ["./skills", "./plugins"]
memory:
  path: "./data/memory.db"
scheduler:
  storage: "./data/scheduler.db"
plugins:
  dir: "./plugins"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// Load config from a DIFFERENT working directory.
	// This simulates running devclaw from / while config is in /home/user/devclaw.
	originalWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("changing to temp dir: %v", err)
	}
	defer os.Chdir(originalWd)

	cfg, err := LoadConfigFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFromFile: %v", err)
	}

	// Verify paths are resolved relative to config file location.
	expectedSkillsDir := filepath.Join(configDir, "skills")
	if len(cfg.Skills.ClawdHubDirs) < 1 || cfg.Skills.ClawdHubDirs[0] != expectedSkillsDir {
		t.Errorf("Skills.ClawdHubDirs[0] = %v, want %v", cfg.Skills.ClawdHubDirs, expectedSkillsDir)
	}

	expectedMemoryPath := filepath.Join(configDir, "data", "memory.db")
	if cfg.Memory.Path != expectedMemoryPath {
		t.Errorf("Memory.Path = %v, want %v", cfg.Memory.Path, expectedMemoryPath)
	}

	expectedSchedulerPath := filepath.Join(configDir, "data", "scheduler.db")
	if cfg.Scheduler.Storage != expectedSchedulerPath {
		t.Errorf("Scheduler.Storage = %v, want %v", cfg.Scheduler.Storage, expectedSchedulerPath)
	}
}

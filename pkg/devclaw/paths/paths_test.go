package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStateDir(t *testing.T) {
	t.Run("respects environment variable", func(t *testing.T) {
		customDir := "/tmp/custom-devclaw-state"
		os.Setenv(StateDirEnv, customDir)
		defer os.Unsetenv(StateDirEnv)

		if got := ResolveStateDir(); got != customDir {
			t.Errorf("ResolveStateDir() = %q, want %q", got, customDir)
		}
	})

	t.Run("fallback to current directory when no home", func(t *testing.T) {
		// This test verifies the fallback behavior
		os.Unsetenv(StateDirEnv)
		got := ResolveStateDir()
		if got == "" {
			t.Error("ResolveStateDir() should not return empty string")
		}
	})
}

func TestResolveDataDir(t *testing.T) {
	os.Setenv(StateDirEnv, "/tmp/test-devclaw")
	defer os.Unsetenv(StateDirEnv)

	got := ResolveDataDir()
	want := "/tmp/test-devclaw/data"
	if got != want {
		t.Errorf("ResolveDataDir() = %q, want %q", got, want)
	}
}

func TestResolveMediaDir(t *testing.T) {
	os.Setenv(StateDirEnv, "/tmp/test-devclaw")
	defer os.Unsetenv(StateDirEnv)

	got := ResolveMediaDir()
	want := "/tmp/test-devclaw/data/media"
	if got != want {
		t.Errorf("ResolveMediaDir() = %q, want %q", got, want)
	}
}

func TestResolveMediaPath(t *testing.T) {
	os.Setenv(StateDirEnv, "/tmp/test-devclaw")
	defer os.Unsetenv(StateDirEnv)

	got := ResolveMediaPath("whatsapp", "session-123")
	want := "/tmp/test-devclaw/data/media/whatsapp/session-123"
	if got != want {
		t.Errorf("ResolveMediaPath() = %q, want %q", got, want)
	}
}

func TestResolveConfigPath(t *testing.T) {
	t.Run("respects environment variable", func(t *testing.T) {
		customPath := "/tmp/custom-config.yaml"
		os.Setenv(ConfigPathEnv, customPath)
		defer os.Unsetenv(ConfigPathEnv)

		if got := ResolveConfigPath(); got != customPath {
			t.Errorf("ResolveConfigPath() = %q, want %q", got, customPath)
		}
	})

	t.Run("default path when no env", func(t *testing.T) {
		os.Unsetenv(ConfigPathEnv)
		os.Unsetenv(StateDirEnv)

		got := ResolveConfigPath()
		if got == "" {
			t.Error("ResolveConfigPath() should not return empty string")
		}
	})
}

func TestResolveVaultPath(t *testing.T) {
	os.Setenv(StateDirEnv, "/tmp/test-devclaw")
	defer os.Unsetenv(StateDirEnv)

	got := ResolveVaultPath()
	want := "/tmp/test-devclaw/.devclaw.vault"
	if got != want {
		t.Errorf("ResolveVaultPath() = %q, want %q", got, want)
	}
}

func TestResolveDatabasePath(t *testing.T) {
	os.Setenv(StateDirEnv, "/tmp/test-devclaw")
	defer os.Unsetenv(StateDirEnv)

	got := ResolveDatabasePath("memory.db")
	want := "/tmp/test-devclaw/data/memory.db"
	if got != want {
		t.Errorf("ResolveDatabasePath() = %q, want %q", got, want)
	}
}

func TestEnsureStateDirs(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()
	os.Setenv(StateDirEnv, tmpDir)
	defer os.Unsetenv(StateDirEnv)

	if err := EnsureStateDirs(); err != nil {
		t.Fatalf("EnsureStateDirs() error = %v", err)
	}

	// Verify directories were created
	expectedDirs := []string{
		ResolveDataDir(),
		ResolveMediaDir(),
		filepath.Join(ResolveMediaDir(), "whatsapp"),
		filepath.Join(ResolveMediaDir(), "telegram"),
		ResolveSessionsDir(),
		ResolveSkillsDir(),
		ResolveWorkspacesDir(),
		ResolvePluginsDir(),
	}

	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory %q was not created", dir)
		}
	}
}

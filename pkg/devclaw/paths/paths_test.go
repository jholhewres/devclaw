package paths

import (
	"os"
	"path/filepath"
	"strings"
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

func TestResolveMediaPathTraversalProtection(t *testing.T) {
	os.Setenv(StateDirEnv, "/tmp/test-devclaw")
	defer os.Unsetenv(StateDirEnv)

	tests := []struct {
		name      string
		channel   string
		sessionID string
		want      string
	}{
		{
			name:      "path traversal with ../",
			channel:   "whatsapp",
			sessionID: "../etc/passwd",
			want:      "/tmp/test-devclaw/data/media/whatsapp/__etc_passwd",
		},
		{
			name:      "path traversal with ..",
			channel:   "../../../etc",
			sessionID: "session-123",
			want:      "/tmp/test-devclaw/data/media/______etc/session-123",
		},
		{
			name:      "backslash path separator",
			channel:   "whatsapp",
			sessionID: "..\\..\\windows",
			want:      "/tmp/test-devclaw/data/media/whatsapp/____windows",
		},
		{
			name:      "mixed path traversal",
			channel:   "a/../b",
			sessionID: "c/../../d",
			want:      "/tmp/test-devclaw/data/media/a___b/c_____d",
		},
		{
			name:      "normal paths unchanged",
			channel:   "whatsapp",
			sessionID: "session-123",
			want:      "/tmp/test-devclaw/data/media/whatsapp/session-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveMediaPath(tt.channel, tt.sessionID)
			if got != tt.want {
				t.Errorf("ResolveMediaPath(%q, %q) = %q, want %q",
					tt.channel, tt.sessionID, got, tt.want)
			}

			// Additional safety check: result should always be under media dir
			mediaDir := ResolveMediaDir()
			relPath, err := filepath.Rel(mediaDir, got)
			if err != nil {
				t.Errorf("Failed to get relative path: %v", err)
			}
			if relPath == ".." || len(relPath) >= 3 && relPath[:3] == "../" {
				t.Errorf("Path escapes media directory: %s", got)
			}
		})
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
		ResolvePluginsDir(),
	}

	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory %q was not created", dir)
		}
	}
}

func TestResolveWorkspaceDir(t *testing.T) {
	os.Setenv(StateDirEnv, "/tmp/test-devclaw")
	defer os.Unsetenv(StateDirEnv)

	tests := []struct {
		name string
		wsID string
		want string
	}{
		{"empty ID returns main workspace", "", "/tmp/test-devclaw/workspace"},
		{"main ID returns main workspace", "main", "/tmp/test-devclaw/workspace"},
		{"named agent returns flat dir", "support", "/tmp/test-devclaw/workspace-support"},
		{"another named agent", "sales", "/tmp/test-devclaw/workspace-sales"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveWorkspaceDir(tt.wsID)
			if got != tt.want {
				t.Errorf("ResolveWorkspaceDir(%q) = %q, want %q", tt.wsID, got, tt.want)
			}
		})
	}
}

func TestResolveWorkspaceDir_PathTraversal(t *testing.T) {
	os.Setenv(StateDirEnv, "/tmp/test-devclaw")
	defer os.Unsetenv(StateDirEnv)

	stateDir := ResolveStateDir()

	tests := []struct {
		name string
		wsID string
	}{
		{"dot-dot slash", "../../../etc"},
		{"dot-dot only", ".."},
		{"slash in ID", "a/b/c"},
		{"backslash in ID", "a\\b\\c"},
		{"dot-dot embedded", "foo/../bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveWorkspaceDir(tt.wsID)
			// Result must stay under the state directory
			relPath, err := filepath.Rel(stateDir, got)
			if err != nil {
				t.Fatalf("filepath.Rel failed: %v", err)
			}
			if relPath == ".." || (len(relPath) >= 3 && relPath[:3] == "../") {
				t.Errorf("ResolveWorkspaceDir(%q) = %q escapes state dir %q", tt.wsID, got, stateDir)
			}
			// Must not contain path separators in the workspace component
			base := filepath.Base(got)
			if strings.ContainsAny(base, "/\\") {
				t.Errorf("ResolveWorkspaceDir(%q) base %q contains path separator", tt.wsID, base)
			}
		})
	}
}

func TestEnsureStateDirs_MigratesOldWorkspaces(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv(StateDirEnv, tmpDir)
	defer os.Unsetenv(StateDirEnv)

	// Create old layout: workspaces/templates/ and workspaces/support/
	oldTemplates := filepath.Join(tmpDir, "workspaces", "templates")
	oldAgent := filepath.Join(tmpDir, "workspaces", "support")
	os.MkdirAll(oldTemplates, 0755)
	os.MkdirAll(oldAgent, 0755)
	os.WriteFile(filepath.Join(oldTemplates, "SOUL.md"), []byte("template soul"), 0600)
	os.WriteFile(filepath.Join(oldAgent, "SOUL.md"), []byte("support soul"), 0600)
	os.WriteFile(filepath.Join(oldAgent, "IDENTITY.md"), []byte("support identity"), 0600)

	if err := EnsureStateDirs(); err != nil {
		t.Fatalf("EnsureStateDirs() error = %v", err)
	}

	// Templates should be in configs/templates/
	data, err := os.ReadFile(filepath.Join(tmpDir, "configs", "templates", "SOUL.md"))
	if err != nil {
		t.Fatalf("template SOUL.md not migrated: %v", err)
	}
	if string(data) != "template soul" {
		t.Errorf("template content = %q, want %q", data, "template soul")
	}

	// Agent workspace should be at workspace-support/
	data, err = os.ReadFile(filepath.Join(tmpDir, "workspace-support", "SOUL.md"))
	if err != nil {
		t.Fatalf("agent SOUL.md not migrated: %v", err)
	}
	if string(data) != "support soul" {
		t.Errorf("agent soul content = %q, want %q", data, "support soul")
	}

	data, err = os.ReadFile(filepath.Join(tmpDir, "workspace-support", "IDENTITY.md"))
	if err != nil {
		t.Fatalf("agent IDENTITY.md not migrated: %v", err)
	}
	if string(data) != "support identity" {
		t.Errorf("agent identity content = %q, want %q", data, "support identity")
	}

	// Old workspaces/ should be removed
	if _, err := os.Stat(filepath.Join(tmpDir, "workspaces")); !os.IsNotExist(err) {
		t.Error("old workspaces/ directory should have been removed")
	}
}

func TestResolveWorkspaceTemplatesDir(t *testing.T) {
	os.Setenv(StateDirEnv, "/tmp/test-devclaw")
	defer os.Unsetenv(StateDirEnv)

	got := ResolveWorkspaceTemplatesDir()
	want := "/tmp/test-devclaw/configs/templates"
	if got != want {
		t.Errorf("ResolveWorkspaceTemplatesDir() = %q, want %q", got, want)
	}
}

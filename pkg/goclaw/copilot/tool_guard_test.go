package copilot

import (
	"log/slog"
	"testing"
)

func TestExpandToolGroups(t *testing.T) {
	t.Parallel()

	t.Run("expands known group", func(t *testing.T) {
		t.Parallel()
		result := ExpandToolGroups([]string{"group:memory"})
		expected := ToolGroups["group:memory"]
		if len(result) != len(expected) {
			t.Errorf("expected %d tools, got %d", len(expected), len(result))
		}
		for i, tool := range expected {
			if result[i] != tool {
				t.Errorf("result[%d] = %q, want %q", i, result[i], tool)
			}
		}
	})

	t.Run("non-group passthrough", func(t *testing.T) {
		t.Parallel()
		result := ExpandToolGroups([]string{"bash", "ssh"})
		if len(result) != 2 || result[0] != "bash" || result[1] != "ssh" {
			t.Errorf("expected [bash, ssh], got %v", result)
		}
	})

	t.Run("mixed", func(t *testing.T) {
		t.Parallel()
		result := ExpandToolGroups([]string{"bash", "group:vault"})
		if len(result) != 1+len(ToolGroups["group:vault"]) {
			t.Errorf("expected %d tools, got %d", 1+len(ToolGroups["group:vault"]), len(result))
		}
		if result[0] != "bash" {
			t.Errorf("first should be 'bash', got %q", result[0])
		}
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		result := ExpandToolGroups(nil)
		if len(result) != 0 {
			t.Errorf("expected empty, got %v", result)
		}
	})

	t.Run("all groups expand", func(t *testing.T) {
		t.Parallel()
		for group, tools := range ToolGroups {
			result := ExpandToolGroups([]string{group})
			if len(result) != len(tools) {
				t.Errorf("group %q: expected %d tools, got %d", group, len(tools), len(result))
			}
		}
	})
}

func newTestGuard(cfg ToolGuardConfig) *ToolGuard {
	return NewToolGuard(cfg, slog.Default())
}

func TestToolGuard_DisabledAllowsEverything(t *testing.T) {
	t.Parallel()
	g := newTestGuard(ToolGuardConfig{Enabled: false})
	r := g.Check("bash", AccessUser, nil)
	if !r.Allowed {
		t.Error("disabled guard should allow everything")
	}
}

func TestToolGuard_AutoApproveBypass(t *testing.T) {
	t.Parallel()
	cfg := DefaultToolGuardConfig()
	cfg.AutoApprove = []string{"web_search"}
	g := newTestGuard(cfg)

	r := g.Check("web_search", AccessUser, nil)
	if !r.Allowed {
		t.Error("auto-approved tool should be allowed")
	}
}

func TestToolGuard_OwnerCanUseOwnerTool(t *testing.T) {
	t.Parallel()
	g := newTestGuard(DefaultToolGuardConfig())
	r := g.Check("bash", AccessOwner, nil)
	if !r.Allowed {
		t.Error("owner should be able to use owner-level tool")
	}
}

func TestToolGuard_UserCannotUseOwnerTool(t *testing.T) {
	t.Parallel()
	g := newTestGuard(DefaultToolGuardConfig())
	r := g.Check("bash", AccessUser, nil)
	if r.Allowed {
		t.Error("user should NOT be able to use owner-level tool")
	}
}

func TestToolGuard_AdminCanUseAdminTool(t *testing.T) {
	t.Parallel()
	g := newTestGuard(DefaultToolGuardConfig())
	r := g.Check("write_file", AccessAdmin, nil)
	if !r.Allowed {
		t.Error("admin should be able to use admin-level tool")
	}
}

func TestToolGuard_UserCanUseUserTool(t *testing.T) {
	t.Parallel()
	g := newTestGuard(DefaultToolGuardConfig())
	r := g.Check("read_file", AccessUser, nil)
	if !r.Allowed {
		t.Error("user should be able to use user-level tool")
	}
}

func TestToolGuard_UserCannotUseAdminTool(t *testing.T) {
	t.Parallel()
	g := newTestGuard(DefaultToolGuardConfig())
	r := g.Check("write_file", AccessUser, nil)
	if r.Allowed {
		t.Error("user should NOT be able to use admin-level tool")
	}
}

func TestToolGuard_RequireConfirmation(t *testing.T) {
	t.Parallel()
	cfg := DefaultToolGuardConfig()
	cfg.RequireConfirmation = []string{"bash"}
	g := newTestGuard(cfg)

	// Admin should get confirmation required.
	r := g.Check("bash", AccessAdmin, nil)
	// bash is owner-level by default, so admin is denied.
	if r.Allowed && !r.RequiresConfirmation {
		t.Error("expected confirmation requirement or denial")
	}
}

func TestToolGuard_OwnerSkipsConfirmation(t *testing.T) {
	t.Parallel()
	cfg := DefaultToolGuardConfig()
	cfg.RequireConfirmation = []string{"bash"}
	g := newTestGuard(cfg)

	r := g.Check("bash", AccessOwner, nil)
	if !r.Allowed {
		t.Error("owner should be allowed")
	}
	if r.RequiresConfirmation {
		t.Error("owner should skip confirmation")
	}
}

func TestToolGuard_UnknownToolUserLevel(t *testing.T) {
	t.Parallel()
	g := newTestGuard(DefaultToolGuardConfig())
	// A tool not in ToolPermissions should default to user-level.
	r := g.Check("custom_skill_tool", AccessUser, nil)
	if !r.Allowed {
		t.Error("unknown tool should default to user-level and be allowed for users")
	}
}

package copilot

import (
	"log/slog"
	"os"
	"testing"
)

func TestNewExecAnalyzer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := DefaultExecAnalysisConfig()
	a := NewExecAnalyzer(cfg, logger)

	if a == nil {
		t.Fatal("expected analyzer, got nil")
	}

	if !a.config.Enabled {
		t.Error("expected analyzer to be enabled")
	}
}

func TestExecAnalyzer_Analyze_Safe(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	a := NewExecAnalyzer(DefaultExecAnalysisConfig(), logger)

	tests := []struct {
		command string
		wantRisk RiskLevel
		wantAction RiskAction
	}{
		{"ls -la", RiskSafe, ActionAllow},
		{"cat file.txt", RiskSafe, ActionAllow},
		{"echo hello", RiskSafe, ActionAllow},
		{"grep pattern file", RiskSafe, ActionAllow},
		{"pwd", RiskSafe, ActionAllow},
		{"whoami", RiskSafe, ActionAllow},
		{"date", RiskSafe, ActionAllow},
		{"head -n 10 file", RiskSafe, ActionAllow},
		{"tail -f log", RiskSafe, ActionAllow},
		{"wc -l file", RiskSafe, ActionAllow},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := a.Analyze(tt.command)
			if result.Risk != tt.wantRisk {
				t.Errorf("Analyze(%q) risk = %q, want %q", tt.command, result.Risk, tt.wantRisk)
			}
			if result.Action != tt.wantAction {
				t.Errorf("Analyze(%q) action = %q, want %q", tt.command, result.Action, tt.wantAction)
			}
		})
	}
}

func TestExecAnalyzer_Analyze_Moderate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	a := NewExecAnalyzer(DefaultExecAnalysisConfig(), logger)

	tests := []struct {
		command string
		wantRisk RiskLevel
		wantAction RiskAction
	}{
		{"npm install", RiskModerate, ActionAllowLog},
		{"yarn build", RiskModerate, ActionAllowLog},
		{"git pull", RiskModerate, ActionAllowLog},
		{"git status", RiskModerate, ActionAllowLog},
		{"docker ps", RiskModerate, ActionAllowLog},
		{"go build", RiskModerate, ActionAllowLog},
		{"python script.py", RiskModerate, ActionAllowLog},
		{"make build", RiskModerate, ActionAllowLog},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := a.Analyze(tt.command)
			if result.Risk != tt.wantRisk {
				t.Errorf("Analyze(%q) risk = %q, want %q", tt.command, result.Risk, tt.wantRisk)
			}
			if result.Action != tt.wantAction {
				t.Errorf("Analyze(%q) action = %q, want %q", tt.command, result.Action, tt.wantAction)
			}
		})
	}
}

func TestExecAnalyzer_Analyze_Dangerous(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	a := NewExecAnalyzer(DefaultExecAnalysisConfig(), logger)

	tests := []struct {
		command string
		wantRisk RiskLevel
		wantAction RiskAction
	}{
		{"rm file.txt", RiskDangerous, ActionRequireApproval},
		{"rm -rf /tmp/test", RiskDangerous, ActionRequireApproval},
		{"sudo apt update", RiskDangerous, ActionRequireApproval},
		{"chmod 777 file", RiskDangerous, ActionRequireApproval},
		{"chown root file", RiskDangerous, ActionRequireApproval},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := a.Analyze(tt.command)
			if result.Risk != tt.wantRisk {
				t.Errorf("Analyze(%q) risk = %q, want %q", tt.command, result.Risk, tt.wantRisk)
			}
			if result.Action != tt.wantAction {
				t.Errorf("Analyze(%q) action = %q, want %q", tt.command, result.Action, tt.wantAction)
			}
		})
	}
}

func TestExecAnalyzer_Analyze_Blocked(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	a := NewExecAnalyzer(DefaultExecAnalysisConfig(), logger)

	tests := []struct {
		command string
		wantRisk RiskLevel
		wantAction RiskAction
	}{
		{"mkfs.ext4 /dev/sda1", RiskBlocked, ActionDeny},
		{"dd if=/dev/zero of=/dev/sda", RiskBlocked, ActionDeny},
		{"shutdown now", RiskBlocked, ActionDeny},
		{"reboot", RiskBlocked, ActionDeny},
		{"halt", RiskBlocked, ActionDeny},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := a.Analyze(tt.command)
			if result.Risk != tt.wantRisk {
				t.Errorf("Analyze(%q) risk = %q, want %q", tt.command, result.Risk, tt.wantRisk)
			}
			if result.Action != tt.wantAction {
				t.Errorf("Analyze(%q) action = %q, want %q", tt.command, result.Action, tt.wantAction)
			}
		})
	}
}

func TestExecAnalyzer_Analyze_Suspicious(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	a := NewExecAnalyzer(DefaultExecAnalysisConfig(), logger)

	tests := []struct {
		command       string
		wantSuspicious bool
	}{
		{"echo $(whoami)", true},                    // Command substitution
		{"echo `id`", true},                          // Backtick execution
		{"cat file | bash", true},                    // Pipe to shell
		{"ls && rm file", true},                      // Chained dangerous
		{"ls; sudo rm file", true},                   // Semicolon dangerous
		{"echo data > /etc/hosts", true},             // Writing to /etc
		{"cat /dev/urandom > /dev/sda", true},        // Writing to /dev
		{"ls -la", false},                            // Normal command
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := a.Analyze(tt.command)
			if result.IsSuspicious != tt.wantSuspicious {
				t.Errorf("Analyze(%q) IsSuspicious = %v, want %v", tt.command, result.IsSuspicious, tt.wantSuspicious)
			}
		})
	}
}

func TestExecAnalyzer_AnalyzeForRole(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	a := NewExecAnalyzer(DefaultExecAnalysisConfig(), logger)

	tests := []struct {
		command    string
		role       string
		wantAction RiskAction
	}{
		// Owner can execute dangerous commands (but still needs approval by default)
		{"rm file.txt", "owner", ActionRequireApproval},
		{"npm install", "owner", ActionAllowLog},

		// Admin can execute moderate but dangerous needs approval
		{"npm install", "admin", ActionAllowLog},

		// User can only execute safe commands
		{"ls -la", "user", ActionAllow},

		// Blocked is always denied
		{"mkfs.ext4 /dev/sda", "owner", ActionDeny},
	}

	for _, tt := range tests {
		t.Run(tt.command+"_"+tt.role, func(t *testing.T) {
			result := a.AnalyzeForRole(tt.command, tt.role)
			if result.Action != tt.wantAction {
				t.Errorf("AnalyzeForRole(%q, %q) action = %q, want %q", tt.command, tt.role, result.Action, tt.wantAction)
			}
		})
	}
}

func TestExecAnalyzer_Disabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultExecAnalysisConfig()
	cfg.Enabled = false
	a := NewExecAnalyzer(cfg, logger)

	// Even dangerous commands should be allowed when disabled.
	result := a.Analyze("rm -rf /")
	if result.Action != ActionAllow {
		t.Errorf("expected Allow when disabled, got %q", result.Action)
	}
	if result.Risk != RiskSafe {
		t.Errorf("expected Safe risk when disabled, got %q", result.Risk)
	}
}

func TestExecAnalyzer_SafeBins(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultExecAnalysisConfig()
	cfg.SafeBins = []string{"/usr/bin/ls", "/usr/bin/cat"}
	a := NewExecAnalyzer(cfg, logger)

	tests := []struct {
		command   string
		wantRisk  RiskLevel
	}{
		{"/usr/bin/ls -la", RiskSafe},
		{"/usr/bin/cat file", RiskSafe},
		{"/usr/local/bin/something", RiskModerate}, // Not in safe bins
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := a.Analyze(tt.command)
			if result.Risk != tt.wantRisk {
				t.Errorf("Analyze(%q) risk = %q, want %q", tt.command, result.Risk, tt.wantRisk)
			}
		})
	}
}

func TestPatternToRegex(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		match   bool
	}{
		{"ls*", "ls", true},
		{"ls*", "ls -la", true},
		{"ls*", "lsxyz", true},
		{"ls*", "cat", false},
		{"cat *", "cat file.txt", true},
		{"cat *", "cat", false},
		{"npm *", "npm install", true},
		{"npm *", "npm", false},
		{"rm *", "rm -rf /", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.input, func(t *testing.T) {
			re, err := patternToRegex(tt.pattern)
			if err != nil {
				t.Fatalf("patternToRegex(%q) error: %v", tt.pattern, err)
			}
			if re.MatchString(tt.input) != tt.match {
				t.Errorf("pattern %q matching %q = %v, want %v", tt.pattern, tt.input, !tt.match, tt.match)
			}
		})
	}
}

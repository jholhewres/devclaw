// Package skills – coding_claude_code.go integrates with the Claude Code CLI
// to provide full-stack coding capabilities: code editing, review, commit, PR,
// deployment, testing, refactoring, and any development task.
//
// Instead of implementing many granular tools (git_status, code_read, etc.),
// this skill delegates everything to Claude Code, which has its own rich set
// of tools (Bash, Read, Edit, Grep, Glob, Write, etc.).
//
// The skill uses --output-format stream-json for real-time progress feedback:
// as Claude Code works, it sends intermediate updates to the user so they
// know what's happening (reading files, running commands, etc.).
//
// Requirements:
//   - Claude Code CLI installed: npm install -g @anthropic-ai/claude-code
//   - Authenticated: claude setup-token or claude login
//   - The user must enable "claude-code" in skills.builtin config.
package skills

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// claudeCodeResult represents the final JSON result from Claude Code CLI.
type claudeCodeResult struct {
	Type      string  `json:"type"`
	Subtype   string  `json:"subtype"`
	Result    string  `json:"result"`
	IsError   bool    `json:"is_error"`
	SessionID string  `json:"session_id"`
	NumTurns  int     `json:"num_turns"`
	TotalCost float64 `json:"total_cost_usd"`
	Duration  int     `json:"duration_ms"`
	Errors    []any   `json:"errors"`
}

// claudeStreamEvent represents a single line from --output-format stream-json.
type claudeStreamEvent struct {
	Type      string  `json:"type"`
	Subtype   string  `json:"subtype"`
	SessionID string  `json:"session_id"`
	Result    string  `json:"result"`
	IsError   bool    `json:"is_error"`
	NumTurns  int     `json:"num_turns"`
	TotalCost float64 `json:"total_cost_usd"`
	Duration  int     `json:"duration_ms"`

	// For tool_use events.
	ToolName string `json:"tool_name"`

	// For text/content events.
	Content string `json:"content"`

	// For tool input.
	Input json.RawMessage `json:"input"`
}

// claudeCodeSkill implements the claude-code skill.
type claudeCodeSkill struct {
	provider ProjectProvider

	// sessions maps GoClaw session key → last Claude Code session ID,
	// allowing multi-step coding tasks to be continued.
	sessions   map[string]string
	sessionsMu sync.RWMutex

	// Configurable defaults (can be overridden per call).
	defaultModel    string
	defaultBudget   float64
	skipPermissions bool
	timeout         time.Duration
}

// NewClaudeCodeSkill creates the claude-code skill.
// provider may be nil if project management is not configured.
func NewClaudeCodeSkill(provider ProjectProvider) Skill {
	// Model: empty means use Claude Code's own default (from its config).
	model := os.Getenv("GOCLAW_CLAUDE_CODE_MODEL")

	// Budget: 0 means no limit (same as interactive Claude Code).
	// Only set a limit if explicitly configured via env var.
	var budget float64
	if budgetStr := os.Getenv("GOCLAW_CLAUDE_CODE_BUDGET"); budgetStr != "" {
		if v, err := parseFloat(budgetStr); err == nil && v > 0 {
			budget = v
		}
	}

	timeoutMin := 15
	if v := os.Getenv("GOCLAW_CLAUDE_CODE_TIMEOUT_MIN"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			timeoutMin = n
		}
	}

	return &claudeCodeSkill{
		provider:        provider,
		sessions:        make(map[string]string),
		defaultModel:    model,
		defaultBudget:   budget,
		skipPermissions: true,
		timeout:         time.Duration(timeoutMin) * time.Minute,
	}
}

func (s *claudeCodeSkill) Metadata() Metadata {
	return Metadata{
		Name:        "claude-code",
		Version:     "1.0.0",
		Author:      "goclaw",
		Description: "Full-stack coding assistant powered by Claude Code CLI. Handles code editing, review, commit, PR, deploy, test, refactor — any development task.",
		Category:    "development",
		Tags: []string{
			"code", "git", "commit", "review", "deploy", "pr", "refactor",
			"programming", "claude-code", "backend", "frontend", "devops",
		},
	}
}

func (s *claudeCodeSkill) Tools() []Tool {
	return []Tool{
		{
			Name: "execute",
			Description: `Execute any coding task using Claude Code. Claude Code has full access to:
- Read, edit, create, search files (Read, Edit, Write, Grep, Glob)
- Run shell commands (Bash: git, npm, docker, make, etc.)
- Create commits, branches, PRs
- Run tests, lint, build
- Deploy, configure servers
- Multi-file refactoring, code review
Send clear, detailed instructions. The task runs in the active project directory.`,
			Parameters: []ToolParameter{
				{Name: "prompt", Type: "string", Description: "The coding task or instruction. Be specific and detailed.", Required: true},
				{Name: "project_id", Type: "string", Description: "Project ID to work on. Empty = active project."},
				{Name: "session_key", Type: "string", Description: "GoClaw session key (auto-provided by system)."},
				{Name: "continue_session", Type: "boolean", Description: "Continue the previous Claude Code session for multi-step tasks. Default: false."},
				{Name: "model", Type: "string", Description: "Claude model alias: 'sonnet', 'opus', 'haiku'. Empty = Claude Code's own default."},
				{Name: "max_budget", Type: "number", Description: "Max budget in USD. 0 or empty = no limit (normal Claude Code behavior)."},
				{Name: "allowed_tools", Type: "string", Description: "Restrict tools (e.g. 'Read,Grep,Glob' for read-only). Empty = all tools."},
				{Name: "add_dirs", Type: "string", Description: "Comma-separated additional directories Claude Code can access."},
				{Name: "permission_mode", Type: "string", Description: "Permission mode: 'default', 'plan' (read-only analysis), 'bypassPermissions'. Default: bypassPermissions."},
			},
			Handler: s.handleExecute,
		},
		{
			Name:        "check",
			Description: "Check if Claude Code CLI is installed, authenticated, and ready. Reports version, auth status, and available models.",
			Parameters:  []ToolParameter{},
			Handler:     s.handleCheck,
		},
	}
}

func (s *claudeCodeSkill) SystemPrompt() string {
	return `You have the claude-code skill which integrates with Claude Code CLI for advanced software development.

WHEN TO USE claude-code_execute:
- Code editing, creation, refactoring (any language/framework)
- Git operations: commit, branch, merge, rebase, PR
- Code review and analysis
- Running tests, lint, build
- Searching codebase (grep, find patterns)
- DevOps: Docker, deploy, server config
- Multi-file changes, large refactors

BEST PRACTICES:
1. Always activate a project first (project-manager_activate) so Claude Code runs in the right directory
2. For multi-step tasks, use continue_session=true to keep context between calls
3. Be specific in your prompts — Claude Code is powerful but needs clear instructions
4. For read-only analysis, set permission_mode="plan"
5. Claude Code has its own tools (Bash, Read, Edit, Grep, Glob, Write, etc.)
6. It runs with no budget limit by default, just like normal interactive Claude Code

DO NOT use this for non-coding tasks. For general questions, web search, etc. use the appropriate other skills.`
}

func (s *claudeCodeSkill) Triggers() []string {
	return []string{
		"code", "git", "commit", "push", "pull request", "PR", "branch",
		"diff", "merge", "deploy", "review", "refactor", "edit code",
		"create file", "test", "lint", "build", "docker", "server",
		"programming", "bug fix", "feature", "backend", "frontend",
	}
}

func (s *claudeCodeSkill) Init(_ context.Context, cfg map[string]any) error {
	if model, ok := cfg["claude_code_model"].(string); ok && model != "" {
		s.defaultModel = model
	}
	if budget, ok := cfg["claude_code_budget"].(float64); ok && budget > 0 {
		s.defaultBudget = budget
	}
	if skip, ok := cfg["claude_code_skip_permissions"].(bool); ok {
		s.skipPermissions = skip
	}
	return nil
}

func (s *claudeCodeSkill) Execute(ctx context.Context, input string) (string, error) {
	result, err := s.handleExecute(ctx, map[string]any{"prompt": input})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", result), nil
}

func (s *claudeCodeSkill) Shutdown() error { return nil }

// ── Handlers ──

func (s *claudeCodeSkill) handleExecute(ctx context.Context, args map[string]any) (any, error) {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Check if claude is available.
	if _, err := exec.LookPath("claude"); err != nil {
		return nil, fmt.Errorf("Claude Code CLI not found. Install: npm install -g @anthropic-ai/claude-code")
	}

	// Resolve working directory from project.
	var workDir string
	var projectCtx string
	if s.provider != nil {
		p := ccResolveProject(s.provider, args)
		if p != nil {
			workDir = p.RootPath
			projectCtx = buildProjectContext(p)
		}
	}

	// Build CLI arguments — use stream-json for real-time progress feedback.
	cliArgs := []string{"-p", "--output-format", "stream-json"}

	// Permission mode.
	permMode, _ := args["permission_mode"].(string)
	if permMode == "" && s.skipPermissions {
		permMode = "bypassPermissions"
	}
	if permMode != "" {
		cliArgs = append(cliArgs, "--permission-mode", permMode)
	}

	// Model.
	model, _ := args["model"].(string)
	if model == "" {
		model = s.defaultModel
	}
	if model != "" {
		cliArgs = append(cliArgs, "--model", model)
	}

	// Budget.
	budget := s.defaultBudget
	if b, ok := args["max_budget"].(float64); ok && b > 0 {
		budget = b
	}
	if budget > 0 {
		cliArgs = append(cliArgs, "--max-budget-usd", fmt.Sprintf("%.2f", budget))
	}

	// Session continuation.
	if cont, _ := args["continue_session"].(bool); cont {
		sessionKey, _ := args["session_key"].(string)
		s.sessionsMu.RLock()
		prevSessionID, hasPrev := s.sessions[sessionKey]
		s.sessionsMu.RUnlock()
		if hasPrev && prevSessionID != "" {
			cliArgs = append(cliArgs, "--resume", prevSessionID)
		} else {
			cliArgs = append(cliArgs, "--continue")
		}
	}

	// Allowed tools restriction.
	if tools, _ := args["allowed_tools"].(string); tools != "" {
		cliArgs = append(cliArgs, "--allowedTools", tools)
	}

	// Additional directories.
	if dirs, _ := args["add_dirs"].(string); dirs != "" {
		for _, d := range strings.Split(dirs, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				cliArgs = append(cliArgs, "--add-dir", d)
			}
		}
	}

	// Inject project context as append-system-prompt so Claude Code knows
	// about the project's language, framework, commands, etc.
	if projectCtx != "" {
		cliArgs = append(cliArgs, "--append-system-prompt", projectCtx)
	}

	// The prompt goes last.
	cliArgs = append(cliArgs, prompt)

	// Execute with timeout.
	execCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "claude", cliArgs...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Set HOME if not set (needed for claude auth).
	cmd.Env = os.Environ()

	// Get stdout pipe for streaming. Stderr is captured separately for error context.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrWriter{buf: &stderrBuf}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start claude: %v", err)
	}

	// Extract progress sender from context (injected by tool_executor via
	// the well-known "goclaw.progress_sender" context key).
	type progressSenderFunc = func(ctx context.Context, message string)
	var progressSend progressSenderFunc
	if ps, ok := ctx.Value("goclaw.progress_sender").(progressSenderFunc); ok {
		progressSend = ps
	}

	// Send initial feedback.
	if progressSend != nil {
		progressSend(ctx, "🔧 Claude Code iniciado, processando...")
	}

	// Parse streaming JSONL output.
	tracker := &ccStreamTracker{
		progressSend: progressSend,
		ctx:          execCtx,
		minInterval:  12 * time.Second,
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024) // 1MB max line
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event claudeStreamEvent
		if json.Unmarshal(line, &event) == nil {
			tracker.handleEvent(&event)
		}
	}

	// Wait for the process to finish.
	cmdErr := cmd.Wait()

	// Build result from tracker.
	result := tracker.result()
	if result != nil {
		// Store session for continuation.
		if result.SessionID != "" {
			sessionKey, _ := args["session_key"].(string)
			if sessionKey != "" {
				s.sessionsMu.Lock()
				s.sessions[sessionKey] = result.SessionID
				s.sessionsMu.Unlock()
			}
		}

		// Check for errors.
		if result.IsError || result.Subtype == "error" {
			errMsg := result.Result
			if errMsg == "" {
				errMsg = fmt.Sprintf("Claude Code error (subtype: %s)", result.Subtype)
			}
			return nil, fmt.Errorf("%s", errMsg)
		}

		// Build response with metadata.
		response := result.Result
		if response == "" && result.Subtype == "error_max_budget_usd" {
			return nil, fmt.Errorf("Claude Code exceeded the budget limit of $%.2f", budget)
		}

		// Append cost/metadata footer.
		if result.TotalCost > 0 || result.NumTurns > 0 {
			meta := fmt.Sprintf("\n\n---\n💰 $%.4f | %d turns | %dms",
				result.TotalCost, result.NumTurns, result.Duration)
			if result.SessionID != "" && len(result.SessionID) >= 8 {
				meta += fmt.Sprintf(" | session: %s", result.SessionID[:8])
			}
			response += meta
		}

		return ccTruncate(response, 15000), nil
	}

	// Tracker didn't capture a result — fallback to error.
	if cmdErr != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("claude code: %s", ccTruncate(stderrStr, 3000))
		}
		return nil, fmt.Errorf("claude code failed: %v", cmdErr)
	}

	// If we have accumulated text from the stream, return it.
	if text := tracker.accumulatedText(); text != "" {
		return ccTruncate(text, 15000), nil
	}

	return "Claude Code completed without output.", nil
}

// stderrWriter captures stderr output with a size limit to prevent unbounded memory.
type stderrWriter struct {
	buf  *strings.Builder
	size int
}

func (w *stderrWriter) Write(p []byte) (n int, err error) {
	if w.size >= 4096 {
		return len(p), nil // Silently discard excess.
	}
	remain := 4096 - w.size
	if len(p) > remain {
		p = p[:remain]
	}
	w.size += len(p)
	w.buf.Write(p)
	return len(p), nil
}

// ccStreamTracker processes Claude Code stream-json events and extracts
// progress information for real-time user feedback.
type ccStreamTracker struct {
	progressSend func(ctx context.Context, message string)
	ctx          context.Context
	minInterval  time.Duration

	lastProgress time.Time
	toolsUsed    []string
	textBuf      strings.Builder
	finalResult  *claudeCodeResult
}

// handleEvent processes a single stream event from Claude Code.
func (t *ccStreamTracker) handleEvent(ev *claudeStreamEvent) {
	switch ev.Type {
	case "result":
		// Final result event.
		t.finalResult = &claudeCodeResult{
			Type:      ev.Type,
			Subtype:   ev.Subtype,
			Result:    ev.Result,
			IsError:   ev.IsError,
			SessionID: ev.SessionID,
			NumTurns:  ev.NumTurns,
			TotalCost: ev.TotalCost,
			Duration:  ev.Duration,
		}

	case "assistant":
		switch ev.Subtype {
		case "tool_use":
			t.toolsUsed = append(t.toolsUsed, ev.ToolName)
			t.maybeSendProgress(t.describeToolUse(ev.ToolName, ev.Input))
		case "text":
			// Accumulate assistant text for the final response.
			if ev.Content != "" {
				t.textBuf.WriteString(ev.Content)
			}
		}

	case "system":
		if ev.Subtype == "init" && ev.SessionID != "" {
			// Capture session ID early.
			if t.finalResult == nil {
				t.finalResult = &claudeCodeResult{SessionID: ev.SessionID}
			} else {
				t.finalResult.SessionID = ev.SessionID
			}
		}
	}
}

// maybeSendProgress sends a progress message if enough time has passed.
func (t *ccStreamTracker) maybeSendProgress(msg string) {
	if t.progressSend == nil || msg == "" {
		return
	}
	now := time.Now()
	if now.Sub(t.lastProgress) < t.minInterval {
		return
	}
	t.lastProgress = now
	t.progressSend(t.ctx, msg)
}

// describeToolUse creates a user-friendly description of what Claude Code is doing.
func (t *ccStreamTracker) describeToolUse(toolName string, input json.RawMessage) string {
	// Try to extract a meaningful detail from the input.
	var detail string
	if len(input) > 0 {
		var inputMap map[string]any
		if json.Unmarshal(input, &inputMap) == nil {
			// Extract common fields.
			if cmd, ok := inputMap["command"].(string); ok {
				detail = ccTruncate(cmd, 80)
			} else if path, ok := inputMap["file_path"].(string); ok {
				detail = path
			} else if path, ok := inputMap["path"].(string); ok {
				detail = path
			} else if pattern, ok := inputMap["pattern"].(string); ok {
				detail = pattern
			}
		}
	}

	// Build a descriptive message based on the tool.
	icon := ccToolIcon(toolName)
	turnCount := len(t.toolsUsed)
	base := fmt.Sprintf("%s %s", icon, toolName)
	if detail != "" {
		base += ": `" + detail + "`"
	}
	return fmt.Sprintf("⏳ [%d] %s", turnCount, base)
}

// ccToolIcon returns an emoji for the Claude Code tool name.
func ccToolIcon(tool string) string {
	switch strings.ToLower(tool) {
	case "bash", "shell":
		return "🖥️"
	case "read", "readfile":
		return "📖"
	case "write", "writefile":
		return "✏️"
	case "edit", "editfile":
		return "📝"
	case "grep", "search":
		return "🔍"
	case "glob":
		return "📂"
	case "listfiles":
		return "📁"
	default:
		return "🔧"
	}
}

// result returns the final result, or nil if not yet available.
func (t *ccStreamTracker) result() *claudeCodeResult {
	return t.finalResult
}

// accumulatedText returns any text accumulated from assistant text events.
func (t *ccStreamTracker) accumulatedText() string {
	return t.textBuf.String()
}

func (s *claudeCodeSkill) handleCheck(_ context.Context, _ map[string]any) (any, error) {
	// Check if claude is in PATH.
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return map[string]any{
			"installed": false,
			"message":   "Claude Code CLI not found. Install with: npm install -g @anthropic-ai/claude-code",
			"docs":      "https://docs.anthropic.com/en/docs/claude-code",
		}, nil
	}

	// Get version.
	versionOut, _ := exec.Command("claude", "--version").CombinedOutput()
	version := strings.TrimSpace(string(versionOut))

	// Check auth by running doctor.
	doctorOut, doctorErr := exec.Command("claude", "doctor").CombinedOutput()
	authOK := doctorErr == nil
	doctorInfo := strings.TrimSpace(string(doctorOut))

	return map[string]any{
		"installed":     true,
		"path":          claudePath,
		"version":       version,
		"authenticated": authOK,
		"doctor":        ccTruncate(doctorInfo, 2000),
		"message":       fmt.Sprintf("Claude Code %s ready at %s", version, claudePath),
	}, nil
}

// ── Helpers ──

// ccResolveProject resolves a project from args (project_id or session active).
// Prefixed with "cc" to avoid conflict with the package-level resolveProject
// from other coding skill files during transition.
func ccResolveProject(provider ProjectProvider, args map[string]any) *ProjectInfo {
	if id, _ := args["project_id"].(string); id != "" {
		return provider.Get(id)
	}
	if key, _ := args["session_key"].(string); key != "" {
		return provider.ActiveProject(key)
	}
	return nil
}

// buildProjectContext creates a system prompt fragment with project metadata.
func buildProjectContext(p *ProjectInfo) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Active project: %s\n", p.Name))
	b.WriteString(fmt.Sprintf("Language: %s\n", p.Language))
	if p.Framework != "" {
		b.WriteString(fmt.Sprintf("Framework: %s\n", p.Framework))
	}
	if p.GitRemote != "" {
		b.WriteString(fmt.Sprintf("Git remote: %s\n", p.GitRemote))
	}
	if p.BuildCmd != "" {
		b.WriteString(fmt.Sprintf("Build command: %s\n", p.BuildCmd))
	}
	if p.TestCmd != "" {
		b.WriteString(fmt.Sprintf("Test command: %s\n", p.TestCmd))
	}
	if p.LintCmd != "" {
		b.WriteString(fmt.Sprintf("Lint command: %s\n", p.LintCmd))
	}
	if p.StartCmd != "" {
		b.WriteString(fmt.Sprintf("Start command: %s\n", p.StartCmd))
	}
	if p.DeployCmd != "" {
		b.WriteString(fmt.Sprintf("Deploy command: %s\n", p.DeployCmd))
	}
	return b.String()
}

// ccTruncate truncates a string to maxLen characters.
func ccTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}

// parseFloat is a simple float parser for env vars.
func parseFloat(s string) (float64, error) {
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	return v, err
}

// parseInt is a simple int parser for env vars.
func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

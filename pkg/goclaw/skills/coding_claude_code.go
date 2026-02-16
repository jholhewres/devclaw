// Package skills – coding_claude_code.go integrates with the Claude Code CLI
// to provide full-stack coding capabilities: code editing, review, commit, PR,
// deployment, testing, refactoring, and any development task.
//
// Instead of implementing many granular tools (git_status, code_read, etc.),
// this skill delegates everything to Claude Code, which has its own rich set
// of tools (Bash, Read, Edit, Grep, Glob, Write, etc.).
//
// The skill follows the same pattern as OpenClaw's cli-runner:
//   - Uses --output-format json (single JSON result, no streaming)
//   - Uses --dangerously-skip-permissions (most reliable permission bypass)
//   - Passes prompt as positional arg (fallback to stdin for long prompts)
//   - Waits for completion and parses the JSON output
//
// Requirements:
//   - Claude Code CLI installed: npm install -g @anthropic-ai/claude-code
//   - Authenticated: claude setup-token or ANTHROPIC_API_KEY
//   - The user must enable "claude-code" in skills.builtin config.
package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// claudeCodeResult represents the JSON result from Claude Code CLI (--output-format json).
// Mirrors OpenClaw's parseCliJson fields.
type claudeCodeResult struct {
	// Standard fields from Claude Code JSON output.
	Type      string  `json:"type"`
	Subtype   string  `json:"subtype"`
	Result    string  `json:"result"`
	Message   string  `json:"message"`
	Content   string  `json:"content"`
	IsError   bool    `json:"is_error"`
	SessionID string  `json:"session_id"`
	NumTurns  int     `json:"num_turns"`
	TotalCost float64 `json:"total_cost_usd"`
	Duration  int     `json:"duration_ms"`
	Errors    []any   `json:"errors"`

	// Usage (may be nested).
	Usage json.RawMessage `json:"usage"`
}

// extractText returns the main text content from the result.
// Mirrors OpenClaw's collectText: checks result > message > content.
func (r *claudeCodeResult) extractText() string {
	if r.Result != "" {
		return r.Result
	}
	if r.Message != "" {
		return r.Message
	}
	if r.Content != "" {
		return r.Content
	}
	return ""
}

// claudeCodeSkill implements the claude-code skill.
type claudeCodeSkill struct {
	provider ProjectProvider

	// sessions maps GoClaw session key → last Claude Code session ID,
	// allowing multi-step coding tasks to be continued.
	sessions   map[string]string
	sessionsMu sync.RWMutex

	// claudeBin caches the resolved path to the claude binary.
	claudeBin   string
	claudeBinMu sync.RWMutex

	// Configurable defaults (can be overridden per call).
	defaultModel    string
	defaultBudget   float64
	skipPermissions bool
	timeout         time.Duration

	// apiKey and baseURL are injected from GoClaw's main LLM config.
	// When set, they are passed as ANTHROPIC_AUTH_TOKEN and ANTHROPIC_BASE_URL
	// to the Claude Code CLI, allowing it to use the same provider (e.g. Z.AI).
	apiKey  string
	baseURL string
}

// NewClaudeCodeSkill creates the claude-code skill.
// provider may be nil if project management is not configured.
// apiKey, baseURL, and defaultModelName are the LLM provider credentials from GoClaw's config;
// when non-empty, they are injected as env vars so Claude Code CLI authenticates
// through the same provider (Z.AI, Anthropic, etc.).
func NewClaudeCodeSkill(provider ProjectProvider, apiKey, baseURL, defaultModelName string) Skill {
	// Model: env var override takes precedence over config.
	model := os.Getenv("GOCLAW_CLAUDE_CODE_MODEL")
	if model == "" {
		model = defaultModelName
	}

	// Budget: 0 means no limit (same as interactive Claude Code).
	var budget float64
	if budgetStr := os.Getenv("GOCLAW_CLAUDE_CODE_BUDGET"); budgetStr != "" {
		if v, err := parseFloat(budgetStr); err == nil && v > 0 {
			budget = v
		}
	}

	timeoutMin := 18
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
		apiKey:          apiKey,
		baseURL:         baseURL,
	}
}

func (s *claudeCodeSkill) Metadata() Metadata {
	return Metadata{
		Name:        "claude-code",
		Version:     "1.1.0",
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
				{Name: "permission_mode", Type: "string", Description: "Permission mode: 'default', 'plan' (read-only analysis), 'bypass'. Default: bypass."},
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

	// Resolve claude binary — auto-install if missing.
	claudeBin, err := s.resolveClaudeBin(ctx)
	if err != nil {
		return nil, err
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

	// ── Build CLI arguments (OpenClaw pattern) ──
	// claude -p --output-format json --dangerously-skip-permissions [flags...] PROMPT
	cliArgs := []string{"-p", "--output-format", "json"}

	// Permission mode (OpenClaw: --dangerously-skip-permissions).
	permMode, _ := args["permission_mode"].(string)
	if permMode == "" && s.skipPermissions {
		permMode = "bypass"
	}
	switch permMode {
	case "bypass", "dangerouslySkipPermissions", "bypassPermissions":
		cliArgs = append(cliArgs, "--dangerously-skip-permissions")
	case "plan":
		cliArgs = append(cliArgs, "--permission-mode", "plan")
	case "default", "":
		// No flag.
	default:
		cliArgs = append(cliArgs, "--permission-mode", permMode)
	}

	// Model.
	model, _ := args["model"].(string)
	if model == "" {
		model = s.defaultModel
	}
	if model != "" {
		// Resolve common aliases (OpenClaw pattern).
		resolved := resolveModelAlias(model)
		cliArgs = append(cliArgs, "--model", resolved)
	}

	// Budget.
	budget := s.defaultBudget
	if b, ok := args["max_budget"].(float64); ok && b > 0 {
		budget = b
	}
	if budget > 0 {
		cliArgs = append(cliArgs, "--max-budget-usd", fmt.Sprintf("%.2f", budget))
	}

	// Session management (OpenClaw pattern: --session-id for every run, --resume for continuation).
	sessionKey, _ := args["session_key"].(string)
	if cont, _ := args["continue_session"].(bool); cont {
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

	// System prompt with project context.
	if projectCtx != "" {
		cliArgs = append(cliArgs, "--append-system-prompt", projectCtx)
	}

	// ── Prompt: always via stdin ──
	// Claude Code CLI v2.x hangs when prompt is passed as positional arg
	// in non-TTY environments (e.g. PM2, SSH). Piping via stdin works reliably.
	// This is the safest approach for server environments.

	// ── Execute ──
	execCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, claudeBin, cliArgs...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Log the CLI invocation for debugging (mask the API key).
	maskedKey := ""
	if s.apiKey != "" {
		if len(s.apiKey) > 8 {
			maskedKey = s.apiKey[:4] + "..." + s.apiKey[len(s.apiKey)-4:]
		} else {
			maskedKey = "***"
		}
	}
	slog.Info("claude-code: executing",
		"bin", claudeBin,
		"args_count", len(cliArgs),
		"work_dir", workDir,
		"has_api_key", s.apiKey != "",
		"api_key_masked", maskedKey,
		"base_url", s.baseURL,
		"model", s.defaultModel,
	)

	// Environment: inject API credentials so Claude Code CLI uses the same
	// provider as GoClaw (e.g. Z.AI). This mirrors how OpenClaw passes
	// ANTHROPIC_AUTH_TOKEN and ANTHROPIC_BASE_URL to the CLI.
	env := os.Environ()
	env = clearEnvKeys(env, "ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY_OLD",
		"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL")

	if s.apiKey != "" {
		env = append(env, "ANTHROPIC_API_KEY="+s.apiKey)
		env = append(env, "ANTHROPIC_AUTH_TOKEN="+s.apiKey)
	}
	if s.baseURL != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+s.baseURL)
	}
	if s.defaultModel != "" {
		env = append(env, "ANTHROPIC_DEFAULT_SONNET_MODEL="+s.defaultModel)
		env = append(env, "ANTHROPIC_DEFAULT_OPUS_MODEL="+s.defaultModel)
	}

	// Increase internal Claude Code timeouts for BashTool pre-flight checks.
	env = append(env, "API_TIMEOUT_MS=600000")
	env = append(env, "BASH_DEFAULT_TIMEOUT_MS=600000")
	env = append(env, "BASH_MAX_TIMEOUT_MS=900000")
	env = append(env, "DISABLE_PROMPT_CACHING=true")

	cmd.Env = env

	// Always pipe prompt via stdin (Claude Code v2.x hangs with positional arg in non-TTY).
	cmd.Stdin = strings.NewReader(prompt)

	// Capture stdout; stream stderr for real-time progress.
	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf

	// Stream stderr in real-time: send periodic progress updates to the user
	// instead of a single "trabalhando..." message. This gives real feedback
	// about what Claude Code is doing (compiling, running tests, etc.).
	ps := progressSenderFromCtx(ctx)
	stderrPipe, pipeErr := cmd.StderrPipe()
	if pipeErr != nil {
		// Fallback: capture stderr as buffer.
		var stderrBuf bytes.Buffer
		cmd.Stderr = &limitedWriter{buf: &stderrBuf, max: 8192}
	}

	// Send initial progress.
	if ps != nil {
		ps(ctx, "🤖 Claude Code iniciando...")
	}

	startErr := cmd.Start()
	if startErr != nil {
		return nil, fmt.Errorf("claude code: failed to start: %w", startErr)
	}

	// Stream stderr in background, capturing output.
	// Progress messages go through the ProgressSender which has its own
	// per-channel cooldown (e.g. 60s for WhatsApp), so we send only meaningful
	// updates and let the cooldown prevent flooding.
	var stderrBuf bytes.Buffer
	var stderrWg sync.WaitGroup
	if pipeErr == nil {
		stderrWg.Add(1)
		go func() {
			defer stderrWg.Done()
			buf := make([]byte, 4096)
			lastSend := time.Now()
			for {
				n, readErr := stderrPipe.Read(buf)
				if n > 0 {
					chunk := string(buf[:n])
					stderrBuf.WriteString(chunk)

					// Send meaningful stderr updates (cooldown is enforced upstream).
					if ps != nil && time.Since(lastSend) >= 30*time.Second {
						line := strings.TrimSpace(chunk)
						if line != "" && !strings.Contains(line, "Pre-flight check") {
							ps(ctx, "🤖 "+ccTruncate(line, 200))
							lastSend = time.Now()
						}
					}
				}
				if readErr != nil {
					break
				}
			}
		}()
	}

	cmdErr := cmd.Wait()

	// Wait for stderr goroutine to finish before reading stderrBuf,
	// preventing a data race between the reader goroutine and main goroutine.
	stderrWg.Wait()

	stdout := strings.TrimSpace(stdoutBuf.String())
	stderr := strings.TrimSpace(stderrBuf.String())

	// Log result for debugging.
	slog.Info("claude-code: execution completed",
		"exit_error", cmdErr,
		"stdout_len", len(stdout),
		"stderr_len", len(stderr),
		"stderr_preview", ccTruncate(stderr, 500),
	)

	// ── Parse result (OpenClaw pattern: parseCliJson) ──
	if cmdErr != nil {
		// Non-zero exit: return error with stderr context.
		errMsg := stderr
		if errMsg == "" {
			errMsg = stdout
		}
		if errMsg == "" {
			errMsg = cmdErr.Error()
		}
		return nil, fmt.Errorf("claude code: %s", ccTruncate(errMsg, 3000))
	}

	// Parse the JSON output.
	var result claudeCodeResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		// If JSON parsing fails, return raw stdout as text.
		if stdout != "" {
			return ccTruncate(stdout, 15000), nil
		}
		return nil, fmt.Errorf("claude code: no output (stderr: %s)", ccTruncate(stderr, 500))
	}

	// Store session ID for continuation.
	if result.SessionID != "" && sessionKey != "" {
		s.sessionsMu.Lock()
		s.sessions[sessionKey] = result.SessionID
		s.sessionsMu.Unlock()
	}

	// Check for errors in the result.
	if result.IsError || result.Subtype == "error" {
		errMsg := result.extractText()
		if errMsg == "" {
			errMsg = fmt.Sprintf("Claude Code error (subtype: %s)", result.Subtype)
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	if result.Subtype == "error_max_budget_usd" {
		return nil, fmt.Errorf("Claude Code exceeded the budget limit of $%.2f", budget)
	}

	// Build response.
	response := result.extractText()
	if response == "" {
		return "Claude Code completed without output.", nil
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

func (s *claudeCodeSkill) handleCheck(ctx context.Context, _ map[string]any) (any, error) {
	claudeBin, err := s.resolveClaudeBin(ctx)
	if err != nil {
		return map[string]any{
			"installed": false,
			"error":     err.Error(),
			"message":   "Claude Code CLI could not be found or installed.",
			"docs":      "https://docs.anthropic.com/en/docs/claude-code",
		}, nil
	}

	// Get version.
	versionOut, _ := exec.Command(claudeBin, "--version").CombinedOutput()
	version := strings.TrimSpace(string(versionOut))

	return map[string]any{
		"installed": true,
		"path":      claudeBin,
		"version":   version,
		"message":   fmt.Sprintf("Claude Code %s ready at %s", version, claudeBin),
	}, nil
}

// ── Binary resolution and auto-installation ──

// resolveClaudeBin finds the claude binary, caching the result.
// If not found, attempts auto-installation via npm.
func (s *claudeCodeSkill) resolveClaudeBin(ctx context.Context) (string, error) {
	// Check cache first.
	s.claudeBinMu.RLock()
	cached := s.claudeBin
	s.claudeBinMu.RUnlock()
	if cached != "" {
		// Verify it still exists.
		if _, err := os.Stat(cached); err == nil {
			return cached, nil
		}
	}

	// Try to find in PATH.
	if p, err := exec.LookPath("claude"); err == nil {
		s.cacheClaudeBin(p)
		return p, nil
	}

	// Check common locations (PM2 may not inherit full user PATH).
	commonPaths := []string{
		os.ExpandEnv("$HOME/.local/bin/claude"),
		os.ExpandEnv("$HOME/.npm-global/bin/claude"),
		"/usr/local/bin/claude",
		"/usr/bin/claude",
	}
	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			s.cacheClaudeBin(p)
			return p, nil
		}
	}

	// Try via bash login shell (picks up nvm/fnm/volta).
	if out, err := exec.CommandContext(ctx, "bash", "-lc", "which claude").CombinedOutput(); err == nil {
		p := strings.TrimSpace(string(out))
		if p != "" {
			s.cacheClaudeBin(p)
			return p, nil
		}
	}

	// Not found — attempt auto-install.
	return s.autoInstallClaude(ctx)
}

// installMu serializes install attempts so concurrent calls don't race.
var installMu sync.Mutex

// autoInstallClaude installs Claude Code CLI via npm.
func (s *claudeCodeSkill) autoInstallClaude(ctx context.Context) (string, error) {
	installMu.Lock()
	defer installMu.Unlock()

	// Double-check after acquiring lock.
	if p, err := exec.LookPath("claude"); err == nil {
		s.cacheClaudeBin(p)
		return p, nil
	}

	notify := progressSenderFromCtx(ctx)
	if notify != nil {
		notify(ctx, "📦 Instalando Claude Code CLI...")
	}

	// Install via bash login shell (inherits nvm/fnm/volta PATH).
	installCmd := exec.CommandContext(ctx, "bash", "-lc", "npm install -g @anthropic-ai/claude-code")
	installCmd.Env = os.Environ()
	installOut, installErr := installCmd.CombinedOutput()
	if installErr != nil {
		outStr := strings.TrimSpace(string(installOut))
		if len(outStr) > 500 {
			outStr = outStr[len(outStr)-500:]
		}
		return "", fmt.Errorf("failed to install Claude Code CLI: %v\nOutput: %s", installErr, outStr)
	}

	// Find the installed binary.
	// First, get npm global bin dir and add to PATH.
	if binOut, err := exec.CommandContext(ctx, "bash", "-lc", "npm bin -g").CombinedOutput(); err == nil {
		globalBin := strings.TrimSpace(string(binOut))
		if globalBin != "" {
			currentPath := os.Getenv("PATH")
			os.Setenv("PATH", globalBin+":"+currentPath)
		}
	}

	// Now find claude.
	claudeBin, err := s.resolveClaudeBinDirect(ctx)
	if err != nil {
		return "", fmt.Errorf("installed but claude not found: %v", err)
	}

	if notify != nil {
		versionOut, _ := exec.CommandContext(ctx, claudeBin, "--version").CombinedOutput()
		version := strings.TrimSpace(string(versionOut))
		notify(ctx, fmt.Sprintf("✅ Claude Code %s instalado", version))
	}

	return claudeBin, nil
}

// resolveClaudeBinDirect searches for claude without auto-install (avoids recursion).
func (s *claudeCodeSkill) resolveClaudeBinDirect(ctx context.Context) (string, error) {
	if p, err := exec.LookPath("claude"); err == nil {
		s.cacheClaudeBin(p)
		return p, nil
	}
	commonPaths := []string{
		os.ExpandEnv("$HOME/.local/bin/claude"),
		os.ExpandEnv("$HOME/.npm-global/bin/claude"),
		"/usr/local/bin/claude",
	}
	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			s.cacheClaudeBin(p)
			return p, nil
		}
	}
	if out, err := exec.CommandContext(ctx, "bash", "-lc", "which claude").CombinedOutput(); err == nil {
		p := strings.TrimSpace(string(out))
		if p != "" {
			s.cacheClaudeBin(p)
			return p, nil
		}
	}
	return "", fmt.Errorf("claude binary not found in PATH or common locations")
}

func (s *claudeCodeSkill) cacheClaudeBin(path string) {
	s.claudeBinMu.Lock()
	s.claudeBin = path
	s.claudeBinMu.Unlock()
}

// ── Helpers ──

// progressSenderFromCtx extracts the progress sender callback from context.
func progressSenderFromCtx(ctx context.Context) func(context.Context, string) {
	type progressSenderFunc = func(ctx context.Context, message string)
	if ps, ok := ctx.Value("goclaw.progress_sender").(progressSenderFunc); ok {
		return ps
	}
	return nil
}

// resolveModelAlias converts common model aliases (OpenClaw pattern).
func resolveModelAlias(model string) string {
	aliases := map[string]string{
		"opus":    "opus",
		"sonnet":  "sonnet",
		"haiku":   "haiku",
		"opus-4":  "opus",
		"opus-4.5": "opus",
		"opus-4.6": "opus",
		"sonnet-4": "sonnet",
		"sonnet-4.1": "sonnet",
		"sonnet-4.5": "sonnet",
		"haiku-3.5": "haiku",
	}
	if resolved, ok := aliases[strings.ToLower(model)]; ok {
		return resolved
	}
	return model
}

// clearEnvKeys removes specified keys from an environment slice.
func clearEnvKeys(env []string, keys ...string) []string {
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		skip := false
		for _, key := range keys {
			if strings.HasPrefix(e, key+"=") {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// limitedWriter captures output with a size limit.
type limitedWriter struct {
	buf  *bytes.Buffer
	max  int
	size int
}

func (w *limitedWriter) Write(p []byte) (n int, err error) {
	if w.size >= w.max {
		return len(p), nil
	}
	remain := w.max - w.size
	if len(p) > remain {
		p = p[:remain]
	}
	w.size += len(p)
	w.buf.Write(p)
	return len(p), nil
}

// ccResolveProject resolves a project from args (project_id or session active).
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

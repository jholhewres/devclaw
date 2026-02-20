// Package copilot – commands.go implements admin commands that can be
// executed via chat messages (WhatsApp, Discord, etc.).
//
// Commands are prefixed with "/" and only available to admins/owners:
//
//	/allow <phone>           - Grant user access
//	/block <phone>           - Block a user
//	/unblock <phone>         - Unblock a user
//	/revoke <phone>          - Revoke user access
//	/admin <phone>           - Promote user to admin
//	/users                   - List all authorized users
//	/ws create <id> <name>   - Create a workspace
//	/ws delete <id>          - Delete a workspace
//	/ws assign <phone> <id>  - Assign user to workspace
//	/ws list                 - List all workspaces
//	/ws info [id]            - Show workspace details
//	/ws set <key> <value>    - Update current workspace setting
//	/group allow             - Allow current group
//	/group block             - Block current group
//	/group assign <ws_id>    - Assign current group to workspace
//	/skills list             - List installed skills
//	/skills defaults         - List available default skills
//	/skills install <n|all>  - Install default skills
//	/status                  - Show bot status
//	/help                    - Show available commands
package copilot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
	"github.com/jholhewres/devclaw/pkg/devclaw/skills"
)

// CommandResult contains the result of a command execution.
type CommandResult struct {
	// Response is the text to send back.
	Response string

	// Handled is true if the message was a valid command.
	Handled bool
}

// IsCommand returns true if the message starts with "/".
func IsCommand(content string) bool {
	return strings.HasPrefix(strings.TrimSpace(content), "/")
}

// containsFlag checks if args contains a flag like --json or --full.
func containsFlag(args []string, flag string) bool {
	for _, arg := range args {
		if strings.ToLower(arg) == strings.ToLower(flag) {
			return true
		}
	}
	return false
}

// HandleCommand processes an admin command from a chat message.
// Returns handled=true if it was a valid command (even if permission denied).
func (a *Assistant) HandleCommand(msg *channels.IncomingMessage) CommandResult {
	content := strings.TrimSpace(msg.Content)
	if !IsCommand(content) {
		return CommandResult{Handled: false}
	}

	// Parse command and args.
	parts := strings.Fields(content)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	// Check permissions.
	senderLevel := a.accessMgr.GetLevel(msg.From)
	isAdmin := senderLevel == AccessOwner || senderLevel == AccessAdmin

	switch cmd {
	case "/help":
		return CommandResult{
			Response: a.helpCommand(isAdmin),
			Handled:  true,
		}

	case "/status":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.statusCommand(), Handled: true}

	case "/allow":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.allowCommand(args, msg.From), Handled: true}

	case "/block":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.blockCommand(args, msg.From), Handled: true}

	case "/unblock":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.unblockCommand(args, msg.From), Handled: true}

	case "/revoke":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.revokeCommand(args, msg.From), Handled: true}

	case "/admin":
		if senderLevel != AccessOwner {
			return CommandResult{Response: "Only owners can promote admins.", Handled: true}
		}
		return CommandResult{Response: a.adminCommand(args, msg.From), Handled: true}

	case "/users":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.usersCommand(), Handled: true}

	case "/ws", "/workspace":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.workspaceCommand(args, msg), Handled: true}

	case "/group":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.groupCommand(args, msg), Handled: true}

	// Approval commands (work even when session is busy).
	case "/approve":
		return CommandResult{Response: a.approveCommand(args, msg), Handled: true}
	case "/deny":
		return CommandResult{Response: a.denyCommand(args, msg), Handled: true}

	// Skill management commands.
	case "/skills":
		return CommandResult{Response: a.skillsCommand(args, msg), Handled: true}

	// Session commands (require resolved workspace + session).
	case "/stop":
		return CommandResult{Response: a.stopCommand(msg), Handled: true}
	case "/model":
		return CommandResult{Response: a.modelCommand(args, msg), Handled: true}
	case "/compact":
		return CommandResult{Response: a.compactCommand(msg), Handled: true}
	case "/new":
		return CommandResult{Response: a.newCommand(msg), Handled: true}
	case "/reset":
		return CommandResult{Response: a.resetCommand(msg), Handled: true}
	case "/think":
		return CommandResult{Response: a.thinkCommand(args, msg), Handled: true}

	case "/tts":
		return CommandResult{Response: a.ttsCommand(args, msg), Handled: true}

	// Extended directives.
	case "/verbose":
		return CommandResult{Response: a.verboseCommand(args, msg), Handled: true}
	case "/reasoning":
		return CommandResult{Response: a.thinkCommand(args, msg), Handled: true} // Alias for /think
	case "/queue":
		return CommandResult{Response: a.queueCommand(args, msg), Handled: true}
	case "/usage":
		return CommandResult{Response: a.usageCommand(args, msg), Handled: true}
	case "/activation":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.activationCommand(args, msg), Handled: true}

	// System administration commands (admin/owner only)
	case "/reload":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.systemCommands.ReloadCommand(args), Handled: true}

	case "/diagnostics":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		full := containsFlag(args, "--full")
		return CommandResult{Response: a.systemCommands.DiagnosticsCommand(full), Handled: true}

	case "/exec":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		if len(args) == 0 {
			return CommandResult{Response: "Usage: /exec <queue|approve|deny> [args]", Handled: true}
		}
		switch strings.ToLower(args[0]) {
		case "queue":
			return CommandResult{Response: a.systemCommands.ExecQueueCommand(), Handled: true}
		case "approve":
			return CommandResult{Response: a.approveCommand(args[1:], msg), Handled: true}
		case "deny":
			return CommandResult{Response: a.denyCommand(args[1:], msg), Handled: true}
		default:
			return CommandResult{Response: "Usage: /exec <queue|approve|deny> [args]", Handled: true}
		}

	case "/channels":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.systemCommands.ChannelsCommand(args), Handled: true}

	case "/maintenance":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.systemCommands.MaintenanceCommand(args, msg.From), Handled: true}

	case "/logs":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.systemCommands.LogsCommand(args), Handled: true}

	case "/health":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.systemCommands.HealthCommand(), Handled: true}

	case "/metrics":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.systemCommands.MetricsCommand(args), Handled: true}

	// Tool profile commands.
	case "/profile":
		return CommandResult{Response: a.profileCommand(args, msg, isAdmin), Handled: true}

	// DM pairing commands.
	case "/pairing":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.pairingCommand(args, msg), Handled: true}

	// Vault management commands.
	case "/vault":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.vaultCommand(args), Handled: true}

	// Hooks management commands.
	case "/hooks":
		if !isAdmin {
			return CommandResult{Response: "Permission denied.", Handled: true}
		}
		return CommandResult{Response: a.hooksCommand(args), Handled: true}

	default:
		return CommandResult{Handled: false}
	}
}

// --- Command implementations ---

func (a *Assistant) helpCommand(isAdmin bool) string {
	var b strings.Builder
	b.WriteString("*DevClaw Commands*\n\n")

	if isAdmin {
		b.WriteString("*Access Control:*\n")
		b.WriteString("/allow <phone> - Grant user access\n")
		b.WriteString("/block <phone> - Block a user\n")
		b.WriteString("/unblock <phone> - Unblock a user\n")
		b.WriteString("/revoke <phone> - Revoke access\n")
		b.WriteString("/admin <phone> - Promote to admin\n")
		b.WriteString("/users - List authorized users\n\n")

		b.WriteString("*Workspaces:*\n")
		b.WriteString("/ws create <id> <name> - Create workspace\n")
		b.WriteString("/ws delete <id> - Delete workspace\n")
		b.WriteString("/ws assign <phone> <id> - Assign user\n")
		b.WriteString("/ws list - List workspaces\n")
		b.WriteString("/ws info [id] - Workspace details\n\n")

		b.WriteString("*Groups:*\n")
		b.WriteString("/group allow - Allow this group\n")
		b.WriteString("/group block - Block this group\n")
		b.WriteString("/group assign <ws_id> - Assign to workspace\n\n")

		b.WriteString("*System:*\n")
		b.WriteString("/reload [section] - Reload configuration\n")
		b.WriteString("/status [--json] - System status\n")
		b.WriteString("/diagnostics [--full] - System diagnostics\n")
		b.WriteString("/channels [connect|disconnect] - Channel management\n")
		b.WriteString("/maintenance [on|off] [msg] - Maintenance mode\n")
		b.WriteString("/logs [level] [lines] - View audit logs\n")
		b.WriteString("/health - Health check\n")
		b.WriteString("/metrics [period] - Usage metrics\n")
		b.WriteString("/profile [list|set <name>] - View or set tool profile\n")
		b.WriteString("/pairing generate|list|requests - DM access tokens\n")
		b.WriteString("/vault list|set|get|delete - Manage secrets\n")
		b.WriteString("/hooks list|enable <name>|disable <name> - Manage hooks\n\n")

		b.WriteString("/status - Bot status (legacy)\n")
	}

	b.WriteString("\n*Approval:*\n")
	b.WriteString("/approve <id> - Approve a pending tool execution\n")
	b.WriteString("/deny <id> - Deny a pending tool execution\n\n")

	b.WriteString("*Skills:*\n")
	b.WriteString("/skills list - List installed skills\n")
	b.WriteString("/skills defaults - List available default skills\n")
	b.WriteString("/skills install <names|all> - Install default skills\n\n")

	b.WriteString("*Session:*\n")
	b.WriteString("/stop - Stop active agent run\n")
	b.WriteString("/model [name] - Show or change model\n")
	b.WriteString("/compact - Compact session history\n")
	b.WriteString("/new - Start new session (keep facts & config)\n")
	b.WriteString("/reset - Full session reset\n")
	b.WriteString("/usage [reset] - Show token usage\n")
	b.WriteString("/think [off|low|medium|high] - Set thinking level\n")
	b.WriteString("/tts [off|always|inbound] - Toggle text-to-speech\n")
	b.WriteString("/verbose [on|off] - Toggle verbose tool narration\n")
	b.WriteString("/reasoning [off|low|medium|high] - Set reasoning level (alias: /think)\n")
	b.WriteString("/queue [collect|steer|followup|interrupt] - Set queue mode\n")
	b.WriteString("/usage [reset|global] - Show token usage\n")

	if isAdmin {
		b.WriteString("/activation [always|mention] - Set group activation mode\n")
	}

	b.WriteString("\n/help - Show this message")
	return b.String()
}

func (a *Assistant) usageCommand(args []string, msg *channels.IncomingMessage) string {
	resolved := a.workspaceMgr.Resolve(msg.Channel, msg.ChatID, msg.From, msg.IsGroup)
	session := resolved.Session
	isAdmin := a.accessMgr.GetLevel(msg.From) == AccessOwner || a.accessMgr.GetLevel(msg.From) == AccessAdmin

	if len(args) > 0 {
		arg := strings.ToLower(args[0])
		if arg == "reset" {
			session.ResetTokenUsage()
			if a.usageTracker != nil {
				a.usageTracker.ResetSession(session.ID)
			}
			return "Usage counters reset."
		}
		if arg == "global" {
			if !isAdmin {
				return "Permission denied."
			}
			if a.usageTracker != nil {
				return a.usageTracker.FormatGlobalUsage()
			}
			return "Usage tracking not available."
		}
		// Session ID - admin only
		if !isAdmin {
			return "Permission denied."
		}
		if a.usageTracker != nil {
			return a.usageTracker.FormatUsage(args[0])
		}
		return "Usage tracking not available."
	}

	// No args: show usage for current chat's session (Session + UsageTracker)
	promptTok, completionTok, requests := session.GetTokenUsage()
	total := promptTok + completionTok
	var b strings.Builder
	b.WriteString("*Token Usage*\n\n")
	b.WriteString(fmt.Sprintf("Prompt: %d | Completion: %d | Total: %d\n", promptTok, completionTok, total))
	b.WriteString(fmt.Sprintf("Requests: %d\n", requests))
	if a.usageTracker != nil {
		if su := a.usageTracker.GetSession(session.ID); su != nil && su.EstimatedCostUSD > 0 {
			b.WriteString(fmt.Sprintf("Est. cost: $%.4f\n", su.EstimatedCostUSD))
		}
	}
	return b.String()
}

func (a *Assistant) approveCommand(args []string, msg *channels.IncomingMessage) string {
	sessionID := MakeSessionID(msg.Channel, msg.ChatID)

	// If no ID provided, approve the most recent pending request for this session.
	var targetID string
	if len(args) >= 1 && args[0] != "" {
		targetID = args[0]
	} else {
		targetID = a.approvalMgr.LatestPendingForSession(sessionID)
		if targetID == "" {
			return "No pending approvals."
		}
	}

	if a.approvalMgr.Resolve(targetID, sessionID, msg.From, true, "") {
		return "✅ Approved."
	}
	return "Approval not found or already resolved."
}

func (a *Assistant) denyCommand(args []string, msg *channels.IncomingMessage) string {
	sessionID := MakeSessionID(msg.Channel, msg.ChatID)

	// If no ID provided, deny the most recent pending request.
	var targetID string
	var reason string
	if len(args) >= 1 && args[0] != "" {
		targetID = args[0]
		if len(args) > 1 {
			reason = strings.Join(args[1:], " ")
		}
	} else {
		targetID = a.approvalMgr.LatestPendingForSession(sessionID)
		if targetID == "" {
			return "No pending approvals."
		}
	}

	if a.approvalMgr.Resolve(targetID, sessionID, msg.From, false, reason) {
		return "❌ Denied."
	}
	return "Approval not found or already resolved."
}

func (a *Assistant) stopCommand(msg *channels.IncomingMessage) string {
	resolved := a.workspaceMgr.Resolve(msg.Channel, msg.ChatID, msg.From, msg.IsGroup)
	if a.StopActiveRun(resolved.Workspace.ID, resolved.Session.ID) {
		return "Agent stopped. Session unlocked."
	}
	return "No active run."
}

func (a *Assistant) modelCommand(args []string, msg *channels.IncomingMessage) string {
	resolved := a.workspaceMgr.Resolve(msg.Channel, msg.ChatID, msg.From, msg.IsGroup)
	cfg := resolved.Session.GetConfig()

	if len(args) == 0 {
		model := cfg.Model
		if model == "" {
			model = resolved.Workspace.Model
		}
		if model == "" {
			model = a.config.Model
		}
		return fmt.Sprintf("Current model: %s", model)
	}

	newModel := strings.TrimSpace(strings.Join(args, " "))
	if newModel == "" {
		return "Usage: /model [model_name]"
	}
	cfg.Model = newModel
	resolved.Session.SetConfig(cfg)
	return fmt.Sprintf("Model changed to: %s", newModel)
}

func (a *Assistant) compactCommand(msg *channels.IncomingMessage) string {
	resolved := a.workspaceMgr.Resolve(msg.Channel, msg.ChatID, msg.From, msg.IsGroup)
	oldLen, newLen := a.forceCompactSession(resolved.Session)
	if oldLen < 5 {
		return fmt.Sprintf("Session history too short to compact (%d entries).", oldLen)
	}
	return fmt.Sprintf("Session compacted. History: %d entries → %d entries.", oldLen, newLen)
}

func (a *Assistant) newCommand(msg *channels.IncomingMessage) string {
	resolved := a.workspaceMgr.Resolve(msg.Channel, msg.ChatID, msg.From, msg.IsGroup)
	session := resolved.Session

	// Session-memory hook: capture history snapshot, then clear.
	// We capture before clearing to avoid a race with the goroutine.
	if a.config.Memory.SessionMemory.Enabled && a.memoryStore != nil {
		maxMsg := a.config.Memory.SessionMemory.Messages
		if maxMsg <= 0 {
			maxMsg = 15
		}
		historySnapshot := session.RecentHistory(maxMsg)
		if len(historySnapshot) >= 2 {
			go a.summarizeAndSaveSessionFromHistory(historySnapshot)
		}
	}

	session.ClearHistory()

	// Clear session-scoped tool trust (user must re-approve tools in new session).
	sessionID := MakeSessionID(msg.Channel, msg.ChatID)
	a.approvalMgr.ClearSessionTrust(sessionID)

	return "New session started. Facts and config preserved."
}

func (a *Assistant) resetCommand(msg *channels.IncomingMessage) string {
	resolved := a.workspaceMgr.Resolve(msg.Channel, msg.ChatID, msg.From, msg.IsGroup)
	session := resolved.Session
	session.ClearHistory()
	session.ClearFacts()
	session.SetActiveSkills(nil)
	session.ResetTokenUsage()
	cfg := session.GetConfig()
	cfg.Model = ""
	cfg.ThinkingLevel = ""
	session.SetConfig(cfg)
	if a.usageTracker != nil {
		a.usageTracker.ResetSession(session.ID)
	}

	// Clear session-scoped tool trust.
	sessionID := MakeSessionID(msg.Channel, msg.ChatID)
	a.approvalMgr.ClearSessionTrust(sessionID)

	return "Session reset completely."
}

func (a *Assistant) thinkCommand(args []string, msg *channels.IncomingMessage) string {
	resolved := a.workspaceMgr.Resolve(msg.Channel, msg.ChatID, msg.From, msg.IsGroup)
	session := resolved.Session

	if len(args) == 0 {
		level := session.GetThinkingLevel()
		if level == "" {
			level = "off"
		}
		return fmt.Sprintf("Thinking level: %s", level)
	}

	level := strings.ToLower(strings.TrimSpace(args[0]))
	valid := map[string]bool{"off": true, "low": true, "medium": true, "high": true}
	if !valid[level] {
		return "Usage: /think [off|low|medium|high]"
	}
	session.SetThinkingLevel(level)
	return fmt.Sprintf("Thinking level: %s", level)
}

func (a *Assistant) ttsCommand(args []string, msg *channels.IncomingMessage) string {
	if len(args) == 0 {
		mode := a.config.TTS.AutoMode
		if !a.config.TTS.Enabled {
			mode = "disabled"
		}
		return fmt.Sprintf("TTS mode: %s (voice: %s)", mode, a.config.TTS.Voice)
	}

	mode := strings.ToLower(strings.TrimSpace(args[0]))
	valid := map[string]bool{"off": true, "always": true, "inbound": true}
	if !valid[mode] {
		return "Usage: /tts [off|always|inbound]"
	}

	a.configMu.Lock()
	if mode == "off" {
		a.config.TTS.Enabled = false
		a.config.TTS.AutoMode = "off"
	} else {
		a.config.TTS.Enabled = true
		a.config.TTS.AutoMode = mode
		// Initialize TTS provider if not yet done.
		if a.ttsProvider == nil {
			a.ttsProvider = a.buildTTSProvider()
		}
	}
	a.configMu.Unlock()

	return fmt.Sprintf("TTS mode set to: %s", mode)
}

func (a *Assistant) statusCommand() string {
	health := a.channelMgr.HealthAll()
	workspaces := a.workspaceMgr.Count()
	users := a.accessMgr.ListUsers()

	var b strings.Builder
	b.WriteString("*DevClaw Status*\n\n")
	b.WriteString(fmt.Sprintf("Workspaces: %d\n", workspaces))
	b.WriteString(fmt.Sprintf("Users: %d\n", len(users)))

	for name, h := range health {
		status := "disconnected"
		if h.Connected {
			status = "connected"
		}
		b.WriteString(fmt.Sprintf("Channel %s: %s (errors: %d)\n", name, status, h.ErrorCount))
	}

	return b.String()
}

func (a *Assistant) allowCommand(args []string, grantedBy string) string {
	if len(args) < 1 {
		return "Usage: /allow <phone_number>"
	}
	jid := args[0]
	if err := a.accessMgr.Grant(jid, AccessUser, grantedBy); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return fmt.Sprintf("User %s has been granted access.", jid)
}

func (a *Assistant) blockCommand(args []string, blockedBy string) string {
	if len(args) < 1 {
		return "Usage: /block <phone_number>"
	}
	jid := args[0]
	a.accessMgr.Block(jid, blockedBy)
	return fmt.Sprintf("User %s has been blocked.", jid)
}

func (a *Assistant) unblockCommand(args []string, unblockedBy string) string {
	if len(args) < 1 {
		return "Usage: /unblock <phone_number>"
	}
	jid := args[0]
	a.accessMgr.Unblock(jid, unblockedBy)
	return fmt.Sprintf("User %s has been unblocked.", jid)
}

func (a *Assistant) revokeCommand(args []string, revokedBy string) string {
	if len(args) < 1 {
		return "Usage: /revoke <phone_number>"
	}
	jid := args[0]
	a.accessMgr.Revoke(jid, revokedBy)
	return fmt.Sprintf("Access revoked for %s.", jid)
}

func (a *Assistant) adminCommand(args []string, grantedBy string) string {
	if len(args) < 1 {
		return "Usage: /admin <phone_number>"
	}
	jid := args[0]
	if err := a.accessMgr.Grant(jid, AccessAdmin, grantedBy); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return fmt.Sprintf("User %s promoted to admin.", jid)
}

func (a *Assistant) usersCommand() string {
	entries := a.accessMgr.ListUsers()
	if len(entries) == 0 {
		return "No users configured."
	}

	var b strings.Builder
	b.WriteString("*Authorized Users:*\n\n")

	for _, e := range entries {
		b.WriteString(fmt.Sprintf("• %s [%s]", e.JID, e.Level))
		if e.Note != "" {
			b.WriteString(fmt.Sprintf(" - %s", e.Note))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (a *Assistant) workspaceCommand(args []string, msg *channels.IncomingMessage) string {
	if len(args) == 0 {
		return "Usage: /ws <create|delete|assign|list|info> [args...]"
	}

	sub := strings.ToLower(args[0])
	subArgs := args[1:]

	switch sub {
	case "create":
		if len(subArgs) < 2 {
			return "Usage: /ws create <id> <name...>"
		}
		id := subArgs[0]
		name := strings.Join(subArgs[1:], " ")
		ws := Workspace{
			ID:   id,
			Name: name,
		}
		if err := a.workspaceMgr.Create(ws, msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("Workspace '%s' (%s) created.", name, id)

	case "delete":
		if len(subArgs) < 1 {
			return "Usage: /ws delete <id>"
		}
		if err := a.workspaceMgr.Delete(subArgs[0], msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("Workspace '%s' deleted.", subArgs[0])

	case "assign":
		if len(subArgs) < 2 {
			return "Usage: /ws assign <phone> <workspace_id>"
		}
		if err := a.workspaceMgr.AssignUser(subArgs[0], subArgs[1], msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("User %s assigned to workspace '%s'.", subArgs[0], subArgs[1])

	case "list":
		workspaces := a.workspaceMgr.List()
		if len(workspaces) == 0 {
			return "No workspaces configured."
		}

		var b strings.Builder
		b.WriteString("*Workspaces:*\n\n")
		for _, ws := range workspaces {
			status := "active"
			if !ws.Active {
				status = "inactive"
			}
			b.WriteString(fmt.Sprintf("• *%s* (%s) - %s\n", ws.Name, ws.ID, status))
			b.WriteString(fmt.Sprintf("  Members: %d | Groups: %d\n", len(ws.Members), len(ws.Groups)))
			if ws.Model != "" {
				b.WriteString(fmt.Sprintf("  Model: %s\n", ws.Model))
			}
		}
		return b.String()

	case "info":
		wsID := ""
		if len(subArgs) > 0 {
			wsID = subArgs[0]
		} else {
			// Show workspace for the current sender.
			if ws, ok := a.workspaceMgr.GetForUser(msg.From); ok {
				wsID = ws.ID
			}
		}
		if wsID == "" {
			return "Usage: /ws info <id>"
		}

		ws, ok := a.workspaceMgr.Get(wsID)
		if !ok {
			return fmt.Sprintf("Workspace '%s' not found.", wsID)
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("*Workspace: %s*\n", ws.Name))
		b.WriteString(fmt.Sprintf("ID: %s\n", ws.ID))
		b.WriteString(fmt.Sprintf("Active: %v\n", ws.Active))
		if ws.Description != "" {
			b.WriteString(fmt.Sprintf("Description: %s\n", ws.Description))
		}
		if ws.Model != "" {
			b.WriteString(fmt.Sprintf("Model: %s\n", ws.Model))
		}
		if ws.Language != "" {
			b.WriteString(fmt.Sprintf("Language: %s\n", ws.Language))
		}
		if ws.Instructions != "" {
			instr := ws.Instructions
			if len(instr) > 100 {
				instr = instr[:100] + "..."
			}
			b.WriteString(fmt.Sprintf("Instructions: %s\n", instr))
		}
		if len(ws.Skills) > 0 {
			b.WriteString(fmt.Sprintf("Skills: %s\n", strings.Join(ws.Skills, ", ")))
		}
		b.WriteString(fmt.Sprintf("Members (%d): %s\n", len(ws.Members), strings.Join(ws.Members, ", ")))
		b.WriteString(fmt.Sprintf("Groups (%d): %s\n", len(ws.Groups), strings.Join(ws.Groups, ", ")))
		if !ws.CreatedAt.IsZero() {
			b.WriteString(fmt.Sprintf("Created: %s", ws.CreatedAt.Format(time.RFC3339)))
			if ws.CreatedBy != "" {
				b.WriteString(fmt.Sprintf(" by %s", ws.CreatedBy))
			}
			b.WriteString("\n")
		}

		return b.String()

	default:
		return "Unknown workspace command. Use: create, delete, assign, list, info"
	}
}

func (a *Assistant) skillsCommand(args []string, msg *channels.IncomingMessage) string {
	if len(args) == 0 {
		return "Usage: /skills <list|defaults|install> [args...]\n\n" +
			"/skills list — installed skills\n" +
			"/skills defaults — available default skills\n" +
			"/skills install <name1> <name2> ... — install default skills\n" +
			"/skills install all — install all default skills"
	}

	sub := strings.ToLower(args[0])
	subArgs := args[1:]

	// Resolve skills directory from config.
	skillsDir := "./skills"
	if len(a.config.Skills.ClawdHubDirs) > 0 {
		skillsDir = a.config.Skills.ClawdHubDirs[0]
	}

	switch sub {
	case "list":
		allSkills := a.skillRegistry.List()
		if len(allSkills) == 0 {
			return "No skills installed.\n\nUse /skills install all to install defaults."
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("*Installed Skills (%d):*\n\n", len(allSkills)))
		for _, meta := range allSkills {
			desc := meta.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			b.WriteString(fmt.Sprintf("• *%s* — %s\n", meta.Name, desc))
		}
		return b.String()

	case "defaults":
		defaults := skills.DefaultSkills()
		installed := make(map[string]bool)
		for _, m := range a.skillRegistry.List() {
			installed[m.Name] = true
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("*Default Skills (%d):*\n\n", len(defaults)))
		for _, d := range defaults {
			status := ""
			if installed[d.Name] {
				status = " ✓"
			}
			b.WriteString(fmt.Sprintf("• *%s*%s — %s\n", d.Name, status, d.Description))
		}
		b.WriteString("\nUse /skills install <name> or /skills install all")
		return b.String()

	case "install":
		if len(subArgs) == 0 {
			return "Usage: /skills install <name1> <name2> ... or /skills install all"
		}

		names := subArgs
		if len(names) == 1 && strings.ToLower(names[0]) == "all" {
			names = skills.DefaultSkillNames()
		}

		installed, skipped, failed := skills.InstallDefaultSkills(skillsDir, names)

		// Hot-reload registry.
		reloadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		reloaded, _ := a.skillRegistry.Reload(reloadCtx)

		var b strings.Builder
		b.WriteString("*Skills Installation:*\n")
		b.WriteString(fmt.Sprintf("  Installed: %d\n", installed))
		if skipped > 0 {
			b.WriteString(fmt.Sprintf("  Already existed: %d\n", skipped))
		}
		if failed > 0 {
			b.WriteString(fmt.Sprintf("  Failed: %d\n", failed))
		}
		b.WriteString(fmt.Sprintf("\nSkill catalog reloaded (%d skills).", reloaded))
		return b.String()

	default:
		return "Unknown subcommand. Use: list, defaults, install"
	}
}

func (a *Assistant) verboseCommand(args []string, msg *channels.IncomingMessage) string {
	resolved := a.workspaceMgr.Resolve(msg.Channel, msg.ChatID, msg.From, msg.IsGroup)
	session := resolved.Session
	cfg := session.GetConfig()

	if len(args) == 0 {
		mode := "off"
		if cfg.Verbose {
			mode = "on"
		}
		return fmt.Sprintf("Verbose mode: %s", mode)
	}

	switch strings.ToLower(args[0]) {
	case "on", "true", "1":
		cfg.Verbose = true
		session.SetConfig(cfg)
		return "Verbose mode: on — tool calls will be narrated."
	case "off", "false", "0":
		cfg.Verbose = false
		session.SetConfig(cfg)
		return "Verbose mode: off — routine tool calls are silent."
	default:
		return "Usage: /verbose [on|off]"
	}
}

func (a *Assistant) queueCommand(args []string, msg *channels.IncomingMessage) string {
	if len(args) == 0 {
		a.configMu.RLock()
		mode := EffectiveQueueMode(a.config.Queue, msg.Channel)
		a.configMu.RUnlock()
		return fmt.Sprintf("Queue mode: %s\n\nAvailable: collect, steer, followup, interrupt, steer-backlog", mode)
	}

	mode, ok := ParseQueueMode(args[0])
	if !ok {
		return "Unknown queue mode. Available: collect, steer, followup, interrupt, steer-backlog"
	}

	// Update the per-channel override.
	a.configMu.Lock()
	if a.config.Queue.ByChannel == nil {
		a.config.Queue.ByChannel = make(map[string]QueueMode)
	}
	a.config.Queue.ByChannel[msg.Channel] = mode
	a.configMu.Unlock()

	return fmt.Sprintf("Queue mode set to: %s (for channel: %s)", mode, msg.Channel)
}

func (a *Assistant) activationCommand(args []string, msg *channels.IncomingMessage) string {
	if len(args) == 0 {
		a.configMu.RLock()
		trigger := a.config.Trigger
		a.configMu.RUnlock()
		if trigger == "" {
			trigger = "always (no trigger)"
		}
		return fmt.Sprintf("Current activation: %s", trigger)
	}

	switch strings.ToLower(args[0]) {
	case "always":
		a.configMu.Lock()
		a.config.Trigger = ""
		a.configMu.Unlock()
		return "Activation mode: always (responds to all messages in groups)"
	case "mention":
		a.configMu.Lock()
		name := a.config.Name
		if name == "" {
			name = "devclaw"
		}
		a.config.Trigger = name
		a.configMu.Unlock()
		return fmt.Sprintf("Activation mode: mention-only (requires '%s' in message)", name)
	default:
		return "Usage: /activation [always|mention]"
	}
}

func (a *Assistant) groupCommand(args []string, msg *channels.IncomingMessage) string {
	if !msg.IsGroup {
		return "This command can only be used in groups."
	}

	if len(args) == 0 {
		return "Usage: /group <allow|block|assign> [args...]"
	}

	sub := strings.ToLower(args[0])
	subArgs := args[1:]

	switch sub {
	case "allow":
		if err := a.accessMgr.GrantGroup(msg.ChatID, AccessUser, msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return "Group allowed."

	case "block":
		if err := a.accessMgr.GrantGroup(msg.ChatID, AccessBlocked, msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return "Group blocked."

	case "assign":
		if len(subArgs) < 1 {
			return "Usage: /group assign <workspace_id>"
		}
		if err := a.workspaceMgr.AssignGroup(msg.ChatID, subArgs[0], msg.From); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("Group assigned to workspace '%s'.", subArgs[0])

	default:
		return "Unknown group command. Use: allow, block, assign"
	}
}

// profileCommand handles the /profile command for viewing and setting tool profiles.
func (a *Assistant) profileCommand(args []string, msg *channels.IncomingMessage, isAdmin bool) string {
	if len(args) == 0 {
		// Show current profile.
		resolved := a.workspaceMgr.Resolve(msg.Channel, msg.ChatID, msg.From, msg.IsGroup)
		ws := resolved.Workspace

		// Determine effective profile (workspace overrides global).
		profileName := ws.ToolProfile
		if profileName == "" {
			profileName = a.config.Security.ToolGuard.Profile
		}
		if profileName == "" {
			profileName = "(none - using permission levels)"
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("*Tool Profile: %s*\n\n", profileName))

		// Show profile details if a profile is set.
		if guard := a.toolExecutor.Guard(); guard != nil {
			if profile := guard.GetActiveProfile(); profile != nil {
				b.WriteString(fmt.Sprintf("Description: %s\n", profile.Description))
				if len(profile.Allow) > 0 {
					b.WriteString(fmt.Sprintf("Allowed: %s\n", strings.Join(profile.Allow, ", ")))
				} else {
					b.WriteString("Allowed: (all)\n")
				}
				if len(profile.Deny) > 0 {
					b.WriteString(fmt.Sprintf("Denied: %s\n", strings.Join(profile.Deny, ", ")))
				}
			}
		}

		b.WriteString("\nUse /profile list to see available profiles.")
		if isAdmin {
			b.WriteString("\nUse /profile set <name> to change the workspace profile.")
		}
		return b.String()
	}

	sub := strings.ToLower(args[0])
	subArgs := args[1:]

	switch sub {
	case "list":
		// List all available profiles.
		profiles := ListProfiles(a.config.Security.ToolGuard.CustomProfiles)

		var b strings.Builder
		b.WriteString("*Available Tool Profiles:*\n\n")

		for _, name := range profiles {
			profile := GetProfile(name, a.config.Security.ToolGuard.CustomProfiles)
			if profile != nil {
				b.WriteString(fmt.Sprintf("• *%s* - %s\n", name, profile.Description))
			}
		}

		b.WriteString("\nBuilt-in: minimal, coding, messaging, full")
		return b.String()

	case "set":
		if !isAdmin {
			return "Permission denied. Only admins can set profiles."
		}
		if len(subArgs) < 1 {
			return "Usage: /profile set <profile_name>"
		}

		profileName := subArgs[0]

		// Validate profile exists.
		if GetProfile(profileName, a.config.Security.ToolGuard.CustomProfiles) == nil {
			return fmt.Sprintf("Profile '%s' not found. Use /profile list to see available profiles.", profileName)
		}

		// Update the workspace profile.
		resolved := a.workspaceMgr.Resolve(msg.Channel, msg.ChatID, msg.From, msg.IsGroup)
		wsID := resolved.Workspace.ID

		err := a.workspaceMgr.Update(wsID, func(ws *Workspace) {
			ws.ToolProfile = profileName
		})
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		return fmt.Sprintf("Tool profile set to '%s' for workspace '%s'.", profileName, wsID)

	default:
		return "Usage: /profile [list|set <name>]"
	}
}

// pairingCommand handles the /pairing command for DM access tokens.
func (a *Assistant) pairingCommand(args []string, msg *channels.IncomingMessage) string {
	if a.pairingMgr == nil {
		return "Pairing system not available (no database)."
	}

	if len(args) == 0 {
		return a.pairingHelp()
	}

	sub := strings.ToLower(args[0])
	subArgs := args[1:]

	switch sub {
	case "generate", "gen", "create":
		return a.pairingGenerateCommand(subArgs, msg.From)

	case "list", "ls":
		return a.pairingListCommand(subArgs)

	case "info":
		return a.pairingInfoCommand(subArgs)

	case "revoke":
		return a.pairingRevokeCommand(subArgs, msg.From)

	case "requests", "pending":
		return a.pairingRequestsCommand()

	case "approve":
		return a.pairingApproveCommand(subArgs, msg.From)

	case "deny":
		return a.pairingDenyCommand(subArgs, msg.From)

	default:
		return a.pairingHelp()
	}
}

func (a *Assistant) pairingHelp() string {
	return `*DM Pairing Commands*

/pairing generate [expires] [max_uses] [role] [options]
  Generate a new pairing token
  expires: 1h, 24h, 7d, 30d, or "never" (default: never)
  max_uses: number or "unlimited" (default: unlimited)
  role: user or admin (default: user)
  Options:
    --auto         Auto-approve users (no admin review)
    --ws <id>      Assign to workspace
    --note <text>  Admin note

/pairing list [--all]
  List active tokens (--all includes revoked)

/pairing info <token_or_id>
  Show token details

/pairing revoke <token_or_id>
  Revoke a token

/pairing requests
  List pending access requests

/pairing approve <request_id>
  Approve a pending request

/pairing deny <request_id> [reason]
  Deny a pending request

*Examples:*
/pairing generate 24h 5 user --auto
/pairing generate 7d unlimited user --ws team-a --note "Team Alpha"
/pairing generate never 1 admin --note "Backup admin"
`
}

func (a *Assistant) pairingGenerateCommand(args []string, createdBy string) string {
	opts := TokenOptions{
		Role:        TokenRoleUser,
		MaxUses:     0, // unlimited
		AutoApprove: false,
	}

	// Parse positional arguments.
	for i := 0; i < len(args) && !strings.HasPrefix(args[i], "--"); i++ {
		arg := strings.ToLower(args[i])

		// Parse expiration.
		if arg == "never" || strings.HasSuffix(arg, "h") || strings.HasSuffix(arg, "d") {
			dur, err := parseDuration(arg)
			if err != nil {
				return fmt.Sprintf("Invalid duration: %s", arg)
			}
			opts.ExpiresIn = dur
			continue
		}

		// Parse max uses.
		if arg == "unlimited" || arg == "0" {
			opts.MaxUses = 0
			continue
		}
		if maxUses, err := parseInt(arg); err == nil && maxUses > 0 {
			opts.MaxUses = maxUses
			continue
		}

		// Parse role.
		if arg == "user" || arg == "admin" {
			opts.Role = TokenRole(arg)
			continue
		}
	}

	// Parse flag options.
	for i := 0; i < len(args); i++ {
		if args[i] == "--auto" {
			opts.AutoApprove = true
		}
		if args[i] == "--ws" && i+1 < len(args) {
			opts.WorkspaceID = args[i+1]
			i++
		}
		if args[i] == "--note" && i+1 < len(args) {
			opts.Note = args[i+1]
			i++
		}
	}

	token, err := a.pairingMgr.GenerateToken(createdBy, opts)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	var expires string
	if token.ExpiresAt != nil {
		expires = token.ExpiresAt.Format("2006-01-02 15:04")
	} else {
		expires = "never"
	}

	maxUses := "unlimited"
	if token.MaxUses > 0 {
		maxUses = fmt.Sprintf("%d", token.MaxUses)
	}

	var workspace string
	if token.WorkspaceID != "" {
		workspace = fmt.Sprintf("\nWorkspace: %s", token.WorkspaceID)
	}

	var note string
	if token.Note != "" {
		note = fmt.Sprintf("\nNote: %s", token.Note)
	}

	var b strings.Builder
	b.WriteString("*Pairing Token Generated*\n\n")
	b.WriteString(fmt.Sprintf("Token: `%s`\n", token.Token))
	b.WriteString(fmt.Sprintf("Role: %s\n", token.Role))
	b.WriteString(fmt.Sprintf("Expires: %s\n", expires))
	b.WriteString(fmt.Sprintf("Max Uses: %s\n", maxUses))
	b.WriteString(fmt.Sprintf("Auto-Approve: %v%s%s\n", token.AutoApprove, workspace, note))
	b.WriteString("\nShare this token with the user. They can send it to the bot to request access.\n")
	b.WriteString("If auto-approve is off, you must run /pairing approve to grant access.")

	return b.String()
}

func (a *Assistant) pairingListCommand(args []string) string {
	includeRevoked := containsFlag(args, "--all")

	tokens, err := a.pairingMgr.ListTokens(includeRevoked)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	if len(tokens) == 0 {
		return "No pairing tokens found."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Pairing Tokens (%d):*\n\n", len(tokens)))

	for _, t := range tokens {
		status := "active"
		if t.Revoked {
			status = "revoked"
		} else if t.IsExpired() {
			status = "expired"
		} else if t.MaxUses > 0 && t.UseCount >= t.MaxUses {
			status = "exhausted"
		}

		uses := fmt.Sprintf("%d", t.UseCount)
		if t.MaxUses > 0 {
			uses = fmt.Sprintf("%d/%d", t.UseCount, t.MaxUses)
		}

		auto := ""
		if t.AutoApprove {
			auto = " [auto]"
		}

		b.WriteString(fmt.Sprintf("• `%s...` %s%s\n", t.Token[:12], status, auto))
		b.WriteString(fmt.Sprintf("  Role: %s | Uses: %s | By: %s\n", t.Role, uses, t.CreatedBy))
	}

	b.WriteString("\nUse /pairing info <token> for details.")
	return b.String()
}

func (a *Assistant) pairingInfoCommand(args []string) string {
	if len(args) < 1 {
		return "Usage: /pairing info <token_or_id>"
	}

	token, err := a.pairingMgr.GetTokenByIDOrPrefix(args[0])
	if err != nil {
		return fmt.Sprintf("Token not found: %v", err)
	}

	var expires string
	if token.ExpiresAt != nil {
		expires = token.ExpiresAt.Format("2006-01-02 15:04")
	} else {
		expires = "never"
	}

	maxUses := "unlimited"
	if token.MaxUses > 0 {
		maxUses = fmt.Sprintf("%d", token.MaxUses)
	}

	status := "active"
	if token.Revoked {
		status = fmt.Sprintf("revoked by %s", token.RevokedBy)
	} else if token.IsExpired() {
		status = "expired"
	}

	var workspace string
	if token.WorkspaceID != "" {
		workspace = fmt.Sprintf("\nWorkspace: %s", token.WorkspaceID)
	}

	var b strings.Builder
	b.WriteString("*Pairing Token*\n\n")
	b.WriteString(fmt.Sprintf("ID: %s\n", token.ID))
	b.WriteString(fmt.Sprintf("Token: `%s`\n", token.Token))
	b.WriteString(fmt.Sprintf("Status: %s\n", status))
	b.WriteString(fmt.Sprintf("Role: %s\n", token.Role))
	b.WriteString(fmt.Sprintf("Expires: %s\n", expires))
	b.WriteString(fmt.Sprintf("Uses: %d/%s\n", token.UseCount, maxUses))
	b.WriteString(fmt.Sprintf("Auto-Approve: %v\n", token.AutoApprove))
	b.WriteString(fmt.Sprintf("Created By: %s\n", token.CreatedBy))
	b.WriteString(fmt.Sprintf("Created At: %s%s\n", token.CreatedAt.Format("2006-01-02 15:04"), workspace))
	b.WriteString(fmt.Sprintf("Note: %s", token.Note))

	return b.String()
}

func (a *Assistant) pairingRevokeCommand(args []string, revokedBy string) string {
	if len(args) < 1 {
		return "Usage: /pairing revoke <token_or_id>"
	}

	token, err := a.pairingMgr.GetTokenByIDOrPrefix(args[0])
	if err != nil {
		return fmt.Sprintf("Token not found: %v", err)
	}

	if err := a.pairingMgr.RevokeToken(token.ID, revokedBy); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Token `%s...` has been revoked.", token.Token[:12])
}

func (a *Assistant) pairingRequestsCommand() string {
	requests, err := a.pairingMgr.ListPendingRequests()
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	if len(requests) == 0 {
		return "No pending access requests."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Pending Requests (%d):*\n\n", len(requests)))

	for _, r := range requests {
		b.WriteString(fmt.Sprintf("• ID: `%s`\n", r.ID[:8]))
		b.WriteString(fmt.Sprintf("  User: %s\n", r.UserJID))
		if r.UserName != "" {
			b.WriteString(fmt.Sprintf("  Name: %s\n", r.UserName))
		}
		b.WriteString(fmt.Sprintf("  Role: %s | Created: %s\n", r.TokenRole, r.CreatedAt.Format("2006-01-02 15:04")))
		if r.TokenNote != "" {
			b.WriteString(fmt.Sprintf("  Token Note: %s\n", r.TokenNote))
		}
		b.WriteString("\n")
	}

	b.WriteString("Use /pairing approve <id> or /pairing deny <id> to respond.")
	return b.String()
}

func (a *Assistant) pairingApproveCommand(args []string, approvedBy string) string {
	if len(args) < 1 {
		return "Usage: /pairing approve <request_id>"
	}

	if err := a.pairingMgr.ApproveRequest(args[0], approvedBy); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return "Request approved. User has been granted access."
}

func (a *Assistant) pairingDenyCommand(args []string, deniedBy string) string {
	if len(args) < 1 {
		return "Usage: /pairing deny <request_id> [reason]"
	}

	reason := ""
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	if err := a.pairingMgr.DenyRequest(args[0], deniedBy, reason); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return "Request denied."
}

// parseDuration parses a duration string like "1h", "24h", "7d", "30d", or "never".
func parseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(s)
	if s == "never" {
		return 0, nil
	}

	// Parse days.
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := parseInt(daysStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// Parse hours.
	if strings.HasSuffix(s, "h") {
		hoursStr := strings.TrimSuffix(s, "h")
		hours, err := parseInt(hoursStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(hours) * time.Hour, nil
	}

	return 0, fmt.Errorf("invalid duration format: %s", s)
}

// parseInt parses a string to int.
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// --- Vault Commands ---

// vaultCommand handles the /vault command for secret management.
func (a *Assistant) vaultCommand(args []string) string {
	if a.vault == nil {
		return "Vault not available."
	}

	if len(args) == 0 {
		return a.vaultHelp()
	}

	sub := strings.ToLower(args[0])
	subArgs := args[1:]

	switch sub {
	case "list", "ls":
		return a.vaultListCommand()

	case "set", "add", "update":
		return a.vaultSetCommand(subArgs)

	case "get", "show":
		return a.vaultGetCommand(subArgs)

	case "delete", "remove", "rm":
		return a.vaultDeleteCommand(subArgs)

	case "unlock":
		return a.vaultUnlockCommand()

	case "lock":
		return a.vaultLockCommand()

	case "status":
		return a.vaultStatusCommand()

	default:
		return a.vaultHelp()
	}
}

func (a *Assistant) vaultHelp() string {
	return `*Vault Commands*

/vault list
  List all secret names (values are hidden)

/vault set <key> <value>
  Add or update a secret
  Example: /vault set OPENAI_API_KEY sk-xxx

/vault get <key>
  Show a secret value (use with caution!)

/vault delete <key>
  Remove a secret

/vault unlock
  Unlock the vault (prompts for password if needed)

/vault lock
  Lock the vault (clears key from memory)

/vault status
  Show vault status (locked/unlocked, count)

*Note:* Vault secrets are automatically injected as environment variables
and take precedence over .env files. Use /reload vault to refresh.
`
}

func (a *Assistant) vaultListCommand() string {
	if !a.vault.IsUnlocked() {
		return "Vault is locked. Use /vault unlock first."
	}

	keys := a.vault.List()
	if len(keys) == 0 {
		return "No secrets stored in vault."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Vault Secrets (%d):*\n\n", len(keys)))
	for _, key := range keys {
		b.WriteString(fmt.Sprintf("• `%s`\n", key))
	}
	b.WriteString("\nUse /vault get <key> to show a value.")
	return b.String()
}

func (a *Assistant) vaultSetCommand(args []string) string {
	if !a.vault.IsUnlocked() {
		return "Vault is locked. Use /vault unlock first."
	}

	if len(args) < 2 {
		return "Usage: /vault set <key> <value>"
	}

	key := strings.ToUpper(args[0])
	value := strings.Join(args[1:], " ")

	if err := a.vault.Set(key, value); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	// Re-inject to update env vars.
	a.InjectVaultEnvVars()

	return fmt.Sprintf("Secret `%s` saved. Environment variable updated.", key)
}

func (a *Assistant) vaultGetCommand(args []string) string {
	if !a.vault.IsUnlocked() {
		return "Vault is locked. Use /vault unlock first."
	}

	if len(args) < 1 {
		return "Usage: /vault get <key>"
	}

	key := strings.ToUpper(args[0])
	value, err := a.vault.Get(key)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	if value == "" {
		return fmt.Sprintf("Secret `%s` not found.", key)
	}

	// Mask most of the value for security.
	if len(value) > 8 {
		masked := value[:4] + "****" + value[len(value)-4:]
		return fmt.Sprintf("`%s` = `%s`", key, masked)
	}
	return fmt.Sprintf("`%s` = `%s`", key, value)
}

func (a *Assistant) vaultDeleteCommand(args []string) string {
	if !a.vault.IsUnlocked() {
		return "Vault is locked. Use /vault unlock first."
	}

	if len(args) < 1 {
		return "Usage: /vault delete <key>"
	}

	key := strings.ToUpper(args[0])

	if err := a.vault.Delete(key); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Secret `%s` deleted.", key)
}

func (a *Assistant) vaultUnlockCommand() string {
	if a.vault.IsUnlocked() {
		return "Vault is already unlocked."
	}

	if !a.vault.Exists() {
		return "Vault does not exist. Create it with a master password first (via CLI or setup wizard)."
	}

	return "Vault requires password. Use CLI to unlock: devclaw vault unlock"
}

func (a *Assistant) vaultLockCommand() string {
	if !a.vault.IsUnlocked() {
		return "Vault is already locked."
	}

	a.vault.Lock()
	return "Vault locked. Secrets cleared from memory."
}

func (a *Assistant) vaultStatusCommand() string {
	var b strings.Builder
	b.WriteString("*Vault Status*\n\n")

	if a.vault == nil {
		b.WriteString("Status: Not available")
		return b.String()
	}

	if a.vault.Exists() {
		b.WriteString("File: " + a.vault.Path() + "\n")
	} else {
		b.WriteString("File: Not created\n")
	}

	if a.vault.IsUnlocked() {
		b.WriteString("Status: Unlocked\n")
		keys := a.vault.List()
		b.WriteString(fmt.Sprintf("Secrets: %d", len(keys)))
	} else {
		b.WriteString("Status: Locked")
	}

	return b.String()
}

// hooksCommand handles the /hooks command for hook management.
func (a *Assistant) hooksCommand(args []string) string {
	if a.hookMgr == nil {
		return "Hooks system not available."
	}

	if len(args) == 0 {
		return a.hooksHelp()
	}

	sub := strings.ToLower(args[0])
	subArgs := args[1:]

	switch sub {
	case "list", "ls":
		return a.hooksListCommand()

	case "enable":
		return a.hooksEnableCommand(subArgs, true)

	case "disable":
		return a.hooksEnableCommand(subArgs, false)

	case "events":
		return a.hooksEventsCommand()

	default:
		return a.hooksHelp()
	}
}

func (a *Assistant) hooksHelp() string {
	return `*Hooks Commands*

/hooks list
  List all registered hooks

/hooks events
  List all available hook events

/hooks enable <name>
  Enable a hook by name

/hooks disable <name>
  Disable a hook by name

*About Hooks*
Hooks are event handlers that run at specific points in the bot lifecycle.
They can be used for logging, auditing, notifications, and custom behavior.

Webhooks can be configured in config.yaml to send events to external URLs.
`
}

func (a *Assistant) hooksListCommand() string {
	hooks := a.hookMgr.ListDetailed()
	if len(hooks) == 0 {
		return "No hooks registered."
	}

	var b strings.Builder
	b.WriteString("*Registered Hooks*\n\n")

	for _, h := range hooks {
		status := "enabled"
		if !h.Enabled {
			status = "disabled"
		}
		b.WriteString(fmt.Sprintf("`%s` (%s)\n", h.Name, status))
		b.WriteString(fmt.Sprintf("  Events: %s\n", strings.Join(hookEventsToStrings(h.Events), ", ")))
		if h.Description != "" {
			b.WriteString(fmt.Sprintf("  Description: %s\n", h.Description))
		}
		b.WriteString(fmt.Sprintf("  Source: %s, Priority: %d\n\n", h.Source, h.Priority))
	}

	return b.String()
}

func (a *Assistant) hooksEventsCommand() string {
	var b strings.Builder
	b.WriteString("*Available Hook Events*\n\n")

	for _, ev := range AllHookEvents {
		b.WriteString(fmt.Sprintf("• `%s` - %s\n", ev, HookEventDescription(ev)))
	}

	return b.String()
}

func (a *Assistant) hooksEnableCommand(args []string, enable bool) string {
	if len(args) < 1 {
		verb := "enable"
		if !enable {
			verb = "disable"
		}
		return fmt.Sprintf("Usage: /hooks %s <name>", verb)
	}

	name := args[0]

	if a.hookMgr.SetEnabled(name, enable) {
		action := "enabled"
		if !enable {
			action = "disabled"
		}
		return fmt.Sprintf("Hook `%s` %s.", name, action)
	}

	return fmt.Sprintf("Hook `%s` not found.", name)
}

func hookEventsToStrings(events []HookEvent) []string {
	result := make([]string, len(events))
	for i, ev := range events {
		result[i] = string(ev)
	}
	return result
}


package webui

import (
	"context"
	"errors"
	"net/http"

	"github.com/jholhewres/devclaw/pkg/devclaw/auth/profiles"
	"github.com/jholhewres/devclaw/pkg/devclaw/plugins"
)

// WhatsAppQREvent mirrors whatsapp.QREvent without importing the channel package.
type WhatsAppQREvent struct {
	Type        string `json:"type"` // "code", "success", "timeout", "error", "refresh"
	Code        string `json:"code,omitempty"`
	Message     string `json:"message"`
	ExpiresAt   string `json:"expires_at,omitempty"`   // ISO timestamp
	SecondsLeft int    `json:"seconds_left,omitempty"` // Seconds until QR expires
}

// WhatsAppStatus holds the current WhatsApp connection state for the UI.
type WhatsAppStatus struct {
	Connected         bool   `json:"connected"`
	State             string `json:"state"` // "disconnected", "connecting", "connected", "waiting_qr", etc.
	NeedsQR           bool   `json:"needs_qr"`
	Phone             string `json:"phone,omitempty"`
	Platform          string `json:"platform,omitempty"`
	ErrorCount        int    `json:"error_count"`
	ReconnectAttempts int    `json:"reconnect_attempts"`
	Message           string `json:"message,omitempty"` // Human-readable status message
}

// WhatsAppAccessConfig holds the WhatsApp access control configuration.
type WhatsAppAccessConfig struct {
	DefaultPolicy  string   `json:"default_policy"`
	Owners         []string `json:"owners"`
	Admins         []string `json:"admins"`
	AllowedUsers   []string `json:"allowed_users"`
	BlockedUsers   []string `json:"blocked_users"`
	AllowedGroups  []string `json:"allowed_groups"`
	BlockedGroups  []string `json:"blocked_groups"`
	PendingMessage string   `json:"pending_message"`
}

// WhatsAppGroupPolicy holds a single group's policy configuration.
type WhatsAppGroupPolicy struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Policy       string   `json:"policy"`
	Policies     []string `json:"policies,omitempty"`
	Keywords     []string `json:"keywords,omitempty"`
	AllowedUsers []string `json:"allowed_users,omitempty"`
	Workspace    string   `json:"workspace,omitempty"`
}

// WhatsAppGroupPolicies holds all group policies.
type WhatsAppGroupPolicies struct {
	DefaultPolicy string                `json:"default_policy"`
	Groups        []WhatsAppGroupPolicy `json:"groups"`
}

// WhatsAppJoinedGroup represents a WhatsApp group the bot is part of.
type WhatsAppJoinedGroup struct {
	JID  string `json:"jid"`
	Name string `json:"name"`
}

// ChannelInstanceInfo describes a single channel instance for the UI.
type ChannelInstanceInfo struct {
	Type       string `json:"type"`        // "whatsapp", "telegram", etc.
	InstanceID string `json:"instance_id"` // "" for default, "business" for named
	FullName   string `json:"full_name"`   // "whatsapp" or "whatsapp:business"
	Label      string `json:"label"`       // Display name: "WhatsApp" or "WhatsApp (business)"
	Connected  bool   `json:"connected"`
	Configured bool   `json:"configured"`
	NeedsQR    bool   `json:"needs_qr"`
	ErrorCount int    `json:"error_count"`
}

// AssistantAdapter wraps a generic set of callbacks to satisfy the AssistantAPI
// interface. This avoids a direct import cycle between copilot and webui.
type AssistantAdapter struct {
	GetConfigMapFn       func() map[string]any
	UpdateConfigMapFn    func(updates map[string]any) error
	ListSessionsFn       func() []SessionInfo
	GetSessionMessagesFn func(sessionID string) []MessageInfo
	GetUsageGlobalFn     func() UsageInfo
	GetChannelHealthFn   func() []ChannelHealthInfo
	GetSchedulerJobsFn   func() []JobInfo
	ToggleJobFn          func(id string, enabled bool) error
	RemoveJobFn          func(id string) error
	ListSkillsFn         func() []SkillInfo
	ToggleSkillFn        func(name string, enabled bool) error
	RemoveSkillFn        func(name string) error
	ReloadSkillsFn       func() error
	SendChatMessageFn    func(sessionID, content string) (string, error)
	StartChatStreamFn    func(ctx context.Context, sessionID, content string) (*RunHandle, error)
	AbortRunFn           func(sessionID string) bool
	DeleteSessionFn      func(sessionID string) error

	// WhatsApp QR support (default instance)
	GetWhatsAppStatusFn   func() WhatsAppStatus
	SubscribeWhatsAppQRFn func() (chan WhatsAppQREvent, func())
	RequestWhatsAppQRFn   func() error
	DisconnectWhatsAppFn  func() error

	// Instance-aware WhatsApp operations
	GetWhatsAppStatusByInstanceFn   func(instanceID string) WhatsAppStatus
	SubscribeWhatsAppQRByInstanceFn func(instanceID string) (chan WhatsAppQREvent, func())
	RequestWhatsAppQRByInstanceFn   func(instanceID string) error
	DisconnectWhatsAppByInstanceFn  func(instanceID string) error

	// Channel instance management (all channel types)
	ListChannelInstancesFn  func(channelType string) []ChannelInstanceInfo
	CreateChannelInstanceFn func(channelType, instanceID string, config map[string]any) error
	DeleteChannelInstanceFn func(channelType, instanceID string) error

	// WhatsApp Access & Groups
	GetWhatsAppAccessConfigFn           func() WhatsAppAccessConfig
	GrantWhatsAppUserAccessFn           func(jid, level string) error
	RevokeWhatsAppUserAccessFn          func(jid string) error
	BlockWhatsAppUserFn                 func(jid string) error
	UnblockWhatsAppUserFn               func(jid string) error
	GetWhatsAppGroupPoliciesFn          func() WhatsAppGroupPolicies
	SetWhatsAppGroupPolicyFn            func(jid string, policy any) error
	UpdateWhatsAppGroupDefaultPolicyFn  func(policy string) error
	UpdateWhatsAppAccessDefaultPolicyFn func(policy string) error
	GetWhatsAppJoinedGroupsFn           func() ([]WhatsAppJoinedGroup, error)
	GetWhatsAppConfigFn                 func() map[string]any
	UpdateWhatsAppConfigFn              func(config map[string]any) error

	// Telegram
	GetTelegramConfigFn    func() TelegramConfig
	UpdateTelegramConfigFn func(config map[string]any) error
	ConnectTelegramFn      func(token string) error
	DisconnectTelegramFn   func() error

	// Security: Audit Log
	GetAuditLogFn   func(limit int) []AuditEntry
	GetAuditCountFn func() int

	// Security: Tool Guard
	GetToolGuardStatusFn func() ToolGuardStatus
	UpdateToolGuardFn    func(update ToolGuardStatus) error

	// Security: Vault (read-only, no values)
	GetVaultStatusFn func() VaultStatus

	// Security: Overview
	GetSecurityStatusFn func() SecurityStatus

	// Domain & Network
	GetDomainConfigFn    func() DomainConfigInfo
	UpdateDomainConfigFn func(update DomainConfigUpdate) error

	// Webhooks
	ListWebhooksFn          func() []WebhookInfo
	CreateWebhookFn         func(url string, events []string) (WebhookInfo, error)
	DeleteWebhookFn         func(id string) error
	ToggleWebhookFn         func(id string, active bool) error
	GetValidWebhookEventsFn func() []string

	// Hooks (lifecycle)
	ListHooksFn      func() []HookInfo
	ToggleHookFn     func(name string, enabled bool) error
	UnregisterHookFn func(name string) error
	GetHookEventsFn  func() []HookEventInfo

	// MCP Servers
	ListMCPServersFn  func() []MCPServerInfo
	CreateMCPServerFn func(name, command string, args []string, env map[string]string) error
	UpdateMCPServerFn func(name string, enabled bool) error
	DeleteMCPServerFn func(name string) error
	StartMCPServerFn  func(name string) error
	StopMCPServerFn   func(name string) error

	// Database
	GetDatabaseStatusFn func() DatabaseStatusInfo

	// Settings: Tool Profiles
	ListToolProfilesFn  func() []ToolProfileInfo
	CreateToolProfileFn func(profile ToolProfileDef) error
	UpdateToolProfileFn func(name string, profile ToolProfileDef) error
	DeleteToolProfileFn func(name string) error

	// Auth Profiles
	GetProfileManagerFn func() profiles.ProfileManager

	// Plugins
	ListPluginsFn     func() []PluginInfoAPI
	GetPluginInfoFn   func(id string) *PluginInfoAPI
	ConfigurePluginFn func(id string, updates map[string]any) error
	TogglePluginFn    func(id string, enabled bool) error
	InstallPluginFn   func(source string) (*plugins.PluginInstallResult, error)
	RemovePluginFn    func(name string) error

	// Models
	ListModelsFn func() []ModelInfo

	// Agents
	ListAgentsFn      func() []AgentInfoAPI
	CreateAgentFn     func(req CreateAgentRequest) (string, error)
	GetAgentFn        func(id string) (*AgentInfoAPI, error)
	UpdateAgentFn     func(id string, req UpdateAgentRequest) error
	DeleteAgentFn     func(id string) error
	SetDefaultAgentFn func(id string) error
	ToggleAgentFn     func(id string, active bool) error

	// Agent Files
	ListAgentFilesFn  func(id string) (*AgentFilesResponse, error)
	UpdateAgentFileFn func(id, filename, content string) error
}

// ModelInfo contains model info for API responses.
type ModelInfo struct {
	ID       string `json:"id"`       // e.g. "claude-sonnet-4-20250514"
	Name     string `json:"name"`     // e.g. "Claude Sonnet 4"
	Provider string `json:"provider"` // e.g. "anthropic"
}

// ToolProfileInfo contains profile info for API responses.
type ToolProfileInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny"`
	Builtin     bool     `json:"builtin"`
}

// ToolProfileDef defines a tool profile for creation/update.
type ToolProfileDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny"`
}

func (a *AssistantAdapter) GetConfigMap() map[string]any {
	if a.GetConfigMapFn != nil {
		return a.GetConfigMapFn()
	}
	return nil
}

func (a *AssistantAdapter) UpdateConfigMap(updates map[string]any) error {
	if a.UpdateConfigMapFn != nil {
		return a.UpdateConfigMapFn(updates)
	}
	return errors.New("config update not available")
}

func (a *AssistantAdapter) ListSessions() []SessionInfo {
	if a.ListSessionsFn != nil {
		return a.ListSessionsFn()
	}
	return nil
}

func (a *AssistantAdapter) GetSessionMessages(sessionID string) []MessageInfo {
	if a.GetSessionMessagesFn != nil {
		return a.GetSessionMessagesFn(sessionID)
	}
	return nil
}

func (a *AssistantAdapter) GetUsageGlobal() UsageInfo {
	if a.GetUsageGlobalFn != nil {
		return a.GetUsageGlobalFn()
	}
	return UsageInfo{}
}

func (a *AssistantAdapter) GetChannelHealth() []ChannelHealthInfo {
	if a.GetChannelHealthFn != nil {
		return a.GetChannelHealthFn()
	}
	return nil
}

func (a *AssistantAdapter) GetSchedulerJobs() []JobInfo {
	if a.GetSchedulerJobsFn != nil {
		return a.GetSchedulerJobsFn()
	}
	return nil
}

func (a *AssistantAdapter) ToggleJob(id string, enabled bool) error {
	if a.ToggleJobFn != nil {
		return a.ToggleJobFn(id, enabled)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) RemoveJob(id string) error {
	if a.RemoveJobFn != nil {
		return a.RemoveJobFn(id)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) ListSkills() []SkillInfo {
	if a.ListSkillsFn != nil {
		return a.ListSkillsFn()
	}
	return nil
}

func (a *AssistantAdapter) ToggleSkill(name string, enabled bool) error {
	if a.ToggleSkillFn != nil {
		return a.ToggleSkillFn(name, enabled)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) RemoveSkill(name string) error {
	if a.RemoveSkillFn != nil {
		return a.RemoveSkillFn(name)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) ReloadSkills() error {
	if a.ReloadSkillsFn != nil {
		return a.ReloadSkillsFn()
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) SendChatMessage(sessionID, content string) (string, error) {
	if a.SendChatMessageFn != nil {
		return a.SendChatMessageFn(sessionID, content)
	}
	return "", nil
}

func (a *AssistantAdapter) StartChatStream(ctx context.Context, sessionID, content string) (*RunHandle, error) {
	if a.StartChatStreamFn != nil {
		return a.StartChatStreamFn(ctx, sessionID, content)
	}
	return nil, errors.New("streaming not available")
}

func (a *AssistantAdapter) AbortRun(sessionID string) bool {
	if a.AbortRunFn != nil {
		return a.AbortRunFn(sessionID)
	}
	return false
}

func (a *AssistantAdapter) DeleteSession(sessionID string) error {
	if a.DeleteSessionFn != nil {
		return a.DeleteSessionFn(sessionID)
	}
	return errors.New("not implemented")
}

// ── WhatsApp Access & Groups ──

func (a *AssistantAdapter) GetWhatsAppAccessConfig() WhatsAppAccessConfig {
	if a.GetWhatsAppAccessConfigFn != nil {
		return a.GetWhatsAppAccessConfigFn()
	}
	return WhatsAppAccessConfig{}
}

func (a *AssistantAdapter) GrantWhatsAppUserAccess(jid, level string) error {
	if a.GrantWhatsAppUserAccessFn != nil {
		return a.GrantWhatsAppUserAccessFn(jid, level)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) RevokeWhatsAppUserAccess(jid string) error {
	if a.RevokeWhatsAppUserAccessFn != nil {
		return a.RevokeWhatsAppUserAccessFn(jid)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) BlockWhatsAppUser(jid string) error {
	if a.BlockWhatsAppUserFn != nil {
		return a.BlockWhatsAppUserFn(jid)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) UnblockWhatsAppUser(jid string) error {
	if a.UnblockWhatsAppUserFn != nil {
		return a.UnblockWhatsAppUserFn(jid)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) GetWhatsAppGroupPolicies() WhatsAppGroupPolicies {
	if a.GetWhatsAppGroupPoliciesFn != nil {
		return a.GetWhatsAppGroupPoliciesFn()
	}
	return WhatsAppGroupPolicies{}
}

func (a *AssistantAdapter) SetWhatsAppGroupPolicy(jid string, policy any) error {
	if a.SetWhatsAppGroupPolicyFn != nil {
		return a.SetWhatsAppGroupPolicyFn(jid, policy)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) UpdateWhatsAppAccessDefaultPolicy(policy string) error {
	if a.UpdateWhatsAppAccessDefaultPolicyFn != nil {
		return a.UpdateWhatsAppAccessDefaultPolicyFn(policy)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) UpdateWhatsAppGroupDefaultPolicy(policy string) error {
	if a.UpdateWhatsAppGroupDefaultPolicyFn != nil {
		return a.UpdateWhatsAppGroupDefaultPolicyFn(policy)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) UpdateWhatsAppConfig(config map[string]any) error {
	if a.UpdateWhatsAppConfigFn != nil {
		return a.UpdateWhatsAppConfigFn(config)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) GetWhatsAppConfig() map[string]any {
	if a.GetWhatsAppConfigFn != nil {
		return a.GetWhatsAppConfigFn()
	}
	return map[string]any{}
}

func (a *AssistantAdapter) GetWhatsAppJoinedGroups() ([]WhatsAppJoinedGroup, error) {
	if a.GetWhatsAppJoinedGroupsFn != nil {
		return a.GetWhatsAppJoinedGroupsFn()
	}
	return nil, errors.New("not implemented")
}

func (a *AssistantAdapter) DisconnectWhatsApp() error {
	if a.DisconnectWhatsAppFn != nil {
		return a.DisconnectWhatsAppFn()
	}
	return errors.New("not implemented")
}

// ── Telegram ──

func (a *AssistantAdapter) GetTelegramConfig() TelegramConfig {
	if a.GetTelegramConfigFn != nil {
		return a.GetTelegramConfigFn()
	}
	return TelegramConfig{}
}

func (a *AssistantAdapter) UpdateTelegramConfig(config map[string]any) error {
	if a.UpdateTelegramConfigFn != nil {
		return a.UpdateTelegramConfigFn(config)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) ConnectTelegram(token string) error {
	if a.ConnectTelegramFn != nil {
		return a.ConnectTelegramFn(token)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) DisconnectTelegram() error {
	if a.DisconnectTelegramFn != nil {
		return a.DisconnectTelegramFn()
	}
	return errors.New("not implemented")
}

// ── Security ──

func (a *AssistantAdapter) GetAuditLog(limit int) []AuditEntry {
	if a.GetAuditLogFn != nil {
		return a.GetAuditLogFn(limit)
	}
	return nil
}

func (a *AssistantAdapter) GetAuditCount() int {
	if a.GetAuditCountFn != nil {
		return a.GetAuditCountFn()
	}
	return 0
}

func (a *AssistantAdapter) GetToolGuardStatus() ToolGuardStatus {
	if a.GetToolGuardStatusFn != nil {
		return a.GetToolGuardStatusFn()
	}
	return ToolGuardStatus{}
}

func (a *AssistantAdapter) UpdateToolGuard(update ToolGuardStatus) error {
	if a.UpdateToolGuardFn != nil {
		return a.UpdateToolGuardFn(update)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) GetVaultStatus() VaultStatus {
	if a.GetVaultStatusFn != nil {
		return a.GetVaultStatusFn()
	}
	return VaultStatus{}
}

func (a *AssistantAdapter) GetSecurityStatus() SecurityStatus {
	if a.GetSecurityStatusFn != nil {
		return a.GetSecurityStatusFn()
	}
	return SecurityStatus{}
}

// ── Domain & Network ──

func (a *AssistantAdapter) GetDomainConfig() DomainConfigInfo {
	if a.GetDomainConfigFn != nil {
		return a.GetDomainConfigFn()
	}
	return DomainConfigInfo{}
}

func (a *AssistantAdapter) UpdateDomainConfig(update DomainConfigUpdate) error {
	if a.UpdateDomainConfigFn != nil {
		return a.UpdateDomainConfigFn(update)
	}
	return errors.New("not implemented")
}

// ── Webhooks ──

func (a *AssistantAdapter) ListWebhooks() []WebhookInfo {
	if a.ListWebhooksFn != nil {
		return a.ListWebhooksFn()
	}
	return nil
}

func (a *AssistantAdapter) CreateWebhook(url string, events []string) (WebhookInfo, error) {
	if a.CreateWebhookFn != nil {
		return a.CreateWebhookFn(url, events)
	}
	return WebhookInfo{}, errors.New("not implemented")
}

func (a *AssistantAdapter) DeleteWebhook(id string) error {
	if a.DeleteWebhookFn != nil {
		return a.DeleteWebhookFn(id)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) ToggleWebhook(id string, active bool) error {
	if a.ToggleWebhookFn != nil {
		return a.ToggleWebhookFn(id, active)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) GetValidWebhookEvents() []string {
	if a.GetValidWebhookEventsFn != nil {
		return a.GetValidWebhookEventsFn()
	}
	return nil
}

// ── Hooks (Lifecycle) ──

func (a *AssistantAdapter) ListHooks() []HookInfo {
	if a.ListHooksFn != nil {
		return a.ListHooksFn()
	}
	return nil
}

func (a *AssistantAdapter) ToggleHook(name string, enabled bool) error {
	if a.ToggleHookFn != nil {
		return a.ToggleHookFn(name, enabled)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) UnregisterHook(name string) error {
	if a.UnregisterHookFn != nil {
		return a.UnregisterHookFn(name)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) GetHookEvents() []HookEventInfo {
	if a.GetHookEventsFn != nil {
		return a.GetHookEventsFn()
	}
	return nil
}

// ── MCP Servers ──

func (a *AssistantAdapter) ListMCPServers() []MCPServerInfo {
	if a.ListMCPServersFn != nil {
		return a.ListMCPServersFn()
	}
	return nil
}

func (a *AssistantAdapter) CreateMCPServer(name, command string, args []string, env map[string]string) error {
	if a.CreateMCPServerFn != nil {
		return a.CreateMCPServerFn(name, command, args, env)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) UpdateMCPServer(name string, enabled bool) error {
	if a.UpdateMCPServerFn != nil {
		return a.UpdateMCPServerFn(name, enabled)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) DeleteMCPServer(name string) error {
	if a.DeleteMCPServerFn != nil {
		return a.DeleteMCPServerFn(name)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) StartMCPServer(name string) error {
	if a.StartMCPServerFn != nil {
		return a.StartMCPServerFn(name)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) StopMCPServer(name string) error {
	if a.StopMCPServerFn != nil {
		return a.StopMCPServerFn(name)
	}
	return errors.New("not implemented")
}

// ── Database ──

func (a *AssistantAdapter) GetDatabaseStatus() DatabaseStatusInfo {
	if a.GetDatabaseStatusFn != nil {
		return a.GetDatabaseStatusFn()
	}
	return DatabaseStatusInfo{}
}

// ── Settings: Tool Profiles ──

func (a *AssistantAdapter) ListToolProfiles() []ToolProfileInfo {
	if a.ListToolProfilesFn != nil {
		return a.ListToolProfilesFn()
	}
	return nil
}

func (a *AssistantAdapter) GetToolGroups() map[string][]string {
	// Tool groups are defined in copilot package, return static list
	return map[string][]string{
		"group:memory":    {"memory_save", "memory_search", "memory_list", "memory_index"},
		"group:web":       {"web_search", "web_fetch"},
		"group:fs":        {"read_file", "write_file", "edit_file", "list_files", "search_files", "glob_files"},
		"group:runtime":   {"bash", "exec", "ssh", "scp", "set_env"},
		"group:subagents": {"spawn_subagent", "list_subagents", "wait_subagent", "stop_subagent"},
		"group:skills":    {"skill_init", "skill_edit", "skill_add_script", "skill_list", "skill_test", "skill_install", "skill_defaults_list", "skill_defaults_install", "skill_remove"},
		"group:scheduler": {"scheduler_add", "scheduler_list", "scheduler_remove", "scheduler_search"},
		"group:vault":     {"vault_status", "vault_save", "vault_get", "vault_list", "vault_delete"},
		"group:sessions":  {"sessions"},
		"group:daemon":    {"daemon"},
		"group:media":     {"describe_image", "transcribe_audio", "image-gen_generate_image", "send_media"},
		"group:browser":   {"browser_open", "browser_search", "browser_screenshot", "browser_extract"},
		"group:skill_db":  {"skill_db_query", "skill_db_index"},
	}
}

func (a *AssistantAdapter) CreateToolProfile(profile ToolProfileDef) error {
	if a.CreateToolProfileFn != nil {
		return a.CreateToolProfileFn(profile)
	}
	return errors.New("tool profile creation not available")
}

func (a *AssistantAdapter) UpdateToolProfile(profile ToolProfileDef) error {
	if a.UpdateToolProfileFn != nil {
		return a.UpdateToolProfileFn(profile.Name, profile)
	}
	return errors.New("tool profile update not available")
}

func (a *AssistantAdapter) DeleteToolProfile(name string) error {
	if a.DeleteToolProfileFn != nil {
		return a.DeleteToolProfileFn(name)
	}
	return errors.New("tool profile deletion not available")
}

// ── Plugins ──

func (a *AssistantAdapter) ListPlugins() []PluginInfoAPI {
	if a.ListPluginsFn != nil {
		return a.ListPluginsFn()
	}
	return nil
}

func (a *AssistantAdapter) GetPluginInfo(id string) *PluginInfoAPI {
	if a.GetPluginInfoFn != nil {
		return a.GetPluginInfoFn(id)
	}
	return nil
}

func (a *AssistantAdapter) ConfigurePlugin(id string, updates map[string]any) error {
	if a.ConfigurePluginFn != nil {
		return a.ConfigurePluginFn(id, updates)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) TogglePlugin(id string, enabled bool) error {
	if a.TogglePluginFn != nil {
		return a.TogglePluginFn(id, enabled)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) InstallPlugin(source string) (*plugins.PluginInstallResult, error) {
	if a.InstallPluginFn != nil {
		return a.InstallPluginFn(source)
	}
	return nil, errors.New("not implemented")
}

func (a *AssistantAdapter) RemovePlugin(name string) error {
	if a.RemovePluginFn != nil {
		return a.RemovePluginFn(name)
	}
	return errors.New("not implemented")
}

// ── Models ──

func (a *AssistantAdapter) ListModels() []ModelInfo {
	if a.ListModelsFn != nil {
		return a.ListModelsFn()
	}
	return nil
}

// ── Agents ──

func (a *AssistantAdapter) ListAgents() []AgentInfoAPI {
	if a.ListAgentsFn != nil {
		return a.ListAgentsFn()
	}
	return nil
}

func (a *AssistantAdapter) CreateAgent(req CreateAgentRequest) (string, error) {
	if a.CreateAgentFn != nil {
		return a.CreateAgentFn(req)
	}
	return "", errors.New("not implemented")
}

func (a *AssistantAdapter) GetAgent(id string) (*AgentInfoAPI, error) {
	if a.GetAgentFn != nil {
		return a.GetAgentFn(id)
	}
	return nil, errors.New("not implemented")
}

func (a *AssistantAdapter) UpdateAgent(id string, req UpdateAgentRequest) error {
	if a.UpdateAgentFn != nil {
		return a.UpdateAgentFn(id, req)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) DeleteAgent(id string) error {
	if a.DeleteAgentFn != nil {
		return a.DeleteAgentFn(id)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) SetDefaultAgent(id string) error {
	if a.SetDefaultAgentFn != nil {
		return a.SetDefaultAgentFn(id)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) ToggleAgent(id string, active bool) error {
	if a.ToggleAgentFn != nil {
		return a.ToggleAgentFn(id, active)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) ListAgentFiles(id string) (*AgentFilesResponse, error) {
	if a.ListAgentFilesFn != nil {
		return a.ListAgentFilesFn(id)
	}
	return nil, errors.New("not implemented")
}

func (a *AssistantAdapter) UpdateAgentFile(id, filename, content string) error {
	if a.UpdateAgentFileFn != nil {
		return a.UpdateAgentFileFn(id, filename, content)
	}
	return errors.New("not implemented")
}

// ── Media API Adapter ──

// MediaAdapter wraps a MediaService to implement the MediaAPI interface.
// This avoids importing the media package directly in webui.
type MediaAdapter struct {
	UploadFn func(r *http.Request, sessionID string) (mediaID string, mediaType string, filename string, size int64, err error)
	GetFn    func(mediaID string) ([]byte, string, string, error)
	ListFn   func(sessionID string, mediaType string, limit int) ([]MediaInfo, error)
	DeleteFn func(mediaID string) error
}

// Upload implements MediaAPI.Upload.
func (m *MediaAdapter) Upload(r *http.Request, sessionID string) (string, string, string, int64, error) {
	if m.UploadFn != nil {
		return m.UploadFn(r, sessionID)
	}
	return "", "", "", 0, errors.New("media upload not available")
}

// Get implements MediaAPI.Get.
func (m *MediaAdapter) Get(mediaID string) ([]byte, string, string, error) {
	if m.GetFn != nil {
		return m.GetFn(mediaID)
	}
	return nil, "", "", errors.New("media get not available")
}

// List implements MediaAPI.List.
func (m *MediaAdapter) List(sessionID string, mediaType string, limit int) ([]MediaInfo, error) {
	if m.ListFn != nil {
		return m.ListFn(sessionID, mediaType, limit)
	}
	return nil, errors.New("media list not available")
}

// Delete implements MediaAPI.Delete.
func (m *MediaAdapter) Delete(mediaID string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(mediaID)
	}
	return errors.New("media delete not available")
}

// GetProfileManager returns the auth profile manager for OAuth/API key management.
func (a *AssistantAdapter) GetProfileManager() profiles.ProfileManager {
	if a.GetProfileManagerFn != nil {
		return a.GetProfileManagerFn()
	}
	return nil
}

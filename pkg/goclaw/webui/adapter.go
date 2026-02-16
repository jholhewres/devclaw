package webui

import (
	"context"
	"errors"
)

// WhatsAppQREvent mirrors whatsapp.QREvent without importing the channel package.
type WhatsAppQREvent struct {
	Type    string `json:"type"`    // "code", "success", "timeout", "error"
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// WhatsAppStatus holds the current WhatsApp connection state for the UI.
type WhatsAppStatus struct {
	Connected bool   `json:"connected"`
	NeedsQR   bool   `json:"needs_qr"`
	Phone     string `json:"phone,omitempty"`
}

// AssistantAdapter wraps a generic set of callbacks to satisfy the AssistantAPI
// interface. This avoids a direct import cycle between copilot and webui.
type AssistantAdapter struct {
	GetConfigMapFn       func() map[string]any
	ListSessionsFn       func() []SessionInfo
	GetSessionMessagesFn func(sessionID string) []MessageInfo
	GetUsageGlobalFn     func() UsageInfo
	GetChannelHealthFn   func() []ChannelHealthInfo
	GetSchedulerJobsFn   func() []JobInfo
	ListSkillsFn         func() []SkillInfo
	SendChatMessageFn    func(sessionID, content string) (string, error)
	StartChatStreamFn    func(ctx context.Context, sessionID, content string) (*RunHandle, error)
	AbortRunFn           func(sessionID string) bool
	DeleteSessionFn      func(sessionID string) error

	// WhatsApp QR support
	GetWhatsAppStatusFn   func() WhatsAppStatus
	SubscribeWhatsAppQRFn func() (chan WhatsAppQREvent, func())
	RequestWhatsAppQRFn   func() error

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
}

func (a *AssistantAdapter) GetConfigMap() map[string]any {
	if a.GetConfigMapFn != nil {
		return a.GetConfigMapFn()
	}
	return nil
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

func (a *AssistantAdapter) ListSkills() []SkillInfo {
	if a.ListSkillsFn != nil {
		return a.ListSkillsFn()
	}
	return nil
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

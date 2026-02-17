/** Typed HTTP client for DevClaw API */

const BASE = '/api'

/** Generic fetch wrapper with auth and error handling */
async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = localStorage.getItem('devclaw_token')
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const res = await fetch(`${BASE}${path}`, {
    ...options,
    headers,
  })

  if (!res.ok) {
    const body = await res.text().catch(() => '')
    throw new ApiError(res.status, body || res.statusText)
  }

  if (res.status === 204) return undefined as T

  return res.json()
}

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

/* ── Types ── */

export interface SessionInfo {
  id: string
  channel: string
  chat_id: string
  message_count: number
  last_message_at: string
  created_at: string
}

export interface MessageInfo {
  role: 'user' | 'assistant' | 'tool'
  content: string
  timestamp: string
  tool_name?: string
  tool_input?: string
}

export interface UsageInfo {
  total_input_tokens: number
  total_output_tokens: number
  total_cost: number
  request_count: number
}

export interface ChannelHealth {
  name: string
  connected: boolean
  error_count: number
  last_msg_at: string
}

export interface JobInfo {
  id: string
  schedule: string
  type: string
  command: string
  enabled: boolean
  run_count: number
  last_run_at: string
  last_error: string
}

export interface SkillInfo {
  name: string
  description: string
  enabled: boolean
  tool_count: number
}

export interface WhatsAppStatus {
  connected: boolean
  needs_qr: boolean
  phone?: string
}

export interface DashboardData {
  sessions: SessionInfo[]
  usage: UsageInfo
  channels: ChannelHealth[]
  jobs: JobInfo[]
}

export interface SetupStatus {
  configured: boolean
  current_step: number
}

/* ── Security Types ── */

export interface AuditEntry {
  id: number
  tool: string
  caller: string
  level: string
  allowed: boolean
  args_summary: string
  result_summary: string
  created_at: string
}

export interface AuditResponse {
  entries: AuditEntry[]
  total: number
}

export interface ToolGuardStatus {
  enabled: boolean
  allow_destructive: boolean
  allow_sudo: boolean
  allow_reboot: boolean
  auto_approve: string[]
  require_confirmation: string[]
  protected_paths: string[]
  ssh_allowed_hosts: string[]
  dangerous_commands: string[]
  tool_permissions: Record<string, string>
}

export interface VaultStatus {
  exists: boolean
  unlocked: boolean
  keys: string[]
}

export interface SecurityStatus {
  gateway_auth_configured: boolean
  webui_auth_configured: boolean
  tool_guard_enabled: boolean
  vault_exists: boolean
  vault_unlocked: boolean
  audit_entry_count: number
}

/* ── API Methods ── */

export const api = {
  /* Dashboard */
  dashboard: () => request<DashboardData>('/dashboard'),

  /* Sessions */
  sessions: {
    list: () => request<SessionInfo[]>('/sessions'),
    messages: (id: string) => request<MessageInfo[]>(`/sessions/${id}/messages`),
    delete: (id: string) => request<void>(`/sessions/${id}`, { method: 'DELETE' }),
  },

  /* Chat */
  chat: {
    send: (sessionId: string, content: string) =>
      request<{ run_id: string }>(`/chat/${sessionId}/send`, {
        method: 'POST',
        body: JSON.stringify({ content }),
      }),
    abort: (sessionId: string) =>
      request<void>(`/chat/${sessionId}/abort`, { method: 'POST' }),
    history: (sessionId: string) =>
      request<MessageInfo[]>(`/chat/${sessionId}/history`),
  },

  /* Skills */
  skills: {
    list: () => request<SkillInfo[]>('/skills'),
    toggle: (name: string, enabled: boolean) =>
      request<void>(`/skills/${name}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
    install: (name: string) =>
      request<void>(`/skills/install`, {
        method: 'POST',
        body: JSON.stringify({ name }),
      }),
  },

  /* Channels */
  channels: {
    list: () => request<ChannelHealth[]>('/channels'),
    whatsapp: {
      status: () => request<WhatsAppStatus>('/channels/whatsapp/status'),
      requestQR: () => request<{ status: string; message: string }>('/channels/whatsapp/qr', { method: 'POST' }),
    },
  },

  /* Config */
  config: {
    get: () => request<Record<string, unknown>>('/config'),
    update: (data: Record<string, unknown>) =>
      request<void>('/config', {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
  },

  /* Usage */
  usage: () => request<UsageInfo>('/usage'),

  /* Jobs */
  jobs: {
    list: () => request<JobInfo[]>('/jobs'),
  },

  /* Setup */
  setup: {
    status: () => request<SetupStatus>('/setup/status'),
    step: (step: number, data: Record<string, unknown>) =>
      request<{ next_step: number; done: boolean }>(`/setup/step/${step}`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    testProvider: (provider: string, apiKey: string, model: string, baseUrl?: string) =>
      request<{ success: boolean; error?: string }>('/setup/test-provider', {
        method: 'POST',
        body: JSON.stringify({ provider, api_key: apiKey, model, base_url: baseUrl || '' }),
      }),
    finalize: (data: Record<string, unknown>) =>
      request<{ status: string; message: string }>('/setup/finalize', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
  },

  /* Security */
  security: {
    overview: () => request<SecurityStatus>('/security'),
    audit: (limit = 50) => request<AuditResponse>(`/security/audit?limit=${limit}`),
    toolGuard: {
      get: () => request<ToolGuardStatus>('/security/tool-guard'),
      update: (data: Partial<ToolGuardStatus>) =>
        request<void>('/security/tool-guard', {
          method: 'PUT',
          body: JSON.stringify(data),
        }),
    },
    vault: () => request<VaultStatus>('/security/vault'),
  },

  /* Auth */
  auth: {
    login: (password: string) =>
      request<{ token: string }>('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ password }),
      }),
  },
}

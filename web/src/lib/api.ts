/** Typed HTTP client for DevClaw API */

const BASE = '/api';

/** Generic fetch wrapper with auth and error handling */
async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = localStorage.getItem('devclaw_token');
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${BASE}${path}`, {
    ...options,
    headers,
  });

  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new ApiError(res.status, body || res.statusText);
  }

  if (res.status === 204) return undefined as T;

  return res.json();
}

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

/* ── Types ── */

export interface SessionInfo {
  id: string;
  channel: string;
  chat_id: string;
  title?: string;
  message_count: number;
  last_message_at: string;
  created_at: string;
}

export interface MessageInfo {
  role: 'user' | 'assistant' | 'tool';
  content: string;
  timestamp: string;
  tool_name?: string;
  tool_input?: string;
  is_error?: boolean;
}

export interface UsageInfo {
  total_input_tokens: number;
  total_output_tokens: number;
  total_cost: number;
  request_count: number;
}

export interface ChannelHealth {
  name: string;
  connected: boolean;
  error_count: number;
  last_msg_at: string;
}

export interface JobInfo {
  id: string;
  schedule: string;
  type: string;
  command: string;
  enabled: boolean;
  run_count: number;
  last_run_at: string;
  last_error: string;
}

export interface SkillInfo {
  name: string;
  label?: string;
  label_pt?: string;
  description: string;
  description_pt?: string;
  enabled: boolean;
  tool_count: number;
}

export interface WhatsAppStatus {
  connected: boolean;
  needs_qr: boolean;
  phone?: string;
}

/* WhatsApp Access & Groups */
export interface WhatsAppAccessConfig {
  default_policy: string;
  owners: string[];
  admins: string[];
  allowed_users: string[];
  blocked_users: string[];
  allowed_groups: string[];
  blocked_groups: string[];
  pending_message: string;
}

export interface WhatsAppGroupPolicy {
  id: string;
  name: string;
  policy: string; // Legacy, single policy
  policies?: string[]; // New, multiple policies (OR logic)
  keywords?: string[];
  allowed_users?: string[];
  workspace?: string;
}

export interface WhatsAppGroupPolicies {
  default_policy: string; // Legacy, single default policy
  default_policies?: string[]; // New, multiple default policies (OR logic)
  groups: WhatsAppGroupPolicy[];
}

export interface WhatsAppSettings {
  respond_to_groups: boolean;
  respond_to_dms: boolean;
  auto_read: boolean;
  send_typing: boolean;
}

export interface DashboardData {
  sessions: SessionInfo[];
  usage: UsageInfo;
  channels: ChannelHealth[];
  jobs: JobInfo[];
}

export interface SetupStatus {
  configured: boolean;
  current_step: number;
}

/* ── Security Types ── */

export interface AuditEntry {
  id: number;
  tool: string;
  caller: string;
  level: string;
  allowed: boolean;
  args_summary: string;
  result_summary: string;
  created_at: string;
}

export interface AuditResponse {
  entries: AuditEntry[];
  total: number;
}

export interface ToolGuardStatus {
  enabled: boolean;
  allow_destructive: boolean;
  allow_sudo: boolean;
  allow_reboot: boolean;
  auto_approve: string[];
  require_confirmation: string[];
  protected_paths: string[];
  ssh_allowed_hosts: string[];
  dangerous_commands: string[];
  tool_permissions: Record<string, string>;
}

export interface VaultStatus {
  exists: boolean;
  unlocked: boolean;
  keys: string[];
}

export interface SecurityStatus {
  gateway_auth_configured: boolean;
  webui_auth_configured: boolean;
  tool_guard_enabled: boolean;
  vault_exists: boolean;
  vault_unlocked: boolean;
  audit_entry_count: number;
}

/* ── Hook Types ── */

export interface HookInfo {
  name: string;
  description: string;
  source: string;
  events: string[];
  priority: number;
  enabled: boolean;
}

export interface HookEventInfo {
  event: string;
  description: string;
  hooks: string[];
}

export interface HookListResponse {
  hooks: HookInfo[];
  events: HookEventInfo[];
}

/* ── Webhook Types ── */

export interface WebhookInfo {
  id: string;
  url: string;
  events: string[];
  active: boolean;
  created_at: string;
}

export interface WebhookListResponse {
  webhooks: WebhookInfo[];
  valid_events: string[];
}

/* ── Domain Types ── */

export interface DomainConfig {
  webui_address: string;
  webui_auth_configured: boolean;
  gateway_enabled: boolean;
  gateway_address: string;
  gateway_auth_configured: boolean;
  cors_origins: string[];
  tailscale_enabled: boolean;
  tailscale_serve: boolean;
  tailscale_funnel: boolean;
  tailscale_port: number;
  tailscale_hostname: string;
  tailscale_url: string;
  webui_url: string;
  gateway_url: string;
  public_url: string;
}

export interface DomainConfigUpdate {
  webui_address?: string;
  webui_auth_token?: string;
  gateway_enabled?: boolean;
  gateway_address?: string;
  gateway_auth_token?: string;
  cors_origins?: string[];
  tailscale_enabled?: boolean;
  tailscale_serve?: boolean;
  tailscale_funnel?: boolean;
  tailscale_port?: number;
}

/* ── MCP Server Types ── */

export interface MCPServerInfo {
  name: string;
  command: string;
  args: string[];
  env: Record<string, string>;
  enabled: boolean;
  status: string;
  error?: string;
}

export interface MCPServerListResponse {
  servers: MCPServerInfo[];
}

/* ── Database Types ── */

export interface DatabaseStatusInfo {
  name: string;
  healthy: boolean;
  latency: number;
  version: string;
  open_connections: number;
  in_use: number;
  idle: number;
  wait_count: number;
  wait_duration: number;
  max_open_conns: number;
  error?: string;
}

/* Tool Profile Settings */
export interface ToolProfileInfo {
  name: string;
  description: string;
  allow: string[];
  deny: string[];
  builtin: boolean;
}

/* ── OAuth Types ── */

export interface OAuthProvider {
  id: string;
  label: string;
  flow_type: 'pkce' | 'device_code' | 'manual';
  experimental?: boolean;
}

export interface OAuthStatus {
  provider: string;
  status: 'valid' | 'expiring_soon' | 'expired' | 'unknown';
  email?: string;
  expires_in?: number;
  has_token: boolean;
}

export interface OAuthStartResponse {
  flow_type: 'pkce' | 'device_code' | 'manual';
  auth_url?: string;
  provider: string;
  user_code?: string;
  verify_url?: string;
  expires_in?: number;
  experimental?: boolean;
}

/* ── Hub Types ── */

export interface HubSkillInfo {
  id: string
  name: string
  provider: string
  service: string
  label: string
  label_pt: string
  description: string
  description_pt: string
  emoji: string
  icon_svg?: string
  category: string
  scopes: string[]
  hub_url: string
  connected: boolean
  connection_id?: string
  email?: string
}

export interface HubConnection {
  id: string
  provider: string
  service: string
  email?: string
  status: string
  connected_at?: string
}

export interface HubConnectResponse {
  session_id: string
  connect_url: string
  expires_in: number
}

export interface HubConfigStatus {
  configured: boolean
  hub_url: string
  connected: boolean
}

/* ── Update Types ── */

export interface UpdateInfo {
  available: boolean;
  current_version: string;
  latest_version: string;
  checked_at: string;
}

export interface VersionInfo {
  version: string;
  update?: UpdateInfo;
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
    abort: (sessionId: string) => request<void>(`/chat/${sessionId}/abort`, { method: 'POST' }),
    history: (sessionId: string) => request<MessageInfo[]>(`/chat/${sessionId}/history`),
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
      request<{ status: string; message: string }>(`/skills/install`, {
        method: 'POST',
        body: JSON.stringify({ name }),
      }),
    remove: (name: string) =>
      request<void>(`/skills/${name}/remove`, {
        method: 'POST',
      }),
  },

  /* Channels */
  channels: {
    list: () => request<ChannelHealth[]>('/channels'),
    whatsapp: {
      status: () => request<WhatsAppStatus>('/channels/whatsapp/status'),
      requestQR: () =>
        request<{ status: string; message: string }>('/channels/whatsapp/qr', { method: 'POST' }),
      disconnect: () =>
        request<{ status: string; message: string }>('/channels/whatsapp/disconnect', {
          method: 'POST',
        }),
      // Access control
      getAccess: () => request<WhatsAppAccessConfig>('/channels/whatsapp/access'),
      updateAccessDefaultPolicy: (policy: string) =>
        request<{ status: string }>('/channels/whatsapp/access', {
          method: 'PATCH',
          body: JSON.stringify({ default_policy: policy }),
        }),
      grantUser: (jid: string, level: string) =>
        request<{ status: string; jid: string; level: string }>(
          `/channels/whatsapp/access/users/${encodeURIComponent(jid)}`,
          {
            method: 'POST',
            body: JSON.stringify({ level }),
          }
        ),
      revokeUser: (jid: string) =>
        request<{ status: string; jid: string }>(
          `/channels/whatsapp/access/users/${encodeURIComponent(jid)}`,
          {
            method: 'DELETE',
          }
        ),
      blockUser: (jid: string) =>
        request<{ status: string; jid: string }>(
          `/channels/whatsapp/access/blocked/${encodeURIComponent(jid)}`,
          {
            method: 'POST',
          }
        ),
      unblockUser: (jid: string) =>
        request<{ status: string; jid: string }>(
          `/channels/whatsapp/access/blocked/${encodeURIComponent(jid)}`,
          {
            method: 'DELETE',
          }
        ),
      // Groups
      getGroups: () => request<WhatsAppGroupPolicies>('/channels/whatsapp/groups'),
      getJoinedGroups: () =>
        request<{ jid: string; name: string }[]>('/channels/whatsapp/groups/joined'),
      updateGroupDefaultPolicy: (policy: string) =>
        request<{ status: string }>('/channels/whatsapp/groups', {
          method: 'PATCH',
          body: JSON.stringify({ default_policy: policy }),
        }),
      setGroupPolicy: (
        jid: string,
        policy: {
          name: string;
          policy: string;
          policies?: string[];
          keywords?: string[];
          allowed_users?: string[];
          workspace?: string;
        }
      ) =>
        request<{ status: string; group: string }>(
          `/channels/whatsapp/groups/${encodeURIComponent(jid)}`,
          {
            method: 'PUT',
            body: JSON.stringify(policy),
          }
        ),
      // Settings
      getSettings: () => request<WhatsAppSettings>('/channels/whatsapp/config'),
      updateConfig: (config: Partial<WhatsAppSettings>) =>
        request<{ status: string }>('/channels/whatsapp/config', {
          method: 'PATCH',
          body: JSON.stringify(config),
        }),
    },
  },

  /* Config */
  config: {
    get: () => request<Record<string, unknown>>('/config'),
    update: (data: Record<string, unknown>) =>
      request<Record<string, unknown>>('/config', {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
  },

  /* Hooks (lifecycle) */
  hooks: {
    list: () => request<HookListResponse>('/hooks'),
    toggle: (name: string, enabled: boolean) =>
      request<void>(`/hooks/${name}`, {
        method: 'PATCH',
        body: JSON.stringify({ enabled }),
      }),
    unregister: (name: string) => request<void>(`/hooks/${name}`, { method: 'DELETE' }),
  },

  /* Webhooks */
  webhooks: {
    list: () => request<WebhookListResponse>('/webhooks'),
    create: (url: string, events: string[]) =>
      request<WebhookInfo>('/webhooks', {
        method: 'POST',
        body: JSON.stringify({ url, events }),
      }),
    delete: (id: string) => request<void>(`/webhooks/${id}`, { method: 'DELETE' }),
    toggle: (id: string, active: boolean) =>
      request<void>(`/webhooks/${id}`, {
        method: 'PATCH',
        body: JSON.stringify({ active }),
      }),
  },

  /* Domain & Network */
  domain: {
    get: () => request<DomainConfig>('/domain'),
    update: (data: DomainConfigUpdate) =>
      request<{ status: string; message: string }>('/domain', {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
  },

  /* Usage */
  usage: () => request<UsageInfo>('/usage'),

  /* Jobs */
  jobs: {
    list: () => request<JobInfo[]>('/jobs'),
    toggle: (id: string, enabled: boolean) =>
      request<void>(`/jobs/${encodeURIComponent(id)}`, {
        method: 'PATCH',
        body: JSON.stringify({ enabled }),
      }),
    remove: (id: string) => request<void>(`/jobs/${encodeURIComponent(id)}`, { method: 'DELETE' }),
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
    logout: () =>
      request<{ status: string }>('/auth/logout', {
        method: 'POST',
      }),
  },

  /* MCP Servers */
  mcp: {
    list: () => request<MCPServerListResponse>('/mcp/servers'),
    create: (
      name: string,
      command: string,
      args: string[] = [],
      env: Record<string, string> = {}
    ) =>
      request<{ status: string }>('/mcp/servers', {
        method: 'POST',
        body: JSON.stringify({ name, command, args, env }),
      }),
    get: (name: string) => request<MCPServerInfo>(`/mcp/servers/${name}`),
    update: (name: string, enabled: boolean) =>
      request<{ status: string }>(`/mcp/servers/${name}`, {
        method: 'PUT',
        body: JSON.stringify({ enabled }),
      }),
    delete: (name: string) =>
      request<{ status: string }>(`/mcp/servers/${name}`, { method: 'DELETE' }),
    start: (name: string) =>
      request<{ status: string }>(`/mcp/servers/${name}/start`, { method: 'POST' }),
    stop: (name: string) =>
      request<{ status: string }>(`/mcp/servers/${name}/stop`, { method: 'POST' }),
  },

  /* Database */
  database: {
    status: () => request<DatabaseStatusInfo>('/database/status'),
  },

  /* System */
  system: {
    restart: () =>
      request<{ status: string }>('/system/restart', {
        method: 'POST',
      }),
    version: () => request<VersionInfo>('/system/version'),
    checkUpdate: () =>
      request<UpdateInfo>('/system/check-update', {
        method: 'POST',
      }),
    update: () =>
      request<{ status: string }>('/system/update', {
        method: 'POST',
      }),
  },

  /* Settings / Tool Profiles */
  settings: {
    toolProfiles: {
      list: () =>
        request<{
          profiles: ToolProfileInfo[];
          groups: Record<string, string[]>;
        }>('/settings/tool-profiles'),
      create: (profile: ToolProfileInfo) =>
        request<ToolProfileInfo>('/settings/tool-profiles', {
          method: 'POST',
          body: JSON.stringify(profile),
        }),
      update: (name: string, profile: ToolProfileInfo) =>
        request<ToolProfileInfo>(`/settings/tool-profiles/${name}`, {
          method: 'PUT',
          body: JSON.stringify(profile),
        }),
      delete: (name: string) =>
        request<void>(`/settings/tool-profiles/${name}`, { method: 'DELETE' }),
    },
  },

  /* Hub (OAuth Hub integration) */
  hub: {
    config: () => request<HubConfigStatus>('/oauth/hub/config'),
    setup: (hubUrl: string, apiKey: string) =>
      request<{ status: string; message: string }>('/oauth/hub/setup', {
        method: 'POST',
        body: JSON.stringify({ hub_url: hubUrl, api_key: apiKey }),
      }),
    skills: () => request<HubSkillInfo[]>('/oauth/hub/skills'),
    installSkill: (skillId: string) =>
      request<{ status: string; message: string }>('/oauth/hub/skills/install', {
        method: 'POST',
        body: JSON.stringify({ skill_id: skillId }),
      }),
    installBundle: (skillId = 'gator-hub') =>
      request<{ status: string; message: string }>('/oauth/hub/skills/install-bundle', {
        method: 'POST',
        body: JSON.stringify({ skill_id: skillId }),
      }),
    installReference: (provider: string, service: string) =>
      request<{ status: string; message: string }>('/oauth/hub/skills/install-reference', {
        method: 'POST',
        body: JSON.stringify({ provider, service }),
      }),
    removeReference: (provider: string, service: string) =>
      request<{ status: string; message: string }>('/oauth/hub/skills/remove-reference', {
        method: 'POST',
        body: JSON.stringify({ provider, service }),
      }),
    connect: (provider: string, service: string, scopes?: string[]) =>
      request<HubConnectResponse>('/oauth/hub/connect', {
        method: 'POST',
        body: JSON.stringify({ provider, service, scopes }),
      }),
    connections: () => request<HubConnection[]>('/oauth/hub/connections'),
    disconnect: (connectionId: string) =>
      request<void>(`/oauth/hub/connections/${connectionId}`, { method: 'DELETE' }),
    status: (sessionId: string) =>
      request<{ status: string; connection_id?: string; email?: string }>(`/oauth/hub/status/${sessionId}`),
  },

  /* OAuth */
  oauth: {
    providers: () => request<OAuthProvider[]>('/oauth/providers'),
    status: () => request<Record<string, OAuthStatus>>('/oauth/status'),
    start: (provider: string) =>
      request<OAuthStartResponse>(`/oauth/start/${provider}`, { method: 'POST' }),
    callback: (provider: string, code: string, state: string) =>
      request<{ status: string }>(`/oauth/callback/${provider}?code=${code}&state=${state}`),
    refresh: (provider: string) =>
      request<{ status: string }>(`/oauth/refresh/${provider}`, { method: 'POST' }),
    logout: (provider: string) => request<void>(`/oauth/logout/${provider}`, { method: 'DELETE' }),
    poll: (provider: string) =>
      request<{ status: string; message?: string }>(`/oauth/poll/${provider}`, { method: 'POST' }),
  },

  /* Auth Profiles */
  authProfiles: {
    providers: () => request<{ providers: AuthProviderInfo[]; count: number }>('/auth/providers'),
    list: () => request<{ profiles: AuthProfileInfo[]; count: number }>('/profiles'),
    get: (id: string) => request<AuthProfileInfo>(`/profiles/${id}`),
    create: (data: CreateAuthProfileRequest) =>
      request<{ id: string; success: boolean; message: string }>('/profiles', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<AuthProfileInfo>) =>
      request<{ id: string; success: boolean }>(`/profiles/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<{ id: string; success: boolean }>(`/profiles/${id}`, { method: 'DELETE' }),
    test: (id: string) =>
      request<{ id: string; valid: boolean; expired: boolean; error?: string }>(
        `/profiles/${id}/test`,
        {
          method: 'POST',
        }
      ),
  },
};

/* ── Auth Profile Types ── */

export interface AuthProviderInfo {
  name: string;
  label: string;
  description: string;
  modes: string[];
  website?: string;
  env_key?: string;
  parent_provider?: string;
}

export interface AuthProfileInfo {
  id: string;
  provider: string;
  name: string;
  mode: 'api_key' | 'oauth' | 'token';
  enabled: boolean;
  priority: number;
  valid: boolean;
  expired: boolean;
  email?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
  last_used_at?: string;
}

export interface CreateAuthProfileRequest {
  provider: string;
  name: string;
  mode: 'api_key' | 'oauth' | 'token';
  api_key?: string;
  token?: string;
  priority?: number;
}

import { useEffect, useState } from 'react'
import {
  Shield,
  Key,
  Activity,
  Lock,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  X,
} from 'lucide-react'
import { api, type SecurityStatus, type AuditEntry, type ToolGuardStatus, type VaultStatus } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

/**
 * Painel de seguranca — estilo discord-gaming dark.
 */
export function Security() {
  const [overview, setOverview] = useState<SecurityStatus | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.security.overview()
      .then(setOverview)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-[var(--color-dc-darker)]">
        <div className="h-8 w-8 rounded-full border-4 border-blue-500/30 border-t-blue-500 animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto bg-[var(--color-dc-darker)]">
      <div className="mx-auto max-w-4xl px-6 py-8">
        {/* Header */}
        <div className="flex items-center gap-3 mb-6">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-orange-500/15">
            <Shield className="h-5 w-5 text-orange-400" />
          </div>
          <div>
            <h1 className="text-xl font-bold text-white tracking-tight">Seguranca</h1>
            <p className="text-xs text-gray-500">Controle de acesso, auditoria e chaves</p>
          </div>
        </div>

        <div className="space-y-4">
          <AuditLogSection entryCount={overview?.audit_entry_count ?? 0} />
          <ToolGuardSection enabled={overview?.tool_guard_enabled ?? false} />
          <VaultSection exists={overview?.vault_exists ?? false} unlocked={overview?.vault_unlocked ?? false} />
          <APIKeysSection
            gatewayConfigured={overview?.gateway_auth_configured ?? false}
            webuiConfigured={overview?.webui_auth_configured ?? false}
          />
        </div>
      </div>
    </div>
  )
}

/* ── Audit Log ── */

function AuditLogSection({ entryCount }: { entryCount: number }) {
  const [open, setOpen] = useState(false)
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [loading, setLoading] = useState(false)

  const load = () => {
    if (entries.length > 0) return
    setLoading(true)
    api.security.audit(100)
      .then((data) => setEntries(data.entries ?? []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  const toggle = () => {
    const next = !open
    setOpen(next)
    if (next) load()
  }

  return (
    <section className="overflow-hidden rounded-2xl border border-white/[0.06] bg-[var(--color-dc-dark)]">
      <button
        onClick={toggle}
        className="flex w-full cursor-pointer items-center justify-between px-5 py-4 text-left transition-colors hover:bg-white/[0.02]"
      >
        <div className="flex items-center gap-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-orange-500/15 text-orange-400">
            <Activity className="h-5 w-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white">Audit Log</h3>
            <p className="text-xs text-gray-500">
              {entryCount > 0 ? `${entryCount} registros` : 'Historico de acoes executadas'}
            </p>
          </div>
        </div>
        {open ? <ChevronDown className="h-4 w-4 text-gray-600" /> : <ChevronRight className="h-4 w-4 text-gray-600" />}
      </button>

      {open && (
        <div className="border-t border-white/[0.04]">
          {loading ? (
            <div className="flex justify-center py-8">
              <div className="h-6 w-6 rounded-full border-2 border-orange-500/30 border-t-orange-500 animate-spin" />
            </div>
          ) : entries.length === 0 ? (
            <p className="px-5 py-8 text-center text-sm text-gray-600">Nenhum registro de auditoria</p>
          ) : (
            <div className="max-h-[400px] overflow-y-auto">
              <table className="w-full text-xs">
                <thead className="sticky top-0 bg-[var(--color-dc-dark)]">
                  <tr className="text-left">
                    <th className="px-5 py-3 text-[10px] font-semibold uppercase tracking-wider text-gray-600">Ferramenta</th>
                    <th className="px-5 py-3 text-[10px] font-semibold uppercase tracking-wider text-gray-600">Caller</th>
                    <th className="px-5 py-3 text-[10px] font-semibold uppercase tracking-wider text-gray-600">Status</th>
                    <th className="px-5 py-3 text-[10px] font-semibold uppercase tracking-wider text-gray-600">Quando</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {entries.map((e) => (
                    <tr key={e.id} className="transition-colors hover:bg-white/[0.02]">
                      <td className="px-5 py-2.5 font-mono text-gray-300">{e.tool}</td>
                      <td className="px-5 py-2.5 text-gray-500">{e.caller || '—'}</td>
                      <td className="px-5 py-2.5">
                        {e.allowed ? (
                          <span className="rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-semibold text-emerald-400 ring-1 ring-emerald-500/20">OK</span>
                        ) : (
                          <span className="rounded-full bg-red-500/10 px-2 py-0.5 text-[10px] font-semibold text-red-400 ring-1 ring-red-500/20">Bloqueado</span>
                        )}
                      </td>
                      <td className="px-5 py-2.5 text-gray-600">{timeAgo(e.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </section>
  )
}

/* ── Tool Guard ── */

function ToolGuardSection({ enabled }: { enabled: boolean }) {
  const [open, setOpen] = useState(false)
  const [guard, setGuard] = useState<ToolGuardStatus | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [newConfirmTool, setNewConfirmTool] = useState('')
  const [newAutoTool, setNewAutoTool] = useState('')

  const load = () => {
    if (guard) return
    setLoading(true)
    api.security.toolGuard.get()
      .then(setGuard)
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  const toggle = () => {
    const next = !open
    setOpen(next)
    if (next) load()
  }

  const save = async (partial: Partial<ToolGuardStatus>) => {
    if (!guard) return
    setSaving(true)
    try {
      const updated = { ...guard, ...partial }
      await api.security.toolGuard.update(updated)
      setGuard(updated)
    } catch { /* ignore */ }
    setSaving(false)
  }

  const addToList = (field: 'require_confirmation' | 'auto_approve', value: string) => {
    if (!guard || !value.trim()) return
    const current = guard[field] ?? []
    if (current.includes(value.trim())) return
    save({ [field]: [...current, value.trim()] })
    if (field === 'require_confirmation') setNewConfirmTool('')
    else setNewAutoTool('')
  }

  const removeFromList = (field: 'require_confirmation' | 'auto_approve', value: string) => {
    if (!guard) return
    save({ [field]: (guard[field] ?? []).filter((v) => v !== value) })
  }

  return (
    <section className="overflow-hidden rounded-2xl border border-white/[0.06] bg-[var(--color-dc-dark)]">
      <button
        onClick={toggle}
        className="flex w-full cursor-pointer items-center justify-between px-5 py-4 text-left transition-colors hover:bg-white/[0.02]"
      >
        <div className="flex items-center gap-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-amber-500/15 text-amber-400">
            <Shield className="h-5 w-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white">Tool Guard</h3>
            <p className="text-xs text-gray-500">
              {enabled ? 'Ativo — controle de permissoes' : 'Desativado'}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ring-1 ${
            enabled
              ? 'bg-emerald-500/10 text-emerald-400 ring-emerald-500/20'
              : 'bg-white/[0.05] text-gray-500 ring-white/[0.06]'
          }`}>
            {enabled ? 'Ativo' : 'Inativo'}
          </span>
          {open ? <ChevronDown className="h-4 w-4 text-gray-600" /> : <ChevronRight className="h-4 w-4 text-gray-600" />}
        </div>
      </button>

      {open && (
        <div className="border-t border-white/[0.04] px-5 py-5 space-y-5">
          {loading || !guard ? (
            <div className="flex justify-center py-6">
              <div className="h-6 w-6 rounded-full border-2 border-blue-500/30 border-t-blue-500 animate-spin" />
            </div>
          ) : (
            <>
              {/* Toggles */}
              <div className="grid gap-3 sm:grid-cols-3">
                <ToggleCard
                  label="Destrutivos"
                  description="rm -rf, mkfs, dd..."
                  icon={<AlertTriangle className="h-4 w-4 text-amber-400" />}
                  enabled={guard.allow_destructive}
                  onChange={(v) => save({ allow_destructive: v })}
                  disabled={saving}
                  color="amber"
                />
                <ToggleCard
                  label="Sudo"
                  description="Permitir sudo"
                  icon={<Shield className="h-4 w-4 text-red-400" />}
                  enabled={guard.allow_sudo}
                  onChange={(v) => save({ allow_sudo: v })}
                  disabled={saving}
                  color="red"
                />
                <ToggleCard
                  label="Reboot"
                  description="shutdown, reboot"
                  icon={<AlertTriangle className="h-4 w-4 text-red-400" />}
                  enabled={guard.allow_reboot}
                  onChange={(v) => save({ allow_reboot: v })}
                  disabled={saving}
                  color="red"
                />
              </div>

              {/* Require Confirmation */}
              <div>
                <h4 className="text-[10px] font-semibold uppercase tracking-wider text-gray-600 mb-2.5">Requer confirmacao</h4>
                <div className="flex flex-wrap gap-2">
                  {(guard.require_confirmation ?? []).map((t) => (
                    <span key={t} className="inline-flex items-center gap-1.5 rounded-lg bg-amber-500/10 px-3 py-1.5 font-mono text-xs text-amber-400 ring-1 ring-amber-500/20">
                      {t}
                      <button onClick={() => removeFromList('require_confirmation', t)} className="cursor-pointer transition-colors hover:text-red-400">
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                  <form
                    className="inline-flex"
                    onSubmit={(e) => { e.preventDefault(); addToList('require_confirmation', newConfirmTool) }}
                  >
                    <input
                      value={newConfirmTool}
                      onChange={(e) => setNewConfirmTool(e.target.value)}
                      placeholder="+ adicionar..."
                      className="h-8 w-32 rounded-lg border border-white/[0.08] bg-[var(--color-dc-darker)] px-3 text-xs text-white outline-none placeholder:text-gray-600 focus:border-orange-500/30"
                    />
                  </form>
                </div>
              </div>

              {/* Auto Approve */}
              <div>
                <h4 className="text-[10px] font-semibold uppercase tracking-wider text-gray-600 mb-2.5">Auto-aprovacao</h4>
                <div className="flex flex-wrap gap-2">
                  {(guard.auto_approve ?? []).map((t) => (
                    <span key={t} className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500/10 px-3 py-1.5 font-mono text-xs text-emerald-400 ring-1 ring-emerald-500/20">
                      {t}
                      <button onClick={() => removeFromList('auto_approve', t)} className="cursor-pointer transition-colors hover:text-red-400">
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                  <form
                    className="inline-flex"
                    onSubmit={(e) => { e.preventDefault(); addToList('auto_approve', newAutoTool) }}
                  >
                    <input
                      value={newAutoTool}
                      onChange={(e) => setNewAutoTool(e.target.value)}
                      placeholder="+ adicionar..."
                      className="h-8 w-32 rounded-lg border border-white/[0.08] bg-[var(--color-dc-darker)] px-3 text-xs text-white outline-none placeholder:text-gray-600 focus:border-orange-500/30"
                    />
                  </form>
                </div>
              </div>

              {/* Protected Paths */}
              {(guard.protected_paths ?? []).length > 0 && (
                <div>
                  <h4 className="text-[10px] font-semibold uppercase tracking-wider text-gray-600 mb-2.5">Paths protegidos</h4>
                  <div className="flex flex-wrap gap-2">
                    {guard.protected_paths.map((p) => (
                      <span key={p} className="rounded-lg bg-white/[0.06] px-3 py-1.5 font-mono text-xs text-gray-400">
                        {p}
                      </span>
                    ))}
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </section>
  )
}

/* ── Vault ── */

function VaultSection({ exists, unlocked }: { exists: boolean; unlocked: boolean }) {
  const [open, setOpen] = useState(false)
  const [vault, setVault] = useState<VaultStatus | null>(null)
  const [loading, setLoading] = useState(false)

  const load = () => {
    if (vault) return
    setLoading(true)
    api.security.vault()
      .then(setVault)
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  const toggle = () => {
    const next = !open
    setOpen(next)
    if (next) load()
  }

  return (
    <section className="overflow-hidden rounded-2xl border border-white/[0.06] bg-[var(--color-dc-dark)]">
      <button
        onClick={toggle}
        className="flex w-full cursor-pointer items-center justify-between px-5 py-4 text-left transition-colors hover:bg-white/[0.02]"
      >
        <div className="flex items-center gap-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-violet-500/15 text-violet-400">
            <Lock className="h-5 w-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white">Vault</h3>
            <p className="text-xs text-gray-500">
              {!exists ? 'Nao configurado' : unlocked ? 'Desbloqueado' : 'Bloqueado'}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          {exists ? (
            <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ring-1 ${
              unlocked
                ? 'bg-emerald-500/10 text-emerald-400 ring-emerald-500/20'
                : 'bg-amber-500/10 text-amber-400 ring-amber-500/20'
            }`}>
              {unlocked ? 'Desbloqueado' : 'Bloqueado'}
            </span>
          ) : (
            <span className="rounded-full bg-white/[0.05] px-2.5 py-0.5 text-[10px] font-semibold text-gray-500 ring-1 ring-white/[0.06]">
              Nao existe
            </span>
          )}
          {open ? <ChevronDown className="h-4 w-4 text-gray-600" /> : <ChevronRight className="h-4 w-4 text-gray-600" />}
        </div>
      </button>

      {open && (
        <div className="border-t border-white/[0.04] px-5 py-5">
          {loading ? (
            <div className="flex justify-center py-6">
              <div className="h-6 w-6 rounded-full border-2 border-blue-500/30 border-t-blue-500 animate-spin" />
            </div>
          ) : !vault || !vault.exists ? (
            <div className="flex flex-col items-center py-6">
              <Lock className="h-10 w-10 text-gray-700" />
              <p className="mt-3 text-sm text-gray-500">Vault nao configurado</p>
              <p className="mt-1 text-xs text-gray-600">
                Use <code className="rounded-md bg-white/[0.06] px-1.5 py-0.5 text-gray-400">devclaw config vault-init</code> para criar
              </p>
            </div>
          ) : !vault.unlocked ? (
            <div className="flex flex-col items-center py-6">
              <Lock className="h-10 w-10 text-amber-400/50" />
              <p className="mt-3 text-sm text-gray-500">Vault esta bloqueado</p>
              <p className="mt-1 text-xs text-gray-600">
                Desbloqueie ao iniciar o servidor com a senha mestra
              </p>
            </div>
          ) : (
            <div>
              <p className="text-xs text-gray-500 mb-4">
                Secrets armazenados (AES-256-GCM + Argon2id). Valores nao sao exibidos.
              </p>
              {vault.keys.length === 0 ? (
                <p className="text-sm text-gray-600">Nenhum secret armazenado</p>
              ) : (
                <div className="space-y-2">
                  {vault.keys.map((key) => (
                    <div
                      key={key}
                      className="flex items-center gap-3 rounded-xl border border-white/[0.04] bg-[var(--color-dc-darker)] px-4 py-3"
                    >
                      <Key className="h-4 w-4 text-violet-400" />
                      <span className="font-mono text-sm text-white">{key}</span>
                      <span className="ml-auto text-xs text-gray-600">••••••••</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </section>
  )
}

/* ── API Keys ── */

function APIKeysSection({ gatewayConfigured, webuiConfigured }: { gatewayConfigured: boolean; webuiConfigured: boolean }) {
  const [open, setOpen] = useState(false)

  return (
    <section className="overflow-hidden rounded-2xl border border-white/[0.06] bg-[var(--color-dc-dark)]">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full cursor-pointer items-center justify-between px-5 py-4 text-left transition-colors hover:bg-white/[0.02]"
      >
        <div className="flex items-center gap-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-cyan-500/15 text-cyan-400">
            <Key className="h-5 w-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white">API Keys</h3>
            <p className="text-xs text-gray-500">Autenticacao do gateway e web UI</p>
          </div>
        </div>
        {open ? <ChevronDown className="h-4 w-4 text-gray-600" /> : <ChevronRight className="h-4 w-4 text-gray-600" />}
      </button>

      {open && (
        <div className="border-t border-white/[0.04] px-5 py-5 space-y-3">
          <div className="flex items-center justify-between rounded-xl border border-white/[0.04] bg-[var(--color-dc-darker)] px-5 py-4">
            <div>
              <p className="text-sm font-semibold text-white">Gateway Auth Token</p>
              <p className="mt-0.5 text-xs text-gray-500">Autenticacao Bearer para a API HTTP</p>
            </div>
            {gatewayConfigured ? (
              <div className="flex items-center gap-1.5 text-emerald-400">
                <CheckCircle2 className="h-4 w-4" />
                <span className="text-xs font-medium">Configurado</span>
              </div>
            ) : (
              <div className="flex items-center gap-1.5 text-gray-600">
                <XCircle className="h-4 w-4" />
                <span className="text-xs">Nao configurado</span>
              </div>
            )}
          </div>

          <div className="flex items-center justify-between rounded-xl border border-white/[0.04] bg-[var(--color-dc-darker)] px-5 py-4">
            <div>
              <p className="text-sm font-semibold text-white">Web UI Auth Token</p>
              <p className="mt-0.5 text-xs text-gray-500">Senha de acesso ao dashboard</p>
            </div>
            {webuiConfigured ? (
              <div className="flex items-center gap-1.5 text-emerald-400">
                <CheckCircle2 className="h-4 w-4" />
                <span className="text-xs font-medium">Configurado</span>
              </div>
            ) : (
              <div className="flex items-center gap-1.5 text-amber-400">
                <AlertTriangle className="h-4 w-4" />
                <span className="text-xs">Acesso publico</span>
              </div>
            )}
          </div>

          <p className="text-xs text-gray-600 pt-1">
            Tokens configurados no <code className="rounded-md bg-white/[0.06] px-1.5 py-0.5 text-gray-400">config.yaml</code>.
            Edite e reinicie o servidor para alterar.
          </p>
        </div>
      )}
    </section>
  )
}

/* ── Toggle Card ── */

function ToggleCard({
  label,
  description,
  icon,
  enabled,
  onChange,
  disabled,
  color = 'amber',
}: {
  label: string
  description: string
  icon: React.ReactNode
  enabled: boolean
  onChange: (value: boolean) => void
  disabled?: boolean
  color?: 'amber' | 'red'
}) {
  const activeColor = color === 'red'
    ? 'border-red-500/20 bg-red-500/[0.05]'
    : 'border-amber-500/20 bg-amber-500/[0.05]'
  const trackColor = color === 'red'
    ? 'bg-red-500'
    : 'bg-amber-500'

  return (
    <button
      onClick={() => onChange(!enabled)}
      disabled={disabled}
      className={`flex cursor-pointer items-center gap-3 rounded-xl border px-4 py-3.5 text-left transition-all ${
        enabled
          ? activeColor
          : 'border-white/[0.06] hover:border-white/[0.1]'
      } ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
    >
      {icon}
      <div className="min-w-0 flex-1">
        <p className="text-xs font-semibold text-white">{label}</p>
        <p className="text-[10px] text-gray-500">{description}</p>
      </div>
      <div className={`h-5 w-9 rounded-full transition-colors ${enabled ? trackColor : 'bg-white/[0.08]'}`}>
        <div className={`mt-0.5 h-4 w-4 rounded-full bg-white transition-transform ${enabled ? 'translate-x-4.5' : 'translate-x-0.5'}`} />
      </div>
    </button>
  )
}

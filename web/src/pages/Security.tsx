import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  Shield,
  Key,
  Activity,
  Lock,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  ChevronDown,
  X,
  ExternalLink,
} from 'lucide-react'
import { api, type SecurityStatus, type AuditEntry, type ToolGuardStatus, type VaultStatus } from '@/lib/api'
import { cn, timeAgo } from '@/lib/utils'
import {
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'

/**
 * Security panel -- vault, tool guard, audit log, API keys.
 */
export function Security() {
  const { t } = useTranslation()
  const [overview, setOverview] = useState<SecurityStatus | null>(null)
  const [loading, setLoading] = useState(true)

  const [loadError, setLoadError] = useState(false)

  useEffect(() => {
    api.security.overview()
      .then(setOverview)
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <LoadingSpinner />
  if (loadError) return <ErrorState message={t('common.error')} onRetry={() => window.location.reload()} retryLabel={t('common.loading')} />

  const vaultOk = overview?.vault_exists && overview?.vault_unlocked
  const guardOk = overview?.tool_guard_enabled
  const authOk = overview?.webui_auth_configured

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8 max-w-4xl mx-auto">
      {/* Header */}
      <div>
        <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-text-muted">{t('security.subtitle')}</p>
        <h1 className="mt-1 text-2xl font-bold text-text-primary tracking-tight">{t('security.title')}</h1>
      </div>

      {/* Quick status */}
      <div className="mt-6 grid grid-cols-3 gap-2.5">
        <StatusPill label={t('security.vault')} ok={!!vaultOk} text={vaultOk ? t('common.enabled') : t('common.disabled')} />
        <StatusPill label={t('security.toolGuard')} ok={!!guardOk} text={guardOk ? t('common.enabled') : t('common.disabled')} />
        <StatusPill label={t('security.auth')} ok={!!authOk} text={authOk ? t('common.enabled') : t('common.disabled')} />
      </div>

      <div className="mt-6 space-y-3">
        <VaultSection exists={overview?.vault_exists ?? false} unlocked={overview?.vault_unlocked ?? false} />
        <ToolGuardSection enabled={overview?.tool_guard_enabled ?? false} />
        <APIKeysSection
          gatewayConfigured={overview?.gateway_auth_configured ?? false}
          webuiConfigured={overview?.webui_auth_configured ?? false}
        />
        <AuditLogSection entryCount={overview?.audit_entry_count ?? 0} />
      </div>
    </div>
  )
}

/* -- Status Pill -- */

function StatusPill({ label, ok, text }: { label: string; ok: boolean; text: string }) {
  return (
    <div className={cn(
      'rounded-xl px-3.5 py-2.5 border',
      ok ? 'bg-bg-surface border-brand/30' : 'bg-bg-surface border-border'
    )}>
      <span className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">{label}</span>
      <div className="mt-0.5 flex items-center gap-1.5">
        <span className={cn('h-1.5 w-1.5 rounded-full', ok ? 'bg-success' : 'bg-text-muted')} />
        <span className={cn('text-xs font-medium', ok ? 'text-text-primary' : 'text-text-muted')}>{text}</span>
      </div>
    </div>
  )
}

/* -- Accordion wrapper -- */

function Accordion({
  icon,
  iconColor,
  title,
  subtitle,
  badge,
  defaultOpen = false,
  onOpen,
  children,
}: {
  icon: React.ReactNode
  iconColor: string
  title: string
  subtitle: string
  badge?: React.ReactNode
  defaultOpen?: boolean
  onOpen?: () => void
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)

  const toggle = () => {
    const next = !open
    setOpen(next)
    if (next && onOpen) onOpen()
  }

  return (
    <section className="overflow-hidden rounded-2xl border border-border bg-bg-surface">
      <button
        onClick={toggle}
        aria-expanded={open}
        className="flex w-full cursor-pointer items-center gap-4 px-5 py-4 text-left transition-colors hover:bg-bg-hover"
      >
        <div className={cn('flex h-9 w-9 shrink-0 items-center justify-center rounded-xl', iconColor)}>
          {icon}
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold text-text-primary">{title}</h3>
          <p className="text-[11px] text-text-muted">{subtitle}</p>
        </div>
        {badge}
        <ChevronDown className={cn('h-4 w-4 shrink-0 text-text-muted transition-transform', open ? '' : '-rotate-90')} />
      </button>
      {open && <div className="border-t border-border px-5 py-5">{children}</div>}
    </section>
  )
}

/* -- Vault -- */

function VaultSection({ exists, unlocked }: { exists: boolean; unlocked: boolean }) {
  const { t } = useTranslation()
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

  const statusBadge = (
    <span className={cn(
      'rounded-full px-2.5 py-0.5 text-[10px] font-semibold',
      !exists
        ? 'bg-bg-subtle text-text-muted'
        : unlocked
        ? 'bg-success-subtle text-success'
        : 'bg-bg-subtle text-warning'
    )}>
      {!exists ? t('security.notConfigured') : unlocked ? t('security.protected') : t('security.inaccessible')}
    </span>
  )

  return (
    <Accordion
      icon={<Lock className="h-4 w-4 text-purple-400" />}
      iconColor="bg-purple-400/10"
      title={t('security.vault')}
      subtitle={t('security.vaultDesc')}
      badge={statusBadge}
      onOpen={load}
    >
      {loading ? (
        <Spinner />
      ) : !vault || !vault.exists ? (
        <EmptyState
          icon={<Lock className="h-8 w-8 text-text-muted" />}
          title={t('security.vaultNotConfigured')}
          description={<>{t('security.vaultNotConfiguredDesc')} <Code>devclaw config vault-init</Code></>}
        />
      ) : !vault.unlocked ? (
        <EmptyState
          icon={<Lock className="h-8 w-8 text-warning/40" />}
          title={t('security.vaultInaccessible')}
          description={t('security.vaultInaccessibleDesc')}
        />
      ) : (
        <div>
          {vault.keys.length === 0 ? (
            <EmptyState
              icon={<Key className="h-8 w-8 text-text-muted" />}
              title={t('security.noSecrets')}
              description={t('security.noSecretsDesc')}
            />
          ) : (
            <div className="space-y-1.5">
              {vault.keys.map((key) => (
                <div
                  key={key}
                  className="flex items-center gap-3 rounded-xl bg-bg-main px-4 py-3 border border-border"
                >
                  <Key className="h-3.5 w-3.5 shrink-0 text-purple-400" />
                  <span className="min-w-0 flex-1 truncate font-mono text-sm text-text-primary">{key}</span>
                  <span className="text-xs tracking-widest text-text-muted">--------</span>
                </div>
              ))}
              <p className="pt-2 text-[11px] text-text-muted">
                {vault.keys.length} secret{vault.keys.length !== 1 ? 's' : ''} {t('security.stored')}
              </p>
            </div>
          )}
        </div>
      )}
    </Accordion>
  )
}

/* -- Tool Guard -- */

function ToolGuardSection({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation()
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

  const statusBadge = (
    <span className={cn(
      'rounded-full px-2.5 py-0.5 text-[10px] font-semibold',
      enabled
        ? 'bg-success-subtle text-success'
        : 'bg-bg-subtle text-text-muted'
    )}>
      {enabled ? t('common.enabled') : t('common.disabled')}
    </span>
  )

  return (
    <Accordion
      icon={<Shield className="h-4 w-4 text-warning" />}
      iconColor="bg-bg-subtle"
      title={t('security.toolGuard')}
      subtitle={t('security.toolGuardDesc')}
      badge={statusBadge}
      onOpen={load}
    >
      {loading || !guard ? (
        <Spinner />
      ) : !enabled ? (
        <EmptyState
          icon={<Shield className="h-8 w-8 text-text-muted" />}
          title={t('security.toolGuardDisabled')}
          description={<>{t('security.toolGuardDisabledDesc')} <Code>config.yaml</Code></>}
        />
      ) : (
        <div className="space-y-5">
          {/* Permission toggles */}
          <div>
            <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-text-muted">{t('security.dangerousPerms')}</p>
            <div className="grid gap-2 sm:grid-cols-3">
              <PermToggle
                label={t('security.destructive')}
                hint={t('security.destructiveHint')}
                enabled={guard.allow_destructive}
                onChange={(v) => save({ allow_destructive: v })}
                disabled={saving}
                color="amber"
              />
              <PermToggle
                label={t('security.sudo')}
                hint={t('security.sudoHint')}
                enabled={guard.allow_sudo}
                onChange={(v) => save({ allow_sudo: v })}
                disabled={saving}
                color="red"
              />
              <PermToggle
                label={t('security.reboot')}
                hint={t('security.rebootHint')}
                enabled={guard.allow_reboot}
                onChange={(v) => save({ allow_reboot: v })}
                disabled={saving}
                color="red"
              />
            </div>
          </div>

          {/* Tag lists side by side */}
          <div className="grid gap-4 sm:grid-cols-2">
            <TagList
              label={t('security.requireConfirmation')}
              hint={t('security.requireConfirmationHint')}
              items={guard.require_confirmation ?? []}
              color="amber"
              onRemove={(v) => removeFromList('require_confirmation', v)}
              inputValue={newConfirmTool}
              onInputChange={setNewConfirmTool}
              onAdd={(v) => addToList('require_confirmation', v)}
            />

            <TagList
              label={t('security.autoApprove')}
              hint={t('security.autoApproveHint')}
              items={guard.auto_approve ?? []}
              color="emerald"
              onRemove={(v) => removeFromList('auto_approve', v)}
              inputValue={newAutoTool}
              onInputChange={setNewAutoTool}
              onAdd={(v) => addToList('auto_approve', v)}
            />
          </div>

          {(guard.protected_paths ?? []).length > 0 && (
            <div>
              <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-text-muted">{t('security.protectedPaths')}</p>
              <div className="flex flex-wrap gap-1.5">
                {guard.protected_paths.map((p) => (
                  <span key={p} className="rounded-lg bg-bg-subtle px-2.5 py-1 font-mono text-xs text-text-secondary">{p}</span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </Accordion>
  )
}

/* -- API Keys -- */

function APIKeysSection({ gatewayConfigured, webuiConfigured }: { gatewayConfigured: boolean; webuiConfigured: boolean }) {
  const { t } = useTranslation()

  return (
    <Accordion
      icon={<Key className="h-4 w-4 text-cyan-400" />}
      iconColor="bg-cyan-400/10"
      title={t('security.authentication')}
      subtitle={t('security.authenticationDesc')}
    >
      <div className="space-y-2">
        <AuthRow label={t('security.gatewayApi')} hint={t('security.gatewayApiHint')} configured={gatewayConfigured} />
        <AuthRow label={t('security.webUi')} hint={t('security.webUiHint')} configured={webuiConfigured} warn={!webuiConfigured} />
      </div>
      <div className="mt-4 flex items-center gap-2 text-[11px] text-text-muted">
        <span>{t('security.changeTokensIn')}</span>
        <Link to="/domain" className="inline-flex items-center gap-1 text-text-muted hover:text-text-primary transition-colors">
          {t('security.domainAccess')}
          <ExternalLink className="h-2.5 w-2.5" />
        </Link>
      </div>
    </Accordion>
  )
}

function AuthRow({ label, hint, configured, warn }: { label: string; hint: string; configured: boolean; warn?: boolean }) {
  const { t } = useTranslation()

  return (
    <div className="flex items-center justify-between rounded-xl bg-bg-main px-4 py-3 border border-border">
      <div>
        <p className="text-sm font-medium text-text-primary">{label}</p>
        <p className="text-[11px] text-text-muted">{hint}</p>
      </div>
      {configured ? (
        <span className="flex items-center gap-1.5 text-xs font-medium text-success">
          <CheckCircle2 className="h-3.5 w-3.5" /> {t('security.configured')}
        </span>
      ) : warn ? (
        <span className="flex items-center gap-1.5 text-xs font-medium text-warning">
          <AlertTriangle className="h-3.5 w-3.5" /> {t('security.unprotected')}
        </span>
      ) : (
        <span className="flex items-center gap-1.5 text-xs text-text-muted">
          <XCircle className="h-3.5 w-3.5" /> {t('security.notConfigured')}
        </span>
      )}
    </div>
  )
}

/* -- Audit Log -- */

function AuditLogSection({ entryCount }: { entryCount: number }) {
  const { t } = useTranslation()
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

  return (
    <Accordion
      icon={<Activity className="h-4 w-4 text-text-muted" />}
      iconColor="bg-bg-subtle"
      title={t('security.auditLog')}
      subtitle={entryCount > 0 ? `${entryCount} ${t('security.records')}` : t('security.auditLogDesc')}
      onOpen={load}
    >
      {loading ? (
        <Spinner />
      ) : entries.length === 0 ? (
        <div className="flex items-center gap-3 py-4">
          <Activity className="h-5 w-5 shrink-0 text-text-muted" />
          <div>
            <p className="text-sm text-text-secondary">{t('security.noActions')}</p>
            <p className="text-[11px] text-text-muted">{t('security.noActionsDesc')}</p>
          </div>
        </div>
      ) : (
        <div className="max-h-[380px] overflow-y-auto -mx-5 -mb-5">
          <table className="w-full text-xs">
            <thead className="sticky top-0 bg-bg-surface">
              <tr>
                <th className="px-5 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-text-muted">{t('security.tool')}</th>
                <th className="px-5 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-text-muted">{t('security.caller')}</th>
                <th className="px-5 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-text-muted">{t('security.status')}</th>
                <th className="px-5 py-2.5 text-right text-[10px] font-semibold uppercase tracking-wider text-text-muted">{t('security.when')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {entries.map((e) => (
                <tr key={e.id} className="transition-colors hover:bg-bg-hover">
                  <td className="px-5 py-2.5 font-mono text-text-primary">{e.tool}</td>
                  <td className="px-5 py-2.5 text-text-muted">{e.caller || '--'}</td>
                  <td className="px-5 py-2.5">
                    {e.allowed ? (
                      <span className="inline-flex items-center gap-1 text-[10px] font-medium text-success">
                        <CheckCircle2 className="h-3 w-3" /> OK
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 text-[10px] font-medium text-error">
                        <XCircle className="h-3 w-3" /> {t('security.denied')}
                      </span>
                    )}
                  </td>
                  <td className="px-5 py-2.5 text-right text-text-muted">{timeAgo(e.created_at, t)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Accordion>
  )
}

/* -- Shared components -- */

function Spinner() {
  return (
    <div className="flex justify-center py-8">
      <div className="h-6 w-6 rounded-full border-2 border-bg-subtle border-t-brand animate-spin" />
    </div>
  )
}

function EmptyState({ icon, title, description }: { icon: React.ReactNode; title: string; description: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center py-8">
      {icon}
      <p className="mt-3 text-sm font-medium text-text-secondary">{title}</p>
      <p className="mt-1 text-xs text-text-muted text-center max-w-xs">{description}</p>
    </div>
  )
}

function Code({ children }: { children: React.ReactNode }) {
  return <code className="rounded bg-bg-subtle px-1.5 py-0.5 text-text-secondary">{children}</code>
}

function PermToggle({
  label,
  hint,
  enabled,
  onChange,
  disabled,
  color = 'amber',
}: {
  label: string
  hint: string
  enabled: boolean
  onChange: (v: boolean) => void
  disabled?: boolean
  color?: 'amber' | 'red'
}) {
  const bgActive = color === 'red' ? 'bg-error-subtle' : 'bg-warning-subtle'
  const trackActive = color === 'red' ? 'bg-error' : 'bg-warning'

  return (
    <button
      onClick={() => onChange(!enabled)}
      disabled={disabled}
      className={cn(
        'flex cursor-pointer items-center gap-3 rounded-xl px-3.5 py-3 text-left border transition-all',
        enabled ? `${bgActive} border-border` : 'border-border bg-bg-main hover:border-border-hover',
        disabled && 'opacity-50 cursor-not-allowed'
      )}
    >
      <div className="min-w-0 flex-1">
        <p className="text-xs font-semibold text-text-primary">{label}</p>
        <p className="text-[10px] text-text-muted">{hint}</p>
      </div>
      <div className={cn(
        'inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors',
        enabled ? trackActive : 'bg-bg-subtle'
      )}>
        <div className={cn(
          'h-4 w-4 rounded-full bg-white shadow-sm transition-transform',
          enabled ? 'translate-x-[18px]' : 'translate-x-0.5'
        )} />
      </div>
    </button>
  )
}

function TagList({
  label,
  hint,
  items,
  color,
  onRemove,
  inputValue,
  onInputChange,
  onAdd,
}: {
  label: string
  hint?: string
  items: string[]
  color: 'amber' | 'emerald'
  onRemove: (v: string) => void
  inputValue: string
  onInputChange: (v: string) => void
  onAdd: (v: string) => void
}) {
  const tagClass = color === 'amber'
    ? 'bg-warning-subtle text-warning'
    : 'bg-success-subtle text-success'

  return (
    <div className="rounded-xl bg-bg-main px-4 py-3 border border-border">
      <p className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">{label}</p>
      {hint && <p className="mt-0.5 text-[10px] text-text-muted">{hint}</p>}
      <div className="mt-2.5 flex flex-wrap gap-1.5">
        {items.map((t) => (
          <span key={t} className={cn('inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 font-mono text-xs', tagClass)}>
            {t}
            <button onClick={() => onRemove(t)} className="cursor-pointer transition-colors hover:text-error">
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
        <form className="inline-flex" onSubmit={(e) => { e.preventDefault(); onAdd(inputValue) }}>
          <input
            value={inputValue}
            onChange={(e) => onInputChange(e.target.value)}
            placeholder={items.length === 0 ? 'tool_name' : '+ add'}
            className="h-7 w-28 rounded-lg bg-transparent px-2 text-xs text-text-secondary outline-none placeholder:text-text-muted focus:placeholder:text-text-muted"
          />
        </form>
      </div>
    </div>
  )
}

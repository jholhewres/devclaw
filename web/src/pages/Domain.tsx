import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Globe,
  Server,
  Save,
  Loader2,
  CheckCircle2,
  ExternalLink,
  Network,
  Eye,
  EyeOff,
  X,
  Lock,
  Unlock,
  ArrowUpRight,
} from 'lucide-react'
import { api } from '@/lib/api'
import type { DomainConfig } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/Button'
import { Card } from '@/components/ui/Card'
import {
  ConfigField,
  ConfigPage,
  LoadingSpinner,
} from '@/components/ui/ConfigComponents'

/**
 * Domain and network configuration page.
 */
export function Domain() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<DomainConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const [webuiAddress, setWebuiAddress] = useState('')
  const [webuiToken, setWebuiToken] = useState('')
  const [showWebuiToken, setShowWebuiToken] = useState(false)

  const [gatewayEnabled, setGatewayEnabled] = useState(false)
  const [gatewayAddress, setGatewayAddress] = useState('')
  const [gatewayToken, setGatewayToken] = useState('')
  const [showGatewayToken, setShowGatewayToken] = useState(false)
  const [corsOrigins, setCorsOrigins] = useState<string[]>([])
  const [newCors, setNewCors] = useState('')

  const [tailscaleEnabled, setTailscaleEnabled] = useState(false)
  const [tailscaleServe, setTailscaleServe] = useState(false)
  const [tailscaleFunnel, setTailscaleFunnel] = useState(false)
  const [tailscalePort, setTailscalePort] = useState(8085)

  useEffect(() => {
    api.domain.get()
      .then((data) => {
        setConfig(data)
        setWebuiAddress(data.webui_address || ':8090')
        setGatewayEnabled(data.gateway_enabled)
        setGatewayAddress(data.gateway_address || ':8085')
        setCorsOrigins(data.cors_origins || [])
        setTailscaleEnabled(data.tailscale_enabled)
        setTailscaleServe(data.tailscale_serve)
        setTailscaleFunnel(data.tailscale_funnel)
        setTailscalePort(data.tailscale_port || 8085)
      })
      .catch(() => setMessage({ type: 'error', text: t('common.error') }))
      .finally(() => setLoading(false))
  }, [t])

  const handleSave = async () => {
    setSaving(true)
    setMessage(null)
    try {
      await api.domain.update({
        webui_address: webuiAddress,
        webui_auth_token: webuiToken || undefined,
        gateway_enabled: gatewayEnabled,
        gateway_address: gatewayAddress,
        gateway_auth_token: gatewayToken || undefined,
        cors_origins: corsOrigins,
        tailscale_enabled: tailscaleEnabled,
        tailscale_serve: tailscaleServe,
        tailscale_funnel: tailscaleFunnel,
        tailscale_port: tailscalePort,
      })
      setMessage({ type: 'success', text: t('common.success') })
      setWebuiToken('')
      setGatewayToken('')
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    } finally {
      setSaving(false)
    }
  }

  const addCorsOrigin = () => {
    const trimmed = newCors.trim()
    if (trimmed && !corsOrigins.includes(trimmed)) {
      setCorsOrigins([...corsOrigins, trimmed])
      setNewCors('')
    }
  }

  if (loading) return <LoadingSpinner />

  return (
    <ConfigPage
      title={t('domain.title')}
      subtitle={t('domain.subtitle')}
      actions={
        <Button size="lg" onClick={handleSave} disabled={saving}>
          {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      }
      message={message}
    >

        {/* Status overview */}
        {config && (
          <div className="mt-6 grid grid-cols-3 gap-2.5">
            <Endpoint
              label={t('domainPage.webui.title')}
              url={config.webui_url}
              active
              secure={config.webui_auth_configured}
            />
            <Endpoint
              label={t('domainPage.gateway.title')}
              url={config.gateway_url}
              active={config.gateway_enabled}
              secure={config.gateway_auth_configured}
            />
            <Endpoint
              label={t('domainPage.tailscale.title')}
              url={config.public_url || config.tailscale_url}
              active={config.tailscale_enabled}
              secure
            />
          </div>
        )}

        {/* -- WebUI -- */}
        <Card className="mt-8 p-5">
          <CardHeader icon={Globe} title={t('domainPage.webui.title')} />
          <div className="mt-5 grid gap-4 sm:grid-cols-2">
            <ConfigField label={t('domain.port')}>
              <Input value={webuiAddress} onChange={setWebuiAddress} placeholder=":8090" />
            </ConfigField>
            <ConfigField label={t('domain.password')}>
              <PasswordInput
                value={webuiToken}
                onChange={setWebuiToken}
                show={showWebuiToken}
                onToggle={() => setShowWebuiToken(!showWebuiToken)}
                placeholder={config?.webui_auth_configured ? '--------' : t('domain.noPassword')}
              />
            </ConfigField>
          </div>
        </Card>

        {/* -- Gateway -- */}
        <Card className="mt-4 p-5">
          <div className="flex items-center justify-between">
            <CardHeader icon={Server} title={t('domainPage.gateway.title')} />
            <Toggle value={gatewayEnabled} onChange={setGatewayEnabled} />
          </div>

          {gatewayEnabled && (
            <div className="mt-5 space-y-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <ConfigField label={t('domain.port')}>
                  <Input value={gatewayAddress} onChange={setGatewayAddress} placeholder=":8085" />
                </ConfigField>
                <ConfigField label={t('domain.authToken')}>
                  <PasswordInput
                    value={gatewayToken}
                    onChange={setGatewayToken}
                    show={showGatewayToken}
                    onToggle={() => setShowGatewayToken(!showGatewayToken)}
                    placeholder={config?.gateway_auth_configured ? '--------' : t('domain.noToken')}
                  />
                </ConfigField>
              </div>

              {/* CORS */}
              <ConfigField label={t('domain.corsOrigins')}>
                <div className="flex flex-wrap gap-1.5">
                  {corsOrigins.map((origin) => (
                    <span
                      key={origin}
                      className="group flex items-center gap-1.5 rounded-lg bg-bg-subtle px-2.5 py-1.5 text-xs font-mono text-text-primary"
                    >
                      {origin}
                      <button
                        onClick={() => setCorsOrigins(corsOrigins.filter((o) => o !== origin))}
                        className="cursor-pointer text-text-muted transition-colors hover:text-error"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                  <input
                    value={newCors}
                    onChange={(e) => setNewCors(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && addCorsOrigin()}
                    onBlur={addCorsOrigin}
                    placeholder={t('domain.addOrigin')}
                    className="min-w-[140px] flex-1 rounded-lg bg-transparent px-2 py-1.5 text-xs text-text-secondary outline-none placeholder:text-text-muted"
                  />
                </div>
              </ConfigField>
            </div>
          )}
        </Card>

        {/* -- Tailscale -- */}
        <Card className="mt-4 mb-10 p-5">
          <div className="flex items-center justify-between">
            <CardHeader icon={Network} title={t('domainPage.tailscale.title')} />
            <Toggle value={tailscaleEnabled} onChange={setTailscaleEnabled} />
          </div>

          {tailscaleEnabled && (
            <div className="mt-5 space-y-3">
              <ToggleRow
                label={t('domain.serve')}
                description={t('domain.serveDesc')}
                value={tailscaleServe}
                onChange={setTailscaleServe}
              />
              <ToggleRow
                label={t('domain.funnel')}
                description={t('domain.funnelDesc')}
                value={tailscaleFunnel}
                onChange={setTailscaleFunnel}
              />

              <div className="pt-1">
                <ConfigField label={t('domain.localPort')}>
                  <Input
                    value={String(tailscalePort)}
                    onChange={(v) => setTailscalePort(parseInt(v) || 8085)}
                    placeholder="8085"
                    type="number"
                  />
                </ConfigField>
              </div>

              {config?.tailscale_hostname && (
                <div className="flex items-center gap-3 rounded-xl bg-success-subtle px-4 py-3 border border-success/20">
                  <CheckCircle2 className="h-4 w-4 shrink-0 text-success" />
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium text-text-primary truncate">{config.tailscale_hostname}</p>
                    {config.tailscale_url && (
                      <a
                        href={config.tailscale_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="flex items-center gap-1 text-xs text-text-muted hover:text-text-primary transition-colors"
                      >
                        {config.tailscale_url}
                        <ArrowUpRight className="h-3 w-3" />
                      </a>
                    )}
                  </div>
                </div>
              )}
            </div>
          )}
        </Card>
    </ConfigPage>
  )
}

/* -- Components -- */

function CardHeader({ icon: Icon, title }: { icon: React.FC<{ className?: string }>; title: string }) {
  return (
    <div className="flex items-center gap-2.5">
      <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-bg-subtle">
        <Icon className="h-4 w-4 text-text-muted" />
      </div>
      <h2 className="text-sm font-semibold text-text-primary">{title}</h2>
    </div>
  )
}

function Input({
  value,
  onChange,
  placeholder,
  type = 'text',
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  type?: string
}) {
  return (
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className="flex h-11 w-full rounded-xl border border-border bg-bg-main px-4 text-sm text-text-primary placeholder:text-text-muted outline-none transition-all hover:border-border-hover focus:border-brand/50 focus:ring-1 focus:ring-brand/20"
    />
  )
}

function PasswordInput({
  value,
  onChange,
  show,
  onToggle,
  placeholder,
}: {
  value: string
  onChange: (v: string) => void
  show: boolean
  onToggle: () => void
  placeholder?: string
}) {
  return (
    <div className="relative">
      <input
        type={show ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="flex h-11 w-full rounded-xl border border-border bg-bg-main px-4 pr-9 text-sm text-text-primary placeholder:text-text-muted outline-none transition-all hover:border-border-hover focus:border-brand/50 focus:ring-1 focus:ring-brand/20"
      />
      <button
        type="button"
        onClick={onToggle}
        className="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary transition-colors"
      >
        {show ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
      </button>
    </div>
  )
}

function Toggle({ value, onChange }: { value: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={value}
      onClick={() => onChange(!value)}
      className={cn(
        'relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors',
        value ? 'bg-brand' : 'bg-bg-subtle'
      )}
    >
      <span
        className={cn(
          'inline-block h-5 w-5 rounded-full bg-white shadow-sm transition-transform',
          value ? 'translate-x-5' : 'translate-x-0.5'
        )}
      />
    </button>
  )
}

function ToggleRow({
  label,
  description,
  value,
  onChange,
}: {
  label: string
  description: string
  value: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between rounded-xl bg-bg-main px-4 py-3 border border-border">
      <div>
        <span className="text-sm font-medium text-text-primary">{label}</span>
        <p className="text-[11px] text-text-muted">{description}</p>
      </div>
      <Toggle value={value} onChange={onChange} />
    </div>
  )
}

function Endpoint({
  label,
  url,
  active,
  secure,
}: {
  label: string
  url?: string
  active: boolean
  secure: boolean
}) {
  const { t } = useTranslation()

  return (
    <div className={cn(
      'rounded-xl px-3.5 py-2.5 border transition-colors',
      active
        ? 'bg-bg-surface border-brand/30'
        : 'bg-bg-surface border-border'
    )}>
      <div className="flex items-center justify-between">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">{label}</span>
        <div className="flex items-center gap-1">
          {active ? (
            <span className="h-1.5 w-1.5 rounded-full bg-success" />
          ) : (
            <span className="h-1.5 w-1.5 rounded-full bg-text-muted" />
          )}
          {active && (secure ? (
            <Lock className="h-3 w-3 text-text-muted" />
          ) : (
            <Unlock className="h-3 w-3 text-warning" />
          ))}
        </div>
      </div>
      {url ? (
        <a
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          className="mt-1 flex items-center gap-1 text-[11px] font-mono text-text-muted hover:text-text-primary transition-colors truncate"
        >
          {url.replace(/^https?:\/\//, '')}
          <ExternalLink className="h-2.5 w-2.5 shrink-0" />
        </a>
      ) : (
        <p className="mt-1 text-[11px] text-text-muted">{active ? t('common.enabled') : t('domain.off')}</p>
      )}
    </div>
  )
}

import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ToggleLeft,
  ToggleRight,
  Puzzle,
  Wrench,
  Bot,
  Zap,
  ChevronDown,
  ChevronRight,
  Save,
  X,
  Eye,
  EyeOff,
  Download,
  Trash2,
} from 'lucide-react'
import { api, type PluginInfo, type PluginConfigField } from '@/lib/api'
import { cn } from '@/lib/utils'
import { PageHeader } from '@/components/ui/PageHeader'
import { SearchInput } from '@/components/ui/SearchInput'
import { Badge } from '@/components/ui/Badge'
import { Card } from '@/components/ui/Card'
import { EmptyState } from '@/components/ui/EmptyState'
import { Button } from '@/components/ui/Button'
import { LoadingSpinner, ErrorState } from '@/components/ui/ConfigComponents'

export function Plugins() {
  const { t } = useTranslation()
  const [plugins, setPlugins] = useState<PluginInfo[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [toggleError, setToggleError] = useState<string | null>(null)
  const [showInstall, setShowInstall] = useState(false)
  const [installSource, setInstallSource] = useState('')
  const [installing, setInstalling] = useState(false)
  const [installError, setInstallError] = useState<string | null>(null)
  const [installSuccess, setInstallSuccess] = useState<string | null>(null)

  useEffect(() => {
    api.plugins
      .list()
      .then(setPlugins)
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  const filtered = plugins.filter(
    (p) =>
      p.name.toLowerCase().includes(search.toLowerCase()) ||
      p.description?.toLowerCase().includes(search.toLowerCase()) ||
      p.id.toLowerCase().includes(search.toLowerCase()),
  )

  const handleToggle = async (id: string, currentEnabled: boolean) => {
    setToggleError(null)
    try {
      await api.plugins.toggle(id, !currentEnabled)
      setPlugins((prev) => prev.map((p) => (p.id === id ? { ...p, enabled: !currentEnabled } : p)))
    } catch {
      setToggleError(id)
      setTimeout(() => setToggleError(null), 3000)
    }
  }

  const handleInstall = async () => {
    if (!installSource.trim()) return
    setInstalling(true)
    setInstallError(null)
    setInstallSuccess(null)
    try {
      const result = await api.plugins.install(installSource.trim())
      setInstallSuccess(`${result.is_new ? 'Installed' : 'Updated'}: ${result.name} v${result.version}`)
      setInstallSource('')
      // Refresh list.
      const updated = await api.plugins.list()
      setPlugins(updated)
      setTimeout(() => {
        setInstallSuccess(null)
        setShowInstall(false)
      }, 3000)
    } catch (err) {
      setInstallError(err instanceof Error ? err.message : 'Install failed')
    } finally {
      setInstalling(false)
    }
  }

  const handleRemove = async (id: string) => {
    try {
      await api.plugins.remove(id)
      setPlugins((prev) => prev.filter((p) => p.id !== id))
      setSelectedId(null)
    } catch {
      setToggleError(id)
      setTimeout(() => setToggleError(null), 3000)
    }
  }

  const enabledCount = plugins.filter((p) => p.enabled).length
  const selected = plugins.find((p) => p.id === selectedId)

  if (loading) return <LoadingSpinner />
  if (loadError) {
    return (
      <ErrorState
        message={t('common.error')}
        onRetry={() => window.location.reload()}
        retryLabel={t('common.retry')}
      />
    )
  }

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8">
      <div className="flex items-center justify-between">
        <PageHeader
          title="Plugins"
          description={`${enabledCount} ${t('skills.enabled').toLowerCase()} / ${plugins.length}`}
        />
        <Button
          onClick={() => setShowInstall(!showInstall)}
          variant={showInstall ? 'ghost' : 'default'}
        >
          {showInstall ? <X className="h-4 w-4" /> : <Download className="h-4 w-4" />}
          {showInstall ? t('common.cancel') : 'Install'}
        </Button>
      </div>

      {showInstall && (
        <Card padding="lg" className="mt-4 border-brand/30">
          <h3 className="text-sm font-semibold text-primary">Install Plugin</h3>
          <p className="mt-1 text-xs text-tertiary">
            Enter a GitHub repository (e.g. user/repo) or local path
          </p>
          <div className="mt-3 flex items-center gap-2">
            <input
              type="text"
              value={installSource}
              onChange={(e) => setInstallSource(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleInstall()}
              placeholder="user/repo or https://github.com/user/repo"
              className="flex-1 rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
              disabled={installing}
            />
            <Button onClick={handleInstall} disabled={installing || !installSource.trim()}>
              {installing ? t('common.saving') : 'Install'}
            </Button>
          </div>
          {installError && <p className="mt-2 text-xs text-fg-error-secondary">{installError}</p>}
          {installSuccess && <p className="mt-2 text-xs text-fg-success-secondary">{installSuccess}</p>}
        </Card>
      )}

      <SearchInput
        value={search}
        onChange={setSearch}
        placeholder={t('skillsPage.searchPlaceholder')}
        className="mt-6"
      />

      <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {filtered.map((plugin) => (
          <Card
            key={plugin.id}
            padding="lg"
            className={cn(
              'group relative overflow-hidden transition-all cursor-pointer',
              plugin.enabled ? 'border-brand/30' : 'hover:border-primary',
              selectedId === plugin.id && 'ring-2 ring-brand',
            )}
            onClick={() => setSelectedId(selectedId === plugin.id ? null : plugin.id)}
          >
            {plugin.enabled && (
              <div className="absolute right-4 top-4">
                <Badge className="bg-brand-secondary text-brand-tertiary">
                  {t('skills.enabled').toLowerCase()}
                </Badge>
              </div>
            )}

            <div
              className={cn(
                'flex h-14 w-14 items-center justify-center rounded-xl transition-colors',
                plugin.enabled
                  ? 'bg-brand-secondary text-brand-tertiary'
                  : 'bg-secondary text-tertiary group-hover:text-secondary',
              )}
              style={plugin.ui?.color ? { backgroundColor: plugin.ui.color + '20', color: plugin.ui.color } : undefined}
            >
              <Puzzle className="h-7 w-7" />
            </div>

            <h3 className="mt-4 text-lg font-semibold text-primary">{plugin.name}</h3>
            <p className="mt-1 text-xs text-tertiary">
              v{plugin.version}
              {plugin.author && ` by ${plugin.author}`}
            </p>
            <p className="mt-2 text-sm leading-relaxed text-secondary line-clamp-2">
              {plugin.description}
            </p>

            {plugin.error && (
              <p className="mt-2 text-xs text-fg-error-secondary">{plugin.error}</p>
            )}

            {toggleError === plugin.id && (
              <p className="mt-2 text-xs text-fg-error-secondary">{t('common.error')}</p>
            )}

            <div className="mt-4 flex items-center justify-between">
              <div className="flex items-center gap-2 flex-wrap">
                {(plugin.tools?.length ?? 0) > 0 && (
                  <span className="flex items-center gap-1 rounded-full bg-secondary px-2.5 py-0.5 text-xs font-medium text-tertiary">
                    <Wrench className="h-3 w-3" />
                    {plugin.tools!.length}
                  </span>
                )}
                {(plugin.agents?.length ?? 0) > 0 && (
                  <span className="flex items-center gap-1 rounded-full bg-secondary px-2.5 py-0.5 text-xs font-medium text-tertiary">
                    <Bot className="h-3 w-3" />
                    {plugin.agents!.length}
                  </span>
                )}
                {(plugin.hooks?.length ?? 0) > 0 && (
                  <span className="flex items-center gap-1 rounded-full bg-secondary px-2.5 py-0.5 text-xs font-medium text-tertiary">
                    <Zap className="h-3 w-3" />
                    {plugin.hooks!.length}
                  </span>
                )}
              </div>
              <div className="flex items-center gap-1">
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    handleRemove(plugin.id)
                  }}
                  className="cursor-pointer text-tertiary transition-colors hover:text-fg-error-secondary opacity-0 group-hover:opacity-100"
                  title="Remove plugin"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    handleToggle(plugin.id, plugin.enabled)
                  }}
                  className="cursor-pointer text-tertiary transition-colors hover:text-primary"
                >
                  {plugin.enabled ? (
                    <ToggleRight className="h-7 w-7 text-brand-tertiary" />
                  ) : (
                    <ToggleLeft className="h-7 w-7" />
                  )}
                </button>
              </div>
            </div>
          </Card>
        ))}
      </div>

      {filtered.length === 0 && (
        <EmptyState
          icon={<Puzzle className="h-6 w-6" />}
          title={search ? t('skillsPage.emptySearch') : t('pluginsPage.empty')}
          description={!search ? t('pluginsPage.emptyDesc') : undefined}
        />
      )}

      {/* Config panel */}
      {selected && selected.config_schema?.fields?.length ? (
        <PluginConfigPanel
          plugin={selected}
          onClose={() => setSelectedId(null)}
          onSaved={(updated) =>
            setPlugins((prev) => prev.map((p) => (p.id === updated.id ? updated : p)))
          }
        />
      ) : null}
    </div>
  )
}

// ── Plugin Config Panel ──

function PluginConfigPanel({
  plugin,
  onClose,
  onSaved,
}: {
  plugin: PluginInfo
  onClose: () => void
  onSaved: (updated: PluginInfo) => void
}) {
  const { t } = useTranslation()
  const fields = plugin.config_schema?.fields ?? []
  const sections = plugin.ui?.sections
  const [values, setValues] = useState<Record<string, unknown>>(() => ({
    ...(plugin.config_values ?? {}),
  }))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showSecrets, setShowSecrets] = useState<Record<string, boolean>>({})

  const hasChanges = JSON.stringify(values) !== JSON.stringify(plugin.config_values ?? {})

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    try {
      await api.plugins.configure(plugin.id, values)
      const updated = await api.plugins.get(plugin.id)
      onSaved(updated)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  // Group fields by UI sections, or show all flat.
  const fieldsByKey = new Map(fields.map((f) => [f.key, f]))
  const sectionFieldKeys = new Set(sections?.flatMap((s) => s.fields) ?? [])

  const renderField = (field: PluginConfigField) => {
    const value = values[field.key]
    const isSecret = field.type === 'secret'
    const visible = showSecrets[field.key]

    return (
      <div key={field.key} className="space-y-1.5">
        <label className="flex items-center gap-2 text-sm font-medium text-primary">
          {field.name || field.key}
          {field.required && <span className="text-fg-error-secondary">*</span>}
        </label>
        {field.description && (
          <p className="text-xs text-tertiary">{field.description}</p>
        )}
        <div className="relative">
          {field.type === 'bool' ? (
            <button
              type="button"
              onClick={() => setValues((v) => ({ ...v, [field.key]: !v[field.key] }))}
              className="cursor-pointer text-tertiary transition-colors hover:text-primary"
            >
              {value ? (
                <ToggleRight className="h-6 w-6 text-brand-tertiary" />
              ) : (
                <ToggleLeft className="h-6 w-6" />
              )}
            </button>
          ) : (
            <>
              <input
                type={isSecret && !visible ? 'password' : 'text'}
                value={String(value ?? '')}
                onChange={(e) => setValues((v) => ({ ...v, [field.key]: e.target.value }))}
                placeholder={field.default != null ? String(field.default) : undefined}
                className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
              />
              {isSecret && (
                <button
                  type="button"
                  onClick={() =>
                    setShowSecrets((s) => ({ ...s, [field.key]: !s[field.key] }))
                  }
                  className="absolute right-2.5 top-2.5 text-tertiary hover:text-primary cursor-pointer"
                >
                  {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              )}
            </>
          )}
        </div>
      </div>
    )
  }

  return (
    <Card padding="lg" className="mt-6 border-brand/30">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-primary">
          {plugin.name} — {t('pluginsPage.settings')}
        </h3>
        <button onClick={onClose} className="text-tertiary hover:text-primary cursor-pointer">
          <X className="h-5 w-5" />
        </button>
      </div>

      <div className="mt-6 space-y-6">
        {sections?.map((section, i) => (
          <ConfigSection key={i} title={section.title} description={section.description} collapsible={section.collapsible}>
            <div className="space-y-4">
              {section.fields.map((key) => {
                const field = fieldsByKey.get(key)
                return field ? renderField(field) : null
              })}
            </div>
          </ConfigSection>
        ))}

        {/* Render remaining fields not covered by sections */}
        {fields.filter((f) => !sectionFieldKeys.has(f.key)).length > 0 && (
          <div className="space-y-4">
            {fields
              .filter((f) => !sectionFieldKeys.has(f.key))
              .map(renderField)}
          </div>
        )}
      </div>

      {error && <p className="mt-4 text-sm text-fg-error-secondary">{error}</p>}

      {hasChanges && (
        <div className="mt-6 flex items-center gap-3">
          <Button onClick={handleSave} disabled={saving}>
            <Save className="h-4 w-4" />
            {saving ? t('common.saving') : t('common.save')}
          </Button>
          <Button
            variant="ghost"
            onClick={() => setValues({ ...(plugin.config_values ?? {}) })}
          >
            {t('pluginsPage.discard')}
          </Button>
        </div>
      )}
    </Card>
  )
}

// ── Collapsible Section ──

function ConfigSection({
  title,
  description,
  collapsible,
  children,
}: {
  title: string
  description?: string
  collapsible?: boolean
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(!collapsible)

  return (
    <div className="rounded-lg border border-secondary p-4">
      <button
        type="button"
        className={cn(
          'flex w-full items-center gap-2 text-left',
          collapsible && 'cursor-pointer',
        )}
        onClick={() => collapsible && setOpen(!open)}
      >
        {collapsible &&
          (open ? <ChevronDown className="h-4 w-4 text-tertiary" /> : <ChevronRight className="h-4 w-4 text-tertiary" />)}
        <div>
          <h4 className="text-sm font-semibold text-primary">{title}</h4>
          {description && <p className="text-xs text-tertiary">{description}</p>}
        </div>
      </button>
      {open && <div className="mt-4">{children}</div>}
    </div>
  )
}

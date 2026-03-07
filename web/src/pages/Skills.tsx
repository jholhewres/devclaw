import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ToggleLeft, ToggleRight, Package, Wrench, Plus, Download, X, Loader2, CheckCircle2 } from 'lucide-react'
import { api, type SkillInfo } from '@/lib/api'
import { cn } from '@/lib/utils'
import { PageHeader } from '@/components/ui/PageHeader'
import { SearchInput } from '@/components/ui/SearchInput'
import { Badge } from '@/components/ui/Badge'
import { Card } from '@/components/ui/Card'
import { EmptyState } from '@/components/ui/EmptyState'
import { Tabs } from '@/components/ui/Tabs'
import { LoadingSpinner, ErrorState } from '@/components/ui/ConfigComponents'

interface AvailableSkill {
  name: string
  description: string
  category: string
  version?: string
  tags?: string[]
  installed: boolean
}

export function Skills() {
  const { t } = useTranslation()
  const [skills, setSkills] = useState<SkillInfo[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [showInstall, setShowInstall] = useState(false)

  const [loadError, setLoadError] = useState(false)

  useEffect(() => {
    api.skills.list()
      .then(setSkills)
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  const filtered = skills.filter(
    (s) =>
      s.name.toLowerCase().includes(search.toLowerCase()) ||
      s.description.toLowerCase().includes(search.toLowerCase()),
  )

  const handleToggle = async (name: string, currentEnabled: boolean) => {
    try {
      await api.skills.toggle(name, !currentEnabled)
      setSkills((prev) =>
        prev.map((s) => (s.name === name ? { ...s, enabled: !currentEnabled } : s)),
      )
    } catch { /* ignore */ }
  }

  const handleInstalled = (name: string) => {
    if (!skills.find((s) => s.name === name)) {
      setSkills((prev) => [...prev, { name, description: t('skills.noSkills'), enabled: false, tool_count: 0 }])
    }
  }

  const enabledCount = skills.filter((s) => s.enabled).length

  if (loading) {
    return <LoadingSpinner />
  }

  if (loadError) {
    return (
      <ErrorState
        message={t('common.error')}
        onRetry={() => window.location.reload()}
        retryLabel={t('common.loading')}
      />
    )
  }

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8 max-w-screen-2xl mx-auto">
      {/* Header */}
      <PageHeader
        title={t('skills.title')}
        description={`${enabledCount} ${t('skills.enabled').toLowerCase()} / ${skills.length}`}
        actions={
          <button
            onClick={() => setShowInstall(true)}
            className="flex cursor-pointer items-center gap-2 rounded-xl bg-brand px-4 py-2.5 text-sm font-medium text-white transition-all hover:bg-brand-hover"
          >
            <Plus className="h-4 w-4" />
            {t('skillsPage.install')}
          </button>
        }
      />

      {/* Search */}
      <SearchInput
        value={search}
        onChange={setSearch}
        placeholder={t('skillsPage.searchPlaceholder')}
        className="mt-6"
      />

      {/* Grid */}
      <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {filtered.map((skill) => (
          <Card
            key={skill.name}
            padding="lg"
            className={cn(
              'group relative overflow-hidden rounded-2xl transition-all',
              skill.enabled
                ? 'border-brand/30'
                : 'hover:border-border-hover'
            )}
          >
            {skill.enabled && (
              <div className="absolute right-4 top-4">
                <Badge className="bg-brand-subtle text-brand">
                  {t('skills.enabled').toLowerCase()}
                </Badge>
              </div>
            )}

            <div className={cn(
              'flex h-14 w-14 items-center justify-center rounded-xl transition-colors',
              skill.enabled
                ? 'bg-brand-subtle text-brand'
                : 'bg-bg-subtle text-text-muted group-hover:text-text-secondary'
            )}>
              <Package className="h-7 w-7" />
            </div>

            <h3 className="mt-4 text-lg font-semibold text-text-primary">{skill.name}</h3>
            <p className="mt-2 text-sm leading-relaxed text-text-secondary line-clamp-2">{skill.description}</p>

            <div className="mt-4 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className="flex items-center gap-1.5 rounded-full bg-bg-subtle px-3 py-1 text-xs font-medium text-text-muted">
                  <Wrench className="h-3 w-3" />
                  {skill.tool_count} {t('skills.tools')}
                </span>
              </div>
              <button
                onClick={() => handleToggle(skill.name, skill.enabled)}
                aria-label={skill.enabled ? t('skillsPage.deactivate') : t('skillsPage.activate')}
                className="cursor-pointer text-text-muted transition-colors hover:text-text-primary"
              >
                {skill.enabled ? (
                  <ToggleRight className="h-7 w-7 text-brand" />
                ) : (
                  <ToggleLeft className="h-7 w-7" />
                )}
              </button>
            </div>
          </Card>
        ))}
      </div>

      {filtered.length === 0 && (
        <EmptyState
          icon={<Package className="h-6 w-6" />}
          title={search ? t('skillsPage.emptySearch') : t('skillsPage.emptyInstalled')}
          description={!search ? t('skillsPage.emptyInstalledDesc') : undefined}
        />
      )}

      {showInstall && (
        <InstallModal
          onClose={() => setShowInstall(false)}
          onInstalled={handleInstalled}
        />
      )}
    </div>
  )
}

function InstallModal({ onClose, onInstalled }: { onClose: () => void; onInstalled: (name: string) => void }) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<string>('catalog')
  const [available, setAvailable] = useState<AvailableSkill[]>([])
  const [loading, setLoading] = useState(true)
  const [fetchError, setFetchError] = useState(false)
  const [search, setSearch] = useState('')
  const [installing, setInstalling] = useState<string | null>(null)
  const [installed, setInstalled] = useState<Set<string>>(new Set())
  const [manualName, setManualName] = useState('')
  const [manualMsg, setManualMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const fetchCatalog = () => {
    setLoading(true)
    setFetchError(false)
    fetch('/api/skills/available', {
      headers: { Authorization: `Bearer ${localStorage.getItem('devclaw_token') || ''}` },
    })
      .then((r) => r.json())
      .then((data: AvailableSkill[]) => {
        setAvailable(Array.isArray(data) ? data : [])
        setInstalled(new Set((Array.isArray(data) ? data : []).filter((s) => s.installed).map((s) => s.name)))
        setFetchError(false)
      })
      .catch(() => setFetchError(true))
      .finally(() => setLoading(false))
  }

  useEffect(() => { fetchCatalog() }, [])

  const filtered = available.filter(
    (s) =>
      s.name.toLowerCase().includes(search.toLowerCase()) ||
      s.description?.toLowerCase().includes(search.toLowerCase()) ||
      s.category?.toLowerCase().includes(search.toLowerCase()),
  )

  const handleInstall = async (name: string) => {
    setInstalling(name)
    try {
      await api.skills.install(name)
      setInstalled((prev) => new Set([...prev, name]))
      onInstalled(name)
    } catch { /* ignore */ }
    setInstalling(null)
  }

  const handleManualInstall = async () => {
    const name = manualName.trim()
    if (!name) return
    setManualMsg(null)
    setInstalling(name)
    try {
      await api.skills.install(name)
      setInstalled((prev) => new Set([...prev, name]))
      onInstalled(name)
      setManualMsg({ type: 'success', text: t('skillsPage.manualInstallSuccess', { name }) })
      setManualName('')
    } catch {
      setManualMsg({ type: 'error', text: t('skillsPage.manualInstallError', { name }) })
    }
    setInstalling(null)
  }

  const categories = [...new Set(available.map((s) => s.category).filter(Boolean))]
  const [activeCategory, setActiveCategory] = useState<string | null>(null)

  const categoryFiltered = activeCategory
    ? filtered.filter((s) => s.category === activeCategory)
    : filtered

  const displayList = [...categoryFiltered].sort((a, b) => {
    const aInst = installed.has(a.name) ? 1 : 0
    const bInst = installed.has(b.name) ? 1 : 0
    if (aInst !== bInst) return aInst - bInst
    return a.name.localeCompare(b.name)
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onClose} onKeyDown={(e) => e.key === 'Escape' && onClose()}>
      <div
        className="relative w-full max-w-2xl max-h-[85vh] overflow-hidden rounded-2xl border border-border bg-bg-surface shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4">
          <div className="flex items-center gap-4">
            <h2 className="text-lg font-semibold text-text-primary">{t('skillsPage.manualInstallTitle')}</h2>
          </div>
          <button onClick={onClose} className="cursor-pointer rounded-lg p-1.5 text-text-muted hover:bg-bg-hover hover:text-text-primary transition-colors">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Tabs */}
        <div className="px-6">
          <Tabs
            tabs={[
              { id: 'catalog', label: t('skillsPage.tabAvailable') },
              { id: 'manual', label: t('skillsPage.installManually') },
            ]}
            activeTab={tab}
            onChange={setTab}
          />
        </div>

        {/* Catalog tab */}
        {tab === 'catalog' && (
          <>
            {/* Search + categories */}
            <div className="border-t border-border px-6 py-3">
              <SearchInput
                value={search}
                onChange={(v) => { setSearch(v); setActiveCategory(null) }}
                placeholder={t('skillsPage.searchPlaceholder')}
              />
              {categories.length > 0 && !search && (
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {categories.map((cat) => (
                    <button
                      key={cat}
                      onClick={() => setActiveCategory(activeCategory === cat ? null : cat)}
                      className={cn(
                        'cursor-pointer rounded-full px-2.5 py-1 text-[11px] font-medium transition-colors',
                        activeCategory === cat
                          ? 'bg-brand-subtle text-brand'
                          : 'bg-bg-subtle text-text-muted hover:bg-bg-hover hover:text-text-primary'
                      )}
                    >
                      {cat}
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* List */}
            <div className="overflow-y-auto px-6 py-4" style={{ maxHeight: 'calc(85vh - 200px)' }}>
              {loading ? (
                <div className="flex flex-col items-center gap-3 py-16">
                  <Loader2 className="h-6 w-6 animate-spin text-text-muted" />
                  <p className="text-xs text-text-muted">{t('skillsPage.loadingCatalog')}</p>
                </div>
              ) : fetchError ? (
                <div className="flex flex-col items-center py-12">
                  <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-error-subtle">
                    <X className="h-7 w-7 text-error" />
                  </div>
                  <p className="mt-4 text-sm font-medium text-text-primary">{t('skillsPage.errorCatalogTitle')}</p>
                  <p className="mt-1 text-xs text-text-muted text-center max-w-xs">
                    {t('skillsPage.errorCatalogDesc')}
                  </p>
                  <button
                    onClick={fetchCatalog}
                    className="mt-4 cursor-pointer rounded-lg bg-bg-subtle px-4 py-2 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
                  >
                    {t('skillsPage.retry')}
                  </button>
                </div>
              ) : displayList.length === 0 ? (
                <EmptyState
                  icon={<Package className="h-6 w-6" />}
                  title={search || activeCategory ? t('skillsPage.emptySearch') : t('skillsPage.emptyAvailable')}
                  description={!(search || activeCategory) ? t('skillsPage.emptyInstalledDesc') : undefined}
                  action={!(search || activeCategory) ? (
                    <button
                      onClick={() => setTab('manual')}
                      className="cursor-pointer text-xs font-medium text-text-muted hover:text-text-primary transition-colors"
                    >
                      {t('skillsPage.installManually')} &rarr;
                    </button>
                  ) : undefined}
                />
              ) : (
                <div className="space-y-1.5">
                  {displayList.map((skill) => {
                    const isInstalled = installed.has(skill.name)
                    const isInstalling = installing === skill.name

                    return (
                      <div
                        key={skill.name}
                        className={cn(
                          'flex items-center gap-4 rounded-xl px-4 py-3 transition-colors border',
                          isInstalled
                            ? 'bg-success-subtle border-success/20'
                            : 'bg-bg-main border-border hover:border-border-hover'
                        )}
                      >
                        <div className={cn(
                          'flex h-9 w-9 shrink-0 items-center justify-center rounded-lg',
                          isInstalled ? 'bg-success-subtle' : 'bg-bg-subtle'
                        )}>
                          <Package className={cn('h-4 w-4', isInstalled ? 'text-success' : 'text-text-muted')} />
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <h3 className="text-sm font-semibold text-text-primary">{skill.name}</h3>
                            {skill.version && (
                              <span className="text-[10px] text-text-muted">v{skill.version}</span>
                            )}
                            {skill.category && (
                              <Badge>{skill.category}</Badge>
                            )}
                          </div>
                          {skill.description && (
                            <p className="mt-0.5 text-xs text-text-muted line-clamp-1">{skill.description}</p>
                          )}
                        </div>
                        <div className="shrink-0">
                          {isInstalled ? (
                            <span className="flex items-center gap-1 text-xs font-medium text-success">
                              <CheckCircle2 className="h-3.5 w-3.5" />
                              {t('skillsPage.installed')}
                            </span>
                          ) : (
                            <button
                              onClick={() => handleInstall(skill.name)}
                              disabled={isInstalling}
                              className="flex cursor-pointer items-center gap-1.5 rounded-lg bg-brand px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-brand-hover disabled:opacity-50"
                            >
                              {isInstalling ? (
                                <Loader2 className="h-3 w-3 animate-spin" />
                              ) : (
                                <Download className="h-3 w-3" />
                              )}
                              {isInstalling ? t('common.loading') : t('skillsPage.install')}
                            </button>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          </>
        )}

        {/* Manual tab */}
        {tab === 'manual' && (
          <div className="border-t border-border px-6 py-6">
            <div className="space-y-5">
              <div>
                <p className="text-sm text-text-primary">
                  {t('skillsPage.manualInstallDesc')}
                </p>
              </div>

              <div>
                <label className="mb-1.5 block text-[11px] font-semibold uppercase tracking-wider text-text-muted">
                  {t('skillsPage.manualInstallPlaceholder')}
                </label>
                <div className="flex gap-2">
                  <input
                    value={manualName}
                    onChange={(e) => setManualName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleManualInstall()}
                    placeholder="ex: docker-manager, api-tester, aws-tools"
                    className="flex-1 rounded-lg border border-border bg-bg-main px-3 py-2.5 text-sm text-text-primary placeholder:text-text-muted outline-none transition-all focus:border-brand/50 hover:border-border-hover"
                  />
                  <button
                    onClick={handleManualInstall}
                    disabled={!manualName.trim() || installing !== null}
                    className="flex cursor-pointer items-center gap-2 rounded-lg bg-brand px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-brand-hover disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {installing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}
                    {t('skillsPage.install')}
                  </button>
                </div>
              </div>

              {manualMsg && (
                <div className={cn(
                  'flex items-center gap-2 rounded-lg px-3 py-2.5 text-xs border',
                  manualMsg.type === 'success'
                    ? 'bg-success-subtle text-success border-success/20'
                    : 'bg-error-subtle text-error border-error/20'
                )}>
                  {manualMsg.type === 'success' ? <CheckCircle2 className="h-3.5 w-3.5 shrink-0" /> : <X className="h-3.5 w-3.5 shrink-0" />}
                  {manualMsg.text}
                </div>
              )}

              <Card className="rounded-xl bg-bg-main">
                <p className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">{t('skillsPage.browseClawHub')}</p>
                <ul className="mt-2 space-y-1.5 text-xs text-text-secondary">
                  <li className="flex items-start gap-2">
                    <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-text-muted" />
                    {t('skillsPage.aboutSkill')} — SKILL.md
                  </li>
                  <li className="flex items-start gap-2">
                    <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-text-muted" />
                    ./skills/{'{name}'}/SKILL.md
                  </li>
                </ul>
              </Card>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

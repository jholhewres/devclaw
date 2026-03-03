import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Search, ToggleLeft, ToggleRight, Zap, Package, Wrench, Plus, Download, X, Loader2, CheckCircle2 } from 'lucide-react'
import { api, type SkillInfo } from '@/lib/api'

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
    return (
      <div className="flex flex-1 items-center justify-center bg-[#0b0d17]">
        <div className="h-10 w-10 rounded-full border-4 border-[#1c1f3a] border-t-[#6366f1] animate-spin" />
      </div>
    )
  }

  if (loadError) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center bg-[#0b0d17]">
        <p className="text-sm text-[#fb7185]">{t('common.error')}</p>
        <button onClick={() => window.location.reload()} className="mt-3 text-xs text-[#64748b] hover:text-[#f1f5f9] transition-colors cursor-pointer">
          {t('common.loading')}
        </button>
      </div>
    )
  }

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8 max-w-screen-2xl mx-auto">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-[#475569]">{t('skills.subtitle')}</p>
          <h1 className="mt-1 text-2xl font-bold text-[#f1f5f9] tracking-tight">{t('skills.title')}</h1>
          <p className="mt-2 text-base text-[#64748b]">
            {enabledCount} {t('skills.enabled').toLowerCase()} / {skills.length}
          </p>
        </div>
        <button
          onClick={() => setShowInstall(true)}
          className="flex cursor-pointer items-center gap-2 rounded-xl bg-[#6366f1] px-4 py-2.5 text-sm font-medium text-white transition-all hover:bg-[#818cf8]"
        >
          <Plus className="h-4 w-4" />
          Instalar Skill
        </button>
      </div>

      {/* Search */}
      <div className="relative mt-6">
          <Search className="absolute left-5 top-1/2 h-5 w-5 -translate-y-1/2 text-[#475569]" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Buscar skills..."
            className="w-full rounded-2xl border border-[rgba(99,102,241,0.12)] bg-[#14172b] px-5 py-4 pl-14 text-base text-[#f1f5f9] outline-none placeholder:text-[#475569] transition-all focus:border-[#6366f1]/50 focus:ring-1 focus:ring-[#6366f1]/20"
          />
        </div>

        {/* Grid */}
        <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.map((skill) => (
            <div
              key={skill.name}
              className={`group relative overflow-hidden rounded-2xl border p-6 transition-all ${
                skill.enabled
                  ? 'border-[#6366f1]/30 bg-[#14172b]'
                  : 'border-[rgba(99,102,241,0.12)] bg-[#14172b] hover:border-[rgba(99,102,241,0.24)]'
              }`}
            >
              {skill.enabled && (
                <div className="absolute right-4 top-4">
                  <span className="rounded-full bg-[#6366f1]/20 px-2.5 py-0.5 text-[10px] font-semibold text-[#6366f1]">ativa</span>
                </div>
              )}

                <div className={`flex h-14 w-14 items-center justify-center rounded-xl ${
                skill.enabled ? 'bg-[#6366f1]/10 text-[#6366f1]' : 'bg-[#1c1f3a] text-[#64748b] group-hover:text-[#94a3b8]'
              } transition-colors`}>
                <Package className="h-7 w-7" />
              </div>

              <h3 className="mt-4 text-lg font-semibold text-[#f1f5f9]">{skill.name}</h3>
              <p className="mt-2 text-sm leading-relaxed text-[#94a3b8] line-clamp-2">{skill.description}</p>

              <div className="mt-4 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="flex items-center gap-1.5 rounded-full bg-[#1c1f3a] px-3 py-1 text-xs font-medium text-[#64748b]">
                    <Wrench className="h-3 w-3" />
                    {skill.tool_count} ferramentas
                  </span>
                </div>
                <button
                  onClick={() => handleToggle(skill.name, skill.enabled)}
                  aria-label={skill.enabled ? `Desativar ${skill.name}` : `Ativar ${skill.name}`}
                  className="cursor-pointer text-[#64748b] transition-colors hover:text-[#f1f5f9]"
                >
                  {skill.enabled ? (
                    <ToggleRight className="h-7 w-7 text-[#6366f1]" />
                  ) : (
                    <ToggleLeft className="h-7 w-7" />
                  )}
                </button>
              </div>
            </div>
          ))}
        </div>

        {filtered.length === 0 && (
          <div className="mt-20 flex flex-col items-center">
            <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-[#1c1f3a]">
              <Zap className="h-8 w-8 text-[#475569]" />
            </div>
            <p className="mt-4 text-base font-medium text-[#64748b]">
              {search ? 'Nenhuma skill encontrada' : 'Nenhuma skill disponível'}
            </p>
          </div>
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
  const [tab, setTab] = useState<'catalog' | 'manual'>('catalog')
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
      setManualMsg({ type: 'success', text: `"${name}" instalada com sucesso.` })
      setManualName('')
    } catch {
      setManualMsg({ type: 'error', text: `Falha ao instalar "${name}". Verifique o nome.` })
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
        className="relative w-full max-w-2xl max-h-[85vh] overflow-hidden rounded-2xl border border-[rgba(99,102,241,0.12)] bg-[#14172b] shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4">
          <div className="flex items-center gap-4">
            <h2 className="text-lg font-semibold text-[#f1f5f9]">Instalar Skill</h2>
            <div className="flex rounded-lg bg-[#1c1f3a] p-0.5">
              <button
                onClick={() => setTab('catalog')}
                className={`cursor-pointer rounded-md px-3 py-1 text-xs font-medium transition-colors ${
                  tab === 'catalog' ? 'bg-[#6366f1] text-white' : 'text-[#64748b] hover:text-[#f1f5f9]'
                }`}
              >
                Catálogo
              </button>
              <button
                onClick={() => setTab('manual')}
                className={`cursor-pointer rounded-md px-3 py-1 text-xs font-medium transition-colors ${
                  tab === 'manual' ? 'bg-[#6366f1] text-white' : 'text-[#64748b] hover:text-[#f1f5f9]'
                }`}
              >
                Manual
              </button>
            </div>
          </div>
          <button onClick={onClose} className="cursor-pointer rounded-lg p-1.5 text-[#64748b] hover:bg-[rgba(99,102,241,0.08)] hover:text-[#f1f5f9] transition-colors">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Catalog tab */}
        {tab === 'catalog' && (
          <>
            {/* Search + categories */}
            <div className="border-t border-[rgba(99,102,241,0.12)] px-6 py-3">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[#475569]" />
                <input
                  value={search}
                  onChange={(e) => { setSearch(e.target.value); setActiveCategory(null) }}
                  placeholder="Buscar skills..."
                  autoFocus
                  className="w-full rounded-lg border border-[rgba(99,102,241,0.12)] bg-[#0b0d17] px-3 py-2.5 pl-10 text-sm text-[#f1f5f9] placeholder:text-[#475569] outline-none transition-all focus:border-[#6366f1]/50"
                />
              </div>
              {categories.length > 0 && !search && (
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {categories.map((cat) => (
                    <button
                      key={cat}
                      onClick={() => setActiveCategory(activeCategory === cat ? null : cat)}
                      className={`cursor-pointer rounded-full px-2.5 py-1 text-[11px] font-medium transition-colors ${
                        activeCategory === cat
                          ? 'bg-[#6366f1]/20 text-[#6366f1]'
                          : 'bg-[#1c1f3a] text-[#64748b] hover:bg-[#242850] hover:text-[#f1f5f9]'
                      }`}
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
                  <Loader2 className="h-6 w-6 animate-spin text-[#64748b]" />
                  <p className="text-xs text-[#64748b]">Carregando catálogo do GitHub...</p>
                </div>
              ) : fetchError ? (
                <div className="flex flex-col items-center py-12">
                  <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-[#f43f5e]/10">
                    <X className="h-7 w-7 text-[#fb7185]" />
                  </div>
                  <p className="mt-4 text-sm font-medium text-[#f1f5f9]">Could not load the catalog</p>
                  <p className="mt-1 text-xs text-[#64748b] text-center max-w-xs">
                    Verifique a conexão com a internet. O catálogo é baixado de github.com/jholhewres/devclaw-skills.
                  </p>
                  <button
                    onClick={fetchCatalog}
                    className="mt-4 cursor-pointer rounded-lg bg-[#1c1f3a] px-4 py-2 text-xs font-medium text-[#94a3b8] transition-colors hover:bg-[#242850] hover:text-[#f1f5f9]"
                  >
                    Tentar novamente
                  </button>
                </div>
              ) : displayList.length === 0 ? (
                <div className="flex flex-col items-center py-12">
                  <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-[#1c1f3a]">
                    <Package className="h-7 w-7 text-[#475569]" />
                  </div>
                  {search || activeCategory ? (
                    <>
                      <p className="mt-4 text-sm font-medium text-[#94a3b8]">Nenhuma skill encontrada</p>
                      <p className="mt-1 text-xs text-[#64748b]">Tente outro termo ou use a aba Manual</p>
                    </>
                  ) : (
                    <>
                      <p className="mt-4 text-sm font-medium text-[#94a3b8]">Catálogo vazio</p>
                      <p className="mt-1 text-xs text-[#64748b] text-center max-w-xs">
                        O catálogo remoto retornou vazio. Você pode instalar manualmente pela aba Manual.
                      </p>
                      <button
                        onClick={() => setTab('manual')}
                        className="mt-3 cursor-pointer text-xs font-medium text-[#64748b] hover:text-[#f1f5f9] transition-colors"
                      >
                        Instalar manualmente →
                      </button>
                    </>
                  )}
                </div>
              ) : (
                <div className="space-y-1.5">
                  {displayList.map((skill) => {
                    const isInstalled = installed.has(skill.name)
                    const isInstalling = installing === skill.name

                    return (
                      <div
                        key={skill.name}
                        className={`flex items-center gap-4 rounded-xl px-4 py-3 transition-colors ${
                          isInstalled
                            ? 'bg-[#10b981]/10 border border-[#10b981]/20'
                            : 'bg-[#0b0d17] border border-[rgba(99,102,241,0.12)] hover:border-[rgba(99,102,241,0.24)]'
                        }`}
                      >
                        <div className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${
                          isInstalled ? 'bg-[#10b981]/20' : 'bg-[#1c1f3a]'
                        }`}>
                          <Package className={`h-4 w-4 ${isInstalled ? 'text-[#10b981]' : 'text-[#64748b]'}`} />
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <h3 className="text-sm font-semibold text-[#f1f5f9]">{skill.name}</h3>
                            {skill.version && (
                              <span className="text-[10px] text-[#475569]">v{skill.version}</span>
                            )}
                            {skill.category && (
                              <span className="rounded bg-[#1c1f3a] px-1.5 py-0.5 text-[10px] font-medium text-[#64748b]">{skill.category}</span>
                            )}
                          </div>
                          {skill.description && (
                            <p className="mt-0.5 text-xs text-[#64748b] line-clamp-1">{skill.description}</p>
                          )}
                        </div>
                        <div className="shrink-0">
                          {isInstalled ? (
                            <span className="flex items-center gap-1 text-xs font-medium text-[#10b981]">
                              <CheckCircle2 className="h-3.5 w-3.5" />
                              Instalada
                            </span>
                          ) : (
                            <button
                              onClick={() => handleInstall(skill.name)}
                              disabled={isInstalling}
                              className="flex cursor-pointer items-center gap-1.5 rounded-lg bg-[#6366f1] px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-[#818cf8] disabled:opacity-50"
                            >
                              {isInstalling ? (
                                <Loader2 className="h-3 w-3 animate-spin" />
                              ) : (
                                <Download className="h-3 w-3" />
                              )}
                              {isInstalling ? 'Instalando...' : 'Instalar'}
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
          <div className="border-t border-[rgba(99,102,241,0.12)] px-6 py-6">
            <div className="space-y-5">
              <div>
                <p className="text-sm text-[#f1f5f9]">
                  Instale uma skill pelo nome exato do diretório no repositório{' '}
                  <a
                    href="https://github.com/jholhewres/devclaw-skills"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-[#6366f1] hover:text-[#a5b4fc] transition-colors"
                  >
                    devclaw-skills
                  </a>.
                </p>
              </div>

              <div>
                <label className="mb-1.5 block text-[11px] font-semibold uppercase tracking-wider text-[#64748b]">
                  Nome da Skill
                </label>
                <div className="flex gap-2">
                  <input
                    value={manualName}
                    onChange={(e) => setManualName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleManualInstall()}
                    placeholder="ex: docker-manager, api-tester, aws-tools"
                    className="flex-1 rounded-lg border border-[rgba(99,102,241,0.12)] bg-[#0b0d17] px-3 py-2.5 text-sm text-[#f1f5f9] placeholder:text-[#475569] outline-none transition-all focus:border-[#6366f1]/50"
                  />
                  <button
                    onClick={handleManualInstall}
                    disabled={!manualName.trim() || installing !== null}
                    className="flex cursor-pointer items-center gap-2 rounded-lg bg-[#6366f1] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[#818cf8] disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {installing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}
                    Instalar
                  </button>
                </div>
              </div>

              {manualMsg && (
                <div className={`flex items-center gap-2 rounded-lg px-3 py-2.5 text-xs border ${
                  manualMsg.type === 'success'
                    ? 'bg-[#10b981]/10 text-[#10b981] border-[#10b981]/20'
                    : 'bg-[#f43f5e]/10 text-[#fb7185] border-[#f43f5e]/20'
                }`}>
                  {manualMsg.type === 'success' ? <CheckCircle2 className="h-3.5 w-3.5 shrink-0" /> : <X className="h-3.5 w-3.5 shrink-0" />}
                  {manualMsg.text}
                </div>
              )}

              <div className="rounded-xl bg-[#0b0d17] px-4 py-3 border border-[rgba(99,102,241,0.12)]">
                <p className="text-[11px] font-semibold uppercase tracking-wider text-[#64748b]">Como funciona</p>
                <ul className="mt-2 space-y-1.5 text-xs text-[#94a3b8]">
                  <li className="flex items-start gap-2">
                    <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-[#64748b]" />
                    O DevClaw baixa o <code className="text-[#f1f5f9]">SKILL.md</code> do repositório no GitHub
                  </li>
                  <li className="flex items-start gap-2">
                    <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-[#64748b]" />
                    O arquivo é salvo em <code className="text-[#f1f5f9]">./skills/{'{nome}'}/SKILL.md</code>
                  </li>
                  <li className="flex items-start gap-2">
                    <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-[#64748b]" />
                    Reinicie o servidor para que a skill fique disponível
                  </li>
                </ul>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

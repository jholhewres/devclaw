import { useEffect, useState } from 'react'
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
  const [skills, setSkills] = useState<SkillInfo[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [showInstall, setShowInstall] = useState(false)

  useEffect(() => {
    api.skills.list()
      .then(setSkills)
      .catch(() => {})
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
      setSkills((prev) => [...prev, { name, description: 'Recem instalada', enabled: false, tool_count: 0 }])
    }
  }

  const enabledCount = skills.filter((s) => s.enabled).length

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-[#0a0a0f]">
        <div className="h-10 w-10 rounded-full border-4 border-orange-500/30 border-t-orange-500 animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto bg-[#0a0a0f]">
      <div className="mx-auto max-w-5xl px-8 py-10">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-gray-600">Gerenciar</p>
            <h1 className="mt-1 text-2xl font-black text-white tracking-tight">Skills</h1>
            <p className="mt-2 text-base text-gray-500">
              {enabledCount} ativas de {skills.length} disponiveis
            </p>
          </div>
          <button
            onClick={() => setShowInstall(true)}
            className="flex items-center gap-2 rounded-xl bg-orange-500 px-4 py-2.5 text-sm font-medium text-white shadow-lg shadow-orange-500/20 transition-all hover:bg-orange-400 hover:shadow-orange-500/30"
          >
            <Plus className="h-4 w-4" />
            Instalar Skill
          </button>
        </div>

        {/* Search */}
        <div className="relative mt-6">
          <Search className="absolute left-5 top-1/2 h-5 w-5 -translate-y-1/2 text-gray-600" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Buscar skills..."
            className="w-full rounded-2xl border border-white/[0.08] bg-[#111118] px-5 py-4 pl-14 text-base text-white outline-none placeholder:text-gray-600 transition-all focus:border-orange-500/30 focus:ring-2 focus:ring-orange-500/10"
          />
        </div>

        {/* Grid */}
        <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.map((skill) => (
            <div
              key={skill.name}
              className={`group relative overflow-hidden rounded-2xl border p-6 transition-all ${
                skill.enabled
                  ? 'border-orange-500/25 bg-orange-500/[0.04]'
                  : 'border-white/[0.06] bg-[#111118] hover:border-orange-500/15'
              }`}
            >
              {skill.enabled && (
                <div className="absolute right-4 top-4">
                  <span className="rounded-full bg-orange-500 px-2.5 py-0.5 text-[10px] font-bold text-white shadow-lg shadow-orange-500/30">ativa</span>
                </div>
              )}

              <div className={`flex h-14 w-14 items-center justify-center rounded-xl ${
                skill.enabled ? 'bg-orange-500/15 text-orange-400' : 'bg-white/[0.05] text-gray-500 group-hover:text-orange-400'
              } transition-colors`}>
                <Package className="h-7 w-7" />
              </div>

              <h3 className="mt-4 text-lg font-bold text-white">{skill.name}</h3>
              <p className="mt-2 text-sm leading-relaxed text-gray-400 line-clamp-2">{skill.description}</p>

              <div className="mt-4 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="flex items-center gap-1.5 rounded-full bg-white/[0.04] px-3 py-1 text-xs font-semibold text-gray-500">
                    <Wrench className="h-3 w-3" />
                    {skill.tool_count} ferramentas
                  </span>
                </div>
                <button
                  onClick={() => handleToggle(skill.name, skill.enabled)}
                  className="cursor-pointer text-gray-500 transition-colors hover:text-white"
                >
                  {skill.enabled ? (
                    <ToggleRight className="h-7 w-7 text-orange-400" />
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
            <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-white/[0.04]">
              <Zap className="h-8 w-8 text-gray-700" />
            </div>
            <p className="mt-4 text-lg font-semibold text-gray-500">
              {search ? 'Nenhuma skill encontrada' : 'Nenhuma skill disponivel'}
            </p>
          </div>
        )}
      </div>

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
  const [available, setAvailable] = useState<AvailableSkill[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [installing, setInstalling] = useState<string | null>(null)
  const [installed, setInstalled] = useState<Set<string>>(new Set())

  useEffect(() => {
    fetch('/api/skills/available', {
      headers: {
        Authorization: `Bearer ${localStorage.getItem('devclaw_token') || ''}`,
      },
    })
      .then((r) => r.json())
      .then((data: AvailableSkill[]) => {
        setAvailable(data)
        setInstalled(new Set(data.filter((s) => s.installed).map((s) => s.name)))
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

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

  const categories = [...new Set(available.map((s) => s.category).filter(Boolean))]

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="relative w-full max-w-2xl max-h-[80vh] overflow-hidden rounded-2xl border border-white/[0.08] bg-[#0f0f17] shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-white/[0.06] px-6 py-4">
          <div>
            <h2 className="text-lg font-bold text-white">Instalar Skills</h2>
            <p className="mt-0.5 text-xs text-zinc-500">{available.length} skills disponiveis no catalogo</p>
          </div>
          <button onClick={onClose} className="rounded-lg p-1.5 text-zinc-500 hover:bg-white/5 hover:text-white transition-colors">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Search */}
        <div className="border-b border-white/[0.06] px-6 py-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-600" />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Buscar por nome, descricao ou categoria..."
              autoFocus
              className="w-full rounded-lg border border-zinc-700/50 bg-zinc-800/50 px-3 py-2.5 pl-10 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-orange-500/50"
            />
          </div>
          {categories.length > 0 && !search && (
            <div className="mt-2 flex flex-wrap gap-1.5">
              {categories.map((cat) => (
                <button
                  key={cat}
                  onClick={() => setSearch(cat)}
                  className="rounded-full bg-zinc-800 px-2.5 py-1 text-[11px] font-medium text-zinc-400 hover:bg-zinc-700 hover:text-white transition-colors"
                >
                  {cat}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* List */}
        <div className="overflow-y-auto px-6 py-4" style={{ maxHeight: 'calc(80vh - 180px)' }}>
          {loading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-orange-400" />
            </div>
          ) : filtered.length === 0 ? (
            <div className="flex flex-col items-center py-12">
              <Package className="h-10 w-10 text-zinc-700" />
              <p className="mt-3 text-sm text-zinc-500">
                {search ? 'Nenhuma skill encontrada' : 'Catalogo vazio'}
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {filtered.map((skill) => {
                const isInstalled = installed.has(skill.name)
                const isInstalling = installing === skill.name

                return (
                  <div
                    key={skill.name}
                    className="flex items-center gap-4 rounded-xl border border-white/[0.04] bg-zinc-800/30 px-4 py-3 transition-colors hover:border-white/[0.08]"
                  >
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-white/[0.04]">
                      <Package className="h-5 w-5 text-zinc-500" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <h3 className="text-sm font-semibold text-white">{skill.name}</h3>
                        {skill.version && (
                          <span className="text-[10px] text-zinc-600">v{skill.version}</span>
                        )}
                        {skill.category && (
                          <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] font-medium text-zinc-500">{skill.category}</span>
                        )}
                      </div>
                      <p className="mt-0.5 text-xs text-zinc-500 line-clamp-1">{skill.description}</p>
                    </div>
                    <div className="shrink-0">
                      {isInstalled ? (
                        <span className="flex items-center gap-1 text-xs font-medium text-emerald-400">
                          <CheckCircle2 className="h-3.5 w-3.5" />
                          Instalada
                        </span>
                      ) : (
                        <button
                          onClick={() => handleInstall(skill.name)}
                          disabled={isInstalling}
                          className="flex items-center gap-1.5 rounded-lg bg-orange-500/10 px-3 py-1.5 text-xs font-medium text-orange-400 transition-colors hover:bg-orange-500/20 disabled:opacity-50"
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
      </div>
    </div>
  )
}

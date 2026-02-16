import { useEffect, useState } from 'react'
import { Search, ToggleLeft, ToggleRight, Zap, Package, Wrench } from 'lucide-react'
import { api, type SkillInfo } from '@/lib/api'

export function Skills() {
  const [skills, setSkills] = useState<SkillInfo[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)

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
        <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-gray-600">Gerenciar</p>
        <h1 className="mt-1 text-2xl font-black text-white tracking-tight">Skills</h1>
        <p className="mt-2 text-base text-gray-500">
          {enabledCount} ativas de {skills.length} disponiveis
        </p>

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

        {/* Grid — game-mode card style */}
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
              {/* Active badge */}
              {skill.enabled && (
                <div className="absolute right-4 top-4">
                  <span className="rounded-full bg-orange-500 px-2.5 py-0.5 text-[10px] font-bold text-white shadow-lg shadow-orange-500/30">ativa</span>
                </div>
              )}

              {/* Icon */}
              <div className={`flex h-14 w-14 items-center justify-center rounded-xl ${
                skill.enabled ? 'bg-orange-500/15 text-orange-400' : 'bg-white/[0.05] text-gray-500 group-hover:text-orange-400'
              } transition-colors`}>
                <Package className="h-7 w-7" />
              </div>

              {/* Content */}
              <h3 className="mt-4 text-lg font-bold text-white">{skill.name}</h3>
              <p className="mt-2 text-sm leading-relaxed text-gray-400 line-clamp-2">{skill.description}</p>

              {/* Bottom row */}
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
    </div>
  )
}

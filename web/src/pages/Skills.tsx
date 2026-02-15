import { useEffect, useState } from 'react'
import { Search, Puzzle, ToggleLeft, ToggleRight } from 'lucide-react'
import { api, type SkillInfo } from '@/lib/api'
import { Input } from '@/components/ui/Input'
import { Badge } from '@/components/ui/Badge'

/**
 * Skills Store — grid de skills com busca, toggle, e info.
 */
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

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-900 dark:border-zinc-700 dark:border-t-zinc-100" />
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-4xl px-6 py-8">
        <h1 className="text-xl font-semibold">Skills</h1>
        <p className="mt-1 text-sm text-zinc-500">
          Gerencie as habilidades do seu assistente
        </p>

        {/* Busca */}
        <div className="relative mt-6">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-400" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Buscar skills..."
            className="pl-9"
          />
        </div>

        {/* Grid */}
        <div className="mt-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.map((skill) => (
            <div
              key={skill.name}
              className="rounded-xl border border-zinc-200 p-4 transition-colors hover:border-zinc-300 dark:border-zinc-800 dark:hover:border-zinc-700"
            >
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-2">
                  <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-zinc-100 dark:bg-zinc-800">
                    <Puzzle className="h-4 w-4 text-zinc-500" />
                  </div>
                  <div>
                    <h3 className="text-sm font-medium">{skill.name}</h3>
                    <Badge variant={skill.enabled ? 'success' : 'default'} className="mt-0.5">
                      {skill.enabled ? 'Ativo' : 'Inativo'}
                    </Badge>
                  </div>
                </div>
                <button
                  onClick={() => handleToggle(skill.name, skill.enabled)}
                  className="text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300 transition-colors"
                >
                  {skill.enabled ? (
                    <ToggleRight className="h-5 w-5 text-emerald-500" />
                  ) : (
                    <ToggleLeft className="h-5 w-5" />
                  )}
                </button>
              </div>
              <p className="mt-2 text-xs text-zinc-500 line-clamp-2">{skill.description}</p>
              <p className="mt-2 text-[11px] text-zinc-400">{skill.tool_count} ferramentas</p>
            </div>
          ))}
        </div>

        {filtered.length === 0 && (
          <p className="mt-12 text-center text-sm text-zinc-400">
            {search ? 'Nenhuma skill encontrada' : 'Nenhuma skill disponível'}
          </p>
        )}
      </div>
    </div>
  )
}

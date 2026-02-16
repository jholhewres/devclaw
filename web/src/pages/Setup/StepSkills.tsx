import { useEffect, useState } from 'react'
import { Puzzle, Check, Search, Package, Code, Zap, Globe, Database, Wrench } from 'lucide-react'
import type { SetupData } from './SetupWizard'

/** Skill vinda do catálogo do repositório goclaw-skills */
interface CatalogSkill {
  name: string
  description: string
  category?: string
  version?: string
  tags?: string[]
  enabled: boolean
  tool_count: number
}

/** Mapa de categorias para ícone e label */
const CATEGORY_META: Record<string, { label: string; icon: React.FC<{ className?: string }>; color: string }> = {
  builtin:      { label: 'Integrado',      icon: Package,  color: 'text-emerald-400' },
  data:         { label: 'Dados',           icon: Globe,    color: 'text-orange-400' },
  development:  { label: 'Desenvolvimento', icon: Code,     color: 'text-violet-400' },
  productivity: { label: 'Produtividade',   icon: Zap,      color: 'text-amber-400' },
  media:        { label: 'Mídia',           icon: Puzzle,   color: 'text-pink-400' },
  infra:        { label: 'Infraestrutura',  icon: Database,  color: 'text-amber-400' },
  integration:  { label: 'Integração',      icon: Wrench,   color: 'text-orange-400' },
}

function getCategoryMeta(cat?: string) {
  return CATEGORY_META[cat ?? ''] ?? { label: cat ?? 'Outro', icon: Puzzle, color: 'text-zinc-400' }
}

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

/**
 * Etapa 5: Escolha de skills do catálogo goclaw-skills.
 */
export function StepSkills({ data, updateData }: Props) {
  const [skills, setSkills] = useState<CatalogSkill[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')

  useEffect(() => {
    fetch('/api/setup/skills')
      .then((r) => r.ok ? r.json() : [])
      .then((d) => setSkills(Array.isArray(d) ? d : []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const toggleSkill = (name: string) => {
    const current = data.enabledSkills
    const next = current.includes(name)
      ? current.filter((s) => s !== name)
      : [...current, name]
    updateData({ enabledSkills: next })
  }

  const selectAll = () => {
    updateData({ enabledSkills: skills.map((s) => s.name) })
  }

  const deselectAll = () => {
    updateData({ enabledSkills: [] })
  }

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-16">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-zinc-700 border-t-orange-500" />
        <p className="text-sm text-zinc-500">Carregando catálogo de skills...</p>
      </div>
    )
  }

  /* Agrupar skills por categoria */
  const filtered = filter
    ? skills.filter(
        (s) =>
          s.name.toLowerCase().includes(filter.toLowerCase()) ||
          s.description.toLowerCase().includes(filter.toLowerCase()) ||
          (s.tags ?? []).some((t) => t.toLowerCase().includes(filter.toLowerCase())),
      )
    : skills

  const grouped = filtered.reduce<Record<string, CatalogSkill[]>>((acc, sk) => {
    const cat = sk.category ?? 'other'
    ;(acc[cat] ??= []).push(sk)
    return acc
  }, {})

  /* Ordem fixa das categorias */
  const categoryOrder = ['builtin', 'data', 'development', 'productivity', 'media', 'infra', 'integration']
  const sortedCategories = Object.keys(grouped).sort(
    (a, b) => (categoryOrder.indexOf(a) === -1 ? 99 : categoryOrder.indexOf(a)) - (categoryOrder.indexOf(b) === -1 ? 99 : categoryOrder.indexOf(b)),
  )

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold text-white">Skills</h2>
          <p className="mt-1 text-sm text-zinc-400">
            Escolha as habilidades do catálogo oficial
          </p>
        </div>
        <div className="flex gap-2 text-xs">
          <button onClick={selectAll} className="cursor-pointer rounded-lg border border-zinc-700/50 bg-zinc-800/50 px-3 py-1.5 text-zinc-400 transition-colors hover:bg-zinc-700/50 hover:text-white">
            Todos
          </button>
          <button onClick={deselectAll} className="cursor-pointer rounded-lg border border-zinc-700/50 bg-zinc-800/50 px-3 py-1.5 text-zinc-400 transition-colors hover:bg-zinc-700/50 hover:text-white">
            Nenhum
          </button>
        </div>
      </div>

      {/* Search */}
      <div className="relative">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-500" />
        <input
          type="text"
          placeholder="Buscar skills..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="w-full rounded-xl border border-zinc-700/50 bg-zinc-800/50 py-2.5 pl-10 pr-4 text-sm text-white placeholder-zinc-500 outline-none transition-all focus:border-orange-500/50 focus:ring-2 focus:ring-orange-500/10"
        />
      </div>

      {skills.length === 0 ? (
        <div className="flex flex-col items-center gap-3 py-12">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-zinc-800/60 ring-1 ring-zinc-700/30">
            <Puzzle className="h-6 w-6 text-zinc-500" />
          </div>
          <p className="text-sm text-zinc-400">Nenhuma skill disponível no momento</p>
          <p className="text-xs text-zinc-500">Skills podem ser instaladas depois pelo chat ou CLI</p>
        </div>
      ) : (
        <div className="max-h-[340px] space-y-5 overflow-y-auto pr-1 scrollbar-thin scrollbar-track-transparent scrollbar-thumb-zinc-700/50">
          {sortedCategories.map((cat) => {
            const meta = getCategoryMeta(cat)
            const CatIcon = meta.icon

            return (
              <div key={cat}>
                {/* Category header */}
                <div className="mb-2 flex items-center gap-2">
                  <CatIcon className={`h-3.5 w-3.5 ${meta.color}`} />
                  <span className={`text-xs font-semibold uppercase tracking-wider ${meta.color}`}>
                    {meta.label}
                  </span>
                  <span className="text-xs text-zinc-600">({grouped[cat].length})</span>
                </div>

                {/* Skills grid */}
                <div className="grid grid-cols-2 gap-1.5">
                  {grouped[cat].map((skill) => {
                    const isActive = data.enabledSkills.includes(skill.name)

                    return (
                      <button
                        key={skill.name}
                        onClick={() => toggleSkill(skill.name)}
                        className={`flex w-full cursor-pointer items-center gap-2.5 rounded-lg border px-3 py-2 text-left transition-all ${
                          isActive
                            ? 'border-orange-500/40 bg-orange-500/5 ring-1 ring-orange-500/15'
                            : 'border-zinc-700/40 bg-zinc-800/20 hover:border-zinc-600 hover:bg-zinc-800/50'
                        }`}
                      >
                        {/* Checkbox */}
                        <div className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border transition-all ${
                          isActive
                            ? 'border-transparent bg-orange-500 text-white'
                            : 'border-zinc-600 bg-zinc-800'
                        }`}>
                          {isActive && <Check className="h-2.5 w-2.5" />}
                        </div>

                        {/* Info */}
                        <div className="min-w-0 flex-1">
                          <span className="text-xs font-medium text-white">{skill.name}</span>
                          <p className="truncate text-[10px] leading-tight text-zinc-500">{skill.description}</p>
                        </div>
                      </button>
                    )
                  })}
                </div>
              </div>
            )
          })}
        </div>
      )}

      {/* Counter */}
      <div className="flex items-center justify-between">
        <p className="text-xs text-zinc-500">
          {data.enabledSkills.length} de {skills.length} skill{skills.length !== 1 ? 's' : ''} selecionada{data.enabledSkills.length !== 1 ? 's' : ''}
        </p>
        {filtered.length !== skills.length && (
          <p className="text-xs text-zinc-600">
            Mostrando {filtered.length} de {skills.length}
          </p>
        )}
      </div>
    </div>
  )
}

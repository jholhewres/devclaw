import { useEffect, useState } from 'react'
import { Puzzle } from 'lucide-react'
import { api, type SkillInfo } from '@/lib/api'
import type { SetupData } from './SetupWizard'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

/**
 * Etapa 5: Escolha de skills para ativar.
 */
export function StepSkills({ data, updateData }: Props) {
  const [skills, setSkills] = useState<SkillInfo[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.skills.list()
      .then(setSkills)
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

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="h-5 w-5 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-900 dark:border-zinc-700 dark:border-t-zinc-100" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-medium">Skills</h2>
        <p className="mt-1 text-sm text-zinc-500">
          Ative as habilidades que o assistente terá
        </p>
      </div>

      <div className="max-h-[300px] space-y-2 overflow-y-auto">
        {skills.map((skill) => (
          <label
            key={skill.name}
            className={`flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-2.5 transition-colors ${
              data.enabledSkills.includes(skill.name)
                ? 'border-zinc-900 bg-zinc-50 dark:border-zinc-400 dark:bg-zinc-800'
                : 'border-zinc-200 hover:border-zinc-300 dark:border-zinc-700 dark:hover:border-zinc-600'
            }`}
          >
            <input
              type="checkbox"
              checked={data.enabledSkills.includes(skill.name)}
              onChange={() => toggleSkill(skill.name)}
              className="h-4 w-4 rounded border-zinc-300"
            />
            <div className="flex h-7 w-7 items-center justify-center rounded bg-zinc-100 dark:bg-zinc-800">
              <Puzzle className="h-3.5 w-3.5 text-zinc-500" />
            </div>
            <div className="min-w-0 flex-1">
              <span className="text-sm font-medium">{skill.name}</span>
              <p className="truncate text-xs text-zinc-500">{skill.description}</p>
            </div>
            <span className="shrink-0 text-[11px] text-zinc-400">
              {skill.tool_count} tools
            </span>
          </label>
        ))}

        {skills.length === 0 && (
          <p className="py-8 text-center text-sm text-zinc-400">
            Nenhuma skill disponível no momento
          </p>
        )}
      </div>
    </div>
  )
}

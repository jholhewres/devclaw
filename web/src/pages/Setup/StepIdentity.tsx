import { Input } from '@/components/ui/Input'
import type { SetupData } from './SetupWizard'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

const LANGUAGES = [
  { value: 'pt-BR', label: 'Português (Brasil)' },
  { value: 'en', label: 'English' },
  { value: 'es', label: 'Español' },
  { value: 'fr', label: 'Français' },
  { value: 'de', label: 'Deutsch' },
]

/**
 * Etapa 1: Nome do assistente, idioma e fuso horário.
 */
export function StepIdentity({ data, updateData }: Props) {
  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-medium">Identidade</h2>
        <p className="mt-1 text-sm text-zinc-500">
          Personalize seu assistente
        </p>
      </div>

      <div className="space-y-3">
        <div>
          <label className="mb-1 block text-sm font-medium">Nome do assistente</label>
          <Input
            value={data.name}
            onChange={(e) => updateData({ name: e.target.value })}
            placeholder="GoClaw"
          />
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium">Idioma</label>
          <select
            value={data.language}
            onChange={(e) => updateData({ language: e.target.value })}
            className="flex h-9 w-full rounded-lg border border-zinc-200 bg-white px-3 text-sm outline-none focus:ring-2 focus:ring-zinc-400 dark:border-zinc-700 dark:bg-zinc-900"
          >
            {LANGUAGES.map((lang) => (
              <option key={lang.value} value={lang.value}>
                {lang.label}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium">Fuso horário</label>
          <Input
            value={data.timezone}
            onChange={(e) => updateData({ timezone: e.target.value })}
            placeholder="America/Sao_Paulo"
          />
          <p className="mt-1 text-xs text-zinc-400">Detectado automaticamente</p>
        </div>
      </div>
    </div>
  )
}

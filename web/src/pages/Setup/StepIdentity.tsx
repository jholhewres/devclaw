import { User, Globe, Clock } from 'lucide-react'
import type { SetupData } from './SetupWizard'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

const LANGUAGES = [
  { value: 'pt-BR', label: 'Português (Brasil)', flag: '🇧🇷' },
  { value: 'en', label: 'English', flag: '🇺🇸' },
  { value: 'es', label: 'Español', flag: '🇪🇸' },
  { value: 'fr', label: 'Français', flag: '🇫🇷' },
  { value: 'de', label: 'Deutsch', flag: '🇩🇪' },
]

/**
 * Etapa 1: Nome do assistente, idioma e fuso horário.
 */
export function StepIdentity({ data, updateData }: Props) {
  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-lg font-semibold text-white">Identidade</h2>
        <p className="mt-1 text-sm text-zinc-400">
          Dê um nome e personalize seu assistente
        </p>
      </div>

      <div className="space-y-4">
        {/* Nome */}
        <div>
          <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <User className="h-3.5 w-3.5 text-zinc-500" />
            Nome do assistente
          </label>
          <input
            value={data.name}
            onChange={(e) => updateData({ name: e.target.value })}
            placeholder="DevClaw"
            className="flex h-11 w-full rounded-xl border border-zinc-700/50 bg-zinc-800/50 px-4 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-orange-500/50 focus:ring-2 focus:ring-orange-500/10"
          />
        </div>

        {/* Idioma */}
        <div>
          <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <Globe className="h-3.5 w-3.5 text-zinc-500" />
            Idioma
          </label>
          <select
            value={data.language}
            onChange={(e) => updateData({ language: e.target.value })}
            className="flex h-11 w-full cursor-pointer rounded-xl border border-zinc-700/50 bg-zinc-800/50 px-4 text-sm text-white outline-none transition-all focus:border-orange-500/50 focus:ring-2 focus:ring-orange-500/10"
          >
            {LANGUAGES.map((lang) => (
              <option key={lang.value} value={lang.value}>
                {lang.flag} {lang.label}
              </option>
            ))}
          </select>
        </div>

        {/* Fuso horário */}
        <div>
          <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <Clock className="h-3.5 w-3.5 text-zinc-500" />
            Fuso horário
          </label>
          <input
            value={data.timezone}
            onChange={(e) => updateData({ timezone: e.target.value })}
            placeholder="America/Sao_Paulo"
            className="flex h-11 w-full rounded-xl border border-zinc-700/50 bg-zinc-800/50 px-4 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-orange-500/50 focus:ring-2 focus:ring-orange-500/10"
          />
          <p className="mt-1.5 text-xs text-zinc-500">Detectado automaticamente do seu navegador</p>
        </div>
      </div>
    </div>
  )
}

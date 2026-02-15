import { Input } from '@/components/ui/Input'
import type { SetupData } from './SetupWizard'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

const MODES = [
  {
    value: 'relaxed' as const,
    label: 'Relaxado',
    description: 'Ferramentas executam sem pedir permissão',
  },
  {
    value: 'strict' as const,
    label: 'Estrito',
    description: 'Comandos potencialmente perigosos pedem aprovação',
  },
  {
    value: 'paranoid' as const,
    label: 'Paranóico',
    description: 'Todas as ações externas pedem aprovação',
  },
]

/**
 * Etapa 3: Password da web UI e modo de acesso.
 */
export function StepSecurity({ data, updateData }: Props) {
  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-medium">Segurança</h2>
        <p className="mt-1 text-sm text-zinc-500">
          Proteja o acesso ao seu assistente
        </p>
      </div>

      <div className="space-y-3">
        {/* Password */}
        <div>
          <label className="mb-1 block text-sm font-medium">Senha da Web UI</label>
          <Input
            type="password"
            value={data.webuiPassword}
            onChange={(e) => updateData({ webuiPassword: e.target.value })}
            placeholder="Defina uma senha para acessar o painel"
          />
          <p className="mt-1 text-xs text-zinc-400">
            Opcional para acesso local. Recomendado se expor na internet.
          </p>
        </div>

        {/* Modo de acesso */}
        <div>
          <label className="mb-2 block text-sm font-medium">Modo de acesso</label>
          <div className="space-y-2">
            {MODES.map((mode) => (
              <label
                key={mode.value}
                className={`flex cursor-pointer items-start gap-3 rounded-lg border px-4 py-3 transition-colors ${
                  data.accessMode === mode.value
                    ? 'border-zinc-900 bg-zinc-50 dark:border-zinc-400 dark:bg-zinc-800'
                    : 'border-zinc-200 hover:border-zinc-300 dark:border-zinc-700 dark:hover:border-zinc-600'
                }`}
              >
                <input
                  type="radio"
                  name="accessMode"
                  value={mode.value}
                  checked={data.accessMode === mode.value}
                  onChange={(e) => updateData({ accessMode: e.target.value as SetupData['accessMode'] })}
                  className="mt-0.5"
                />
                <div>
                  <span className="text-sm font-medium">{mode.label}</span>
                  <p className="text-xs text-zinc-500">{mode.description}</p>
                </div>
              </label>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

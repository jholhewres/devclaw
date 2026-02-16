import { Shield, ShieldCheck, ShieldAlert, Lock } from 'lucide-react'
import type { SetupData } from './SetupWizard'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

const MODES = [
  {
    value: 'relaxed' as const,
    label: 'Relaxado',
    description: 'Ferramentas executam sem pedir permissão. Ideal para uso pessoal.',
    icon: Shield,
    color: 'emerald',
  },
  {
    value: 'strict' as const,
    label: 'Estrito',
    description: 'Comandos potencialmente perigosos pedem aprovação antes de executar.',
    icon: ShieldCheck,
    color: 'blue',
  },
  {
    value: 'paranoid' as const,
    label: 'Paranóico',
    description: 'Todas as ações externas pedem aprovação. Máxima segurança.',
    icon: ShieldAlert,
    color: 'amber',
  },
]

const COLOR_MAP = {
  emerald: {
    active: 'border-emerald-500/50 bg-emerald-500/10 ring-1 ring-emerald-500/20',
    icon: 'text-emerald-400',
    dot: 'bg-emerald-400',
  },
  blue: {
    active: 'border-orange-500/50 bg-orange-500/10 ring-1 ring-orange-500/20',
    icon: 'text-orange-400',
    dot: 'bg-orange-400',
  },
  amber: {
    active: 'border-amber-500/50 bg-amber-500/10 ring-1 ring-amber-500/20',
    icon: 'text-amber-400',
    dot: 'bg-amber-400',
  },
}

/**
 * Etapa 3: Password da web UI e modo de acesso.
 */
export function StepSecurity({ data, updateData }: Props) {
  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-lg font-semibold text-white">Segurança</h2>
        <p className="mt-1 text-sm text-zinc-400">
          Proteja o acesso e defina o nível de controle das ferramentas
        </p>
      </div>

      <div className="space-y-5">
        {/* Password */}
        <div>
          <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <Lock className="h-3.5 w-3.5 text-zinc-500" />
            Senha da Web UI
          </label>
          <input
            type="password"
            value={data.webuiPassword}
            onChange={(e) => updateData({ webuiPassword: e.target.value })}
            placeholder="Defina uma senha para o painel"
            className="flex h-11 w-full rounded-xl border border-zinc-700/50 bg-zinc-800/50 px-4 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-orange-500/50 focus:ring-2 focus:ring-orange-500/10"
          />
          <p className="mt-1.5 text-xs text-zinc-500">
            Opcional para acesso local. Recomendado se expor na internet.
          </p>
        </div>

        {/* Modo de acesso */}
        <div>
          <label className="mb-3 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <Shield className="h-3.5 w-3.5 text-zinc-500" />
            Modo de acesso
          </label>
          <div className="space-y-2.5">
            {MODES.map((mode) => {
              const colors = COLOR_MAP[mode.color as keyof typeof COLOR_MAP]
              const isActive = data.accessMode === mode.value
              const Icon = mode.icon

              return (
                <button
                  key={mode.value}
                  onClick={() => updateData({ accessMode: mode.value })}
                  className={`flex w-full items-start gap-4 rounded-xl border px-4 py-3.5 text-left transition-all ${
                    isActive
                      ? colors.active
                      : 'border-zinc-700/50 bg-zinc-800/30 hover:border-zinc-600 hover:bg-zinc-800/60'
                  }`}
                >
                  <div className={`mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ${
                    isActive ? 'bg-white/5' : 'bg-zinc-800'
                  }`}>
                    <Icon className={`h-4 w-4 ${isActive ? colors.icon : 'text-zinc-500'}`} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-white">{mode.label}</span>
                      {isActive && (
                        <div className={`h-1.5 w-1.5 rounded-full ${colors.dot}`} />
                      )}
                    </div>
                    <p className="mt-0.5 text-xs text-zinc-400">{mode.description}</p>
                  </div>
                </button>
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}

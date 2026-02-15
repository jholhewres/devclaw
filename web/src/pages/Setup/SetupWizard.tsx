import { useState } from 'react'
import { CheckCircle2 } from 'lucide-react'
import { StepIdentity } from './StepIdentity'
import { StepProvider } from './StepProvider'
import { StepSecurity } from './StepSecurity'
import { StepChannels } from './StepChannels'
import { StepSkills } from './StepSkills'

/** Dados coletados no wizard */
export interface SetupData {
  /* Etapa 1: Identidade */
  name: string
  language: string
  timezone: string

  /* Etapa 2: Provider */
  provider: string
  apiKey: string
  model: string

  /* Etapa 3: Segurança */
  webuiPassword: string
  accessMode: 'relaxed' | 'strict' | 'paranoid'

  /* Etapa 4: Canais */
  channels: Record<string, boolean>

  /* Etapa 5: Skills */
  enabledSkills: string[]
}

const INITIAL_DATA: SetupData = {
  name: 'GoClaw',
  language: 'pt-BR',
  timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  provider: 'openai',
  apiKey: '',
  model: '',
  webuiPassword: '',
  accessMode: 'strict',
  channels: {},
  enabledSkills: [],
}

const STEPS = [
  { id: 1, label: 'Identidade' },
  { id: 2, label: 'Provider AI' },
  { id: 3, label: 'Segurança' },
  { id: 4, label: 'Canais' },
  { id: 5, label: 'Skills' },
]

/**
 * Wizard de setup em 5 etapas.
 * Cada etapa é um componente que recebe data + updateData.
 * Após finalizar, exibe tela de sucesso pedindo para reiniciar o servidor.
 */
export function SetupWizard() {
  const [step, setStep] = useState(1)
  const [data, setData] = useState<SetupData>(INITIAL_DATA)
  const [submitting, setSubmitting] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState('')

  const updateData = (partial: Partial<SetupData>) => {
    setData((prev) => ({ ...prev, ...partial }))
  }

  const next = () => setStep((s) => Math.min(s + 1, 5))
  const prev = () => setStep((s) => Math.max(s - 1, 1))

  const handleFinalize = async () => {
    setSubmitting(true)
    setError('')
    try {
      const res = await fetch('/api/setup/finalize', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      })
      if (res.ok) {
        setDone(true)
      } else {
        const body = await res.json().catch(() => ({}))
        setError(body.error || 'Falha ao salvar configuração')
      }
    } catch {
      setError('Erro de conexão')
    } finally {
      setSubmitting(false)
    }
  }

  // Tela de sucesso pós-setup
  if (done) {
    return (
      <div className="flex flex-col items-center gap-4 py-8 text-center">
        <CheckCircle2 className="h-12 w-12 text-emerald-500" />
        <h2 className="text-xl font-semibold">Configuração salva!</h2>
        <p className="text-sm text-zinc-500 max-w-sm">
          O arquivo <code className="rounded bg-zinc-100 px-1 dark:bg-zinc-800">config.yaml</code> foi
          criado. Reinicie o servidor para aplicar as configurações.
        </p>
        <div className="mt-2 rounded-lg bg-zinc-100 px-4 py-2 font-mono text-sm dark:bg-zinc-800">
          copilot serve
        </div>
        {data.webuiPassword && (
          <p className="mt-2 text-xs text-zinc-400">
            Após reiniciar, faça login com a senha definida na etapa de segurança.
          </p>
        )}
      </div>
    )
  }

  return (
    <div>
      {/* Stepper */}
      <div className="mb-6 flex items-center gap-2">
        {STEPS.map(({ id, label }) => (
          <div key={id} className="flex items-center gap-2">
            <div
              className={`flex h-7 w-7 items-center justify-center rounded-full text-xs font-medium transition-colors ${
                id === step
                  ? 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900'
                  : id < step
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                    : 'bg-zinc-100 text-zinc-400 dark:bg-zinc-800'
              }`}
            >
              {id < step ? '✓' : id}
            </div>
            <span className={`hidden text-xs sm:inline ${
              id === step ? 'text-zinc-900 dark:text-zinc-100 font-medium' : 'text-zinc-400'
            }`}>
              {label}
            </span>
            {id < 5 && <div className="h-px w-4 bg-zinc-200 dark:bg-zinc-700" />}
          </div>
        ))}
      </div>

      {/* Etapa atual */}
      <div className="min-h-[280px]">
        {step === 1 && <StepIdentity data={data} updateData={updateData} />}
        {step === 2 && <StepProvider data={data} updateData={updateData} />}
        {step === 3 && <StepSecurity data={data} updateData={updateData} />}
        {step === 4 && <StepChannels data={data} updateData={updateData} />}
        {step === 5 && <StepSkills data={data} updateData={updateData} />}
      </div>

      {/* Erro */}
      {error && (
        <p className="mt-2 text-sm text-red-500 dark:text-red-400">{error}</p>
      )}

      {/* Navegação */}
      <div className="mt-6 flex items-center justify-between">
        <button
          onClick={prev}
          disabled={step === 1}
          className="text-sm text-zinc-500 hover:text-zinc-700 disabled:opacity-0 dark:hover:text-zinc-300 transition-colors"
        >
          ← Voltar
        </button>

        <div className="flex items-center gap-3">
          <span className="text-xs text-zinc-400">Etapa {step} de 5</span>
          {step < 5 ? (
            <button
              onClick={next}
              className="rounded-lg bg-zinc-900 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800 transition-colors dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
            >
              Próximo →
            </button>
          ) : (
            <button
              onClick={handleFinalize}
              disabled={submitting}
              className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50 transition-colors"
            >
              {submitting ? 'Configurando...' : 'Finalizar ✓'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

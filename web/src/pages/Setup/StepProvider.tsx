import { useState } from 'react'
import { CheckCircle2, XCircle, Loader2 } from 'lucide-react'
import { Input } from '@/components/ui/Input'
import { Button } from '@/components/ui/Button'
import { api } from '@/lib/api'
import type { SetupData } from './SetupWizard'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

const PROVIDERS = [
  { value: 'openai', label: 'OpenAI', models: ['gpt-4o', 'gpt-4o-mini', 'gpt-4-turbo', 'o1', 'o3-mini'] },
  { value: 'anthropic', label: 'Anthropic', models: ['claude-sonnet-4-20250514', 'claude-3-5-haiku-20241022', 'claude-3-opus-20240229'] },
  { value: 'openrouter', label: 'OpenRouter', models: [] },
  { value: 'ollama', label: 'Ollama (local)', models: [] },
  { value: 'custom', label: 'Custom (OpenAI-compatible)', models: [] },
]

/**
 * Etapa 2: Escolha do provider AI, API key, modelo.
 */
export function StepProvider({ data, updateData }: Props) {
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ success: boolean; error?: string } | null>(null)

  const provider = PROVIDERS.find((p) => p.value === data.provider)

  const handleTest = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      const result = await api.setup.testProvider(data.provider, data.apiKey, data.model)
      setTestResult(result)
    } catch {
      setTestResult({ success: false, error: 'Falha ao testar conexão' })
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-medium">Provider AI</h2>
        <p className="mt-1 text-sm text-zinc-500">
          Configure o modelo de linguagem
        </p>
      </div>

      <div className="space-y-3">
        {/* Provider */}
        <div>
          <label className="mb-1 block text-sm font-medium">Provider</label>
          <select
            value={data.provider}
            onChange={(e) => {
              updateData({ provider: e.target.value, model: '' })
              setTestResult(null)
            }}
            className="flex h-9 w-full rounded-lg border border-zinc-200 bg-white px-3 text-sm outline-none focus:ring-2 focus:ring-zinc-400 dark:border-zinc-700 dark:bg-zinc-900"
          >
            {PROVIDERS.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
        </div>

        {/* API Key */}
        {data.provider !== 'ollama' && (
          <div>
            <label className="mb-1 block text-sm font-medium">API Key</label>
            <Input
              type="password"
              value={data.apiKey}
              onChange={(e) => {
                updateData({ apiKey: e.target.value })
                setTestResult(null)
              }}
              placeholder={data.provider === 'openai' ? 'sk-...' : 'Sua API key'}
            />
            <p className="mt-1 text-xs text-zinc-400">
              Será armazenada no vault criptografado
            </p>
          </div>
        )}

        {/* Modelo */}
        <div>
          <label className="mb-1 block text-sm font-medium">Modelo</label>
          {provider && provider.models.length > 0 ? (
            <select
              value={data.model}
              onChange={(e) => updateData({ model: e.target.value })}
              className="flex h-9 w-full rounded-lg border border-zinc-200 bg-white px-3 text-sm outline-none focus:ring-2 focus:ring-zinc-400 dark:border-zinc-700 dark:bg-zinc-900"
            >
              <option value="">Selecione um modelo</option>
              {provider.models.map((m) => (
                <option key={m} value={m}>{m}</option>
              ))}
            </select>
          ) : (
            <Input
              value={data.model}
              onChange={(e) => updateData({ model: e.target.value })}
              placeholder="Nome do modelo"
            />
          )}
        </div>

        {/* Testar conexão */}
        <div className="flex items-center gap-3 pt-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleTest}
            disabled={testing || !data.apiKey || !data.model}
          >
            {testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
            Testar conexão
          </Button>

          {testResult && (
            <div className="flex items-center gap-1.5 text-sm">
              {testResult.success ? (
                <>
                  <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                  <span className="text-emerald-600 dark:text-emerald-400">Conectado</span>
                </>
              ) : (
                <>
                  <XCircle className="h-4 w-4 text-red-500" />
                  <span className="text-red-600 dark:text-red-400">{testResult.error}</span>
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

import { useEffect, useState } from 'react'
import { Save, RotateCcw } from 'lucide-react'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/Button'

/**
 * Editor de configuração — visualização e edição do config.yaml.
 * Modo raw (YAML/JSON) com syntax highlighting.
 */
export function Config() {
  const [config, setConfig] = useState<Record<string, unknown> | null>(null)
  const [rawText, setRawText] = useState('')
  const [originalText, setOriginalText] = useState('')
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  useEffect(() => {
    api.config.get()
      .then((data) => {
        setConfig(data)
        const text = JSON.stringify(data, null, 2)
        setRawText(text)
        setOriginalText(text)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const hasChanges = rawText !== originalText

  const handleSave = async () => {
    setSaving(true)
    setMessage(null)
    try {
      const parsed = JSON.parse(rawText)
      await api.config.update(parsed)
      setConfig(parsed)
      setOriginalText(rawText)
      setMessage({ type: 'success', text: 'Configuração salva com sucesso' })
    } catch (err) {
      setMessage({
        type: 'error',
        text: err instanceof SyntaxError ? 'JSON inválido' : 'Erro ao salvar',
      })
    } finally {
      setSaving(false)
    }
  }

  const handleReset = () => {
    setRawText(originalText)
    setMessage(null)
  }

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-900 dark:border-zinc-700 dark:border-t-zinc-100" />
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-6 py-8">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-semibold">Configuração</h1>
            <p className="mt-1 text-sm text-zinc-500">
              Edite a configuração do assistente
            </p>
          </div>
          <div className="flex items-center gap-2">
            {hasChanges && (
              <Button variant="ghost" size="sm" onClick={handleReset}>
                <RotateCcw className="h-3.5 w-3.5" />
                Desfazer
              </Button>
            )}
            <Button
              size="sm"
              onClick={handleSave}
              disabled={!hasChanges || saving}
            >
              <Save className="h-3.5 w-3.5" />
              {saving ? 'Salvando...' : 'Salvar'}
            </Button>
          </div>
        </div>

        {message && (
          <div className={`mt-4 rounded-lg px-4 py-2 text-sm ${
            message.type === 'success'
              ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-400'
              : 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-400'
          }`}>
            {message.text}
          </div>
        )}

        {/* Editor */}
        <div className="mt-6 overflow-hidden rounded-xl border border-zinc-200 dark:border-zinc-800">
          <div className="border-b border-zinc-200 bg-zinc-50 px-4 py-2 dark:border-zinc-800 dark:bg-zinc-900">
            <span className="text-xs text-zinc-500">config.json</span>
          </div>
          <textarea
            value={rawText}
            onChange={(e) => setRawText(e.target.value)}
            className="w-full resize-none bg-zinc-950 p-4 font-mono text-[13px] text-zinc-100 outline-none"
            rows={Math.max(20, rawText.split('\n').length + 2)}
            spellCheck={false}
          />
        </div>

        {/* Preview de seções */}
        {config && !hasChanges && (
          <div className="mt-6 space-y-3">
            <h2 className="text-sm font-medium text-zinc-500">Seções</h2>
            <div className="grid gap-2 sm:grid-cols-2">
              {Object.keys(config).map((key) => (
                <div
                  key={key}
                  className="rounded-lg border border-zinc-200 px-3 py-2 text-sm dark:border-zinc-800"
                >
                  <span className="font-medium">{key}</span>
                  <span className="ml-2 text-xs text-zinc-400">
                    {typeof config[key] === 'object'
                      ? `${Object.keys(config[key] as object).length} campos`
                      : String(config[key])}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

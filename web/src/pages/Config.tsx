import { useEffect, useState } from 'react'
import { Save, RotateCcw } from 'lucide-react'
import { api } from '@/lib/api'

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
      setMessage({ type: 'success', text: 'Configuracao salva com sucesso' })
    } catch (err) {
      setMessage({
        type: 'error',
        text: err instanceof SyntaxError ? 'JSON invalido' : 'Erro ao salvar',
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
      <div className="flex flex-1 items-center justify-center bg-[#0a0a0f]">
        <div className="h-10 w-10 rounded-full border-4 border-orange-500/30 border-t-orange-500 animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-[#0a0a0f]">
      <div className="mx-auto w-full max-w-5xl flex-1 overflow-y-auto px-8 py-10">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-gray-600">Sistema</p>
            <h1 className="mt-1 text-2xl font-black text-white tracking-tight">Configuracao</h1>
            <p className="mt-2 text-base text-gray-500">Edite a configuracao do assistente</p>
          </div>
          <div className="flex items-center gap-3">
            {hasChanges && (
              <button
                onClick={handleReset}
                className="flex cursor-pointer items-center gap-2 rounded-xl border border-white/[0.08] bg-[#111118] px-5 py-3 text-sm font-semibold text-gray-400 transition-all hover:border-white/[0.12] hover:text-white"
              >
                <RotateCcw className="h-4 w-4" />
                Desfazer
              </button>
            )}
            <button
              onClick={handleSave}
              disabled={!hasChanges || saving}
              className="flex cursor-pointer items-center gap-2 rounded-xl bg-gradient-to-r from-orange-500 to-amber-500 px-5 py-3 text-sm font-bold text-white shadow-lg shadow-orange-500/20 transition-all hover:shadow-orange-500/30 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Save className="h-4 w-4" />
              {saving ? 'Salvando...' : 'Salvar'}
            </button>
          </div>
        </div>

        {/* Message */}
        {message && (
          <div className={`mt-6 rounded-2xl px-5 py-4 text-base ring-1 ${
            message.type === 'success'
              ? 'bg-emerald-500/5 text-emerald-400 ring-emerald-500/20'
              : 'bg-red-500/5 text-red-400 ring-red-500/20'
          }`}>
            {message.text}
          </div>
        )}

        {/* Editor */}
        <div className="mt-8 overflow-hidden rounded-2xl border border-white/[0.06]">
          <div className="flex items-center justify-between border-b border-white/[0.06] bg-[#111118] px-6 py-3">
            <span className="text-[11px] font-bold uppercase tracking-[0.15em] text-gray-600">config.json</span>
            {hasChanges && (
              <span className="rounded-full bg-orange-500/15 px-3 py-1 text-[10px] font-bold text-orange-400 ring-1 ring-orange-500/20">
                Modificado
              </span>
            )}
          </div>
          <textarea
            value={rawText}
            onChange={(e) => setRawText(e.target.value)}
            className="w-full resize-none bg-[#0a0a0f] p-6 font-mono text-sm leading-relaxed text-gray-300 outline-none"
            rows={Math.max(20, rawText.split('\n').length + 2)}
            spellCheck={false}
          />
        </div>

        {/* Sections preview */}
        {config && !hasChanges && (
          <div className="mt-8">
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-gray-600 mb-4">Secoes</p>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {Object.keys(config).map((key) => (
                <div
                  key={key}
                  className="rounded-2xl border border-white/[0.06] bg-[#111118] px-5 py-4"
                >
                  <span className="text-base font-bold text-white">{key}</span>
                  <p className="mt-1 text-sm text-gray-500">
                    {typeof config[key] === 'object'
                      ? `${Object.keys(config[key] as object).length} campos`
                      : String(config[key])}
                  </p>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Zap,
  Power,
  PowerOff,
  Trash2,
  CheckCircle2,
  XCircle,
  ChevronDown,
  ChevronRight,
  Info,
  Filter,
} from 'lucide-react'
import { api } from '@/lib/api'
import type { HookInfo, HookEventInfo } from '@/lib/api'

/**
 * Lifecycle hooks management page.
 */
export function Hooks() {
  const { t } = useTranslation()
  const [hooks, setHooks] = useState<HookInfo[]>([])
  const [events, setEvents] = useState<HookEventInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [view, setView] = useState<'hooks' | 'events'>('hooks')
  const [filterEvent, setFilterEvent] = useState<string>('')

  const loadData = useCallback(async () => {
    try {
      const data = await api.hooks.list()
      setHooks(data.hooks || [])
      setEvents(data.events || [])
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    loadData()
  }, [loadData])

  /** Toggle hook enabled status */
  const handleToggle = async (name: string, enabled: boolean) => {
    setMessage(null)
    try {
      await api.hooks.toggle(name, enabled)
      await loadData()
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    }
  }

  /** Remove a hook */
  const handleDelete = async (name: string) => {
    setMessage(null)
    try {
      await api.hooks.unregister(name)
      setMessage({ type: 'success', text: t('common.success') })
      await loadData()
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    }
  }

  /** Hooks filtered by selected event */
  const filteredHooks = filterEvent
    ? hooks.filter((h) => h.events.includes(filterEvent))
    : hooks

  /** Total active hooks count */
  const activeCount = hooks.filter((h) => h.enabled).length

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-[#0b0d17]">
        <div className="h-10 w-10 rounded-full border-4 border-[#1c1f3a] border-t-[#6366f1] animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-[#0b0d17]">
      <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-[#475569]">
              {t('hooks.subtitle')}
            </p>
            <h1 className="mt-1 text-2xl font-bold text-[#f1f5f9] tracking-tight">
              {t('hooks.title')}
            </h1>
            <p className="mt-2 text-base text-[#64748b]">
              {hooks.length} · {activeCount} {t('common.enabled').toLowerCase()}
            </p>
          </div>

          {/* View toggle */}
          <div className="flex items-center gap-1 rounded-xl border border-[rgba(99,102,241,0.12)] bg-[#14172b] p-1">
            <button
              onClick={() => setView('hooks')}
              className={`cursor-pointer rounded-lg px-3 py-1.5 text-xs font-medium transition-all ${
                view === 'hooks'
                  ? 'bg-[#6366f1] text-white'
                  : 'text-[#64748b] hover:text-[#f1f5f9]'
              }`}
            >
              Hooks
            </button>
            <button
              onClick={() => setView('events')}
              className={`cursor-pointer rounded-lg px-3 py-1.5 text-xs font-medium transition-all ${
                view === 'events'
                  ? 'bg-[#6366f1] text-white'
                  : 'text-[#64748b] hover:text-[#f1f5f9]'
              }`}
            >
              Eventos
            </button>
          </div>
        </div>

        {/* Message */}
        {message && (
          <div
            className={`mt-6 rounded-xl px-5 py-4 text-sm border ${
              message.type === 'success'
                ? 'bg-[#10b981]/10 text-[#10b981] border-[#10b981]/20'
                : 'bg-[#f43f5e]/10 text-[#fb7185] border-[#f43f5e]/20'
            }`}
          >
            {message.text}
          </div>
        )}

        {view === 'hooks' ? (
          <>
            {/* Filtro por evento */}
            {hooks.length > 0 && (
              <div className="mt-6 flex items-center gap-3">
                <Filter className="h-4 w-4 text-[#64748b]" />
                <select
                  value={filterEvent}
                  onChange={(e) => setFilterEvent(e.target.value)}
                  aria-label="Filtrar por evento"
                  className="h-9 cursor-pointer rounded-lg border border-[rgba(99,102,241,0.12)] bg-[#14172b] px-3 text-xs text-[#f1f5f9] outline-none transition-all hover:border-[rgba(99,102,241,0.24)] focus:border-[#6366f1]/50"
                >
                  <option value="">Todos os eventos</option>
                  {events
                    .filter((ev) => ev.hooks.length > 0)
                    .map((ev) => (
                      <option key={ev.event} value={ev.event}>
                        {ev.event} ({ev.hooks.length})
                      </option>
                    ))}
                </select>
                {filterEvent && (
                  <button
                    onClick={() => setFilterEvent('')}
                    className="cursor-pointer text-xs text-[#64748b] hover:text-[#f1f5f9] transition-colors"
                  >
                    Limpar filtro
                  </button>
                )}
              </div>
            )}

            {/* Lista de hooks */}
            <div className="mt-6">
              <div className="mb-5 flex items-center gap-3">
                <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-[#1c1f3a]">
                  <Zap className="h-4 w-4 text-[#64748b]" />
                </div>
                <div>
                  <h2 className="text-base font-semibold text-[#f1f5f9]">Hooks registrados</h2>
                  <p className="text-xs text-[#64748b]">
                    {filteredHooks.length === 0
                      ? 'Nenhum hook encontrado'
                      : `${filteredHooks.length} hook${filteredHooks.length > 1 ? 's' : ''}`}
                  </p>
                </div>
              </div>

              {filteredHooks.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-[rgba(99,102,241,0.12)] bg-[#14172b] px-8 py-14 text-center">
                  <Zap className="mx-auto h-10 w-10 text-[#475569]" />
                  <p className="mt-4 text-sm text-[#64748b]">
                    {filterEvent
                      ? `Nenhum hook registrado para o evento "${filterEvent}"`
                      : 'Nenhum hook registrado. Hooks são adicionados por plugins, skills e pelo sistema.'}
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {filteredHooks.map((hook) => (
                    <HookCard
                      key={hook.name}
                      hook={hook}
                      onToggle={handleToggle}
                      onDelete={handleDelete}
                    />
                  ))}
                </div>
              )}
            </div>
          </>
        ) : (
          /* Vista de eventos */
          <div className="mt-6 space-y-3">
            {events.map((ev) => (
              <EventCard
                key={ev.event}
                event={ev}
                onFilterByEvent={(event) => {
                  setFilterEvent(event)
                  setView('hooks')
                }}
              />
            ))}
          </div>
        )}

        {/* Info card */}
        <div className="mt-10 mb-6 rounded-2xl border border-[rgba(99,102,241,0.12)] bg-[#14172b] p-6">
          <h3 className="text-sm font-semibold text-[#94a3b8] mb-3">Sobre Hooks</h3>
          <p className="text-xs text-[#64748b] leading-relaxed">
            Hooks permitem que plugins, skills e o sistema observem e modifiquem o comportamento
            do agente em pontos específicos do ciclo de vida. Hooks com menor prioridade
            executam primeiro. Para eventos bloqueantes (<code className="text-[#94a3b8]">pre_tool_use</code>,{' '}
            <code className="text-[#94a3b8]">user_prompt_submit</code>), o primeiro hook que
            bloquear impede a operação.
          </p>
          <div className="mt-3 flex items-start gap-2 rounded-lg bg-[#1c1f3a] px-3 py-2">
            <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[#f59e0b]" />
            <p className="text-xs text-[#f59e0b]">
              Desativar hooks do sistema pode afetar funcionalidades essenciais. Use com cautela.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

/* ── Componente de Card de Hook ── */

function HookCard({
  hook,
  onToggle,
  onDelete,
}: {
  hook: HookInfo
  onToggle: (name: string, enabled: boolean) => void
  onDelete: (name: string) => void
}) {
  const [confirming, setConfirming] = useState(false)

  const sourceLabel = (source: string) => {
    if (!source || source === 'system') return 'Sistema'
    if (source.startsWith('plugin:')) return `Plugin: ${source.slice(7)}`
    if (source.startsWith('skill:')) return `Skill: ${source.slice(6)}`
    return source
  }

  const sourceColor = (source: string) => {
    if (!source || source === 'system') return 'text-[#f1f5f9] bg-[#1c1f3a]'
    if (source.startsWith('plugin:')) return 'text-[#f1f5f9] bg-[#1c1f3a]'
    if (source.startsWith('skill:')) return 'text-[#f1f5f9] bg-[#1c1f3a]'
    return 'text-[#94a3b8] bg-[#1c1f3a]'
  }

  return (
    <div
      className={`rounded-2xl border bg-[#14172b] p-5 transition-all ${
        hook.enabled ? 'border-[rgba(99,102,241,0.12)]' : 'border-[rgba(99,102,241,0.06)] opacity-60'
      }`}
    >
      {/* Linha superior */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2.5 mb-1.5">
            {hook.enabled ? (
              <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-[#10b981]" />
            ) : (
              <XCircle className="h-3.5 w-3.5 shrink-0 text-[#64748b]" />
            )}
            <span className="text-sm font-semibold text-[#f1f5f9] truncate">{hook.name}</span>
            <span
              className={`shrink-0 rounded-md px-2 py-0.5 text-[10px] font-medium ${sourceColor(
                hook.source
              )}`}
            >
              {sourceLabel(hook.source)}
            </span>
          </div>

          {hook.description && (
            <p className="text-xs text-[#94a3b8] mt-1">{hook.description}</p>
          )}
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {/* Prioridade */}
          <span
            className="flex h-8 items-center rounded-lg px-2 text-[11px] font-mono text-[#64748b]"
            title="Prioridade (menor = executa primeiro)"
          >
            P{hook.priority}
          </span>

          {/* Toggle */}
          <button
            onClick={() => onToggle(hook.name, !hook.enabled)}
            title={hook.enabled ? 'Desativar' : 'Ativar'}
            className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-[#64748b] transition-colors hover:bg-[#1c1f3a] hover:text-[#f1f5f9]"
          >
            {hook.enabled ? (
              <PowerOff className="h-4 w-4" />
            ) : (
              <Power className="h-4 w-4" />
            )}
          </button>

          {/* Excluir */}
          {confirming ? (
            <button
              onClick={() => {
                onDelete(hook.name)
                setConfirming(false)
              }}
              className="flex h-8 cursor-pointer items-center gap-1 rounded-lg bg-[#f43f5e]/10 px-2 text-xs font-medium text-[#fb7185] transition-colors hover:bg-[#f43f5e]/20"
            >
              Confirmar
            </button>
          ) : (
            <button
              onClick={() => {
                setConfirming(true)
                setTimeout(() => setConfirming(false), 3000)
              }}
              title="Remover"
              className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-[#64748b] transition-colors hover:bg-[#f43f5e]/10 hover:text-[#fb7185]"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      {/* Eventos */}
      <div className="mt-3 flex flex-wrap gap-1.5">
        {hook.events.map((event) => (
          <span
            key={event}
            className="rounded-md bg-[#1c1f3a] px-2 py-0.5 text-[11px] font-mono text-[#94a3b8]"
          >
            {event}
          </span>
        ))}
      </div>
    </div>
  )
}

/* ── Componente de Card de Evento ── */

function EventCard({
  event,
  onFilterByEvent,
}: {
  event: HookEventInfo
  onFilterByEvent: (event: string) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const hasHooks = event.hooks.length > 0

  return (
    <div className="rounded-2xl border border-[rgba(99,102,241,0.12)] bg-[#14172b] transition-all">
      <button
        onClick={() => setExpanded(!expanded)}
        aria-expanded={expanded}
        className="flex w-full cursor-pointer items-center justify-between px-5 py-4"
      >
        <div className="flex items-center gap-3">
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-[#64748b]" />
          ) : (
            <ChevronRight className="h-4 w-4 text-[#64748b]" />
          )}
          <div className="text-left">
            <code className="text-sm font-semibold text-[#f1f5f9]">{event.event}</code>
            <p className="text-xs text-[#64748b] mt-0.5">{event.description}</p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {hasHooks ? (
            <span className="rounded-full bg-[#6366f1]/20 px-2.5 py-0.5 text-[11px] font-medium text-[#6366f1]">
              {event.hooks.length} hook{event.hooks.length > 1 ? 's' : ''}
            </span>
          ) : (
            <span className="text-[11px] text-[#475569]">sem hooks</span>
          )}
        </div>
      </button>

      {expanded && hasHooks && (
        <div className="border-t border-[rgba(99,102,241,0.12)] px-5 py-3">
          <div className="space-y-1.5">
            {event.hooks.map((hookName) => (
              <div
                key={hookName}
                className="flex items-center justify-between rounded-lg bg-[#0b0d17] px-3 py-2"
              >
                <span className="text-xs font-mono text-[#f1f5f9]">{hookName}</span>
                <button
                  onClick={() => onFilterByEvent(event.event)}
                  className="cursor-pointer text-[11px] text-[#64748b] hover:text-[#f1f5f9] transition-colors"
                >
                  Ver na lista
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

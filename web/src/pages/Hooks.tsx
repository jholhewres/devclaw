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
import { cn } from '@/lib/utils'
import { Card } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { Tabs } from '@/components/ui/Tabs'
import { EmptyState } from '@/components/ui/EmptyState'
import { LoadingSpinner } from '@/components/ui/ConfigComponents'

/**
 * Lifecycle hooks management page.
 */
export function Hooks() {
  const { t } = useTranslation()
  const [hooks, setHooks] = useState<HookInfo[]>([])
  const [events, setEvents] = useState<HookEventInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [view, setView] = useState<string>('hooks')
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
    return <LoadingSpinner />
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-bg-main">
      <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-text-muted">
              {t('hooks.subtitle')}
            </p>
            <h1 className="mt-1 text-2xl font-bold text-text-primary tracking-tight">
              {t('hooks.title')}
            </h1>
            <p className="mt-2 text-base text-text-muted">
              {hooks.length} · {activeCount} {t('hooks.active')}
            </p>
          </div>

          {/* View toggle */}
          <Tabs
            tabs={[
              { id: 'hooks', label: t('hooks.tabHooks') },
              { id: 'events', label: t('hooks.tabEvents') },
            ]}
            activeTab={view}
            onChange={setView}
          />
        </div>

        {/* Message */}
        {message && (
          <div
            className={cn(
              'mt-6 rounded-xl px-5 py-4 text-sm border',
              message.type === 'success'
                ? 'bg-success-subtle text-success border-success/20'
                : 'bg-error-subtle text-error border-error/20'
            )}
          >
            {message.text}
          </div>
        )}

        {view === 'hooks' ? (
          <>
            {/* Filter by event */}
            {hooks.length > 0 && (
              <div className="mt-6 flex items-center gap-3">
                <Filter className="h-4 w-4 text-text-muted" />
                <select
                  value={filterEvent}
                  onChange={(e) => setFilterEvent(e.target.value)}
                  aria-label={t('hooks.allEvents')}
                  className="h-9 cursor-pointer rounded-lg border border-border bg-bg-surface px-3 text-xs text-text-primary outline-none transition-all hover:border-border-hover focus:border-brand/50"
                >
                  <option value="">{t('hooks.allEvents')}</option>
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
                    className="cursor-pointer text-xs text-text-muted hover:text-text-primary transition-colors"
                  >
                    {t('hooks.clearFilter')}
                  </button>
                )}
              </div>
            )}

            {/* Hook list */}
            <div className="mt-6">
              <div className="mb-5 flex items-center gap-3">
                <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-bg-subtle">
                  <Zap className="h-4 w-4 text-text-muted" />
                </div>
                <div>
                  <h2 className="text-base font-semibold text-text-primary">{t('hooks.tabHooks')} {t('hooks.registered')}</h2>
                  <p className="text-xs text-text-muted">
                    {filteredHooks.length === 0
                      ? t('hooks.noHooks')
                      : `${filteredHooks.length} hook${filteredHooks.length > 1 ? 's' : ''}`}
                  </p>
                </div>
              </div>

              {filteredHooks.length === 0 ? (
                <EmptyState
                  icon={<Zap className="h-6 w-6" />}
                  title={filterEvent ? t('hooks.noHooksForEvent', { event: filterEvent }) : t('hooks.noHooks')}
                  description={!filterEvent ? t('hooks.noHooksHint') : undefined}
                  className="rounded-2xl border border-dashed border-border bg-bg-surface"
                />
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
          /* Events view */
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
        <Card padding="lg" className="mt-10 mb-6 rounded-2xl">
          <h3 className="text-sm font-semibold text-text-secondary mb-3">{t('hooks.aboutTitle')}</h3>
          <p className="text-xs text-text-muted leading-relaxed">
            {t('hooks.aboutTip1')} {t('hooks.aboutTip2')} {t('hooks.aboutTip3')}
          </p>
          <div className="mt-3 flex items-start gap-2 rounded-lg bg-bg-subtle px-3 py-2">
            <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-warning" />
            <p className="text-xs text-warning">
              {t('hooks.aboutWarning')}
            </p>
          </div>
        </Card>
      </div>
    </div>
  )
}

/* -- Hook Card Component -- */

function HookCard({
  hook,
  onToggle,
  onDelete,
}: {
  hook: HookInfo
  onToggle: (name: string, enabled: boolean) => void
  onDelete: (name: string) => void
}) {
  const { t } = useTranslation()
  const [confirming, setConfirming] = useState(false)

  const sourceLabel = (source: string) => {
    if (!source || source === 'system') return t('hooks.sourceSystem')
    if (source.startsWith('plugin:')) return `${t('hooks.sourcePlugin')}: ${source.slice(7)}`
    if (source.startsWith('skill:')) return `${t('hooks.sourceSkill')}: ${source.slice(6)}`
    return source
  }

  return (
    <Card
      padding="md"
      className={cn(
        'rounded-2xl p-5 transition-all',
        !hook.enabled && 'opacity-60'
      )}
    >
      {/* Top row */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2.5 mb-1.5">
            {hook.enabled ? (
              <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-success" />
            ) : (
              <XCircle className="h-3.5 w-3.5 shrink-0 text-text-muted" />
            )}
            <span className="text-sm font-semibold text-text-primary truncate">{hook.name}</span>
            <Badge>{sourceLabel(hook.source)}</Badge>
          </div>

          {hook.description && (
            <p className="text-xs text-text-secondary mt-1">{hook.description}</p>
          )}
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {/* Priority */}
          <span
            className="flex h-8 items-center rounded-lg px-2 text-[11px] font-mono text-text-muted"
            title={t('hooks.priority')}
          >
            P{hook.priority}
          </span>

          {/* Toggle */}
          <button
            onClick={() => onToggle(hook.name, !hook.enabled)}
            title={hook.enabled ? t('common.disabled') : t('common.enabled')}
            className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
          >
            {hook.enabled ? (
              <PowerOff className="h-4 w-4" />
            ) : (
              <Power className="h-4 w-4" />
            )}
          </button>

          {/* Delete */}
          {confirming ? (
            <button
              onClick={() => {
                onDelete(hook.name)
                setConfirming(false)
              }}
              className="flex h-8 cursor-pointer items-center gap-1 rounded-lg bg-error-subtle px-2 text-xs font-medium text-error transition-colors hover:bg-error/20"
            >
              {t('common.confirm')}
            </button>
          ) : (
            <button
              onClick={() => {
                setConfirming(true)
                setTimeout(() => setConfirming(false), 3000)
              }}
              title={t('common.delete')}
              className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-text-muted transition-colors hover:bg-error-subtle hover:text-error"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      {/* Events */}
      <div className="mt-3 flex flex-wrap gap-1.5">
        {hook.events.map((event) => (
          <span
            key={event}
            className="rounded-md bg-bg-subtle px-2 py-0.5 text-[11px] font-mono text-text-secondary"
          >
            {event}
          </span>
        ))}
      </div>
    </Card>
  )
}

/* -- Event Card Component -- */

function EventCard({
  event,
  onFilterByEvent,
}: {
  event: HookEventInfo
  onFilterByEvent: (event: string) => void
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const hasHooks = event.hooks.length > 0

  return (
    <Card padding="sm" className="rounded-2xl transition-all p-0">
      <button
        onClick={() => setExpanded(!expanded)}
        aria-expanded={expanded}
        className="flex w-full cursor-pointer items-center justify-between px-5 py-4"
      >
        <div className="flex items-center gap-3">
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-text-muted" />
          ) : (
            <ChevronRight className="h-4 w-4 text-text-muted" />
          )}
          <div className="text-left">
            <code className="text-sm font-semibold text-text-primary">{event.event}</code>
            <p className="text-xs text-text-muted mt-0.5">{event.description}</p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {hasHooks ? (
            <Badge className="bg-brand-subtle text-brand">
              {event.hooks.length} hook{event.hooks.length > 1 ? 's' : ''}
            </Badge>
          ) : (
            <span className="text-[11px] text-text-muted">{t('hooks.noHooksAttached')}</span>
          )}
        </div>
      </button>

      {expanded && hasHooks && (
        <div className="border-t border-border px-5 py-3">
          <div className="space-y-1.5">
            {event.hooks.map((hookName) => (
              <div
                key={hookName}
                className="flex items-center justify-between rounded-lg bg-bg-main px-3 py-2"
              >
                <span className="text-xs font-mono text-text-primary">{hookName}</span>
                <button
                  onClick={() => onFilterByEvent(event.event)}
                  className="cursor-pointer text-[11px] text-text-muted hover:text-text-primary transition-colors"
                >
                  {t('hooks.viewInList')}
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </Card>
  )
}

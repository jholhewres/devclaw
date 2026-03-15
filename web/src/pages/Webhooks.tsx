import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Webhook,
  Plus,
  Trash2,
  Loader2,
  CheckCircle2,
  XCircle,
  Power,
  PowerOff,
  Copy,
  Check,
  AlertTriangle,
} from 'lucide-react'
import { api } from '@/lib/api'
import type { WebhookInfo } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Card } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { EmptyState } from '@/components/ui/EmptyState'
import { Button } from '@/components/ui/Button'
import { ConfigPage, LoadingSpinner } from '@/components/ui/ConfigComponents'

/**
 * Webhook management page.
 */
export function Webhooks() {
  const { t } = useTranslation()
  const [webhooks, setWebhooks] = useState<WebhookInfo[]>([])
  const [validEvents, setValidEvents] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  /* Create form */
  const [showForm, setShowForm] = useState(false)
  const [newUrl, setNewUrl] = useState('')
  const [selectedEvents, setSelectedEvents] = useState<string[]>([])
  const [creating, setCreating] = useState(false)

  /* Copied ID (for visual feedback) */
  const [copiedId, setCopiedId] = useState<string | null>(null)

  const loadWebhooks = useCallback(async () => {
    try {
      const data = await api.webhooks.list()
      setWebhooks(data.webhooks || [])
      setValidEvents(data.valid_events || [])
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    loadWebhooks()
  }, [loadWebhooks])

  /** Create a new webhook */
  const handleCreate = async () => {
    if (!newUrl.trim()) return
    setCreating(true)
    setMessage(null)
    try {
      await api.webhooks.create(newUrl.trim(), selectedEvents)
      setNewUrl('')
      setSelectedEvents([])
      setShowForm(false)
      setMessage({ type: 'success', text: t('common.success') })
      await loadWebhooks()
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    } finally {
      setCreating(false)
    }
  }

  /** Delete a webhook */
  const handleDelete = async (id: string) => {
    setMessage(null)
    try {
      await api.webhooks.delete(id)
      setMessage({ type: 'success', text: t('common.success') })
      await loadWebhooks()
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    }
  }

  /** Toggle webhook active status */
  const handleToggle = async (id: string, active: boolean) => {
    setMessage(null)
    try {
      await api.webhooks.toggle(id, active)
      await loadWebhooks()
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    }
  }

  /** Toggle event selection in form */
  const toggleEvent = (event: string) => {
    setSelectedEvents((prev) =>
      prev.includes(event) ? prev.filter((e) => e !== event) : [...prev, event]
    )
  }

  /** Copy webhook ID to clipboard */
  const copyId = async (id: string) => {
    try {
      await navigator.clipboard.writeText(id)
      setCopiedId(id)
      setTimeout(() => setCopiedId(null), 2000)
    } catch {
      /* clipboard not available */
    }
  }

  if (loading) {
    return <LoadingSpinner />
  }

  return (
    <ConfigPage
      title={t('webhooks.title')}
      subtitle={t('webhooks.subtitle')}
      description={t('webhooks.desc')}
      actions={
        <Button size="lg" onClick={() => setShowForm(!showForm)}>
          <Plus className="h-4 w-4" />
          {t('webhooks.addWebhook')}
        </Button>
      }
      message={message}
    >
        {/* Create form */}
        {showForm && (
          <Card padding="lg" className="mb-6">
            <h3 className="text-base font-semibold text-text-primary mb-5">{t('webhooks.newWebhook')}</h3>

            <div className="space-y-5">
              {/* URL */}
              <div>
                <label className="mb-2 block text-sm font-medium text-text-secondary">{t('webhooks.webhookUrl')}</label>
                <input
                  value={newUrl}
                  onChange={(e) => setNewUrl(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
                  placeholder={t('webhooks.urlPlaceholder')}
                  className="h-11 w-full rounded-xl border border-border bg-bg-surface px-4 text-sm text-text-primary outline-none placeholder:text-text-muted transition-all hover:border-border-hover focus:border-brand/50 focus:ring-1 focus:ring-brand/20"
                />
                <p className="mt-1.5 text-xs text-text-muted">
                  {t('webhooks.payloadHint')}
                </p>
              </div>

              {/* Events */}
              <div>
                <label className="mb-3 block text-sm font-medium text-text-secondary">{t('webhooks.selectEvents')}</label>
                <div className="flex flex-wrap gap-2">
                  {validEvents.map((event) => {
                    const isSelected = selectedEvents.includes(event)
                    return (
                      <button
                        key={event}
                        onClick={() => toggleEvent(event)}
                        className={cn(
                          'cursor-pointer rounded-lg px-3 py-1.5 text-xs font-medium transition-all',
                          isSelected
                            ? 'bg-brand-subtle text-brand'
                            : 'bg-bg-subtle text-text-muted hover:bg-bg-hover hover:text-text-primary'
                        )}
                      >
                        {event}
                      </button>
                    )
                  })}
                </div>
                {selectedEvents.length === 0 && (
                  <p className="mt-2 flex items-center gap-1.5 text-xs text-warning">
                    <AlertTriangle className="h-3 w-3" />
                    {t('webhooks.selectAtLeastOne')}
                  </p>
                )}
              </div>

              {/* Actions */}
              <div className="flex items-center justify-end gap-3 pt-2">
                <Button
                  variant="ghost"
                  onClick={() => {
                    setShowForm(false)
                    setNewUrl('')
                    setSelectedEvents([])
                  }}
                >
                  {t('common.cancel')}
                </Button>
                <Button
                  onClick={handleCreate}
                  disabled={creating || !newUrl.trim()}
                >
                  {creating ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Webhook className="h-4 w-4" />
                  )}
                  {creating ? t('common.loading') : t('webhooks.createWebhook')}
                </Button>
              </div>
            </div>
          </Card>
        )}

        {/* Webhook list */}
        <div>
          <div className="mb-5 flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-bg-subtle">
              <Webhook className="h-4 w-4 text-text-muted" />
            </div>
            <div>
              <h2 className="text-base font-semibold text-text-primary">{t('webhooks.title')} {t('webhooks.registered')}</h2>
              <p className="text-xs text-text-muted">
                {webhooks.length === 0
                  ? t('webhooks.noWebhooks')
                  : `${webhooks.length} webhook${webhooks.length > 1 ? 's' : ''}`}
              </p>
            </div>
          </div>

          {webhooks.length === 0 ? (
            <EmptyState
              icon={<Webhook className="h-6 w-6" />}
              title={t('webhooks.noWebhooks')}
              description={t('webhooks.noWebhooksHint')}
              action={
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowForm(true)}
                >
                  {t('webhooks.addWebhook')}
                </Button>
              }
              className="rounded-2xl border border-dashed border-border bg-bg-surface"
            />
          ) : (
            <div className="space-y-3">
              {webhooks.map((wh) => (
                <WebhookCard
                  key={wh.id}
                  webhook={wh}
                  copiedId={copiedId}
                  onToggle={handleToggle}
                  onDelete={handleDelete}
                  onCopyId={copyId}
                />
              ))}
            </div>
          )}
        </div>

        {/* Quick documentation */}
        <Card padding="lg" className="mt-10 mb-6">
          <h3 className="text-sm font-semibold text-text-secondary mb-3">{t('webhooks.availableEvents')}</h3>
          <div className="grid grid-cols-2 gap-y-2 gap-x-4">
            {validEvents.map((event) => (
              <div key={event} className="flex items-center gap-2">
                <div className="h-1.5 w-1.5 rounded-full bg-text-muted" />
                <code className="text-xs font-mono text-text-secondary">{event}</code>
              </div>
            ))}
          </div>
          <p className="mt-4 text-xs text-text-muted">
            {t('webhooks.payloadHint')}
          </p>
        </Card>
    </ConfigPage>
  )
}

/* -- Webhook Card Component -- */

function WebhookCard({
  webhook,
  copiedId,
  onToggle,
  onDelete,
  onCopyId,
}: {
  webhook: WebhookInfo
  copiedId: string | null
  onToggle: (id: string, active: boolean) => void
  onDelete: (id: string) => void
  onCopyId: (id: string) => void
}) {
  const { t } = useTranslation()
  const [confirming, setConfirming] = useState(false)

  return (
    <Card
      padding="md"
      className={cn(
        'p-5 transition-all',
        !webhook.active && 'opacity-60'
      )}
    >
      {/* Top row: status + actions */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1.5">
            {webhook.active ? (
              <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-success" />
            ) : (
              <XCircle className="h-3.5 w-3.5 shrink-0 text-text-muted" />
            )}
            <Badge variant={webhook.active ? 'success' : 'default'}>
              {webhook.active ? t('webhooks.active') : t('webhooks.inactive')}
            </Badge>
          </div>
          <p className="truncate font-mono text-sm text-text-primary" title={webhook.url}>
            {webhook.url}
          </p>
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {/* Toggle active/inactive */}
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onToggle(webhook.id, !webhook.active)}
            title={webhook.active ? t('common.disabled') : t('common.enabled')}
          >
            {webhook.active ? (
              <PowerOff className="h-4 w-4" />
            ) : (
              <Power className="h-4 w-4" />
            )}
          </Button>

          {/* Copy ID */}
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onCopyId(webhook.id)}
            title={t('webhooks.copyId')}
          >
            {copiedId === webhook.id ? (
              <Check className="h-4 w-4 text-success" />
            ) : (
              <Copy className="h-4 w-4" />
            )}
          </Button>

          {/* Delete */}
          {confirming ? (
            <Button
              variant="destructive-subtle"
              size="xs"
              onClick={() => {
                onDelete(webhook.id)
                setConfirming(false)
              }}
            >
              {t('common.confirm')}
            </Button>
          ) : (
            <Button
              variant="ghost"
              size="icon"
              onClick={() => {
                setConfirming(true)
                setTimeout(() => setConfirming(false), 3000)
              }}
              title={t('common.delete')}
              className="hover:bg-error-subtle hover:text-error"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>

      {/* Events */}
      <div className="mt-3 flex flex-wrap gap-1.5">
        {webhook.events && webhook.events.length > 0 ? (
          webhook.events.map((event) => (
            <span
              key={event}
              className="rounded-md bg-bg-subtle px-2 py-0.5 text-[11px] font-mono text-text-secondary"
            >
              {event}
            </span>
          ))
        ) : (
          <span className="text-[11px] text-text-muted italic">{t('webhooks.selectAtLeastOne')}</span>
        )}
      </div>

      {/* Meta: ID + created date */}
      <div className="mt-3 flex items-center gap-4 text-[11px] text-text-muted">
        <span>ID: {webhook.id}</span>
        <span>
          {t('webhooks.createdAtDate', {
            date: new Date(webhook.created_at).toLocaleDateString('en-US', {
              day: '2-digit',
              month: 'short',
              year: 'numeric',
              hour: '2-digit',
              minute: '2-digit',
            })
          })}
        </span>
      </div>
    </Card>
  )
}

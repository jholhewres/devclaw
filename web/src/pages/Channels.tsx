import { useEffect, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  AlertTriangle,
  QrCode,
  Smartphone,
  ArrowRight,
  Eye,
  EyeOff,
  Check,
  ExternalLink,
  RefreshCw,
  Plus,
  Trash2,
} from 'lucide-react'
import { api, type ChannelHealth } from '@/lib/api'
import { Button } from '@/components/ui/Button'
import { timeAgo, cn } from '@/lib/utils'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { StatusDot } from '@/components/ui/StatusDot'
import { Modal } from '@/components/ui/Modal'
import { Input } from '@/components/ui/Input'
import { LoadingSpinner } from '@/components/ui/ConfigComponents'

/* Channel metadata (icons, colors, descriptions, token hints) */
const CHANNEL_META: Record<string, {
  color: string
  icon: ReactNode
  descKey: string
  tokenHintKey: string
  tokenHintUrl?: string
  tokenFields: { key: string; labelKey: string; placeholder: string }[]
  managePath: string
}> = {
  whatsapp: {
    color: '#22c55e',
    icon: (
      <svg viewBox="0 0 24 24" className="h-6 w-6" fill="currentColor">
        <path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/>
      </svg>
    ),
    descKey: 'channelsPage.whatsappDesc',
    tokenHintKey: '',
    tokenFields: [],
    managePath: '/channels/whatsapp',
  },
  telegram: {
    color: '#3b82f6',
    icon: (
      <svg viewBox="0 0 24 24" className="h-6 w-6" fill="currentColor">
        <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.479.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/>
      </svg>
    ),
    descKey: 'channelsPage.telegramDesc',
    tokenHintKey: 'channelsPage.telegramTokenHint',
    tokenHintUrl: 'https://t.me/BotFather',
    tokenFields: [
      { key: 'token', labelKey: 'channelsPage.botToken', placeholder: '123456:ABC-DEF1234...' },
    ],
    managePath: '/channels/telegram',
  },
}

const CHANNEL_NAME_KEYS: Record<string, string> = {
  whatsapp: 'channels.whatsapp',
  telegram: 'channels.telegram',
  discord: 'channels.discord',
  slack: 'channels.slack',
}

type ParsedChannel = ChannelHealth & { baseType: string; instanceId: string }

const INSTANCE_ID_REGEX = /^[a-zA-Z0-9_-]{1,64}$/

/**
 * Channel management page.
 * Shows channels grouped by type, with instance management (add/delete).
 */
export function Channels() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [channels, setChannels] = useState<ChannelHealth[]>([])
  const [loading, setLoading] = useState(true)
  const [addModal, setAddModal] = useState<{ type: string } | null>(null)

  const loadChannels = () => {
    setLoading(true)
    api.channels.list()
      .then(setChannels)
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => { loadChannels() }, [])

  // Parse instance info from channel names like "whatsapp:business"
  const parseChannel = (ch: ChannelHealth): ParsedChannel => {
    const [baseType, ...rest] = ch.name.split(':')
    return { ...ch, baseType, instanceId: rest.join(':') }
  }

  const parsed = channels.map(parseChannel)

  // Group by channel type
  const grouped: Record<string, ParsedChannel[]> = {}
  for (const ch of parsed) {
    if (!grouped[ch.baseType]) grouped[ch.baseType] = []
    grouped[ch.baseType].push(ch)
  }

  // Ensure whatsapp and telegram always show, even with 0 instances
  for (const type of ['whatsapp', 'telegram']) {
    if (!grouped[type]) grouped[type] = []
  }

  // Sort: whatsapp first, telegram second, rest alphabetical
  const typeOrder = ['whatsapp', 'telegram']
  const sortedTypes = Object.keys(grouped).sort((a, b) => {
    const ai = typeOrder.indexOf(a)
    const bi = typeOrder.indexOf(b)
    if (ai >= 0 && bi >= 0) return ai - bi
    if (ai >= 0) return -1
    if (bi >= 0) return 1
    return a.localeCompare(b)
  })

  const navigateToChannel = (ch: ParsedChannel) => {
    const meta = CHANNEL_META[ch.baseType]
    const basePath = meta?.managePath ?? `/channels/${ch.baseType}`
    navigate(ch.instanceId ? `${basePath}/${ch.instanceId}` : basePath)
  }

  if (loading) {
    return <LoadingSpinner />
  }

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8">
      <PageHeader
        title={t('channels.title')}
        description={t('channelsPage.subtitle')}
      />

      <div className="mt-8 space-y-8">
        {sortedTypes.map((type) => {
          const meta = CHANNEL_META[type]
          const typeName = CHANNEL_NAME_KEYS[type] ? t(CHANNEL_NAME_KEYS[type]) : type
          const instances = grouped[type]

          return (
            <div key={type}>
              {/* Section header */}
              <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-2.5">
                  {meta && (
                    <div className="text-tertiary" style={{ color: meta.color }}>
                      {meta.icon}
                    </div>
                  )}
                  <h3 className="text-sm font-semibold text-primary">{typeName}</h3>
                  <span className="text-xs text-quaternary">
                    {instances.length > 0
                      ? `${instances.length} ${instances.length === 1 ? 'instance' : 'instances'}`
                      : ''}
                  </span>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setAddModal({ type })}
                >
                  <Plus className="h-3.5 w-3.5" />
                  {t('channelsPage.addInstance')}
                </Button>
              </div>

              {/* Instance cards */}
              <div className="space-y-3">
                {instances.length === 0 ? (
                  <Card padding="md" className="text-center py-6">
                    <p className="text-sm text-tertiary">{t(meta?.descKey ?? 'channelsPage.connect')}</p>
                    <Button
                      size="sm"
                      variant="outline"
                      className="mt-3"
                      onClick={() => setAddModal({ type })}
                    >
                      <Plus className="h-3.5 w-3.5" />
                      {t('channelsPage.addInstance')}
                    </Button>
                  </Card>
                ) : (
                  instances.map((ch) =>
                    type === 'whatsapp' ? (
                      <WhatsAppCard
                        key={ch.name}
                        channel={ch}
                        onNavigate={() => navigateToChannel(ch)}
                        onDeleted={loadChannels}
                      />
                    ) : (
                      <ConfigurableChannelCard
                        key={ch.name}
                        channel={ch}
                        onNavigate={() => navigateToChannel(ch)}
                        onSaved={loadChannels}
                        onDeleted={loadChannels}
                      />
                    )
                  )
                )}
              </div>
            </div>
          )
        })}
      </div>

      {/* Add Instance Modal */}
      {addModal && (
        <AddInstanceModal
          channelType={addModal.type}
          onClose={() => setAddModal(null)}
          onCreated={() => {
            setAddModal(null)
            loadChannels()
          }}
        />
      )}
    </div>
  )
}

/* ── Add Instance Modal ── */

function AddInstanceModal({
  channelType,
  onClose,
  onCreated,
}: {
  channelType: string
  onClose: () => void
  onCreated: () => void
}) {
  const { t } = useTranslation()
  const typeName = CHANNEL_NAME_KEYS[channelType] ? t(CHANNEL_NAME_KEYS[channelType]) : channelType
  const [instanceId, setInstanceId] = useState('')
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)

  const validate = (id: string): string => {
    if (!id.trim()) return t('channelsPage.instanceIdRequired')
    if (!INSTANCE_ID_REGEX.test(id.trim())) return t('channelsPage.instanceIdInvalid')
    return ''
  }

  const handleCreate = async () => {
    const id = instanceId.trim()
    const err = validate(id)
    if (err) {
      setError(err)
      return
    }
    setCreating(true)
    setError('')
    try {
      await api.channels.createInstance(channelType, id)
      onCreated()
    } catch (e) {
      setError(e instanceof Error ? e.message : t('common.error'))
    } finally {
      setCreating(false)
    }
  }

  return (
    <Modal
      isOpen
      onClose={onClose}
      title={t('channelsPage.addInstanceTitle', { channel: typeName })}
      description={t('channelsPage.addInstanceDesc', { channel: typeName })}
      size="sm"
      footer={
        <>
          <Button size="sm" variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            size="sm"
            onClick={handleCreate}
            disabled={!instanceId.trim() || creating}
          >
            {creating ? t('channelsPage.creating') : t('channelsPage.createInstance')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <label className="text-sm font-medium text-primary">
          {t('channelsPage.instanceIdLabel')}
        </label>
        <Input
          value={instanceId}
          onChange={(e) => {
            setInstanceId(e.target.value)
            setError('')
          }}
          placeholder={t('channelsPage.instanceIdPlaceholder')}
          onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
          autoFocus
        />
        <p className="text-xs text-tertiary">
          {t('channelsPage.instanceIdHint')}
        </p>
        {error && (
          <p className="text-xs text-fg-error-secondary">{error}</p>
        )}
      </div>
    </Modal>
  )
}

/* ── Delete Instance Confirmation ── */

function DeleteInstanceButton({
  channelType,
  instanceId,
  onDeleted,
}: {
  channelType: string
  instanceId: string
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const [showConfirm, setShowConfirm] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const handleDelete = async () => {
    setDeleting(true)
    try {
      await api.channels.deleteInstance(channelType, instanceId)
      setShowConfirm(false)
      onDeleted()
    } catch {
      // error silently — the reload will show the instance still exists
    } finally {
      setDeleting(false)
    }
  }

  return (
    <>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation()
          setShowConfirm(true)
        }}
        className="rounded-lg p-1.5 text-tertiary hover:text-fg-error-secondary hover:bg-bg-hover transition-colors"
        title={t('channelsPage.deleteInstance')}
      >
        <Trash2 className="h-3.5 w-3.5" />
      </button>

      {showConfirm && (
        <Modal
          isOpen
          onClose={() => setShowConfirm(false)}
          title={t('channelsPage.deleteInstanceTitle', { id: instanceId })}
          description={t('channelsPage.deleteInstanceDesc')}
          size="sm"
          footer={
            <>
              <Button size="sm" variant="outline" onClick={() => setShowConfirm(false)}>
                {t('common.cancel')}
              </Button>
              <Button
                size="sm"
                variant="destructive"
                onClick={handleDelete}
                disabled={deleting}
              >
                {deleting ? t('channelsPage.deleting') : t('common.delete')}
              </Button>
            </>
          }
        />
      )}
    </>
  )
}

/* ── WhatsApp Card ── */

function WhatsAppCard({
  channel,
  onNavigate,
  onDeleted,
}: {
  channel: ParsedChannel
  onNavigate: () => void
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const connected = channel.connected
  const hasLastMsg = channel.last_msg_at && channel.last_msg_at !== '0001-01-01T00:00:00Z'
  const meta = CHANNEL_META.whatsapp
  const instanceLabel = channel.instanceId
    ? ` (${channel.instanceId})`
    : ''

  return (
    <Card
      padding="lg"
      className={cn(
        'transition-all',
        connected ? 'border-success/20' : ''
      )}
    >
      <div className="flex items-start gap-4">
        <div className={cn(
          'flex h-12 w-12 shrink-0 items-center justify-center rounded-xl transition-colors',
          connected ? 'bg-success-primary' : 'bg-secondary'
        )}>
          <div className={cn(connected ? 'text-fg-success-secondary' : 'text-tertiary')}>
            {meta.icon}
          </div>
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-3">
            <h3 className="text-base font-semibold text-primary">
              {t('channels.whatsapp')}{instanceLabel}
            </h3>
            {channel.instanceId ? (
              <Badge variant="default" className="text-xs font-mono">
                {channel.instanceId}
              </Badge>
            ) : (
              <Badge variant="default" className="text-xs">
                {t('channelsPage.defaultInstance')}
              </Badge>
            )}
            <StatusDot
              status={connected ? 'online' : 'offline'}
              label={connected ? t('common.online') : t('common.offline')}
            />
          </div>

          <p className="mt-1 text-sm text-tertiary">
            {connected
              ? hasLastMsg
                ? `${t('channelsPage.manage')} - ${timeAgo(channel.last_msg_at, t)}`
                : t('common.connected')
              : t('channelsPage.connect')}
          </p>

          <div className="mt-4 flex items-center gap-3">
            <Button size="md" variant={connected ? 'outline' : 'default'} onClick={onNavigate}>
              {connected ? (
                <>
                  <Smartphone className="h-4 w-4" />
                  {t('channelsPage.manage')}
                </>
              ) : (
                <>
                  <QrCode className="h-4 w-4" />
                  {t('channelsPage.connect')}
                  <ArrowRight className="h-4 w-4" />
                </>
              )}
            </Button>

            {channel.error_count > 0 && (
              <Badge variant="warning" className="flex items-center gap-1.5 px-3 py-2">
                <AlertTriangle className="h-3.5 w-3.5" />
                {t('channelsPage.errorCount', { count: channel.error_count })}
              </Badge>
            )}

            {channel.instanceId && (
              <div className="ml-auto">
                <DeleteInstanceButton
                  channelType="whatsapp"
                  instanceId={channel.instanceId}
                  onDeleted={onDeleted}
                />
              </div>
            )}
          </div>
        </div>
      </div>
    </Card>
  )
}

/* ── Configurable Channel Card (Telegram, Discord, Slack) ── */

function ConfigurableChannelCard({
  channel,
  onNavigate,
  onSaved,
  onDeleted,
}: {
  channel: ParsedChannel
  onNavigate: () => void
  onSaved: () => void
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const meta = CHANNEL_META[channel.baseType]
  const connected = channel.connected
  const configured = channel.configured
  const hasLastMsg = channel.last_msg_at && channel.last_msg_at !== '0001-01-01T00:00:00Z'

  const baseName = CHANNEL_NAME_KEYS[channel.baseType]
    ? t(CHANNEL_NAME_KEYS[channel.baseType])
    : channel.baseType
  const instanceLabel = channel.instanceId ? ` (${channel.instanceId})` : ''
  const displayName = baseName + instanceLabel

  // Token form state
  const [tokens, setTokens] = useState<Record<string, string>>({})
  const [showTokens, setShowTokens] = useState<Record<string, boolean>>({})
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  const updateToken = (key: string, value: string) => {
    setTokens((prev) => ({ ...prev, [key]: value }))
    setSaved(false)
  }

  const toggleShow = (key: string) => {
    setShowTokens((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  const hasTokenInput = meta?.tokenFields.some((f) => tokens[f.key]?.trim())

  const handleSave = async () => {
    if (!hasTokenInput) return
    setSaving(true)
    try {
      const channelData: Record<string, string> = {}
      for (const field of meta.tokenFields) {
        const val = tokens[field.key]?.trim()
        if (val) channelData[field.key] = val
      }
      await api.config.update({
        channels: { [channel.name]: channelData },
      })
      setSaved(true)
      setTokens({})
      onSaved()
    } catch {
      // error handling
    } finally {
      setSaving(false)
    }
  }

  if (!meta) return null

  // Configured channel — show status card with navigation
  if (configured) {
    return (
      <Card
        padding="md"
        className={cn(
          'transition-all cursor-pointer hover:border-primary',
          !connected && 'opacity-80'
        )}
        onClick={onNavigate}
      >
        <div className="flex items-center gap-4">
          <div className={cn(
            'flex h-10 w-10 shrink-0 items-center justify-center rounded-lg transition-colors',
            connected ? 'bg-secondary' : 'bg-secondary/50'
          )} style={connected ? { color: meta.color } : undefined}>
            <div className={cn(!connected && 'text-tertiary')}>
              {meta.icon}
            </div>
          </div>

          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2.5">
              <h3 className="text-sm font-semibold text-primary">{displayName}</h3>
              {channel.instanceId ? (
                <Badge variant="default" className="text-xs font-mono">
                  {channel.instanceId}
                </Badge>
              ) : (
                <Badge variant="default" className="text-xs">
                  {t('channelsPage.defaultInstance')}
                </Badge>
              )}
              <StatusDot
                status={connected ? 'online' : 'offline'}
                label={connected ? t('common.online') : t('common.offline')}
              />
            </div>
            <p className="mt-0.5 text-xs text-tertiary">
              {connected
                ? hasLastMsg
                  ? `${t('channelsPage.manage')} - ${timeAgo(channel.last_msg_at, t)}`
                  : t('common.connected')
                : t('channelsPage.restartRequired')}
            </p>
          </div>

          <div className="flex items-center gap-3">
            {channel.error_count > 0 && (
              <span className="flex items-center gap-1 text-xs text-fg-warning-secondary">
                <AlertTriangle className="h-3.5 w-3.5" />
                {channel.error_count}
              </span>
            )}
            {configured && !connected && (
              <Badge variant="default" className="text-xs">
                <RefreshCw className="h-3 w-3 mr-1" />
                {t('channelsPage.restartRequired')}
              </Badge>
            )}
            {channel.instanceId && (
              <DeleteInstanceButton
                channelType={channel.baseType}
                instanceId={channel.instanceId}
                onDeleted={onDeleted}
              />
            )}
            <ArrowRight className="h-4 w-4 text-tertiary" />
          </div>
        </div>
      </Card>
    )
  }

  // Unconfigured channel — show token form
  return (
    <Card padding="lg" className="transition-all">
      <div className="flex items-start gap-4">
        <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-secondary text-tertiary">
          {meta.icon}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-3">
            <h3 className="text-base font-semibold text-primary">{displayName}</h3>
            {channel.instanceId ? (
              <Badge variant="default" className="text-xs font-mono">
                {channel.instanceId}
              </Badge>
            ) : (
              <Badge variant="default" className="text-xs">
                {t('channelsPage.defaultInstance')}
              </Badge>
            )}
            <Badge variant="default" className="text-xs">
              {t('channelsPage.notConfigured')}
            </Badge>
          </div>

          <p className="mt-1 text-sm text-tertiary">
            {t(meta.descKey)}
          </p>

          {/* Token inputs */}
          <div className="mt-4 space-y-3">
            {meta.tokenFields.map((field) => (
              <div key={field.key}>
                <label className="block text-xs font-medium text-secondary mb-1.5">
                  {t(field.labelKey)}
                </label>
                <div className="relative">
                  <input
                    type={showTokens[field.key] ? 'text' : 'password'}
                    value={tokens[field.key] || ''}
                    onChange={(e) => updateToken(field.key, e.target.value)}
                    placeholder={field.placeholder}
                    className="flex h-10 w-full rounded-lg border border-secondary bg-primary px-3 pr-10 text-sm text-primary placeholder:text-quaternary outline-none transition-all hover:border-primary focus:border-brand/50 focus:ring-1 focus:ring-brand/20 font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => toggleShow(field.key)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-tertiary hover:text-primary cursor-pointer"
                  >
                    {showTokens[field.key]
                      ? <EyeOff className="h-4 w-4" />
                      : <Eye className="h-4 w-4" />
                    }
                  </button>
                </div>
              </div>
            ))}
          </div>

          {/* Hint + actions */}
          <div className="mt-4 flex items-center justify-between gap-3">
            <div className="flex items-center gap-3">
              <Button
                size="md"
                disabled={!hasTokenInput || saving}
                onClick={handleSave}
              >
                {saving ? (
                  t('common.saving')
                ) : saved ? (
                  <>
                    <Check className="h-4 w-4" />
                    {t('channelsPage.tokenSaved')}
                  </>
                ) : (
                  t('common.save')
                )}
              </Button>

              {channel.instanceId && (
                <DeleteInstanceButton
                  channelType={channel.baseType}
                  instanceId={channel.instanceId}
                  onDeleted={onDeleted}
                />
              )}
            </div>

            {meta.tokenHintUrl && (
              <a
                href={meta.tokenHintUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1.5 text-xs text-tertiary hover:text-brand-tertiary transition-colors"
              >
                {t(meta.tokenHintKey)}
                <ExternalLink className="h-3 w-3" />
              </a>
            )}
          </div>
        </div>
      </div>
    </Card>
  )
}

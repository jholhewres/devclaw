import { useEffect, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  AlertTriangle,
  QrCode,
  Smartphone,
  ArrowRight,
  Clock,
  Eye,
  EyeOff,
  Check,
  ExternalLink,
  RefreshCw,
} from 'lucide-react'
import { api, type ChannelHealth } from '@/lib/api'
import { Button } from '@/components/ui/Button'
import { timeAgo, cn } from '@/lib/utils'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { StatusDot } from '@/components/ui/StatusDot'
import { LoadingSpinner } from '@/components/ui/ConfigComponents'

/* Channel metadata (icons, colors, descriptions, token hints) */
const CHANNEL_META: Record<string, {
  color: string
  icon: ReactNode
  descKey: string
  tokenHintKey: string
  tokenHintUrl?: string
  tokenFields: { key: string; labelKey: string; placeholder: string }[]
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
  },
}

/**
 * Channel management page.
 * Shows core channels (WhatsApp + Telegram) with status for configured ones
 * and token configuration forms for unconfigured ones.
 */
export function Channels() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [channels, setChannels] = useState<ChannelHealth[]>([])
  const [loading, setLoading] = useState(true)

  const loadChannels = () => {
    setLoading(true)
    api.channels.list()
      .then(setChannels)
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => { loadChannels() }, [])

  const whatsapp = channels.find((ch) => ch.name === 'whatsapp')
  const otherChannels = channels.filter((ch) => ch.name !== 'whatsapp')

  if (loading) {
    return <LoadingSpinner />
  }

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8 max-w-4xl mx-auto">
      <PageHeader
        title={t('channels.title')}
        description={t('channelsPage.subtitle')}
      />

      <div className="mt-8 space-y-4">
        {whatsapp && (
          <WhatsAppCard channel={whatsapp} onNavigate={() => navigate('/channels/whatsapp')} />
        )}
        {otherChannels.map((ch) => (
          <ConfigurableChannelCard key={ch.name} channel={ch} onSaved={loadChannels} />
        ))}
      </div>
    </div>
  )
}

/* -- WhatsApp Card -- */

function WhatsAppCard({ channel, onNavigate }: { channel: ChannelHealth; onNavigate: () => void }) {
  const { t } = useTranslation()
  const connected = channel.connected
  const hasLastMsg = channel.last_msg_at && channel.last_msg_at !== '0001-01-01T00:00:00Z'
  const meta = CHANNEL_META.whatsapp

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
          connected ? 'bg-success-subtle' : 'bg-bg-subtle'
        )}>
          <div className={cn(connected ? 'text-success' : 'text-text-muted')}>
            {meta.icon}
          </div>
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-3">
            <h3 className="text-base font-semibold text-text-primary">{t('channels.whatsapp')}</h3>
            <StatusDot
              status={connected ? 'online' : 'offline'}
              label={connected ? t('common.online') : t('common.offline')}
            />
          </div>

          <p className="mt-1 text-sm text-text-muted">
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
          </div>
        </div>
      </div>
    </Card>
  )
}

/* -- Configurable Channel Card (Telegram) -- */

function ConfigurableChannelCard({ channel, onSaved }: { channel: ChannelHealth; onSaved: () => void }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const meta = CHANNEL_META[channel.name]
  const connected = channel.connected
  const configured = channel.configured
  const hasLastMsg = channel.last_msg_at && channel.last_msg_at !== '0001-01-01T00:00:00Z'

  const channelNameKeys: Record<string, string> = {
    telegram: 'channels.telegram',
  }
  const displayName = channelNameKeys[channel.name] ? t(channelNameKeys[channel.name]) : channel.name

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
      // error handling — toast would go here
    } finally {
      setSaving(false)
    }
  }

  if (!meta) return null

  // Configured channel — show status card with navigation to management page
  if (configured) {
    return (
      <Card
        padding="md"
        className={cn(
          'transition-all cursor-pointer hover:border-border-hover',
          !connected && 'opacity-80'
        )}
        onClick={() => navigate(`/channels/${channel.name}`)}
      >
        <div className="flex items-center gap-4">
          <div className={cn(
            'flex h-10 w-10 shrink-0 items-center justify-center rounded-lg transition-colors',
            connected ? 'bg-bg-subtle' : 'bg-bg-subtle/50'
          )} style={connected ? { color: meta.color } : undefined}>
            <div className={cn(!connected && 'text-text-muted')}>
              {meta.icon}
            </div>
          </div>

          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2.5">
              <h3 className="text-sm font-semibold text-text-primary">{displayName}</h3>
              <StatusDot
                status={connected ? 'online' : 'offline'}
                label={connected ? t('common.online') : t('common.offline')}
              />
            </div>
            <p className="mt-0.5 text-xs text-text-muted">
              {connected
                ? hasLastMsg
                  ? `${t('channelsPage.manage')} - ${timeAgo(channel.last_msg_at, t)}`
                  : t('common.connected')
                : t('channelsPage.restartRequired')}
            </p>
          </div>

          <div className="flex items-center gap-3">
            {channel.error_count > 0 && (
              <span className="flex items-center gap-1 text-xs text-warning">
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
            <ArrowRight className="h-4 w-4 text-text-muted" />
          </div>
        </div>
      </Card>
    )
  }

  // Unconfigured channel — show token form
  return (
    <Card padding="lg" className="transition-all">
      <div className="flex items-start gap-4">
        <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-bg-subtle text-text-muted">
          {meta.icon}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-3">
            <h3 className="text-base font-semibold text-text-primary">{displayName}</h3>
            <Badge variant="default" className="text-xs">
              {t('channelsPage.notConfigured')}
            </Badge>
          </div>

          <p className="mt-1 text-sm text-text-muted">
            {t(meta.descKey)}
          </p>

          {/* Token inputs */}
          <div className="mt-4 space-y-3">
            {meta.tokenFields.map((field) => (
              <div key={field.key}>
                <label className="block text-xs font-medium text-text-secondary mb-1.5">
                  {t(field.labelKey)}
                </label>
                <div className="relative">
                  <input
                    type={showTokens[field.key] ? 'text' : 'password'}
                    value={tokens[field.key] || ''}
                    onChange={(e) => updateToken(field.key, e.target.value)}
                    placeholder={field.placeholder}
                    className="flex h-10 w-full rounded-lg border border-border bg-bg-main px-3 pr-10 text-sm text-text-primary placeholder:text-text-muted outline-none transition-all hover:border-border-hover focus:border-brand/50 focus:ring-1 focus:ring-brand/20 font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => toggleShow(field.key)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary cursor-pointer"
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
            </div>

            {meta.tokenHintUrl && (
              <a
                href={meta.tokenHintUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1.5 text-xs text-text-muted hover:text-brand transition-colors"
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

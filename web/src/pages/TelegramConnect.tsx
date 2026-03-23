import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeft,
  Settings,
  Monitor,
  CheckCircle2,
  WifiOff,
  XCircle,
  Eye,
  EyeOff,
  Trash2,
  Users,
  UserCircle2,
  Loader2,
} from 'lucide-react'
import { api, type TelegramConfig, type TelegramAccessConfig } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/Button'
import { Toggle } from '@/components/ui/Toggle'
import { Select } from '@/components/ui/Select'
import { Input } from '@/components/ui/Input'
import { Tabs } from '@/components/ui/Tabs'
import { Card } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { Modal } from '@/components/ui/Modal'
import { StatusDot } from '@/components/ui/StatusDot'

type Tab = 'connection' | 'access' | 'settings'

const tabFromHash = (): Tab => {
  const hash = window.location.hash.replace('#', '')
  if (['connection', 'access', 'settings'].includes(hash)) {
    return hash as Tab
  }
  return 'connection'
}

export function TelegramConnect() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { instanceId } = useParams<{ instanceId?: string }>()
  const instanceLabel = instanceId ? ` (${instanceId})` : ''
  const [activeTab, setActiveTab] = useState<Tab>(tabFromHash())
  const [config, setConfig] = useState<TelegramConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const loadConfig = () => {
    setLoading(true)
    setError('')
    if (instanceId) {
      api.channels.instanceStatus('telegram', instanceId)
        .then((status) => {
          setConfig({
            configured: true,
            connected: status.connected,
            bot_username: status.phone || '',
            respond_to_groups: true,
            respond_to_dms: true,
            send_typing: true,
            reaction_notifications: 'off',
            allowed_chats: [],
          })
        })
        .catch((err) => {
          setError(err instanceof Error ? err.message : t('common.error'))
        })
        .finally(() => setLoading(false))
    } else {
      api.channels.telegram
        .getConfig()
        .then(setConfig)
        .catch((err) => {
          setError(err instanceof Error ? err.message : t('common.error'))
        })
        .finally(() => setLoading(false))
    }
  }

  useEffect(() => {
    loadConfig()
  }, [])

  const handleTabChange = (tabId: string) => {
    setActiveTab(tabId as Tab)
    window.location.hash = tabId
  }

  const tabs = [
    { id: 'connection', label: t('telegram.tabs.connection'), icon: <Monitor className="h-4 w-4" /> },
    { id: 'access', label: t('telegram.tabs.access'), icon: <Users className="h-4 w-4" /> },
    { id: 'settings', label: t('telegram.tabs.settings'), icon: <Settings className="h-4 w-4" /> },
  ]

  return (
    <div className="flex-1 overflow-y-auto bg-primary">
      <div className="mx-auto max-w-3xl px-6 py-8">
        {/* Back + Header */}
        <div className="flex flex-col gap-3">
          <button
            onClick={() => navigate('/channels')}
            className="flex items-center gap-1.5 text-sm text-tertiary hover:text-primary transition-colors cursor-pointer w-fit"
          >
            <ArrowLeft className="h-4 w-4" />
            {t('channelsPage.backToChannels')}
          </button>
          <div className="flex flex-col gap-1">
            <h2 className="text-lg font-medium text-primary">{t('telegram.title')}{instanceLabel}</h2>
            <p className="text-sm text-tertiary">{t('telegram.subtitle')}</p>
          </div>
        </div>

        {/* Tabs */}
        <div className="mt-6">
          <Tabs tabs={tabs} activeTab={activeTab} onChange={handleTabChange} />
        </div>

        {/* Tab Content */}
        <div className="mt-6">
          {activeTab === 'connection' && (
            <ConnectionTab config={config} loading={loading} error={error} onReload={loadConfig} instanceId={instanceId} />
          )}
          {activeTab === 'access' && <AccessTab />}
          {activeTab === 'settings' && (
            <SettingsTab config={config} onConfigChange={setConfig} />
          )}
        </div>
      </div>
    </div>
  )
}

/* ── Connection Tab ── */

function ConnectionTab({
  config,
  loading,
  error,
  onReload,
  instanceId,
}: {
  config: TelegramConfig | null
  loading: boolean
  error: string
  onReload: () => void
  instanceId?: string
}) {
  const { t } = useTranslation()
  const [token, setToken] = useState('')
  const [showToken, setShowToken] = useState(false)
  const [connecting, setConnecting] = useState(false)
  const [disconnecting, setDisconnecting] = useState(false)
  const [actionError, setActionError] = useState('')

  const handleConnect = async () => {
    if (!token.trim()) return
    setConnecting(true)
    setActionError('')
    try {
      await api.channels.telegram.connect(token.trim())
      setToken('')
      onReload()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t('common.error'))
    } finally {
      setConnecting(false)
    }
  }

  const handleDisconnect = async () => {
    setDisconnecting(true)
    setActionError('')
    try {
      if (instanceId) {
        await api.channels.instanceDisconnect('telegram', instanceId)
      } else {
        await api.channels.telegram.disconnect()
      }
      onReload()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t('common.error'))
    } finally {
      setDisconnecting(false)
    }
  }

  if (loading) {
    return (
      <div className="flex flex-col items-center gap-4 py-16">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-bg-subtle border-t-brand" />
        <p className="text-sm text-tertiary">{t('common.loading')}</p>
      </div>
    )
  }

  if (error) {
    return (
      <Card className="p-8 text-center">
        <XCircle className="mx-auto h-10 w-10 text-fg-error-secondary" />
        <p className="mt-3 text-sm text-primary">{t('telegram.connectionError')}</p>
        <p className="mt-1 text-xs text-tertiary">{error}</p>
        <Button size="sm" variant="outline" className="mt-4" onClick={onReload}>
          {t('common.retry')}
        </Button>
      </Card>
    )
  }

  if (!config) return null

  // Not configured — show token input form
  if (!config.configured && !config.connected) {
    return (
      <div className="flex flex-col gap-6">
        <Card className="p-6">
          <div className="flex items-center gap-4 mb-6">
            <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-secondary">
              <WifiOff className="h-7 w-7 text-tertiary" />
            </div>
            <div>
              <h3 className="text-base font-semibold text-primary">
                {t('telegram.notConfigured')}
              </h3>
              <p className="mt-1 text-sm text-tertiary">
                {t('telegram.notConfiguredDesc')}
              </p>
            </div>
          </div>

          <div className="flex flex-col gap-3">
            <label className="text-sm font-medium text-primary">
              {t('telegram.tokenLabel')}
            </label>
            <div className="relative">
              <input
                type={showToken ? 'text' : 'password'}
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder={t('telegram.tokenPlaceholder')}
                className="w-full rounded-lg border border-secondary bg-primary px-3 py-2.5 pr-10 text-sm text-primary placeholder:text-tertiary focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand"
                onKeyDown={(e) => e.key === 'Enter' && handleConnect()}
              />
              <button
                type="button"
                onClick={() => setShowToken(!showToken)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-tertiary hover:text-primary"
              >
                {showToken ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            <p className="text-xs text-tertiary">
              {t('telegram.tokenHint')}
            </p>

            {actionError && (
              <p className="text-xs text-fg-error-secondary">{actionError}</p>
            )}

            <Button
              size="sm"
              onClick={handleConnect}
              disabled={!token.trim() || connecting}
              className="w-fit"
            >
              {connecting ? t('telegram.connecting') : t('telegram.connect')}
            </Button>
          </div>
        </Card>
      </div>
    )
  }

  // Configured — show connection status
  return (
    <div className="flex flex-col gap-6">
      <Card className="p-6">
        <div className="flex items-center gap-4">
          <div className={cn(
            'flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl',
            config.connected ? 'bg-success-primary' : 'bg-secondary'
          )}>
            {config.connected ? (
              <CheckCircle2 className="h-7 w-7 text-fg-success-secondary" />
            ) : (
              <WifiOff className="h-7 w-7 text-tertiary" />
            )}
          </div>

          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-3">
              <h3 className="text-base font-semibold text-primary">
                {config.connected ? t('telegram.connected') : t('telegram.disconnectedConfigured')}
              </h3>
              <StatusDot
                status={config.connected ? 'online' : 'offline'}
                label={config.connected ? t('common.online') : t('common.offline')}
              />
            </div>

            {config.connected && config.bot_username && (
              <p className="mt-1 text-sm text-tertiary">
                @{config.bot_username}
              </p>
            )}

            {!config.connected && (
              <p className="mt-1 text-sm text-tertiary">
                {t('telegram.disconnectedConfiguredHint')}
              </p>
            )}
          </div>
        </div>

        {actionError && (
          <p className="mt-3 text-xs text-fg-error-secondary">{actionError}</p>
        )}

        <div className="mt-4 flex items-center gap-3">
          <Button
            size="sm"
            variant="outline"
            onClick={handleDisconnect}
            disabled={disconnecting}
          >
            <Trash2 className="h-3.5 w-3.5 mr-1.5" />
            {disconnecting ? t('telegram.removing') : t('telegram.removeToken')}
          </Button>
        </div>
      </Card>

      {/* Info */}
      <Card className="p-6">
        <h3 className="mb-3 text-sm font-semibold text-primary">{t('telegram.info')}</h3>
        <div className="space-y-2 text-sm text-tertiary">
          <p>{t('telegram.infoToken')}</p>
          <p>{t('telegram.infoRestart')}</p>
        </div>
      </Card>
    </div>
  )
}

/* ── Access Tab ── */

type UserLevel = 'owner' | 'admin' | 'user' | 'blocked'

function AccessTab() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<TelegramAccessConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [showAddModal, setShowAddModal] = useState(false)

  const levelOptions = [
    { value: 'admin', label: t('telegram.access.admin') },
    { value: 'user', label: t('telegram.access.allowed') },
    { value: 'blocked', label: t('telegram.access.blocked') },
  ]

  const loadConfig = useCallback(() => {
    setLoading(true)
    api.channels.telegram
      .getAccess()
      .then(setConfig)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    loadConfig()
  }, [loadConfig])

  const handleDefaultPolicyChange = async (policy: string) => {
    if (!config) return
    setSaving(true)
    try {
      await api.channels.telegram.updateAccessDefaultPolicy(policy)
      setConfig({ ...config, default_policy: policy })
    } catch (err) {
      console.error('Failed to update default policy:', err)
    } finally {
      setSaving(false)
    }
  }

  const handleAddUser = async (id: string, level: string) => {
    if (level === 'blocked') {
      await api.channels.telegram.blockUser(id)
    } else {
      await api.channels.telegram.grantUser(id, level)
    }
    loadConfig()
  }

  const handleRemoveUser = async (id: string, currentLevel: UserLevel) => {
    if (currentLevel === 'blocked') {
      await api.channels.telegram.unblockUser(id)
    } else {
      await api.channels.telegram.revokeUser(id)
    }
    loadConfig()
  }

  const handleChangeLevel = async (id: string, currentLevel: UserLevel, newLevel: UserLevel) => {
    if (currentLevel === newLevel) return
    if (currentLevel === 'blocked') {
      await api.channels.telegram.unblockUser(id)
    } else {
      await api.channels.telegram.revokeUser(id)
    }
    if (newLevel === 'blocked') {
      await api.channels.telegram.blockUser(id)
    } else {
      await api.channels.telegram.grantUser(id, newLevel)
    }
    loadConfig()
  }

  if (loading) {
    return (
      <div className="flex flex-col items-center gap-4 py-16">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-bg-subtle border-t-brand" />
        <p className="text-sm text-tertiary">{t('common.loading')}</p>
      </div>
    )
  }

  if (!config) {
    return (
      <div className="rounded-xl border border-warning/20 bg-warning-primary px-4 py-3">
        <p className="text-sm text-primary">{t('telegram.access.notAvailable')}</p>
      </div>
    )
  }

  const allUsers: { id: string; level: UserLevel }[] = [
    ...(config.owners || []).map((id) => ({ id, level: 'owner' as UserLevel })),
    ...(config.admins || []).map((id) => ({ id, level: 'admin' as UserLevel })),
    ...(config.allowed_users || []).map((id) => ({ id, level: 'user' as UserLevel })),
    ...(config.blocked_users || []).map((id) => ({ id, level: 'blocked' as UserLevel })),
  ]

  return (
    <div className="flex flex-col gap-6">
      {/* Default Policy */}
      <Card className="p-6">
        <h3 className="mb-4 text-sm font-semibold text-primary">{t('telegram.access.defaultPolicy')}</h3>
        <div className="flex gap-2">
          {['allow', 'deny'].map((policy) => (
            <button
              key={policy}
              onClick={() => handleDefaultPolicyChange(policy)}
              disabled={saving}
              className={cn(
                'cursor-pointer rounded-lg px-4 py-2 text-sm font-medium transition-colors',
                config.default_policy === policy
                  ? 'bg-brand text-white'
                  : 'bg-tertiary text-secondary hover:bg-active'
              )}
            >
              {t(`telegram.access.policies.${policy}`)}
            </button>
          ))}
        </div>
      </Card>

      {/* Users List */}
      <Card className="p-6">
        <div className="mb-4 flex items-center justify-between">
          <h3 className="text-sm font-semibold text-primary">
            {t('telegram.access.users')} ({allUsers.length})
          </h3>
          <Button size="sm" variant="secondary" onClick={() => setShowAddModal(true)}>
            {t('common.add')}
          </Button>
        </div>

        {allUsers.length > 0 ? (
          <div className="flex flex-col divide-y divide-secondary">
            {allUsers.map(({ id, level }) => (
              <div key={id} className="flex items-center gap-3 py-3 first:pt-0 last:pb-0">
                <UserCircle2 className="h-5 w-5 shrink-0 text-tertiary" />
                <span className="min-w-0 flex-1 truncate text-sm font-medium text-primary font-mono">
                  {id}
                </span>
                <Select
                  options={levelOptions}
                  value={level}
                  onChange={(val) => handleChangeLevel(id, level, val as UserLevel)}
                  className="w-36 shrink-0"
                />
                <button
                  onClick={() => handleRemoveUser(id, level)}
                  className="shrink-0 cursor-pointer rounded-md p-1 text-tertiary hover:text-fg-error-secondary transition"
                  title={t('common.remove')}
                >
                  <XCircle className="h-4 w-4" />
                </button>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-tertiary">{t('telegram.access.noUsers')}</p>
        )}
      </Card>

      <AddUserModal
        isOpen={showAddModal}
        onClose={() => setShowAddModal(false)}
        onAdd={handleAddUser}
      />
    </div>
  )
}

/* ── Add User Modal ── */

interface AddUserModalProps {
  isOpen: boolean
  onClose: () => void
  onAdd: (jid: string, level: string) => Promise<void>
}

function AddUserModal({ isOpen, onClose, onAdd }: AddUserModalProps) {
  const { t } = useTranslation()
  const [userId, setUserId] = useState('')
  const [level, setLevel] = useState<UserLevel>('user')
  const [loading, setLoading] = useState(false)

  const levelOptions = [
    { value: 'admin', label: t('telegram.access.admin') },
    { value: 'user', label: t('telegram.access.allowed') },
    { value: 'blocked', label: t('telegram.access.blocked') },
  ]

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!userId.trim()) return
    setLoading(true)
    try {
      await onAdd(userId.trim(), level)
      setUserId('')
      setLevel('user')
      onClose()
    } catch (err) {
      console.error('Failed to add user:', err)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('telegram.access.addUser')}
      size="sm"
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button size="sm" onClick={handleSubmit} disabled={loading || !userId.trim()}>
            {loading && <Loader2 className="h-4 w-4 animate-spin" />}
            {t('common.add')}
          </Button>
        </>
      }
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <label className="text-sm font-medium text-primary">{t('telegram.access.userId')}</label>
          <Input
            value={userId}
            onChange={(e) => setUserId(e.target.value)}
            placeholder={t('telegram.access.userIdPlaceholder')}
          />
        </div>
        <Select
          label={t('telegram.access.permission')}
          options={levelOptions}
          value={level}
          onChange={(val) => setLevel(val as UserLevel)}
        />
      </form>
    </Modal>
  )
}

/* ── Settings Tab ── */

function SettingsTab({
  config,
  onConfigChange,
}: {
  config: TelegramConfig | null
  onConfigChange: (config: TelegramConfig) => void
}) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)

  if (!config) {
    return (
      <Card className="p-8 text-center">
        <p className="text-sm text-tertiary">{t('telegram.settingsNotAvailable')}</p>
      </Card>
    )
  }

  const handleToggle = async (key: 'respond_to_groups' | 'respond_to_dms' | 'send_typing') => {
    const newConfig = { ...config, [key]: !config[key] }
    setSaving(true)
    try {
      await api.channels.telegram.updateConfig({ [key]: newConfig[key] })
      onConfigChange(newConfig)
    } catch (err) {
      console.error('Failed to update Telegram settings:', err)
    } finally {
      setSaving(false)
    }
  }

  const handleReactionChange = async (value: string) => {
    setSaving(true)
    try {
      await api.channels.telegram.updateConfig({ reaction_notifications: value })
      onConfigChange({ ...config, reaction_notifications: value })
    } catch (err) {
      console.error('Failed to update reaction notifications:', err)
    } finally {
      setSaving(false)
    }
  }

  const reactionOptions = [
    { value: 'off', label: t('telegram.reactions.off') },
    { value: 'own', label: t('telegram.reactions.own') },
    { value: 'all', label: t('telegram.reactions.all') },
  ]

  return (
    <div className="flex flex-col gap-6">
      {/* Bot Settings */}
      <Card className="p-6">
        <h3 className="mb-4 text-sm font-semibold text-primary">{t('telegram.settings.bot')}</h3>

        <div className="flex items-start justify-between border-b border-secondary py-4">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-primary">
              {t('telegram.settings.respondToGroups')}
            </span>
            <span className="text-xs text-tertiary">
              {t('telegram.settings.respondToGroupsDesc')}
            </span>
          </div>
          <Toggle
            checked={config.respond_to_groups}
            onChange={() => handleToggle('respond_to_groups')}
            disabled={saving}
            size="sm"
          />
        </div>

        <div className="flex items-start justify-between border-b border-secondary py-4">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-primary">
              {t('telegram.settings.respondToDMs')}
            </span>
            <span className="text-xs text-tertiary">
              {t('telegram.settings.respondToDMsDesc')}
            </span>
          </div>
          <Toggle
            checked={config.respond_to_dms}
            onChange={() => handleToggle('respond_to_dms')}
            disabled={saving}
            size="sm"
          />
        </div>

        <div className="flex items-start justify-between py-4">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-primary">
              {t('telegram.settings.sendTyping')}
            </span>
            <span className="text-xs text-tertiary">
              {t('telegram.settings.sendTypingDesc')}
            </span>
          </div>
          <Toggle
            checked={config.send_typing}
            onChange={() => handleToggle('send_typing')}
            disabled={saving}
            size="sm"
          />
        </div>
      </Card>

      {/* Reaction Notifications */}
      <Card className="p-6">
        <h3 className="mb-2 text-sm font-semibold text-primary">
          {t('telegram.settings.reactions')}
        </h3>
        <p className="mb-4 text-sm text-tertiary">
          {t('telegram.settings.reactionsDesc')}
        </p>
        <Select
          options={reactionOptions}
          value={config.reaction_notifications || 'off'}
          disabled={saving}
          onChange={handleReactionChange}
          className="max-w-xs"
        />
      </Card>

      {/* Allowed Chats */}
      <Card className="p-6">
        <h3 className="mb-2 text-sm font-semibold text-primary">
          {t('telegram.settings.allowedChats')}
        </h3>
        <p className="mb-4 text-sm text-tertiary">
          {t('telegram.settings.allowedChatsDesc')}
        </p>
        {config.allowed_chats && config.allowed_chats.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {config.allowed_chats.map((chatId) => (
              <Badge key={chatId} variant="default" className="font-mono text-xs">
                {chatId}
              </Badge>
            ))}
          </div>
        ) : (
          <p className="text-xs text-tertiary">
            {t('telegram.settings.noAllowedChats')}
          </p>
        )}
      </Card>
    </div>
  )
}

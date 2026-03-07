import { useEffect, useRef, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { QRCodeSVG } from 'qrcode.react'
import {
  CheckCircle2,
  RefreshCw,
  WifiOff,
  QrCode,
  Shield,
  ArrowLeft,
  Monitor,
  Users,
  UserCircle2,
  Settings,
  XCircle,
  Loader2,
} from 'lucide-react'
import {
  api,
  type WhatsAppStatus,
  type WhatsAppAccessConfig,
  type WhatsAppGroupPolicies,
  type WhatsAppSettings,
} from '@/lib/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/Button'
import { Select } from '@/components/ui/Select'
import { Input } from '@/components/ui/Input'
import { Toggle } from '@/components/ui/Toggle'
import { Tabs } from '@/components/ui/Tabs'
import { Badge } from '@/components/ui/Badge'
import { Modal } from '@/components/ui/Modal'
import { Card } from '@/components/ui/Card'

type ConnectionState = 'loading' | 'connected' | 'waiting_qr' | 'scanning' | 'timeout' | 'error'

type Tab = 'connection' | 'access' | 'groups' | 'settings'

const tabFromHash = (): Tab => {
  const hash = window.location.hash.replace('#', '')
  if (['connection', 'access', 'groups', 'settings'].includes(hash)) {
    return hash as Tab
  }
  return 'connection'
}

export function WhatsAppConnect() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [activeTab, setActiveTab] = useState<Tab>(tabFromHash())
  const [state, setState] = useState<ConnectionState>('loading')
  const [qrCode, setQrCode] = useState('')
  const [message, setMessage] = useState('')
  const [refreshing, setRefreshing] = useState(false)
  const [disconnecting, setDisconnecting] = useState(false)
  const eventSourceRef = useRef<EventSource | null>(null)

  const connectSSE = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
      eventSourceRef.current = null
    }

    const token = localStorage.getItem('devclaw_token')
    const url = token
      ? `/api/channels/whatsapp/qr?token=${encodeURIComponent(token)}`
      : '/api/channels/whatsapp/qr'

    const es = new EventSource(url)
    eventSourceRef.current = es

    es.addEventListener('status', (e) => {
      const data: WhatsAppStatus = JSON.parse(e.data)
      if (data.connected) {
        setState('connected')
        setMessage(t('whatsapp.connected'))
      } else if (data.needs_qr) {
        setState('waiting_qr')
        setMessage(t('whatsapp.waitingQR'))
      }
    })

    es.addEventListener('code', (e) => {
      const data = JSON.parse(e.data)
      setQrCode(data.code)
      setState('waiting_qr')
      setMessage(data.message || t('whatsapp.scanQR'))
    })

    es.addEventListener('success', (e) => {
      const data = JSON.parse(e.data)
      setState('connected')
      setMessage(data.message || t('whatsapp.connected'))
      setQrCode('')
    })

    es.addEventListener('timeout', (e) => {
      const data = JSON.parse(e.data)
      setState('timeout')
      setMessage(data.message || t('whatsapp.qrExpired'))
      setQrCode('')
    })

    es.addEventListener('error', (e) => {
      if (e instanceof MessageEvent && e.data) {
        const data = JSON.parse(e.data)
        setState('error')
        setMessage(data.message || t('whatsapp.connectionError'))
      }
    })

    es.addEventListener('close', () => {
      es.close()
    })

    es.onerror = () => {
      setState('error')
      setMessage(t('whatsapp.sseLost'))
    }
  }, [t])

  useEffect(() => {
    api.channels.whatsapp
      .status()
      .then((status) => {
        if (status.connected) {
          setState('connected')
          setMessage(t('whatsapp.connected'))
        } else {
          connectSSE()
          api.channels.whatsapp.requestQR().catch(() => {})
        }
      })
      .catch(() => {
        setState('error')
        setMessage(t('whatsapp.statusError'))
      })

    return () => {
      eventSourceRef.current?.close()
    }
  }, [connectSSE, t])

  const handleRefresh = async () => {
    setRefreshing(true)
    try {
      await api.channels.whatsapp.requestQR()
      setState('waiting_qr')
      setMessage(t('whatsapp.generatingQR'))
      setQrCode('')
      connectSSE()
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('whatsapp.connectionError')
      setMessage(msg)
    } finally {
      setRefreshing(false)
    }
  }

  const handleDisconnect = async () => {
    setDisconnecting(true)
    try {
      await api.channels.whatsapp.disconnect()
      setState('waiting_qr')
      setMessage(t('whatsapp.disconnected'))
      setQrCode('')
      connectSSE()
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('whatsapp.disconnectError')
      setMessage(msg)
    } finally {
      setDisconnecting(false)
    }
  }

  const handleTabChange = (tabId: string) => {
    setActiveTab(tabId as Tab)
    window.location.hash = tabId
  }

  const tabs = [
    { id: 'connection', label: t('whatsapp.tabs.connection'), icon: <Monitor className="h-4 w-4" /> },
    { id: 'access', label: t('whatsapp.tabs.access'), icon: <Users className="h-4 w-4" /> },
    { id: 'groups', label: t('whatsapp.tabs.groups'), icon: <UserCircle2 className="h-4 w-4" /> },
    { id: 'settings', label: t('whatsapp.tabs.settings'), icon: <Settings className="h-4 w-4" /> },
  ]

  return (
    <div className="flex-1 overflow-y-auto bg-bg-main">
      <div className="mx-auto max-w-3xl px-6 py-8">
        {/* Back + Header */}
        <div className="flex flex-col gap-3">
          <button
            onClick={() => navigate('/channels')}
            className="flex items-center gap-1.5 text-sm text-text-muted hover:text-text-primary transition-colors cursor-pointer w-fit"
          >
            <ArrowLeft className="h-4 w-4" />
            {t('channelsPage.backToChannels')}
          </button>
          <div className="flex flex-col gap-1">
            <h2 className="text-lg font-medium text-text-primary">{t('whatsapp.title')}</h2>
            <p className="text-sm text-text-muted">{t('whatsapp.subtitle')}</p>
          </div>
        </div>

        {/* Tabs */}
        <div className="mt-6">
          <Tabs tabs={tabs} activeTab={activeTab} onChange={handleTabChange} />
        </div>

        {/* Tab Content */}
        <div className="mt-6">
          {activeTab === 'connection' && (
            <ConnectionTab
              state={state}
              message={message}
              qrCode={qrCode}
              refreshing={refreshing}
              onRefresh={handleRefresh}
              onDisconnect={handleDisconnect}
              disconnecting={disconnecting}
            />
          )}
          {activeTab === 'access' && <AccessTab />}
          {activeTab === 'groups' && <GroupsTab />}
          {activeTab === 'settings' && <SettingsTab />}
        </div>
      </div>
    </div>
  )
}

/* ── Connection Tab ── */

interface ConnectionTabProps {
  state: ConnectionState
  message: string
  qrCode: string
  refreshing: boolean
  onRefresh: () => void
  onDisconnect: () => void
  disconnecting: boolean
}

function ConnectionTab({
  state,
  message,
  qrCode,
  refreshing,
  onRefresh,
  onDisconnect,
  disconnecting,
}: ConnectionTabProps) {
  const { t } = useTranslation()

  if (state === 'loading') {
    return (
      <div className="flex flex-col items-center gap-4 py-16">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-bg-subtle border-t-brand" />
        <p className="text-sm text-text-muted">{t('whatsapp.checkingConnection')}</p>
      </div>
    )
  }

  if (state === 'connected') {
    return (
      <Card className="flex flex-col items-center px-6 py-6">
        <div className="flex h-12 w-12 items-center justify-center rounded-full bg-success-subtle">
          <CheckCircle2 className="h-8 w-8 text-success" />
        </div>
        <h2 className="mt-4 text-center text-lg font-semibold text-text-primary">
          {t('whatsapp.connected')}
        </h2>
        <p className="mt-1 text-center text-sm text-text-muted">{message}</p>
        <div className="mt-3">
          <Badge variant="success">{t('common.online')}</Badge>
        </div>
        <div className="mt-5">
          <Button
            variant="secondary"
            size="sm"
            disabled={disconnecting}
            onClick={onDisconnect}
          >
            <WifiOff className="h-4 w-4" />
            {disconnecting ? t('whatsapp.disconnecting') : t('whatsapp.disconnect')}
          </Button>
        </div>
      </Card>
    )
  }

  if (state === 'waiting_qr' || state === 'scanning') {
    return (
      <div className="flex flex-col gap-6">
        {/* Mobile hint */}
        <div className="md:hidden rounded-xl border border-warning/20 bg-warning-subtle px-4 py-3">
          <p className="text-sm text-warning">{t('whatsapp.mobileHint')}</p>
        </div>
        <div className="grid items-center gap-8 md:grid-cols-[auto_1fr]">
          {/* QR side */}
          <div className="flex flex-col items-center gap-4">
            <Card className="relative rounded-2xl p-5">
              {/* Decorative corners */}
              <div className="absolute -left-px -top-px h-6 w-6 rounded-tl-2xl border-l-2 border-t-2 border-brand" />
              <div className="absolute -right-px -top-px h-6 w-6 rounded-tr-2xl border-r-2 border-t-2 border-brand" />
              <div className="absolute -bottom-px -left-px h-6 w-6 rounded-bl-2xl border-b-2 border-l-2 border-brand" />
              <div className="absolute -bottom-px -right-px h-6 w-6 rounded-br-2xl border-b-2 border-r-2 border-brand" />

              {qrCode ? (
                <div className="rounded-xl bg-white p-3">
                  <QRCodeSVG
                    value={qrCode}
                    size={240}
                    level="L"
                    bgColor="#ffffff"
                    fgColor="#000000"
                  />
                </div>
              ) : (
                <div className="flex h-[264px] w-[264px] items-center justify-center">
                  <div className="flex flex-col items-center gap-3">
                    <QrCode className="h-12 w-12 animate-pulse text-text-muted" />
                    <p className="text-xs text-text-muted">{t('whatsapp.generatingQR')}</p>
                  </div>
                </div>
              )}
            </Card>
            <button
              onClick={onRefresh}
              disabled={refreshing}
              className="flex cursor-pointer items-center gap-1.5 text-xs text-text-muted transition-colors hover:text-text-primary disabled:opacity-50"
            >
              <RefreshCw className={cn('h-3 w-3', refreshing && 'animate-spin')} />
              {t('whatsapp.generateNew')}
            </button>
          </div>

          {/* Instructions side */}
          <div className="flex flex-col gap-5">
            <h3 className="text-sm font-semibold text-text-primary">{t('whatsapp.howToConnect')}</h3>
            <div className="flex flex-col gap-4">
              <StepItem number={1} text={t('whatsapp.step1')} />
              <StepItem number={2} text={t('whatsapp.step2')} />
              <StepItem number={3} text={t('whatsapp.step3')} />
              <StepItem number={4} text={t('whatsapp.step4')} />
            </div>
            <div className="flex max-w-fit items-start gap-2 rounded-xl bg-bg-subtle px-3 py-2.5">
              <Shield className="mt-0.5 h-3.5 w-3.5 shrink-0 text-text-muted" />
              <p className="text-[11px] text-text-secondary">{t('whatsapp.e2eHint')}</p>
            </div>
          </div>
        </div>
      </div>
    )
  }

  if (state === 'timeout') {
    return (
      <div className="flex flex-col items-center rounded-2xl border border-warning/20 bg-warning-subtle px-8 py-10">
        <div className="flex h-20 w-20 items-center justify-center rounded-full bg-warning-subtle border border-warning/30">
          <WifiOff className="h-10 w-10 text-warning" />
        </div>
        <h2 className="mt-5 text-lg font-semibold text-text-primary">{t('whatsapp.qrExpired')}</h2>
        <p className="mt-1 text-sm text-warning">{message}</p>
        <div className="mt-6">
          <Button onClick={onRefresh} disabled={refreshing} size="sm">
            {refreshing ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="h-4 w-4" />
            )}
            {t('whatsapp.generateNewQR')}
          </Button>
        </div>
      </div>
    )
  }

  // error
  return (
    <div className="flex flex-col items-center rounded-2xl border border-error/20 bg-error-subtle px-8 py-10">
      <div className="flex h-20 w-20 items-center justify-center rounded-full bg-error-subtle border border-error/30">
        <WifiOff className="h-10 w-10 text-error" />
      </div>
      <h2 className="mt-5 text-lg font-semibold text-text-primary">{t('whatsapp.connectionError')}</h2>
      <p className="mt-1 text-sm text-error">{message}</p>
      <div className="mt-6">
        <Button onClick={onRefresh} disabled={refreshing} size="sm">
          {refreshing ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <RefreshCw className="h-4 w-4" />
          )}
          {t('whatsapp.tryAgain')}
        </Button>
      </div>
    </div>
  )
}

/* ── Access Tab ── */

type UserLevel = 'owner' | 'admin' | 'user' | 'blocked'

function AccessTab() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<WhatsAppAccessConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [showAddModal, setShowAddModal] = useState(false)

  const levelOptions = [
    { value: 'owner', label: t('whatsapp.owner') },
    { value: 'admin', label: t('whatsapp.admin') },
    { value: 'user', label: t('whatsapp.allowed') },
    { value: 'blocked', label: t('whatsapp.blocked') },
  ]

  const loadConfig = useCallback(() => {
    setLoading(true)
    api.channels.whatsapp
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
      await api.channels.whatsapp.updateAccessDefaultPolicy(policy)
      setConfig({ ...config, default_policy: policy })
    } catch (err) {
      console.error('Failed to update default policy:', err)
    } finally {
      setSaving(false)
    }
  }

  const handleAddUser = async (jid: string, level: string) => {
    if (level === 'blocked') {
      await api.channels.whatsapp.blockUser(jid)
    } else {
      await api.channels.whatsapp.grantUser(jid, level)
    }
    loadConfig()
  }

  const handleRemoveUser = async (jid: string, currentLevel: UserLevel) => {
    if (currentLevel === 'blocked') {
      await api.channels.whatsapp.unblockUser(jid)
    } else {
      await api.channels.whatsapp.revokeUser(jid)
    }
    loadConfig()
  }

  const handleChangeLevel = async (jid: string, currentLevel: UserLevel, newLevel: UserLevel) => {
    if (currentLevel === newLevel) return
    if (currentLevel === 'blocked') {
      await api.channels.whatsapp.unblockUser(jid)
    } else {
      await api.channels.whatsapp.revokeUser(jid)
    }
    if (newLevel === 'blocked') {
      await api.channels.whatsapp.blockUser(jid)
    } else {
      await api.channels.whatsapp.grantUser(jid, newLevel)
    }
    loadConfig()
  }

  if (loading) {
    return (
      <div className="flex flex-col items-center gap-4 py-16">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-bg-subtle border-t-brand" />
        <p className="text-sm text-text-muted">{t('common.loading')}</p>
      </div>
    )
  }

  if (!config) {
    return (
      <div className="rounded-xl border border-warning/20 bg-warning-subtle px-4 py-3">
        <p className="text-sm text-text-primary">{t('whatsapp.accessNotAvailable')}</p>
      </div>
    )
  }

  const allUsers: { jid: string; level: UserLevel }[] = [
    ...(config.owners || []).map((jid) => ({ jid, level: 'owner' as UserLevel })),
    ...(config.admins || []).map((jid) => ({ jid, level: 'admin' as UserLevel })),
    ...(config.allowed_users || []).map((jid) => ({ jid, level: 'user' as UserLevel })),
    ...(config.blocked_users || []).map((jid) => ({ jid, level: 'blocked' as UserLevel })),
  ]

  return (
    <div className="flex flex-col gap-6">
      {/* Default Policy */}
      <Card className="p-6">
        <h3 className="mb-4 text-sm font-semibold text-text-primary">{t('whatsapp.defaultPolicy')}</h3>
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
                  : 'bg-bg-elevated text-text-secondary hover:bg-bg-active'
              )}
            >
              {t(`whatsapp.policies.${policy}`)}
            </button>
          ))}
        </div>
      </Card>

      {/* Users List */}
      <Card className="p-6">
        <div className="mb-4 flex items-center justify-between">
          <h3 className="text-sm font-semibold text-text-primary">
            {t('whatsapp.users')} ({allUsers.length})
          </h3>
          <Button size="sm" variant="secondary" onClick={() => setShowAddModal(true)}>
            {t('common.add')}
          </Button>
        </div>

        {allUsers.length > 0 ? (
          <div className="flex flex-col divide-y divide-border">
            {allUsers.map(({ jid, level }) => (
              <div key={jid} className="flex items-center gap-3 py-3 first:pt-0 last:pb-0">
                <UserCircle2 className="h-5 w-5 shrink-0 text-text-muted" />
                <span className="min-w-0 flex-1 truncate text-sm font-medium text-text-primary">
                  {jid.replace('@s.whatsapp.net', '')}
                </span>
                <Select
                  options={levelOptions}
                  value={level}
                  onChange={(val) => handleChangeLevel(jid, level, val as UserLevel)}
                  className="w-36 shrink-0"
                />
                <button
                  onClick={() => handleRemoveUser(jid, level)}
                  className="shrink-0 cursor-pointer rounded-md p-1 text-text-muted hover:text-error transition"
                  title={t('common.remove')}
                >
                  <XCircle className="h-4 w-4" />
                </button>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-text-muted">{t('whatsapp.noUsers')}</p>
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
  const [jid, setJid] = useState('')
  const [level, setLevel] = useState<UserLevel>('user')
  const [loading, setLoading] = useState(false)

  const levelOptions = [
    { value: 'owner', label: t('whatsapp.owner') },
    { value: 'admin', label: t('whatsapp.admin') },
    { value: 'user', label: t('whatsapp.allowed') },
    { value: 'blocked', label: t('whatsapp.blocked') },
  ]

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!jid.trim()) return
    setLoading(true)
    try {
      await onAdd(jid.trim(), level)
      setJid('')
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
      title={t('whatsapp.addUser.title')}
      size="sm"
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button size="sm" onClick={handleSubmit} disabled={loading || !jid.trim()}>
            {loading && <Loader2 className="h-4 w-4 animate-spin" />}
            {t('common.add')}
          </Button>
        </>
      }
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <label className="text-sm font-medium text-text-primary">{t('whatsapp.phoneNumber')}</label>
          <Input
            value={jid}
            onChange={(e) => setJid(e.target.value)}
            placeholder={t('whatsapp.jidPlaceholder')}
          />
        </div>
        <Select
          label={t('whatsapp.permission')}
          options={levelOptions}
          value={level}
          onChange={(val) => setLevel(val as UserLevel)}
        />
      </form>
    </Modal>
  )
}

/* ── Groups Tab ── */

function GroupsTab() {
  const { t } = useTranslation()
  const [policies, setPolicies] = useState<WhatsAppGroupPolicies | null>(null)
  const [joinedGroups, setJoinedGroups] = useState<{ jid: string; name: string }[]>([])
  const [loading, setLoading] = useState(true)
  const [loadingGroups, setLoadingGroups] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editingGroup, setEditingGroup] = useState<string | null>(null)
  const [editKeywords, setEditKeywords] = useState('')

  const loadPolicies = useCallback(() => {
    setLoading(true)
    api.channels.whatsapp
      .getGroups()
      .then(setPolicies)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    loadPolicies()
  }, [loadPolicies])

  const handleLoadJoinedGroups = async () => {
    setLoadingGroups(true)
    try {
      const groups = await api.channels.whatsapp.getJoinedGroups()
      setJoinedGroups(groups || [])
    } catch (err) {
      console.error('Failed to load joined groups:', err)
    } finally {
      setLoadingGroups(false)
    }
  }

  const handleAddGroup = async (jid: string, name: string) => {
    setSaving(true)
    try {
      await api.channels.whatsapp.setGroupPolicy(jid, {
        name,
        policy: '',
        policies: [],
      })
      loadPolicies()
    } catch (err) {
      console.error('Failed to add group:', err)
    } finally {
      setSaving(false)
    }
  }

  const handleDefaultPolicyChange = async (policy: string) => {
    if (!policies) return
    setSaving(true)
    try {
      await api.channels.whatsapp.updateGroupDefaultPolicy(policy)
      setPolicies({ ...policies, default_policy: policy })
    } catch (err) {
      console.error('Failed to update default policy:', err)
    } finally {
      setSaving(false)
    }
  }

  const handleGroupPoliciesToggle = async (jid: string, policy: string, checked: boolean) => {
    if (!policies) return
    const group = (policies.groups || []).find((g) => g.id === jid)
    if (!group) return

    const currentPolicies = group.policies || (group.policy ? [group.policy] : [])
    const newPolicies = checked
      ? [...new Set([...currentPolicies, policy])]
      : currentPolicies.filter((p) => p !== policy)

    setSaving(true)
    try {
      await api.channels.whatsapp.setGroupPolicy(jid, {
        name: group.name || '',
        policy: newPolicies[0] || '',
        policies: newPolicies,
        keywords: group.keywords,
        allowed_users: group.allowed_users,
        workspace: group.workspace,
      })
      loadPolicies()
    } catch (err) {
      console.error('Failed to update group policies:', err)
    } finally {
      setSaving(false)
    }
  }

  const handleKeywordsSave = async (jid: string) => {
    if (!policies) return
    const group = (policies.groups || []).find((g) => g.id === jid)
    if (!group) return

    const keywords = editKeywords
      .split(',')
      .map((k) => k.trim().toLowerCase())
      .filter((k) => k.length > 0)

    setSaving(true)
    try {
      await api.channels.whatsapp.setGroupPolicy(jid, {
        name: group.name || '',
        policy: group.policy,
        policies: group.policies,
        keywords,
        allowed_users: group.allowed_users,
        workspace: group.workspace,
      })
      loadPolicies()
      setEditingGroup(null)
      setEditKeywords('')
    } catch (err) {
      console.error('Failed to update keywords:', err)
    } finally {
      setSaving(false)
    }
  }

  const startEditKeywords = (group: { id: string; keywords?: string[] }) => {
    setEditingGroup(group.id)
    setEditKeywords((group.keywords || []).join(', '))
  }

  if (loading) {
    return (
      <div className="flex flex-col items-center gap-4 py-16">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-bg-subtle border-t-brand" />
        <p className="text-sm text-text-muted">{t('common.loading')}</p>
      </div>
    )
  }

  if (!policies) {
    return (
      <div className="rounded-xl border border-warning/20 bg-warning-subtle px-4 py-3">
        <p className="text-sm text-text-primary">{t('whatsapp.groupsNotAvailable')}</p>
      </div>
    )
  }

  const policyOptionIds = ['always', 'mention', 'reply', 'keyword', 'disabled', 'allowlist']

  const policySelectOptions = policyOptionIds.map((id) => ({
    value: id,
    label: t(`whatsapp.groupPolicies.${id}`),
  }))

  return (
    <div className="flex flex-col gap-6">
      {/* Default Group Policy */}
      <Card className="p-6">
        <h3 className="mb-2 text-sm font-semibold text-text-primary">
          {t('whatsapp.defaultGroupPolicy')}
        </h3>
        <p className="mb-4 text-sm text-text-muted">{t('whatsapp.defaultGroupPolicyDesc')}</p>
        <Select
          placeholder={t('whatsapp.selectPolicy')}
          options={policySelectOptions}
          value={policies.default_policy}
          disabled={saving}
          onChange={(val) => handleDefaultPolicyChange(val)}
          className="max-w-xs"
        />
      </Card>

      {/* Group List */}
      <Card className="p-6">
        <div className="mb-4 flex items-center justify-between">
          <h3 className="text-sm font-semibold text-text-primary">
            {t('whatsapp.configuredGroups')} ({(policies.groups || []).length})
          </h3>
          <Button size="sm" variant="secondary" onClick={handleLoadJoinedGroups} disabled={loadingGroups}>
            {loadingGroups && <Loader2 className="h-4 w-4 animate-spin" />}
            {t('whatsapp.loadGroups')}
          </Button>
        </div>

        {(policies.groups || []).length > 0 ? (
          <div className="flex flex-col gap-4">
            {(policies.groups || []).map((group) => {
              const activePolicies = group.policies || (group.policy ? [group.policy] : [])
              return (
                <div key={group.id} className="rounded-lg bg-bg-subtle p-4">
                  <div className="mb-3 flex items-start justify-between">
                    <div className="flex flex-col gap-1">
                      <span className="text-sm font-medium text-text-primary">
                        {group.name || group.id}
                      </span>
                      <span className="text-xs text-text-muted">{group.id.replace('@g.us', '')}</span>
                    </div>
                  </div>

                  {/* Policies Checkboxes */}
                  <div className="mb-3">
                    <label className="mb-2 block text-xs font-medium text-text-secondary">
                      {t('whatsapp.groupPoliciesLabel')}
                    </label>
                    <div className="flex flex-wrap gap-2">
                      {policyOptionIds.map((policy) => {
                        const isActive = activePolicies.includes(policy)
                        return (
                          <button
                            key={policy}
                            onClick={() => handleGroupPoliciesToggle(group.id, policy, !isActive)}
                            disabled={saving}
                            className={cn(
                              'cursor-pointer rounded-lg px-3 py-1.5 text-xs font-medium transition-colors',
                              isActive
                                ? 'bg-brand text-white'
                                : 'bg-bg-elevated text-text-muted hover:bg-bg-active'
                            )}
                          >
                            {t(`whatsapp.groupPolicies.${policy}`)}
                          </button>
                        )
                      })}
                    </div>
                  </div>

                  {/* Keywords */}
                  <div>
                    <label className="mb-2 block text-xs font-medium text-text-secondary">
                      {t('whatsapp.keywordsLabel')}
                    </label>
                    {editingGroup === group.id ? (
                      <div className="flex items-center gap-2">
                        <Input
                          value={editKeywords}
                          onChange={(e) => setEditKeywords(e.target.value)}
                          placeholder={t('whatsapp.keywordsPlaceholder')}
                          className="flex-1"
                        />
                        <Button
                          size="sm"
                          onClick={() => handleKeywordsSave(group.id)}
                          disabled={saving}
                        >
                          {t('common.save')}
                        </Button>
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => {
                            setEditingGroup(null)
                            setEditKeywords('')
                          }}
                        >
                          {t('common.cancel')}
                        </Button>
                      </div>
                    ) : (
                      <div className="flex items-center gap-2">
                        <div className="flex flex-wrap gap-1">
                          {(group.keywords || []).length > 0 ? (
                            group.keywords!.map((kw) => (
                              <span
                                key={kw}
                                className="rounded bg-bg-elevated px-2 py-0.5 text-xs text-text-secondary"
                              >
                                {kw}
                              </span>
                            ))
                          ) : (
                            <span className="text-xs text-text-muted">
                              {t('whatsapp.noKeywords')}
                            </span>
                          )}
                        </div>
                        <button
                          onClick={() => startEditKeywords(group)}
                          className="cursor-pointer text-xs text-brand hover:underline"
                        >
                          {t('common.edit')}
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        ) : (
          <p className="text-sm text-text-muted">{t('whatsapp.noConfiguredGroups')}</p>
        )}
      </Card>

      {/* Available Groups (from WhatsApp) */}
      {joinedGroups.length > 0 && (
        <Card className="p-6">
          <h3 className="mb-4 text-sm font-semibold text-text-primary">
            {t('whatsapp.availableGroups')} ({joinedGroups.length})
          </h3>
          <div className="flex flex-col gap-2">
            {joinedGroups.map((group) => {
              const isConfigured = (policies?.groups || []).some((g) => g.id === group.jid)
              return (
                <div
                  key={group.jid}
                  className="flex items-center justify-between rounded-lg bg-bg-subtle p-3"
                >
                  <div className="flex flex-col gap-1">
                    <span className="text-sm font-medium text-text-primary">
                      {group.name || group.jid}
                    </span>
                    <span className="text-xs text-text-muted">{group.jid}</span>
                  </div>
                  {isConfigured ? (
                    <span className="text-xs text-success">{t('whatsapp.groupConfigured')}</span>
                  ) : (
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => handleAddGroup(group.jid, group.name)}
                      disabled={saving}
                    >
                      {t('whatsapp.configureGroup')}
                    </Button>
                  )}
                </div>
              )
            })}
          </div>
        </Card>
      )}
    </div>
  )
}

/* ── Settings Tab ── */

function SettingsTab() {
  const { t } = useTranslation()
  const [settings, setSettings] = useState<WhatsAppSettings>({
    respond_to_groups: true,
    respond_to_dms: true,
    auto_read: true,
    send_typing: true,
  })
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setLoading(true)
    api.channels.whatsapp
      .getSettings()
      .then(setSettings)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const handleToggle = async (
    key: 'auto_read' | 'send_typing' | 'respond_to_groups' | 'respond_to_dms'
  ) => {
    const newSettings = { ...settings, [key]: !settings[key] }
    setSaving(true)
    try {
      await api.channels.whatsapp.updateConfig(newSettings)
      setSettings(newSettings)
    } catch (err) {
      console.error('Failed to update settings:', err)
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex flex-col items-center gap-4 py-16">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-bg-subtle border-t-brand" />
        <p className="text-sm text-text-muted">{t('common.loading')}</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      {/* Bot Settings */}
      <Card className="p-6">
        <h3 className="mb-4 text-sm font-semibold text-text-primary">{t('whatsapp.settings.bot')}</h3>

        <div className="flex items-start justify-between border-b border-border py-4">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-text-primary">
              {t('whatsapp.settings.respondToGroups')}
            </span>
            <span className="text-xs text-text-muted">
              {t('whatsapp.settings.respondToGroupsDesc')}
            </span>
          </div>
          <Toggle
            checked={settings.respond_to_groups}
            onChange={() => handleToggle('respond_to_groups')}
            disabled={saving}
            size="sm"
          />
        </div>

        <div className="flex items-start justify-between py-4">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-text-primary">
              {t('whatsapp.settings.respondToDMs')}
            </span>
            <span className="text-xs text-text-muted">{t('whatsapp.settings.respondToDMsDesc')}</span>
          </div>
          <Toggle
            checked={settings.respond_to_dms}
            onChange={() => handleToggle('respond_to_dms')}
            disabled={saving}
            size="sm"
          />
        </div>
      </Card>

      {/* Behavior Settings */}
      <Card className="p-6">
        <h3 className="mb-4 text-sm font-semibold text-text-primary">
          {t('whatsapp.settings.behavior')}
        </h3>

        <div className="flex items-start justify-between border-b border-border py-4">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-text-primary">
              {t('whatsapp.settings.autoRead')}
            </span>
            <span className="text-xs text-text-muted">{t('whatsapp.settings.autoReadDesc')}</span>
          </div>
          <Toggle
            checked={settings.auto_read}
            onChange={() => handleToggle('auto_read')}
            disabled={saving}
            size="sm"
          />
        </div>

        <div className="flex items-start justify-between py-4">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-text-primary">
              {t('whatsapp.settings.sendTyping')}
            </span>
            <span className="text-xs text-text-muted">{t('whatsapp.settings.sendTypingDesc')}</span>
          </div>
          <Toggle
            checked={settings.send_typing}
            onChange={() => handleToggle('send_typing')}
            disabled={saving}
            size="sm"
          />
        </div>
      </Card>
    </div>
  )
}

/* ── Helpers ── */

function StepItem({ number, text }: { number: number; text: string }) {
  return (
    <div className="flex items-start gap-3">
      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-subtle text-[11px] font-semibold text-brand">
        {number}
      </div>
      <p className="text-sm text-text-secondary leading-relaxed">{text}</p>
    </div>
  )
}

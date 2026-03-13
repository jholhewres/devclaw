import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  Cpu,
  Radio,
  Clock,
  ArrowRight,
  Puzzle,
  Shield,
  Settings,
  MessageSquare,
  Zap,
  Activity,
  DollarSign,
  TrendingUp,
} from 'lucide-react'
import { api, type DashboardData } from '@/lib/api'
import { formatTokens, timeAgo, cn } from '@/lib/utils'
import { Card } from '@/components/ui/Card'
import { PageHeader } from '@/components/ui/PageHeader'
import { StatusDot } from '@/components/ui/StatusDot'

function getGreeting(t: (key: string) => string): string {
  const h = new Date().getHours()
  if (h < 12) return t('greetings.morning')
  if (h < 18) return t('greetings.afternoon')
  return t('greetings.evening')
}

export function Dashboard() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [fetchError, setFetchError] = useState(false)

  const fetchData = () => {
    setLoading(true)
    setFetchError(false)
    api.dashboard()
      .then(setData)
      .catch(() => {
        Promise.all([
          api.sessions.list().catch(() => []),
          api.usage().catch(() => ({ total_input_tokens: 0, total_output_tokens: 0, total_cost: 0, request_count: 0 })),
          api.channels.list().catch(() => []),
          api.jobs.list().catch(() => []),
        ]).then(([sessions, usage, channels, jobs]) => {
          setData({ sessions, usage, channels, jobs })
        }).catch(() => setFetchError(true))
      })
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetchData()
  }, [])

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-bg-main">
        <div className="h-8 w-8 rounded-full border-4 border-bg-subtle border-t-brand animate-spin" />
      </div>
    )
  }

  if (fetchError || !data) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center bg-bg-main">
        <p className="text-sm text-error">{t('common.loadError.title')}</p>
        <p className="mt-1 text-xs text-text-muted">{t('common.loadError.description')}</p>
        <button
          onClick={fetchData}
          className="mt-3 cursor-pointer text-xs text-brand transition-colors hover:text-brand-hover"
        >
          {t('common.loadError.retry')}
        </button>
      </div>
    )
  }

  const channels = data.channels ?? []
  const jobs = data.jobs ?? []
  const usage = data.usage ?? { total_input_tokens: 0, total_output_tokens: 0, total_cost: 0, request_count: 0 }
  const connectedChannels = channels.filter((c) => c.connected).length
  const activeJobs = jobs.filter((j) => j.enabled).length
  const totalTokens = usage.total_input_tokens + usage.total_output_tokens
  const sessionCount = (data.sessions ?? []).length
  const allChannelsOk = connectedChannels === channels.length && channels.length > 0

  return (
    <div className="mx-auto max-w-screen-2xl px-4 py-8 sm:px-6 lg:px-8">
      {/* Header */}
      <PageHeader
        title={t('dashboard.title')}
        description={getGreeting(t)}
        actions={
          <div className="flex items-center gap-3 rounded-xl border border-border bg-bg-surface px-4 py-2.5">
            <div className="flex items-center gap-1.5">
              <Activity className="h-3 w-3 text-brand" />
              <span className="text-xs font-semibold text-text-primary">{formatTokens(totalTokens)}</span>
              <span className="text-[10px] text-text-muted">{t('dashboard.tokens')}</span>
            </div>
            <span className="h-4 w-px bg-border" />
            <div className="flex items-center gap-1">
              <DollarSign className="h-3 w-3 text-brand" />
              <span className="text-xs font-semibold text-text-primary">
                {usage.total_cost?.toFixed(2) ?? '0.00'}
              </span>
            </div>
          </div>
        }
      />

      {/* Metric cards */}
      <div className="mt-6 grid grid-cols-2 gap-4 lg:grid-cols-4">
        <MetricCard
          label={t('dashboard.requests')}
          value={String(usage.request_count ?? 0)}
          icon={<Cpu className="h-4 w-4" />}
          iconColor="text-brand"
          iconBg="bg-brand-subtle"
          onClick={() => navigate('/config')}
        />
        <MetricCard
          label={t('dashboard.channels')}
          value={`${connectedChannels}/${channels.length}`}
          icon={<Radio className="h-4 w-4" />}
          iconColor={allChannelsOk ? 'text-success' : channels.length === 0 ? 'text-text-muted' : 'text-warning'}
          iconBg={allChannelsOk ? 'bg-success-subtle' : channels.length === 0 ? 'bg-bg-subtle' : 'bg-warning-subtle'}
          trend={allChannelsOk ? 'ok' : undefined}
          onClick={() => navigate('/channels')}
        />
        <MetricCard
          label={t('dashboard.jobs')}
          value={String(activeJobs)}
          icon={<Clock className="h-4 w-4" />}
          iconColor="text-info"
          iconBg="bg-info-subtle"
          onClick={() => navigate('/jobs')}
        />
        <MetricCard
          label={t('dashboard.sessions')}
          value={String(sessionCount)}
          icon={<MessageSquare className="h-4 w-4" />}
          iconColor="text-brand"
          iconBg="bg-brand-subtle"
          onClick={() => navigate('/sessions')}
        />
      </div>

      {/* Quick actions */}
      <div className="mt-8">
        <SectionLabel>{t('dashboard.quickAccess')}</SectionLabel>
        <div className="mt-2.5 grid grid-cols-2 gap-3 sm:grid-cols-4">
          <QuickAction icon={Settings} label={t('dashboard.providers')} onClick={() => navigate('/config')} />
          <QuickAction icon={Puzzle} label={t('sidebar.skills')} onClick={() => navigate('/skills')} />
          <QuickAction icon={MessageSquare} label={t('sidebar.chat')} onClick={() => navigate('/')} />
          <QuickAction icon={Shield} label={t('sidebar.security')} onClick={() => navigate('/security')} />
        </div>
      </div>

      {/* Channels */}
      {channels.length > 0 && (
        <div className="mt-8">
          <div className="flex items-center justify-between">
            <SectionLabel>{t('dashboard.channels')}</SectionLabel>
            <button
              onClick={() => navigate('/channels')}
              className="flex cursor-pointer items-center gap-1 text-[11px] text-text-muted transition-colors hover:text-text-primary"
            >
              {t('common.viewAll')} <ArrowRight className="h-3 w-3" />
            </button>
          </div>
          <div className="mt-2.5 space-y-2">
            {channels.map((ch) => (
              <Card
                key={ch.name}
                interactive
                padding="md"
                onClick={() => navigate('/channels')}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <StatusDot
                      status={ch.connected ? 'online' : 'offline'}
                      pulse={ch.connected}
                    />
                    <span className="text-sm font-medium capitalize text-text-primary">{ch.name}</span>
                  </div>
                  <span
                    className={cn(
                      'text-[11px] font-medium',
                      ch.connected ? 'text-success' : 'text-text-muted'
                    )}
                  >
                    {ch.connected ? t('common.online') : t('common.offline')}
                  </span>
                </div>
              </Card>
            ))}
          </div>
        </div>
      )}

      {/* Recent sessions */}
      {sessionCount > 0 && (
        <div className="mt-8 mb-6">
          <div className="flex items-center justify-between">
            <SectionLabel>{t('dashboard.recentSessions')}</SectionLabel>
            <button
              onClick={() => navigate('/sessions')}
              className="flex cursor-pointer items-center gap-1 text-[11px] text-text-muted transition-colors hover:text-text-primary"
            >
              {t('common.viewAll')} <ArrowRight className="h-3 w-3" />
            </button>
          </div>
          <div className="mt-2.5 space-y-2">
            {(data.sessions ?? []).slice(0, 5).map((s) => (
              <Card
                key={s.id}
                interactive
                padding="md"
                onClick={() => navigate(`/chat/${encodeURIComponent(s.id)}`)}
              >
                <div className="flex items-center justify-between">
                  <div className="flex min-w-0 items-center gap-3">
                    <Zap className="h-3.5 w-3.5 shrink-0 text-brand" />
                    <span className="truncate text-sm text-text-primary">{s.id}</span>
                  </div>
                  <span className="shrink-0 text-[11px] text-text-muted">
                    {s.message_count} {t('common.msgs')} &middot; {timeAgo(s.last_message_at, t)}
                  </span>
                </div>
              </Card>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

/* -- Section label -- */
function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">
      {children}
    </p>
  )
}

/* -- Metric Card -- */
function MetricCard({
  label,
  value,
  icon,
  iconColor,
  iconBg,
  trend,
  onClick,
}: {
  label: string
  value: string
  icon: React.ReactNode
  iconColor: string
  iconBg: string
  trend?: 'ok'
  onClick?: () => void
}) {
  return (
    <Card interactive padding="lg" onClick={onClick}>
      <div className="flex items-center justify-between">
        <div className={cn('flex h-9 w-9 items-center justify-center rounded-lg', iconBg)}>
          <span className={iconColor}>{icon}</span>
        </div>
        {trend === 'ok' && (
          <TrendingUp className="h-4 w-4 text-success" />
        )}
      </div>
      <p className="mt-3 text-2xl font-bold tracking-tight text-text-primary">{value}</p>
      <p className="mt-0.5 text-[11px] font-semibold uppercase tracking-wider text-text-muted">
        {label}
      </p>
    </Card>
  )
}

/* -- Quick Action -- */
function QuickAction({ icon: Icon, label, onClick }: {
  icon: React.ElementType
  label: string
  onClick: () => void
}) {
  return (
    <Card interactive padding="md" onClick={onClick} className="group">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-bg-subtle text-text-muted transition-colors group-hover:bg-brand-subtle group-hover:text-brand">
          <Icon className="h-4 w-4" />
        </div>
        <span className="text-sm font-medium text-text-secondary transition-colors group-hover:text-text-primary">
          {label}
        </span>
      </div>
    </Card>
  )
}

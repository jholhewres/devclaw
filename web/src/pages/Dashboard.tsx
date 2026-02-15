import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  MessageSquare,
  Cpu,
  Radio,
  Clock,
  ArrowRight,
  CheckCircle2,
  XCircle,
} from 'lucide-react'
import { api, type DashboardData } from '@/lib/api'
import { Badge } from '@/components/ui/Badge'
import { formatTokens, timeAgo } from '@/lib/utils'

/**
 * Dashboard — visão geral do assistente.
 * Métricas, status dos canais, últimas conversas.
 */
export function Dashboard() {
  const navigate = useNavigate()
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.dashboard()
      .then(setData)
      .catch(() => {
        // Fallback: carregar individualmente
        Promise.all([
          api.sessions.list().catch(() => []),
          api.usage().catch(() => ({ total_input_tokens: 0, total_output_tokens: 0, total_cost: 0, request_count: 0 })),
          api.channels.list().catch(() => []),
          api.jobs.list().catch(() => []),
        ]).then(([sessions, usage, channels, jobs]) => {
          setData({ sessions, usage, channels, jobs })
        })
      })
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-900 dark:border-zinc-700 dark:border-t-zinc-100" />
      </div>
    )
  }

  if (!data) return null

  const connectedChannels = data.channels.filter((c) => c.connected).length
  const activeJobs = data.jobs.filter((j) => j.enabled).length
  const totalTokens = data.usage.total_input_tokens + data.usage.total_output_tokens

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-4xl px-6 py-8">
        <h1 className="text-xl font-semibold">Dashboard</h1>
        <p className="mt-1 text-sm text-zinc-500">Visão geral do seu assistente</p>

        {/* Métricas */}
        <div className="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
          <MetricCard
            icon={MessageSquare}
            label="Sessões"
            value={String(data.sessions.length)}
          />
          <MetricCard
            icon={Cpu}
            label="Tokens"
            value={formatTokens(totalTokens)}
          />
          <MetricCard
            icon={Radio}
            label="Canais"
            value={`${connectedChannels}/${data.channels.length}`}
            variant={connectedChannels === data.channels.length ? 'success' : 'warning'}
          />
          <MetricCard
            icon={Clock}
            label="Jobs"
            value={`${activeJobs} ativos`}
          />
        </div>

        {/* Canais + Conversas recentes */}
        <div className="mt-8 grid gap-6 lg:grid-cols-2">
          {/* Canais */}
          <div className="rounded-xl border border-zinc-200 dark:border-zinc-800">
            <div className="flex items-center justify-between border-b border-zinc-200 px-4 py-3 dark:border-zinc-800">
              <h2 className="text-sm font-medium">Canais</h2>
              <button
                onClick={() => navigate('/channels')}
                className="text-xs text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors"
              >
                Ver todos <ArrowRight className="inline h-3 w-3" />
              </button>
            </div>
            <div className="divide-y divide-zinc-100 dark:divide-zinc-800">
              {data.channels.length === 0 ? (
                <p className="px-4 py-6 text-center text-sm text-zinc-400">
                  Nenhum canal configurado
                </p>
              ) : (
                data.channels.map((ch) => (
                  <div key={ch.name} className="flex items-center justify-between px-4 py-2.5">
                    <div className="flex items-center gap-2">
                      {ch.connected ? (
                        <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                      ) : (
                        <XCircle className="h-4 w-4 text-red-400" />
                      )}
                      <span className="text-sm capitalize">{ch.name}</span>
                    </div>
                    <Badge variant={ch.connected ? 'success' : 'error'}>
                      {ch.connected ? 'Conectado' : 'Offline'}
                    </Badge>
                  </div>
                ))
              )}
            </div>
          </div>

          {/* Conversas recentes */}
          <div className="rounded-xl border border-zinc-200 dark:border-zinc-800">
            <div className="flex items-center justify-between border-b border-zinc-200 px-4 py-3 dark:border-zinc-800">
              <h2 className="text-sm font-medium">Conversas recentes</h2>
              <button
                onClick={() => navigate('/sessions')}
                className="text-xs text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors"
              >
                Ver todas <ArrowRight className="inline h-3 w-3" />
              </button>
            </div>
            <div className="divide-y divide-zinc-100 dark:divide-zinc-800">
              {data.sessions.length === 0 ? (
                <p className="px-4 py-6 text-center text-sm text-zinc-400">
                  Nenhuma conversa ainda
                </p>
              ) : (
                data.sessions.slice(0, 5).map((session) => (
                  <button
                    key={session.id}
                    onClick={() => navigate(`/chat/${encodeURIComponent(session.id)}`)}
                    className="flex w-full items-center justify-between px-4 py-2.5 text-left hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm">{session.chat_id || session.id}</p>
                      <p className="text-xs text-zinc-400">
                        {session.message_count} msgs · {session.channel}
                      </p>
                    </div>
                    <span className="ml-3 shrink-0 text-xs text-zinc-400">
                      {timeAgo(session.last_message_at)}
                    </span>
                  </button>
                ))
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

/* ── Metric Card ── */

function MetricCard({
  icon: Icon,
  label,
  value,
  variant,
}: {
  icon: React.ElementType
  label: string
  value: string
  variant?: 'success' | 'warning'
}) {
  return (
    <div className="rounded-xl border border-zinc-200 px-4 py-3 dark:border-zinc-800">
      <div className="flex items-center gap-2">
        <Icon className={`h-4 w-4 ${
          variant === 'success' ? 'text-emerald-500' :
          variant === 'warning' ? 'text-amber-500' :
          'text-zinc-400'
        }`} />
        <span className="text-xs text-zinc-500">{label}</span>
      </div>
      <p className="mt-1 text-lg font-semibold">{value}</p>
    </div>
  )
}

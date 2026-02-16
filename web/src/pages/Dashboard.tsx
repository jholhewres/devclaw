import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  MessageSquare,
  Cpu,
  Radio,
  Clock,
  ArrowRight,
  Zap,
  Shield,
  Settings,
} from 'lucide-react'
import { api, type DashboardData } from '@/lib/api'
import { formatTokens, timeAgo } from '@/lib/utils'

export function Dashboard() {
  const navigate = useNavigate()
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
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
        })
      })
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-[#0a0a0f]">
        <div className="h-10 w-10 rounded-full border-4 border-orange-500/30 border-t-orange-500 animate-spin" />
      </div>
    )
  }

  if (!data) return null

  const sessions = data.sessions ?? []
  const channels = data.channels ?? []
  const jobs = data.jobs ?? []
  const usage = data.usage ?? { total_input_tokens: 0, total_output_tokens: 0, total_cost: 0, request_count: 0 }
  const connectedChannels = channels.filter((c) => c.connected).length
  const activeJobs = jobs.filter((j) => j.enabled).length
  const totalTokens = usage.total_input_tokens + usage.total_output_tokens

  return (
    <div className="flex-1 overflow-y-auto bg-[#0a0a0f]">
      <div className="mx-auto max-w-5xl px-8 py-10">

        {/* Section: Quick Stats */}
        <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-gray-600">Visao geral</p>
        <h1 className="mt-1 text-2xl font-black text-white tracking-tight">Dashboard</h1>

        {/* Game-mode style cards */}
        <div className="mt-8 grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard
            icon={MessageSquare}
            title="Sessoes"
            value={String(sessions.length)}
            subtitle="conversas ativas"
            onClick={() => navigate('/sessions')}
          />
          <StatCard
            icon={Cpu}
            title="Tokens"
            value={formatTokens(totalTokens)}
            subtitle="total consumido"
          />
          <StatCard
            icon={Radio}
            title="Canais"
            value={`${connectedChannels}/${channels.length}`}
            subtitle={connectedChannels === channels.length && channels.length > 0 ? 'todos online' : 'verificar conexao'}
            active={connectedChannels === channels.length && channels.length > 0}
            onClick={() => navigate('/channels')}
          />
          <StatCard
            icon={Clock}
            title="Jobs"
            value={String(activeJobs)}
            subtitle="agendados ativos"
            onClick={() => navigate('/jobs')}
          />
        </div>

        {/* Quick actions */}
        <p className="mt-10 text-[11px] font-bold uppercase tracking-[0.15em] text-gray-600">Acoes rapidas</p>
        <h2 className="mt-1 text-xl font-bold text-white">Gerenciar</h2>

        <div className="mt-5 grid grid-cols-1 gap-4 sm:grid-cols-3">
          <ActionCard
            icon={Zap}
            title="Skills"
            description="Gerencie as habilidades do assistente"
            onClick={() => navigate('/skills')}
          />
          <ActionCard
            icon={Shield}
            title="Seguranca"
            description="Audit log, vault e permissoes"
            onClick={() => navigate('/security')}
          />
          <ActionCard
            icon={Settings}
            title="Configuracao"
            description="Edite o config.yaml do sistema"
            onClick={() => navigate('/config')}
          />
        </div>

        {/* Bottom grid */}
        <div className="mt-10 grid gap-5 lg:grid-cols-2">
          {/* Channels */}
          <div className="rounded-2xl border border-white/[0.06] bg-[#111118] overflow-hidden">
            <div className="flex items-center justify-between border-b border-white/[0.06] px-6 py-4">
              <div>
                <p className="text-[10px] font-bold uppercase tracking-[0.15em] text-gray-600">Status</p>
                <h3 className="text-lg font-bold text-white">Canais</h3>
              </div>
              <button
                onClick={() => navigate('/channels')}
                className="flex cursor-pointer items-center gap-1.5 rounded-xl bg-white/[0.04] px-4 py-2 text-xs font-semibold text-gray-400 transition-all hover:bg-orange-500/10 hover:text-orange-400"
              >
                Ver todos <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="divide-y divide-white/[0.04] px-2">
              {channels.length === 0 ? (
                <p className="px-4 py-10 text-center text-base text-gray-600">Nenhum canal configurado</p>
              ) : (
                channels.map((ch) => (
                  <div key={ch.name} className="flex items-center justify-between px-4 py-4">
                    <div className="flex items-center gap-4">
                      <div className={`h-3 w-3 rounded-full ${ch.connected ? 'bg-emerald-400 shadow-lg shadow-emerald-400/40' : 'bg-red-500'}`} />
                      <span className="text-base font-semibold text-white capitalize">{ch.name}</span>
                    </div>
                    <span className={`rounded-full px-3 py-1 text-xs font-bold ${
                      ch.connected
                        ? 'bg-emerald-500/15 text-emerald-400'
                        : 'bg-red-500/15 text-red-400'
                    }`}>
                      {ch.connected ? 'Online' : 'Offline'}
                    </span>
                  </div>
                ))
              )}
            </div>
          </div>

          {/* Recent sessions */}
          <div className="rounded-2xl border border-white/[0.06] bg-[#111118] overflow-hidden">
            <div className="flex items-center justify-between border-b border-white/[0.06] px-6 py-4">
              <div>
                <p className="text-[10px] font-bold uppercase tracking-[0.15em] text-gray-600">Recentes</p>
                <h3 className="text-lg font-bold text-white">Conversas</h3>
              </div>
              <button
                onClick={() => navigate('/sessions')}
                className="flex cursor-pointer items-center gap-1.5 rounded-xl bg-white/[0.04] px-4 py-2 text-xs font-semibold text-gray-400 transition-all hover:bg-orange-500/10 hover:text-orange-400"
              >
                Ver todas <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="divide-y divide-white/[0.04] px-2">
              {sessions.length === 0 ? (
                <p className="px-4 py-10 text-center text-base text-gray-600">Nenhuma conversa ainda</p>
              ) : (
                sessions.slice(0, 5).map((session) => (
                  <button
                    key={session.id}
                    onClick={() => navigate(`/chat/${encodeURIComponent(session.id)}`)}
                    className="flex w-full cursor-pointer items-center justify-between px-4 py-4 text-left transition-colors hover:bg-white/[0.02]"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-base font-semibold text-white">{session.chat_id || session.id}</p>
                      <p className="mt-0.5 text-sm text-gray-500">{session.message_count} msgs · {session.channel}</p>
                    </div>
                    <span className="ml-4 shrink-0 text-sm text-gray-600">{timeAgo(session.last_message_at)}</span>
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

/* ── Stat Card (game-mode style) ── */

function StatCard({ icon: Icon, title, value, subtitle, active, onClick }: {
  icon: React.ElementType
  title: string
  value: string
  subtitle: string
  active?: boolean
  onClick?: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={`group relative cursor-pointer overflow-hidden rounded-2xl border p-6 text-left transition-all ${
        active
          ? 'border-orange-500/30 bg-orange-500/[0.04]'
          : 'border-white/[0.06] bg-[#111118] hover:border-orange-500/20'
      }`}
    >
      {/* Icon */}
      <div className={`flex h-12 w-12 items-center justify-center rounded-xl ${
        active ? 'bg-orange-500/15 text-orange-400' : 'bg-white/[0.05] text-gray-500 group-hover:text-orange-400'
      } transition-colors`}>
        <Icon className="h-6 w-6" />
      </div>

      {/* Content */}
      <h3 className="mt-4 text-2xl font-black text-white">{value}</h3>
      <p className="mt-0.5 text-sm font-semibold text-orange-400">{title}</p>
      <p className="mt-1 text-xs text-gray-500">{subtitle}</p>

      {active && (
        <div className="absolute right-3 top-3">
          <span className="rounded-full bg-orange-500 px-2 py-0.5 text-[10px] font-bold text-white">ativo</span>
        </div>
      )}
    </button>
  )
}

/* ── Action Card ── */

function ActionCard({ icon: Icon, title, description, onClick }: {
  icon: React.ElementType
  title: string
  description: string
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className="group flex cursor-pointer items-start gap-5 rounded-2xl border border-white/[0.06] bg-[#111118] p-6 text-left transition-all hover:border-orange-500/20"
    >
      <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-white/[0.05] text-gray-500 transition-colors group-hover:bg-orange-500/15 group-hover:text-orange-400">
        <Icon className="h-6 w-6" />
      </div>
      <div>
        <h3 className="text-lg font-bold text-white">{title}</h3>
        <p className="mt-1 text-sm text-gray-500">{description}</p>
      </div>
    </button>
  )
}

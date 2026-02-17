import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Cpu,
  Radio,
  Clock,
  ArrowRight,
  Puzzle,
  Shield,
  Settings,
  MessageSquare,
} from 'lucide-react'
import { api, type DashboardData } from '@/lib/api'
import { formatTokens } from '@/lib/utils'

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
      <div className="flex flex-1 items-center justify-center bg-[var(--color-dc-darker)]">
        <div className="h-10 w-10 rounded-full border-4 border-orange-500/30 border-t-orange-500 animate-spin" />
      </div>
    )
  }

  if (!data) return null

  const channels = data.channels ?? []
  const jobs = data.jobs ?? []
  const usage = data.usage ?? { total_input_tokens: 0, total_output_tokens: 0, total_cost: 0, request_count: 0 }
  const connectedChannels = channels.filter((c) => c.connected).length
  const activeJobs = jobs.filter((j) => j.enabled).length
  const totalTokens = usage.total_input_tokens + usage.total_output_tokens

  return (
    <div className="flex-1 overflow-y-auto bg-[var(--color-dc-darker)]">
      <div className="mx-auto max-w-5xl px-8 py-10">

        {/* Status bar */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-white">Dashboard</h1>
            <p className="mt-1 text-sm text-gray-500">Controle do agente DevClaw</p>
          </div>
          <div className="flex items-center gap-4 rounded-xl border border-white/[0.06] bg-[var(--color-dc-dark)] px-5 py-3">
            <div className="text-right">
              <p className="text-xs text-gray-500">Tokens hoje</p>
              <p className="text-sm font-semibold text-white">{formatTokens(totalTokens)}</p>
            </div>
            <div className="h-8 w-px bg-white/[0.06]" />
            <div className="text-right">
              <p className="text-xs text-gray-500">Custo est.</p>
              <p className="text-sm font-semibold text-white">${usage.total_cost?.toFixed(2) ?? '0.00'}</p>
            </div>
          </div>
        </div>

        {/* Stat cards */}
        <div className="mt-8 grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard
            icon={Cpu}
            title="Requisicoes"
            value={String(usage.request_count ?? 0)}
            subtitle="chamadas ao LLM"
          />
          <StatCard
            icon={Radio}
            title="Canais"
            value={`${connectedChannels}/${channels.length}`}
            subtitle={connectedChannels === channels.length && channels.length > 0 ? 'todos online' : 'verificar'}
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
          <StatCard
            icon={MessageSquare}
            title="Sessoes"
            value={String((data.sessions ?? []).length)}
            subtitle="conversas"
            onClick={() => navigate('/sessions')}
          />
        </div>

        {/* Quick actions */}
        <h2 className="mt-10 text-lg font-semibold text-white">Acesso rapido</h2>
        <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <QuickAction icon={Settings} label="Configurar Provider" onClick={() => navigate('/config')} />
          <QuickAction icon={Puzzle} label="Gerenciar Skills" onClick={() => navigate('/skills')} />
          <QuickAction icon={MessageSquare} label="Abrir Chat" onClick={() => navigate('/chat')} />
          <QuickAction icon={Shield} label="Seguranca & Vault" onClick={() => navigate('/security')} />
        </div>

        {/* Channels status */}
        <div className="mt-10">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold text-white">Canais</h2>
            <button
              onClick={() => navigate('/channels')}
              className="flex cursor-pointer items-center gap-1.5 text-sm text-gray-500 transition-colors hover:text-orange-400"
            >
              Ver todos <ArrowRight className="h-3.5 w-3.5" />
            </button>
          </div>
          <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            {channels.length === 0 ? (
              <p className="col-span-full rounded-xl border border-white/[0.06] bg-[var(--color-dc-dark)] px-6 py-8 text-center text-sm text-gray-500">
                Nenhum canal configurado
              </p>
            ) : (
              channels.map((ch) => (
                <div
                  key={ch.name}
                  className="flex items-center justify-between rounded-xl border border-white/[0.06] bg-[var(--color-dc-dark)] px-5 py-4"
                >
                  <span className="text-sm font-medium text-white capitalize">{ch.name}</span>
                  <div className="flex items-center gap-2">
                    <div className={`h-2.5 w-2.5 rounded-full ${ch.connected ? 'bg-emerald-400' : 'bg-red-500'}`} />
                    <span className={`text-xs font-medium ${ch.connected ? 'text-emerald-400' : 'text-red-400'}`}>
                      {ch.connected ? 'Online' : 'Offline'}
                    </span>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

      </div>
    </div>
  )
}

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
      className={`group cursor-pointer rounded-xl border p-5 text-left transition-all ${
        active
          ? 'border-orange-500/20 bg-orange-500/[0.04]'
          : 'border-white/[0.06] bg-[var(--color-dc-dark)] hover:border-white/[0.1]'
      }`}
    >
      <div className={`flex h-10 w-10 items-center justify-center rounded-lg ${
        active ? 'bg-orange-500/15 text-orange-400' : 'bg-white/[0.05] text-gray-500'
      } transition-colors`}>
        <Icon className="h-5 w-5" />
      </div>
      <h3 className="mt-3 text-2xl font-bold text-white">{value}</h3>
      <p className="text-sm font-medium text-gray-400">{title}</p>
      <p className="mt-0.5 text-xs text-gray-600">{subtitle}</p>
    </button>
  )
}

function QuickAction({ icon: Icon, label, onClick }: {
  icon: React.ElementType
  label: string
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className="group flex cursor-pointer items-center gap-3 rounded-xl border border-white/[0.06] bg-[var(--color-dc-dark)] px-5 py-4 text-left transition-all hover:border-white/[0.1]"
    >
      <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-white/[0.05] text-gray-500 transition-colors group-hover:bg-orange-500/15 group-hover:text-orange-400">
        <Icon className="h-4.5 w-4.5" />
      </div>
      <span className="text-sm font-medium text-gray-300 group-hover:text-white">{label}</span>
    </button>
  )
}

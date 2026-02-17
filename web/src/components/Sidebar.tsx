import { useNavigate, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  Puzzle,
  Radio,
  Clock,
  Settings,
  Shield,
  MessageSquare,
  Terminal,
  PanelLeftClose,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAppStore } from '@/stores/app'
import type { SessionInfo } from '@/lib/api'

interface SidebarProps {
  sessions: SessionInfo[]
}

/** Navigation grouped by purpose: Control (daily operations) and Config (settings). */
const controlLinks = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/channels', icon: Radio, label: 'Canais' },
  { to: '/jobs', icon: Clock, label: 'Jobs' },
  { to: '/sessions', icon: MessageSquare, label: 'Sessoes' },
]

const configLinks = [
  { to: '/config', icon: Settings, label: 'Provider & Config' },
  { to: '/skills', icon: Puzzle, label: 'Skills' },
  { to: '/security', icon: Shield, label: 'Seguranca & Vault' },
]

export function Sidebar({ sessions: _sessions }: SidebarProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const { sidebarOpen, toggleSidebar } = useAppStore()

  if (!sidebarOpen) return null

  return (
    <aside className="flex h-full w-64 flex-col border-r border-white/[0.06] bg-[var(--color-dc-dark)]">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-4">
        <button
          onClick={() => navigate('/')}
          className="flex cursor-pointer items-center gap-2.5 transition-opacity hover:opacity-80"
        >
          <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-orange-500/15">
            <Terminal className="h-5 w-5 text-orange-400" />
          </div>
          <span className="text-base font-bold text-white">
            Dev<span className="text-orange-400">Claw</span>
          </span>
        </button>
        <button
          onClick={toggleSidebar}
          title="Fechar sidebar"
          className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-gray-500 transition-colors hover:bg-white/[0.06] hover:text-gray-300"
        >
          <PanelLeftClose className="h-4.5 w-4.5" />
        </button>
      </div>

      {/* Control section */}
      <nav className="flex-1 overflow-y-auto px-3 pb-3">
        <p className="mb-1.5 px-2 text-[11px] font-semibold uppercase tracking-wider text-gray-600">
          Controle
        </p>
        {controlLinks.map(({ to, icon: Icon, label }) => {
          const isActive = location.pathname === to || (to !== '/' && location.pathname.startsWith(to))
          return (
            <button
              key={to}
              onClick={() => navigate(to)}
              className={cn(
                'flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                isActive
                  ? 'bg-orange-500/10 text-orange-400'
                  : 'text-gray-400 hover:bg-white/[0.04] hover:text-gray-200',
              )}
            >
              <Icon className="h-4.5 w-4.5" />
              {label}
            </button>
          )
        })}

        <p className="mb-1.5 mt-6 px-2 text-[11px] font-semibold uppercase tracking-wider text-gray-600">
          Configuracao
        </p>
        {configLinks.map(({ to, icon: Icon, label }) => {
          const isActive = location.pathname === to
          return (
            <button
              key={to}
              onClick={() => navigate(to)}
              className={cn(
                'flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                isActive
                  ? 'bg-orange-500/10 text-orange-400'
                  : 'text-gray-400 hover:bg-white/[0.04] hover:text-gray-200',
              )}
            >
              <Icon className="h-4.5 w-4.5" />
              {label}
            </button>
          )
        })}

        {/* Chat quick link */}
        <div className="mt-6">
          <button
            onClick={() => navigate('/chat')}
            className="flex w-full cursor-pointer items-center justify-center gap-2 rounded-lg border border-orange-500/20 bg-orange-500/5 px-3 py-2.5 text-sm font-medium text-orange-400 transition-all hover:bg-orange-500/10"
          >
            <MessageSquare className="h-4 w-4" />
            Abrir Chat
          </button>
        </div>
      </nav>

      {/* Footer */}
      <div className="border-t border-white/[0.06] px-4 py-3">
        <div className="flex items-center justify-between">
          <p className="text-xs text-gray-600">DevClaw v1.6.0</p>
          <div className="flex items-center gap-1.5">
            <div className="h-2 w-2 rounded-full bg-emerald-400" />
            <span className="text-[11px] text-gray-500">online</span>
          </div>
        </div>
      </div>
    </aside>
  )
}

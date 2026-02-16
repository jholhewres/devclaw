import { useNavigate, useLocation } from 'react-router-dom'
import {
  MessageSquare,
  LayoutDashboard,
  Puzzle,
  Radio,
  Settings,
  Shield,
  Clock,
  Plus,
  PanelLeftClose,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAppStore } from '@/stores/app'
import type { SessionInfo } from '@/lib/api'
import { timeAgo, truncate } from '@/lib/utils'

interface SidebarProps {
  sessions: SessionInfo[]
}

const navLinks = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/skills', icon: Puzzle, label: 'Skills' },
  { to: '/channels', icon: Radio, label: 'Canais' },
  { to: '/jobs', icon: Clock, label: 'Jobs' },
  { to: '/config', icon: Settings, label: 'Config' },
  { to: '/security', icon: Shield, label: 'Seguranca' },
]

export function Sidebar({ sessions }: SidebarProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const { sidebarOpen, toggleSidebar } = useAppStore()

  const grouped = groupByDate(sessions ?? [])

  if (!sidebarOpen) return null

  return (
    <aside className="flex h-full w-64 flex-col border-r border-white/[0.06] bg-[#111118]">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-3">
        <button
          onClick={() => navigate('/chat')}
          className="flex cursor-pointer items-center gap-2.5 transition-opacity hover:opacity-80"
        >
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-orange-500 to-amber-500">
            <svg className="h-4 w-4 text-white" viewBox="0 0 24 24" fill="currentColor">
              <ellipse cx="7" cy="5" rx="2.5" ry="3" />
              <ellipse cx="17" cy="5" rx="2.5" ry="3" />
              <ellipse cx="3.5" cy="11" rx="2" ry="2.5" />
              <ellipse cx="20.5" cy="11" rx="2" ry="2.5" />
              <path d="M7 14c0-2.8 2.2-5 5-5s5 2.2 5 5c0 3.5-2 6-5 7-3-1-5-3.5-5-7z" />
            </svg>
          </div>
          <span className="text-sm font-bold tracking-wide text-white">
            Go<span className="text-orange-400">Claw</span>
          </span>
        </button>
        <div className="flex items-center gap-0.5">
          <button
            onClick={() => navigate('/chat')}
            title="Nova conversa"
            className="flex h-7 w-7 cursor-pointer items-center justify-center rounded-lg text-gray-500 transition-colors hover:bg-white/[0.06] hover:text-orange-400"
          >
            <Plus className="h-4 w-4" />
          </button>
          <button
            onClick={toggleSidebar}
            title="Fechar sidebar"
            className="flex h-7 w-7 cursor-pointer items-center justify-center rounded-lg text-gray-500 transition-colors hover:bg-white/[0.06] hover:text-gray-300"
          >
            <PanelLeftClose className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Sessions */}
      <div className="flex-1 overflow-y-auto px-2 pb-2">
        {grouped.map(({ label, items }) => (
          <div key={label} className="mb-3">
            <p className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wider text-gray-600">
              {label}
            </p>
            {items.map((session) => {
              const isActive = location.pathname === `/chat/${encodeURIComponent(session.id)}`
              return (
                <button
                  key={session.id}
                  onClick={() => navigate(`/chat/${encodeURIComponent(session.id)}`)}
                  className={cn(
                    'flex w-full cursor-pointer flex-col rounded-lg px-2 py-1.5 text-left text-sm transition-all',
                    isActive
                      ? 'bg-orange-500/10 text-white border-l-2 border-orange-500'
                      : 'text-gray-400 hover:bg-white/[0.04] hover:text-gray-200',
                  )}
                >
                  <div className="flex items-center gap-2">
                    <MessageSquare className={cn('h-3.5 w-3.5 shrink-0', isActive ? 'text-orange-400' : 'text-gray-600')} />
                    <span className="truncate text-[13px]">{truncate(session.chat_id || session.id, 28)}</span>
                  </div>
                  <p className="ml-[22px] text-[10px] text-gray-600">
                    {timeAgo(session.last_message_at)}
                  </p>
                </button>
              )
            })}
          </div>
        ))}

        {(sessions ?? []).length === 0 && (
          <div className="flex flex-col items-center gap-2 px-3 py-12">
            <MessageSquare className="h-5 w-5 text-gray-700" />
            <p className="text-xs text-gray-600">Nenhuma conversa</p>
          </div>
        )}
      </div>

      {/* Nav links */}
      <nav className="border-t border-white/[0.06] px-2 py-2">
        {navLinks.map(({ to, icon: Icon, label }) => {
          const isActive = location.pathname === to
          return (
            <button
              key={to}
              onClick={() => navigate(to)}
              className={cn(
                'flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-[13px] font-medium transition-all',
                isActive
                  ? 'bg-orange-500/10 text-orange-400'
                  : 'text-gray-500 hover:bg-white/[0.04] hover:text-gray-300',
              )}
            >
              <Icon className="h-4 w-4" />
              {label}
            </button>
          )
        })}
      </nav>

      {/* Footer */}
      <div className="border-t border-white/[0.06] px-3 py-2.5">
        <p className="text-[10px] text-gray-700">GoClaw v0.1</p>
      </div>
    </aside>
  )
}

/* ── Helpers ── */

interface GroupedSessions {
  label: string
  items: SessionInfo[]
}

function groupByDate(sessions: SessionInfo[]): GroupedSessions[] {
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const yesterday = new Date(today.getTime() - 86400000)
  const weekAgo = new Date(today.getTime() - 7 * 86400000)

  const groups: Record<string, SessionInfo[]> = {
    Hoje: [],
    Ontem: [],
    'Esta semana': [],
    Anteriores: [],
  }

  const sorted = [...sessions].sort(
    (a, b) => new Date(b.last_message_at).getTime() - new Date(a.last_message_at).getTime(),
  )

  for (const s of sorted) {
    const d = new Date(s.last_message_at)
    if (d >= today) groups['Hoje'].push(s)
    else if (d >= yesterday) groups['Ontem'].push(s)
    else if (d >= weekAgo) groups['Esta semana'].push(s)
    else groups['Anteriores'].push(s)
  }

  return Object.entries(groups)
    .filter(([, items]) => items.length > 0)
    .map(([label, items]) => ({ label, items }))
}

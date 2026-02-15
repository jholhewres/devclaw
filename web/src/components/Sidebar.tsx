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
  Sun,
  Moon,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAppStore } from '@/stores/app'
import { Button } from '@/components/ui/Button'
import type { SessionInfo } from '@/lib/api'
import { timeAgo, truncate } from '@/lib/utils'

interface SidebarProps {
  /** Lista de sessões para exibir */
  sessions: SessionInfo[]
}

/** Links de navegação */
const navLinks = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/skills', icon: Puzzle, label: 'Skills' },
  { to: '/channels', icon: Radio, label: 'Canais' },
  { to: '/jobs', icon: Clock, label: 'Jobs' },
  { to: '/config', icon: Settings, label: 'Config' },
  { to: '/security', icon: Shield, label: 'Segurança' },
]

export function Sidebar({ sessions }: SidebarProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const { sidebarOpen, toggleSidebar, theme, setTheme } = useAppStore()

  /** Agrupar sessões por período */
  const grouped = groupByDate(sessions)

  if (!sidebarOpen) return null

  return (
    <aside className="flex h-full w-64 flex-col border-r border-zinc-200 bg-zinc-50 dark:border-zinc-800 dark:bg-zinc-900">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-3">
        <button
          onClick={() => navigate('/chat')}
          className="flex items-center gap-2 text-sm font-semibold hover:opacity-80 transition-opacity"
        >
          <span className="text-lg">🐾</span>
          <span>GoClaw</span>
        </button>
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="icon" onClick={() => navigate('/chat')} title="Nova conversa">
            <Plus className="h-4 w-4" />
          </Button>
          <Button variant="ghost" size="icon" onClick={toggleSidebar} title="Fechar sidebar">
            <PanelLeftClose className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Sessões de chat */}
      <div className="flex-1 overflow-y-auto px-2 pb-2">
        {grouped.map(({ label, items }) => (
          <div key={label} className="mb-3">
            <p className="px-2 py-1 text-[11px] font-medium uppercase tracking-wider text-zinc-400">
              {label}
            </p>
            {items.map((session) => (
              <button
                key={session.id}
                onClick={() => navigate(`/chat/${encodeURIComponent(session.id)}`)}
                className={cn(
                  'w-full rounded-lg px-2 py-1.5 text-left text-sm transition-colors',
                  'hover:bg-zinc-200/70 dark:hover:bg-zinc-800',
                  location.pathname === `/chat/${encodeURIComponent(session.id)}`
                    ? 'bg-zinc-200 dark:bg-zinc-800'
                    : '',
                )}
              >
                <div className="flex items-center gap-2">
                  <MessageSquare className="h-3.5 w-3.5 shrink-0 text-zinc-400" />
                  <span className="truncate">{truncate(session.chat_id || session.id, 28)}</span>
                </div>
                <p className="ml-5.5 text-[11px] text-zinc-400">
                  {timeAgo(session.last_message_at)}
                </p>
              </button>
            ))}
          </div>
        ))}

        {sessions.length === 0 && (
          <p className="px-3 py-8 text-center text-xs text-zinc-400">
            Nenhuma conversa ainda
          </p>
        )}
      </div>

      {/* Nav links */}
      <nav className="border-t border-zinc-200 px-2 py-2 dark:border-zinc-800">
        {navLinks.map(({ to, icon: Icon, label }) => (
          <button
            key={to}
            onClick={() => navigate(to)}
            className={cn(
              'flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-sm transition-colors',
              'hover:bg-zinc-200/70 dark:hover:bg-zinc-800',
              location.pathname === to
                ? 'bg-zinc-200 text-zinc-900 dark:bg-zinc-800 dark:text-zinc-100'
                : 'text-zinc-600 dark:text-zinc-400',
            )}
          >
            <Icon className="h-4 w-4" />
            {label}
          </button>
        ))}
      </nav>

      {/* Theme toggle */}
      <div className="border-t border-zinc-200 px-3 py-2 dark:border-zinc-800">
        <button
          onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
          className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-xs text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors"
        >
          {theme === 'dark' ? <Sun className="h-3.5 w-3.5" /> : <Moon className="h-3.5 w-3.5" />}
          {theme === 'dark' ? 'Modo claro' : 'Modo escuro'}
        </button>
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

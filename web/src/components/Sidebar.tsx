import { useEffect, useState } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  Puzzle,
  Radio,
  Clock,
  Settings,
  MessageSquare,
  ChevronLeft,
  ChevronRight,
  BarChart3,
  Menu,
  X,
  LogOut,
  Plus,
  History,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAppStore } from '@/stores/app'
import { Tooltip } from '@/components/ui/Tooltip'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { Logo } from '@/components/Logo'
import { api, type SessionInfo } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface MenuItem {
  nameKey: string
  icon: React.ElementType
  route: string
  sectionKey: string
}

/* ------------------------------------------------------------------ */
/*  Navigation data                                                    */
/* ------------------------------------------------------------------ */

const menuItems: MenuItem[] = [
  // Main
  {
    nameKey: 'sidebar.chat',
    icon: MessageSquare,
    route: '/',
    sectionKey: 'sidebarSections.main',
  },
  {
    nameKey: 'sidebar.sessions',
    icon: History,
    route: '/sessions',
    sectionKey: 'sidebarSections.main',
  },
  // Manage
  {
    nameKey: 'sidebar.skills',
    icon: Puzzle,
    route: '/skills',
    sectionKey: 'sidebarSections.manage',
  },
  {
    nameKey: 'sidebar.channels',
    icon: Radio,
    route: '/channels',
    sectionKey: 'sidebarSections.manage',
  },
  {
    nameKey: 'sidebar.jobs',
    icon: Clock,
    route: '/jobs',
    sectionKey: 'sidebarSections.manage',
  },
  {
    nameKey: 'sidebar.statistics',
    icon: BarChart3,
    route: '/stats',
    sectionKey: 'sidebarSections.manage',
  },
  // System
  {
    nameKey: 'sidebar.settings',
    icon: Settings,
    route: '/settings',
    sectionKey: 'sidebarSections.system',
  },
]

/* ------------------------------------------------------------------ */
/*  Sidebar                                                            */
/* ------------------------------------------------------------------ */

export function Sidebar() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()

  const sidebarOpen = useAppStore((s) => s.sidebarOpen)
  const setSidebarOpen = useAppStore((s) => s.setSidebarOpen)
  const sidebarCollapsed = useAppStore((s) => s.sidebarCollapsed)
  const setSidebarCollapsed = useAppStore((s) => s.setSidebarCollapsed)

  const [recentSessions, setRecentSessions] = useState<SessionInfo[]>([])

  /* ---- helpers ---- */

  const isActive = (route: string) => {
    if (location.pathname === route) return true
    if (route !== '/' && location.pathname.startsWith(route)) return true
    return false
  }

  /* Load recent sessions for sidebar list */
  useEffect(() => {
    api.sessions
      .list()
      .then((sessions) => {
        const webuiSessions = sessions
          .filter((s) => s.channel === 'webui' || s.id.startsWith('webui:'))
          .slice(0, 6)
        setRecentSessions(webuiSessions)
      })
      .catch(() => {})
  }, [location.pathname])

  /* Close mobile sidebar on desktop resize */
  useEffect(() => {
    const onResize = () => {
      if (window.innerWidth >= 1024) setSidebarOpen(false)
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [setSidebarOpen])

  const showFullText = !sidebarCollapsed || sidebarOpen

  /* Group items by section */
  const sections: Record<string, MenuItem[]> = {}
  menuItems.forEach((item) => {
    if (!sections[item.sectionKey]) sections[item.sectionKey] = []
    sections[item.sectionKey].push(item)
  })

  /* ---- nav item click ---- */

  const handleNavigate = (route: string) => {
    navigate(route)
    setSidebarOpen(false)
  }

  const handleLogout = () => {
    localStorage.removeItem('devclaw_token')
    window.location.href = '/login'
  }

  /* ---------------------------------------------------------------- */
  /*  Render helpers                                                   */
  /* ---------------------------------------------------------------- */

  const renderNavItem = (item: MenuItem) => {
    const Icon = item.icon
    const active = isActive(item.route)
    const label = t(item.nameKey)

    const button = (
      <button
        onClick={() => handleNavigate(item.route)}
        className={cn(
          'group relative flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all duration-200',
          active
            ? 'bg-bg-active text-brand'
            : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary',
          !showFullText && 'justify-center',
        )}
      >
        {active && (
          <span className="absolute left-0 top-1/2 h-6 w-[2px] -translate-y-1/2 rounded-r-full bg-brand" />
        )}
        <Icon className={cn('h-[18px] w-[18px] shrink-0', active && 'text-brand')} />
        {showFullText && <span>{label}</span>}
      </button>
    )

    if (!showFullText) {
      return (
        <Tooltip key={item.nameKey} content={label} side="right">
          {button}
        </Tooltip>
      )
    }

    return <div key={item.nameKey}>{button}</div>
  }

  /* ---------------------------------------------------------------- */
  /*  Render                                                           */
  /* ---------------------------------------------------------------- */

  return (
    <>
      {/* Mobile overlay backdrop */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/30 backdrop-blur-sm transition-opacity duration-200 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar panel */}
      <aside
        className={cn(
          'fixed left-0 top-0 z-50 flex h-screen flex-col border-r border-border bg-bg-surface transition-all duration-200',
          sidebarOpen ? 'w-64' : sidebarCollapsed ? 'w-[72px]' : 'w-64',
          sidebarOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0',
        )}
      >
        {/* ---- Logo area ---- */}
        <div className="flex h-14 items-center justify-between px-4">
          <button
            onClick={() => navigate('/')}
            className={cn(
              showFullText ? '' : 'flex w-full justify-center'
            )}
          >
            <Logo size="sm" iconOnly={!showFullText} />
          </button>

          {/* Mobile close */}
          <button
            onClick={() => setSidebarOpen(false)}
            className="flex h-8 w-8 items-center justify-center rounded-lg text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary lg:hidden"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* ---- Navigation ---- */}
        <nav className="flex-1 overflow-y-auto px-3 py-3">
          <div className="space-y-5">
            {Object.entries(sections).map(([sectionKey, items]) => (
              <div key={sectionKey} className="space-y-0.5">
                {showFullText ? (
                  <div className="mb-1.5 px-3">
                    <span className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">
                      {t(sectionKey)}
                    </span>
                  </div>
                ) : (
                  <div className="mb-1.5 flex justify-center">
                    <div className="h-px w-8 bg-border" />
                  </div>
                )}
                {items.map((item) => renderNavItem(item))}
              </div>
            ))}
          </div>

          {/* ---- Recent conversations ---- */}
          {showFullText && recentSessions.length > 0 && (
            <div className="mt-6 border-t border-border pt-4">
              <div className="mb-2 flex items-center justify-between px-3">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">
                  {t('sidebar.recent')}
                </span>
                <button
                  onClick={() => handleNavigate('/')}
                  className="rounded-md p-1 text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
                  title={t('sidebar.newConversation')}
                >
                  <Plus className="h-3.5 w-3.5" />
                </button>
              </div>
              <div className="space-y-0.5">
                {recentSessions.map((session) => {
                  const sessionActive =
                    location.pathname === `/chat/${encodeURIComponent(session.id)}`
                  return (
                    <button
                      key={session.id}
                      onClick={() =>
                        handleNavigate(`/chat/${encodeURIComponent(session.id)}`)
                      }
                      className={cn(
                        'flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left transition-all duration-200',
                        sessionActive
                          ? 'bg-brand-subtle text-text-primary'
                          : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary',
                      )}
                    >
                      <MessageSquare className="h-3.5 w-3.5 shrink-0 text-text-muted" />
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-xs">
                          {session.id.replace('webui:', '')}
                        </p>
                        <p className="text-[10px] text-text-muted">
                          {timeAgo(session.last_message_at)}
                        </p>
                      </div>
                    </button>
                  )
                })}
              </div>
            </div>
          )}
        </nav>

        {/* ---- Footer: language, logout, collapse ---- */}
        <div className="border-t border-border p-3">
          {/* Language + Logout row */}
          <div className={cn('flex items-center gap-1', !showFullText && 'flex-col')}>
            <LanguageSwitcher compact={!showFullText} />
            <Tooltip content={t('userMenu.logout')} side="right">
              <button
                onClick={handleLogout}
                className={cn(
                  'flex items-center gap-2 rounded-lg px-2.5 py-2 text-sm text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary',
                  !showFullText && 'justify-center px-2',
                )}
              >
                <LogOut className="h-4 w-4" />
              </button>
            </Tooltip>
          </div>

          {/* Collapse toggle (desktop only) */}
          <button
            onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
            className={cn(
              'mt-1 hidden w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-text-muted transition-all duration-200 hover:bg-bg-hover hover:text-text-primary lg:flex',
              !showFullText && 'justify-center',
            )}
            title={
              sidebarCollapsed ? t('sidebar.expandMenu') : t('sidebar.collapseMenu')
            }
          >
            {sidebarCollapsed ? (
              <ChevronRight className="h-[18px] w-[18px] shrink-0" />
            ) : (
              <>
                <ChevronLeft className="h-[18px] w-[18px] shrink-0" />
                <span>{t('sidebar.collapseMenu')}</span>
              </>
            )}
          </button>
        </div>
      </aside>

      {/* Mobile hamburger button */}
      <button
        onClick={() => setSidebarOpen(true)}
        className={cn(
          'fixed left-4 top-4 z-50 flex h-10 w-10 items-center justify-center rounded-lg border border-border bg-bg-surface text-text-primary shadow-md transition-opacity duration-200 lg:hidden',
          sidebarOpen ? 'pointer-events-none opacity-0' : 'opacity-100',
        )}
        aria-label={t('sidebar.menu')}
      >
        <Menu className="h-5 w-5" />
      </button>
    </>
  )
}

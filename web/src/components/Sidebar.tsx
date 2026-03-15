import { useEffect, useState } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  MessageSquare,
  Clock,
  Puzzle,
  ChevronLeft,
  ChevronRight,
  Menu,
  X,
  LogOut,
  Plus,
  Settings,
  Sun,
  Moon,
  Monitor,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAppStore } from '@/stores/app'
import { Tooltip } from '@/components/ui/Tooltip'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { Logo } from '@/components/Logo'
import { api, type SessionInfo } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

/* ------------------------------------------------------------------ */
/*  Navigation data                                                    */
/* ------------------------------------------------------------------ */

interface NavItem {
  nameKey: string
  icon: React.ElementType
  route: string
  /** Routes that should also highlight this item as active */
  activeRoutes?: string[]
  onClick?: () => void
}

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
  const sessionVersion = useAppStore((s) => s.sessionVersion)

  const [recentSessions, setRecentSessions] = useState<SessionInfo[]>([])

  const isActive = (route: string, extra?: string[]) => {
    if (location.pathname === route) return true
    if (route !== '/' && location.pathname.startsWith(route)) return true
    if (extra?.some((r) => location.pathname.startsWith(r))) return true
    return false
  }

  useEffect(() => {
    api.sessions
      .list()
      .then((sessions) => {
        const webuiSessions = (sessions || [])
          .filter((s) => s.channel === 'webui' || s.id.startsWith('webui:'))
          .slice(0, 8)
        setRecentSessions(webuiSessions)
      })
      .catch(() => {})
  }, [location.pathname, sessionVersion])

  useEffect(() => {
    const onResize = () => {
      if (window.innerWidth >= 1024) setSidebarOpen(false)
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [setSidebarOpen])

  const showFullText = !sidebarCollapsed || sidebarOpen

  const handleNavigate = (route: string) => {
    navigate(route)
    setSidebarOpen(false)
  }

  const handleNewConversation = () => {
    useAppStore.getState().setActiveSession(null)
    navigate('/')
    setSidebarOpen(false)
  }

  const handleLogout = () => {
    localStorage.removeItem('devclaw_token')
    window.location.href = '/login'
  }

  const navItems: NavItem[] = [
    {
      nameKey: 'sidebar.newConversation',
      icon: MessageSquare,
      route: '/',
      onClick: handleNewConversation,
    },
    {
      nameKey: 'sidebar.jobs',
      icon: Clock,
      route: '/jobs',
    },
    {
      nameKey: 'sidebar.skills',
      icon: Puzzle,
      route: '/skills',
    },
  ]

  /* ---- render nav item ---- */

  const renderNavItem = (item: NavItem) => {
    const Icon = item.icon
    const active = isActive(item.route, item.activeRoutes)
    const label = t(item.nameKey)

    const button = (
      <button
        onClick={() => {
          if (item.onClick) {
            item.onClick()
          } else {
            handleNavigate(item.route)
          }
        }}
        className={cn(
          'group relative flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-all duration-200 cursor-pointer',
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

  return (
    <>
      {/* Mobile overlay */}
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
        {/* Logo */}
        <div className="flex h-14 items-center justify-between px-4">
          <button
            onClick={() => handleNewConversation()}
            className={cn(
              'cursor-pointer',
              showFullText ? '' : 'flex w-full justify-center'
            )}
          >
            <Logo size="sm" iconOnly={!showFullText} />
          </button>
          <button
            onClick={() => setSidebarOpen(false)}
            className="flex h-8 w-8 items-center justify-center rounded-xl text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary lg:hidden"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Navigation */}
        <nav className="px-3 pt-2">
          <div className="space-y-0.5">
            {navItems.map((item) => renderNavItem(item))}
          </div>
        </nav>

        {/* Recent conversations */}
        {showFullText && recentSessions.length > 0 && (
          <div className="mt-4 flex-1 overflow-y-auto px-3">
            <div className="border-t border-border pt-4">
              <div className="mb-2 flex items-center justify-between px-3">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">
                  {t('sidebar.recent')}
                </span>
                <button
                  onClick={handleNewConversation}
                  className="rounded-md p-1 text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary cursor-pointer"
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
                        'flex w-full items-center gap-2.5 rounded-xl px-3 py-2 text-left transition-all duration-200 cursor-pointer',
                        sessionActive
                          ? 'bg-brand-subtle text-text-primary'
                          : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary',
                      )}
                    >
                      <MessageSquare className="h-3.5 w-3.5 shrink-0 text-text-muted" />
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-xs">
                          {session.title || session.id.replace('webui:', '')}
                        </p>
                        <p className="text-[10px] text-text-muted">
                          {timeAgo(session.last_message_at, t)}
                        </p>
                      </div>
                    </button>
                  )
                })}
              </div>
            </div>
          </div>
        )}

        {/* Collapsed: spacer */}
        {!showFullText && <div className="flex-1" />}

        {/* Empty spacer when no conversations */}
        {showFullText && recentSessions.length === 0 && <div className="flex-1" />}

        {/* Footer */}
        <div className="border-t border-border p-3">
          {/* Settings button */}
          {renderNavItem({
            nameKey: 'sidebar.settings',
            icon: Settings,
            route: '/system',
            activeRoutes: ['/system', '/channels', '/security', '/dev', '/config', '/webhooks', '/hooks', '/memory', '/database', '/budget', '/domain', '/mcp', '/access', '/groups'],
          })}

          {/* Theme + Language + Logout */}
          <div className={cn('mt-1 flex items-center gap-1', !showFullText && 'flex-col')}>
            <ThemeToggle compact={!showFullText} />
            <LanguageSwitcher compact={!showFullText} />
            <Tooltip content={t('userMenu.logout')} side="right">
              <button
                onClick={handleLogout}
                className={cn(
                  'flex items-center gap-2 rounded-xl px-2.5 py-2 text-sm text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary cursor-pointer',
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
              'mt-1 hidden w-full items-center gap-3 rounded-xl px-3 py-2 text-sm font-medium text-text-muted transition-all duration-200 hover:bg-bg-hover hover:text-text-primary lg:flex cursor-pointer',
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
          'fixed left-4 top-4 z-50 flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-bg-surface text-text-primary shadow-md transition-opacity duration-200 lg:hidden cursor-pointer',
          sidebarOpen ? 'pointer-events-none opacity-0' : 'opacity-100',
        )}
        aria-label={t('sidebar.menu')}
      >
        <Menu className="h-5 w-5" />
      </button>
    </>
  )
}

/** Theme toggle button: cycles light → dark → system */
function ThemeToggle({ compact }: { compact: boolean }) {
  const { t } = useTranslation()
  const theme = useAppStore((s) => s.theme)
  const setTheme = useAppStore((s) => s.setTheme)

  const next = () => {
    if (theme === 'light') setTheme('dark')
    else if (theme === 'dark') setTheme('system')
    else setTheme('light')
  }

  const Icon = theme === 'dark' ? Moon : theme === 'system' ? Monitor : Sun
  const label =
    theme === 'dark'
      ? t('userMenu.themeDark')
      : theme === 'system'
        ? t('userMenu.themeSystem')
        : t('userMenu.themeLight')

  const button = (
    <button
      onClick={next}
      className={cn(
        'flex items-center gap-2 rounded-xl px-2.5 py-2 text-sm text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary cursor-pointer',
        compact && 'justify-center px-2',
      )}
      aria-label={label}
    >
      <Icon className="h-4 w-4" />
    </button>
  )

  if (compact) {
    return (
      <Tooltip content={label} side="right">
        {button}
      </Tooltip>
    )
  }

  return button
}

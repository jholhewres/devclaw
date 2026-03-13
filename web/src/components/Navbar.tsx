import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Settings, LogOut, ChevronDown, Terminal } from 'lucide-react'
import { useState, useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'
import { useAppStore } from '@/stores/app'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { version } from '../../package.json'

export function Navbar() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const sidebarCollapsed = useAppStore((s) => s.sidebarCollapsed)
  const [isUserMenuOpen, setIsUserMenuOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  /* Close dropdown on outside click */
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setIsUserMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  const handleLogout = () => {
    localStorage.removeItem('devclaw_token')
    window.location.href = '/login'
  }

  return (
    <header
      className={cn(
        'fixed right-0 top-0 z-30 flex h-14 items-center border-b border-border bg-bg-surface/80 backdrop-blur-md transition-all duration-200',
        sidebarCollapsed ? 'lg:left-[72px]' : 'lg:left-64',
      )}
    >
      <div className="flex w-full items-center justify-end gap-2 px-4 sm:px-6">
        {/* Language switcher */}
        <LanguageSwitcher />

        {/* User dropdown */}
        <div className="relative" ref={dropdownRef}>
          <button
            onClick={() => setIsUserMenuOpen(!isUserMenuOpen)}
            className="flex items-center gap-2 rounded-lg px-2 py-1.5 transition-colors hover:bg-bg-hover"
          >
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-brand text-white">
              <Terminal className="h-4 w-4" />
            </div>
            <span className="hidden text-sm font-medium text-text-primary md:inline">
              DevClaw
            </span>
            <ChevronDown
              className={cn(
                'h-4 w-4 text-text-muted transition-transform duration-200',
                isUserMenuOpen && 'rotate-180',
              )}
            />
          </button>

          {/* Dropdown menu */}
          {isUserMenuOpen && (
            <div className="absolute right-0 mt-2 w-52 overflow-hidden rounded-xl border border-border bg-bg-elevated shadow-xl">
              {/* User info header */}
              <div className="border-b border-border bg-bg-surface px-4 py-3">
                <p className="text-sm font-medium text-text-primary">DevClaw</p>
                <p className="text-xs text-text-muted">v{version}</p>
              </div>

              {/* Menu items */}
              <div className="py-1">
                <button
                  onClick={() => {
                    navigate('/system')
                    setIsUserMenuOpen(false)
                  }}
                  className="flex w-full items-center gap-3 px-4 py-2.5 text-sm text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
                >
                  <Settings className="h-4 w-4" />
                  <span>{t('sidebar.system')}</span>
                </button>
              </div>

              {/* Logout */}
              <div className="border-t border-border">
                <button
                  onClick={handleLogout}
                  className="flex w-full items-center gap-3 px-4 py-2.5 text-sm text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
                >
                  <LogOut className="h-4 w-4" />
                  <span>{t('userMenu.logout')}</span>
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </header>
  )
}

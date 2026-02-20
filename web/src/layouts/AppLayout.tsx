import { Outlet } from 'react-router-dom'
import { useState, useEffect } from 'react'
import { PanelLeft, Terminal } from 'lucide-react'
import { Sidebar } from '@/components/Sidebar'
import { useAppStore } from '@/stores/app'
import { api, type SessionInfo } from '@/lib/api'

export function AppLayout() {
  const { sidebarOpen, setSidebarOpen } = useAppStore()
  const [sessions, setSessions] = useState<SessionInfo[]>([])

  useEffect(() => {
    api.sessions.list().then((s) => setSessions(s ?? [])).catch(() => {
      /* session list unavailable — non-critical */
    })
    const interval = setInterval(() => {
      api.sessions.list().then((s) => setSessions(s ?? [])).catch(() => {})
    }, 30000)
    return () => clearInterval(interval)
  }, [])

  return (
    <div className="flex h-full overflow-hidden bg-dc-darker">
      <Sidebar />

      <main className="flex flex-1 flex-col overflow-hidden">
        {!sidebarOpen && (
          <div className="flex h-11 items-center gap-2 border-b border-zinc-800 bg-dc-dark px-4">
            <button
              onClick={() => setSidebarOpen(true)}
              aria-label="Open sidebar"
              className="flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-300"
            >
              <PanelLeft className="h-3.5 w-3.5" />
            </button>
            <div className="flex items-center gap-2">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-blue-500/15">
                <Terminal className="h-4 w-4 text-blue-400" />
              </div>
              <span className="text-sm font-medium text-zinc-200">
                Dev<span className="text-blue-400">Claw</span>
              </span>
            </div>
          </div>
        )}

        <Outlet context={{ sessions, refreshSessions: () => api.sessions.list().then(setSessions).catch(() => {}) }} />
      </main>
    </div>
  )
}

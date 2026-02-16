import { Outlet } from 'react-router-dom'
import { useState, useEffect } from 'react'
import { PanelLeft } from 'lucide-react'
import { Sidebar } from '@/components/Sidebar'
import { useAppStore } from '@/stores/app'
import { api, type SessionInfo } from '@/lib/api'

export function AppLayout() {
  const { sidebarOpen, setSidebarOpen } = useAppStore()
  const [sessions, setSessions] = useState<SessionInfo[]>([])

  useEffect(() => {
    api.sessions.list().then((s) => setSessions(s ?? [])).catch(() => {})
    const interval = setInterval(() => {
      api.sessions.list().then((s) => setSessions(s ?? [])).catch(() => {})
    }, 30000)
    return () => clearInterval(interval)
  }, [])

  return (
    <div className="flex h-full overflow-hidden bg-[#0a0a0f]">
      <Sidebar sessions={sessions} />

      <main className="flex flex-1 flex-col overflow-hidden">
        {!sidebarOpen && (
          <div className="flex items-center gap-2 border-b border-white/[0.06] bg-[#111118] px-3 py-2">
            <button
              onClick={() => setSidebarOpen(true)}
              title="Abrir sidebar"
              className="flex h-7 w-7 cursor-pointer items-center justify-center rounded-lg text-gray-500 transition-colors hover:bg-white/[0.06] hover:text-gray-300"
            >
              <PanelLeft className="h-4 w-4" />
            </button>
            <div className="flex items-center gap-2">
              <div className="flex h-6 w-6 items-center justify-center rounded-md bg-gradient-to-br from-orange-500 to-amber-500">
                <svg className="h-3 w-3 text-white" viewBox="0 0 24 24" fill="currentColor">
                  <ellipse cx="7" cy="5" rx="2.5" ry="3" />
                  <ellipse cx="17" cy="5" rx="2.5" ry="3" />
                  <ellipse cx="3.5" cy="11" rx="2" ry="2.5" />
                  <ellipse cx="20.5" cy="11" rx="2" ry="2.5" />
                  <path d="M7 14c0-2.8 2.2-5 5-5s5 2.2 5 5c0 3.5-2 6-5 7-3-1-5-3.5-5-7z" />
                </svg>
              </div>
              <span className="text-sm font-bold text-white">
                Go<span className="text-orange-400">Claw</span>
              </span>
            </div>
          </div>
        )}

        <Outlet context={{ sessions, refreshSessions: () => api.sessions.list().then(setSessions) }} />
      </main>
    </div>
  )
}

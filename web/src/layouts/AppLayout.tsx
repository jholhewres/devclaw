import { Outlet } from 'react-router-dom'
import { useState, useEffect } from 'react'
import { PanelLeft } from 'lucide-react'
import { Sidebar } from '@/components/Sidebar'
import { Button } from '@/components/ui/Button'
import { useAppStore } from '@/stores/app'
import { api, type SessionInfo } from '@/lib/api'

/**
 * Layout principal da aplicação.
 * Sidebar à esquerda + conteúdo à direita.
 * A sidebar mostra sessões de chat e links de navegação.
 */
export function AppLayout() {
  const { sidebarOpen, setSidebarOpen } = useAppStore()
  const [sessions, setSessions] = useState<SessionInfo[]>([])

  /* Carregar sessões para a sidebar */
  useEffect(() => {
    api.sessions.list().then(setSessions).catch(() => {})

    // Poll a cada 30s para manter atualizado
    const interval = setInterval(() => {
      api.sessions.list().then(setSessions).catch(() => {})
    }, 30000)

    return () => clearInterval(interval)
  }, [])

  return (
    <div className="flex h-full overflow-hidden">
      {/* Sidebar */}
      <Sidebar sessions={sessions} />

      {/* Conteúdo principal */}
      <main className="flex flex-1 flex-col overflow-hidden">
        {/* Header com toggle da sidebar quando fechada */}
        {!sidebarOpen && (
          <div className="flex items-center gap-2 border-b border-zinc-200 px-3 py-2 dark:border-zinc-800">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setSidebarOpen(true)}
              title="Abrir sidebar"
            >
              <PanelLeft className="h-4 w-4" />
            </Button>
            <span className="text-sm font-medium">🐾 GoClaw</span>
          </div>
        )}

        <Outlet context={{ sessions, refreshSessions: () => api.sessions.list().then(setSessions) }} />
      </main>
    </div>
  )
}

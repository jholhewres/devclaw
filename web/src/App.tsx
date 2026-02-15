import { useEffect, useState } from 'react'
import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { AppLayout } from '@/layouts/AppLayout'
import { SetupLayout } from '@/layouts/SetupLayout'
import { Chat } from '@/pages/Chat'
import { Dashboard } from '@/pages/Dashboard'
import { Sessions } from '@/pages/Sessions'
import { Skills } from '@/pages/Skills'
import { Channels } from '@/pages/Channels'
import { Config } from '@/pages/Config'
import { Security } from '@/pages/Security'
import { Jobs } from '@/pages/Jobs'
import { Login } from '@/pages/Login'
import { SetupWizard } from '@/pages/Setup/SetupWizard'

/** Estado global de autenticação obtido de /api/auth/status */
interface AuthState {
  loading: boolean
  authRequired: boolean
  authenticated: boolean
  setupComplete: boolean
}

/**
 * Guard que verifica o estado de auth e setup, redirecionando conforme necessário:
 * - Se não configurado → /setup
 * - Se auth requerida e não autenticado → /login
 * - Caso contrário → renderiza os filhos
 */
function AuthGuard({ children }: { children: React.ReactNode }) {
  const location = useLocation()
  const [state, setState] = useState<AuthState>({
    loading: true,
    authRequired: false,
    authenticated: false,
    setupComplete: true,
  })

  useEffect(() => {
    const token = localStorage.getItem('goclaw_token')
    const headers: Record<string, string> = {}
    if (token) headers['Authorization'] = `Bearer ${token}`

    fetch('/api/auth/status', { headers })
      .then((res) => res.json())
      .then((data) => {
        setState({
          loading: false,
          authRequired: data.auth_required ?? false,
          authenticated: data.authenticated ?? false,
          setupComplete: data.setup_complete ?? true,
        })
      })
      .catch(() => {
        // Se o endpoint falhar, assume configurado e sem auth
        setState({
          loading: false,
          authRequired: false,
          authenticated: true,
          setupComplete: true,
        })
      })
  }, [location.pathname])

  if (state.loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-zinc-50 dark:bg-zinc-950">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-900 dark:border-zinc-700 dark:border-t-zinc-100" />
      </div>
    )
  }

  // Não configurado → redireciona para o wizard
  if (!state.setupComplete && location.pathname !== '/setup') {
    return <Navigate to="/setup" replace />
  }

  // Auth requerida e não autenticado → redireciona para login
  if (state.authRequired && !state.authenticated && location.pathname !== '/login') {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}

/**
 * Roteamento principal da aplicação.
 *
 * - /setup → Wizard de configuração inicial (layout centrado, sem sidebar)
 * - /login → Página de login (layout centrado, sem sidebar)
 * - / → Dashboard (layout com sidebar)
 * - /chat → Chat padrão
 * - /chat/:sessionId → Chat de sessão específica
 * - /sessions → Lista de sessões
 * - /skills → Store de skills
 * - /channels → Status dos canais
 * - /config → Editor de configuração
 * - /security → Painel de segurança
 * - /jobs → Cron jobs
 */
export function App() {
  return (
    <AuthGuard>
      <Routes>
        {/* Setup wizard — layout separado */}
        <Route element={<SetupLayout />}>
          <Route path="/setup" element={<SetupWizard />} />
        </Route>

        {/* Login — sem layout */}
        <Route path="/login" element={<Login />} />

        {/* App principal — layout com sidebar */}
        <Route element={<AppLayout />}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/chat" element={<Chat />} />
          <Route path="/chat/:sessionId" element={<Chat />} />
          <Route path="/sessions" element={<Sessions />} />
          <Route path="/skills" element={<Skills />} />
          <Route path="/channels" element={<Channels />} />
          <Route path="/config" element={<Config />} />
          <Route path="/security" element={<Security />} />
          <Route path="/jobs" element={<Jobs />} />
        </Route>
      </Routes>
    </AuthGuard>
  )
}

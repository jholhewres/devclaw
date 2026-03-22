import { useEffect, useState } from 'react'
import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { AppLayout } from '@/layouts/AppLayout'
import { SetupLayout } from '@/layouts/SetupLayout'
import { SettingsLayout } from '@/layouts/SettingsLayout'
import { Chat } from '@/pages/Chat'
import { Sessions } from '@/pages/Sessions'
import { Skills } from '@/pages/Skills'
import { Jobs } from '@/pages/Jobs'
import { Login } from '@/pages/Login'
import { SetupWizard } from '@/pages/Setup/SetupWizard'

// Settings pages
import { System } from '@/pages/System'
import { Channels } from '@/pages/Channels'
import { WhatsAppConnect } from '@/pages/WhatsAppConnect'
import { TelegramConnect } from '@/pages/TelegramConnect'
import { Security } from '@/pages/Security'
import { DevSettings } from '@/pages/DevSettings'
import { ApiConfig } from '@/pages/ApiConfig'
import { Webhooks } from '@/pages/Webhooks'
import { Hooks } from '@/pages/Hooks'
import { Memory } from '@/pages/Memory'
import { DatabasePage } from '@/pages/Database'
import { Budget } from '@/pages/Budget'
import { Domain } from '@/pages/Domain'
import { Mcp } from '@/pages/Mcp'
import { Access } from '@/pages/Access'
import { Groups } from '@/pages/Groups'

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
    const token = localStorage.getItem('devclaw_token')
    const headers: Record<string, string> = {}
    if (token) headers['Authorization'] = `Bearer ${token}`

    fetch('/api/auth/status', { headers })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        return res.json()
      })
      .then((data) => {
        setState({
          loading: false,
          authRequired: data.auth_required ?? false,
          authenticated: data.authenticated ?? false,
          setupComplete: data.setup_complete ?? true,
        })
      })
      .catch(() => {
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
      <div className="flex min-h-screen items-center justify-center bg-bg-main">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-bg-elevated border-t-brand" />
      </div>
    )
  }

  if (!state.setupComplete && location.pathname !== '/setup') {
    return <Navigate to="/setup" replace />
  }

  if (state.setupComplete && location.pathname === '/setup') {
    return <Navigate to="/" replace />
  }

  if (state.authRequired && !state.authenticated && location.pathname !== '/login') {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}

export function App() {
  return (
    <AuthGuard>
      <Routes>
        {/* Setup wizard */}
        <Route element={<SetupLayout />}>
          <Route path="/setup" element={<SetupWizard />} />
        </Route>

        {/* Login */}
        <Route path="/login" element={<Login />} />

        {/* App principal */}
        <Route element={<AppLayout />}>
          <Route path="/" element={<Chat />} />
          <Route path="/chat/:sessionId" element={<Chat />} />
          <Route path="/sessions" element={<Sessions />} />
          <Route path="/skills" element={<Skills />} />
          <Route path="/jobs" element={<Jobs />} />

          {/* Settings — route-based tabs */}
          <Route element={<SettingsLayout />}>
            {/* System tab */}
            <Route path="/system" element={<System />} />

            {/* Channels tab */}
            <Route path="/channels" element={<Channels />} />
            <Route path="/channels/whatsapp" element={<WhatsAppConnect />} />
            <Route path="/channels/telegram" element={<TelegramConnect />} />

            {/* Security tab */}
            <Route path="/security" element={<Security />} />

            {/* Dev tab */}
            <Route path="/dev" element={<DevSettings />} />
            <Route path="/config" element={<ApiConfig />} />
            <Route path="/webhooks" element={<Webhooks />} />
            <Route path="/hooks" element={<Hooks />} />
            <Route path="/memory" element={<Memory />} />
            <Route path="/database" element={<DatabasePage />} />
            <Route path="/budget" element={<Budget />} />
            <Route path="/domain" element={<Domain />} />
            <Route path="/mcp" element={<Mcp />} />
            <Route path="/access" element={<Access />} />
            <Route path="/groups" element={<Groups />} />
          </Route>
        </Route>

        {/* Fallback: redirect old /settings to /system */}
        <Route path="/settings" element={<Navigate to="/system" replace />} />
      </Routes>
    </AuthGuard>
  )
}

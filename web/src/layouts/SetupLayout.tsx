import { Outlet } from 'react-router-dom'

/**
 * Layout do wizard de setup.
 * Centrado na tela, sem sidebar — visual limpo e focado.
 */
export function SetupLayout() {
  return (
    <div className="flex min-h-full items-center justify-center bg-zinc-50 p-4 dark:bg-zinc-950">
      <div className="w-full max-w-lg">
        {/* Logo */}
        <div className="mb-8 text-center">
          <span className="text-4xl">🐾</span>
          <h1 className="mt-2 text-2xl font-semibold tracking-tight">GoClaw</h1>
          <p className="mt-1 text-sm text-zinc-500">Configure seu assistente AI</p>
        </div>

        {/* Conteúdo do wizard */}
        <div className="rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
          <Outlet />
        </div>
      </div>
    </div>
  )
}

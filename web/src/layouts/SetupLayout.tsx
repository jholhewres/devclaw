import { Outlet } from 'react-router-dom'

/**
 * Layout do setup wizard — estilo gaming dark com accent orange/amber.
 */
export function SetupLayout() {
  return (
    <div className="relative flex min-h-full items-center justify-center overflow-hidden bg-[#0a0a0f] p-4">
      {/* Background gradients */}
      <div className="pointer-events-none absolute inset-0">
        <div className="absolute -left-40 -top-40 h-[500px] w-[500px] rounded-full bg-orange-600/8 blur-[120px]" />
        <div className="absolute -bottom-40 -right-40 h-[500px] w-[500px] rounded-full bg-amber-500/8 blur-[120px]" />
      </div>

      {/* Grid pattern */}
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.02]"
        style={{
          backgroundImage: `linear-gradient(rgba(255,255,255,.15) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,.15) 1px, transparent 1px)`,
          backgroundSize: '60px 60px',
        }}
      />

      <div className="relative z-10 w-full max-w-xl">
        {/* Logo */}
        <div className="mb-8 text-center">
          <div className="inline-flex items-center justify-center">
            <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-orange-500 to-amber-500 shadow-xl shadow-orange-500/20">
              <svg className="h-7 w-7 text-white" viewBox="0 0 24 24" fill="currentColor">
                <ellipse cx="7" cy="5" rx="2.5" ry="3" />
                <ellipse cx="17" cy="5" rx="2.5" ry="3" />
                <ellipse cx="3.5" cy="11" rx="2" ry="2.5" />
                <ellipse cx="20.5" cy="11" rx="2" ry="2.5" />
                <path d="M7 14c0-2.8 2.2-5 5-5s5 2.2 5 5c0 3.5-2 6-5 7-3-1-5-3.5-5-7z" />
              </svg>
            </div>
          </div>
          <h1 className="mt-4 text-2xl font-black tracking-tight text-white">
            Go<span className="text-orange-400">Claw</span>
          </h1>
          <p className="mt-1 text-sm text-gray-500">Configure seu assistente AI</p>
        </div>

        {/* Card */}
        <div className="rounded-2xl border border-orange-500/10 bg-[#111118]/90 p-8 shadow-2xl shadow-black/40 backdrop-blur-xl">
          <Outlet />
        </div>

        <p className="mt-6 text-center text-[11px] text-gray-700">
          DevClaw &mdash; AI Assistant Framework
        </p>
      </div>
    </div>
  )
}

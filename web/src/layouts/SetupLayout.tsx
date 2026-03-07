import { Outlet } from 'react-router-dom'
import { Logo } from '@/components/Logo'

/**
 * Setup wizard layout — dark-first design with animated gradient background.
 */
export function SetupLayout() {
  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-bg-main p-6">
      {/* Background effects */}
      <div className="pointer-events-none absolute inset-0">
        {/* Radial gradient overlay */}
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-bg-subtle/20 via-transparent to-transparent" />

        {/* Animated glow orbs */}
        <div className="absolute -left-40 -top-40 h-[600px] w-[600px] animate-pulse-subtle rounded-full bg-brand/5 blur-[120px]" />
        <div className="absolute -bottom-40 -right-40 h-[600px] w-[600px] animate-pulse-subtle rounded-full bg-brand/[0.03] blur-[120px] [animation-delay:1s]" />

        {/* Subtle grid pattern */}
        <div
          className="absolute inset-0 opacity-[0.02]"
          style={{
            backgroundImage: `linear-gradient(rgba(255,255,255,0.1) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.1) 1px, transparent 1px)`,
            backgroundSize: '60px 60px',
          }}
        />
      </div>

      <div className="relative z-10 w-full max-w-lg">
        {/* Logo */}
        <div className="mb-6 text-center">
          <div className="inline-flex items-center justify-center">
            <Logo className="h-10" />
          </div>
        </div>

        {/* Card */}
        <div className="animate-fade-in-up rounded-2xl border border-border bg-bg-surface/90 p-6 shadow-lg backdrop-blur-xl">
          <Outlet />
        </div>

        <p className="mt-5 text-center text-xs text-text-muted">
          DevClaw &mdash; AI Agent for Tech Teams
        </p>
      </div>
    </div>
  )
}

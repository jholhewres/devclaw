import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Lock, ArrowRight, Loader2, Eye, EyeOff } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { Logo } from '@/components/Logo'
import { Button } from '@/components/ui/Button'
import { cn } from '@/lib/utils'
import { version } from '../../package.json'

export function Login() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [showPassword, setShowPassword] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const { token } = await api.auth.login(password)
      setPassword('')
      if (token) {
        localStorage.setItem('devclaw_token', token)
      }
      navigate('/')
    } catch (err) {
      // Extract error message from various error formats
      let errorMessage = t('login.invalidPassword')

      if (err instanceof ApiError) {
        // Try to parse JSON error response
        try {
          const parsed = JSON.parse(err.message)
          errorMessage = parsed.error || t('login.invalidPassword')
        } catch {
          errorMessage = err.status === 401 ? t('login.invalidPassword') : (err.message || t('common.error'))
        }
      } else if (err instanceof Error) {
        // Try to parse JSON from regular Error message too
        try {
          const parsed = JSON.parse(err.message)
          errorMessage = parsed.error || t('login.invalidPassword')
        } catch {
          errorMessage = err.message || t('common.error')
        }
      }

      setError(errorMessage)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-primary p-4">
      {/* Subtle radial gradient backdrop */}
      <div
        className="pointer-events-none absolute inset-0"
        style={{
          background:
            'radial-gradient(ellipse 60% 50% at 50% 40%, rgba(0,129,242,0.06) 0%, transparent 70%)',
        }}
      />

      <div className="relative z-10 w-full max-w-sm">
        {/* Logo */}
        <div className="mb-8 flex flex-col items-center gap-4">
          <Logo size="lg" />
          <p className="text-sm text-tertiary">{t('login.subtitle')}</p>
        </div>

        {/* Card */}
        <div className="rounded-2xl border border-secondary bg-primary p-6 shadow-lg">
          <form onSubmit={handleSubmit} className="space-y-5">
            <div>
              <label
                htmlFor="password"
                className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-quaternary"
              >
                <Lock className="h-3.5 w-3.5" />
                {t('login.password')}
              </label>
              <div className="relative">
                <input
                  id="password"
                  type={showPassword ? 'text' : 'password'}
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value)
                    setError('')
                  }}
                  placeholder={t('login.passwordPlaceholder')}
                  autoComplete="current-password"
                  autoFocus
                  className={cn(
                    'w-full rounded-xl border bg-secondary px-4 py-3 pr-11 text-sm text-primary',
                    'placeholder:text-quaternary outline-none transition-all',
                    'focus:border-brand-solid focus:ring-1 focus:ring-brand/30',
                    error ? 'border-error/50' : 'border-secondary'
                  )}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-tertiary transition-colors hover:text-primary"
                  tabIndex={-1}
                  aria-label={showPassword ? 'Hide password' : 'Show password'}
                >
                  {showPassword ? (
                    <EyeOff className="h-4 w-4" />
                  ) : (
                    <Eye className="h-4 w-4" />
                  )}
                </button>
              </div>
            </div>

            {error && (
              <div className="rounded-xl border border-error/20 bg-error-primary px-4 py-3 text-sm text-fg-error-secondary">
                {error}
              </div>
            )}

            <Button type="submit" size="lg" className="w-full" disabled={loading || !password}>
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <>
                  {t('login.login')}
                  <ArrowRight className="h-4 w-4" />
                </>
              )}
            </Button>
          </form>
        </div>

        {/* Footer */}
        <p className="mt-6 text-center text-xs text-tertiary">
          DevClaw v{version}
        </p>
      </div>
    </div>
  )
}

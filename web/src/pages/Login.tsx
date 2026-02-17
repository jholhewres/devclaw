import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Lock, ArrowRight, Loader2 } from 'lucide-react'
import { api } from '@/lib/api'

export function Login() {
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const { token } = await api.auth.login(password)
      if (token) {
        localStorage.setItem('devclaw_token', token)
      }
      navigate('/')
    } catch (err) {
      if (err instanceof Error) {
        setError(err.message.includes('401') ? 'Senha incorreta' : err.message)
      } else {
        setError('Erro ao fazer login')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-[#0a0a0f] p-4">
      <div className="pointer-events-none absolute inset-0">
        <div className="absolute left-1/2 top-1/3 h-[400px] w-[400px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-orange-500/8 blur-[100px]" />
      </div>

      <div className="relative z-10 w-full max-w-sm">
        <div className="mb-8 text-center">
          <div className="inline-flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-orange-500 to-amber-500 shadow-xl shadow-orange-500/20">
            <svg className="h-8 w-8 text-white" viewBox="0 0 24 24" fill="currentColor">
              <ellipse cx="7" cy="5" rx="2.5" ry="3" />
              <ellipse cx="17" cy="5" rx="2.5" ry="3" />
              <ellipse cx="3.5" cy="11" rx="2" ry="2.5" />
              <ellipse cx="20.5" cy="11" rx="2" ry="2.5" />
              <path d="M7 14c0-2.8 2.2-5 5-5s5 2.2 5 5c0 3.5-2 6-5 7-3-1-5-3.5-5-7z" />
            </svg>
          </div>
          <h1 className="mt-4 text-2xl font-black tracking-tight text-white">
            Go<span className="text-orange-400">Claw</span>
          </h1>
          <p className="mt-1 text-sm text-gray-500">Entre com sua senha para continuar</p>
        </div>

        <div className="rounded-2xl border border-orange-500/10 bg-[#111118] p-6">
          <div className="-mx-6 -mt-6 mb-6 h-1 rounded-t-2xl bg-gradient-to-r from-orange-500 via-amber-400 to-orange-500" />

          <form onSubmit={handleSubmit} className="space-y-5">
            <div>
              <label htmlFor="password" className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wider text-gray-500">
                <Lock className="h-3 w-3" />
                Senha
              </label>
              <input
                id="password"
                type="password"
                value={password}
                onChange={(e) => { setPassword(e.target.value); setError('') }}
                placeholder="Sua senha da Web UI"
                autoFocus
                className="w-full rounded-xl border border-white/[0.08] bg-[#0a0a0f] px-4 py-3 text-sm text-white placeholder-gray-600 outline-none transition-all focus:border-orange-500/40 focus:ring-2 focus:ring-orange-500/10"
              />
            </div>

            {error && (
              <div className="rounded-lg border border-red-500/20 bg-red-500/5 px-3 py-2 text-sm text-red-400">
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={loading || !password}
              className="flex w-full cursor-pointer items-center justify-center gap-2 rounded-xl bg-gradient-to-r from-orange-500 to-amber-500 px-4 py-3 text-sm font-bold text-white shadow-lg shadow-orange-500/20 transition-all hover:from-orange-600 hover:to-amber-600 hover:shadow-orange-500/30 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <>
                  Entrar
                  <ArrowRight className="h-4 w-4" />
                </>
              )}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}

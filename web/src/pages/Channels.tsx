import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AlertTriangle, Radio, QrCode, Wifi, WifiOff } from 'lucide-react'
import { api, type ChannelHealth } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

export function Channels() {
  const navigate = useNavigate()
  const [channels, setChannels] = useState<ChannelHealth[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.channels.list()
      .then(setChannels)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-[var(--color-dc-darker)]">
        <div className="h-10 w-10 rounded-full border-4 border-orange-500/30 border-t-orange-500 animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto bg-[var(--color-dc-darker)]">
      <div className="mx-auto max-w-5xl px-8 py-10">
        <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-gray-600">Comunicacao</p>
        <h1 className="mt-1 text-2xl font-black text-white tracking-tight">Canais</h1>
        <p className="mt-2 text-base text-gray-500">Status e configuracao dos canais</p>

        <div className="mt-8 space-y-4">
          {channels.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-white/[0.08] py-20">
              <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-white/[0.04]">
                <Radio className="h-8 w-8 text-gray-700" />
              </div>
              <p className="mt-4 text-lg font-semibold text-gray-500">Nenhum canal configurado</p>
              <p className="mt-1 text-sm text-gray-600">Configure canais no config.yaml</p>
            </div>
          ) : (
            channels.map((ch) => (
              <div
                key={ch.name}
                className={`relative overflow-hidden rounded-2xl border p-6 transition-all ${
                  ch.connected
                    ? 'border-emerald-500/25 bg-emerald-500/[0.03]'
                    : 'border-white/[0.06] bg-[var(--color-dc-dark)]'
                }`}
              >
                {/* Active badge */}
                {ch.connected && (
                  <div className="absolute right-5 top-5">
                    <span className="rounded-full bg-emerald-500 px-3 py-1 text-[10px] font-bold text-white shadow-lg shadow-emerald-500/30">online</span>
                  </div>
                )}

                <div className="flex items-start gap-5">
                  {/* Icon */}
                  <div className={`flex h-14 w-14 shrink-0 items-center justify-center rounded-xl ${
                    ch.connected
                      ? 'bg-emerald-500/15 text-emerald-400'
                      : 'bg-white/[0.05] text-gray-500'
                  }`}>
                    {ch.connected ? <Wifi className="h-7 w-7" /> : <WifiOff className="h-7 w-7" />}
                  </div>

                  {/* Content */}
                  <div className="flex-1">
                    <h3 className="text-xl font-bold capitalize text-white">{ch.name}</h3>
                    <p className="mt-1 text-sm text-gray-500">
                      {ch.last_msg_at && ch.last_msg_at !== '0001-01-01T00:00:00Z'
                        ? `Ultima mensagem: ${timeAgo(ch.last_msg_at)}`
                        : 'Sem mensagens recentes'}
                    </p>

                    {/* Tags row */}
                    <div className="mt-4 flex flex-wrap items-center gap-2">
                      {ch.error_count > 0 && (
                        <span className="flex items-center gap-1.5 rounded-full bg-amber-500/15 px-3 py-1 text-xs font-bold text-amber-400">
                          <AlertTriangle className="h-3 w-3" />
                          {ch.error_count} erros
                        </span>
                      )}
                      {ch.name === 'whatsapp' && !ch.connected && (
                        <button
                          onClick={() => navigate('/channels/whatsapp')}
                          className="flex cursor-pointer items-center gap-2 rounded-full bg-gradient-to-r from-orange-500 to-amber-500 px-4 py-1.5 text-xs font-bold text-white shadow-lg shadow-orange-500/20 transition-all hover:shadow-orange-500/30"
                        >
                          <QrCode className="h-3.5 w-3.5" />
                          Conectar via QR Code
                        </button>
                      )}
                      <span className={`rounded-full px-3 py-1 text-xs font-bold ${
                        ch.connected
                          ? 'bg-emerald-500/10 text-emerald-400'
                          : 'bg-red-500/10 text-red-400'
                      }`}>
                        {ch.connected ? 'Conectado' : 'Offline'}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

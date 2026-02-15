import { useEffect, useState } from 'react'
import { CheckCircle2, XCircle, AlertTriangle, Radio } from 'lucide-react'
import { api, type ChannelHealth } from '@/lib/api'
import { Badge } from '@/components/ui/Badge'
import { timeAgo } from '@/lib/utils'

/**
 * Página de canais — status e gestão de cada canal de comunicação.
 */
export function Channels() {
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
      <div className="flex flex-1 items-center justify-center">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-900 dark:border-zinc-700 dark:border-t-zinc-100" />
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-4xl px-6 py-8">
        <h1 className="text-xl font-semibold">Canais</h1>
        <p className="mt-1 text-sm text-zinc-500">
          Status e configuração dos canais de comunicação
        </p>

        <div className="mt-6 space-y-3">
          {channels.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-zinc-300 py-12 dark:border-zinc-700">
              <Radio className="h-8 w-8 text-zinc-300 dark:text-zinc-600" />
              <p className="mt-3 text-sm text-zinc-500">Nenhum canal configurado</p>
              <p className="mt-1 text-xs text-zinc-400">
                Configure canais no arquivo config.yaml
              </p>
            </div>
          ) : (
            channels.map((ch) => (
              <div
                key={ch.name}
                className="flex items-center justify-between rounded-xl border border-zinc-200 px-5 py-4 dark:border-zinc-800"
              >
                <div className="flex items-center gap-3">
                  <div className={`flex h-10 w-10 items-center justify-center rounded-lg ${
                    ch.connected
                      ? 'bg-emerald-50 dark:bg-emerald-900/20'
                      : 'bg-zinc-100 dark:bg-zinc-800'
                  }`}>
                    {ch.connected ? (
                      <CheckCircle2 className="h-5 w-5 text-emerald-500" />
                    ) : (
                      <XCircle className="h-5 w-5 text-zinc-400" />
                    )}
                  </div>
                  <div>
                    <h3 className="text-sm font-medium capitalize">{ch.name}</h3>
                    <p className="text-xs text-zinc-500">
                      {ch.last_msg_at && ch.last_msg_at !== '0001-01-01T00:00:00Z'
                        ? `Última mensagem: ${timeAgo(ch.last_msg_at)}`
                        : 'Sem mensagens'}
                    </p>
                  </div>
                </div>

                <div className="flex items-center gap-3">
                  {ch.error_count > 0 && (
                    <div className="flex items-center gap-1 text-amber-500">
                      <AlertTriangle className="h-3.5 w-3.5" />
                      <span className="text-xs">{ch.error_count} erros</span>
                    </div>
                  )}
                  <Badge variant={ch.connected ? 'success' : 'error'}>
                    {ch.connected ? 'Conectado' : 'Offline'}
                  </Badge>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

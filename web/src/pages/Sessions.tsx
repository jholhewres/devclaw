import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search, MessageSquare, Trash2 } from 'lucide-react'
import { api, type SessionInfo } from '@/lib/api'
import { Input } from '@/components/ui/Input'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { timeAgo } from '@/lib/utils'

/**
 * Lista de sessões com busca, filtro por canal, e ações.
 */
export function Sessions() {
  const navigate = useNavigate()
  const [sessions, setSessions] = useState<SessionInfo[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.sessions.list()
      .then(setSessions)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const filtered = sessions.filter(
    (s) =>
      s.id.toLowerCase().includes(search.toLowerCase()) ||
      s.channel.toLowerCase().includes(search.toLowerCase()) ||
      (s.chat_id && s.chat_id.toLowerCase().includes(search.toLowerCase())),
  )

  const handleDelete = async (id: string) => {
    try {
      await api.sessions.delete(id)
      setSessions((prev) => prev.filter((s) => s.id !== id))
    } catch { /* ignore */ }
  }

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
        <h1 className="text-xl font-semibold">Sessões</h1>
        <p className="mt-1 text-sm text-zinc-500">
          Todas as conversas do assistente
        </p>

        {/* Busca */}
        <div className="relative mt-6">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-400" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Buscar sessões..."
            className="pl-9"
          />
        </div>

        {/* Lista */}
        <div className="mt-4 space-y-2">
          {filtered.map((session) => (
            <div
              key={session.id}
              className="flex items-center justify-between rounded-xl border border-zinc-200 px-4 py-3 hover:border-zinc-300 dark:border-zinc-800 dark:hover:border-zinc-700 transition-colors"
            >
              <button
                onClick={() => navigate(`/chat/${encodeURIComponent(session.id)}`)}
                className="flex flex-1 items-center gap-3 text-left"
              >
                <MessageSquare className="h-4 w-4 shrink-0 text-zinc-400" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">
                    {session.chat_id || session.id}
                  </p>
                  <p className="text-xs text-zinc-500">
                    {session.message_count} mensagens · {timeAgo(session.last_message_at)}
                  </p>
                </div>
                <Badge>{session.channel}</Badge>
              </button>

              <Button
                variant="ghost"
                size="icon"
                onClick={(e) => {
                  e.stopPropagation()
                  handleDelete(session.id)
                }}
                className="ml-2 text-zinc-400 hover:text-red-500"
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}

          {filtered.length === 0 && (
            <p className="py-12 text-center text-sm text-zinc-400">
              {search ? 'Nenhuma sessão encontrada' : 'Nenhuma sessão ativa'}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

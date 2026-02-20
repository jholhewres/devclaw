import { useParams } from 'react-router-dom'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import {
  GitBranch,
  Database,
  Globe,
  FileCode,
  Server,
  Wrench,
  Zap,
  Sparkles,
} from 'lucide-react'
import { ChatMessage } from '@/components/ChatMessage'
import { ChatInput } from '@/components/ChatInput'
import { useChat } from '@/hooks/useChat'

/** Extracts a user-friendly message from raw LLM/API errors. */
function friendlyError(raw: string, t: (key: string) => string): string {
  if (raw.includes('404')) return t('chatPage.errorModel')
  if (raw.includes('401') || raw.includes('authentication')) return t('chatPage.errorAuth')
  if (raw.includes('429') || raw.includes('rate_limit')) return t('chatPage.errorRateLimit')
  if (raw.includes('500') || raw.includes('server_error')) return t('chatPage.errorServer')
  if (raw.includes('timeout') || raw.includes('ETIMEDOUT')) return t('chatPage.errorTimeout')
  if (raw.includes('ECONNREFUSED')) return t('chatPage.errorConnect')
  if (raw.includes('LLM call failed')) {
    const match = raw.match(/API returned (\d+)/)
    if (match) return `${t('chatPage.errorGeneric')} (${match[1]})`
    return t('chatPage.errorGeneric')
  }
  return raw
}

export function Chat() {
  const { t } = useTranslation()
  const { sessionId } = useParams<{ sessionId: string }>()
  const resolvedId = sessionId ? decodeURIComponent(sessionId) : 'webui:default'
  const { messages, streamingContent, isStreaming, error, sendMessage, abort } = useChat(resolvedId)
  const bottomRef = useRef<HTMLDivElement>(null)

  const SUGGESTIONS = [
    { icon: GitBranch, label: t('chatPage.gitStatus'), prompt: t('chatPage.gitStatusPrompt') },
    { icon: Server, label: t('chatPage.processes'), prompt: t('chatPage.processesPrompt') },
    { icon: Database, label: t('chatPage.dbSchema'), prompt: t('chatPage.dbSchemaPrompt') },
    { icon: FileCode, label: t('chatPage.analyzeCode'), prompt: t('chatPage.analyzeCodePrompt') },
    { icon: Globe, label: t('chatPage.apiTest'), prompt: t('chatPage.apiTestPrompt') },
    { icon: Wrench, label: t('chatPage.dockerPs'), prompt: t('chatPage.dockerPsPrompt') },
  ]

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  const hasMessages = messages.length > 0 || streamingContent

  const friendlyErrorLocal = (raw: string) => friendlyError(raw, t)

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <div className="flex-1 overflow-y-auto">
        {!hasMessages ? (
          <div className="flex flex-1 flex-col items-center justify-center px-6 pb-6 pt-8">
            <div className="w-full max-w-2xl space-y-6">
              {/* Branding */}
              <div className="text-center space-y-3">
                <div className="inline-flex items-center gap-2 rounded-full bg-blue-500/10 px-3 py-1.5 text-[11px] font-medium text-blue-400 ring-1 ring-blue-500/20">
                  <Sparkles className="h-3.5 w-3.5" />
                  {t('chatPage.assistant')}
                </div>
                <h1 className="text-3xl font-bold tracking-tight text-white">
                  {t('chatPage.whatDo')}
                </h1>
                <p className="mx-auto max-w-md text-sm text-zinc-500">
                  {t('chatPage.askOrPick')}
                </p>
              </div>

              {/* Suggestions grid */}
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                {SUGGESTIONS.map((s) => (
                  <button
                    key={s.label}
                    onClick={() => sendMessage(s.prompt)}
                    className="group flex cursor-pointer items-center gap-2.5 rounded-xl bg-zinc-800/40 px-3.5 py-3 text-left ring-1 ring-zinc-700/30 transition-all hover:bg-zinc-800/60 hover:ring-blue-500/20"
                  >
                    <s.icon className="h-4 w-4 shrink-0 text-zinc-500 transition-colors group-hover:text-blue-400" />
                    <span className="text-xs font-medium text-zinc-400 transition-colors group-hover:text-zinc-200">{s.label}</span>
                  </button>
                ))}
              </div>

              {/* Quick tips */}
              <div className="flex items-center justify-center gap-4 text-[11px] text-zinc-600">
                <span className="flex items-center gap-1.5">
                  <Zap className="h-3 w-3 text-blue-500/50" />
                  {t('chatPage.nativeTools')}
                </span>
                <span className="h-3 w-px bg-zinc-700/50" />
                <span>{t('chatPage.enterToSend')}</span>
              </div>
            </div>
          </div>
        ) : (
          <div className="mx-auto max-w-3xl space-y-6 px-6 py-6">
            {messages.map((msg, i) => (
              <ChatMessage key={`${msg.role}-${msg.timestamp}-${i}`} role={msg.role} content={msg.content} toolName={msg.tool_name} toolInput={msg.tool_input} />
            ))}
            {streamingContent && (
              <ChatMessage role="assistant" content={streamingContent} isStreaming />
            )}
            {error && (
              <div className="rounded-xl border border-red-500/20 bg-red-500/5 px-5 py-4">
                <p className="text-sm font-medium text-red-400">{friendlyErrorLocal(error)}</p>
                {error !== friendlyErrorLocal(error) && (
                  <details className="mt-2">
                    <summary className="cursor-pointer text-xs text-red-400/60 hover:text-red-400/80">{t('chatPage.technicalDetails')}</summary>
                    <pre className="mt-1.5 overflow-x-auto whitespace-pre-wrap font-mono text-xs text-red-400/50">{error}</pre>
                  </details>
                )}
              </div>
            )}
            <div ref={bottomRef} />
          </div>
        )}
      </div>

      <div className="mx-auto w-full max-w-3xl px-6 pb-4 pt-2">
        <ChatInput onSend={sendMessage} onAbort={abort} isStreaming={isStreaming} />
      </div>
    </div>
  )
}

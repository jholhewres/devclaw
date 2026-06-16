import { useParams } from 'react-router-dom'
import { useEffect, useRef, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Loader2,
  Search,
  Lightbulb,
  Pencil,
  Zap,
  BarChart3,
  Route,
  type LucideIcon,
} from 'lucide-react'
import { ChatMessage } from '@/components/ChatMessage'
import { ChatInput } from '@/components/ChatInput'
import { useChat } from '@/hooks/useChat'
import { cn } from '@/lib/utils'

const WEBUI_SESSION_ID = 'webui:main'

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

interface SuggestionChip {
  key: string
  icon: LucideIcon
  promptKey: string
}

const SUGGESTIONS: SuggestionChip[] = [
  { key: 'research', icon: Search, promptKey: 'chatPage.researchPrompt' },
  { key: 'explain', icon: Lightbulb, promptKey: 'chatPage.explainPrompt' },
  { key: 'write', icon: Pencil, promptKey: 'chatPage.writePrompt' },
  { key: 'automate', icon: Zap, promptKey: 'chatPage.automatePrompt' },
  { key: 'analyze', icon: BarChart3, promptKey: 'chatPage.analyzePrompt' },
  { key: 'plan', icon: Route, promptKey: 'chatPage.planPrompt' },
]

export function Chat() {
  const { t } = useTranslation()
  const { sessionId } = useParams<{ sessionId: string }>()
  const resolvedSessionId = sessionId ? decodeURIComponent(sessionId) : WEBUI_SESSION_ID

  const {
    messages,
    streamingContent,
    isStreaming,
    error,
    isLoadingHistory,
    sendMessage: chatSend,
    abort,
  } = useChat(resolvedSessionId)

  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isStreaming) {
        abort()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [isStreaming, abort])

  const sendMessage = useCallback(
    (content: string) => {
      chatSend(content)
    },
    [chatSend],
  )

  const hasMessages = messages.length > 0 || streamingContent || isStreaming
  const friendlyErrorLocal = (raw: string) => friendlyError(raw, t)

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-1 overflow-hidden">
        <div className="flex flex-1 flex-col overflow-hidden">
          <div className="flex-1 overflow-y-auto">
            {!hasMessages && !isLoadingHistory ? (
              <div className="flex h-full flex-col items-center justify-center px-6">
                <div className="w-full max-w-2xl space-y-8">
                  <div className="space-y-3 text-center">
                    <h1 className="text-3xl font-semibold tracking-tight text-primary sm:text-4xl">
                      {t('chatPage.howCanHelp')}
                    </h1>
                    <p className="text-base text-tertiary">
                      {t('chatPage.howCanHelpDesc')}
                    </p>
                  </div>

                  <ChatInput
                    onSend={sendMessage}
                    onAbort={abort}
                    isStreaming={isStreaming}
                    placeholder={t('chatPage.placeholder')}
                    autoFocus
                  />

                  <div className="flex flex-wrap items-center justify-center gap-2">
                    {SUGGESTIONS.map(({ key, icon: Icon, promptKey }) => (
                      <button
                        key={key}
                        onClick={() => sendMessage(t(promptKey))}
                        className={cn(
                          'flex items-center gap-2.5 rounded-xl border border-secondary px-4 py-2.5',
                          'text-sm font-medium text-secondary',
                          'transition-all hover:border-brand/40 hover:bg-brand-primary/30 hover:text-brand',
                        )}
                      >
                        <Icon className="size-4" />
                        {t(`chatPage.${key}`)}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            ) : (
              <div className="py-6">
                <div className="mx-auto max-w-3xl space-y-5 px-4 sm:px-6 lg:px-8">
                  {isLoadingHistory && (
                    <div className="flex items-center justify-center py-8">
                      <Loader2 className="h-6 w-6 animate-spin text-tertiary" />
                    </div>
                  )}
                  {messages.map((msg, i) => (
                    <ChatMessage
                      key={`${msg.role}-${msg.timestamp}-${i}`}
                      role={msg.role}
                      content={msg.content}
                      toolName={msg.tool_name}
                      toolInput={msg.tool_input}
                      isError={msg.is_error}
                      media={msg.media}
                    />
                  ))}
                  {isStreaming && (
                    <ChatMessage
                      role="assistant"
                      content={streamingContent}
                      isStreaming
                    />
                  )}
                  {error && (
                    <div className="rounded-xl border border-error/20 bg-error-primary px-4 py-3">
                      <p className="text-sm font-medium text-fg-error-secondary">
                        {friendlyErrorLocal(error)}
                      </p>
                      {error !== friendlyErrorLocal(error) && (
                        <details className="mt-2">
                          <summary className="cursor-pointer text-xs text-fg-error-secondary/60 hover:text-fg-error-secondary/80">
                            {t('chatPage.technicalDetails')}
                          </summary>
                          <pre className="mt-1.5 overflow-x-auto whitespace-pre-wrap font-mono text-xs text-fg-error-secondary/50">
                            {error}
                          </pre>
                        </details>
                      )}
                    </div>
                  )}
                  <div ref={bottomRef} />
                </div>
              </div>
            )}
          </div>

          {hasMessages && (
            <div className="mx-auto w-full max-w-3xl px-4 pb-4 sm:px-6 lg:px-8">
              <ChatInput onSend={sendMessage} onAbort={abort} isStreaming={isStreaming} />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

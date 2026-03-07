import { useParams } from 'react-router-dom'
import { useEffect, useRef, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Loader2,
  GitBranch,
  Container,
  Database as DatabaseIcon,
  Code,
  Globe,
  Zap,
} from 'lucide-react'
import { ChatMessage } from '@/components/ChatMessage'
import { ChatInput } from '@/components/ChatInput'
import { useChat } from '@/hooks/useChat'
import { cn } from '@/lib/utils'

/** Generate a unique session ID */
function generateSessionId(): string {
  const timestamp = Date.now().toString(36)
  const random = Math.random().toString(36).substring(2, 8)
  return `webui:${timestamp}-${random}`
}

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

/** Suggestion chip data */
const SUGGESTION_CHIPS = [
  { key: 'gitStatus', icon: GitBranch },
  { key: 'processes', icon: Zap },
  { key: 'dbSchema', icon: DatabaseIcon },
  { key: 'analyzeCode', icon: Code },
  { key: 'apiTest', icon: Globe },
  { key: 'dockerPs', icon: Container },
] as const

export function Chat() {
  const { t } = useTranslation()
  const { sessionId } = useParams<{ sessionId: string }>()

  const urlSessionId = sessionId ? decodeURIComponent(sessionId) : null

  const [localSessionId, setLocalSessionId] = useState<string | null>(null)
  const [initialMessage, setInitialMessage] = useState<string | null>(null)

  const resolvedId = urlSessionId || localSessionId

  const {
    messages,
    streamingContent,
    isStreaming,
    error,
    isLoadingHistory,
    sendMessage: chatSend,
    abort,
  } = useChat(resolvedId)

  const bottomRef = useRef<HTMLDivElement>(null)

  const chatSendRef = useRef(chatSend)
  useEffect(() => {
    chatSendRef.current = chatSend
  }, [chatSend])

  const initialMessageSentRef = useRef(false)

  useEffect(() => {
    if (resolvedId && initialMessage && !initialMessageSentRef.current) {
      initialMessageSentRef.current = true
      const timeoutId = setTimeout(() => {
        chatSendRef.current(initialMessage)
      }, 50)
      return () => clearTimeout(timeoutId)
    }
  }, [resolvedId, initialMessage])

  useEffect(() => {
    if (urlSessionId) {
      setLocalSessionId(null)
      setInitialMessage(null)
      initialMessageSentRef.current = false
    }
  }, [urlSessionId])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  const hasMessages = messages.length > 0 || streamingContent || isStreaming
  const showChatView = hasMessages || !!resolvedId

  const friendlyErrorLocal = (raw: string) => friendlyError(raw, t)

  const sendMessage = useCallback(
    (content: string) => {
      if (resolvedId) {
        chatSend(content)
      } else {
        const newSessionId = generateSessionId()
        initialMessageSentRef.current = false
        setInitialMessage(content)
        setLocalSessionId(newSessionId)
      }
    },
    [resolvedId, chatSend],
  )

  useEffect(() => {
    if (localSessionId && hasMessages && !urlSessionId) {
      window.history.replaceState({}, '', `/chat/${encodeURIComponent(localSessionId)}`)
    }
  }, [localSessionId, hasMessages, urlSessionId])

  return (
    <div className="flex h-[calc(100vh-3.5rem)] flex-col lg:h-screen">
      <div className="flex flex-1 overflow-hidden">
        <div className="flex flex-1 flex-col overflow-hidden">
          <div className="flex-1 overflow-y-auto">
            {!showChatView ? (
              /* ---- Empty state / Hero ---- */
              <div className="flex h-full flex-col items-center justify-center px-6">
                <div className="w-full max-w-2xl space-y-8">
                  <div className="space-y-3 text-center">
                    <h1 className="text-3xl font-bold tracking-tight text-text-primary md:text-[40px] md:leading-tight">
                      {t('chatPage.howCanHelp')}
                    </h1>
                    <p className="mx-auto max-w-md text-sm text-text-muted">
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
                    {SUGGESTION_CHIPS.map(({ key, icon: Icon }) => (
                      <button
                        key={key}
                        onClick={() => sendMessage(t(`chatPage.${key}Prompt`))}
                        className={cn(
                          'flex items-center gap-2 rounded-full border border-border px-3.5 py-2',
                          'text-xs font-medium text-text-secondary',
                          'transition-all hover:border-border-hover hover:bg-bg-hover hover:text-text-primary',
                        )}
                      >
                        <Icon className="h-3.5 w-3.5 text-text-muted" />
                        {t(`chatPage.${key}`)}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            ) : (
              /* ---- Messages ---- */
              <div className="py-6">
                <div className="mx-auto max-w-3xl space-y-5 px-4 sm:px-6 lg:px-8">
                  {isLoadingHistory && (
                    <div className="flex items-center justify-center py-8">
                      <Loader2 className="h-6 w-6 animate-spin text-text-muted" />
                    </div>
                  )}
                  {messages.map((msg, i) => (
                    <ChatMessage
                      key={`${msg.role}-${msg.timestamp}-${i}`}
                      role={msg.role}
                      content={msg.content}
                      toolName={msg.tool_name}
                      toolInput={msg.tool_input}
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
                    <div className="rounded-xl border border-error/20 bg-error-subtle px-4 py-3">
                      <p className="text-sm font-medium text-error">
                        {friendlyErrorLocal(error)}
                      </p>
                      {error !== friendlyErrorLocal(error) && (
                        <details className="mt-2">
                          <summary className="cursor-pointer text-xs text-error/60 hover:text-error/80">
                            {t('chatPage.technicalDetails')}
                          </summary>
                          <pre className="mt-1.5 overflow-x-auto whitespace-pre-wrap font-mono text-xs text-error/50">
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

          {showChatView && (
            <div className="mx-auto w-full max-w-3xl px-4 pb-4 sm:px-6 lg:px-8">
              <ChatInput onSend={sendMessage} onAbort={abort} isStreaming={isStreaming} />
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

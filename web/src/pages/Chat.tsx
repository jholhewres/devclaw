import { useParams } from 'react-router-dom'
import { useEffect, useRef } from 'react'
import { MessageSquarePlus } from 'lucide-react'
import { ChatMessage } from '@/components/ChatMessage'
import { ChatInput } from '@/components/ChatInput'
import { useChat } from '@/hooks/useChat'

/**
 * Página de chat — o core da experiência.
 * Estilo Claude/ChatGPT: mensagens fluindo, streaming com cursor,
 * scroll automático, input expandível.
 */
export function Chat() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const resolvedId = sessionId ? decodeURIComponent(sessionId) : 'webui:default'
  const { messages, streamingContent, isStreaming, error, sendMessage, abort } = useChat(resolvedId)
  const bottomRef = useRef<HTMLDivElement>(null)

  /* Auto-scroll ao receber novas mensagens */
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  const hasMessages = messages.length > 0 || streamingContent

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Área de mensagens */}
      <div className="flex-1 overflow-y-auto">
        {!hasMessages ? (
          /* Estado vazio — tela de boas-vindas */
          <div className="flex h-full flex-col items-center justify-center px-4">
            <div className="mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-zinc-100 dark:bg-zinc-800">
              <MessageSquarePlus className="h-8 w-8 text-zinc-400" />
            </div>
            <h2 className="text-lg font-medium">Como posso ajudar?</h2>
            <p className="mt-1 text-sm text-zinc-500">
              Escreva uma mensagem para começar a conversa.
            </p>
          </div>
        ) : (
          /* Mensagens */
          <div className="mx-auto max-w-3xl space-y-4 px-4 py-6">
            {messages.map((msg, i) => (
              <ChatMessage
                key={i}
                role={msg.role}
                content={msg.content}
                toolName={msg.tool_name}
                toolInput={msg.tool_input}
              />
            ))}

            {/* Streaming do assistente */}
            {streamingContent && (
              <ChatMessage role="assistant" content={streamingContent} isStreaming />
            )}

            {/* Erro */}
            {error && (
              <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
                {error}
              </div>
            )}

            <div ref={bottomRef} />
          </div>
        )}
      </div>

      {/* Input */}
      <div className="mx-auto w-full max-w-3xl">
        <ChatInput
          onSend={sendMessage}
          onAbort={abort}
          isStreaming={isStreaming}
        />
      </div>
    </div>
  )
}

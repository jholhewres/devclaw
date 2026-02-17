import { useParams } from 'react-router-dom'
import { useEffect, useRef } from 'react'
import { Terminal, Sparkles } from 'lucide-react'
import { ChatMessage } from '@/components/ChatMessage'
import { ChatInput } from '@/components/ChatInput'
import { useChat } from '@/hooks/useChat'

export function Chat() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const resolvedId = sessionId ? decodeURIComponent(sessionId) : 'webui:default'
  const { messages, streamingContent, isStreaming, error, sendMessage, abort } = useChat(resolvedId)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  const hasMessages = messages.length > 0 || streamingContent

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-[var(--color-dc-darker)]">
      <div className="flex-1 overflow-y-auto">
        {!hasMessages ? (
          <div className="flex h-full flex-col items-center justify-center px-4">
            <div className="relative mb-8">
              <div className="flex h-20 w-20 items-center justify-center rounded-2xl bg-orange-500/10">
                <Terminal className="h-10 w-10 text-orange-400" />
              </div>
              <div className="absolute -right-1.5 -top-1.5 flex h-7 w-7 items-center justify-center rounded-full bg-orange-500">
                <Sparkles className="h-3.5 w-3.5 text-white" />
              </div>
            </div>
            <h2 className="text-2xl font-bold text-white">Como posso ajudar?</h2>
            <p className="mt-2 text-sm text-gray-500">Escreva uma mensagem para comecar a conversa</p>
          </div>
        ) : (
          <div className="mx-auto max-w-3xl space-y-1 px-6 py-8">
            {messages.map((msg, i) => (
              <ChatMessage key={i} role={msg.role} content={msg.content} toolName={msg.tool_name} toolInput={msg.tool_input} />
            ))}
            {streamingContent && (
              <ChatMessage role="assistant" content={streamingContent} isStreaming />
            )}
            {error && (
              <div className="rounded-xl border border-red-500/20 bg-red-500/5 px-5 py-4 text-sm text-red-400">{error}</div>
            )}
            <div ref={bottomRef} />
          </div>
        )}
      </div>

      <div className="mx-auto w-full max-w-3xl">
        <ChatInput onSend={sendMessage} onAbort={abort} isStreaming={isStreaming} />
      </div>
    </div>
  )
}

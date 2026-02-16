import { useParams } from 'react-router-dom'
import { useEffect, useRef } from 'react'
import { Sparkles } from 'lucide-react'
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
    <div className="flex flex-1 flex-col overflow-hidden bg-[#0a0a0f]">
      <div className="flex-1 overflow-y-auto">
        {!hasMessages ? (
          <div className="flex h-full flex-col items-center justify-center px-4">
            <div className="relative mb-8">
              <div className="flex h-24 w-24 items-center justify-center rounded-3xl bg-gradient-to-br from-orange-500/15 to-amber-500/15 ring-1 ring-orange-500/20">
                <svg className="h-12 w-12 text-orange-400" viewBox="0 0 24 24" fill="currentColor">
                  <ellipse cx="7" cy="5" rx="2.5" ry="3" />
                  <ellipse cx="17" cy="5" rx="2.5" ry="3" />
                  <ellipse cx="3.5" cy="11" rx="2" ry="2.5" />
                  <ellipse cx="20.5" cy="11" rx="2" ry="2.5" />
                  <path d="M7 14c0-2.8 2.2-5 5-5s5 2.2 5 5c0 3.5-2 6-5 7-3-1-5-3.5-5-7z" />
                </svg>
              </div>
              <div className="absolute -right-1.5 -top-1.5 flex h-8 w-8 items-center justify-center rounded-full bg-orange-500 shadow-lg shadow-orange-500/30">
                <Sparkles className="h-4 w-4 text-white" />
              </div>
            </div>
            <h2 className="text-2xl font-black text-white tracking-tight">Como posso ajudar?</h2>
            <p className="mt-3 text-base text-gray-500">Escreva uma mensagem para comecar a conversa</p>
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
              <div className="rounded-2xl border border-red-500/20 bg-red-500/5 px-6 py-4 text-base text-red-400">{error}</div>
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

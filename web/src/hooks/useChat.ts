import { useState, useCallback, useRef, useEffect } from 'react'
import { api, type MessageInfo } from '@/lib/api'
import { createSSEConnection, type SSEEvent } from '@/lib/sse'

interface ChatState {
  messages: MessageInfo[]
  streamingContent: string
  isStreaming: boolean
  error: string | null
}

/**
 * Hook para gerenciar o chat com SSE streaming.
 * Conecta ao endpoint de stream da sessão e acumula tokens.
 */
export function useChat(sessionId: string | null) {
  const [state, setState] = useState<ChatState>({
    messages: [],
    streamingContent: '',
    isStreaming: false,
    error: null,
  })
  const cleanupRef = useRef<(() => void) | null>(null)
  const streamContentRef = useRef('')

  /* Carregar histórico ao mudar de sessão */
  useEffect(() => {
    // FIX: Clean up any active SSE stream from the previous session
    // to prevent events from leaking across sessions.
    cleanupRef.current?.()
    cleanupRef.current = null
    streamContentRef.current = ''

    if (!sessionId) {
      setState({ messages: [], streamingContent: '', isStreaming: false, error: null })
      return
    }

    setState({ messages: [], streamingContent: '', isStreaming: false, error: null })

    api.chat.history(sessionId).then((messages) => {
      setState((s) => ({ ...s, messages, error: null }))
    }).catch(() => {
      // Sessão nova, sem histórico
    })
  }, [sessionId])

  /* Enviar mensagem */
  const sendMessage = useCallback(
    async (content: string) => {
      if (!sessionId || !content.trim()) return

      // Adicionar mensagem do usuário imediatamente
      const userMsg: MessageInfo = {
        role: 'user',
        content: content.trim(),
        timestamp: new Date().toISOString(),
      }
      setState((s) => ({
        ...s,
        messages: [...s.messages, userMsg],
        streamingContent: '',
        isStreaming: true,
        error: null,
      }))
      streamContentRef.current = ''

      try {
        // Enviar e conectar ao stream
        const { run_id } = await api.chat.send(sessionId, content.trim())

        // Conectar SSE para receber a resposta
        cleanupRef.current?.()
        cleanupRef.current = createSSEConnection({
          url: `/api/chat/${sessionId}/stream?run_id=${run_id}`,
          onEvent: (event: SSEEvent) => handleStreamEvent(event),
          maxRetries: 0, // Não reconectar streams de chat
        })
      } catch (err) {
        setState((s) => ({
          ...s,
          isStreaming: false,
          error: err instanceof Error ? err.message : 'Erro ao enviar mensagem',
        }))
      }
    },
    [sessionId],
  )

  /* Processar eventos SSE do stream */
  const handleStreamEvent = useCallback((event: SSEEvent) => {
    switch (event.type) {
      case 'delta': {
        const data = event.data as { content: string }
        streamContentRef.current += data.content
        setState((s) => ({
          ...s,
          streamingContent: streamContentRef.current,
        }))
        break
      }
      case 'tool_use': {
        const data = event.data as { tool: string; input: Record<string, unknown> }
        const toolMsg: MessageInfo = {
          role: 'tool',
          content: `Usando: ${data.tool}`,
          timestamp: new Date().toISOString(),
          tool_name: data.tool,
          tool_input: JSON.stringify(data.input, null, 2),
        }
        setState((s) => ({
          ...s,
          messages: [...s.messages, toolMsg],
        }))
        break
      }
      case 'tool_result': {
        const data = event.data as { tool: string; output: string }
        const resultMsg: MessageInfo = {
          role: 'tool',
          content: data.output,
          timestamp: new Date().toISOString(),
          tool_name: data.tool,
        }
        setState((s) => ({
          ...s,
          messages: [...s.messages, resultMsg],
        }))
        break
      }
      case 'done': {
        // Flush streaming content como mensagem final
        if (streamContentRef.current) {
          const assistantMsg: MessageInfo = {
            role: 'assistant',
            content: streamContentRef.current,
            timestamp: new Date().toISOString(),
          }
          setState((s) => ({
            ...s,
            messages: [...s.messages, assistantMsg],
            streamingContent: '',
            isStreaming: false,
          }))
        } else {
          setState((s) => ({ ...s, isStreaming: false, streamingContent: '' }))
        }
        streamContentRef.current = ''
        cleanupRef.current?.()
        cleanupRef.current = null
        break
      }
      case 'error': {
        const data = event.data as { message: string }
        setState((s) => ({
          ...s,
          isStreaming: false,
          streamingContent: '',
          error: data.message,
        }))
        cleanupRef.current?.()
        cleanupRef.current = null
        break
      }
    }
  }, [])

  /* Abortar */
  const abort = useCallback(async () => {
    if (!sessionId) return
    cleanupRef.current?.()
    cleanupRef.current = null

    try {
      await api.chat.abort(sessionId)
    } catch { /* ignore */ }

    // Flush o que tiver
    if (streamContentRef.current) {
      const msg: MessageInfo = {
        role: 'assistant',
        content: streamContentRef.current + '\n\n*[Abortado]*',
        timestamp: new Date().toISOString(),
      }
      setState((s) => ({
        ...s,
        messages: [...s.messages, msg],
        streamingContent: '',
        isStreaming: false,
      }))
    } else {
      setState((s) => ({ ...s, isStreaming: false, streamingContent: '' }))
    }
    streamContentRef.current = ''
  }, [sessionId])

  /* Cleanup ao desmontar */
  useEffect(() => {
    return () => {
      cleanupRef.current?.()
    }
  }, [])

  return {
    messages: state.messages,
    streamingContent: state.streamingContent,
    isStreaming: state.isStreaming,
    error: state.error,
    sendMessage,
    abort,
  }
}

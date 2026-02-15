/**
 * SSE (Server-Sent Events) client with typed events, auto-reconnection,
 * and token-based auth.
 */

export interface SSEOptions {
  /** URL to connect to */
  url: string
  /** Event handlers by event type */
  onEvent: (event: SSEEvent) => void
  /** Called when connection is established */
  onOpen?: () => void
  /** Called when connection drops (before reconnect) */
  onError?: (error: Event) => void
  /** Auto-reconnect delay in ms (default: 2000) */
  reconnectDelay?: number
  /** Max reconnect attempts (default: 10, 0 = infinite) */
  maxRetries?: number
}

export interface SSEEvent {
  type: string
  data: unknown
}

/** Chat-specific SSE event types */
export type ChatSSEEvent =
  | { type: 'delta'; data: { content: string } }
  | { type: 'tool_use'; data: { tool: string; input: Record<string, unknown> } }
  | { type: 'tool_result'; data: { tool: string; output: string; is_error: boolean } }
  | { type: 'done'; data: { usage: { input_tokens: number; output_tokens: number } } }
  | { type: 'error'; data: { message: string } }

/**
 * Create a managed SSE connection with auto-reconnection.
 * Returns a cleanup function to close the connection.
 */
export function createSSEConnection(options: SSEOptions): () => void {
  const {
    url,
    onEvent,
    onOpen,
    onError,
    reconnectDelay = 2000,
    maxRetries = 10,
  } = options

  let eventSource: EventSource | null = null
  let retryCount = 0
  let closed = false
  let retryTimeout: ReturnType<typeof setTimeout> | null = null

  function connect() {
    if (closed) return

    // Append auth token to URL if available
    const token = localStorage.getItem('goclaw_token')
    const separator = url.includes('?') ? '&' : '?'
    const fullUrl = token ? `${url}${separator}token=${token}` : url

    eventSource = new EventSource(fullUrl)

    eventSource.onopen = () => {
      retryCount = 0
      onOpen?.()
    }

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        onEvent({ type: data.type || 'message', data: data.data || data })
      } catch {
        onEvent({ type: 'message', data: event.data })
      }
    }

    // Handle named events (delta, tool_use, tool_result, done, error)
    for (const eventType of ['delta', 'tool_use', 'tool_result', 'done', 'error', 'channel.status', 'notification']) {
      eventSource.addEventListener(eventType, (event: MessageEvent) => {
        try {
          const data = JSON.parse(event.data)
          onEvent({ type: eventType, data })
        } catch {
          onEvent({ type: eventType, data: event.data })
        }
      })
    }

    eventSource.onerror = (error) => {
      onError?.(error)
      eventSource?.close()

      if (!closed && (maxRetries === 0 || retryCount < maxRetries)) {
        retryCount++
        const delay = Math.min(reconnectDelay * Math.pow(1.5, retryCount - 1), 30000)
        retryTimeout = setTimeout(connect, delay)
      }
    }
  }

  connect()

  return () => {
    closed = true
    if (retryTimeout) clearTimeout(retryTimeout)
    eventSource?.close()
  }
}

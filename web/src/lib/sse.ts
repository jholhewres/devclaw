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
    const token = localStorage.getItem('devclaw_token')
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

/**
 * Options for POST-based SSE connections (unified send+stream).
 */
export interface POSTSSEOptions {
  url: string
  body: Record<string, unknown>
  onEvent: (event: SSEEvent) => void
  onError?: (error: Error) => void
  signal?: AbortSignal
}

/**
 * Create a POST-based SSE connection using fetch + ReadableStream.
 * Unlike EventSource (GET-only), this sends JSON in the request body
 * and reads the SSE response on the same connection — eliminating
 * the extra round-trip of the two-step send → stream flow.
 * Returns a cleanup function to abort the connection.
 */
export function createPOSTSSEConnection(options: POSTSSEOptions): () => void {
  const { url, body, onEvent, onError } = options
  const controller = new AbortController()

  const token = localStorage.getItem('devclaw_token')
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = `Bearer ${token}`

  fetch(url, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
    signal: controller.signal,
  })
    .then(async (res) => {
      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        throw new Error(text)
      }

      const reader = res.body?.getReader()
      if (!reader) throw new Error('No readable stream')

      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })

        // Parse SSE frames: "event: type\ndata: json\n\n"
        const frames = buffer.split('\n\n')
        buffer = frames.pop() || ''

        for (const frame of frames) {
          if (!frame.trim()) continue
          let eventType = 'message'
          let eventData = ''

          for (const line of frame.split('\n')) {
            if (line.startsWith('event: ')) {
              eventType = line.slice(7).trim()
            } else if (line.startsWith('data: ')) {
              eventData = line.slice(6)
            }
          }

          if (eventData) {
            try {
              onEvent({ type: eventType, data: JSON.parse(eventData) })
            } catch {
              onEvent({ type: eventType, data: eventData })
            }
          }
        }
      }
    })
    .catch((err) => {
      if (err.name === 'AbortError') return
      onError?.(err instanceof Error ? err : new Error(String(err)))
    })

  return () => controller.abort()
}

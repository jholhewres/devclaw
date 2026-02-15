import { useEffect, useRef } from 'react'
import { createSSEConnection, type SSEEvent } from '@/lib/sse'

/**
 * Hook para conectar a um endpoint SSE com auto-reconexão.
 * Retorna void — o callback recebe os eventos.
 */
export function useSSE(
  url: string | null,
  onEvent: (event: SSEEvent) => void,
  deps: unknown[] = [],
) {
  const callbackRef = useRef(onEvent)
  callbackRef.current = onEvent

  useEffect(() => {
    if (!url) return

    const cleanup = createSSEConnection({
      url,
      onEvent: (event) => callbackRef.current(event),
    })

    return cleanup
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url, ...deps])
}

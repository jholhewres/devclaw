import { useCallback, useEffect } from 'react';
import { api, type MediaAttachment } from '@/lib/api';
import { createPOSTSSEConnection, type SSEEvent } from '@/lib/sse';
import { useChatStore } from '@/stores/chat';

/**
 * Global map to track active stream abort functions across unmounts.
 * This allows background persistence when navigating between pages.
 */
const activeAbortControllers: Record<string, () => void> = {};

/**
 * Aborts an active stream for a specific session ID.
 * Exported to be used in external components like Sessions.tsx.
 * Uses substring search to handle ID mismatch (prefixed vs non-prefixed).
 */
export const abortActiveStream = (id: string) => {
  const actualKey = Object.keys(activeAbortControllers).find((key) => key.includes(id));
  
  if (actualKey && activeAbortControllers[actualKey]) {
    activeAbortControllers[actualKey]();
    delete activeAbortControllers[actualKey];
  }
}

/**
 * FAIL-SAFE SUBSCRIPTION: Monitors the store for session removals.
 * Redundant to handleDelete but essential as a state-level watchdog 
 * to ensure no ghost connections persist if a session is cleared 
 * from the store by any other side effect or global reset.
 */
useChatStore.subscribe((state) => {
  Object.keys(activeAbortControllers).forEach((id) => {
    if (!state.sessions[id]) {
      abortActiveStream(id);
    }
  });
});

/**
 * Strips internal LLM tags that should never be shown to the user.
 * Mirrors the server-side StripInternalTags logic.
 */
function stripInternalTags(text: string): string {
  return (
    text
      .replace(/\[\[reply_to[^\]]*\]\]/g, '')
      .replace(/<final>[\s\S]*?<\/final>/g, '')
      .replace(/<\/?final>/g, '')
      // Note: <thinking> and <reasoning> tags are intentionally preserved here
      // so that ChatMessageV2's extractThinkingContent() can extract them
      // and display them in a collapsible ThinkingBlock.
      .replace(/\bNO_REPLY\b/g, '')
      .replace(/\bHEARTBEAT_OK\b/g, '')
      .replace(/\[Tools used:[^\]]*\]\n?/g, '')
      .replace(/<\w+_proven\w+>[\s\S]*?<\/\w+_proven\w+>\n?/g, '')
  );
}

/**
 * Hook to manage chat logic with SSE streaming and global state persistence.
 * Connects to the session stream endpoint and accumulates tokens.
 */
export function useChat(sessionId: string | null) {
  const sessionState = useChatStore((s) => (sessionId ? s.sessions[sessionId] : null));
  const { setSessionState, addMessage, resetStreaming } = useChatStore();
  const messages = sessionState?.messages || [];
  const streamingContent = sessionState?.streamingContent || '';
  const isStreaming = sessionState?.isStreaming || false;
  const error = sessionState?.error || null;
  const isLoadingHistory = sessionState?.isLoadingHistory || false;

  // const cleanupRef = useRef<(() => void) | null>(null); // Removed to allow background persistence across unmounts

  /* Load history when session changes */
  useEffect(() => {
    // FIX: Clean up any active SSE stream from the previous session
    // to prevent events from leaking across sessions.    
    /* if (cleanupRef.current) {
      cleanupRef.current();
      cleanupRef.current = null;
    } // Removed: Switching sessions should not abort background streams 
    */    

    if (!sessionId) {
      // If no session, the store will naturally be empty/null, 
      // but we ensure clean state for the UI if needed.
      return;
    }

    // ANTI-LOOP GUARD: Do not fetch if already loading, if an error occurred, 
    // if history has already been attempted, OR if we are already streaming.
    const session = useChatStore.getState().sessions[sessionId];
    const hasLoadedHistory = session?.hasLoadedHistory || false;

    if (hasLoadedHistory || isLoadingHistory || error || isStreaming) {
      return;
    }

    setSessionState(sessionId, { isLoadingHistory: true });
    api.chat.history(sessionId)
      .then((msgs) => {
        const cleaned = (msgs || []).map((m) =>
          m.role === 'assistant' ? { ...m, content: stripInternalTags(m.content).trim() } : m
        );

        const currentState = useChatStore.getState().sessions[sessionId];
        
        // RACE CONDITION CHECK: Prevent overwriting state if the user started streaming 
        // while the history request was still in flight.
        if (currentState && !currentState.isStreaming) {
          setSessionState(sessionId, {
            messages: currentState.messages.length === 0 ? cleaned : currentState.messages,
            error: null,
            isLoadingHistory: false,
            hasLoadedHistory: true,
          });
        } else if (currentState) {
          setSessionState(sessionId, {
            isLoadingHistory: false,
            hasLoadedHistory: true,
          });
        }
      })
      .catch((err) => {
        // New session, no history - stop loading and mark as attempted to prevent loop
        setSessionState(sessionId, {
          isLoadingHistory: false,
          error: err.message,
          hasLoadedHistory: true,
        });
      });
  }, [
    sessionId,
    isLoadingHistory,
    isStreaming,
    error,
    setSessionState,
  ]);

  /* Process SSE stream events */
  const handleStreamEvent = useCallback((event: SSEEvent, targetId: string) => {
    // Immediate safety check: if session is gone, kill the stream
    if (!useChatStore.getState().sessions[targetId]) {
      abortActiveStream(targetId);
      return;
    }

    // Read current buffer directly from store to ensure we have the latest data for the specific session
    const currentBuffer = useChatStore.getState().sessions[targetId]?.streamBuffer || '';

    switch (event.type) {
      case 'run_start': {
        // Unified endpoint sends run_id on start — no action needed,
        // but we could store it for advanced abort flows if desired.
        break;
      }
      case 'delta': {
        const data = event.data as { content: string };
        const newBuffer = currentBuffer + data.content;
        setSessionState(targetId, {
          streamBuffer: newBuffer,
          streamingContent: stripInternalTags(newBuffer),
        });
        break;
      }
      case 'tool_use': {
        const data = event.data as { tool: string; input: Record<string, unknown> };
        addMessage(targetId, {
          role: 'tool',
          content: data.tool,
          timestamp: new Date().toISOString(),
          tool_name: data.tool,
          tool_input: JSON.stringify(data.input, null, 2),
        });
        break;
      }
      case 'tool_result': {
        const data = event.data as { tool: string; output: string; is_error?: boolean };
        addMessage(targetId, {
          role: 'tool',
          content: data.output,
          timestamp: new Date().toISOString(),
          tool_name: data.tool,
          is_error: data.is_error,
        });
        break;
      }
      case 'media': {
        const data = event.data as MediaAttachment;
        addMessage(targetId, {
          role: 'assistant',
          content: data.caption || '',
          timestamp: new Date().toISOString(),
          media: data,
        });
        break;
      }
      case 'done': {
        // Flush streaming content as final message, stripping internal tags
        if (currentBuffer) {
          addMessage(targetId, {
            role: 'assistant',
            content: stripInternalTags(currentBuffer).trim(),
            timestamp: new Date().toISOString(),
          });
        }
        resetStreaming(targetId);
        delete activeAbortControllers[targetId];
        break;
      }
      case 'error': {
        const data = event.data as { message: string };
        setSessionState(targetId, {
          isStreaming: false,
          streamingContent: '',
          error: data.message,
        });
        delete activeAbortControllers[targetId];
        break;
      }
    }
  }, [
    setSessionState,
    addMessage,
    resetStreaming,
  ]);

  /* Send message */
  const sendMessage = useCallback(async (content: string) => {
    if (!sessionId || !content.trim()) return;

    // Maintain closure for background processing
    const currentId = sessionId;

    addMessage(currentId, {
      role: 'user',
      content: content.trim(),
      timestamp: new Date().toISOString(),
    });

    // KILL SWITCH: Set hasLoadedHistory to true immediately to block the history useEffect.
    setSessionState(currentId, {
      streamingContent: '',
      streamBuffer: '',
      isStreaming: true,
      error: null,
      hasLoadedHistory: true, 
    });

    try {
      // cleanupRef.current?.(); // Removed: Handled via session-specific mapping in activeAbortControllers

      abortActiveStream(currentId);
      activeAbortControllers[currentId] = createPOSTSSEConnection({
        url: `/api/chat/${currentId}/stream`,
        body: { content: content.trim() },
        onEvent: (event: SSEEvent) => handleStreamEvent(event, currentId),
        onError: (err) => {
          setSessionState(currentId, {
            isStreaming: false,
            error: err.message || 'Stream error',
          });
          delete activeAbortControllers[currentId];
        },
      });
    } catch (err) {
      setSessionState(currentId, {
        isStreaming: false,
        error: err instanceof Error ? err.message : 'Failed to send message',
      });
    }
  }, [
    sessionId,
    addMessage,
    setSessionState,
    handleStreamEvent,
  ]);

  /* Abort explicitly */
  const abort = useCallback(async () => {
    if (!sessionId) return;

    // cleanupRef.current?.(); // Removed in favor of global controller mapping
    // cleanupRef.current = null;

    abortActiveStream(sessionId);

    const currentBuffer = useChatStore.getState().sessions[sessionId]?.streamBuffer || '';

    try {
      await api.chat.abort(sessionId);
    } catch {
      /* ignore */
    }

    if (currentBuffer) {
      addMessage(sessionId, {
        role: 'assistant',
        content: `${stripInternalTags(currentBuffer)}\n\n*[Aborted]*`,
        timestamp: new Date().toISOString(),
      });
    }
    resetStreaming(sessionId);
  }, [
    sessionId,
    addMessage,
    resetStreaming,
  ]);

  /* Global lifecycle and cleanup listeners */
  useEffect(() => {
    // Ensure all streams are aborted if the page is refreshed or closed
    const handleBeforeUnload = () => {
      Object.values(activeAbortControllers).forEach((abortFn) => abortFn());
    };

    window.addEventListener('beforeunload', handleBeforeUnload);

    return () => {
      // cleanupRef.current?.(); // Removed: Allow streaming to continue when navigating away from the chat page
      window.removeEventListener('beforeunload', handleBeforeUnload);
    };
  }, []);

  return {
    messages,
    streamingContent,
    isStreaming,
    error,
    isLoadingHistory,
    sendMessage,
    abort,
  };
}

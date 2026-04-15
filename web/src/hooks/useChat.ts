import { useCallback, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { api, type MessageInfo, type MediaAttachment } from '@/lib/api';
import { createPOSTSSEConnection, type SSEEvent } from '@/lib/sse';
import { useChatStore } from '@/stores/chat';

/**
 * Strips internal LLM control tags and control tokens.
 * Mirrors server-side cleanup logic.
 */
function stripInternalTags(text: string): string {
  return (
    text
      .replace(/\[\[reply_to[^\]]*\]\]/g, '')
      .replace(/<final>[\s\S]*?<\/final>/g, '')
      .replace(/<\/?final>/g, '')
      .replace(/\bNO_REPLY\b/g, '')
      .replace(/\bHEARTBEAT_OK\b/g, '')
      .replace(/\[Tools used:[^\]]*\]\n?/g, '')
      .replace(/<\/?\w+_proven\w+>[\s\S]*?<\/\w+_proven\w+>?\n?/g, '')
  );
}

/**
 * Hook to manage chat logic with SSE streaming and global state persistence.
 * Solves the "cut-off" issue by using a global streamBuffer.
 */
export function useChat(sessionId: string | null) {
  const { t } = useTranslation();
  const sessionState = useChatStore((s) => (sessionId ? s.sessions[sessionId] : null));
  const { setSessionState, addMessage, resetStreaming } = useChatStore();

  const messages = sessionState?.messages || [];
  const streamingContent = sessionState?.streamingContent || '';
  const streamBuffer = sessionState?.streamBuffer || '';
  const isStreaming = sessionState?.isStreaming || false;
  const error = sessionState?.error || null;
  const isLoadingHistory = sessionState?.isLoadingHistory || false;

  const cleanupRef = useRef<(() => void) | null>(null);

  /** Synchronize history while preventing race conditions during new message sending */
  useEffect(() => {
    if (!sessionId || messages.length > 0 || isLoadingHistory) return;

    setSessionState(sessionId, { isLoadingHistory: true });

    api.chat.history(sessionId)
      .then((msgs) => {
        const cleaned: MessageInfo[] = (msgs || []).map((m) =>
          m.role === 'assistant' ? { ...m, content: stripInternalTags(m.content).trim() } : m
        );

        // Only apply history if no new messages were added during the fetch
        const currentState = useChatStore.getState().sessions[sessionId];
        if (currentState && currentState.messages.length === 0) {
          setSessionState(sessionId, { messages: cleaned, error: null, isLoadingHistory: false });
        } else {
          setSessionState(sessionId, { isLoadingHistory: false });
        }
      })
      .catch((err) => {
        setSessionState(sessionId, { isLoadingHistory: false, error: err.message });
      });
  }, [sessionId, messages.length, isLoadingHistory, setSessionState]);

  /** Handle SSE events and update the global persistent buffer */
  const handleStreamEvent = useCallback(
    (event: SSEEvent) => {
      if (!sessionId) return;
      
      const currentBuffer = useChatStore.getState().sessions[sessionId]?.streamBuffer || '';

      switch (event.type) {
        case 'delta': {
          const data = event.data as { content: string };
          const newBuffer = currentBuffer + data.content;
          setSessionState(sessionId, {
            streamBuffer: newBuffer,
            streamingContent: stripInternalTags(newBuffer),
          });
          break;
        }
        case 'tool_use': {
          const data = event.data as { tool: string; input: Record<string, unknown> };
          const toolMsg: MessageInfo = {
            role: 'tool',
            content: data.tool,
            timestamp: new Date().toISOString(),
            tool_name: data.tool,
            tool_input: JSON.stringify(data.input, null, 2),
          };
          addMessage(sessionId, toolMsg);
          break;
        }
        case 'tool_result': {
          const data = event.data as { tool: string; output: string; is_error?: boolean };
          const resultMsg: MessageInfo = {
            role: 'tool',
            content: data.output,
            timestamp: new Date().toISOString(),
            tool_name: data.tool,
            is_error: data.is_error,
          };
          addMessage(sessionId, resultMsg);
          break;
        }
        case 'media': {
          const data = event.data as MediaAttachment;
          const mediaMsg: MessageInfo = {
            role: 'assistant',
            content: data.caption || '',
            timestamp: new Date().toISOString(),
            media: data,
          };
          addMessage(sessionId, mediaMsg);
          break;
        }
        case 'done': {
          if (currentBuffer) {
            const finalMsg: MessageInfo = {
              role: 'assistant',
              content: stripInternalTags(currentBuffer).trim(),
              timestamp: new Date().toISOString(),
            };
            addMessage(sessionId, finalMsg);
          }
          resetStreaming(sessionId);
          cleanupRef.current = null;
          break;
        }
        case 'error': {
          const data = event.data as { message: string };
          setSessionState(sessionId, { isStreaming: false, error: data.message });
          cleanupRef.current = null;
          break;
        }
      }
    },
    [sessionId, setSessionState, addMessage, resetStreaming]
  );

  const sendMessage = useCallback(
    async (content: string) => {
      if (!sessionId || !content.trim()) return;

      const userMsg: MessageInfo = {
        role: 'user',
        content: content.trim(),
        timestamp: new Date().toISOString(),
      };

      addMessage(sessionId, userMsg);
      setSessionState(sessionId, { streamingContent: '', streamBuffer: '', isStreaming: true, error: null });

      try {
        cleanupRef.current?.();
        cleanupRef.current = createPOSTSSEConnection({
          url: `/api/chat/${sessionId}/stream`,
          body: { content: content.trim() },
          onEvent: handleStreamEvent,
          onError: (err) => setSessionState(sessionId, { isStreaming: false, error: err.message }),
        });
      } catch (err) {
        setSessionState(sessionId, {
          isStreaming: false,
          error: err instanceof Error ? err.message : 'Error',
        });
      }
    },
    [sessionId, addMessage, setSessionState, handleStreamEvent]
  );

  const abort = useCallback(async () => {
    if (!sessionId) return;
    cleanupRef.current?.();
    cleanupRef.current = null;

    try {
      await api.chat.abort(sessionId);
    } catch { /* ignore */ }

    if (streamBuffer) {
      const abortMsg: MessageInfo = {
        role: 'assistant',
        content: `${stripInternalTags(streamBuffer)}\n\n*[${t('chatPage.aborted')}]*`,
        timestamp: new Date().toISOString(),
      };
      addMessage(sessionId, abortMsg);
    }
    resetStreaming(sessionId);
  }, [sessionId, t, streamBuffer, addMessage, resetStreaming]);

  return { 
    messages, 
    streamingContent, 
    isStreaming, 
    error, 
    isLoadingHistory, 
    sendMessage, 
    abort };
}
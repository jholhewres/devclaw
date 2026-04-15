import { create } from 'zustand';
import { type MessageInfo } from '@/lib/api';

/**
 * Chat session state definition.
 * Uses streamBuffer to persist background accumulation across component unmounts.
 */
interface ChatSessionState {
  messages: MessageInfo[];
  streamingContent: string;
  streamBuffer: string; 
  isStreaming: boolean;
  isLoadingHistory: boolean;
  error: string | null;
}

interface ChatStore {
  /** Map of sessionId to its corresponding state */
  sessions: Record<string, ChatSessionState>;
  
  /** Updates the state of a specific session */
  setSessionState: (id: string, state: Partial<ChatSessionState>) => void;
  
  /** Appends a new message to the session history */
  addMessage: (id: string, message: MessageInfo) => void;
  
  /** Cleans up streaming states after completion or abort */
  resetStreaming: (id: string) => void;
  
  /** Removes a session from the local cache */
  clearSession: (id: string) => void;
}

export const useChatStore = create<ChatStore>((set) => ({
  sessions: {},
  
  setSessionState: (id, newState) => set((s) => ({
    sessions: {
      ...s.sessions,
      [id]: { 
        ...(s.sessions[id] || { 
          messages: [], 
          streamingContent: '', 
          streamBuffer: '', 
          isStreaming: false, 
          isLoadingHistory: false, 
          error: null 
        }), 
        ...newState 
      }
    }
  })),

  addMessage: (id, msg) => set((s) => ({
    sessions: {
      ...s.sessions,
      [id]: {
        ...(s.sessions[id] || { 
          messages: [], 
          streamingContent: '', 
          streamBuffer: '', 
          isStreaming: false, 
          isLoadingHistory: false, 
          error: null 
        }),
        messages: [...(s.sessions[id]?.messages || []), msg]
      }
    }
  })),

  resetStreaming: (id) => set((s) => ({
    sessions: {
      ...s.sessions,
      [id]: {
        ...(s.sessions[id] || { 
          messages: [], 
          streamingContent: '', 
          streamBuffer: '', 
          isStreaming: false, 
          isLoadingHistory: false, 
          error: null 
        }),
        streamingContent: '',
        streamBuffer: '',
        isStreaming: false
      }
    }
  })),

  clearSession: (id) => set((s) => {
    const newSessions = { ...s.sessions };
    delete newSessions[id];
    return { sessions: newSessions };
  })
}));
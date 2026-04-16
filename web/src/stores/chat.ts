import { create } from 'zustand';
import { type MessageInfo } from '@/lib/api';

/**
 * Chat session state definition.
 * Uses streamBuffer to persist background accumulation across component unmounts.
 */
interface ChatSessionState {
  readonly messages: MessageInfo[];
  streamingContent: string;
  streamBuffer: string;
  isStreaming: boolean;
  isLoadingHistory: boolean;
  error: string | null;
  hasLoadedHistory?: boolean;
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

/**
 * Default state used for session initialization and resets.
 * Marked 'as const' to ensure deep immutability of the template.
 */
const defaultSessionState: ChatSessionState = {
  messages: [],
  streamingContent: '',
  streamBuffer: '',
  isStreaming: false,
  isLoadingHistory: false,
  error: null,
} as const;

export const useChatStore = create<ChatStore>((set) => ({
  sessions: {},

  setSessionState: (id, newState) => set((s) => ({
    sessions: {
      ...s.sessions,
      [id]: { ...(s.sessions[id] || defaultSessionState), ...newState },
    },
  })),

  addMessage: (id, msg) => set((s) => ({
    sessions: {
      ...s.sessions,
      [id]: {
        ...(s.sessions[id] || defaultSessionState),
        messages: [
          ...(s.sessions[id]?.messages || []),
          msg,
        ],
      },
    },
  })),

  resetStreaming: (id) => set((s) => ({
    sessions: {
      ...s.sessions,
      [id]: {
        ...(s.sessions[id] || defaultSessionState),
        streamingContent: '',
        streamBuffer: '',
        isStreaming: false,
        error: null,
      },
    },
  })),

  clearSession: (id) => set((s) => {
    /**
     * Use object destructuring to remove the specific session key.
     * This is an immutable and clean way to omit a key in the store.
     */
    const { [id]: _, ...rest } = s.sessions;
    return { sessions: rest };
  }),
}));

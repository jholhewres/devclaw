import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type Theme = 'light' | 'dark' | 'system';

interface AppState {
  /* ── UI State ── */
  sidebarOpen: boolean;
  sidebarCollapsed: boolean;
  theme: Theme;
  activeSessionId: string | null;
  sessionVersion: number;

  /* ── Actions ── */
  toggleSidebar: () => void;
  setSidebarOpen: (open: boolean) => void;
  setSidebarCollapsed: (collapsed: boolean) => void;
  toggleSidebarCollapsed: () => void;
  setTheme: (theme: Theme) => void;
  setActiveSession: (id: string | null) => void;
  invalidateSessions: () => void;
}

export const useAppStore = create<AppState>()(
  persist(
    (set) => ({
      sidebarOpen: true,
      sidebarCollapsed: false,
      theme: 'light',
      activeSessionId: null,
      sessionVersion: 0,

      toggleSidebar: () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),
      setSidebarOpen: (open) => set({ sidebarOpen: open }),
      setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),
      toggleSidebarCollapsed: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setTheme: (theme) => {
        set({ theme });
        applyTheme(theme);
      },
      setActiveSession: (id) => set({ activeSessionId: id }),
      invalidateSessions: () => set((s) => ({ sessionVersion: s.sessionVersion + 1 })),
    }),
    {
      name: 'devclaw-ui',
      partialize: ({ sidebarOpen, sidebarCollapsed, theme, activeSessionId }) => ({
        sidebarOpen,
        sidebarCollapsed,
        theme,
        activeSessionId,
      }),
    }
  )
);

/** Apply theme to document. Light-first design. */
function applyTheme(_theme: Theme) {
  const root = document.documentElement;
  root.classList.remove('light', 'dark', 'dark-mode');
  root.classList.add('light');
}

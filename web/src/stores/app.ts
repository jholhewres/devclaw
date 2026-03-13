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
      onRehydrateStorage: () => (state) => {
        if (state?.theme) applyTheme(state.theme);
      },
    }
  )
);

/** Apply theme to document. Supports light, dark, and system preference. */
function applyTheme(theme: Theme) {
  const root = document.documentElement;
  root.classList.remove('light', 'dark');

  if (theme === 'system') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    root.classList.add(prefersDark ? 'dark' : 'light');
  } else {
    root.classList.add(theme);
  }
}

/** Listen for system theme changes when in 'system' mode */
if (typeof window !== 'undefined') {
  window
    .matchMedia('(prefers-color-scheme: dark)')
    .addEventListener('change', () => {
      const currentTheme = useAppStore.getState().theme;
      if (currentTheme === 'system') {
        applyTheme('system');
      }
    });
}

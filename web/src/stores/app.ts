import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type Theme = 'light' | 'dark' | 'system'

interface AppState {
  /* ── UI State ── */
  sidebarOpen: boolean
  theme: Theme
  activeSessionId: string | null

  /* ── Actions ── */
  toggleSidebar: () => void
  setSidebarOpen: (open: boolean) => void
  setTheme: (theme: Theme) => void
  setActiveSession: (id: string | null) => void
}

export const useAppStore = create<AppState>()(
  persist(
    (set) => ({
      sidebarOpen: true,
      theme: 'system',
      activeSessionId: null,

      toggleSidebar: () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),
      setSidebarOpen: (open) => set({ sidebarOpen: open }),
      setTheme: (theme) => {
        set({ theme })
        applyTheme(theme)
      },
      setActiveSession: (id) => set({ activeSessionId: id }),
    }),
    {
      name: 'devclaw-ui',
      partialize: (state) => ({
        sidebarOpen: state.sidebarOpen,
        theme: state.theme,
        activeSessionId: state.activeSessionId,
      }),
    },
  ),
)

/** Apply theme to document */
function applyTheme(theme: Theme) {
  const root = document.documentElement
  if (theme === 'system') {
    root.classList.remove('light', 'dark')
    const isDark = window.matchMedia('(prefers-color-scheme: dark)').matches
    root.classList.add(isDark ? 'dark' : 'light')
  } else {
    root.classList.remove('light', 'dark')
    root.classList.add(theme)
  }
}

// Apply on load
if (typeof window !== 'undefined') {
  const stored = JSON.parse(localStorage.getItem('devclaw-ui') || '{}')
  applyTheme(stored?.state?.theme || 'system')

  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    const current = useAppStore.getState().theme
    if (current === 'system') applyTheme('system')
  })
}

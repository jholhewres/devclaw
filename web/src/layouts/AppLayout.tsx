import { Outlet } from 'react-router-dom'
import { Sidebar } from '@/components/Sidebar'
import { useAppStore } from '@/stores/app'

export function AppLayout() {
  const sidebarCollapsed = useAppStore((s) => s.sidebarCollapsed)

  return (
    <div className="min-h-screen bg-bg-main">
      <Sidebar />
      <div
        className={`transition-all duration-200 ${
          sidebarCollapsed ? 'lg:pl-[72px]' : 'lg:pl-64'
        }`}
      >
        <main className="min-h-screen pt-14 lg:pt-0">
          <Outlet />
        </main>
      </div>
    </div>
  )
}

import { Outlet, useLocation } from 'react-router-dom';
import { Sidebar } from '@/components/Sidebar';
import { cx } from '@/utils/cx';

/** Chat routes need locked scroll (container-level overflow) */
function isChatRoute(pathname: string) {
  return pathname === '/' || pathname.startsWith('/chat/');
}

export function AppLayout() {
  const location = useLocation();
  const chat = isChatRoute(location.pathname);

  return (
    <div className="bg-nav flex min-h-screen">
      <Sidebar />

      <main className="flex min-w-0 flex-1 pt-14 lg:pt-3 lg:pr-3 lg:pb-3">
        {chat ? (
          /* Chat: locked scroll within the rounded card */
          <div className="bg-primary flex min-h-0 flex-1 flex-col overflow-hidden lg:rounded-3xl">
            <Outlet />
          </div>
        ) : (
          /* Non-chat: normal document scroll */
          <div
            className={cx(
              'bg-primary flex min-w-0 flex-1 flex-col lg:rounded-3xl',
              'overflow-y-auto',
            )}
          >
            <Outlet />
          </div>
        )}
      </main>
    </div>
  );
}

import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEffect } from 'react';
import {
  MessageSquare,
  Clock,
  Puzzle,
  Blocks,
  Bot,
  Store,
  Settings,
  Sun,
  Moon,
  Monitor,
  Menu,
  X,
  LogOut,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { Button as AriaButton } from 'react-aria-components';
import { cx } from '@/utils/cx';
import { useAppStore, type Theme } from '@/stores/app';
import { Logo } from '@/components/Logo';
import { api } from '@/lib/api';

const NAV_ITEMS = [
  { route: '/', icon: MessageSquare, labelKey: 'sidebar.chat' },
  { route: '/jobs', icon: Clock, labelKey: 'sidebar.jobs' },
  { route: '/skills', icon: Puzzle, labelKey: 'sidebar.skills' },
  { route: '/agents', icon: Bot, labelKey: 'sidebar.agents' },
  { route: '/workflows', icon: Blocks, labelKey: 'sidebar.workflows' },
  { route: '/sessions', icon: Store, labelKey: 'sidebar.sessions' },
] as const;

const THEME_NEXT: Record<Theme, Theme> = {
  light: 'dark',
  dark: 'system',
  system: 'light',
};

const THEME_META: Record<Theme, { icon: LucideIcon; labelKey: string }> = {
  light: { icon: Moon, labelKey: 'userMenu.themeDark' },
  dark: { icon: Monitor, labelKey: 'userMenu.themeSystem' },
  system: { icon: Sun, labelKey: 'userMenu.themeLight' },
};

function NavItem({
  icon: Icon,
  label,
  active,
  onPress,
}: {
  icon: LucideIcon;
  label: string;
  active?: boolean;
  onPress: () => void;
}) {
  return (
    <AriaButton
      onPress={onPress}
      className={cx(
        'flex w-full cursor-pointer items-center gap-3 rounded-lg px-3 py-2.5 text-[15px] font-medium outline-hidden transition-colors',
        active
          ? 'bg-white/10 text-fg-primary'
          : 'text-fg-tertiary hover:bg-white/[0.04] hover:text-fg-secondary',
      )}
    >
      <Icon className="size-[18px] shrink-0" />
      <span className="truncate">{label}</span>
    </AriaButton>
  );
}

function SidebarContent({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const theme = useAppStore((s) => s.theme);
  const setTheme = useAppStore((s) => s.setTheme);

  const isChat = location.pathname === '/' || location.pathname.startsWith('/chat/');

  const { icon: ThemeIcon, labelKey: themeLabelKey } = THEME_META[theme];
  const themeLabel = t(themeLabelKey);

  const handleLogout = async () => {
    await api.auth.logout().catch(() => {});
    localStorage.removeItem('devclaw_token');
    navigate('/login');
  };

  return (
    <nav className="flex h-full flex-col">
      <div className="flex flex-col gap-0.5 p-3">
        {NAV_ITEMS.map((item) => {
          const Icon = item.icon;
          const active = item.route === '/'
            ? isChat
            : location.pathname.startsWith(item.route);
          return (
            <NavItem
              key={item.route}
              icon={Icon}
              label={t(item.labelKey)}
              active={active}
              onPress={() => {
                navigate(item.route);
                onNavigate?.();
              }}
            />
          );
        })}
      </div>

      <div className="mt-auto flex flex-col gap-0.5 border-t border-secondary p-3">
        <NavItem
          icon={Settings}
          label={t('userMenu.settings')}
          onPress={() => {
            navigate('/system');
            onNavigate?.();
          }}
        />
        <NavItem
          icon={ThemeIcon}
          label={themeLabel}
          onPress={() => setTheme(THEME_NEXT[theme])}
        />
        <NavItem
          icon={LogOut}
          label={t('userMenu.logout')}
          onPress={handleLogout}
        />
      </div>
    </nav>
  );
}

export function AppLayout() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const sidebarOpen = useAppStore((s) => s.sidebarOpen);
  const setSidebarOpen = useAppStore((s) => s.setSidebarOpen);

  useEffect(() => {
    const closeMobileSidebarOnDesktop = () => {
      if (window.innerWidth >= 1024) {
        setSidebarOpen(false);
      }
    };

    window.addEventListener('resize', closeMobileSidebarOnDesktop);
    return () => window.removeEventListener('resize', closeMobileSidebarOnDesktop);
  }, [setSidebarOpen]);

  return (
    <div className="flex h-screen flex-col bg-nav">
      <header className="flex h-11 shrink-0 items-center border-b border-secondary bg-nav px-3">
        <AriaButton
          onPress={() => setSidebarOpen(!sidebarOpen)}
          className="flex size-8 items-center justify-center rounded-lg text-fg-secondary outline-hidden hover:bg-white/[0.04] lg:hidden cursor-pointer"
          aria-label={t('sidebar.menu')}
        >
          {sidebarOpen ? <X className="size-[18px]" /> : <Menu className="size-[18px]" />}
        </AriaButton>
        <AriaButton
          onPress={() => navigate('/')}
          className="ml-3 flex shrink-0 items-center cursor-pointer outline-hidden"
        >
          <Logo size="sm" />
        </AriaButton>
      </header>

      <div className="flex flex-1 overflow-hidden">
        {sidebarOpen && (
          <div
            className="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm lg:hidden"
            onClick={() => setSidebarOpen(false)}
          />
        )}

        <aside
          className={cx(
            'fixed top-11 bottom-0 left-0 z-50 w-60 border-r border-secondary bg-nav lg:hidden',
            'transition-transform duration-200',
            sidebarOpen ? 'translate-x-0' : '-translate-x-full',
          )}
        >
          <SidebarContent onNavigate={() => setSidebarOpen(false)} />
        </aside>

        <aside className="hidden w-56 shrink-0 border-r border-secondary bg-nav lg:flex lg:flex-col">
          <SidebarContent />
        </aside>

        <main className="flex min-w-0 flex-1 flex-col overflow-y-auto bg-primary">
          <Outlet />
        </main>
      </div>

      <footer className="flex h-6 shrink-0 items-center justify-between border-t border-secondary bg-nav px-4 text-[11px] text-fg-quaternary">
        <span>DevClaw</span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block size-1.5 rounded-full bg-success" />
          {t('common.connected')}
        </span>
      </footer>
    </div>
  );
}

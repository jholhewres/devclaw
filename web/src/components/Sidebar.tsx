import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  MessageSquare,
  Clock,
  Puzzle,
  Blocks,
  Bot,
  Menu,
  X,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react';
import { Button as AriaButton } from 'react-aria-components';
import { cx } from '@/utils/cx';
import { useAppStore } from '@/stores/app';
import { Logo } from '@/components/Logo';
import { NavItem, type NavItemData } from '@/components/sidebar/NavItem';
import { ConversationList } from '@/components/sidebar/ConversationList';
import { UserCard } from '@/components/sidebar/UserCard';
import { Tooltip } from '@/components/base/tooltip/tooltip';

export function Sidebar() {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const sidebarOpen = useAppStore((s) => s.sidebarOpen);
  const setSidebarOpen = useAppStore((s) => s.setSidebarOpen);
  const sidebarCollapsed = useAppStore((s) => s.sidebarCollapsed);
  const setSidebarCollapsed = useAppStore((s) => s.setSidebarCollapsed);

  // Close mobile overlay on resize to desktop
  useEffect(() => {
    const onResize = () => {
      if (window.innerWidth >= 1024) setSidebarOpen(false);
    };
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, [setSidebarOpen]);

  const handleNewConversation = () => {
    useAppStore.getState().setActiveSession(null);
    navigate('/');
    setSidebarOpen(false);
  };

  const showFull = !sidebarCollapsed || sidebarOpen;

  const navItems: NavItemData[] = [
    {
      nameKey: 'sidebar.newConversation',
      icon: MessageSquare,
      route: '/',
      onClick: handleNewConversation,
    },
    { nameKey: 'sidebar.jobs', icon: Clock, route: '/jobs' },
    { nameKey: 'sidebar.skills', icon: Puzzle, route: '/skills' },
    { nameKey: 'sidebar.agents', icon: Bot, route: '/agents' },
    { nameKey: 'sidebar.plugins', icon: Blocks, route: '/plugins' },
  ];


  return (
    <>
      {/* Mobile overlay backdrop */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/30 backdrop-blur-sm transition-opacity duration-200 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar panel */}
      <aside
        className={cx(
          'fixed top-0 left-0 z-50 flex h-screen flex-col bg-nav transition-all duration-200 lg:sticky lg:top-0',
          sidebarOpen && 'w-[280px]',
          !sidebarOpen && (sidebarCollapsed ? 'lg:w-16' : 'lg:w-[280px]'),
          sidebarOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0',
        )}
      >
        {/* Header: Logo + mobile close */}
        <div className="flex h-14 items-center justify-between px-4">
          <AriaButton
            onPress={handleNewConversation}
            className={cx(
              'cursor-pointer outline-hidden',
              !showFull && 'flex w-full justify-center',
            )}
          >
            <Logo size="sm" iconOnly={!showFull} />
          </AriaButton>

          {/* Mobile close button */}
          <AriaButton
            onPress={() => setSidebarOpen(false)}
            className="flex size-8 items-center justify-center rounded-lg text-fg-quaternary outline-hidden transition-colors hover:bg-primary_hover hover:text-fg-tertiary lg:hidden cursor-pointer"
          >
            <X className="size-5" />
          </AriaButton>
        </div>

        {/* Main navigation */}
        <nav className="px-3 pt-1">
          <div className="space-y-0.5">
            {navItems.map((item) => (
              <NavItem key={item.nameKey} item={item} collapsed={!showFull} />
            ))}
          </div>
        </nav>

        {/* Conversations + flex spacer */}
        <ConversationList collapsed={!showFull} />

        {/* User card (theme, language, settings, logout) */}
        <UserCard collapsed={!showFull} />

        {/* Desktop collapse toggle */}
        <div className="hidden border-t border-secondary px-3 py-2 lg:block">
          <Tooltip
            title={sidebarCollapsed ? t('sidebar.expandMenu') : t('sidebar.collapseMenu')}
            placement="right"
            delay={200}
            isDisabled={!sidebarCollapsed}
          >
            <AriaButton
              onPress={() => setSidebarCollapsed(!sidebarCollapsed)}
              className={cx(
                'flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-fg-quaternary outline-hidden transition-all duration-150 hover:bg-primary_hover hover:text-fg-tertiary cursor-pointer',
                !showFull && 'justify-center',
              )}
            >
              {sidebarCollapsed ? (
                <ChevronRight className="size-[18px] shrink-0" />
              ) : (
                <>
                  <ChevronLeft className="size-[18px] shrink-0" />
                  <span>{t('sidebar.collapseMenu')}</span>
                </>
              )}
            </AriaButton>
          </Tooltip>
        </div>
      </aside>

      {/* Mobile hamburger button */}
      <AriaButton
        onPress={() => setSidebarOpen(true)}
        className={cx(
          'fixed top-3 left-3 z-50 flex size-10 items-center justify-center rounded-xl bg-primary text-fg-secondary shadow-md outline-hidden transition-opacity duration-200 lg:hidden cursor-pointer',
          sidebarOpen ? 'pointer-events-none opacity-0' : 'opacity-100',
        )}
        aria-label={t('sidebar.menu')}
      >
        <Menu className="size-5" />
      </AriaButton>
    </>
  );
}

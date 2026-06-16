import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Monitor,
  Radio,
  Shield,
  Code2,
  Webhook,
  Zap,
  Database,
  HardDrive,
  CreditCard,
  Globe,
  Box,
  Users,
  FolderGit2,
  ArrowLeft,
  Settings,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { cx } from '@/utils/cx';

interface SettingsSection {
  id: string;
  labelKey: string;
  icon: LucideIcon;
  route: string;
  routes: string[];
}

interface SettingsGroup {
  labelKey: string;
  items: SettingsSection[];
}

const SETTINGS_GROUPS: SettingsGroup[] = [
  {
    labelKey: 'sidebarSections.system',
    items: [
      { id: 'system', labelKey: 'settingsPage.tabSystem', icon: Monitor, route: '/system', routes: ['/system'] },
      { id: 'channels', labelKey: 'settingsPage.tabCanais', icon: Radio, route: '/channels', routes: ['/channels', '/channels/whatsapp', '/channels/telegram'] },
      { id: 'security', labelKey: 'settingsPage.tabSeguranca', icon: Shield, route: '/security', routes: ['/security'] },
    ],
  },
  {
    labelKey: 'sidebarSections.settings',
    items: [
      { id: 'dev', labelKey: 'settingsPage.tabDev', icon: Code2, route: '/dev', routes: ['/dev'] },
      { id: 'config', labelKey: 'sidebar.apiConfig', icon: Settings, route: '/config', routes: ['/config'] },
      { id: 'webhooks', labelKey: 'sidebar.webhooks', icon: Webhook, route: '/webhooks', routes: ['/webhooks'] },
      { id: 'hooks', labelKey: 'sidebar.hooks', icon: Zap, route: '/hooks', routes: ['/hooks'] },
      { id: 'mcp', labelKey: 'sidebar.mcp', icon: Box, route: '/mcp', routes: ['/mcp'] },
      { id: 'database', labelKey: 'sidebar.database', icon: Database, route: '/database', routes: ['/database'] },
      { id: 'memory', labelKey: 'sidebar.memory', icon: HardDrive, route: '/memory', routes: ['/memory'] },
      { id: 'budget', labelKey: 'sidebar.budget', icon: CreditCard, route: '/budget', routes: ['/budget'] },
      { id: 'domain', labelKey: 'sidebar.domainNetwork', icon: Globe, route: '/domain', routes: ['/domain'] },
      { id: 'access', labelKey: 'sidebar.access', icon: Users, route: '/access', routes: ['/access'] },
      { id: 'groups', labelKey: 'sidebar.groups', icon: FolderGit2, route: '/groups', routes: ['/groups'] },
    ],
  },
];

const SETTINGS_SECTIONS = SETTINGS_GROUPS.flatMap((group) => group.items);

function findActiveSection(pathname: string): SettingsSection | null {
  for (const group of SETTINGS_GROUPS) {
    for (const item of group.items) {
      if (item.routes.some((r) => pathname === r || pathname.startsWith(`${r}/`))) {
        return item;
      }
    }
  }
  return null;
}

export function SettingsLayout() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();

  const activeSection = findActiveSection(location.pathname);
  const activeId = activeSection?.id ?? null;

  return (
    <div className="flex h-full">
      <aside className="hidden w-52 shrink-0 border-r border-secondary bg-secondary/30 lg:flex lg:flex-col">
        <div className="flex h-10 items-center border-b border-secondary px-4">
          <button
            onClick={() => navigate('/')}
            className="flex items-center gap-2 text-xs font-medium text-secondary transition-colors hover:text-primary"
          >
            <ArrowLeft className="size-3.5" />
            {t('common.back')}
          </button>
        </div>

        <nav className="flex-1 overflow-y-auto p-3">
          {SETTINGS_GROUPS.map((group) => (
            <div key={group.labelKey} className="mb-4">
              <p className="mb-1 px-2 text-[10px] font-semibold uppercase tracking-wider text-quaternary">
                {t(group.labelKey)}
              </p>
              <div className="space-y-0.5">
                {group.items.map((item) => {
                  const Icon = item.icon;
                  const active = activeId === item.id;
                  return (
                    <button
                      key={item.id}
                      onClick={() => navigate(item.route)}
                      className={cx(
                        'flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-sm font-medium transition-colors',
                        active
                          ? 'bg-active text-fg-primary'
                          : 'text-fg-tertiary hover:bg-primary_hover hover:text-fg-secondary',
                      )}
                    >
                      <Icon className="size-4" />
                      <span>{t(item.labelKey)}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          ))}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col overflow-y-auto">
        <div className="border-b border-secondary px-4 py-3 lg:hidden">
          <select
            value={activeSection?.route ?? '/system'}
            onChange={(event) => navigate(event.target.value)}
            className="w-full rounded-lg border border-secondary bg-secondary px-3 py-2 text-sm font-medium text-primary outline-hidden"
          >
            {SETTINGS_SECTIONS.map((item) => (
              <option key={item.id} value={item.route}>
                {t(item.labelKey)}
              </option>
            ))}
          </select>
        </div>
        <div className="px-4 py-6 sm:px-6 lg:px-8">
          <Outlet />
        </div>
      </div>
    </div>
  );
}

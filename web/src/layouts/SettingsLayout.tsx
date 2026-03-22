import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft } from 'lucide-react';
import { Button as AriaButton } from 'react-aria-components';
import { Tabs } from '@/components/application/tabs/tabs';
import { NativeSelect } from '@/components/base/select/select';

const SETTINGS_TABS = [
  {
    id: 'system',
    labelKey: 'settingsPage.tabSystem',
    defaultRoute: '/system',
    routes: ['/system'],
  },
  {
    id: 'canais',
    labelKey: 'settingsPage.tabCanais',
    defaultRoute: '/channels',
    routes: ['/channels', '/channels/whatsapp', '/channels/telegram'],
  },
  {
    id: 'seguranca',
    labelKey: 'settingsPage.tabSeguranca',
    defaultRoute: '/security',
    routes: ['/security'],
  },
  {
    id: 'dev',
    labelKey: 'settingsPage.tabDev',
    defaultRoute: '/dev',
    routes: ['/dev', '/config', '/webhooks', '/hooks', '/mcp', '/database', '/memory', '/budget', '/domain', '/access', '/groups'],
  },
] as const;

function getActiveTab(pathname: string) {
  return SETTINGS_TABS.find((tab) =>
    tab.routes.some((r) => pathname === r || pathname.startsWith(`${r}/`))
  );
}

/** Routes that are sub-pages — show only Outlet with back button, no title or tabs */
const SUB_ROUTES = [
  '/config',
  '/webhooks',
  '/hooks',
  '/memory',
  '/database',
  '/budget',
  '/domain',
  '/mcp',
  '/access',
  '/groups',
  '/channels/whatsapp',
  '/channels/telegram',
];

/** Get the parent route for a sub-page */
function getParentRoute(pathname: string): string {
  if (pathname.startsWith('/channels/')) return '/channels';
  return '/dev';
}

export function SettingsLayout() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();

  const isSubPage = SUB_ROUTES.some(
    (r) => location.pathname === r || location.pathname.startsWith(`${r}/`)
  );

  const activeTab = getActiveTab(location.pathname);
  const selectedKey = activeTab?.id ?? 'system';

  const handleTabChange = (id: string | number) => {
    const tab = SETTINGS_TABS.find((t) => t.id === String(id));
    if (tab) navigate(tab.defaultRoute);
  };

  const tabItems = SETTINGS_TABS.map((tab) => ({
    id: tab.id,
    children: t(tab.labelKey),
  }));

  const selectOptions = SETTINGS_TABS.map((tab) => ({
    value: tab.id,
    label: t(tab.labelKey),
  }));

  if (isSubPage) {
    const parentRoute = getParentRoute(location.pathname);
    return (
      <div className="w-full px-4 py-6 sm:px-6 lg:px-8">
        <AriaButton
          onPress={() => navigate(parentRoute)}
          className="mb-4 inline-flex cursor-pointer items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm font-medium text-tertiary outline-hidden transition-colors hover:bg-secondary hover:text-primary"
        >
          <ArrowLeft className="size-4" />
          {t('common.back')}
        </AriaButton>
        <Outlet />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header + Tabs */}
      <div className="shrink-0 border-b border-secondary px-4 pt-6 sm:px-6 lg:px-8">
        <div className="w-full">
          <h1 className="text-display-xs font-semibold text-primary">
            {t('settingsPage.title')}
          </h1>

          {/* Desktop: Tabs */}
          <div className="mt-4 hidden lg:block">
            <Tabs selectedKey={selectedKey} onSelectionChange={handleTabChange}>
              <Tabs.List type="underline" items={tabItems}>
                {(item) => <Tabs.Item id={item.id}>{item.children}</Tabs.Item>}
              </Tabs.List>
            </Tabs>
          </div>

          {/* Mobile: Select */}
          <div className="mt-4 mb-4 lg:hidden">
            <NativeSelect
              options={selectOptions}
              value={selectedKey}
              onChange={(e) => handleTabChange(e.target.value)}
            />
          </div>
        </div>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto">
        <div className="w-full px-4 py-6 sm:px-6 lg:px-8">
          <Outlet />
        </div>
      </div>
    </div>
  );
}

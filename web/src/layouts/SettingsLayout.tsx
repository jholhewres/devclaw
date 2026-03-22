import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Tabs } from '@/components/ui/Tabs'
import { Select } from '@/components/ui/Select'

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
    routes: ['/dev', '/config', '/webhooks', '/hooks', '/mcp', '/database'],
  },
] as const

function getActiveTab(pathname: string) {
  return SETTINGS_TABS.find((tab) =>
    tab.routes.some((r) => pathname === r || pathname.startsWith(`${r}/`))
  )
}

/** Routes that are sub-pages — show only Outlet, no title or tabs */
const SUB_ROUTES = [
  '/config',
  '/webhooks',
  '/hooks',
  '/channels/whatsapp',
  '/channels/telegram',
]

export function SettingsLayout() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()

  const isSubPage = SUB_ROUTES.some(
    (r) => location.pathname === r || location.pathname.startsWith(`${r}/`)
  )

  const activeTab = getActiveTab(location.pathname)
  const selectedKey = activeTab?.id ?? 'system'

  const handleTabChange = (id: string) => {
    const tab = SETTINGS_TABS.find((t) => t.id === id)
    if (tab) navigate(tab.defaultRoute)
  }

  const tabs = SETTINGS_TABS.map((tab) => ({
    id: tab.id,
    label: t(tab.labelKey),
  }))

  const selectOptions = tabs.map((tab) => ({
    value: tab.id,
    label: tab.label,
  }))

  if (isSubPage) {
    return (
      <div className="mx-auto w-full max-w-5xl px-4 py-6 sm:px-6 lg:px-8">
        <Outlet />
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header + Tabs */}
      <div className="shrink-0 border-b border-border bg-bg-surface px-4 pt-6 sm:px-6 lg:px-8">
        <div className="mx-auto max-w-5xl">
          <h1 className="text-xl font-semibold text-text-primary">
            {t('settingsPage.title')}
          </h1>

          {/* Desktop: Tabs */}
          <div className="mt-4 hidden lg:block">
            <Tabs
              tabs={tabs}
              activeTab={selectedKey}
              onChange={handleTabChange}
              className="border-b-0"
            />
          </div>

          {/* Mobile: Select */}
          <div className="mt-4 mb-4 lg:hidden">
            <Select
              options={selectOptions}
              value={selectedKey}
              onChange={handleTabChange}
            />
          </div>
        </div>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-5xl px-4 py-6 sm:px-6 lg:px-8">
          <Outlet />
        </div>
      </div>
    </div>
  )
}

import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Settings as SettingsIcon, Cpu, Shield, Database, Zap } from 'lucide-react'
import { Tabs } from '@/components/ui/Tabs'

// Tab content — lazy imports of existing page components
import { System } from '@/pages/System'
import { Domain } from '@/pages/Domain'
import { Config } from '@/pages/Config'
import { ApiConfig } from '@/pages/ApiConfig'
import { Mcp } from '@/pages/Mcp'
import { Access } from '@/pages/Access'
import { Groups } from '@/pages/Groups'
import { Security } from '@/pages/Security'
import { Memory } from '@/pages/Memory'
import { DatabasePage } from '@/pages/Database'
import { Budget } from '@/pages/Budget'
import { Channels } from '@/pages/Channels'
import { Webhooks } from '@/pages/Webhooks'
import { Hooks } from '@/pages/Hooks'

const TAB_IDS = ['general', 'ai', 'access', 'data', 'integrations'] as const
type TabId = (typeof TAB_IDS)[number]

function isValidTab(tab: string | null): tab is TabId {
  return TAB_IDS.includes(tab as TabId)
}

export function Settings() {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()

  const tabFromUrl = searchParams.get('tab')
  const activeTab: TabId = isValidTab(tabFromUrl) ? tabFromUrl : 'general'

  const handleTabChange = (tab: string) => {
    setSearchParams({ tab })
  }

  const tabs = [
    {
      id: 'general',
      label: t('settingsPage.tabGeneral'),
      icon: <SettingsIcon className="h-4 w-4" />,
    },
    {
      id: 'ai',
      label: t('settingsPage.tabAI'),
      icon: <Cpu className="h-4 w-4" />,
    },
    {
      id: 'access',
      label: t('settingsPage.tabAccess'),
      icon: <Shield className="h-4 w-4" />,
    },
    {
      id: 'data',
      label: t('settingsPage.tabData'),
      icon: <Database className="h-4 w-4" />,
    },
    {
      id: 'integrations',
      label: t('settingsPage.tabIntegrations'),
      icon: <Zap className="h-4 w-4" />,
    },
  ]

  return (
    <div className="flex h-screen flex-col overflow-hidden">
      {/* Header + Tabs */}
      <div className="shrink-0 border-b border-border bg-bg-surface px-4 pt-6 sm:px-6 lg:px-8">
        <div className="mx-auto max-w-5xl">
          <h1 className="text-xl font-semibold text-text-primary">
            {t('settingsPage.title')}
          </h1>
          <Tabs
            tabs={tabs}
            activeTab={activeTab}
            onChange={handleTabChange}
            className="mt-4 border-b-0"
          />
        </div>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto">
        {activeTab === 'general' && <GeneralTab />}
        {activeTab === 'ai' && <AITab />}
        {activeTab === 'access' && <AccessTab />}
        {activeTab === 'data' && <DataTab />}
        {activeTab === 'integrations' && <IntegrationsTab />}
      </div>
    </div>
  )
}

/* ── Tab content wrappers ── */

function GeneralTab() {
  return (
    <div className="space-y-0">
      <System />
      <Domain />
    </div>
  )
}

function AITab() {
  return (
    <div className="space-y-0">
      <Config />
      <ApiConfig />
      <Mcp />
    </div>
  )
}

function AccessTab() {
  return (
    <div className="space-y-0">
      <Access />
      <Groups />
      <Security />
    </div>
  )
}

function DataTab() {
  return (
    <div className="space-y-0">
      <Memory />
      <DatabasePage />
      <Budget />
    </div>
  )
}

function IntegrationsTab() {
  return (
    <div className="space-y-0">
      <Channels />
      <Webhooks />
      <Hooks />
    </div>
  )
}

import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  Cpu,
  Webhook,
  Database,
  Globe,
  GitBranch,
  ArrowRight,
  AlertTriangle,
} from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Card } from '@/components/ui/Card'

interface DevCard {
  titleKey: string
  descriptionKey: string
  icon: React.ElementType
  action: { type: 'navigate'; buttonKey: string; route: string } | { type: 'badge' }
}

const DEV_CARDS: DevCard[] = [
  {
    titleKey: 'devSettingsPage.apiIa.title',
    descriptionKey: 'devSettingsPage.apiIa.description',
    icon: Cpu,
    action: { type: 'navigate', buttonKey: 'devSettingsPage.apiIa.button', route: '/config' },
  },
  {
    titleKey: 'devSettingsPage.webhooks.title',
    descriptionKey: 'devSettingsPage.webhooks.description',
    icon: Webhook,
    action: { type: 'navigate', buttonKey: 'devSettingsPage.webhooks.button', route: '/webhooks' },
  },
  {
    titleKey: 'devSettingsPage.memory.title',
    descriptionKey: 'devSettingsPage.memory.description',
    icon: Database,
    action: { type: 'navigate', buttonKey: 'devSettingsPage.memory.button', route: '/memory' },
  },
  {
    titleKey: 'devSettingsPage.domain.title',
    descriptionKey: 'devSettingsPage.domain.description',
    icon: Globe,
    action: { type: 'navigate', buttonKey: 'devSettingsPage.domain.button', route: '/domain' },
  },
  {
    titleKey: 'devSettingsPage.mcp.title',
    descriptionKey: 'devSettingsPage.mcp.description',
    icon: GitBranch,
    action: { type: 'badge' },
  },
]

export function DevSettings() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  return (
    <div className="flex flex-col gap-6">
      {/* Warning badge */}
      <div className="flex items-center gap-2 rounded-lg bg-warning-secondary px-4 py-3">
        <AlertTriangle className="h-4 w-4 shrink-0 text-fg-warning-secondary" />
        <span className="text-sm text-primary">
          <span className="font-semibold">{t('devSettingsPage.warning.badge')}</span>
          {' — '}
          {t('devSettingsPage.warning.description')}
        </span>
      </div>

      {/* Card list */}
      <Card className="divide-y divide-secondary p-0">
        {DEV_CARDS.map((card) => {
          const Icon = card.icon
          const { action } = card
          return (
            <div
              key={card.titleKey}
              className="flex flex-col gap-3 p-6 lg:flex-row lg:items-start"
            >
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-secondary">
                <Icon className="h-5 w-5 text-primary" />
              </div>

              <div className="flex min-w-0 flex-1 flex-col gap-3 lg:flex-row lg:items-start lg:justify-between lg:gap-4">
                <div className="flex min-w-0 flex-col gap-0.5">
                  <h3 className="text-sm font-semibold text-primary lg:text-base">
                    {t(card.titleKey)}
                  </h3>
                  <p className="text-sm text-tertiary">{t(card.descriptionKey)}</p>
                </div>

                {action.type === 'navigate' ? (
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => navigate(action.route)}
                    className="shrink-0 self-start"
                  >
                    {t(action.buttonKey)}
                    <ArrowRight className="h-4 w-4" />
                  </Button>
                ) : (
                  <Badge className="shrink-0 self-start">
                    {t('devSettingsPage.comingSoon')}
                  </Badge>
                )}
              </div>
            </div>
          )
        })}
      </Card>
    </div>
  )
}

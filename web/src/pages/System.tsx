import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bot, Globe } from 'lucide-react'
import { api } from '@/lib/api'
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigSelect,
  ConfigActions,
  ConfigInfoBox,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'

interface SystemConfig {
  name: string
  trigger: string
  language: string
  timezone: string
}

const LANGUAGES = [
  { value: 'pt-BR', label: 'Português (Brasil)' },
  { value: 'pt-PT', label: 'Português (Portugal)' },
  { value: 'en-US', label: 'English (US)' },
  { value: 'en-GB', label: 'English (UK)' },
  { value: 'es-ES', label: 'Español (España)' },
  { value: 'es-MX', label: 'Español (México)' },
  { value: 'fr-FR', label: 'Français' },
  { value: 'de-DE', label: 'Deutsch' },
  { value: 'it-IT', label: 'Italiano' },
  { value: 'ja-JP', label: '日本語' },
  { value: 'ko-KR', label: '한국어' },
  { value: 'zh-CN', label: '中文 (简体)' },
  { value: 'zh-TW', label: '中文 (繁體)' },
]

const TIMEZONES = [
  { value: 'America/Sao_Paulo', label: 'Brasília (GMT-3)' },
  { value: 'America/New_York', label: 'New York (GMT-5)' },
  { value: 'America/Los_Angeles', label: 'Los Angeles (GMT-8)' },
  { value: 'Europe/London', label: 'London (GMT+0)' },
  { value: 'Europe/Paris', label: 'Paris (GMT+1)' },
  { value: 'Europe/Berlin', label: 'Berlin (GMT+1)' },
  { value: 'Asia/Tokyo', label: 'Tokyo (GMT+9)' },
  { value: 'Asia/Shanghai', label: 'Shanghai (GMT+8)' },
  { value: 'Asia/Dubai', label: 'Dubai (GMT+4)' },
  { value: 'Australia/Sydney', label: 'Sydney (GMT+10)' },
  { value: 'UTC', label: 'UTC (GMT+0)' },
]

export function System() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<SystemConfig | null>(null)
  const [original, setOriginal] = useState<SystemConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  useEffect(() => {
    api.config.get()
      .then((data) => {
        const d = data as unknown as SystemConfig
        setConfig(d)
        setOriginal(JSON.parse(JSON.stringify(d)))
      })
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  const hasChanges = JSON.stringify(config) !== JSON.stringify(original)

  const handleSave = async () => {
    if (!config) return
    setSaving(true)
    setMessage(null)
    try {
      await api.config.update({
        name: config.name,
        trigger: config.trigger,
        language: config.language,
        timezone: config.timezone,
      })
      setOriginal(JSON.parse(JSON.stringify(config)))
      setMessage({ type: 'success', text: t('common.success') })
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    } finally {
      setSaving(false)
    }
  }

  const handleReset = () => {
    if (original) {
      setConfig(JSON.parse(JSON.stringify(original)))
    }
    setMessage(null)
  }

  if (loading) return <LoadingSpinner />
  if (loadError || !config) return <ErrorState message={t('common.error')} onRetry={() => window.location.reload()} retryLabel={t('common.retry')} />

  return (
    <ConfigPage
      title={t('system.title')}
      subtitle={t('system.subtitle')}
      description={t('system.desc')}
      message={message}
      actions={
        <ConfigActions
          onSave={handleSave}
          onReset={handleReset}
          saving={saving}
          hasChanges={hasChanges}
          saveLabel={t('common.save')}
          savingLabel={t('common.saving')}
          resetLabel={t('common.reset')}
        />
      }
    >
      {/* Identity Section */}
      <ConfigSection
        icon={Bot}
        title={t('system.identity')}
        description={t('system.identityDesc')}
      >
        <ConfigField label={t('system.assistantName')} hint={t('system.assistantNameHint')}>
          <ConfigInput
            value={config.name}
            onChange={(v) => setConfig((prev) => prev ? { ...prev, name: v } : prev)}
            placeholder="DevClaw"
          />
        </ConfigField>

        <ConfigField label={t('system.trigger')} hint={t('system.triggerHint')}>
          <ConfigInput
            value={config.trigger}
            onChange={(v) => setConfig((prev) => prev ? { ...prev, trigger: v } : prev)}
            placeholder="!devclaw"
          />
        </ConfigField>
      </ConfigSection>

      {/* Locale Section */}
      <ConfigSection
        icon={Globe}
        title={t('system.localization')}
        description={t('system.localizationDesc')}
      >
        <ConfigField label={t('system.primaryLanguage')} hint={t('system.primaryLanguageHint')}>
          <ConfigSelect
            value={config.language}
            onChange={(v) => setConfig((prev) => prev ? { ...prev, language: v } : prev)}
            options={LANGUAGES}
          />
        </ConfigField>

        <ConfigField label={t('system.timezone')} hint={t('system.timezoneHint')}>
          <ConfigSelect
            value={config.timezone}
            onChange={(v) => setConfig((prev) => prev ? { ...prev, timezone: v } : prev)}
            options={TIMEZONES}
          />
        </ConfigField>
      </ConfigSection>

      {/* Info */}
      <ConfigInfoBox
        title={t('system.tips')}
        items={[
          t('system.tip1'),
          t('system.tip2'),
          t('system.tip3'),
          t('system.tip4'),
        ]}
      />
    </ConfigPage>
  )
}

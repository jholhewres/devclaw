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

const LANGUAGE_KEYS = [
  { value: 'pt-BR', key: 'languages.ptBR' },
  { value: 'pt-PT', key: 'languages.ptPT' },
  { value: 'en-US', key: 'languages.enUS' },
  { value: 'en-GB', key: 'languages.enGB' },
  { value: 'es-ES', key: 'languages.esES' },
  { value: 'es-MX', key: 'languages.esMX' },
  { value: 'fr-FR', key: 'languages.frFR' },
  { value: 'de-DE', key: 'languages.deDE' },
  { value: 'it-IT', key: 'languages.itIT' },
  { value: 'ja-JP', key: 'languages.jaJP' },
  { value: 'ko-KR', key: 'languages.koKR' },
  { value: 'zh-CN', key: 'languages.zhCN' },
  { value: 'zh-TW', key: 'languages.zhTW' },
]

const TIMEZONE_KEYS = [
  { value: 'America/Sao_Paulo', key: 'timezones.brasilia' },
  { value: 'America/New_York', key: 'timezones.newYork' },
  { value: 'America/Los_Angeles', key: 'timezones.losAngeles' },
  { value: 'Europe/London', key: 'timezones.london' },
  { value: 'Europe/Paris', key: 'timezones.paris' },
  { value: 'Europe/Berlin', key: 'timezones.berlin' },
  { value: 'Asia/Tokyo', key: 'timezones.tokyo' },
  { value: 'Asia/Shanghai', key: 'timezones.shanghai' },
  { value: 'Asia/Dubai', key: 'timezones.dubai' },
  { value: 'Australia/Sydney', key: 'timezones.sydney' },
  { value: 'UTC', key: 'timezones.utc' },
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
            options={LANGUAGE_KEYS.map(l => ({ value: l.value, label: t(l.key) }))}
          />
        </ConfigField>

        <ConfigField label={t('system.timezone')} hint={t('system.timezoneHint')}>
          <ConfigSelect
            value={config.timezone}
            onChange={(v) => setConfig((prev) => prev ? { ...prev, timezone: v } : prev)}
            options={TIMEZONE_KEYS.map(tz => ({ value: tz.value, label: t(tz.key) }))}
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

import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Image, Mic, Eye, EyeOff, Cpu } from 'lucide-react'
import { api } from '@/lib/api'
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigSelect,
  ConfigToggle,
  ConfigActions,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'

interface MediaConfig {
  vision_enabled: boolean
  vision_model: string
  vision_detail: string
  transcription_enabled: boolean
  transcription_model: string
  transcription_base_url: string
  transcription_api_key: boolean | string
  transcription_language: string
}

interface ConfigData {
  name: string
  trigger: string
  model: string
  language: string
  timezone: string
  provider: string
  base_url: string
  api_key_configured: boolean
  media: MediaConfig
}

function PasswordInput({ value, onChange, placeholder }: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  const [show, setShow] = useState(false)

  return (
    <div className="relative">
      <input
        type={show ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="h-11 w-full rounded-xl border border-secondary bg-primary px-4 pr-10 text-sm text-primary outline-none transition-all placeholder:text-quaternary hover:border-primary focus:border-brand/50 focus:ring-1 focus:ring-brand/20"
      />
      <button
        type="button"
        onClick={() => setShow(!show)}
        className="absolute right-3 top-1/2 -translate-y-1/2 text-tertiary hover:text-primary cursor-pointer"
      >
        {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}

export function Config() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<ConfigData | null>(null)
  const [original, setOriginal] = useState<ConfigData | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [transcriptionApiKey, setTranscriptionApiKey] = useState('')
  const [mainApiKey, setMainApiKey] = useState('')

  useEffect(() => {
    api.config.get()
      .then((data) => {
        const d = data as unknown as ConfigData
        setConfig(d)
        setOriginal(JSON.parse(JSON.stringify(d)))
      })
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  const hasChanges = JSON.stringify(config) !== JSON.stringify(original) || transcriptionApiKey !== '' || mainApiKey !== ''

  const updateMedia = useCallback((key: keyof MediaConfig, value: unknown) => {
    setConfig((prev) => prev ? { ...prev, media: { ...prev.media, [key]: value } } : prev)
  }, [])

  const handleSave = async () => {
    if (!config) return
    setSaving(true)
    setMessage(null)
    try {
      const payload: Record<string, unknown> = {
        provider: config.provider,
        model: config.model,
        base_url: config.base_url,
        media: {
          ...config.media,
          transcription_api_key: transcriptionApiKey || undefined,
        },
      }
      if (mainApiKey) {
        payload.api_key = mainApiKey
      }
      const result = await api.config.update(payload) as unknown as ConfigData
      setConfig(result)
      setOriginal(JSON.parse(JSON.stringify(result)))
      setTranscriptionApiKey('')
      setMainApiKey('')
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
      setTranscriptionApiKey('')
      setMainApiKey('')
    }
    setMessage(null)
  }

  if (loading) return <LoadingSpinner />
  if (loadError || !config) return <ErrorState message={t('common.error')} onRetry={() => window.location.reload()} retryLabel={t('common.loading')} />

  return (
    <ConfigPage
      title={t('config.title')}
      subtitle={t('config.subtitle')}
      description={config.name}
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
      {/* Provider & Model Section */}
      <ConfigSection
        icon={Cpu}
        title={t('config.providerTitle')}
        description={t('config.providerDesc')}
      >
        <ConfigField label={t('config.provider')}>
          <ConfigInput
            value={config.provider}
            onChange={(v) => setConfig((prev) => prev ? { ...prev, provider: v } : prev)}
            placeholder="ex: openai, anthropic, google, zai, groq, ollama"
          />
        </ConfigField>

        <ConfigField label={t('config.model')}>
          <ConfigInput
            value={config.model}
            onChange={(v) => setConfig((prev) => prev ? { ...prev, model: v } : prev)}
            placeholder="ex: gpt-4o, claude-sonnet-4-20250514, gemini-2.5-pro"
          />
        </ConfigField>

        <ConfigField label={t('config.baseUrl')} hint={t('config.baseUrlHint')}>
          <ConfigInput
            value={config.base_url}
            onChange={(v) => setConfig((prev) => prev ? { ...prev, base_url: v } : prev)}
            placeholder="https://api.example.com/v1"
          />
        </ConfigField>

        <ConfigField label={t('config.apiKey')} hint={t('config.apiKeyHint')}>
          <PasswordInput
            value={mainApiKey}
            onChange={setMainApiKey}
            placeholder={config.api_key_configured ? `••••••• (${t('config.apiKeyConfigured')})` : t('config.apiKeyPlaceholder')}
          />
        </ConfigField>
      </ConfigSection>

      {/* Vision Section */}
      <ConfigSection
        icon={Image}
        title={t('config.visionTitle')}
        description={t('config.visionDesc')}
      >
        <ConfigToggle
          enabled={config.media.vision_enabled}
          onChange={(v) => updateMedia('vision_enabled', v)}
          label={t('config.visionEnable')}
        />

        {config.media.vision_enabled && (
          <>
            <ConfigField label={t('config.visionModel')} hint={t('config.visionModelHint', { model: config.model })}>
              <ConfigInput
                value={config.media.vision_model}
                onChange={(v) => updateMedia('vision_model', v)}
                placeholder={t('config.visionModelPlaceholder')}
              />
            </ConfigField>

            <ConfigField label={t('config.visionQuality')}>
              <ConfigSelect
                value={config.media.vision_detail}
                onChange={(v) => updateMedia('vision_detail', v)}
                options={[
                  { value: 'auto', label: t('config.visionQualityAuto') },
                  { value: 'low', label: t('config.visionQualityLow') },
                  { value: 'high', label: t('config.visionQualityHigh') },
                ]}
              />
            </ConfigField>
          </>
        )}
      </ConfigSection>

      {/* Transcription Section */}
      <ConfigSection
        icon={Mic}
        title={t('config.transcriptionTitle')}
        description={t('config.transcriptionDesc')}
      >
        <ConfigToggle
          enabled={config.media.transcription_enabled}
          onChange={(v) => updateMedia('transcription_enabled', v)}
          label={t('config.transcriptionEnable')}
        />

        {config.media.transcription_enabled && (
          <>
            <ConfigField label={t('config.transcriptionModel')}>
              <ConfigInput
                value={config.media.transcription_model}
                onChange={(v) => updateMedia('transcription_model', v)}
                placeholder={t('config.transcriptionModelPlaceholder')}
              />
            </ConfigField>

            <ConfigField label={t('config.transcriptionBaseUrl')} hint={t('config.transcriptionBaseUrlHint')}>
              <ConfigInput
                value={config.media.transcription_base_url}
                onChange={(v) => updateMedia('transcription_base_url', v)}
                placeholder={t('config.transcriptionBaseUrlPlaceholder')}
              />
            </ConfigField>

            <ConfigField label={t('config.transcriptionLanguage')} hint={t('config.transcriptionLanguageHint')}>
              <ConfigSelect
                value={config.media.transcription_language}
                onChange={(v) => updateMedia('transcription_language', v)}
                placeholder={t('config.autoDetect')}
                options={[
                  { value: 'pt', label: 'Português' },
                  { value: 'en', label: 'English' },
                  { value: 'es', label: 'Español' },
                  { value: 'fr', label: 'Français' },
                  { value: 'de', label: 'Deutsch' },
                  { value: 'it', label: 'Italiano' },
                  { value: 'ja', label: '日本語' },
                  { value: 'ko', label: '한국어' },
                  { value: 'zh', label: '中文' },
                ]}
              />
            </ConfigField>

            <ConfigField label={t('config.transcriptionApiKey')} hint={t('config.transcriptionApiKeyHint')}>
              <PasswordInput
                value={transcriptionApiKey}
                onChange={setTranscriptionApiKey}
                placeholder={config.media.transcription_api_key ? `••••••• (${t('config.apiKeyConfigured')})` : t('config.transcriptionApiKeyPlaceholder')}
              />
            </ConfigField>
          </>
        )}
      </ConfigSection>
    </ConfigPage>
  )
}

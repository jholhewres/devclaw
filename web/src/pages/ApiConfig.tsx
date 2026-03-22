import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Eye,
  EyeOff,
  Sparkles,
  CheckCircle2,
  ExternalLink,
  Zap,
  Loader2,
  Image,
  Mic,
  Plus,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/Button'
import {
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigSelect,
  ConfigToggle,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'
import { ModelCombobox } from '@/components/ui/ModelCombobox'
import { UnsavedChangesBar } from '@/components/ui/UnsavedChangesBar'
import {
  DEFAULT_PROVIDER_IDS,
  EXPANDED_PROVIDER_IDS,
  getProviderByValue,
  getVisibleModels,
  getDefaultBaseUrl,
  getProviderIcon,
} from '@/lib/providers'

/* ── Types ── */

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
  provider: string
  base_url: string
  api_key_configured: boolean
  api_key_masked?: string
  model: string
  models?: string[]
  params?: {
    context1m?: boolean
    tool_stream?: boolean
  }
  media: MediaConfig
}

interface ConnectionTestResult {
  success: boolean
  latency?: number
  error?: string
}

/* ── Helpers ── */

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
        className="absolute right-3 top-1/2 -translate-y-1/2 cursor-pointer text-tertiary hover:text-primary"
      >
        {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}

function Radio({ checked }: { checked: boolean }) {
  return (
    <div
      className={cn(
        'flex h-4 w-4 items-center justify-center rounded-full border-2 transition-colors',
        checked
          ? 'border-brand-solid bg-brand-solid'
          : 'border-primary',
      )}
    >
      {checked && <div className="h-1.5 w-1.5 rounded-full bg-white" />}
    </div>
  )
}

/* ── Component ── */

export function ApiConfig() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<ConfigData | null>(null)
  const [original, setOriginal] = useState<ConfigData | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [apiKey, setApiKey] = useState('')
  const [transcriptionApiKey, setTranscriptionApiKey] = useState('')
  const [testingConnection, setTestingConnection] = useState(false)
  const [testResult, setTestResult] = useState<ConnectionTestResult | null>(null)
  const [selectedEndpoint, setSelectedEndpoint] = useState('')
  const [validationErrors, setValidationErrors] = useState<Record<string, string>>({})
  const [expandedGrid, setExpandedGrid] = useState(false)

  const findProvider = (id: string) => getProviderByValue(id)
  const gridIds = expandedGrid ? EXPANDED_PROVIDER_IDS : DEFAULT_PROVIDER_IDS
  const allCards = gridIds.map((id) => findProvider(id)!).filter(Boolean)

  useEffect(() => {
    loadConfig()
  }, [])

  const loadConfig = async () => {
    try {
      const data = await api.config.get() as unknown as ConfigData
      if (!data.media) {
        data.media = {
          vision_enabled: false,
          vision_model: '',
          vision_detail: 'auto',
          transcription_enabled: false,
          transcription_model: '',
          transcription_base_url: '',
          transcription_api_key: false,
          transcription_language: '',
        }
      }
      setConfig(data)
      setOriginal(JSON.parse(JSON.stringify(data)))
      const prov = findProvider(data.provider)
      if (prov?.baseUrls) {
        const match = prov.baseUrls.find(ep => ep.value === data.base_url)
        if (match) setSelectedEndpoint(match.value)
      }
    } catch {
      setLoadError(true)
    } finally {
      setLoading(false)
    }
  }

  const hasChanges =
    JSON.stringify(config) !== JSON.stringify(original) ||
    apiKey !== '' ||
    transcriptionApiKey !== ''

  const provider = config ? findProvider(config.provider) : undefined
  const visibleModels = getVisibleModels(provider, selectedEndpoint)

  const handleProviderChange = useCallback((providerId: string) => {
    const newProvider = findProvider(providerId)
    const defaultUrl = newProvider ? getDefaultBaseUrl(newProvider) : ''
    setConfig(prev => prev ? {
      ...prev,
      provider: providerId,
      base_url: defaultUrl,
      model: '',
    } : prev)
    setSelectedEndpoint(defaultUrl)
    setTestResult(null)
    setValidationErrors({})
  }, [])

  const handleEndpointChange = useCallback((endpoint: string) => {
    setConfig(prev => prev ? { ...prev, base_url: endpoint, model: '' } : prev)
    setSelectedEndpoint(endpoint)
    setTestResult(null)
  }, [])

  const updateMedia = useCallback((key: keyof MediaConfig, value: unknown) => {
    setConfig(prev => prev ? { ...prev, media: { ...prev.media, [key]: value } } : prev)
  }, [])

  const validate = (): Record<string, string> => {
    if (!config) return {}
    const errors: Record<string, string> = {}
    if (!config.provider) errors.provider = t('apiConfig.validation.providerRequired')
    if (!config.model) errors.model = t('apiConfig.validation.modelRequired')
    if (!provider?.noKey && !config.api_key_configured && !apiKey) {
      errors.apiKey = t('apiConfig.validation.apiKeyRequired')
    }
    return errors
  }

  const scrollToFirstError = (errors: Record<string, string>) => {
    const firstKey = Object.keys(errors)[0]
    const el = document.querySelector(`[data-field="${firstKey}"]`)
    if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }

  const handleTestConnection = async () => {
    if (!config) return
    const errors = validate()
    if (Object.keys(errors).length > 0) {
      setValidationErrors(errors)
      scrollToFirstError(errors)
      return
    }
    setTestingConnection(true)
    setTestResult(null)
    try {
      const result = await api.setup.testProvider(
        config.provider,
        apiKey || 'test',
        config.model,
        config.base_url,
      )
      setTestResult({ success: result.success, error: result.error })
    } catch (error) {
      setTestResult({
        success: false,
        error: error instanceof Error ? error.message : t('apiConfig.connectionFailed'),
      })
    } finally {
      setTestingConnection(false)
    }
  }

  const handleSave = async (): Promise<boolean> => {
    if (!config) return false
    const errors = validate()
    if (Object.keys(errors).length > 0) {
      setValidationErrors(errors)
      scrollToFirstError(errors)
      return false
    }
    setSaving(true)
    try {
      const payload: Record<string, unknown> = {
        provider: config.provider,
        model: config.model,
        base_url: config.base_url,
        params: config.params,
        media: {
          ...config.media,
          transcription_api_key: transcriptionApiKey || undefined,
        },
      }
      if (apiKey) payload.api_key = apiKey
      const result = await api.config.update(payload) as unknown as ConfigData
      if (!result.media) result.media = config.media
      setConfig(result)
      setOriginal(JSON.parse(JSON.stringify(result)))
      setApiKey('')
      setTranscriptionApiKey('')
      setValidationErrors({})
      return true
    } catch {
      return false
    } finally {
      setSaving(false)
    }
  }

  const handleReset = () => {
    if (original) {
      setConfig(JSON.parse(JSON.stringify(original)))
      setApiKey('')
      setTranscriptionApiKey('')
      const prov = findProvider(original.provider)
      if (prov?.baseUrls) {
        const match = prov.baseUrls.find(ep => ep.value === original.base_url)
        setSelectedEndpoint(match?.value || '')
      } else {
        setSelectedEndpoint('')
      }
    }
    setTestResult(null)
    setValidationErrors({})
  }

  if (loading) return <LoadingSpinner />
  if (loadError || !config) return <ErrorState onRetry={() => window.location.reload()} />

  return (
    <div className="flex flex-col gap-4">
      {/* Page header */}
      <div className="flex flex-col gap-1 mb-2">
        <h1 className="text-lg font-medium text-primary">{t('config.pageTitle')}</h1>
        <p className="text-sm text-tertiary">{t('config.pageDescription')}</p>
      </div>

      {/* ═══ Provider & Model ═══ */}
      <ConfigSection
        icon={Sparkles}
        title={t('config.providerModel')}
        description={t('config.providerDesc')}
        defaultOpen
      >
        <div className="flex flex-col gap-6">
          {/* Provider grid */}
          <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-4">
            {allCards.map((p) => {
              const isSelected = config.provider === p.value
              const icon = getProviderIcon(p.value)
              return (
                <button
                  key={p.value}
                  type="button"
                  onClick={() => handleProviderChange(p.value)}
                  className={cn(
                    'flex cursor-pointer flex-col items-start gap-3 rounded-xl border p-4 transition-colors',
                    isSelected
                      ? 'border-brand-solid ring-1 ring-brand ring-inset'
                      : 'border-secondary hover:border-primary',
                  )}
                >
                  <div className="flex w-full items-start justify-between">
                    <div className="flex h-7 w-7 items-center justify-center text-tertiary">
                      {icon}
                    </div>
                    <Radio checked={isSelected} />
                  </div>
                  <span className="text-sm font-medium text-primary">{p.label}</span>
                </button>
              )
            })}

            {/* "See more" card */}
            {!expandedGrid && EXPANDED_PROVIDER_IDS.length > DEFAULT_PROVIDER_IDS.length && (
              <button
                type="button"
                onClick={() => setExpandedGrid(true)}
                className="flex cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-secondary p-4 transition-colors hover:border-primary hover:bg-secondary"
              >
                <Plus className="h-5 w-5 text-tertiary" />
                <span className="text-sm font-medium text-tertiary">{t('config.seeMore')}</span>
              </button>
            )}
          </div>

          {/* Divider */}
          <div className="h-px w-full bg-border" />

          {/* Endpoint selector */}
          {provider?.baseUrls && (
            <div className="flex flex-col gap-1.5">
              <p className="text-sm font-medium text-secondary">{t('apiConfig.endpoint')}</p>
              <div className="grid grid-cols-2 gap-3">
                {provider.baseUrls.map((ep) => {
                  const isActive = selectedEndpoint === ep.value
                  return (
                    <button
                      key={ep.value}
                      type="button"
                      onClick={() => handleEndpointChange(ep.value)}
                      className={cn(
                        'cursor-pointer rounded-xl border px-4 py-3 text-left transition-colors',
                        isActive
                          ? 'border-brand-solid ring-1 ring-brand ring-inset'
                          : 'border-secondary hover:border-primary',
                      )}
                    >
                      <span className={cn(
                        'text-sm font-medium',
                        isActive ? 'text-primary' : 'text-secondary',
                      )}>
                        {ep.label}
                      </span>
                      {ep.value && (
                        <p className="mt-0.5 truncate font-mono text-xs text-tertiary">
                          {ep.value.replace('https://', '')}
                        </p>
                      )}
                    </button>
                  )
                })}
              </div>
            </div>
          )}

          {/* Model */}
          <div data-field="model">
            <ConfigField
              label={t('apiConfig.model')}
              hint={validationErrors.model}
            >
              {visibleModels.length > 0 ? (
                <ModelCombobox
                  value={config.model}
                  onChange={(v) => {
                    setConfig(prev => prev ? { ...prev, model: v } : prev)
                    setValidationErrors(prev => { const n = { ...prev }; delete n.model; return n })
                  }}
                  suggestions={visibleModels}
                  placeholder={t('apiConfig.selectOrTypeModel')}
                />
              ) : (
                <ConfigInput
                  value={config.model}
                  onChange={(v) => {
                    setConfig(prev => prev ? { ...prev, model: v } : prev)
                    setValidationErrors(prev => { const n = { ...prev }; delete n.model; return n })
                  }}
                  placeholder={t('setupPage.modelName')}
                />
              )}
            </ConfigField>
          </div>

          {/* Custom base URL */}
          {provider?.customBaseUrl && (
            <ConfigField label={t('apiConfig.baseUrl')} hint={t('apiConfig.baseUrlHint')}>
              <ConfigInput
                value={config.base_url}
                onChange={(v) => setConfig(prev => prev ? { ...prev, base_url: v } : prev)}
                placeholder="https://api.example.com/v1"
              />
            </ConfigField>
          )}

          {/* API Key */}
          {!provider?.noKey && (
            <div data-field="apiKey">
              <ConfigField
                label={t('apiConfig.apiKey')}
                hint={validationErrors.apiKey || t('apiConfig.apiKeyHint')}
              >
                <PasswordInput
                  value={apiKey}
                  onChange={(v) => {
                    setApiKey(v)
                    setValidationErrors(prev => { const n = { ...prev }; delete n.apiKey; return n })
                  }}
                  placeholder={
                    config.api_key_configured
                      ? (config.api_key_masked || `••••••• (${t('config.apiKeyConfigured')})`)
                      : provider?.keyPlaceholder || t('apiConfig.apiKeyPlaceholder')
                  }
                />
              </ConfigField>
            </div>
          )}

          {/* Provider-specific params */}
          {provider?.value === 'anthropic' && (
            <ConfigToggle
              enabled={config.params?.context1m || false}
              onChange={(v) => setConfig(prev => prev ? {
                ...prev, params: { ...prev.params, context1m: v }
              } : prev)}
              label={t('apiConfig.context1m')}
              description={t('apiConfig.context1mDesc')}
            />
          )}

          {(provider?.value === 'zai' || (provider?.value === 'anthropic' && config.base_url.includes('api.z.ai'))) && (
            <ConfigToggle
              enabled={config.params?.tool_stream || false}
              onChange={(v) => setConfig(prev => prev ? {
                ...prev, params: { ...prev.params, tool_stream: v }
              } : prev)}
              label={t('apiConfig.toolStream')}
              description={t('apiConfig.toolStreamDesc')}
            />
          )}

          {/* Actions row: link + test connection */}
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            {provider?.freeUrl ? (
              <a
                href={provider.freeUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 text-sm font-semibold text-brand-tertiary transition-opacity hover:opacity-80"
              >
                {t('apiConfig.getApiKey')}
                <ExternalLink className="h-4 w-4" />
              </a>
            ) : <span />}

            <div className="flex items-center gap-3">
              {testResult && (
                <span className={cn(
                  'flex items-center gap-1.5 text-sm',
                  testResult.success ? 'text-fg-success-secondary' : 'text-fg-error-secondary',
                )}>
                  {testResult.success ? (
                    <>
                      <CheckCircle2 className="h-4 w-4" />
                      {testResult.latency ? `${testResult.latency}ms` : '✓'}
                    </>
                  ) : (
                    testResult.error || t('apiConfig.connectionFailed')
                  )}
                </span>
              )}
              <Button
                variant="outline"
                size="sm"
                onClick={handleTestConnection}
                disabled={
                  testingConnection ||
                  (!apiKey && !config.api_key_configured && !provider?.noKey) ||
                  !config.model
                }
              >
                {testingConnection ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t('apiConfig.testing')}
                  </>
                ) : (
                  <>
                    <Zap className="h-4 w-4" />
                    {t('apiConfig.testConnection')}
                  </>
                )}
              </Button>
            </div>
          </div>
        </div>
      </ConfigSection>

      {/* ═══ Vision ═══ */}
      <ConfigSection
        icon={Image}
        title={t('config.visionTitle')}
        description={t('config.visionDesc')}
        trailing={
          <ConfigToggle
            enabled={config.media.vision_enabled}
            onChange={(v) => updateMedia('vision_enabled', v)}
            label=""
          />
        }
      >
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
      </ConfigSection>

      {/* ═══ Transcription ═══ */}
      <ConfigSection
        icon={Mic}
        title={t('config.transcriptionTitle')}
        description={t('config.transcriptionDesc')}
        trailing={
          <ConfigToggle
            enabled={config.media.transcription_enabled}
            onChange={(v) => updateMedia('transcription_enabled', v)}
            label=""
          />
        }
      >
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
            placeholder={
              config.media.transcription_api_key
                ? `••••••• (${t('config.apiKeyConfigured')})`
                : t('config.transcriptionApiKeyPlaceholder')
            }
          />
        </ConfigField>
      </ConfigSection>

      <UnsavedChangesBar
        hasChanges={hasChanges}
        saving={saving}
        onSave={handleSave}
        onDiscard={handleReset}
      />
    </div>
  )
}

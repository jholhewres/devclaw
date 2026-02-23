import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Eye,
  EyeOff,
  Key,
  CheckCircle2,
  XCircle,
  Loader2,
  Zap,
  Cpu,
  AlertTriangle,
  ExternalLink,
  AlertCircle,
} from 'lucide-react'
import { api } from '@/lib/api'
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigSelect,
  ConfigActions,
  ConfigCard,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'
import { ProviderCard } from '@/components/ui/ProviderCard'
import { EndpointSelector } from '@/components/ui/EndpointSelector'
import {
  categorizeProviders,
  getProviderByValue,
  getVisibleModels,
  getDefaultBaseUrl,
  PROVIDER_CATEGORIES,
} from '@/lib/providers'

interface APIConfigData {
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
}

interface ConnectionTestResult {
  success: boolean
  latency?: number
  error?: string
  models?: string[]
}

// Password input with toggle
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
        className="h-11 w-full rounded-xl border border-white/10 bg-[#111827] px-4 pr-10 text-sm text-[#f8fafc] outline-none transition-all placeholder:text-[#475569] hover:border-white/20 focus:border-[#3b82f6]/50 focus:ring-1 focus:ring-[#3b82f6]/20"
      />
      <button
        type="button"
        onClick={() => setShow(!show)}
        className="absolute right-3 top-1/2 -translate-y-1/2 text-[#64748b] hover:text-[#f8fafc] cursor-pointer"
      >
        {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}

export function ApiConfig() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<APIConfigData | null>(null)
  const [original, setOriginal] = useState<APIConfigData | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [apiKey, setApiKey] = useState('')
  const [testingConnection, setTestingConnection] = useState(false)
  const [testResult, setTestResult] = useState<ConnectionTestResult | null>(null)
  const [selectedEndpoint, setSelectedEndpoint] = useState<string>('')
  const [validationErrors, setValidationErrors] = useState<Record<string, string>>({})

  // Categorized providers
  const { free, paid, local } = categorizeProviders()

  useEffect(() => {
    loadConfig()
  }, [])

  const loadConfig = async () => {
    try {
      const data = await api.config.get() as unknown as APIConfigData
      setConfig(data)
      setOriginal(JSON.parse(JSON.stringify(data)))
      // Set selected endpoint from base_url if matches a provider's baseUrls
      const provider = getProviderByValue(data.provider)
      if (provider?.baseUrls) {
        const matchingEndpoint = provider.baseUrls.find(ep => ep.value === data.base_url)
        if (matchingEndpoint) {
          setSelectedEndpoint(matchingEndpoint.value)
        }
      }
    } catch {
      setLoadError(true)
    } finally {
      setLoading(false)
    }
  }

  const hasChanges = JSON.stringify(config) !== JSON.stringify(original) || apiKey !== ''

  // Get current provider definition
  const provider = config ? getProviderByValue(config.provider) : undefined
  const visibleModels = getVisibleModels(provider, selectedEndpoint)

  const handleProviderChange = useCallback((providerId: string) => {
    const newProvider = getProviderByValue(providerId)
    const defaultUrl = newProvider ? getDefaultBaseUrl(newProvider) : ''

    setConfig(prev => prev ? {
      ...prev,
      provider: providerId,
      base_url: defaultUrl,
      model: '',
    } : prev)
    setSelectedEndpoint('')
    setTestResult(null)
    setValidationErrors({})
  }, [])

  const handleEndpointChange = useCallback((endpoint: string) => {
    setConfig(prev => prev ? {
      ...prev,
      base_url: endpoint,
    } : prev)
    setSelectedEndpoint(endpoint)
    setTestResult(null)
  }, [])

  const handleTestConnection = async () => {
    if (!config) return

    // Validate before testing
    const errors: Record<string, string> = {}
    if (!config.provider) errors.provider = t('apiConfig.validation.providerRequired')
    if (!config.model) errors.model = t('apiConfig.validation.modelRequired')
    if (!provider?.noKey && !config.api_key_configured && !apiKey) {
      errors.apiKey = t('apiConfig.validation.apiKeyRequired')
    }
    if (Object.keys(errors).length > 0) {
      setValidationErrors(errors)
      return
    }

    setTestingConnection(true)
    setTestResult(null)
    setMessage(null)

    try {
      const result = await api.setup.testProvider(
        config.provider,
        apiKey || 'test',
        config.model,
        config.base_url
      )
      setTestResult({
        success: result.success,
        error: result.error,
      })
    } catch (error) {
      setTestResult({
        success: false,
        error: error instanceof Error ? error.message : 'Connection test failed',
      })
    } finally {
      setTestingConnection(false)
    }
  }

  const handleSave = async () => {
    if (!config) return

    // Validate before saving
    const errors: Record<string, string> = {}
    if (!config.provider) errors.provider = t('apiConfig.validation.providerRequired')
    if (!config.model) errors.model = t('apiConfig.validation.modelRequired')
    if (!provider?.noKey && !config.api_key_configured && !apiKey) {
      errors.apiKey = t('apiConfig.validation.apiKeyRequired')
    }
    if (Object.keys(errors).length > 0) {
      setValidationErrors(errors)
      return
    }

    setSaving(true)
    setMessage(null)
    try {
      const payload: Record<string, unknown> = {
        provider: config.provider,
        model: config.model,
        base_url: config.base_url,
        params: config.params,
      }
      if (apiKey) {
        payload.api_key = apiKey
      }
      const result = await api.config.update(payload) as unknown as APIConfigData
      setConfig(result)
      setOriginal(JSON.parse(JSON.stringify(result)))
      setApiKey('')
      setValidationErrors({})
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
      setApiKey('')
      // Reset selected endpoint
      const prov = getProviderByValue(original.provider)
      if (prov?.baseUrls) {
        const matchingEndpoint = prov.baseUrls.find(ep => ep.value === original.base_url)
        setSelectedEndpoint(matchingEndpoint?.value || '')
      } else {
        setSelectedEndpoint('')
      }
    }
    setMessage(null)
    setTestResult(null)
    setValidationErrors({})
  }

  if (loading) return <LoadingSpinner />
  if (loadError || !config) return <ErrorState onRetry={() => window.location.reload()} />

  return (
    <ConfigPage
      title={t('apiConfig.title')}
      subtitle={t('apiConfig.subtitle')}
      description={t('apiConfig.description')}
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
      {/* Restart Warning */}
      {hasChanges && (
        <div className="mb-6 flex items-center gap-2 rounded-xl border border-[#f59e0b]/20 bg-[#f59e0b]/5 px-4 py-3">
          <AlertCircle className="h-4 w-4 text-[#f59e0b] flex-shrink-0" />
          <p className="text-sm text-[#fcd34d]">{t('apiConfig.restartWarning')}</p>
        </div>
      )}

      {/* Provider Selection - Free */}
      <ConfigSection
        icon={Cpu}
        title={t('apiConfig.freeProviders')}
        description=""
      >
        <div className="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-6 gap-2 -mt-2">
          {free.map((p) => (
            <ProviderCard
              key={p.value}
              provider={p}
              isSelected={config.provider === p.value}
              onClick={() => handleProviderChange(p.value)}
              accentColor={PROVIDER_CATEGORIES.free.accentColor}
              size="sm"
            />
          ))}
        </div>
      </ConfigSection>

      {/* Provider Selection - Paid */}
      <ConfigSection
        icon={Cpu}
        title={t('apiConfig.paidProviders')}
        description=""
      >
        <div className="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-6 gap-2 -mt-2">
          {paid.map((p) => (
            <ProviderCard
              key={p.value}
              provider={p}
              isSelected={config.provider === p.value}
              onClick={() => handleProviderChange(p.value)}
              accentColor={PROVIDER_CATEGORIES.paid.accentColor}
              size="sm"
            />
          ))}
        </div>
      </ConfigSection>

      {/* Provider Selection - Local */}
      <ConfigSection
        icon={Cpu}
        title={t('apiConfig.localProviders')}
        description=""
      >
        <div className="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-6 gap-2 -mt-2">
          {local.map((p) => (
            <ProviderCard
              key={p.value}
              provider={p}
              isSelected={config.provider === p.value}
              onClick={() => handleProviderChange(p.value)}
              accentColor={PROVIDER_CATEGORIES.local.accentColor}
              size="sm"
            />
          ))}
        </div>
      </ConfigSection>

      {/* API Configuration */}
      <ConfigSection
        icon={Key}
        title={t('apiConfig.connectionSection')}
        description={t('apiConfig.connectionSectionDesc')}
      >
        {/* Provider info with link */}
        {provider && provider.freeUrl && (
          <div className="flex items-center gap-2 rounded-lg border border-white/10 bg-[#0c1222] px-3 py-2 mb-4">
            <div className="flex-1">
              <p className="text-xs text-[#94a3b8]">
                {provider.freeNote || provider.description}
              </p>
            </div>
            <a
              href={provider.freeUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-1 text-xs text-[#3b82f6] hover:text-[#60a5fa] transition-colors"
            >
              {t('apiConfig.getApiKey')}
              <ExternalLink className="h-3 w-3" />
            </a>
          </div>
        )}

        {/* Endpoint selector */}
        {provider?.baseUrls && (
          <ConfigField label={t('apiConfig.endpoint')} hint={t('apiConfig.endpointHint')}>
            <EndpointSelector
              endpoints={provider.baseUrls}
              value={selectedEndpoint}
              onChange={handleEndpointChange}
            />
          </ConfigField>
        )}

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
          <div className="space-y-2">
            <ConfigField
              label={t('apiConfig.apiKey')}
              hint={t('apiConfig.apiKeyHint')}
            >
              <PasswordInput
                value={apiKey}
                onChange={setApiKey}
                placeholder={config.api_key_configured ? (config.api_key_masked || '••••••• (configured)') : provider?.keyPlaceholder || t('apiConfig.apiKeyPlaceholder')}
              />
            </ConfigField>
            {validationErrors.apiKey && (
              <p className="text-xs text-[#f87171]">{validationErrors.apiKey}</p>
            )}
          </div>
        )}

        {/* Model */}
        <div className="space-y-2">
          <ConfigField
            label={t('apiConfig.model')}
            hint={t('apiConfig.modelHint')}
          >
          {visibleModels.length > 0 ? (
            <ConfigSelect
              value={config.model}
              onChange={(v) => setConfig(prev => prev ? { ...prev, model: v } : prev)}
              options={visibleModels.map(m => ({ value: m, label: m }))}
              placeholder={t('apiConfig.selectModel')}
            />
          ) : (
            <ConfigInput
              value={config.model}
              onChange={(v) => setConfig(prev => prev ? { ...prev, model: v } : prev)}
              placeholder="gpt-4o-mini, claude-sonnet-4-20250514, gemini-2.5-pro"
            />
          )}
        </ConfigField>
        {validationErrors.model && (
          <p className="text-xs text-[#f87171] -mt-3">{validationErrors.model}</p>
        )}
        </div>

        {/* Provider-specific params */}
        {provider?.value === 'anthropic' && (
          <div className="flex items-center gap-3 py-2">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={config.params?.context1m || false}
                onChange={(e) => setConfig(prev => prev ? {
                  ...prev,
                  params: { ...prev.params, context1m: e.target.checked }
                } : prev)}
                className="h-4 w-4 rounded border-white/20 bg-[#111827] text-[#3b82f6] focus:ring-[#3b82f6]/20"
              />
              <span className="text-sm text-[#94a3b8]">{t('apiConfig.context1m')}</span>
            </label>
            <span className="text-xs text-[#64748b]">{t('apiConfig.context1mDesc')}</span>
          </div>
        )}

        {provider?.value === 'zai' && (
          <div className="flex items-center gap-3 py-2">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={config.params?.tool_stream || false}
                onChange={(e) => setConfig(prev => prev ? {
                  ...prev,
                  params: { ...prev.params, tool_stream: e.target.checked }
                } : prev)}
                className="h-4 w-4 rounded border-white/20 bg-[#111827] text-[#3b82f6] focus:ring-[#3b82f6]/20"
              />
              <span className="text-sm text-[#94a3b8]">{t('apiConfig.toolStream')}</span>
            </label>
            <span className="text-xs text-[#64748b]">{t('apiConfig.toolStreamDesc')}</span>
          </div>
        )}

        {/* Connection Test */}
        <div className="pt-4 border-t border-white/5">
          <div className="flex items-center justify-between">
            <button
              onClick={handleTestConnection}
              disabled={testingConnection || !config.base_url}
              className="flex cursor-pointer items-center gap-2 rounded-xl border border-white/10 bg-[#1e293b] px-4 py-2.5 text-sm font-medium text-[#94a3b8] transition-all hover:border-white/20 hover:text-[#f8fafc] disabled:opacity-50 disabled:cursor-not-allowed"
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
            </button>

            {/* Test Result */}
            {testResult && (
              <div className={`flex items-center gap-2 px-3 py-1.5 rounded-lg ${
                testResult.success
                  ? 'bg-[#22c55e]/10 text-[#22c55e]'
                  : 'bg-[#ef4444]/10 text-[#f87171]'
              }`}>
                {testResult.success ? (
                  <>
                    <CheckCircle2 className="h-4 w-4" />
                    <span className="text-sm font-medium">
                      {testResult.latency && `${testResult.latency}ms`}
                    </span>
                  </>
                ) : (
                  <>
                    <XCircle className="h-4 w-4" />
                    <span className="text-sm">{t('apiConfig.connectionFailed')}</span>
                  </>
                )}
              </div>
            )}
          </div>

          {/* Error Message */}
          {testResult && !testResult.success && testResult.error && (
            <div className="mt-3 flex items-start gap-2 rounded-lg bg-[#ef4444]/5 border border-[#ef4444]/10 p-3">
              <AlertTriangle className="h-4 w-4 text-[#f87171] flex-shrink-0 mt-0.5" />
              <p className="text-xs text-[#f87171]">{testResult.error}</p>
            </div>
          )}
        </div>
      </ConfigSection>

      {/* Status Card */}
      <ConfigCard
        title={t('apiConfig.statusTitle')}
        icon={config.api_key_configured ? CheckCircle2 : AlertTriangle}
        status={config.api_key_configured ? 'success' : 'warning'}
        className="mb-10"
        actions={
          <div className="text-right">
            <p className="text-xs text-[#64748b]">{t('apiConfig.currentProvider')}</p>
            <p className="text-sm font-medium text-[#f8fafc] capitalize">{config.provider}</p>
          </div>
        }
      >
        <p className={`text-sm ${config.api_key_configured ? 'text-[#22c55e]' : 'text-[#f59e0b]'}`}>
          {config.api_key_configured
            ? t('apiConfig.statusConfigured')
            : t('apiConfig.statusNotConfigured')
          }
        </p>
      </ConfigCard>
    </ConfigPage>
  )
}

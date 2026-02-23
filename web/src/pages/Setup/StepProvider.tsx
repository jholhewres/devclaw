import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, XCircle, Key, Cpu, ExternalLink, Link } from 'lucide-react'
import { api } from '@/lib/api'
import type { SetupData } from './SetupWizard'
import {
  StepContainer, StepHeader, FieldGroup, Field,
  Input, PasswordInput, Select, Button,
} from './SetupComponents'
import {
  PROVIDERS,
  categorizeProviders,
  getVisibleModels,
  getProviderIcon,
  PROVIDER_CATEGORIES,
  type ProviderDef,
} from '@/lib/providers'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

export function StepProvider({ data, updateData }: Props) {
  const { t } = useTranslation()
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ success: boolean; error?: string } | null>(null)

  // Categorized providers
  const { free, paid, local } = categorizeProviders()

  // Get current provider
  const provider = PROVIDERS.find((p) => p.value === data.provider)
  const activeEndpoint = provider?.baseUrls?.find((ep) => ep.value === data.baseUrl)
  const visibleModels = getVisibleModels(provider, data.baseUrl)

  const handleTest = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      const result = await api.setup.testProvider(data.provider, data.apiKey, data.model, data.baseUrl)
      setTestResult(result)
    } catch (err) {
      setTestResult({ success: false, error: err instanceof Error ? err.message : t('setupPage.connectionFailed') })
    } finally {
      setTesting(false)
    }
  }

  const handleProviderChange = (value: string) => {
    updateData({ provider: value, model: '', baseUrl: '' })
    setTestResult(null)
  }

  const handleApiKeyChange = (value: string) => {
    updateData({ apiKey: value })
    setTestResult(null)
  }

  // Provider card button component
  const ProviderButton = ({ p, category }: { p: ProviderDef; category: 'free' | 'paid' | 'local' }) => {
    const categoryStyle = PROVIDER_CATEGORIES[category]
    const isSelected = data.provider === p.value
    const icon = getProviderIcon(p.value)

    return (
      <button
        key={p.value}
        onClick={() => handleProviderChange(p.value)}
        className={`flex cursor-pointer flex-col items-center gap-1 rounded-xl border px-2 py-2.5 text-center transition-all ${
          isSelected
            ? `${categoryStyle.borderColor} ${categoryStyle.bgColor}`
            : 'border-white/10 bg-[#0c1222] hover:border-white/20 hover:bg-[#111827]'
        }`}
        style={isSelected ? {
          borderColor: `${categoryStyle.accentColor}80`,
          backgroundColor: `${categoryStyle.accentColor}15`,
        } : undefined}
        title={p.description}
      >
        <div className={isSelected ? categoryStyle.textColor : 'text-[#64748b]'} style={isSelected ? { color: categoryStyle.accentColor } : undefined}>
          {icon}
        </div>
        <span className={`text-[10px] font-medium ${isSelected ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}>
          {p.label}
        </span>
      </button>
    )
  }

  return (
    <StepContainer>
      <StepHeader
        title={t('setupPage.providerTitle')}
        description={t('setupPage.providerDesc')}
      />

      <FieldGroup>
        {/* Free Providers */}
        <Field label={t('setupPage.freeProviders')} icon={Cpu}>
          <div className="grid grid-cols-5 gap-1.5">
            {free.map((p) => (
              <ProviderButton key={p.value} p={p} category="free" />
            ))}
          </div>
        </Field>

        {/* Paid Providers */}
        <Field label={t('setupPage.paidProviders')} icon={Cpu}>
          <div className="grid grid-cols-5 gap-1.5">
            {paid.map((p) => (
              <ProviderButton key={p.value} p={p} category="paid" />
            ))}
          </div>
        </Field>

        {/* Local / Self-Hosted Providers */}
        <Field label={t('setupPage.localProviders')} icon={Cpu}>
          <div className="grid grid-cols-5 gap-1.5">
            {local.map((p) => (
              <ProviderButton key={p.value} p={p} category="local" />
            ))}
          </div>
        </Field>

        {/* Provider info with link */}
        {provider && provider.freeUrl && (
          <div className="flex items-center gap-2 rounded-lg border border-white/10 bg-[#0c1222] px-3 py-2">
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
              {t('setupPage.getApiKey')}
              <ExternalLink className="h-3 w-3" />
            </a>
          </div>
        )}

        {/* Endpoint selector */}
        {provider?.baseUrls && (
          <Field label={t('setupPage.endpoint')} icon={Link}>
            <div className="grid grid-cols-2 gap-2">
              {provider.baseUrls.map((ep) => (
                <button
                  key={ep.value}
                  onClick={() => updateData({ baseUrl: ep.value })}
                  className={`cursor-pointer rounded-xl border px-3 py-2.5 text-left transition-all ${
                    data.baseUrl === ep.value
                      ? 'border-[#3b82f6]/50 bg-[#3b82f6]/10'
                      : 'border-white/10 bg-[#0c1222] hover:border-white/20 hover:bg-[#111827]'
                  }`}
                >
                  <span className={`text-xs font-medium ${data.baseUrl === ep.value ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}>
                    {ep.label}
                  </span>
                  {ep.value && (
                    <p className="mt-0.5 truncate text-[10px] text-[#64748b] font-mono">
                      {ep.value.replace('https://', '')}
                    </p>
                  )}
                </button>
              ))}
            </div>
          </Field>
        )}

        {/* Custom base URL */}
        {provider?.customBaseUrl && (
          <Field label="Base URL" icon={Link} hint="OpenAI-compatible endpoint (/v1/chat/completions)">
            <Input
              value={data.baseUrl}
              onChange={(val) => updateData({ baseUrl: val })}
              placeholder="https://api.example.com/v1"
              mono
            />
          </Field>
        )}

        {/* API Key */}
        {!provider?.noKey && (
          <Field label={t('setupPage.apiKey')} icon={Key} hint={t('setupPage.apiKeyHint')}>
            <PasswordInput
              value={data.apiKey}
              onChange={handleApiKeyChange}
              placeholder={provider?.keyPlaceholder || t('setupPage.apiKey')}
            />
          </Field>
        )}

        {/* Model */}
        <Field label={t('setupPage.model')} icon={Cpu}>
          {visibleModels.length > 0 ? (
            <Select
              value={data.model}
              onChange={(val) => updateData({ model: val })}
              placeholder={t('setupPage.selectModel')}
              groups={[
                ...(activeEndpoint?.extraModels ? [{
                  label: activeEndpoint.label,
                  options: activeEndpoint.extraModels.map(m => ({ value: m, label: m })),
                }] : []),
                ...(provider ? [{
                  label: provider.label,
                  options: provider.models.map(m => ({ value: m, label: m })),
                }] : []),
              ]}
            />
          ) : (
            <Input
              value={data.model}
              onChange={(val) => updateData({ model: val })}
              placeholder={t('setupPage.modelName')}
            />
          )}
        </Field>

        {/* Test connection */}
        <div className="flex items-center gap-3 pt-1">
          <Button
            onClick={handleTest}
            disabled={testing || (!data.apiKey && !provider?.noKey) || !data.model}
            loading={testing}
            icon={ExternalLink}
          >
            {t('setupPage.testConnection')}
          </Button>

          {testResult && (
            <div className="flex items-center gap-1.5 text-sm">
              {testResult.success ? (
                <span className="flex items-center gap-1.5 text-[#22c55e]">
                  <CheckCircle2 className="h-4 w-4" /> {t('setupPage.connected')}
                </span>
              ) : (
                <span className="flex items-center gap-1.5 text-[#f87171]">
                  <XCircle className="h-4 w-4" /> {testResult.error}
                </span>
              )}
            </div>
          )}
        </div>
      </FieldGroup>
    </StepContainer>
  )
}

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, XCircle, Key, Cpu, ExternalLink, Link, Plus } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api } from '@/lib/api'
import type { SetupData } from './SetupWizard'
import {
  StepContainer, StepHeader, FieldGroup, Field,
  Input, PasswordInput, Button,
} from './SetupComponents'
import { ModelCombobox } from '@/components/ui/ModelCombobox'
import {
  PROVIDERS,
  DEFAULT_PROVIDER_IDS,
  EXPANDED_PROVIDER_IDS,
  getVisibleModels,
  getProviderIcon,
} from '@/lib/providers'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

export function StepProvider({ data, updateData }: Props) {
  const { t } = useTranslation()
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ success: boolean; error?: string } | null>(null)
  const [expandedGrid, setExpandedGrid] = useState(false)

  // Build provider grid
  const gridIds = expandedGrid ? EXPANDED_PROVIDER_IDS : DEFAULT_PROVIDER_IDS
  const allCards = gridIds.map((id) => PROVIDERS.find((p) => p.value === id)!).filter(Boolean)

  // Get current provider
  const provider = PROVIDERS.find((p) => p.value === data.provider)
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

  return (
    <StepContainer>
      <StepHeader
        title={t('setupPage.providerTitle')}
        description={t('setupPage.providerDesc')}
      />

      <FieldGroup>
        {/* Provider grid */}
        <Field label={t('setupPage.providerTitle')} icon={Cpu}>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
            {allCards.map((p) => {
              const isSelected = data.provider === p.value
              const icon = getProviderIcon(p.value)
              return (
                <button
                  key={p.value}
                  type="button"
                  onClick={() => handleProviderChange(p.value)}
                  className={cn(
                    'flex cursor-pointer flex-col items-start gap-2.5 rounded-xl border p-3 transition-colors',
                    isSelected
                      ? 'border-brand-solid ring-1 ring-brand ring-inset'
                      : 'border-primary bg-primary hover:border-text-muted hover:bg-primary',
                  )}
                  title={p.description}
                >
                  <div className="flex w-full items-start justify-between">
                    <div className="flex h-6 w-6 items-center justify-center text-tertiary">
                      {icon}
                    </div>
                    <div
                      className={cn(
                        'flex h-4 w-4 items-center justify-center rounded-full border-2 transition-colors',
                        isSelected ? 'border-brand-solid bg-brand-solid' : 'border-primary',
                      )}
                    >
                      {isSelected && <div className="h-1.5 w-1.5 rounded-full bg-white" />}
                    </div>
                  </div>
                  <span className={cn(
                    'text-xs font-medium',
                    isSelected ? 'text-primary' : 'text-secondary',
                  )}>
                    {p.label}
                  </span>
                </button>
              )
            })}

            {/* See more */}
            {!expandedGrid && EXPANDED_PROVIDER_IDS.length > DEFAULT_PROVIDER_IDS.length && (
              <button
                type="button"
                onClick={() => setExpandedGrid(true)}
                className="flex cursor-pointer flex-col items-center justify-center gap-1.5 rounded-xl border border-dashed border-primary p-3 transition-colors hover:border-text-muted hover:bg-primary"
              >
                <Plus className="h-4 w-4 text-tertiary" />
                <span className="text-xs font-medium text-tertiary">{t('config.seeMore')}</span>
              </button>
            )}
          </div>
        </Field>

        {/* Provider info with link */}
        {provider && provider.freeUrl && (
          <div className="flex items-center gap-2 rounded-lg border border-primary bg-primary px-3 py-2">
            <div className="flex-1">
              <p className="text-xs text-secondary">
                {provider.freeNote || provider.description}
              </p>
            </div>
            <a
              href={provider.freeUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-1 text-xs text-brand-tertiary transition-colors hover:text-brand-hover"
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
                  className={cn(
                    'cursor-pointer rounded-xl border px-3 py-2.5 text-left transition-all',
                    data.baseUrl === ep.value
                      ? 'border-brand/50 bg-brand-secondary'
                      : 'border-primary bg-primary hover:border-text-muted hover:bg-primary',
                  )}
                >
                  <span className={cn(
                    'text-xs font-medium',
                    data.baseUrl === ep.value ? 'text-primary' : 'text-secondary',
                  )}>
                    {ep.label}
                  </span>
                  {ep.value && (
                    <p className="mt-0.5 truncate font-mono text-[10px] text-tertiary">
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
            <ModelCombobox
              value={data.model}
              onChange={(val) => updateData({ model: val })}
              suggestions={visibleModels}
              placeholder={t('setupPage.selectOrTypeModel')}
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
                <span className="flex items-center gap-1.5 text-fg-success-secondary">
                  <CheckCircle2 className="h-4 w-4" /> {t('setupPage.connected')}
                </span>
              ) : (
                <span className="flex items-center gap-1.5 text-fg-error-secondary">
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

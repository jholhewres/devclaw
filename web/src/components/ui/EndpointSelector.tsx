/**
 * EndpointSelector - Select endpoint for providers with multiple base URLs
 * Used by: ApiConfig.tsx, Setup/StepProvider.tsx
 */

import { useTranslation } from 'react-i18next'
import type { BaseUrlOption } from '@/lib/providers'
import { cn } from '@/lib/utils'

export interface EndpointSelectorProps {
  endpoints: BaseUrlOption[]
  value: string
  onChange: (value: string) => void
  accentColor?: string
  layout?: 'grid' | 'list'
}

export function EndpointSelector({
  endpoints,
  value,
  onChange,
  accentColor = '#3b82f6',
  layout = 'grid',
}: EndpointSelectorProps) {
  const { t } = useTranslation()

  if (endpoints.length === 0) return null

  if (layout === 'list') {
    return (
      <div className="space-y-2">
        {endpoints.map((ep) => (
          <button
            key={ep.value}
            onClick={() => onChange(ep.value)}
            className={cn(
              'w-full cursor-pointer rounded-xl border px-4 py-3 text-left transition-all',
              value !== ep.value &&
                'border-secondary bg-primary hover:border-primary hover:bg-primary',
            )}
            style={value === ep.value ? {
              borderColor: `${accentColor}80`,
              backgroundColor: `${accentColor}15`,
            } : undefined}
          >
            <span className={cn(
              'text-sm font-medium',
              value === ep.value ? 'text-primary' : 'text-secondary',
            )}>
              {ep.label}
            </span>
            {ep.value && (
              <p className="mt-0.5 truncate text-xs text-tertiary font-mono">
                {ep.value.replace('https://', '').replace('http://', '')}
              </p>
            )}
            {ep.extraModels && ep.extraModels.length > 0 && (
              <p className="mt-1 text-[10px] text-fg-success-secondary">
                {t('common.extraModels', { count: ep.extraModels.length })}
              </p>
            )}
          </button>
        ))}
      </div>
    )
  }

  return (
    <div className="grid grid-cols-2 gap-2">
      {endpoints.map((ep) => (
        <button
          key={ep.value}
          onClick={() => onChange(ep.value)}
          className={cn(
            'cursor-pointer rounded-xl border px-3 py-2.5 text-left transition-all',
            value !== ep.value &&
              'border-secondary bg-primary hover:border-primary hover:bg-primary',
          )}
          style={value === ep.value ? {
            borderColor: `${accentColor}80`,
            backgroundColor: `${accentColor}15`,
          } : undefined}
        >
          <span className={cn(
            'text-xs font-medium',
            value === ep.value ? 'text-primary' : 'text-secondary',
          )}>
            {ep.label}
          </span>
          {ep.value && (
            <p className="mt-0.5 truncate text-[10px] text-tertiary font-mono">
              {ep.value.replace('https://', '').replace('http://', '')}
            </p>
          )}
        </button>
      ))}
    </div>
  )
}

export default EndpointSelector

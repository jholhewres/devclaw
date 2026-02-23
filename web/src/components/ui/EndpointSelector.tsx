/**
 * EndpointSelector - Select endpoint for providers with multiple base URLs
 * Used by: ApiConfig.tsx, Setup/StepProvider.tsx
 */

import type { BaseUrlOption } from '@/lib/providers'

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
  if (endpoints.length === 0) return null

  if (layout === 'list') {
    return (
      <div className="space-y-2">
        {endpoints.map((ep) => (
          <button
            key={ep.value}
            onClick={() => onChange(ep.value)}
            className={`w-full cursor-pointer rounded-xl border px-4 py-3 text-left transition-all ${
              value === ep.value
                ? `border-[${accentColor}]/50 bg-[${accentColor}]/10`
                : 'border-white/10 bg-[#0c1222] hover:border-white/20 hover:bg-[#111827]'
            }`}
            style={value === ep.value ? {
              borderColor: `${accentColor}80`,
              backgroundColor: `${accentColor}15`,
            } : undefined}
          >
            <span className={`text-sm font-medium ${value === ep.value ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}>
              {ep.label}
            </span>
            {ep.value && (
              <p className="mt-0.5 truncate text-xs text-[#64748b] font-mono">
                {ep.value.replace('https://', '').replace('http://', '')}
              </p>
            )}
            {ep.extraModels && ep.extraModels.length > 0 && (
              <p className="mt-1 text-[10px] text-[#22c55e]">
                +{ep.extraModels.length} extra models
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
          className={`cursor-pointer rounded-xl border px-3 py-2.5 text-left transition-all ${
            value === ep.value
              ? `border-[${accentColor}]/50 bg-[${accentColor}]/10`
              : 'border-white/10 bg-[#0c1222] hover:border-white/20 hover:bg-[#111827]'
          }`}
          style={value === ep.value ? {
            borderColor: `${accentColor}80`,
            backgroundColor: `${accentColor}15`,
          } : undefined}
        >
          <span className={`text-xs font-medium ${value === ep.value ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}>
            {ep.label}
          </span>
          {ep.value && (
            <p className="mt-0.5 truncate text-[10px] text-[#64748b] font-mono">
              {ep.value.replace('https://', '').replace('http://', '')}
            </p>
          )}
        </button>
      ))}
    </div>
  )
}

export default EndpointSelector

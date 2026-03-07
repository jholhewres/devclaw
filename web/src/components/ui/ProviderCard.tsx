/**
 * ProviderCard - Reusable provider selection card
 * Used by: ApiConfig.tsx, Setup/StepProvider.tsx
 */

import { CheckCircle2 } from 'lucide-react'
import type { ProviderDef } from '@/lib/providers'
import { getProviderIcon } from '@/lib/providers'

export interface ProviderCardProps {
  provider: ProviderDef
  isSelected: boolean
  onClick: () => void
  accentColor?: string
  size?: 'sm' | 'md'
  showDescription?: boolean
}

export function ProviderCard({
  provider,
  isSelected,
  onClick,
  accentColor,
  size = 'md',
  showDescription = false,
}: ProviderCardProps) {
  const color = accentColor || provider.color || '#64748b'
  const icon = getProviderIcon(provider.value)

  const sizeClasses = size === 'sm'
    ? 'p-2.5 gap-1'
    : 'p-4 gap-2'

  return (
    <button
      onClick={onClick}
      className={`relative flex flex-col items-center justify-center rounded-xl border transition-all cursor-pointer ${sizeClasses} ${
        isSelected
          ? ''
          : 'border-border bg-bg-main hover:border-border-hover hover:bg-bg-surface'
      }`}
      style={isSelected ? {
        borderColor: `${color}80`,
        backgroundColor: `${color}15`,
      } : undefined}
      title={provider.description}
    >
      {isSelected && (
        <div className="absolute top-2 right-2">
          <CheckCircle2 className="h-4 w-4" style={{ color }} />
        </div>
      )}

      <div
        className={`flex items-center justify-center rounded-lg ${size === 'sm' ? 'h-8 w-8' : 'h-10 w-10'}`}
        style={{ backgroundColor: `${color}20` }}
      >
        <div style={{ color }}>
          {icon}
        </div>
      </div>

      <span
        className={`font-medium ${size === 'sm' ? 'text-[10px]' : 'text-sm'} ${isSelected ? 'text-text-primary' : 'text-text-secondary'}`}
      >
        {provider.label}
      </span>

      {showDescription && size === 'md' && (
        <span className="text-[10px] text-text-muted text-center line-clamp-1">
          {provider.description}
        </span>
      )}
    </button>
  )
}

/**
 * ProviderCardCompact - Smaller variant for grids
 */
export function ProviderCardCompact({
  provider,
  isSelected,
  onClick,
  accentColor,
}: Omit<ProviderCardProps, 'size' | 'showDescription'>) {
  const color = accentColor || provider.color || '#64748b'
  const icon = getProviderIcon(provider.value)

  return (
    <button
      onClick={onClick}
      className={`flex cursor-pointer flex-col items-center gap-1 rounded-xl border px-2 py-2.5 text-center transition-all ${
        isSelected
          ? ''
          : 'border-border bg-bg-main hover:border-border-hover hover:bg-bg-surface'
      }`}
      style={isSelected ? {
        borderColor: `${color}80`,
        backgroundColor: `${color}15`,
      } : undefined}
      title={provider.description}
    >
      <div style={isSelected ? { color } : undefined} className={isSelected ? '' : 'text-text-muted'}>
        {icon}
      </div>
      <span className={`text-[10px] font-medium ${isSelected ? 'text-text-primary' : 'text-text-secondary'}`}>
        {provider.label}
      </span>
    </button>
  )
}

export default ProviderCard

/**
 * ProviderCard - Reusable provider selection card
 * Used by: ApiConfig.tsx, Setup/StepProvider.tsx
 */

import { CheckCircle2 } from 'lucide-react'
import type { ProviderDef } from '@/lib/providers'
import { getProviderIcon } from '@/lib/providers'
import { cn } from '@/lib/utils'

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

  return (
    <button
      onClick={onClick}
      className={cn(
        'relative flex cursor-pointer flex-col items-center justify-center rounded-xl border transition-all',
        size === 'sm' ? 'gap-1 p-2.5' : 'gap-2 p-4',
        !isSelected &&
          'border-secondary bg-primary hover:border-primary hover:bg-primary',
      )}
      style={isSelected ? {
        borderColor: `${color}80`,
        backgroundColor: `${color}15`,
      } : undefined}
      title={provider.description}
    >
      {isSelected && (
        <div className="absolute right-2 top-2">
          <CheckCircle2 className="h-4 w-4" style={{ color }} />
        </div>
      )}

      <div
        className={cn(
          'flex items-center justify-center rounded-lg',
          size === 'sm' ? 'h-8 w-8' : 'h-10 w-10',
        )}
        style={{ backgroundColor: `${color}20` }}
      >
        <div style={{ color }}>
          {icon}
        </div>
      </div>

      <span
        className={cn(
          'font-medium',
          size === 'sm' ? 'text-[10px]' : 'text-sm',
          isSelected ? 'text-primary' : 'text-secondary',
        )}
      >
        {provider.label}
      </span>

      {showDescription && size === 'md' && (
        <span className="line-clamp-1 text-center text-[10px] text-tertiary">
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
      className={cn(
        'flex cursor-pointer flex-col items-center gap-1 rounded-xl border px-2 py-2.5 text-center transition-all',
        !isSelected &&
          'border-secondary bg-primary hover:border-primary hover:bg-primary',
      )}
      style={isSelected ? {
        borderColor: `${color}80`,
        backgroundColor: `${color}15`,
      } : undefined}
      title={provider.description}
    >
      <div
        className={cn(!isSelected && 'text-tertiary')}
        style={isSelected ? { color } : undefined}
      >
        {icon}
      </div>
      <span className={cn(
        'text-[10px] font-medium',
        isSelected ? 'text-primary' : 'text-secondary',
      )}>
        {provider.label}
      </span>
    </button>
  )
}

export default ProviderCard

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
          ? `border-[${color}]/50 bg-[${color}]/10`
          : 'border-white/10 bg-[#0c1222] hover:border-white/20 hover:bg-[#111827]'
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
        className={`font-medium ${size === 'sm' ? 'text-[10px]' : 'text-sm'} ${isSelected ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}
      >
        {provider.label}
      </span>

      {showDescription && size === 'md' && (
        <span className="text-[10px] text-[#64748b] text-center line-clamp-1">
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
          ? `border-[${color}]/50 bg-[${color}]/10`
          : 'border-white/10 bg-[#0c1222] hover:border-white/20 hover:bg-[#111827]'
      }`}
      style={isSelected ? {
        borderColor: `${color}80`,
        backgroundColor: `${color}15`,
      } : undefined}
      title={provider.description}
    >
      <div className={isSelected ? `text-[${color}]` : 'text-[#64748b]'} style={isSelected ? { color } : undefined}>
        {icon}
      </div>
      <span className={`text-[10px] font-medium ${isSelected ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}>
        {provider.label}
      </span>
    </button>
  )
}

export default ProviderCard

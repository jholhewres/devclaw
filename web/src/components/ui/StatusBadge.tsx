import { cn } from '@/lib/utils'

type Status = 'online' | 'offline' | 'error' | 'warning' | 'info'

interface StatusBadgeProps {
  status: Status
  label?: string
  pulse?: boolean
  className?: string
}

const statusConfig = {
  online: { dot: 'bg-[#10b981]', text: 'text-[#10b981]', bg: 'bg-[#10b981]/10', defaultLabel: 'Online' },
  offline: { dot: 'bg-[#64748b]', text: 'text-[#64748b]', bg: 'bg-[#64748b]/10', defaultLabel: 'Offline' },
  error: { dot: 'bg-[#f43f5e]', text: 'text-[#f43f5e]', bg: 'bg-[#f43f5e]/10', defaultLabel: 'Error' },
  warning: { dot: 'bg-[#f59e0b]', text: 'text-[#f59e0b]', bg: 'bg-[#f59e0b]/10', defaultLabel: 'Warning' },
  info: { dot: 'bg-[#6366f1]', text: 'text-[#818cf8]', bg: 'bg-[#6366f1]/10', defaultLabel: 'Info' },
}

export function StatusBadge({ status, label, pulse = false, className }: StatusBadgeProps) {
  const config = statusConfig[status]
  const displayLabel = label ?? config.defaultLabel

  return (
    <span className={cn('inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium', config.bg, config.text, className)}>
      <span className={cn('h-1.5 w-1.5 rounded-full', config.dot, pulse && 'animate-pulse')} />
      {displayLabel}
    </span>
  )
}

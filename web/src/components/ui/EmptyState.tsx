import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

interface EmptyStateProps {
  icon?: React.ElementType
  title: string
  description?: string
  action?: ReactNode
  className?: string
}

export function EmptyState({ icon: Icon, title, description, action, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center rounded-2xl border border-[rgba(99,102,241,0.1)] bg-[#14172b] p-12 text-center', className)}>
      {Icon && (
        <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-card">
          <Icon className="h-8 w-8 text-[#64748b]" />
        </div>
      )}
      <h3 className="text-sm font-medium text-[#94a3b8]">{title}</h3>
      {description && <p className="mt-1.5 text-xs text-[#64748b] max-w-xs">{description}</p>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  )
}

import type { HTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

const variants = {
  default: 'bg-[#1c1f3a] text-[#94a3b8]',
  brand: 'bg-[#6366f1]/10 text-[#818cf8]',
  success: 'bg-[#10b981]/10 text-[#10b981]',
  warning: 'bg-[#f59e0b]/10 text-[#f59e0b]',
  error: 'bg-[#f43f5e]/10 text-[#f43f5e]',
}

interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: keyof typeof variants
  dot?: boolean
  pulse?: boolean
}

export function Badge({ className, variant = 'default', dot = false, pulse = false, children, ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium',
        variants[variant],
        className,
      )}
      {...props}
    >
      {dot && (
        <span className={cn(
          'h-1.5 w-1.5 rounded-full bg-current',
          pulse && 'animate-pulse',
        )} />
      )}
      {children}
    </span>
  )
}

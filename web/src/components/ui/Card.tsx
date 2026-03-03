import { type HTMLAttributes, forwardRef } from 'react'
import { cn } from '@/lib/utils'

const variants = {
  default: 'bg-[#14172b] border-[rgba(99,102,241,0.12)]',
  glass: 'glass',
  outlined: 'bg-transparent border-[rgba(99,102,241,0.15)]',
  interactive: 'bg-[#14172b] border-[rgba(99,102,241,0.12)] hover:border-[rgba(99,102,241,0.24)] hover:bg-[#1c1f3a] cursor-pointer transition-all duration-200 hover:-translate-y-0.5',
  gradient: 'bg-gradient-card border-[rgba(99,102,241,0.15)]',
}

interface CardProps extends HTMLAttributes<HTMLDivElement> {
  variant?: keyof typeof variants
  glow?: boolean
  padding?: 'none' | 'sm' | 'md' | 'lg'
}

const paddings = {
  none: '',
  sm: 'p-4',
  md: 'p-6',
  lg: 'p-8',
}

export const Card = forwardRef<HTMLDivElement, CardProps>(
  ({ className, variant = 'default', glow = false, padding = 'md', ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        'rounded-2xl border',
        variants[variant],
        paddings[padding],
        glow && 'glow-brand-sm',
        className,
      )}
      {...props}
    />
  ),
)
Card.displayName = 'Card'

import { type ButtonHTMLAttributes, forwardRef } from 'react'
import { cn } from '@/lib/utils'

const variants = {
  default: 'bg-[#1c1f3a] text-[#f1f5f9] hover:bg-[#242850] border border-[rgba(99,102,241,0.12)] hover:border-[rgba(99,102,241,0.24)]',
  primary: 'bg-[#6366f1] text-white hover:bg-[#818cf8]',
  gradient: 'bg-gradient-brand text-white hover:opacity-90 shadow-md hover:shadow-lg',
  secondary: 'bg-[#14172b] text-[#94a3b8] hover:text-[#f1f5f9] hover:bg-[#1c1f3a] border border-[rgba(99,102,241,0.1)]',
  ghost: 'text-[#94a3b8] hover:bg-white/5 hover:text-[#f1f5f9]',
  destructive: 'bg-[#f43f5e] text-white hover:bg-[#e11d48]',
  outline: 'border border-[rgba(99,102,241,0.15)] text-[#94a3b8] hover:bg-[rgba(99,102,241,0.08)] hover:text-[#f1f5f9] hover:border-[rgba(99,102,241,0.3)]',
}

const sizes = {
  sm: 'h-8 px-3 text-xs',
  md: 'h-9 px-4 text-sm',
  lg: 'h-10 px-6 text-sm',
  xl: 'h-11 px-8 text-sm',
  icon: 'h-9 w-9',
}

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof variants
  size?: keyof typeof sizes
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'default', size = 'md', ...props }, ref) => (
    <button
      ref={ref}
      className={cn(
        'inline-flex items-center justify-center gap-2 rounded-xl font-medium',
        'transition-all duration-200 cursor-pointer',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#6366f1]/40',
        'disabled:opacity-50 disabled:pointer-events-none',
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    />
  ),
)
Button.displayName = 'Button'

import { type InputHTMLAttributes, forwardRef, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  icon?: ReactNode
  suffix?: ReactNode
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, icon, suffix, ...props }, ref) => (
    <div className="relative">
      {icon && (
        <div className="absolute left-3 top-1/2 -translate-y-1/2 text-[#64748b]">
          {icon}
        </div>
      )}
      <input
        ref={ref}
        className={cn(
          'flex h-10 w-full rounded-xl border border-[rgba(99,102,241,0.12)] bg-[#14172b] px-3.5 py-1 text-sm text-[#f1f5f9]',
          'placeholder:text-[#475569]',
          'focus-visible:outline-none focus-visible:border-[#6366f1]/50 focus-visible:ring-1 focus-visible:ring-[#6366f1]/20',
          'hover:border-[rgba(99,102,241,0.24)]',
          'disabled:cursor-not-allowed disabled:opacity-50',
          'transition-all duration-200',
          icon && 'pl-10',
          suffix && 'pr-10',
          className,
        )}
        {...props}
      />
      {suffix && (
        <div className="absolute right-3 top-1/2 -translate-y-1/2 text-[#64748b]">
          {suffix}
        </div>
      )}
    </div>
  ),
)
Input.displayName = 'Input'

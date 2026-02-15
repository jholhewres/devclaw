import { type InputHTMLAttributes, forwardRef } from 'react'
import { cn } from '@/lib/utils'

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        'flex h-9 w-full rounded-lg border border-zinc-200 bg-white px-3 py-1 text-sm',
        'placeholder:text-zinc-400 dark:border-zinc-700 dark:bg-zinc-900',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400',
        'disabled:cursor-not-allowed disabled:opacity-50',
        'transition-colors',
        className,
      )}
      {...props}
    />
  ),
)
Input.displayName = 'Input'

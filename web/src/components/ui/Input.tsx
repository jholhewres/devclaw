import { type InputHTMLAttributes, forwardRef } from 'react';
import { cn } from '@/lib/utils';

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        'flex h-9 w-full rounded-lg border border-border bg-bg-surface px-3 py-1 text-sm text-text-primary',
        'placeholder:text-text-muted',
        'focus-visible:ring-2 focus-visible:ring-brand/40 focus-visible:outline-none',
        'disabled:cursor-not-allowed disabled:opacity-50',
        'transition-colors',
        className
      )}
      {...props}
    />
  )
);
Input.displayName = 'Input';

import { type ButtonHTMLAttributes, forwardRef } from 'react';
import { cn } from '@/lib/utils';

const variants = {
  default: 'bg-brand text-white hover:bg-brand-hover',
  secondary: 'bg-bg-elevated text-text-primary hover:bg-bg-active',
  ghost: 'hover:bg-bg-hover text-text-secondary hover:text-text-primary',
  destructive: 'bg-error text-white hover:bg-red-600',
  outline: 'border border-border hover:bg-bg-hover hover:border-border-hover',
};

const sizes = {
  sm: 'h-8 px-3 text-xs',
  md: 'h-9 px-4 text-sm',
  lg: 'h-10 px-6 text-sm',
  icon: 'h-9 w-9',
};

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof variants;
  size?: keyof typeof sizes;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'default', size = 'md', ...props }, ref) => (
    <button
      ref={ref}
      className={cn(
        'inline-flex items-center justify-center gap-2 rounded-lg font-medium',
        'cursor-pointer transition-colors duration-150',
        'focus-visible:ring-2 focus-visible:ring-brand/40 focus-visible:outline-none',
        'disabled:pointer-events-none disabled:opacity-50',
        variants[variant],
        sizes[size],
        className
      )}
      {...props}
    />
  )
);
Button.displayName = 'Button';

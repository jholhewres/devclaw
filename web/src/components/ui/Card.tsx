import { type HTMLAttributes, forwardRef } from 'react';
import { cn } from '@/lib/utils';

const paddingSizes = {
  sm: 'p-3',
  md: 'p-4',
  lg: 'p-6',
};

interface CardProps extends HTMLAttributes<HTMLDivElement> {
  /** Add hover effect with border highlight and subtle lift */
  interactive?: boolean;
  /** Inner padding size */
  padding?: keyof typeof paddingSizes;
}

export const Card = forwardRef<HTMLDivElement, CardProps>(
  ({ className, interactive = false, padding = 'md', children, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        'rounded-lg border border-border bg-bg-surface',
        paddingSizes[padding],
        interactive && [
          'cursor-pointer',
          'transition-all duration-200 ease-out',
          'hover:border-border-hover hover:-translate-y-0.5 hover:shadow-md',
          'active:translate-y-0 active:shadow-sm',
        ],
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
);
Card.displayName = 'Card';

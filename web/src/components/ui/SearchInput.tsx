import { type InputHTMLAttributes, forwardRef } from 'react';
import { Search, X } from 'lucide-react';
import { cn } from '@/lib/utils';

interface SearchInputProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, 'onChange' | 'type'> {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
}

export const SearchInput = forwardRef<HTMLInputElement, SearchInputProps>(
  ({ value, onChange, placeholder = 'Search...', className, ...props }, ref) => (
    <div className={cn('relative', className)}>
      {/* Search icon */}
      <Search
        className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted"
        aria-hidden="true"
      />

      <input
        ref={ref}
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={cn(
          'h-9 w-full rounded-lg border border-border bg-bg-surface',
          'pl-9 pr-8 text-sm text-text-primary',
          'placeholder:text-text-muted',
          'transition-colors duration-150',
          'hover:border-border-hover',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:border-border-focus'
        )}
        {...props}
      />

      {/* Clear button */}
      {value && (
        <button
          type="button"
          onClick={() => onChange('')}
          className={cn(
            'absolute right-2 top-1/2 -translate-y-1/2',
            'flex h-5 w-5 items-center justify-center rounded',
            'text-text-muted hover:text-text-primary',
            'hover:bg-bg-hover active:bg-bg-active',
            'transition-colors duration-150',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand'
          )}
          aria-label="Clear search"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  )
);
SearchInput.displayName = 'SearchInput';

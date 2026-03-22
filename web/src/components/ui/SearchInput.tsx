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
        className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-fg-quaternary"
        aria-hidden="true"
      />

      <input
        ref={ref}
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={cn(
          'h-11 w-full rounded-lg border border-primary bg-primary',
          'pl-10 pr-9 text-sm text-primary shadow-xs',
          'transition-all outline-none placeholder:text-quaternary',
          'hover:border-primary_hover',
          'focus:border-brand focus:ring-1 focus:ring-brand'
        )}
        {...props}
      />

      {/* Clear button */}
      {value && (
        <button
          type="button"
          onClick={() => onChange('')}
          className={cn(
            'absolute right-2.5 top-1/2 -translate-y-1/2',
            'flex h-5 w-5 items-center justify-center rounded',
            'text-fg-quaternary hover:text-fg-secondary',
            'hover:bg-primary_hover active:bg-active',
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

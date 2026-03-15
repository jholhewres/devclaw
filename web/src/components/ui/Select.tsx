import { type SelectHTMLAttributes, forwardRef, useId } from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';

interface SelectOption {
  value: string;
  label: string;
}

interface SelectProps
  extends Omit<SelectHTMLAttributes<HTMLSelectElement>, 'onChange'> {
  label?: string;
  value?: string;
  onChange?: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  hint?: string;
  error?: string;
  disabled?: boolean;
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  (
    {
      label,
      value,
      onChange,
      options,
      placeholder,
      hint,
      error,
      disabled = false,
      className,
      ...props
    },
    ref
  ) => {
    const id = useId();

    return (
      <div className={cn('flex flex-col gap-1.5', className)}>
        {label && (
          <label
            htmlFor={id}
            className="text-sm font-medium text-text-primary"
          >
            {label}
          </label>
        )}

        <div className="relative">
          <select
            ref={ref}
            id={id}
            value={value}
            disabled={disabled}
            onChange={(e) => onChange?.(e.target.value)}
            className={cn(
              'h-11 w-full cursor-pointer appearance-none rounded-xl border bg-bg-surface px-4 pr-10 text-sm',
              'text-text-primary placeholder:text-text-muted',
              'transition-all outline-none',
              'hover:border-border-hover',
              'focus:border-brand/50 focus:ring-1 focus:ring-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
              error
                ? 'border-error focus:ring-error'
                : 'border-border',
              !value && placeholder && 'text-text-muted'
            )}
            {...props}
          >
            {placeholder && (
              <option value="" disabled>
                {placeholder}
              </option>
            )}
            {options.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>

          {/* Chevron */}
          <ChevronDown
            className={cn(
              'pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2',
              'text-text-muted',
              disabled && 'opacity-50'
            )}
          />
        </div>

        {/* Hint or error */}
        {(hint || error) && (
          <p
            className={cn(
              'text-xs',
              error ? 'text-error' : 'text-text-muted'
            )}
          >
            {error || hint}
          </p>
        )}
      </div>
    );
  }
);
Select.displayName = 'Select';

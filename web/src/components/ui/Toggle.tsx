import { type InputHTMLAttributes, useId } from 'react';
import { cn } from '@/lib/utils';

const sizeConfig = {
  sm: {
    track: 'h-5 w-9',
    knob: 'h-3.5 w-3.5',
    translate: 'translate-x-4',
    offset: 'translate-x-0.5',
  },
  md: {
    track: 'h-6 w-11',
    knob: 'h-4.5 w-4.5',
    translate: 'translate-x-5',
    offset: 'translate-x-0.5',
  },
};

interface ToggleProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, 'onChange' | 'size' | 'type'> {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label?: string;
  description?: string;
  disabled?: boolean;
  size?: keyof typeof sizeConfig;
}

export function Toggle({
  checked,
  onChange,
  label,
  description,
  disabled = false,
  size = 'md',
  className,
  ...props
}: ToggleProps) {
  const id = useId();
  const config = sizeConfig[size];

  return (
    <label
      htmlFor={id}
      className={cn(
        'inline-flex items-start gap-3',
        disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer',
        className
      )}
    >
      {/* Hidden checkbox for accessibility */}
      <input
        id={id}
        type="checkbox"
        role="switch"
        aria-checked={checked}
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
        className="sr-only peer"
        {...props}
      />

      {/* Track */}
      <span
        aria-hidden="true"
        className={cn(
          'relative inline-flex flex-shrink-0 items-center rounded-full',
          'transition-colors duration-200 ease-in-out',
          config.track,
          checked ? 'bg-brand' : 'bg-bg-subtle',
          !disabled && !checked && 'hover:bg-bg-elevated',
          !disabled && checked && 'hover:bg-brand-hover'
        )}
      >
        {/* Knob */}
        <span
          className={cn(
            'inline-block rounded-full bg-white shadow-sm',
            'transition-transform duration-200 ease-in-out',
            config.knob,
            checked ? config.translate : config.offset
          )}
        />
      </span>

      {/* Label + description */}
      {(label || description) && (
        <span className="flex flex-col pt-0.5">
          {label && (
            <span className="text-sm font-medium text-text-primary leading-tight">
              {label}
            </span>
          )}
          {description && (
            <span className="mt-0.5 text-xs text-text-secondary leading-snug">
              {description}
            </span>
          )}
        </span>
      )}
    </label>
  );
}

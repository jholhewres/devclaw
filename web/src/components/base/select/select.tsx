import { type SelectHTMLAttributes, useId } from 'react';
import { ChevronDown } from 'lucide-react';
import { HintText } from '@/components/base/input/hint-text';
import { Label } from '@/components/base/input/label';
import { cx } from '@/utils/cx';

interface NativeSelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  hint?: string;
  error?: string;
  selectClassName?: string;
  options: { label: string; value: string; disabled?: boolean }[];
}

export const NativeSelect = ({
  label,
  hint,
  error,
  options,
  className,
  selectClassName,
  ...props
}: NativeSelectProps) => {
  const id = useId();
  const selectId = `select-native-${id}`;
  const hintId = `select-native-hint-${id}`;
  const isInvalid = !!error;

  return (
    <div className={cx('w-full', className)}>
      {label && (
        <Label htmlFor={selectId} id={selectId} className="mb-1.5">
          {label}
        </Label>
      )}

      <div className="relative grid w-full items-center">
        <select
          {...props}
          id={selectId}
          aria-describedby={hintId}
          aria-labelledby={selectId}
          aria-invalid={isInvalid || undefined}
          className={cx(
            'bg-primary text-md text-primary ring-primary placeholder:text-fg-quaternary focus-visible:ring-brand disabled:bg-disabled_subtle disabled:text-disabled appearance-none rounded-lg px-3.5 py-2.5 font-medium shadow-xs ring-1 outline-hidden transition duration-100 ease-linear ring-inset focus-visible:ring-2 disabled:cursor-not-allowed',
            isInvalid && 'ring-error_subtle focus-visible:ring-error',
            selectClassName
          )}
        >
          {options.map((opt) => (
            <option key={opt.value} value={opt.value} disabled={opt.disabled}>
              {opt.label}
            </option>
          ))}
        </select>
        <ChevronDown
          aria-hidden="true"
          className="text-fg-quaternary pointer-events-none absolute right-3.5 size-5"
        />
      </div>

      {(hint || error) && (
        <HintText className="mt-1.5" id={hintId} isInvalid={isInvalid}>
          {error || hint}
        </HintText>
      )}
    </div>
  );
};

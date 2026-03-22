import type { ReactNode, Ref } from 'react';
import type { TextProps as AriaTextProps } from 'react-aria-components';
import { Text as AriaText } from 'react-aria-components';
import { cx } from '@/utils/cx';

interface HintTextProps extends AriaTextProps {
  isInvalid?: boolean;
  ref?: Ref<HTMLElement>;
  children: ReactNode;
}

export const HintText = ({ isInvalid, className, ...props }: HintTextProps) => {
  return (
    <AriaText
      {...props}
      slot={isInvalid ? 'errorMessage' : 'description'}
      className={cx(
        'text-tertiary text-sm',
        isInvalid && 'text-error-primary',
        'group-invalid:text-error-primary',
        className
      )}
    />
  );
};

HintText.displayName = 'HintText';

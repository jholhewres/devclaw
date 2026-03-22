import type { ReactNode, Ref } from 'react';
import React from 'react';
import type {
  TextAreaProps as AriaTextAreaProps,
  TextFieldProps as AriaTextFieldProps,
} from 'react-aria-components';
import { TextArea as AriaTextArea, TextField as AriaTextField } from 'react-aria-components';
import { HintText } from '@/components/base/input/hint-text';
import { Label } from '@/components/base/input/label';
import { cx } from '@/utils/cx';

const getResizeHandleBg = (color: string) => {
  return `url(data:image/svg+xml;base64,${btoa(`<svg width="18" height="18" viewBox="0 0 18 18" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M10 2L2 10" stroke="${color}" stroke-linecap="round"/><path d="M11 7L7 11" stroke="${color}" stroke-linecap="round"/></svg>`)})`;
};

interface TextAreaBaseProps extends AriaTextAreaProps {
  ref?: Ref<HTMLTextAreaElement>;
}

export const TextAreaBase = ({ className, ...props }: TextAreaBaseProps) => {
  return (
    <AriaTextArea
      {...props}
      style={
        {
          '--resize-handle-bg': getResizeHandleBg('#D5D7DA'),
          '--resize-handle-bg-dark': getResizeHandleBg('#373A41'),
        } as React.CSSProperties
      }
      className={(state) =>
        cx(
          'bg-primary text-md text-primary ring-primary placeholder:text-placeholder autofill:text-primary w-full scroll-py-3 rounded-lg px-3.5 py-3 shadow-xs ring-1 transition duration-100 ease-linear ring-inset autofill:rounded-lg focus:outline-hidden',
          '[&::-webkit-resizer]:bg-(image:--resize-handle-bg) [&::-webkit-resizer]:bg-contain dark:[&::-webkit-resizer]:bg-(image:--resize-handle-bg-dark)',
          state.isFocused && !state.isDisabled && 'ring-brand ring-2',
          state.isDisabled && 'bg-disabled_subtle text-disabled ring-disabled cursor-not-allowed',
          state.isInvalid && 'ring-error_subtle',
          state.isInvalid && state.isFocused && 'ring-error ring-2',
          typeof className === 'function' ? className(state) : className
        )
      }
    />
  );
};

TextAreaBase.displayName = 'TextAreaBase';

interface TextFieldProps extends AriaTextFieldProps {
  label?: string;
  hint?: ReactNode;
  tooltip?: string;
  textAreaClassName?: TextAreaBaseProps['className'];
  ref?: Ref<HTMLDivElement>;
  textAreaRef?: TextAreaBaseProps['ref'];
  hideRequiredIndicator?: boolean;
  placeholder?: string;
  rows?: number;
  cols?: number;
}

export const TextArea = ({
  label,
  hint,
  tooltip,
  textAreaRef,
  hideRequiredIndicator,
  textAreaClassName,
  placeholder,
  className,
  rows,
  cols,
  ...props
}: TextFieldProps) => {
  return (
    <AriaTextField
      {...props}
      className={(state) =>
        cx(
          'group flex h-max w-full flex-col items-start justify-start gap-1.5',
          typeof className === 'function' ? className(state) : className
        )
      }
    >
      {({ isInvalid, isRequired }) => (
        <>
          {label && (
            <Label
              isRequired={hideRequiredIndicator ? !hideRequiredIndicator : isRequired}
              tooltip={tooltip}
            >
              {label}
            </Label>
          )}

          <TextAreaBase
            placeholder={placeholder}
            className={textAreaClassName}
            ref={textAreaRef}
            rows={rows}
            cols={cols}
          />

          {hint && <HintText isInvalid={isInvalid}>{hint}</HintText>}
        </>
      )}
    </AriaTextField>
  );
};

TextArea.displayName = 'TextArea';

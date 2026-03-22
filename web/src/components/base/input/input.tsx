import {
  type ComponentType,
  type HTMLAttributes,
  type ReactNode,
  type Ref,
  createContext,
  useContext,
} from 'react';
import { HelpCircle, AlertCircle } from 'lucide-react';
import type {
  InputProps as AriaInputProps,
  TextFieldProps as AriaTextFieldProps,
} from 'react-aria-components';
import {
  Group as AriaGroup,
  Input as AriaInput,
  TextField as AriaTextField,
} from 'react-aria-components';
import { HintText } from '@/components/base/input/hint-text';
import { Label } from '@/components/base/input/label';
import { Tooltip, TooltipTrigger } from '@/components/base/tooltip/tooltip';
import { cx, sortCx } from '@/utils/cx';

export interface InputBaseProps extends TextFieldProps {
  tooltip?: string;
  size?: 'sm' | 'md';
  placeholder?: string;
  iconClassName?: string;
  inputClassName?: string;
  wrapperClassName?: string;
  tooltipClassName?: string;
  shortcut?: string | boolean;
  ref?: Ref<HTMLInputElement>;
  groupRef?: Ref<HTMLDivElement>;
  icon?: ComponentType<HTMLAttributes<HTMLOrSVGElement>>;
}

export const InputBase = ({
  ref,
  tooltip,
  shortcut,
  groupRef,
  size = 'sm',
  isInvalid,
  isDisabled,
  icon: Icon,
  placeholder,
  wrapperClassName,
  tooltipClassName,
  inputClassName,
  iconClassName,
  isRequired: _isRequired,
  ...inputProps
}: Omit<InputBaseProps, 'label' | 'hint'>) => {
  const hasTrailingIcon = tooltip || isInvalid;
  const hasLeadingIcon = Icon;

  const context = useContext(TextFieldContext);
  const inputSize = context?.size || size;

  const sizes = sortCx({
    sm: {
      root: cx('px-3 py-2 lg:py-3', hasTrailingIcon && 'pr-9', hasLeadingIcon && 'pl-10'),
      iconLeading: 'left-3',
      iconTrailing: 'right-3',
      shortcut: 'pr-2.5',
    },
    md: {
      root: cx('px-3.5 py-2.5 lg:py-3', hasTrailingIcon && 'pr-9.5', hasLeadingIcon && 'pl-10.5'),
      iconLeading: 'left-3.5',
      iconTrailing: 'right-3.5',
      shortcut: 'pr-3',
    },
  });

  return (
    <AriaGroup
      {...{ isDisabled, isInvalid }}
      ref={groupRef}
      className={({ isFocusWithin, isDisabled, isInvalid }) =>
        cx(
          'bg-primary ring-primary relative flex w-full flex-row place-content-center place-items-center rounded-lg shadow-xs ring-1 transition-shadow duration-100 ease-linear ring-inset',
          isFocusWithin && !isDisabled && 'ring-brand ring-2',
          isDisabled && 'bg-disabled_subtle ring-disabled cursor-not-allowed',
          'group-disabled:bg-disabled_subtle group-disabled:ring-disabled group-disabled:cursor-not-allowed',
          isInvalid && 'ring-error_subtle',
          'group-invalid:ring-error_subtle',
          isInvalid && isFocusWithin && 'ring-error ring-2',
          isFocusWithin && 'group-invalid:ring-error group-invalid:ring-2',
          context?.wrapperClassName,
          wrapperClassName
        )
      }
    >
      {Icon && (
        <Icon
          className={cx(
            'text-fg-quaternary pointer-events-none absolute size-5',
            isDisabled && 'text-fg-disabled',
            sizes[inputSize].iconLeading,
            context?.iconClassName,
            iconClassName
          )}
        />
      )}

      <AriaInput
        {...(inputProps as AriaInputProps)}
        ref={ref}
        placeholder={placeholder}
        className={cx(
          'text-md text-primary placeholder:text-placeholder autofill:text-primary m-0 w-full bg-transparent ring-0 outline-hidden autofill:rounded-lg',
          isDisabled && 'text-disabled cursor-not-allowed',
          sizes[inputSize].root,
          context?.inputClassName,
          inputClassName
        )}
      />

      {tooltip && !isInvalid && (
        <Tooltip title={tooltip} placement="top">
          <TooltipTrigger
            className={cx(
              'text-fg-quaternary hover:text-fg-quaternary_hover focus:text-fg-quaternary_hover absolute cursor-pointer transition duration-200',
              sizes[inputSize].iconTrailing,
              context?.tooltipClassName,
              tooltipClassName
            )}
          >
            <HelpCircle className="size-4" />
          </TooltipTrigger>
        </Tooltip>
      )}

      {isInvalid && (
        <AlertCircle
          className={cx(
            'text-fg-error-secondary pointer-events-none absolute size-4',
            sizes[inputSize].iconTrailing,
            context?.tooltipClassName,
            tooltipClassName
          )}
        />
      )}

      {shortcut && (
        <div
          className={cx(
            'to-bg-primary pointer-events-none absolute inset-y-0.5 right-0.5 z-10 flex items-center rounded-r-[inherit] bg-linear-to-r from-transparent to-40% pl-8',
            sizes[inputSize].shortcut
          )}
        >
          <span
            className={cx(
              'text-quaternary ring-secondary pointer-events-none rounded px-1 py-px text-xs font-medium ring-1 select-none ring-inset',
              isDisabled && 'text-disabled bg-transparent'
            )}
            aria-hidden="true"
          >
            {typeof shortcut === 'string' ? shortcut : '⌘K'}
          </span>
        </div>
      )}
    </AriaGroup>
  );
};

InputBase.displayName = 'InputBase';

interface BaseProps {
  label?: string;
  hint?: ReactNode;
}

interface TextFieldProps
  extends
    BaseProps,
    AriaTextFieldProps,
    Pick<
      InputBaseProps,
      'size' | 'wrapperClassName' | 'inputClassName' | 'iconClassName' | 'tooltipClassName'
    > {
  ref?: Ref<HTMLDivElement>;
}

const TextFieldContext = createContext<TextFieldProps>({});

export const TextField = ({ className, ...props }: TextFieldProps) => {
  return (
    <TextFieldContext.Provider value={props}>
      <AriaTextField
        {...props}
        data-input-wrapper
        className={(state) =>
          cx(
            'group flex h-max w-full flex-col items-start justify-start gap-1.5',
            typeof className === 'function' ? className(state) : className
          )
        }
      />
    </TextFieldContext.Provider>
  );
};

TextField.displayName = 'TextField';

interface InputProps extends InputBaseProps, BaseProps {
  hideRequiredIndicator?: boolean;
}

export const Input = ({
  size = 'sm',
  placeholder,
  icon: Icon,
  label,
  hint,
  shortcut,
  hideRequiredIndicator,
  className,
  ref,
  groupRef,
  tooltip,
  iconClassName,
  inputClassName,
  wrapperClassName,
  tooltipClassName,
  ...props
}: InputProps) => {
  return (
    <TextField aria-label={!label ? placeholder : undefined} {...props} className={className}>
      {({ isRequired, isInvalid }) => (
        <>
          {label && (
            <Label isRequired={hideRequiredIndicator ? !hideRequiredIndicator : isRequired}>
              {label}
            </Label>
          )}

          <InputBase
            {...{
              ref,
              groupRef,
              size,
              placeholder,
              icon: Icon,
              shortcut,
              iconClassName,
              inputClassName,
              wrapperClassName,
              tooltipClassName,
              tooltip,
            }}
          />

          {hint && <HintText isInvalid={isInvalid}>{hint}</HintText>}
        </>
      )}
    </TextField>
  );
};

Input.displayName = 'Input';

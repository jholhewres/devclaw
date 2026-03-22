import type { ReactNode } from 'react';
import type { SwitchProps as AriaSwitchProps } from 'react-aria-components';
import { Switch as AriaSwitch } from 'react-aria-components';
import { cx } from '@/utils/cx';

interface ToggleBaseProps {
  size?: 'sm' | 'md';
  className?: string;
  isHovered?: boolean;
  isFocusVisible?: boolean;
  isSelected?: boolean;
  isDisabled?: boolean;
}

export const ToggleBase = ({
  className,
  isHovered,
  isDisabled,
  isFocusVisible,
  isSelected,
  size = 'sm',
}: ToggleBaseProps) => {
  const styles = {
    sm: {
      root: 'h-5 w-9 p-0.5',
      switch: cx('size-4', isSelected && 'translate-x-4'),
    },
    md: {
      root: 'h-6 w-11 p-0.5',
      switch: cx('size-5', isSelected && 'translate-x-5'),
    },
  };

  const classes = styles[size];

  return (
    <div
      className={cx(
        'bg-tertiary outline-brand cursor-pointer rounded-full transition duration-150 ease-linear',
        isSelected && 'bg-brand-solid',
        isSelected && isHovered && 'bg-brand-solid_hover',
        isDisabled && 'bg-disabled cursor-not-allowed',
        isFocusVisible && 'outline-2 outline-offset-2',
        classes.root,
        className
      )}
    >
      <div
        style={{
          transition:
            'transform 0.15s ease-in-out, translate 0.15s ease-in-out, border-color 0.1s linear, background-color 0.1s linear',
        }}
        className={cx(
          'bg-fg-white rounded-full shadow-sm',
          isDisabled && 'bg-toggle-button-fg_disabled',
          classes.switch
        )}
      />
    </div>
  );
};

interface ToggleProps extends AriaSwitchProps {
  size?: 'sm' | 'md';
  label?: string;
  hint?: ReactNode;
}

export const Toggle = ({
  label,
  hint,
  className,
  size = 'sm',
  ...ariaSwitchProps
}: ToggleProps) => {
  const sizes = {
    sm: {
      root: 'gap-2',
      textWrapper: '',
      label: 'text-sm font-medium',
      hint: 'text-sm',
    },
    md: {
      root: 'gap-3',
      textWrapper: 'gap-0.5',
      label: 'text-md font-medium',
      hint: 'text-md',
    },
  };

  return (
    <AriaSwitch
      {...ariaSwitchProps}
      className={(renderProps) =>
        cx(
          'flex w-max items-start',
          renderProps.isDisabled && 'cursor-not-allowed',
          sizes[size].root,
          typeof className === 'function' ? className(renderProps) : className
        )
      }
    >
      {({ isSelected, isDisabled, isFocusVisible, isHovered }) => (
        <>
          <ToggleBase
            size={size}
            isHovered={isHovered}
            isDisabled={isDisabled}
            isFocusVisible={isFocusVisible}
            isSelected={isSelected}
          />

          {(label || hint) && (
            <div className={cx('flex flex-col', sizes[size].textWrapper)}>
              {label && (
                <p className={cx('text-secondary select-none', sizes[size].label)}>{label}</p>
              )}
              {hint && (
                <span
                  className={cx('text-tertiary', sizes[size].hint)}
                  onClick={(event) => event.stopPropagation()}
                >
                  {hint}
                </span>
              )}
            </div>
          )}
        </>
      )}
    </AriaSwitch>
  );
};

import type { FC, RefAttributes } from 'react';
import { MoreHorizontal, MoreVertical } from 'lucide-react';
import type {
  ButtonProps as AriaButtonProps,
  MenuItemProps as AriaMenuItemProps,
  MenuProps as AriaMenuProps,
  PopoverProps as AriaPopoverProps,
  SeparatorProps as AriaSeparatorProps,
} from 'react-aria-components';
import {
  Button as AriaButton,
  Header as AriaHeader,
  Menu as AriaMenu,
  MenuItem as AriaMenuItem,
  MenuSection as AriaMenuSection,
  MenuTrigger as AriaMenuTrigger,
  Popover as AriaPopover,
  Separator as AriaSeparator,
} from 'react-aria-components';
import { cx } from '@/utils/cx';

interface DropdownItemProps extends AriaMenuItemProps {
  label?: string;
  addon?: string;
  unstyled?: boolean;
  icon?: FC<{ className?: string }>;
}

const DropdownItem = ({
  label,
  children,
  addon,
  icon: Icon,
  unstyled,
  ...props
}: DropdownItemProps) => {
  if (unstyled) {
    return (
      <AriaMenuItem id={label} textValue={label} {...props} className="outline-hidden cursor-default">
        {children}
      </AriaMenuItem>
    );
  }

  return (
    <AriaMenuItem
      {...props}
      className={(state) =>
        cx(
          'group block cursor-pointer px-1.5 py-px outline-hidden',
          state.isDisabled && 'cursor-not-allowed',
          typeof props.className === 'function' ? props.className(state) : props.className
        )
      }
    >
      {(state) => (
        <div
          className={cx(
            'relative flex items-center rounded-md px-2.5 py-2 transition duration-100 ease-linear',
            !state.isDisabled && 'group-hover:bg-primary_hover',
            state.isFocused && 'bg-primary_hover',
            state.isFocusVisible && 'outline-brand outline-2 -outline-offset-2'
          )}
        >
          {Icon && (
            <Icon
              aria-hidden="true"
              className={cx(
                'mr-2 size-4 shrink-0 stroke-[2.25px]',
                state.isDisabled ? 'text-fg-disabled' : 'text-fg-quaternary'
              )}
            />
          )}

          <span
            className={cx(
              'grow truncate text-sm font-semibold transition-colors',
              state.isDisabled
                ? 'text-disabled'
                : 'text-secondary group-hover:text-secondary_hover',
              state.isFocused && 'text-secondary_hover'
            )}
          >
            {label || (typeof children === 'function' ? children(state) : children)}
          </span>

          {addon && (
            <span
              className={cx(
                'ring-secondary ml-3 shrink-0 rounded px-1 py-px text-xs font-medium ring-1 ring-inset',
                state.isDisabled ? 'text-disabled' : 'text-quaternary'
              )}
            >
              {addon}
            </span>
          )}
        </div>
      )}
    </AriaMenuItem>
  );
};

interface DropdownMenuProps<T extends object> extends AriaMenuProps<T> {}

const DropdownMenu = <T extends object>(props: DropdownMenuProps<T>) => {
  return (
    <AriaMenu
      {...props}
      className={(state) =>
        cx(
          'h-min overflow-y-auto py-1 outline-hidden select-none',
          typeof props.className === 'function' ? props.className(state) : props.className
        )
      }
    />
  );
};

interface DropdownPopoverProps extends AriaPopoverProps {}

const DropdownPopover = (props: DropdownPopoverProps) => {
  return (
    <AriaPopover
      placement="bottom right"
      {...props}
      className={(state) =>
        cx(
          'bg-primary ring-secondary_alt w-62 origin-(--trigger-anchor-point) overflow-auto rounded-lg shadow-lg ring-1 will-change-transform',
          state.isEntering &&
            'animate-in fade-in placement-right:slide-in-from-left-0.5 placement-top:slide-in-from-bottom-0.5 placement-bottom:slide-in-from-top-0.5 duration-150 ease-out',
          state.isExiting &&
            'animate-out fade-out placement-right:slide-out-to-left-0.5 placement-top:slide-out-to-bottom-0.5 placement-bottom:slide-out-to-top-0.5 duration-100 ease-in',
          typeof props.className === 'function' ? props.className(state) : props.className
        )
      }
    >
      {props.children}
    </AriaPopover>
  );
};

const DropdownSeparator = (props: AriaSeparatorProps) => {
  return (
    <AriaSeparator
      {...props}
      className={cx('bg-border-secondary my-1 h-px w-full', props.className)}
    />
  );
};

interface DropdownDotsButtonProps extends AriaButtonProps, RefAttributes<HTMLButtonElement> {
  variant?: 'horizontal' | 'vertical';
}

const DropdownDotsButton = ({ variant = 'horizontal', ...props }: DropdownDotsButtonProps) => {
  const Icon = variant === 'vertical' ? MoreVertical : MoreHorizontal;
  return (
    <AriaButton
      {...props}
      aria-label="Open menu"
      className={(state) =>
        cx(
          'text-fg-quaternary cursor-pointer rounded-md transition duration-100 ease-linear',
          (state.isPressed || state.isHovered) && 'text-fg-quaternary_hover',
          (state.isPressed || state.isFocusVisible) && 'outline-brand outline-2 outline-offset-2',
          typeof props.className === 'function' ? props.className(state) : props.className
        )
      }
    >
      <Icon className="transition-inherit-all size-5" />
    </AriaButton>
  );
};

export const Dropdown = {
  Root: AriaMenuTrigger,
  Popover: DropdownPopover,
  Menu: DropdownMenu,
  Section: AriaMenuSection,
  SectionHeader: AriaHeader,
  Item: DropdownItem,
  Separator: DropdownSeparator,
  DotsButton: DropdownDotsButton,
};

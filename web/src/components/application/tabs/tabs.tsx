import type { ComponentPropsWithRef, ReactNode } from 'react';
import { Fragment, createContext, useContext } from 'react';
import type {
  TabListProps as AriaTabListProps,
  TabProps as AriaTabProps,
  TabRenderProps as AriaTabRenderProps,
} from 'react-aria-components';
import {
  Tab as AriaTab,
  TabList as AriaTabList,
  TabPanel as AriaTabPanel,
  Tabs as AriaTabs,
  TabsContext,
  useSlottedContext,
} from 'react-aria-components';
import type { BadgeColors } from '@/components/base/badges/badge-types';
import { Badge } from '@/components/base/badges/badges';
import { cx } from '@/utils/cx';

type Orientation = 'horizontal' | 'vertical';

type HorizontalTypes =
  | 'button-brand'
  | 'button-gray'
  | 'button-border'
  | 'button-minimal'
  | 'underline';
type VerticalTypes = 'button-brand' | 'button-gray' | 'button-border' | 'button-minimal' | 'line';
type TabTypeColors<T> = T extends 'horizontal' ? HorizontalTypes : VerticalTypes;

const getTabStyles = ({ isFocusVisible, isSelected, isHovered }: AriaTabRenderProps) => ({
  'button-brand': cx(
    'outline-brand',
    isFocusVisible && 'outline-2 -outline-offset-2',
    (isSelected || isHovered) && 'bg-brand-primary_alt text-brand-secondary'
  ),
  'button-gray': cx(
    'outline-brand',
    isHovered && 'bg-primary_hover text-secondary',
    isFocusVisible && 'outline-2 -outline-offset-2',
    isSelected && 'bg-active text-secondary'
  ),
  'button-border': cx(
    'outline-brand',
    (isSelected || isHovered) && 'bg-primary_alt text-secondary shadow-sm',
    isFocusVisible && 'outline-2 -outline-offset-2'
  ),
  'button-minimal': cx(
    'rounded-lg outline-brand',
    isHovered && 'text-secondary',
    isFocusVisible && 'outline-2 -outline-offset-2',
    isSelected && 'bg-primary_alt text-secondary shadow-xs ring-1 ring-primary ring-inset'
  ),
  underline: cx(
    'rounded-none border-b-2 border-transparent outline-brand',
    (isSelected || isHovered) && 'border-fg-brand-primary_alt text-brand-secondary',
    isFocusVisible && 'outline-2 -outline-offset-2'
  ),
  line: cx(
    'rounded-none border-l-2 border-transparent outline-brand',
    (isSelected || isHovered) && 'border-fg-brand-primary_alt text-brand-secondary',
    isFocusVisible && 'outline-2 -outline-offset-2'
  ),
});

const sizes = {
  sm: {
    'button-brand': 'text-sm font-semibold py-2 px-3',
    'button-gray': 'text-sm font-semibold py-2 px-3',
    'button-border': 'text-sm font-semibold py-2 px-3',
    'button-minimal': 'text-sm font-semibold py-2 px-3',
    underline: 'text-sm font-semibold px-1 pb-2.5 pt-0',
    line: 'text-sm font-semibold pl-2.5 pr-3 py-0.5',
  },
  md: {
    'button-brand': 'text-md font-semibold py-2.5 px-3',
    'button-gray': 'text-md font-semibold py-2.5 px-3',
    'button-border': 'text-md font-semibold py-2.5 px-3',
    'button-minimal': 'text-md font-semibold py-2.5 px-3',
    underline: 'text-md font-semibold px-1 pb-2.5 pt-0',
    line: 'text-md font-semibold pr-3.5 pl-3 py-1',
  },
};

const getHorizontalStyles = ({ size, fullWidth }: { size?: 'sm' | 'md'; fullWidth?: boolean }) => ({
  'button-brand': 'gap-1',
  'button-gray': 'gap-1',
  'button-border': cx(
    'gap-1 rounded-[10px] bg-secondary_alt p-1 ring-1 ring-secondary ring-inset',
    size === 'md' && 'rounded-xl p-1.5'
  ),
  'button-minimal': 'gap-0.5 rounded-lg bg-secondary_alt ring-1 ring-inset ring-secondary',
  underline: cx('gap-3', fullWidth && 'w-full gap-4'),
  line: 'gap-2',
});

const getColorStyles = ({ isSelected, isHovered }: Partial<AriaTabRenderProps>) => ({
  'button-brand': isSelected || isHovered ? 'brand' : 'gray',
  'button-gray': 'gray',
  'button-border': 'gray',
  'button-minimal': 'gray',
  underline: isSelected || isHovered ? 'brand' : 'gray',
  line: isSelected || isHovered ? 'brand' : 'gray',
});

interface TabListComponentProps<
  T extends object,
  K extends Orientation,
> extends AriaTabListProps<T> {
  size?: keyof typeof sizes;
  type?: TabTypeColors<K>;
  orientation?: K;
  items: T[];
  fullWidth?: boolean;
}

const TabListContext = createContext<
  Omit<TabListComponentProps<TabComponentProps, Orientation>, 'items'>
>({
  size: 'sm',
  type: 'button-brand',
});

export const TabList = <T extends Orientation>({
  size = 'sm',
  type = 'button-brand',
  orientation: orientationProp,
  fullWidth,
  className,
  children,
  ...otherProps
}: TabListComponentProps<TabComponentProps, T>) => {
  const context = useSlottedContext(TabsContext);
  const orientation = orientationProp ?? context?.orientation ?? 'horizontal';

  return (
    <TabListContext.Provider value={{ size, type, orientation, fullWidth }}>
      <AriaTabList
        {...otherProps}
        className={(state) =>
          cx(
            'group flex',
            getHorizontalStyles({ size, fullWidth })[type as HorizontalTypes],
            orientation === 'vertical' && 'w-max flex-col',
            orientation === 'horizontal' &&
              type === 'underline' &&
              'before:bg-border-secondary relative before:absolute before:inset-x-0 before:bottom-0 before:h-px',
            typeof className === 'function' ? className(state) : className
          )
        }
      >
        {children ?? ((item) => <Tab {...item}>{item.children}</Tab>)}
      </AriaTabList>
    </TabListContext.Provider>
  );
};

export const TabPanel = (props: ComponentPropsWithRef<typeof AriaTabPanel>) => {
  return (
    <AriaTabPanel
      {...props}
      className={(state) =>
        cx(
          'outline-brand focus-visible:outline-2 focus-visible:outline-offset-2',
          typeof props.className === 'function' ? props.className(state) : props.className
        )
      }
    />
  );
};

interface TabComponentProps extends AriaTabProps {
  label?: ReactNode;
  children?: ReactNode | ((props: AriaTabRenderProps) => ReactNode);
  badge?: number | string;
  badgeColor?: BadgeColors;
}

export const Tab = (props: TabComponentProps) => {
  const { label, children, badge, badgeColor, ...otherProps } = props;
  const { size = 'sm', type = 'button-brand', fullWidth } = useContext(TabListContext);

  return (
    <AriaTab
      {...otherProps}
      className={(prop) =>
        cx(
          'text-quaternary z-10 flex h-max cursor-pointer items-center justify-center gap-2 rounded-md whitespace-nowrap transition duration-100 ease-linear',
          'group-orientation-vertical:justify-start',
          fullWidth && 'w-full flex-1',
          sizes[size][type],
          getTabStyles(prop)[type],
          typeof props.className === 'function' ? props.className(prop) : props.className
        )
      }
    >
      {(state) => (
        <Fragment>
          {typeof children === 'function' ? children(state) : children || label}
          {badge && (
            <Badge
              size={size}
              type="pill-color"
              color={badgeColor ?? (getColorStyles(state)[type] as BadgeColors)}
              className={cx('transition-inherit-all hidden md:flex', size === 'sm' && '-my-px')}
            >
              {badge}
            </Badge>
          )}
        </Fragment>
      )}
    </AriaTab>
  );
};

export const Tabs = ({ className, ...props }: ComponentPropsWithRef<typeof AriaTabs>) => {
  return (
    <AriaTabs
      keyboardActivation="manual"
      {...props}
      className={(state) =>
        cx('flex w-full flex-col', typeof className === 'function' ? className(state) : className)
      }
    />
  );
};

Tabs.Panel = TabPanel;
Tabs.List = TabList;
Tabs.Item = Tab;

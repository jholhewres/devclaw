import type { FC, ReactNode, Ref } from 'react';
import { isValidElement } from 'react';
import { cx, sortCx } from '@/utils/cx';
import { isReactComponent } from '@/utils/is-react-component';

const iconsSizes = {
  sm: '*:data-icon:size-4',
  md: '*:data-icon:size-5',
  lg: '*:data-icon:size-6',
  xl: '*:data-icon:size-7',
};

const styles = sortCx({
  light: {
    base: 'rounded-full',
    sizes: {
      sm: 'size-8',
      md: 'size-10',
      lg: 'size-12',
      xl: 'size-14',
    },
    colors: {
      brand: 'bg-brand-secondary text-featured-icon-light-fg-brand',
      gray: 'bg-tertiary text-featured-icon-light-fg-gray',
      error: 'bg-error-secondary text-featured-icon-light-fg-error',
      warning: 'bg-warning-secondary text-featured-icon-light-fg-warning',
      success: 'bg-success-secondary text-featured-icon-light-fg-success',
    },
  },
  dark: {
    base: 'text-fg-white shadow-xs-skeumorphic before:absolute before:inset-px before:border before:border-white/12 before:mask-b-from-0%',
    sizes: {
      sm: 'size-8 rounded-md before:rounded-[5px]',
      md: 'size-10 rounded-lg before:rounded-[7px]',
      lg: 'size-12 rounded-[10px] before:rounded-[9px]',
      xl: 'size-14 rounded-xl before:rounded-[11px]',
    },
    colors: {
      brand: 'bg-brand-solid before:border-utility-brand-200/12',
      gray: 'bg-secondary-solid before:border-utility-gray-200/12',
      error: 'bg-error-solid before:border-utility-error-200/12',
      warning: 'bg-warning-solid before:border-utility-error-200/12',
      success: 'bg-success-solid before:border-utility-success-200/12',
    },
  },
  modern: {
    base: 'bg-primary shadow-xs-skeumorphic ring-1 ring-inset',
    sizes: {
      sm: 'size-8 rounded-md',
      md: 'size-10 rounded-lg',
      lg: 'size-12 rounded-[10px]',
      xl: 'size-14 rounded-xl',
    },
    colors: {
      brand: '',
      gray: 'text-fg-secondary ring-primary',
      error: '',
      warning: '',
      success: '',
    },
  },
  outline: {
    base: 'before:absolute before:rounded-full before:border-2 after:absolute after:rounded-full after:border-2',
    sizes: {
      sm: 'size-4 before:size-6 after:size-8.5',
      md: 'size-5 before:size-7 after:size-9.5',
      lg: 'size-6 before:size-8 after:size-10.5',
      xl: 'size-7 before:size-9 after:size-11.5',
    },
    colors: {
      brand:
        'text-fg-brand-primary before:border-fg-brand-primary/30 after:border-fg-brand-primary/10',
      gray: 'text-fg-tertiary before:border-fg-tertiary/30 after:border-fg-tertiary/10',
      error:
        'text-fg-error-primary before:border-fg-error-primary/30 after:border-fg-error-primary/10',
      warning:
        'text-fg-warning-primary before:border-fg-warning-primary/30 after:border-fg-warning-primary/10',
      success:
        'text-fg-success-primary before:border-fg-success-primary/30 after:border-fg-success-primary/10',
    },
  },
});

interface FeaturedIconProps {
  ref?: Ref<HTMLDivElement>;
  children?: ReactNode;
  className?: string;
  icon?: FC<{ className?: string }> | ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  color: 'brand' | 'gray' | 'success' | 'warning' | 'error';
  theme?: 'light' | 'dark' | 'outline' | 'modern';
}

export const FeaturedIcon = (props: FeaturedIconProps) => {
  const {
    size = 'sm',
    theme: variant = 'light',
    color = 'brand',
    icon: Icon,
    ...otherProps
  } = props;

  return (
    <div
      {...otherProps}
      data-featured-icon
      className={cx(
        'relative flex shrink-0 items-center justify-center',
        iconsSizes[size],
        styles[variant].base,
        styles[variant].sizes[size],
        styles[variant].colors[color],
        props.className
      )}
    >
      {isReactComponent(Icon) && <Icon data-icon className="z-1" />}
      {isValidElement(Icon) && <div className="z-1">{Icon}</div>}
      {props.children}
    </div>
  );
};

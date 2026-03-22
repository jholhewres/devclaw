import type { ComponentPropsWithRef } from 'react';
import { createContext, useContext } from 'react';
import { Search } from 'lucide-react';
import { FeaturedIcon as FeaturedIconBase } from '@/components/foundations/featured-icon/featured-icon';
import { cx } from '@/utils/cx';

interface RootContextProps {
  size?: 'sm' | 'md' | 'lg';
}

const RootContext = createContext<RootContextProps>({ size: 'lg' });

interface RootProps extends ComponentPropsWithRef<'div'>, RootContextProps {}

const Root = ({ size = 'lg', ...props }: RootProps) => {
  return (
    <RootContext.Provider value={{ size }}>
      <div
        {...props}
        className={cx(
          'mx-auto flex w-full max-w-lg flex-col items-center justify-center',
          props.className
        )}
      />
    </RootContext.Provider>
  );
};

const FeaturedIcon = ({
  color = 'gray',
  theme = 'modern',
  icon = Search,
  size = 'lg',
  ...props
}: ComponentPropsWithRef<typeof FeaturedIconBase>) => {
  const { size: rootSize } = useContext(RootContext);

  return (
    <FeaturedIconBase
      {...props}
      {...{ color, theme, icon }}
      size={rootSize === 'lg' ? 'xl' : size}
    />
  );
};

const Header = ({ ...props }: ComponentPropsWithRef<'header'>) => {
  const { size } = useContext(RootContext);

  return (
    <header
      {...props}
      className={cx(
        'relative mb-4',
        (size === 'md' || size === 'lg') && 'mb-5',
        props.className
      )}
    >
      {props.children}
    </header>
  );
};

const Content = (props: ComponentPropsWithRef<'div'>) => {
  const { size } = useContext(RootContext);

  return (
    <main
      {...props}
      className={cx(
        'z-10 mb-6 flex w-full max-w-88 flex-col items-center justify-center gap-1',
        (size === 'md' || size === 'lg') && 'mb-8 gap-2',
        props.className
      )}
    />
  );
};

const Footer = (props: ComponentPropsWithRef<'div'>) => {
  return <footer {...props} className={cx('z-10 flex gap-3', props.className)} />;
};

const Title = (props: ComponentPropsWithRef<'h1'>) => {
  const { size } = useContext(RootContext);

  return (
    <h1
      {...props}
      className={cx(
        'text-md text-primary font-semibold',
        size === 'md' && 'text-lg font-semibold',
        size === 'lg' && 'text-xl font-semibold',
        props.className
      )}
    />
  );
};

const Description = (props: ComponentPropsWithRef<'p'>) => {
  const { size } = useContext(RootContext);

  return (
    <p
      {...props}
      className={cx(
        'text-tertiary text-center text-sm',
        size === 'lg' && 'text-md',
        props.className
      )}
    />
  );
};

const EmptyState = Root as typeof Root & {
  Title: typeof Title;
  Header: typeof Header;
  Footer: typeof Footer;
  Content: typeof Content;
  Description: typeof Description;
  FeaturedIcon: typeof FeaturedIcon;
};

EmptyState.Title = Title;
EmptyState.Header = Header;
EmptyState.Footer = Footer;
EmptyState.Content = Content;
EmptyState.Description = Description;
EmptyState.FeaturedIcon = FeaturedIcon;

export { EmptyState };

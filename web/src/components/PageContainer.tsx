import { cx } from '@/utils/cx';

interface PageContainerProps {
  children: React.ReactNode;
  className?: string;
}

export function PageContainer({ children, className }: PageContainerProps) {
  return (
    <div
      className={cx(
        'w-full px-4 pt-8 pb-20 sm:px-6 sm:pt-10 lg:px-8',
        className
      )}
    >
      {children}
    </div>
  );
}

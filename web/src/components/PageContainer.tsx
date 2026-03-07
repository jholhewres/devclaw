import { cx } from '@/utils/cx';

interface PageContainerProps {
  children: React.ReactNode;
  className?: string;
}

export function PageContainer({ children, className }: PageContainerProps) {
  return (
    <div
      className={cx(
        'mx-auto w-full max-w-[1440px] p-6 pb-20 sm:p-8 sm:pb-20 lg:p-12 lg:pb-20',
        className
      )}
    >
      {children}
    </div>
  );
}

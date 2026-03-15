import { cn } from '@/lib/utils';

interface PageContainerProps {
  children: React.ReactNode;
  className?: string;
}

export function PageContainer({ children, className }: PageContainerProps) {
  return (
    <div
      className={cn(
        'mx-auto w-full max-w-4xl px-4 pt-8 pb-20 sm:px-6 sm:pt-10 lg:px-8',
        className
      )}
    >
      {children}
    </div>
  );
}

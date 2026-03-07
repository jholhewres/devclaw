import { type ReactNode, useState, useRef, useCallback } from 'react';
import { cn } from '@/lib/utils';

type Side = 'top' | 'bottom' | 'left' | 'right';

const positionClasses: Record<Side, string> = {
  top: 'bottom-full left-1/2 -translate-x-1/2 mb-2',
  bottom: 'top-full left-1/2 -translate-x-1/2 mt-2',
  left: 'right-full top-1/2 -translate-y-1/2 mr-2',
  right: 'left-full top-1/2 -translate-y-1/2 ml-2',
};

const arrowClasses: Record<Side, string> = {
  top: 'top-full left-1/2 -translate-x-1/2 border-t-text-primary border-x-transparent border-b-transparent',
  bottom: 'bottom-full left-1/2 -translate-x-1/2 border-b-text-primary border-x-transparent border-t-transparent',
  left: 'left-full top-1/2 -translate-y-1/2 border-l-text-primary border-y-transparent border-r-transparent',
  right: 'right-full top-1/2 -translate-y-1/2 border-r-text-primary border-y-transparent border-l-transparent',
};

interface TooltipProps {
  content: string;
  children: ReactNode;
  side?: Side;
  className?: string;
}

export function Tooltip({
  content,
  children,
  side = 'top',
  className,
}: TooltipProps) {
  const [visible, setVisible] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const show = useCallback(() => {
    clearTimeout(timeoutRef.current);
    timeoutRef.current = setTimeout(() => setVisible(true), 150);
  }, []);

  const hide = useCallback(() => {
    clearTimeout(timeoutRef.current);
    setVisible(false);
  }, []);

  return (
    <span
      className={cn('relative inline-flex', className)}
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocus={show}
      onBlur={hide}
    >
      {children}

      {visible && content && (
        <span
          role="tooltip"
          className={cn(
            'absolute z-50 pointer-events-none',
            'whitespace-nowrap rounded-md bg-text-primary px-2.5 py-1.5',
            'text-xs font-medium text-text-inverse shadow-md',
            'animate-fade-in',
            positionClasses[side]
          )}
        >
          {content}
          <span
            aria-hidden="true"
            className={cn(
              'absolute h-0 w-0 border-4',
              arrowClasses[side]
            )}
          />
        </span>
      )}
    </span>
  );
}

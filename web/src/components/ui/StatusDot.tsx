import { cn } from '@/lib/utils';

type Status = 'online' | 'offline' | 'error' | 'warning';

const statusColors: Record<Status, { dot: string; pulse: string }> = {
  online: {
    dot: 'bg-success-solid',
    pulse: 'bg-success-solid/40',
  },
  offline: {
    dot: 'bg-quaternary',
    pulse: 'bg-quaternary/40',
  },
  error: {
    dot: 'bg-error-solid',
    pulse: 'bg-error-solid/40',
  },
  warning: {
    dot: 'bg-warning-solid',
    pulse: 'bg-warning-solid/40',
  },
};

interface StatusDotProps {
  status: Status;
  label?: string;
  pulse?: boolean;
  className?: string;
}

export function StatusDot({
  status,
  label,
  pulse = false,
  className,
}: StatusDotProps) {
  const colors = statusColors[status];

  return (
    <span
      className={cn('inline-flex items-center gap-2', className)}
    >
      <span className="relative flex h-2.5 w-2.5">
        {pulse && (
          <span
            className={cn(
              'absolute inset-0 rounded-full',
              'animate-ping',
              colors.pulse
            )}
          />
        )}
        <span
          className={cn(
            'relative inline-flex h-2.5 w-2.5 rounded-full',
            colors.dot
          )}
        />
      </span>

      {label && (
        <span className="text-sm text-secondary">{label}</span>
      )}
    </span>
  );
}

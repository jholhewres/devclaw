import { Terminal } from 'lucide-react'
import { cn } from '@/lib/utils'

interface Props {
  className?: string;
  /** Show only the icon (no text) */
  iconOnly?: boolean;
  /** Size variant */
  size?: 'sm' | 'md' | 'lg';
}

const sizes = {
  sm: { box: 'h-7 w-7 rounded-lg', icon: 'h-3.5 w-3.5', text: 'text-sm' },
  md: { box: 'h-8 w-8 rounded-lg', icon: 'h-4 w-4', text: 'text-base' },
  lg: { box: 'h-10 w-10 rounded-xl', icon: 'h-5 w-5', text: 'text-lg' },
} as const;

export function Logo({ className, iconOnly = false, size = 'md' }: Props) {
  const s = sizes[size];

  return (
    <div className={cn('flex items-center gap-2.5', className)}>
      <div className={cn('flex items-center justify-center bg-brand', s.box)}>
        <Terminal className={cn('text-white', s.icon)} />
      </div>
      {!iconOnly && (
        <span className={cn('font-semibold text-text-primary', s.text)}>
          Dev<span className="text-text-muted">Claw</span>
        </span>
      )}
    </div>
  );
}

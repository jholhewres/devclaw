import { Terminal } from 'lucide-react';
import { cx } from '@/utils/cx';

interface Props {
  className?: string;
  iconOnly?: boolean;
  size?: 'sm' | 'md' | 'lg';
}

const sizes = {
  sm: { box: 'size-7 rounded-lg', icon: 'size-3.5', text: 'text-sm' },
  md: { box: 'size-8 rounded-lg', icon: 'size-4', text: 'text-base' },
  lg: { box: 'size-10 rounded-xl', icon: 'size-5', text: 'text-lg' },
} as const;

export function Logo({ className, iconOnly = false, size = 'md' }: Props) {
  const s = sizes[size];

  return (
    <div className={cx('flex items-center gap-2.5', className)}>
      <div className={cx('flex items-center justify-center bg-brand-solid text-white', s.box)}>
        <Terminal className={s.icon} />
      </div>
      {!iconOnly && (
        <span className={cx('font-semibold text-primary', s.text)}>
          Dev<span className="text-quaternary">Claw</span>
        </span>
      )}
    </div>
  );
}

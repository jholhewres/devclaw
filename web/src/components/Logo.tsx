import { cx } from '@/utils/cx';

interface Props {
  className?: string;
  iconOnly?: boolean;
  size?: 'sm' | 'md' | 'lg';
}

const sizes = {
  sm: { box: 'size-7 rounded-lg', icon: 'size-[15px]', text: 'text-sm' },
  md: { box: 'size-8 rounded-lg', icon: 'size-[18px]', text: 'text-base' },
  lg: { box: 'size-10 rounded-xl', icon: 'size-[22px]', text: 'text-lg' },
} as const;

/**
 * ClawMark — the three-slash brand motif: three diagonal strokes with a slight
 * curve, getting shorter toward the bottom. Stroke uses currentColor.
 */
export function ClawMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} aria-hidden="true">
      <g fill="none" stroke="currentColor" strokeWidth={2.4} strokeLinecap="round">
        <path d="M5 4 C 9 8, 13 12, 18 17" />
        <path d="M8 3 C 12 7, 16 11, 20 14" />
        <path d="M12 4 C 15 7, 18 10, 21 11" opacity="0.85" />
      </g>
    </svg>
  );
}

export function Logo({ className, iconOnly = false, size = 'md' }: Props) {
  const s = sizes[size];

  return (
    <div className={cx('flex items-center gap-2.5', className)}>
      <div className={cx('flex items-center justify-center bg-primary-solid text-brand-solid', s.box)}>
        <ClawMark className={s.icon} />
      </div>
      {!iconOnly && (
        <span className={cx('font-semibold tracking-[-0.015em] text-primary', s.text)}>
          Dev<span className="font-medium text-tertiary">Claw</span>
        </span>
      )}
    </div>
  );
}

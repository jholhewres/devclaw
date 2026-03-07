import { useCallback, useEffect, useState } from 'react';
import { ChevronDown } from '@untitledui/icons';
import { cx } from '@/utils/cx';

interface CollapsibleSectionProps {
  icon: React.FC<{ className?: string }>;
  title: string;
  subtitle: string;
  iconColor?: string;
  trailing?: React.ReactNode;
  badge?: React.ReactNode;
  collapsible?: boolean;
  defaultOpen?: boolean;
  children: React.ReactNode;
}

export function CollapsibleSection({
  icon: Icon,
  title,
  subtitle,
  iconColor,
  trailing,
  badge,
  collapsible = true,
  defaultOpen = false,
  children,
}: CollapsibleSectionProps) {
  const [open, setOpen] = useState(defaultOpen);

  // When trailing changes, open the panel (accordion behavior)
  useEffect(() => {
    if (trailing && collapsible) {
      setOpen(true);
    }
  }, [trailing, collapsible, setOpen]);

  const canToggle = collapsible && !trailing;
  const canToggleWithTrailing = collapsible && !!trailing;

  const handleToggle = useCallback(() => {
    setOpen(!open);
  }, [setOpen, open]);

  const handleClick = useCallback(() => {
    if (canToggle || canToggleWithTrailing) {
      handleToggle();
    }
  }, [canToggle, canToggleWithTrailing, handleToggle]);

  const Wrapper = canToggle || canToggleWithTrailing ? 'button' : 'div';

  return (
    <div className="border-secondary rounded-3xl border">
      <Wrapper
        {...(Wrapper === 'button' ? { onClick: handleClick, 'aria-expanded': open } : {})}
        className={cx(
          'flex w-full flex-col gap-2 p-5 text-left lg:flex-row lg:items-center lg:p-6',
          (canToggle || canToggleWithTrailing) && 'cursor-pointer'
        )}
      >
        <div className="flex items-center justify-between lg:contents">
          <div
            className="bg-tertiary flex size-8 shrink-0 items-center justify-center rounded-lg lg:size-12 lg:rounded-2xl"
            style={iconColor ? { color: iconColor } : undefined}
          >
            <Icon
              className={cx('size-5 lg:size-6', !iconColor && 'text-[var(--color-fg-primary)]')}
            />
          </div>
          {/* Mobile trailing slot (chevron or toggle) */}
          {trailing ? (
            <div className="lg:hidden" onClick={(e) => e.stopPropagation()}>
              {trailing}
            </div>
          ) : (
            <div className="flex items-center gap-2 lg:hidden">
              {badge}
              {collapsible && (
                <ChevronDown
                  className={cx(
                    'size-5 shrink-0 text-[var(--color-fg-quaternary)] transition-transform duration-200',
                    open && 'rotate-180'
                  )}
                />
              )}
            </div>
          )}
        </div>
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <div className="min-w-0 flex-1">
            <h3 className="text-md text-primary font-medium lg:text-lg lg:font-semibold">
              {title}
            </h3>
            <p className="text-tertiary text-sm lg:truncate">{subtitle}</p>
          </div>
          {/* Desktop trailing slot (chevron or toggle) */}
          {trailing ? (
            <div className="hidden lg:block" onClick={(e) => e.stopPropagation()}>
              {trailing}
            </div>
          ) : (
            <div className="hidden items-center gap-2 lg:flex">
              {badge}
              {collapsible && (
                <ChevronDown
                  className={cx(
                    'size-5 shrink-0 text-[var(--color-fg-quaternary)] transition-transform duration-200',
                    open && 'rotate-180'
                  )}
                />
              )}
            </div>
          )}
        </div>
      </Wrapper>
      {open && (
        <div className="border-secondary flex flex-col gap-4 border-t px-5 py-4 lg:px-6 lg:py-5">
          {children}
        </div>
      )}
    </div>
  );
}

import type { ReactNode } from 'react';
import { AlertCircle, CheckCircle, Info } from 'lucide-react';
import { Button } from '@/components/base/buttons/button';
import { CloseButton } from '@/components/base/buttons/close-button';
import { FeaturedIcon } from '@/components/foundations/featured-icon/featured-icon';
import { cx } from '@/utils/cx';

const iconMap = {
  default: Info,
  brand: Info,
  gray: Info,
  error: AlertCircle,
  warning: AlertCircle,
  success: CheckCircle,
};

interface AlertFloatingProps {
  title: string;
  description?: ReactNode;
  confirmLabel?: string;
  dismissLabel?: string;
  color?: 'default' | 'brand' | 'gray' | 'error' | 'warning' | 'success';
  onClose?: () => void;
  onConfirm?: () => void;
  className?: string;
}

export const AlertFloating = ({
  title,
  description,
  confirmLabel,
  onClose,
  onConfirm,
  color = 'default',
  dismissLabel = 'Dismiss',
  className,
}: AlertFloatingProps) => {
  return (
    <div
      className={cx(
        'border-primary bg-primary_alt relative flex flex-col gap-4 rounded-xl border p-4 shadow-xs md:flex-row',
        className
      )}
    >
      <FeaturedIcon
        icon={iconMap[color]}
        color={color === 'default' ? 'gray' : color}
        theme={color === 'default' ? 'modern' : 'outline'}
        size="md"
      />

      <div className="flex flex-1 flex-col gap-3">
        <div className="flex flex-col gap-1">
          <p className="text-secondary pr-8 text-sm font-semibold md:pr-0">{title}</p>
          {description && <p className="text-tertiary text-sm">{description}</p>}
        </div>

        {(onConfirm || onClose) && (
          <div className="flex gap-3">
            {onClose && (
              <Button onClick={onClose} size="sm" color="link-gray">
                {dismissLabel}
              </Button>
            )}
            {onConfirm && (
              <Button onClick={onConfirm} size="sm" color="link-color">
                {confirmLabel}
              </Button>
            )}
          </div>
        )}
      </div>

      {onClose && (
        <CloseButton
          onPress={onClose}
          size="sm"
          label={dismissLabel}
          className="absolute top-0 right-0"
        />
      )}
    </div>
  );
};

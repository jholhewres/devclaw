import type { ReactNode, Ref } from 'react';
import { HelpCircle } from 'lucide-react';
import type { LabelProps as AriaLabelProps } from 'react-aria-components';
import { Label as AriaLabel } from 'react-aria-components';
import { Tooltip, TooltipTrigger } from '@/components/base/tooltip/tooltip';
import { cx } from '@/utils/cx';

interface LabelProps extends AriaLabelProps {
  children: ReactNode;
  isRequired?: boolean;
  tooltip?: string;
  tooltipDescription?: string;
  ref?: Ref<HTMLLabelElement>;
}

export const Label = ({
  isRequired,
  tooltip,
  tooltipDescription,
  className,
  ...props
}: LabelProps) => {
  return (
    <AriaLabel
      data-label="true"
      {...props}
      className={cx(
        'text-secondary flex cursor-default items-center gap-0.5 text-sm font-medium',
        className
      )}
    >
      {props.children}

      <span
        className={cx(
          'text-brand-tertiary hidden',
          isRequired && 'block',
          typeof isRequired === 'undefined' && 'group-required:block'
        )}
      >
        *
      </span>

      {tooltip && (
        <Tooltip title={tooltip} description={tooltipDescription} placement="top">
          <TooltipTrigger
            isDisabled={false}
            className="text-fg-quaternary hover:text-fg-quaternary_hover focus:text-fg-quaternary_hover cursor-pointer transition duration-200"
          >
            <HelpCircle className="size-4" />
          </TooltipTrigger>
        </Tooltip>
      )}
    </AriaLabel>
  );
};

Label.displayName = 'Label';

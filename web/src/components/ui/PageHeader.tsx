import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ArrowLeft } from 'lucide-react';
import { cn } from '@/lib/utils';

interface PageHeaderProps {
  title: string;
  description?: string;
  actions?: ReactNode;
  backLink?: string;
  className?: string;
}

export function PageHeader({
  title,
  description,
  actions,
  backLink,
  className,
}: PageHeaderProps) {
  const { t } = useTranslation();
  return (
    <div className={cn('flex items-start justify-between gap-4', className)}>
      <div className="min-w-0 flex-1">
        {backLink && (
          <a
            href={backLink}
            className={cn(
              'mb-3 inline-flex items-center gap-1.5 text-sm text-text-secondary',
              'hover:text-text-primary transition-colors duration-150',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:rounded'
            )}
          >
            <ArrowLeft className="h-4 w-4" />
            {t('common.back')}
          </a>
        )}

        <h1 className="text-xl font-semibold tracking-tight text-text-primary">
          {title}
        </h1>

        {description && (
          <p className="mt-1 text-sm text-text-secondary">{description}</p>
        )}
      </div>

      {actions && (
        <div className="flex flex-shrink-0 items-center gap-2">{actions}</div>
      )}
    </div>
  );
}

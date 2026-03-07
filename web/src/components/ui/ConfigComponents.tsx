import { useState, type ReactNode } from 'react';
import { ChevronDown, ChevronUp, Save, RotateCcw, Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';

// ============================================
// Config Page Wrapper
// ============================================

interface ConfigPageProps {
  title: string;
  subtitle?: string;
  description?: string;
  children: ReactNode;
  actions?: ReactNode;
  message?: { type: 'success' | 'error'; text: string } | null;
}

export function ConfigPage({
  title,
  subtitle,
  description,
  children,
  actions,
  message,
}: ConfigPageProps) {
  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-bg-main">
      <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            {subtitle && (
              <p className="text-[11px] font-bold tracking-[0.15em] text-text-muted uppercase">
                {subtitle}
              </p>
            )}
            <h1 className="mt-1 text-2xl font-bold tracking-tight text-text-primary">{title}</h1>
            {description && <p className="mt-2 text-base text-text-muted">{description}</p>}
          </div>
          {actions && <div className="flex items-center gap-3">{actions}</div>}
        </div>

        {/* Message */}
        {message && (
          <div
            className={cn(
              'mt-6 rounded-xl border px-5 py-4 text-sm',
              message.type === 'success'
                ? 'border-success/20 bg-success-subtle text-success'
                : 'border-error/20 bg-error-subtle text-error'
            )}
          >
            {message.text}
          </div>
        )}

        {/* Content */}
        <div className="mt-10">{children}</div>
      </div>
    </div>
  );
}

// ============================================
// Config Section (Collapsible)
// ============================================

interface ConfigSectionProps {
  icon?: React.ElementType;
  title: string;
  description?: string;
  children: ReactNode;
  collapsible?: boolean;
  defaultCollapsed?: boolean;
  className?: string;
  iconColor?: string;
}

export function ConfigSection({
  icon: Icon,
  title,
  description,
  children,
  collapsible = false,
  defaultCollapsed = false,
  className,
  iconColor,
}: ConfigSectionProps) {
  const [isCollapsed, setIsCollapsed] = useState(defaultCollapsed);

  const content = (
    <div className={cn('space-y-5 rounded-2xl border border-border bg-bg-surface p-6', className)}>
      {children}
    </div>
  );

  if (collapsible) {
    return (
      <section className="mb-10">
        <button
          onClick={() => setIsCollapsed(!isCollapsed)}
          className="group mb-6 flex w-full items-center gap-3 text-left"
        >
          {Icon && (
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-bg-surface transition-colors group-hover:border-border-hover">
              <Icon className="h-5 w-5 text-text-muted" />
            </div>
          )}
          <div className="flex-1">
            <h2 className="text-lg font-semibold text-text-primary">{title}</h2>
            {description && <p className="text-sm text-text-muted">{description}</p>}
          </div>
          {isCollapsed ? (
            <ChevronDown className="h-5 w-5 text-text-muted transition-colors group-hover:text-text-primary" />
          ) : (
            <ChevronUp className="h-5 w-5 text-text-muted transition-colors group-hover:text-text-primary" />
          )}
        </button>
        {!isCollapsed && content}
      </section>
    );
  }

  return (
    <section className="mb-10">
      {(Icon || title) && (
        <div className="mb-6 flex items-center gap-3">
          {Icon && (
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-bg-surface">
              <Icon className="h-5 w-5" style={{ color: iconColor || undefined }} />
            </div>
          )}
          <div>
            <h2 className="text-lg font-semibold text-text-primary">{title}</h2>
            {description && <p className="text-sm text-text-muted">{description}</p>}
          </div>
        </div>
      )}
      {content}
    </section>
  );
}

// ============================================
// Config Field (Label + Hint)
// ============================================

interface ConfigFieldProps {
  label: string;
  hint?: string;
  children: ReactNode;
  className?: string;
}

export function ConfigField({ label, hint, children, className }: ConfigFieldProps) {
  return (
    <div className={cn('space-y-2', className)}>
      <label className="block text-xs font-semibold tracking-wider text-text-muted uppercase">
        {label}
      </label>
      {children}
      {hint && <p className="text-xs text-text-muted">{hint}</p>}
    </div>
  );
}

// ============================================
// Config Input
// ============================================

interface ConfigInputProps {
  value: string | number;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: 'text' | 'password' | 'number' | 'email' | 'url' | 'time';
  disabled?: boolean;
  className?: string;
}

export function ConfigInput({
  value,
  onChange,
  placeholder,
  type = 'text',
  disabled = false,
  className,
}: ConfigInputProps) {
  return (
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      disabled={disabled}
      className={cn(
        'h-11 w-full rounded-xl border border-border bg-bg-surface px-4 text-sm text-text-primary transition-all outline-none placeholder:text-text-muted hover:border-border-hover focus:border-brand/50 focus:ring-1 focus:ring-brand/20',
        disabled && 'cursor-not-allowed opacity-50',
        className
      )}
    />
  );
}

// ============================================
// Config Textarea
// ============================================

interface ConfigTextareaProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  rows?: number;
  disabled?: boolean;
  className?: string;
}

export function ConfigTextarea({
  value,
  onChange,
  placeholder,
  rows = 3,
  disabled = false,
  className,
}: ConfigTextareaProps) {
  return (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      rows={rows}
      disabled={disabled}
      className={cn(
        'w-full resize-none rounded-xl border border-border bg-bg-surface px-4 py-3 text-sm text-text-primary transition-all outline-none placeholder:text-text-muted hover:border-border-hover focus:border-brand/50 focus:ring-1 focus:ring-brand/20',
        disabled && 'cursor-not-allowed opacity-50',
        className
      )}
    />
  );
}

// ============================================
// Config Select
// ============================================

interface SelectOption {
  value: string;
  label: string;
}

interface ConfigSelectProps {
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

export function ConfigSelect({
  value,
  onChange,
  options,
  placeholder,
  disabled = false,
  className,
}: ConfigSelectProps) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
      className={cn(
        'h-11 w-full cursor-pointer appearance-none rounded-xl border border-border bg-bg-surface px-4 pr-10 text-sm text-text-primary transition-all outline-none hover:border-border-hover focus:border-brand/50 focus:ring-1 focus:ring-brand/20',
        disabled && 'cursor-not-allowed opacity-50',
        className
      )}
      style={{
        backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 24 24' fill='none' stroke='%2364748b' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E")`,
        backgroundRepeat: 'no-repeat',
        backgroundPosition: 'right 12px center',
      }}
    >
      {placeholder && (
        <option value="" disabled>
          {placeholder}
        </option>
      )}
      {options.map((opt) => (
        <option key={opt.value} value={opt.value}>
          {opt.label}
        </option>
      ))}
    </select>
  );
}

// ============================================
// Config Toggle
// ============================================

interface ConfigToggleProps {
  enabled: boolean;
  onChange: (value: boolean) => void;
  label: string;
  description?: string;
  disabled?: boolean;
}

export function ConfigToggle({
  enabled,
  onChange,
  label,
  description,
  disabled = false,
}: ConfigToggleProps) {
  return (
    <button
      type="button"
      onClick={() => !disabled && onChange(!enabled)}
      disabled={disabled}
      className={cn(
        'group flex items-start gap-3',
        !disabled && 'cursor-pointer',
        disabled && 'cursor-not-allowed opacity-50'
      )}
    >
      <div
        className={cn(
          'relative mt-0.5 h-6 w-11 flex-shrink-0 rounded-full transition-colors',
          enabled ? 'bg-brand' : 'bg-bg-subtle'
        )}
      >
        <div
          className={cn(
            'absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform',
            enabled && 'translate-x-5'
          )}
        />
      </div>
      <div className="flex flex-col items-start">
        <span className="text-sm text-text-secondary transition-colors group-hover:text-text-primary">
          {label}
        </span>
        {description && <span className="mt-0.5 text-xs text-text-muted">{description}</span>}
      </div>
    </button>
  );
}

// ============================================
// Config Tag List
// ============================================

interface ConfigTagListProps {
  tags: string[];
  onAdd?: (tag: string) => void;
  onRemove?: (tag: string) => void;
  addPlaceholder?: string;
  readOnly?: boolean;
  emptyMessage?: string;
}

export function ConfigTagList({
  tags,
  onAdd,
  onRemove,
  addPlaceholder = 'Add item...',
  readOnly = false,
  emptyMessage = 'No items',
}: ConfigTagListProps) {
  const [inputValue, setInputValue] = useState('');

  const handleAdd = () => {
    if (inputValue.trim() && onAdd) {
      onAdd(inputValue.trim());
      setInputValue('');
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleAdd();
    }
  };

  return (
    <div className="space-y-3">
      {/* Tags */}
      <div className="flex flex-wrap gap-2">
        {tags.length === 0 && !readOnly && (
          <span className="text-sm text-text-muted italic">{emptyMessage}</span>
        )}
        {tags.map((tag) => (
          <span
            key={tag}
            className="group inline-flex items-center gap-1.5 rounded-lg bg-bg-subtle px-3 py-1.5 text-sm text-text-secondary"
          >
            {tag}
            {!readOnly && onRemove && (
              <button
                onClick={() => onRemove(tag)}
                className="text-text-muted transition-colors hover:text-error"
              >
                <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M6 18L18 6M6 6l12 12"
                  />
                </svg>
              </button>
            )}
          </span>
        ))}
      </div>

      {/* Add Input */}
      {!readOnly && onAdd && (
        <div className="flex gap-2">
          <input
            type="text"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={addPlaceholder}
            className="h-10 flex-1 rounded-lg border border-border bg-bg-surface px-3 text-sm text-text-primary transition-all outline-none placeholder:text-text-muted hover:border-border-hover focus:border-brand/50"
          />
          <button
            onClick={handleAdd}
            disabled={!inputValue.trim()}
            className="h-10 rounded-lg bg-brand px-4 text-sm font-medium text-white transition-colors hover:bg-brand-hover disabled:cursor-not-allowed disabled:opacity-50"
          >
            Add
          </button>
        </div>
      )}
    </div>
  );
}

// ============================================
// Config Actions (Save/Reset buttons)
// ============================================

interface ConfigActionsProps {
  onSave: () => void;
  onReset?: () => void;
  saving?: boolean;
  hasChanges?: boolean;
  saveLabel?: string;
  savingLabel?: string;
  resetLabel?: string;
}

export function ConfigActions({
  onSave,
  onReset,
  saving = false,
  hasChanges = true,
  saveLabel = 'Save',
  savingLabel = 'Saving...',
  resetLabel = 'Reset',
}: ConfigActionsProps) {
  return (
    <div className="flex items-center gap-3">
      {onReset && hasChanges && (
        <button
          onClick={onReset}
          disabled={saving}
          className="flex cursor-pointer items-center gap-2 rounded-xl border border-border bg-bg-surface px-5 py-3 text-sm font-medium text-text-secondary transition-all hover:border-border-hover hover:text-text-primary disabled:opacity-50"
        >
          <RotateCcw className="h-4 w-4" />
          {resetLabel}
        </button>
      )}
      <button
        onClick={onSave}
        disabled={!hasChanges || saving}
        className="flex cursor-pointer items-center gap-2 rounded-xl bg-brand px-5 py-3 text-sm font-semibold text-white transition-all hover:bg-brand-hover disabled:cursor-not-allowed disabled:opacity-50"
      >
        {saving ? (
          <>
            <Loader2 className="h-4 w-4 animate-spin" />
            {savingLabel}
          </>
        ) : (
          <>
            <Save className="h-4 w-4" />
            {saveLabel}
          </>
        )}
      </button>
    </div>
  );
}

// ============================================
// Config Card (for items like servers, webhooks)
// ============================================

interface ConfigCardProps {
  title: string;
  subtitle?: string;
  icon?: React.ElementType;
  iconColor?: string;
  status?: 'success' | 'error' | 'warning' | 'neutral';
  actions?: ReactNode;
  children?: ReactNode;
  className?: string;
}

export function ConfigCard({
  title,
  subtitle,
  icon: Icon,
  iconColor,
  status = 'neutral',
  actions,
  children,
  className,
}: ConfigCardProps) {
  const statusColors = {
    success: 'bg-success-subtle',
    error: 'bg-error-subtle',
    warning: 'bg-warning-subtle',
    neutral: 'bg-bg-subtle',
  };

  const iconColors = {
    success: 'text-success',
    error: 'text-error',
    warning: 'text-warning',
    neutral: 'text-text-muted',
  };

  return (
    <div className={cn('rounded-2xl border border-border bg-bg-surface p-6', className)}>
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-3">
          {Icon && (
            <div
              className={cn(
                'flex h-10 w-10 items-center justify-center rounded-xl',
                statusColors[status]
              )}
            >
              <Icon
                className={cn('h-5 w-5', iconColors[status])}
                style={{ color: status === 'neutral' ? iconColor : undefined }}
              />
            </div>
          )}
          <div>
            <h3 className="text-base font-semibold text-text-primary">{title}</h3>
            {subtitle && <p className="text-sm text-text-muted">{subtitle}</p>}
          </div>
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
      {children && <div className="mt-4 border-t border-border pt-4">{children}</div>}
    </div>
  );
}

// ============================================
// Config Empty State
// ============================================

interface ConfigEmptyStateProps {
  icon?: React.ElementType;
  title: string;
  description?: string;
  action?: ReactNode;
}

export function ConfigEmptyState({
  icon: Icon,
  title,
  description,
  action,
}: ConfigEmptyStateProps) {
  return (
    <div className="rounded-2xl border border-border bg-bg-surface p-8 text-center">
      {Icon && <Icon className="mx-auto mb-4 h-12 w-12 text-text-muted" />}
      <p className="text-sm text-text-muted">{title}</p>
      {description && <p className="mt-2 text-xs text-text-muted">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

// ============================================
// Config Info Box
// ============================================

interface ConfigInfoBoxProps {
  title?: string;
  items: string[];
}

export function ConfigInfoBox({ title, items }: ConfigInfoBoxProps) {
  return (
    <div className="mb-10 rounded-2xl border border-border bg-bg-surface/50 p-6">
      {title && <h3 className="mb-3 text-sm font-semibold text-text-muted">{title}</h3>}
      <ul className="space-y-2 text-xs text-text-muted">
        {items.map((item, index) => (
          <li key={index}>• {item}</li>
        ))}
      </ul>
    </div>
  );
}

// ============================================
// Loading Spinner
// ============================================

export function LoadingSpinner() {
  return (
    <div className="flex flex-1 items-center justify-center bg-bg-main">
      <div className="h-10 w-10 animate-spin rounded-full border-4 border-bg-subtle border-t-brand" />
    </div>
  );
}

// ============================================
// Error State
// ============================================

interface ErrorStateProps {
  message?: string;
  onRetry?: () => void;
  retryLabel?: string;
}

export function ErrorState({ message = 'Error', onRetry, retryLabel = 'Retry' }: ErrorStateProps) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center bg-bg-main">
      <p className="text-sm text-error">{message}</p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="mt-3 cursor-pointer text-xs text-text-muted transition-colors hover:text-text-primary"
        >
          {retryLabel}
        </button>
      )}
    </div>
  );
}

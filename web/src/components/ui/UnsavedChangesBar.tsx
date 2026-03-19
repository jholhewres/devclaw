import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AlertCircle, CheckCircle2, Loader2 } from 'lucide-react';
import { useAppStore } from '@/stores/app';
import { cn } from '@/lib/utils';

type Feedback = 'success' | 'error' | null;

interface UnsavedChangesBarProps {
  hasChanges: boolean;
  saving: boolean;
  onSave: () => Promise<boolean>;
  onDiscard: () => void;
}

export function UnsavedChangesBar({
  hasChanges,
  saving,
  onSave,
  onDiscard,
}: UnsavedChangesBarProps) {
  const { t } = useTranslation();
  const [feedback, setFeedback] = useState<Feedback>(null);
  const [visible, setVisible] = useState(false);
  const feedbackTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const frozenFeedback = useRef<Feedback>(null);
  const sidebarOpen = useAppStore((s) => s.sidebarOpen);
  const sidebarCollapsed = useAppStore((s) => s.sidebarCollapsed);

  // Stable refs for callbacks
  const onSaveRef = useRef(onSave);
  const onDiscardRef = useRef(onDiscard);
  useEffect(() => { onSaveRef.current = onSave; }, [onSave]);
  useEffect(() => { onDiscardRef.current = onDiscard; }, [onDiscard]);

  const shouldShow = hasChanges || feedback !== null;

  // Freeze last visual state so exit animation doesn't flash
  if (shouldShow) frozenFeedback.current = feedback;
  const activeFeedback = shouldShow ? feedback : frozenFeedback.current;

  useEffect(() => {
    if (shouldShow) {
      setVisible(true);
    } else {
      const timer = setTimeout(() => setVisible(false), 200);
      return () => clearTimeout(timer);
    }
  }, [shouldShow]);

  useEffect(
    () => () => {
      if (feedbackTimer.current) clearTimeout(feedbackTimer.current);
    },
    [],
  );

  const handleSave = useCallback(async () => {
    if (saving) return;
    try {
      const saved = await onSaveRef.current();
      if (saved) {
        setFeedback('success');
        feedbackTimer.current = setTimeout(() => setFeedback(null), 3000);
      }
    } catch {
      setFeedback('error');
    }
  }, [saving]);

  const handleDiscard = useCallback(() => {
    setFeedback(null);
    onDiscardRef.current();
  }, []);

  // Ctrl+S / Cmd+S shortcut
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        if (hasChanges && !saving) handleSave();
      }
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [hasChanges, saving, handleSave]);

  // beforeunload guard
  useEffect(() => {
    if (!hasChanges) return;
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [hasChanges]);

  // Sidebar width for positioning
  let sidebarWidth = 0;
  if (sidebarOpen) {
    sidebarWidth = sidebarCollapsed ? 64 : 296;
  }

  if (!visible) return null;

  const isError = activeFeedback === 'error';
  const isSuccess = activeFeedback === 'success';

  return (
    <div
      className="fixed right-3 bottom-3 z-40 transition-all duration-300 max-lg:!left-3"
      style={{ left: sidebarWidth > 0 ? `calc(${sidebarWidth}px + 0.75rem)` : '0.75rem' }}
    >
      <div className="flex justify-center">
        <div
          className={cn(
            'flex w-max items-center gap-4 rounded-full px-4 py-2.5 shadow-lg transition-all',
            isSuccess
              ? 'bg-success-subtle'
              : isError
                ? 'bg-error-subtle'
                : 'border border-border bg-bg-surface',
            shouldShow
              ? 'translate-y-0 opacity-100'
              : 'pointer-events-none translate-y-4 opacity-0',
          )}
        >
          {/* Icon + message */}
          <div className="flex items-center gap-2">
            {isSuccess ? (
              <CheckCircle2 className="h-5 w-5 shrink-0 text-success" />
            ) : (
              <AlertCircle className="h-5 w-5 shrink-0 text-text-muted" />
            )}
            <span
              className={cn(
                'whitespace-nowrap text-sm font-medium',
                isSuccess
                  ? 'text-success'
                  : isError
                    ? 'text-error'
                    : 'text-text-secondary',
              )}
            >
              {isSuccess
                ? t('unsavedChanges.saved')
                : isError
                  ? t('unsavedChanges.error')
                  : t('unsavedChanges.message')}
            </span>
          </div>

          {/* Buttons */}
          {!isSuccess && (
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={handleDiscard}
                disabled={saving}
                className="cursor-pointer rounded-lg px-3 py-1.5 text-sm font-medium text-text-secondary transition-colors hover:bg-bg-subtle hover:text-text-primary disabled:opacity-50"
              >
                {t('unsavedChanges.discard')}
              </button>
              <button
                type="button"
                onClick={handleSave}
                disabled={saving}
                className="flex cursor-pointer items-center gap-1.5 rounded-lg bg-brand px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-brand-hover disabled:opacity-50"
              >
                {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {isError ? t('unsavedChanges.retry') : t('unsavedChanges.save')}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

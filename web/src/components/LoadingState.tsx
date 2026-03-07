interface LoadingStateProps {
  description?: string;
}

export function LoadingState({ description }: LoadingStateProps = {}) {
  return (
    <div className="flex flex-col items-center justify-center gap-4 py-16">
      <div className="border-secondary border-t-fg-brand-primary size-8 animate-spin rounded-full border-4" />
      {description && <p className="text-tertiary text-sm">{description}</p>}
    </div>
  );
}

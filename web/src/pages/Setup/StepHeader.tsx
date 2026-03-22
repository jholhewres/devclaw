interface Props {
  title: string;
  description?: string;
}

export function StepHeader({ title, description }: Props) {
  return (
    <div className="flex flex-col gap-1">
      <h1 className="text-2xl font-medium text-primary">{title}</h1>
      {description && <p className="text-base text-tertiary">{description}</p>}
    </div>
  );
}

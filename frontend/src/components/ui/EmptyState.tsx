import type { ReactElement, ReactNode } from 'react';

export function EmptyState({
  title,
  level = 2,
  sentence,
  children,
}: {
  readonly title: string;
  readonly level?: 2 | 3;
  readonly sentence: string;
  readonly children?: ReactNode;
}): ReactElement {
  const Heading = level === 2 ? 'h2' : 'h3';
  return (
    <div data-empty-state="" className="flex flex-col items-start gap-2">
      <Heading className="text-heading">{title}</Heading>
      <p className="text-text-muted">{sentence}</p>
      {children === undefined ? null : <div className="flex flex-wrap items-center gap-3">{children}</div>}
    </div>
  );
}

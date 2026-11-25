import type { ReactElement, ReactNode, Ref } from 'react';

export function SectionHead({
  headingId,
  title,
  action = null,
  heading,
}: {
  readonly headingId: string;
  readonly title: ReactNode;
  readonly action?: ReactNode;
  readonly heading?: Ref<HTMLHeadingElement>;
}): ReactElement {
  const focusable = heading !== undefined;
  return (
    <div data-section-head="" className="flex flex-wrap items-baseline justify-between gap-x-3">
      <h2
        ref={heading}
        id={headingId}
        tabIndex={focusable ? -1 : undefined}
        className={focusable ? 'text-heading outline-none' : 'text-heading'}
      >
        {title}
      </h2>
      {action}
    </div>
  );
}

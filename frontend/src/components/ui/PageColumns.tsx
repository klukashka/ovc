import type { ReactElement, ReactNode } from 'react';

export function PageColumns({
  main,
  aside,
  asideLabel,
}: {
  readonly main: ReactNode;
  readonly aside: ReactNode;
  readonly asideLabel: string;
}): ReactElement {
  return (
    <div className="flex flex-col gap-10 wide:grid wide:grid-cols-[minmax(0,1fr)_var(--container-aside)] wide:items-start wide:gap-16">
      <div data-column="main" className="flex min-w-0 flex-col gap-8">
        {main}
      </div>
      <aside data-column="aside" aria-label={asideLabel} className="flex min-w-0 flex-col gap-8">
        {aside}
      </aside>
    </div>
  );
}

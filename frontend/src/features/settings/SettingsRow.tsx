import { type ReactElement, type ReactNode, useId } from 'react';

const rowClass =
  'grid gap-2 border-t border-border py-4 tablet:grid-cols-[var(--container-settings-label)_minmax(0,1fr)] tablet:items-center tablet:gap-6';

function RowLabel({
  label,
  labelFor,
  description,
}: {
  readonly label: string;
  readonly labelFor: string | undefined;
  readonly description: string | undefined;
}): ReactElement {
  return (
    <div data-settings-label className="flex flex-col gap-0.5">
      {labelFor === undefined ? (
        <span aria-hidden="true" className="text-strong">
          {label}
        </span>
      ) : (
        <label htmlFor={labelFor} className="text-strong">
          {label}
        </label>
      )}
      {description === undefined ? null : <span className="text-small text-text-muted">{description}</span>}
    </div>
  );
}

export function SettingsRow({
  label,
  labelFor,
  description,
  children,
}: {
  readonly label: string;
  readonly labelFor?: string;
  readonly description?: string;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <div className={rowClass}>
      <RowLabel label={label} labelFor={labelFor} description={description} />
      <div data-settings-value className="flex min-w-0 flex-col items-start gap-2">
        {children}
      </div>
    </div>
  );
}

export function SettingsRows({ children }: { readonly children: ReactNode }): ReactElement {
  return <div className="flex flex-col border-b border-border">{children}</div>;
}

export function SettingsSection({
  heading,
  description,
  children,
}: {
  readonly heading: string;
  readonly description?: string;
  readonly children: ReactNode;
}): ReactElement {
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className={`${rowClass} tablet:items-start`}>
      <div data-settings-label className="flex flex-col gap-1">
        <h2 id={headingId} className="text-heading">
          {heading}
        </h2>
        {description === undefined ? null : <p className="text-small text-text-muted">{description}</p>}
      </div>
      <div data-settings-value className="flex min-w-0 flex-col items-start gap-3">
        {children}
      </div>
    </section>
  );
}

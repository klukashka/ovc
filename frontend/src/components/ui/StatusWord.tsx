import type { ReactElement, ReactNode } from 'react';

export type Tone = 'info' | 'warning' | 'danger' | 'muted' | 'accent' | 'success';

const textTone: Readonly<Record<Tone, string>> = {
  info: 'text-info',
  warning: 'text-warning',
  danger: 'text-danger',
  muted: 'text-text-muted',
  accent: 'text-accent-text',
  success: 'text-success',
};

const dotTone: Readonly<Record<Tone, string>> = {
  info: 'bg-info',
  warning: 'bg-warning',
  danger: 'bg-danger',
  muted: 'bg-text-muted',
  accent: 'bg-accent-text',
  success: 'bg-success',
};

export function StatusWord({ tone, children }: { readonly tone: Tone; readonly children: ReactNode }): ReactElement {
  return (
    <span className={`inline-flex items-center gap-1.5 text-small ${textTone[tone]}`}>
      <span aria-hidden="true" className={`size-1.5 shrink-0 rounded-full ${dotTone[tone]}`} />
      {children}
    </span>
  );
}

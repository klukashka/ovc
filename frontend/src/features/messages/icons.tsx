import type { ReactElement } from 'react';

function Outline({ size, path }: { readonly size: number; readonly path: string }): ReactElement {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.3}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d={path} />
    </svg>
  );
}

export function FileIcon({ size }: { readonly size: number }): ReactElement {
  return (
    <Outline size={size} path="M9 2H4.5A1.5 1.5 0 003 3.5v9A1.5 1.5 0 004.5 14h7a1.5 1.5 0 001.5-1.5V6L9 2zM9 2v4h4" />
  );
}

export function ArrowDownIcon({ size }: { readonly size: number }): ReactElement {
  return <Outline size={size} path="M8 3v10M3.5 8.5L8 13l4.5-4.5" />;
}

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

export function SearchIcon({ size }: { readonly size: number }): ReactElement {
  return <Outline size={size} path="M7 12.5a5.5 5.5 0 100-11 5.5 5.5 0 000 11zM11 11l3.5 3.5" />;
}

export function CloseIcon({ size }: { readonly size: number }): ReactElement {
  return <Outline size={size} path="M4 4l8 8M12 4l-8 8" />;
}

export function ReadTicksIcon({ read }: { readonly read: boolean }): ReactElement {
  return <Outline size={14} path={read ? 'M1.5 8.5l3 3 6-7M7 11.5l1 0 6-7' : 'M3.5 8.5l3 3 6-7'} />;
}

export function SendIcon({ size }: { readonly size: number }): ReactElement {
  return <Outline size={size} path="M2 7.5L14 2l-4.5 12-2-5-5.5-1.5zM7.5 9L14 2" />;
}

export function EmojiIcon({ size }: { readonly size: number }): ReactElement {
  return <Outline size={size} path="M8 14a6 6 0 100-12 6 6 0 000 12zM5 9.5c1 2 5 2 6 0M5.5 6h.01M10.5 6h.01" />;
}

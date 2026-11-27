import { type ReactElement, useState } from 'react';

import { type PaletteColour, paletteAvatarClass } from './palette.ts';

const graphemes = new Intl.Segmenter(undefined, { granularity: 'grapheme' });

const sizeClass = {
  medium: 'size-10 text-body',
  small: 'size-8 text-small',
  large: 'size-14 text-heading',
} as const;

export function Avatar({
  name,
  avatarId,
  size = 'small',
  colour = null,
}: {
  readonly name: string;
  readonly colour?: PaletteColour | null;
  readonly avatarId: string | null;
  readonly size?: keyof typeof sizeClass;
}): ReactElement {
  const [failedId, setFailedId] = useState<string | null>(null);
  if (avatarId !== null && avatarId !== failedId) {
    return (
      <img
        src={`/api/v1/avatars/${encodeURIComponent(avatarId)}`}
        alt=""
        onError={() => {
          setFailedId(avatarId);
        }}
        className={`${sizeClass[size]} shrink-0 rounded-full object-cover`}
      />
    );
  }
  const initial =
    graphemes
      .segment(name.replace(/(?!\u200d)[\s\p{Cf}]/gu, '').replace(/^\u200d+/u, ''))
      .containing(0)
      ?.segment.toUpperCase() ?? '';
  const tint =
    initial !== '' && colour !== null && Object.hasOwn(paletteAvatarClass, colour)
      ? paletteAvatarClass[colour]
      : 'bg-hover text-text-muted';
  return (
    <span
      dir="auto"
      aria-hidden="true"
      className={`${sizeClass[size]} inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full ${tint}`}
    >
      {initial || (
        <svg
          width="20"
          height="20"
          viewBox="0 0 20 20"
          fill="none"
          stroke="currentColor"
          aria-hidden="true"
          focusable="false"
        >
          <circle cx="10" cy="6" r="3" />
          <path d="M4 18v-2a6 6 0 0 1 12 0v2" />
        </svg>
      )}
    </span>
  );
}

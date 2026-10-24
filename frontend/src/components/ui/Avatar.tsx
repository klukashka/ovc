import { type ReactElement, useState } from 'react';

const sizeClass = {
  small: 'size-8 text-small',
  large: 'size-14 text-heading',
} as const;

export function Avatar({
  name,
  avatarId,
  size = 'small',
}: {
  readonly name: string;
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
  const initial = Array.from(name.trim()).at(0)?.toUpperCase() ?? '';
  return (
    <span
      aria-hidden="true"
      className={`${sizeClass[size]} inline-flex shrink-0 items-center justify-center rounded-full bg-hover text-text-muted`}
    >
      {initial}
    </span>
  );
}

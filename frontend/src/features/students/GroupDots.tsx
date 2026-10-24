import type { ReactElement } from 'react';

import type { StudentCard } from './api.ts';

type Colour = StudentCard['groups'][number]['colour'];

const dotColour: Readonly<Record<Colour, string>> = {
  clay: 'bg-clay',
  slate: 'bg-slate',
  moss: 'bg-moss',
  plum: 'bg-plum',
  ochre: 'bg-ochre',
  teal: 'bg-teal',
  rust: 'bg-rust',
  ink: 'bg-ink',
};

export function GroupDots({ groups }: { readonly groups: StudentCard['groups'] }): ReactElement | null {
  if (groups.length === 0) {
    return null;
  }
  return (
    <ul className="flex flex-wrap gap-x-3 gap-y-1 text-small text-text-muted">
      {groups.map((group) => (
        <li key={group.id} className="inline-flex items-center gap-1.5">
          <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${dotColour[group.colour]}`} />
          {group.name}
        </li>
      ))}
    </ul>
  );
}

import type { ReactElement } from 'react';

import { paletteDotClass } from '../../components/ui/palette.ts';
import type { StudentCard } from './api.ts';

export function GroupDots({ groups }: { readonly groups: StudentCard['groups'] }): ReactElement | null {
  if (groups.length === 0) {
    return null;
  }
  return (
    <ul className="flex min-w-0 flex-wrap gap-x-3 gap-y-1 text-small text-text-muted">
      {groups.map((group) => (
        <li key={group.id} className="inline-flex min-w-0 items-center gap-1.5">
          <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[group.colour]}`} />
          <span className="min-w-0 [overflow-wrap:anywhere]">{group.name}</span>
        </li>
      ))}
    </ul>
  );
}

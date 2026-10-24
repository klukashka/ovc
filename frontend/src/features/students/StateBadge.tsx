import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { type Tone, StatusWord } from '../../components/ui/StatusWord.tsx';
import type { RelationshipState } from './api.ts';

type BadgedState = Exclude<RelationshipState, 'active'>;

const tones: Readonly<Record<BadgedState, Tone>> = {
  invited: 'info',
  provisional: 'warning',
  reclaim_pending: 'info',
  ended: 'muted',
};

export function StateBadge({ state }: { readonly state: RelationshipState }): ReactElement | null {
  const { t } = useTranslation();
  if (state === 'active') {
    return null;
  }
  return <StatusWord tone={tones[state]}>{t(`relationshipState.${state}`)}</StatusWord>;
}

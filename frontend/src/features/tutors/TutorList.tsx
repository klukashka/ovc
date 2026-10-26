import { type UseQueryResult, useQuery } from '@tanstack/react-query';
import { type ReactElement, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { ActionMenu } from '../../components/ui/ActionMenu.tsx';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { MoreIcon } from '../../components/ui/icons.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { LoadFailed } from '../../components/ui/LoadState.tsx';
import { paletteDotClass } from '../../components/ui/palette.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { type MyTutor, fetchMyTutors, myTutorsKey } from './api.ts';

export function useMyTutors(): UseQueryResult<readonly MyTutor[]> {
  const client = useApiClient();
  return useQuery({
    queryKey: myTutorsKey,
    queryFn: () => fetchMyTutors(client),
    retry: false,
  });
}

export function TutorsLoadFailed({ query }: { readonly query: UseQueryResult }): ReactElement {
  return (
    <LoadFailed
      error={asApiError(query.error)}
      onRetry={() => {
        void query.refetch();
      }}
    />
  );
}

function EndedLine({ tutor }: { readonly tutor: MyTutor }): ReactElement | null {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  if (tutor.account_closed_until !== null) {
    return (
      <p className="text-small text-text-muted">
        {t('tutors.accountClosed', { date: formatDate(tutor.account_closed_until, me.language, me.timezone) })}
      </p>
    );
  }
  if (tutor.ended_at !== null) {
    return (
      <p className="text-small text-text-muted">
        {t('tutors.endedOn', { date: formatDate(tutor.ended_at, me.language, me.timezone) })}
      </p>
    );
  }
  return null;
}

function TutorRow({
  tutor,
  withBio,
  onStopStudying,
}: {
  readonly tutor: MyTutor;
  readonly withBio: boolean;
  readonly onStopStudying: ((tutor: MyTutor) => void) | undefined;
}): ReactElement {
  const { t } = useTranslation();
  const profile = tutor.tutor;
  return (
    <li data-row={tutor.relationship_id} className="flex items-start gap-3 border-t border-border py-3">
      <Avatar name={profile.name} avatarId={profile.avatar_id} />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="inline-flex items-center gap-2 text-strong">
          <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[tutor.colour]}`} />
          {profile.name}
        </p>
        {profile.subjects.length === 0 ? null : (
          <p className="text-small text-text-muted">{profile.subjects.join(', ')}</p>
        )}
        {withBio && profile.bio !== '' ? <p className="text-small whitespace-pre-line">{profile.bio}</p> : null}
        <EndedLine tutor={tutor} />
      </div>
      {onStopStudying === undefined || tutor.state === 'ended' ? null : (
        <ActionMenu
          label={t('tutors.more', { name: profile.name })}
          unavailable={false}
          items={[
            {
              label: t('tutors.stopStudying'),
              onChoose: (): void => {
                onStopStudying(tutor);
              },
            },
          ]}
        >
          <MoreIcon />
        </ActionMenu>
      )}
    </li>
  );
}

export function TutorList({
  tutors,
  label,
  withBio,
  onStopStudying,
}: {
  readonly tutors: readonly MyTutor[];
  readonly label: string;
  readonly withBio: boolean;
  readonly onStopStudying?: (tutor: MyTutor) => void;
}): ReactElement {
  const listRef = useRef<HTMLUListElement>(null);
  useFocusAfterRemovedRow(
    listRef,
    tutors.map((tutor) => tutor.relationship_id),
    'button',
  );
  return (
    <ul ref={listRef} aria-label={label} className="w-full border-b border-border">
      {tutors.map((tutor) => (
        <TutorRow key={tutor.relationship_id} tutor={tutor} withBio={withBio} onStopStudying={onStopStudying} />
      ))}
    </ul>
  );
}

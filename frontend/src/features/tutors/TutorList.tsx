import { useQuery } from '@tanstack/react-query';
import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { Avatar } from '../../components/ui/Avatar.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { paletteDotClass } from '../../components/ui/palette.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { type MyTutor, currentTutors, fetchMyTutors, myTutorsKey } from './api.ts';

function TutorRow({ tutor, withBio }: { readonly tutor: MyTutor; readonly withBio: boolean }): ReactElement {
  const profile = tutor.tutor;
  return (
    <li className="flex items-start gap-3 border-t border-border py-3">
      <Avatar name={profile.name} avatarId={profile.avatar_id} />
      <div className="flex min-w-0 flex-col gap-1">
        <p className="inline-flex items-center gap-2 text-strong">
          <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[tutor.colour]}`} />
          {profile.name}
        </p>
        {profile.subjects.length === 0 ? null : (
          <p className="text-small text-text-muted">{profile.subjects.join(', ')}</p>
        )}
        {withBio && profile.bio !== '' ? <p className="text-small whitespace-pre-line">{profile.bio}</p> : null}
      </div>
    </li>
  );
}

export function TutorList({ withBio }: { readonly withBio: boolean }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const query = useQuery({
    queryKey: myTutorsKey,
    queryFn: ({ signal }) => fetchMyTutors(client, signal),
    retry: false,
  });
  if (query.isPending) {
    return <DelayedLoading />;
  }
  if (query.isError) {
    return (
      <LoadFailed
        error={asApiError(query.error)}
        onRetry={() => {
          void query.refetch();
        }}
      />
    );
  }
  const tutors = currentTutors(query.data);
  if (tutors.length === 0) {
    return <p className="text-text-muted">{t('tutors.empty')}</p>;
  }
  return (
    <ul aria-label={t('tutors.list')} className="w-full border-b border-border">
      {tutors.map((tutor) => (
        <TutorRow key={tutor.relationship_id} tutor={tutor} withBio={withBio} />
      ))}
    </ul>
  );
}

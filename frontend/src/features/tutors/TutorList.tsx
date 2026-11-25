import { type UseQueryResult, useQuery } from '@tanstack/react-query';
import { type ReactElement, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { ActionMenu } from '../../components/ui/ActionMenu.tsx';
import { Avatar } from '../../components/ui/Avatar.tsx';
import {
  DataTable,
  type TableColumn,
  cellClass,
  endCellClass,
  narrowOnlyClass,
  primaryCellClass,
  secondaryCellClass,
  tableRowClass,
} from '../../components/ui/DataTable.tsx';
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
        <p className="flex min-w-0 items-center gap-2 text-strong">
          <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[tutor.colour]}`} />
          <span dir="auto" className="min-w-0 break-words">
            {profile.name}
          </span>
        </p>
        {profile.subjects.length === 0 ? null : (
          <p dir="auto" className="text-small text-text-muted">
            {profile.subjects.join(', ')}
          </p>
        )}
        {withBio && profile.bio !== '' ? <p className="text-small whitespace-pre-line">{profile.bio}</p> : null}
        <EndedLine tutor={tutor} />
        {tutor.channel_id === null ? null : (
          <Link
            to={`/messages/${encodeURIComponent(tutor.channel_id)}`}
            className="self-start text-accent-text underline max-tablet:inline-flex max-tablet:min-h-11 max-tablet:items-center"
          >
            {t('section.messages')}
          </Link>
        )}
      </div>
      {onStopStudying === undefined || tutor.state === 'ended' ? null : (
        <ActionMenu
          label={t('tutors.more', { name: profile.name })}
          unavailable={false}
          items={[
            {
              label: t('tutors.stopStudying'),
              danger: true,
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

function TutorCell({ tutor, withBio }: { readonly tutor: MyTutor; readonly withBio: boolean }): ReactElement {
  const profile = tutor.tutor;
  const subjects = profile.subjects.join(', ');
  return (
    <th scope="row" className={primaryCellClass}>
      <span className="flex min-w-0 items-start gap-3">
        <Avatar name={profile.name} avatarId={profile.avatar_id} />
        <span className="flex min-w-0 flex-col gap-1">
          <span className="inline-flex min-w-0 items-center gap-2 text-strong">
            <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[tutor.colour]}`} />
            <span dir="auto" className="min-w-0">
              {profile.name}
            </span>
          </span>
          {subjects === '' ? null : (
            <span dir="auto" className={`text-small text-text-muted ${narrowOnlyClass}`}>
              {subjects}
            </span>
          )}
          {withBio && profile.bio !== '' ? (
            <span dir="auto" title={profile.bio} className="line-clamp-2 text-small font-normal whitespace-pre-line">
              {profile.bio}
            </span>
          ) : null}
        </span>
      </span>
    </th>
  );
}

function MessagesLink({ tutor }: { readonly tutor: MyTutor }): ReactElement | null {
  const { t } = useTranslation();
  if (tutor.channel_id === null) {
    return null;
  }
  return (
    <Link
      to={`/messages/${encodeURIComponent(tutor.channel_id)}`}
      className="inline-flex min-h-11 items-center px-2 text-small whitespace-nowrap text-accent-text underline"
    >
      {t('section.messages')}
    </Link>
  );
}

function SubjectsCell({ tutor }: { readonly tutor: MyTutor }): ReactElement {
  return (
    <td className={secondaryCellClass}>
      <span dir="auto" className="text-small text-text-muted">
        {tutor.tutor.subjects.join(', ')}
      </span>
    </td>
  );
}

export function CurrentTutorTable({
  tutors,
  label,
  onStopStudying,
}: {
  readonly tutors: readonly MyTutor[];
  readonly label: string;
  readonly onStopStudying: (tutor: MyTutor) => void;
}): ReactElement {
  const { t } = useTranslation();
  const bodyRef = useRef<HTMLTableSectionElement>(null);
  useFocusAfterRemovedRow(
    bodyRef,
    tutors.map((tutor) => tutor.relationship_id),
    'button',
  );
  const columns: readonly TableColumn[] = [
    { key: 'tutor', label: t('tutors.columns.tutor'), width: 'w-2/3 tablet:w-6/12' },
    { key: 'subjects', label: t('tutors.columns.subjects'), secondary: true, width: 'w-3/12' },
    { key: 'actions', label: t('tutors.columns.actions'), visuallyHidden: true, width: 'w-1/3 tablet:w-3/12' },
  ];
  return (
    <DataTable label={label} columns={columns} bodyRef={bodyRef}>
      {tutors.map((tutor) => (
        <tr key={tutor.relationship_id} data-row={tutor.relationship_id} className={tableRowClass}>
          <TutorCell tutor={tutor} withBio />
          <SubjectsCell tutor={tutor} />
          <td className={endCellClass}>
            <span className="flex flex-wrap items-center justify-end gap-x-2">
              <MessagesLink tutor={tutor} />
              {tutor.state === 'ended' ? null : (
                <ActionMenu
                  label={t('tutors.more', { name: tutor.tutor.name })}
                  unavailable={false}
                  items={[
                    {
                      label: t('tutors.stopStudying'),
                      danger: true,
                      onChoose: (): void => {
                        onStopStudying(tutor);
                      },
                    },
                  ]}
                >
                  <MoreIcon />
                </ActionMenu>
              )}
            </span>
          </td>
        </tr>
      ))}
    </DataTable>
  );
}

export function PastTutorTable({
  tutors,
  label,
}: {
  readonly tutors: readonly MyTutor[];
  readonly label: string;
}): ReactElement {
  const { t } = useTranslation();
  const bodyRef = useRef<HTMLTableSectionElement>(null);
  useFocusAfterRemovedRow(
    bodyRef,
    tutors.map((tutor) => tutor.relationship_id),
    'a',
  );
  const columns: readonly TableColumn[] = [
    { key: 'tutor', label: t('tutors.columns.tutor'), width: 'w-1/2 tablet:w-4/12' },
    { key: 'subjects', label: t('tutors.columns.subjects'), secondary: true, width: 'w-3/12' },
    { key: 'ended', label: t('tutors.columns.ended'), width: 'w-1/4 tablet:w-3/12' },
    { key: 'actions', label: t('tutors.columns.actions'), visuallyHidden: true, width: 'w-1/4 tablet:w-2/12' },
  ];
  return (
    <DataTable label={label} columns={columns} bodyRef={bodyRef}>
      {tutors.map((tutor) => (
        <tr key={tutor.relationship_id} data-row={tutor.relationship_id} className={tableRowClass}>
          <TutorCell tutor={tutor} withBio={false} />
          <SubjectsCell tutor={tutor} />
          <td className={cellClass}>
            <EndedLine tutor={tutor} />
          </td>
          <td className={endCellClass}>
            <MessagesLink tutor={tutor} />
          </td>
        </tr>
      ))}
    </DataTable>
  );
}
